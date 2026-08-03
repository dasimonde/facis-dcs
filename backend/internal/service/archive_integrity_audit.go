package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/tsa"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

var archiveContentHashPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type archiveNotaryChainReader struct {
	url         string
	bearerToken string
	httpClient  *http.Client
}

type archiveNotaryEvent struct {
	EventType       string  `json:"eventType"`
	DID             string  `json:"did"`
	ContractVersion int     `json:"contractVersion"`
	ArchiveEntryID  string  `json:"archiveEntryId"`
	ContentHash     string  `json:"contentHash"`
	SnapshotCID     string  `json:"snapshotCid"`
	StoredBy        string  `json:"storedBy"`
	StoredAt        string  `json:"storedAt"`
	ReceivedAt      string  `json:"receivedAt"`
	PreviousHash    *string `json:"previousHash"`
	EventHash       string  `json:"eventHash"`
}

type archiveNotaryReceiptData struct {
	ReceiptType    string  `json:"receiptType"`
	ArchiveEntryID string  `json:"archiveEntryId"`
	EventHash      string  `json:"eventHash"`
	PreviousHash   *string `json:"previousHash"`
	ReceivedAt     string  `json:"receivedAt"`
}

type archiveTSAReceiptData struct {
	ReceiptType    string    `json:"receipt_type"`
	Token          string    `json:"token"`
	TokenEncoding  string    `json:"token_encoding"`
	HashAlgorithm  string    `json:"hash_algorithm"`
	MessageImprint string    `json:"message_imprint"`
	GeneratedAt    time.Time `json:"generated_at"`
	Policy         string    `json:"policy,omitempty"`
	SerialNumber   string    `json:"serial_number,omitempty"`
}

type archiveStoreEventData struct {
	DID             string                    `json:"did"`
	ContractVersion int                       `json:"contract_version"`
	StoredBy        string                    `json:"stored_by"`
	ContentHash     string                    `json:"content_hash"`
	SnapshotCID     string                    `json:"snapshot_cid"`
	ArchiveStatus   string                    `json:"archive_status"`
	NotaryReceipt   *archiveNotaryReceiptData `json:"notary_receipt"`
	TSAReceipt      *archiveTSAReceiptData    `json:"tsa_receipt"`
}

func newArchiveNotaryChainReaderFromEnv() (*archiveNotaryChainReader, error) {
	url := strings.TrimSpace(os.Getenv("ORCE_ARCHIVE_AUDIT_LOG_URL"))
	if url == "" {
		return nil, fmt.Errorf("ORCE_ARCHIVE_AUDIT_LOG_URL is required for archive audit")
	}
	token := strings.TrimSpace(os.Getenv("ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN is required for archive audit")
	}
	return &archiveNotaryChainReader{
		url:         url,
		bearerToken: token,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (r *archiveNotaryChainReader) Read(ctx context.Context) (map[string][]archiveNotaryEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create ORCE archive audit log request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.bearerToken)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ORCE archive audit log: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch ORCE archive audit log returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseArchiveNotaryChain(resp.Body)
}

func parseArchiveNotaryChain(reader io.Reader) (map[string][]archiveNotaryEvent, error) {
	events := map[string][]archiveNotaryEvent{}
	scanner := bufio.NewScanner(reader)
	var previousHash *string
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event archiveNotaryEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("decode ORCE archive audit log line %d: %w", lineNumber, err)
		}
		if event.ArchiveEntryID == "" {
			return nil, fmt.Errorf("ORCE archive audit log line %d has empty archiveEntryId", lineNumber)
		}
		if event.EventHash == "" {
			return nil, fmt.Errorf("ORCE archive audit log line %d has empty eventHash", lineNumber)
		}
		if !stringPtrsEqual(previousHash, event.PreviousHash) {
			return nil, fmt.Errorf("ORCE archive audit log line %d has invalid previousHash", lineNumber)
		}
		calculated := hashArchiveNotaryEvent(event)
		if calculated != event.EventHash {
			return nil, fmt.Errorf("ORCE archive audit log line %d has invalid eventHash", lineNumber)
		}
		hash := event.EventHash
		previousHash = &hash
		events[event.ArchiveEntryID] = append(events[event.ArchiveEntryID], event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ORCE archive audit log: %w", err)
	}
	return events, nil
}

