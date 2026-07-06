package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
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

func (s *processAuditAndCompliancesrvc) archiveIntegrityTrailEntries(
	ctx context.Context,
	entry db.ContractArchiveEntry,
	entryIndex int,
	archiveStoreEvents []datatype.AuditLogEntry,
	notaryEvents map[string][]archiveNotaryEvent,
) (*processauditandcompliance.PACResourceAuditTrailEntry, error) {
	if !entry.ContractSnapshot.IsNotNullValue() {
		return nil, fmt.Errorf("archive entry %s#%d has empty contract_snapshot", entry.DID, entry.ContractVersion)
	}
	if !archiveContentHashPattern.MatchString(entry.ContentHash) {
		return nil, fmt.Errorf("archive entry %s#%d has invalid content_hash", entry.DID, entry.ContractVersion)
	}
	calculatedHash, err := cwecommand.HashArchiveSnapshot(entry.ContractSnapshot)
	if err != nil {
		return nil, fmt.Errorf("archive entry %s#%d has invalid contract_snapshot JSON: %w", entry.DID, entry.ContractVersion, err)
	}
	if calculatedHash != entry.ContentHash {
		return nil, fmt.Errorf("archive entry %s#%d content_hash mismatch", entry.DID, entry.ContractVersion)
	}
	if strings.TrimSpace(entry.SnapshotCID) == "" {
		return nil, fmt.Errorf("archive entry %s#%d has empty snapshot_cid", entry.DID, entry.ContractVersion)
	}
	if s.ATrailReader.IPFSClient == nil {
		return nil, fmt.Errorf("IPFS client is required for archive audit")
	}
	ipfsResult, err := s.ATrailReader.IPFSClient.FetchFile(entry.SnapshotCID)
	if err != nil {
		return nil, fmt.Errorf("fetch archive snapshot from IPFS: %w", err)
	}
	ipfsHash, err := cwecommand.HashArchiveSnapshot(datatype.JSON(ipfsResult.Data))
	if err != nil {
		return nil, fmt.Errorf("archive entry %s#%d has invalid IPFS snapshot JSON: %w", entry.DID, entry.ContractVersion, err)
	}
	if ipfsHash != entry.ContentHash {
		return nil, fmt.Errorf("archive entry %s#%d IPFS snapshot hash mismatch", entry.DID, entry.ContractVersion)
	}
	if !jsonSemanticallyEqual(entry.ContractSnapshot, ipfsResult.Data) {
		return nil, fmt.Errorf("archive entry %s#%d IPFS snapshot JSON mismatch", entry.DID, entry.ContractVersion)
	}

	archiveEntryID := archiveNotaryEntryID(entry.DID, entry.ContractVersion)
	storeEvent, receipt, eventTSAReceipt, err := findArchiveStoreEvent(entry, archiveEntryID, archiveStoreEvents)
	if err != nil {
		return nil, err
	}
	notaryEventCandidates := notaryEvents[archiveEntryID]
	if len(notaryEventCandidates) == 0 {
		return nil, fmt.Errorf("ORCE archive audit log does not contain archiveEntryId %s", archiveEntryID)
	}
	notaryEvent, err := selectArchiveNotaryEvent(archiveEntryID, receipt, notaryEventCandidates)
	if err != nil {
		return nil, err
	}
	if notaryEvent.ContentHash != entry.ContentHash || notaryEvent.SnapshotCID != entry.SnapshotCID {
		return nil, fmt.Errorf("ORCE archive audit log event does not match archive entry %s", archiveEntryID)
	}
	if notaryEvent.EventType != "ARCHIVE_STORED" || notaryEvent.DID != entry.DID || notaryEvent.ContractVersion != entry.ContractVersion {
		return nil, fmt.Errorf("ORCE archive audit log event identity mismatch for %s", archiveEntryID)
	}
	if receipt.ArchiveEntryID != archiveEntryID || receipt.EventHash != notaryEvent.EventHash || !stringPtrsEqual(receipt.PreviousHash, notaryEvent.PreviousHash) || !archiveTimestampsEqual(receipt.ReceivedAt, notaryEvent.ReceivedAt) {
		return nil, fmt.Errorf("archive notary receipt does not match ORCE audit log for %s", archiveEntryID)
	}

	tsaVerified := false
	tsaGeneratedAt := ""
	tsaPolicy := ""
	tsaSerialNumber := ""
	tsaReceipt, err := readArchiveTSAReceipt(entry, eventTSAReceipt)
	if err != nil {
		return nil, err
	}
	if tsaReceipt != nil {
		storedAt, err := time.Parse(time.RFC3339Nano, notaryEvent.StoredAt)
		if err != nil {
			return nil, fmt.Errorf("ORCE archive audit log event has invalid storedAt for %s: %w", archiveEntryID, err)
		}
		notaryReceivedAt, err := time.Parse(time.RFC3339Nano, notaryEvent.ReceivedAt)
		if err != nil {
			return nil, fmt.Errorf("ORCE archive audit log event has invalid receivedAt for %s: %w", archiveEntryID, err)
		}
		evidence, err := cwecommand.BuildArchiveTimestampEvidence(cwecommand.ArchiveNotaryPayload{
			EventType:       notaryEvent.EventType,
			ArchiveEntryID:  notaryEvent.ArchiveEntryID,
			DID:             notaryEvent.DID,
			ContractVersion: notaryEvent.ContractVersion,
			ContentHash:     notaryEvent.ContentHash,
			SnapshotCID:     notaryEvent.SnapshotCID,
			StoredBy:        notaryEvent.StoredBy,
			StoredAt:        storedAt,
		}, &cwecommand.ArchiveNotaryReceipt{
			ReceiptType:    receipt.ReceiptType,
			ArchiveEntryID: receipt.ArchiveEntryID,
			EventHash:      receipt.EventHash,
			PreviousHash:   receipt.PreviousHash,
			ReceivedAt:     notaryReceivedAt,
		})
		if err != nil {
			return nil, err
		}
		evidenceBytes, err := cwecommand.CanonicalArchiveTimestampEvidence(evidence)
		if err != nil {
			return nil, err
		}
		ts, err := tsa.VerifyReceipt(tsa.Receipt{
			Token:          tsaReceipt.Token,
			TokenEncoding:  tsaReceipt.TokenEncoding,
			HashAlgorithm:  tsaReceipt.HashAlgorithm,
			MessageImprint: tsaReceipt.MessageImprint,
			GeneratedAt:    tsaReceipt.GeneratedAt,
			Policy:         tsaReceipt.Policy,
			SerialNumber:   tsaReceipt.SerialNumber,
		}, evidenceBytes)
		if err != nil {
			return nil, fmt.Errorf("archive TSA receipt verification failed for %s: %w", archiveEntryID, err)
		}
		if ts.Time.Before(storedAt) {
			return nil, fmt.Errorf("archive TSA timestamp precedes storedAt for %s", archiveEntryID)
		}
		tsaVerified = true
		tsaGeneratedAt = ts.Time.UTC().Format(time.RFC3339Nano)
		if len(ts.Policy) > 0 {
			tsaPolicy = ts.Policy.String()
		}
		if ts.SerialNumber != nil {
			tsaSerialNumber = ts.SerialNumber.String()
		}
	}

	did := entry.DID
	now := time.Now().UTC().Format(time.RFC3339)
	return &processauditandcompliance.PACResourceAuditTrailEntry{
		ID:        int64(-5100000 - entryIndex),
		Component: componenttype.ContractStorageArchive.String(),
		EventType: "ARCHIVE_INTEGRITY_AUDIT_CHECK",
		EventData: map[string]any{
			"objectType":            "contractArchiveEntry",
			"objectDid":             entry.DID,
			"contractVersion":       entry.ContractVersion,
			"archiveEntryId":        archiveEntryID,
			"contentHashVerified":   true,
			"ipfsSnapshotVerified":  true,
			"notaryReceiptVerified": true,
			"orceChainVerified":     true,
			"storeEventId":          storeEvent.ID,
			"notaryEventHash":       notaryEvent.EventHash,
			"notaryPreviousHash":    notaryEvent.PreviousHash,
			"notaryReceivedAt":      notaryEvent.ReceivedAt,
			"tsaTimestampVerified":  tsaVerified,
			"tsaGeneratedAt":        tsaGeneratedAt,
			"tsaPolicy":             tsaPolicy,
			"tsaSerialNumber":       tsaSerialNumber,
			"snapshotHashAlgorithm": "SHA-256",
			"checkedAt":             now,
		},
		Did:       &did,
		CreatedAt: now,
	}, nil
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