func hashArchiveNotaryEvent(event archiveNotaryEvent) string {
	payload := fmt.Sprintf(
		`{"eventType":%s,"did":%s,"contractVersion":%d,"archiveEntryId":%s,"contentHash":%s,"snapshotCid":%s,"storedBy":%s,"storedAt":%s,"receivedAt":%s,"previousHash":%s}`,
		mustJSONString(event.EventType),
		mustJSONString(event.DID),
		event.ContractVersion,
		mustJSONString(event.ArchiveEntryID),
		mustJSONString(event.ContentHash),
		mustJSONString(event.SnapshotCID),
		mustJSONString(event.StoredBy),
		mustJSONString(event.StoredAt),
		mustJSONString(event.ReceivedAt),
		mustJSONNullableString(event.PreviousHash),
	)
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func mustJSONString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func mustJSONNullableString(value *string) string {
	if value == nil {
		return "null"
	}
	return mustJSONString(*value)
}

func stringPtrsEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func archiveTimestampsEqual(a, b string) bool {
	aTime, err := time.Parse(time.RFC3339Nano, a)
	if err != nil {
		return false
	}
	bTime, err := time.Parse(time.RFC3339Nano, b)
	if err != nil {
		return false
	}
	return aTime.Equal(bTime)
}

func selectArchiveNotaryEvent(archiveEntryID string, receipt *archiveNotaryReceiptData, events []archiveNotaryEvent) (archiveNotaryEvent, error) {
	if receipt == nil {
		return archiveNotaryEvent{}, fmt.Errorf("archive notary receipt is required for %s", archiveEntryID)
	}
	for _, event := range events {
		if receipt.ArchiveEntryID == archiveEntryID &&
			receipt.EventHash == event.EventHash &&
			stringPtrsEqual(receipt.PreviousHash, event.PreviousHash) &&
			archiveTimestampsEqual(receipt.ReceivedAt, event.ReceivedAt) {
			return event, nil
		}
	}
	return archiveNotaryEvent{}, fmt.Errorf("ORCE archive audit log has no event matching stored notary receipt for %s", archiveEntryID)
}

var archiveIntegrityRules = []string{
	"ARCHIVE_DB_SNAPSHOT",
	"ARCHIVE_CONTENT_HASH",
	"ARCHIVE_IPFS_SNAPSHOT",
	"ARCHIVE_ORCE_RECEIPT",
	"ARCHIVE_ORCE_CHAIN",
	"ARCHIVE_TSA_RFC3161",
}

// archiveIntegrityTrailEntries turns validation into independent, stable
// findings. A corrupt archive entry is audit evidence, not an endpoint error.
func (s *processAuditAndCompliancesrvc) archiveIntegrityTrailEntries(
	ctx context.Context,
	entry db.ContractArchiveEntry,
	entryIndex int,
	archiveStoreEvents []datatype.AuditLogEntry,
	notaryEvents map[string][]archiveNotaryEvent,
	chainErr error,
) []*processauditandcompliance.PACResourceAuditTrailEntry {
	checks := s.evaluateArchiveIntegrityChecks(ctx, entry, archiveStoreEvents, notaryEvents, chainErr)
	now := time.Now().UTC().Format(time.RFC3339)
	did := entry.DID
	findings := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0, len(archiveIntegrityRules))
	for ruleIndex, rule := range archiveIntegrityRules {
		check := checks[rule]
		result, reason := "PASSED", "Integrity evidence verified"
		if check != nil {
			result, reason = "FAILED", check.Error()
		}
		kind := "CHECK"
		ruleID := rule
		resultValue := result
		reasonValue := reason
		findings = append(findings, &processauditandcompliance.PACResourceAuditTrailEntry{
			ID: int64(-5100000 - entryIndex*100 - ruleIndex), Component: componenttype.ContractStorageArchive.String(),
			EventType: "ARCHIVE_INTEGRITY_AUDIT_CHECK", Did: &did, CreatedAt: now,
			Kind: &kind, Result: &resultValue, RuleID: &ruleID, Reason: &reasonValue,
			EventData: map[string]any{"kind": kind, "result": result, "rule_id": rule, "ruleId": rule, "reason": reason, "message": reason, "severity": result, "archiveEntryId": archiveNotaryEntryID(entry.DID, entry.ContractVersion)},
		})
	}
	return findings
}

func (s *processAuditAndCompliancesrvc) evaluateArchiveIntegrityChecks(ctx context.Context, entry db.ContractArchiveEntry, archiveStoreEvents []datatype.AuditLogEntry, notaryEvents map[string][]archiveNotaryEvent, chainErr error) map[string]error {
	checks := make(map[string]error, len(archiveIntegrityRules))
	if !entry.ContractSnapshot.IsNotNullValue() {
		checks["ARCHIVE_DB_SNAPSHOT"] = errors.New("contract_snapshot is empty")
	} else if _, err := cwecommand.CanonicalizeArchiveSnapshot(entry.ContractSnapshot); err != nil {
		checks["ARCHIVE_DB_SNAPSHOT"] = fmt.Errorf("contract_snapshot is invalid: %w", err)
	}
	if !archiveContentHashPattern.MatchString(entry.ContentHash) {
		checks["ARCHIVE_CONTENT_HASH"] = errors.New("content_hash has invalid SHA-256 format")
	} else if checks["ARCHIVE_DB_SNAPSHOT"] != nil {
		checks["ARCHIVE_CONTENT_HASH"] = errors.New("content hash cannot be verified because DB snapshot is invalid")
	} else if calculated, err := cwecommand.HashArchiveSnapshot(entry.ContractSnapshot); err != nil || calculated != entry.ContentHash {
		checks["ARCHIVE_CONTENT_HASH"] = errors.New("content_hash does not match DB snapshot")
	}
	if strings.TrimSpace(entry.SnapshotCID) == "" {
		checks["ARCHIVE_IPFS_SNAPSHOT"] = errors.New("snapshot_cid is empty")
	} else if s.ATrailReader.Artifacts == nil {
		checks["ARCHIVE_IPFS_SNAPSHOT"] = errors.New("artifact store is unavailable")
	} else if fetched, err := s.ATrailReader.Artifacts.Get(ctx, artifactstore.ContractScope(entry.DID), entry.SnapshotCID); err != nil {
		checks["ARCHIVE_IPFS_SNAPSHOT"] = fmt.Errorf("IPFS snapshot fetch failed: %w", err)
	} else if hash, err := cwecommand.HashArchiveSnapshot(datatype.JSON(fetched)); err != nil || hash != entry.ContentHash || !jsonSemanticallyEqual(entry.ContractSnapshot, fetched) {
		checks["ARCHIVE_IPFS_SNAPSHOT"] = errors.New("IPFS snapshot does not match archived DB snapshot and content hash")
	}
	archiveEntryID := archiveNotaryEntryID(entry.DID, entry.ContractVersion)
	_, receipt, eventTSAReceipt, receiptErr := findArchiveStoreEvent(entry, archiveEntryID, archiveStoreEvents)
	if receiptErr != nil {
		checks["ARCHIVE_ORCE_RECEIPT"] = receiptErr
	}
	candidates := notaryEvents[archiveEntryID]
	if chainErr != nil {
		checks["ARCHIVE_ORCE_CHAIN"] = chainErr
	} else if len(candidates) == 0 {
		checks["ARCHIVE_ORCE_CHAIN"] = fmt.Errorf("ORCE chain has no event for %s", archiveEntryID)
	}
	if receiptErr == nil && checks["ARCHIVE_ORCE_CHAIN"] == nil {
		if _, err := selectArchiveNotaryEvent(archiveEntryID, receipt, candidates); err != nil {
			checks["ARCHIVE_ORCE_RECEIPT"] = err
		}
	}
	if err := verifyArchiveTSAEvidence(entry, receipt, eventTSAReceipt, candidates); err != nil {
		checks["ARCHIVE_TSA_RFC3161"] = err
	}
	return checks
}

func verifyArchiveTSAEvidence(entry db.ContractArchiveEntry, receipt *archiveNotaryReceiptData, eventReceipt *archiveTSAReceiptData, candidates []archiveNotaryEvent) error {
	archiveEntryID := archiveNotaryEntryID(entry.DID, entry.ContractVersion)
	if receipt == nil {
		return errors.New("TSA cannot be verified without archive notary receipt")
	}
	notaryEvent, err := selectArchiveNotaryEvent(archiveEntryID, receipt, candidates)
	if err != nil {
		return fmt.Errorf("TSA cannot be verified: %w", err)
	}
	tsaReceipt, err := readArchiveTSAReceipt(entry, eventReceipt)
	if err != nil {
		return err
	}
	if tsaReceipt == nil {
		return errors.New("archive TSA RFC-3161 receipt is missing")
	}
	storedAt, err := time.Parse(time.RFC3339Nano, notaryEvent.StoredAt)
	if err != nil {
		return fmt.Errorf("invalid ORCE storedAt: %w", err)
	}
	receivedAt, err := time.Parse(time.RFC3339Nano, notaryEvent.ReceivedAt)
	if err != nil {
		return fmt.Errorf("invalid ORCE receivedAt: %w", err)
	}
	evidence, err := cwecommand.BuildArchiveTimestampEvidence(cwecommand.ArchiveNotaryPayload{EventType: notaryEvent.EventType, ArchiveEntryID: notaryEvent.ArchiveEntryID, DID: notaryEvent.DID, ContractVersion: notaryEvent.ContractVersion, ContentHash: notaryEvent.ContentHash, SnapshotCID: notaryEvent.SnapshotCID, StoredBy: notaryEvent.StoredBy, StoredAt: storedAt}, &cwecommand.ArchiveNotaryReceipt{ReceiptType: receipt.ReceiptType, ArchiveEntryID: receipt.ArchiveEntryID, EventHash: receipt.EventHash, PreviousHash: receipt.PreviousHash, ReceivedAt: receivedAt})
	if err != nil {
		return err
	}
	evidenceBytes, err := cwecommand.CanonicalArchiveTimestampEvidence(evidence)
	if err != nil {
		return err
	}
	stamp, err := tsa.VerifyReceipt(tsa.Receipt{Token: tsaReceipt.Token, TokenEncoding: tsaReceipt.TokenEncoding, HashAlgorithm: tsaReceipt.HashAlgorithm, MessageImprint: tsaReceipt.MessageImprint, GeneratedAt: tsaReceipt.GeneratedAt, Policy: tsaReceipt.Policy, SerialNumber: tsaReceipt.SerialNumber}, evidenceBytes)
	if err != nil {
		return fmt.Errorf("archive TSA receipt verification failed: %w", err)
	}
	if archiveTSATimestampPrecedesStoredAt(stamp.Time, storedAt) {
		return errors.New("archive TSA timestamp precedes storedAt")
	}
	return nil
}

func archiveTSATimestampPrecedesStoredAt(stampTime, storedAt time.Time) bool {
	// RFC 3161 tokens produced by the configured TSA encode whole seconds,
	// while the archive event may retain sub-second precision. Treat both
	// timestamps as simultaneous when they fall in the same UTC second.
	return stampTime.Before(storedAt.Truncate(time.Second))
}

func archiveIntegrityRuleForError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "tsa") || strings.Contains(message, "timestamp"):
		return "ARCHIVE_TSA_RFC3161"
	case strings.Contains(message, "receipt") || strings.Contains(message, "store event"):
		return "ARCHIVE_ORCE_RECEIPT"
	case strings.Contains(message, "orce") || strings.Contains(message, "previoushash") || strings.Contains(message, "eventhash"):
		return "ARCHIVE_ORCE_CHAIN"
	case strings.Contains(message, "ipfs") || strings.Contains(message, "snapshot_cid"):
		return "ARCHIVE_IPFS_SNAPSHOT"
	case strings.Contains(message, "content_hash") || strings.Contains(message, "content hash"):
		return "ARCHIVE_CONTENT_HASH"
	default:
		return "ARCHIVE_DB_SNAPSHOT"
	}
}

func jsonSemanticallyEqual(left datatype.JSON, right json.RawMessage) bool {
	var leftValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	var rightValue any
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	leftCanonical, err := json.Marshal(leftValue)
	if err != nil {
		return false
	}
	rightCanonical, err := json.Marshal(rightValue)
	if err != nil {
		return false
	}
	return bytes.Equal(leftCanonical, rightCanonical)
}

func findArchiveStoreEvent(entry db.ContractArchiveEntry, archiveEntryID string, auditEntries []datatype.AuditLogEntry) (datatype.AuditLogEntry, *archiveNotaryReceiptData, *archiveTSAReceiptData, error) {
	for _, auditEntry := range auditEntries {
		if auditEntry.EventType != eventtype.StoreArchived.String() {
			continue
		}
		var data archiveStoreEventData
		if err := json.Unmarshal(auditEntry.EventData, &data); err != nil {
			return datatype.AuditLogEntry{}, nil, nil, fmt.Errorf("decode archive store event %d: %w", auditEntry.ID, err)
		}
		if data.DID != entry.DID || data.ContractVersion != entry.ContractVersion || data.ContentHash != entry.ContentHash || data.SnapshotCID != entry.SnapshotCID {
			continue
		}
		if data.NotaryReceipt == nil {
			return datatype.AuditLogEntry{}, nil, nil, fmt.Errorf("archive store event for %s has no notary_receipt", archiveEntryID)
		}
		if data.NotaryReceipt.EventHash == "" {
			return datatype.AuditLogEntry{}, nil, nil, fmt.Errorf("archive store event for %s has empty notary eventHash", archiveEntryID)
		}
		return auditEntry, data.NotaryReceipt, data.TSAReceipt, nil
	}
	return datatype.AuditLogEntry{}, nil, nil, fmt.Errorf("archive store event for %s was not found in audit trail", archiveEntryID)
}

func archiveNotaryEntryID(did string, contractVersion int) string {
	return fmt.Sprintf("%s#%d", did, contractVersion)
}

func readArchiveTSAReceipt(entry db.ContractArchiveEntry, eventReceipt *archiveTSAReceiptData) (*archiveTSAReceiptData, error) {
	var dbReceipt *archiveTSAReceiptData
	if entry.TSAReceipt != nil && entry.TSAReceipt.IsNotNullValue() {
		bytes, err := entry.TSAReceipt.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("marshal archive TSA receipt for %s#%d: %w", entry.DID, entry.ContractVersion, err)
		}
		if string(bytes) != "{}" && string(bytes) != "null" {
			var parsed archiveTSAReceiptData
			if err := json.Unmarshal(bytes, &parsed); err != nil {
				return nil, fmt.Errorf("decode archive TSA receipt for %s#%d: %w", entry.DID, entry.ContractVersion, err)
			}
			if parsed.Token != "" {
				dbReceipt = &parsed
			}
		}
	}
	if dbReceipt == nil {
		return nil, fmt.Errorf("archive entry %s#%d has no tsa_receipt", entry.DID, entry.ContractVersion)
	}
	if eventReceipt == nil {
		return nil, fmt.Errorf("archive store event for %s#%d has no tsa_receipt", entry.DID, entry.ContractVersion)
	}
	if dbReceipt.Token != eventReceipt.Token || dbReceipt.MessageImprint != eventReceipt.MessageImprint {
		return nil, fmt.Errorf("archive TSA receipt does not match store event for %s#%d", entry.DID, entry.ContractVersion)
	}
	return dbReceipt, nil
}
