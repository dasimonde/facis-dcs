package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/tsa"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

// NotarizeArchiveEntry takes the archive notary receipt for entry and, when
// the TSA is enabled, an RFC 3161 timestamp over the notarized evidence,
// which it records as the entry's TSA receipt. Both receipts go into the
// STORE_ARCHIVED_CONTRACT event. A nil notary records no receipt. The signing
// workflow and the Archive Manager's store use it alike.
func NotarizeArchiveEntry(ctx context.Context, notary ArchiveNotary, timestamps ArchiveTimestampIssuer, entry *db.ContractArchiveEntry) (*contractevents.ArchiveNotaryReceipt, *contractevents.ArchiveTSAReceipt, error) {
	if notary == nil {
		return nil, nil, nil
	}
	notaryReceipt, tsaReceipt, tsaReceiptJSON, err := notarize(ctx, notary, timestamps, ArchiveNotaryPayload{
		EventType:       "ARCHIVE_STORED",
		ArchiveEntryID:  fmt.Sprintf("%s#%d", entry.DID, entry.ContractVersion),
		DID:             entry.DID,
		ContractVersion: entry.ContractVersion,
		ContentHash:     entry.ContentHash,
		SnapshotCID:     entry.SnapshotCID,
		StoredBy:        entry.StoredBy,
		StoredAt:        entry.StoredAt,
	})
	if err != nil {
		return nil, nil, err
	}
	entry.TSAReceipt = tsaReceiptJSON
	return notaryReceipt, tsaReceipt, nil
}

// notarize takes the notary receipt for payload and, when the TSA is
// enabled, an RFC 3161 timestamp over the notarized evidence.
func notarize(ctx context.Context, notary ArchiveNotary, timestamps ArchiveTimestampIssuer, payload ArchiveNotaryPayload) (*contractevents.ArchiveNotaryReceipt, *contractevents.ArchiveTSAReceipt, *datatype.JSON, error) {
	receipt, err := notary.NotarizeArchiveEntry(ctx, payload)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not notarize archive entry: %w", err)
	}
	notaryReceipt := &contractevents.ArchiveNotaryReceipt{
		ReceiptType:    receipt.ReceiptType,
		ArchiveEntryID: receipt.ArchiveEntryID,
		EventHash:      receipt.EventHash,
		PreviousHash:   receipt.PreviousHash,
		ReceivedAt:     receipt.ReceivedAt,
	}
	if timestamps == nil || !timestamps.Enabled() {
		return notaryReceipt, nil, nil, nil
	}
	evidence, err := BuildArchiveTimestampEvidence(payload, receipt)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not build archive TSA evidence: %w", err)
	}
	evidenceBytes, err := CanonicalArchiveTimestampEvidence(evidence)
	if err != nil {
		return nil, nil, nil, err
	}
	rawReceipt, err := timestamps.TimestampBytes(ctx, evidenceBytes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not timestamp archive entry: %w", err)
	}
	tsaReceipt := &contractevents.ArchiveTSAReceipt{
		ReceiptType:    "ARCHIVE_TSA_RECEIPT",
		Token:          rawReceipt.Token,
		TokenEncoding:  rawReceipt.TokenEncoding,
		HashAlgorithm:  rawReceipt.HashAlgorithm,
		MessageImprint: rawReceipt.MessageImprint,
		GeneratedAt:    rawReceipt.GeneratedAt,
		Policy:         rawReceipt.Policy,
		SerialNumber:   rawReceipt.SerialNumber,
	}
	tsaReceiptJSON, err := datatype.NewJSON(tsaReceipt)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not encode archive TSA receipt: %w", err)
	}
	return notaryReceipt, tsaReceipt, &tsaReceiptJSON, nil
}

type ArchiveTimestampIssuer interface {
	TimestampBytes(ctx context.Context, data []byte) (*tsa.Receipt, error)
	Enabled() bool
}

type ArchiveTimestampEvidence struct {
	ArchiveEntryID     string  `json:"archiveEntryId"`
	DID                string  `json:"did"`
	ContractVersion    int     `json:"contractVersion"`
	ContentHash        string  `json:"contentHash"`
	SnapshotCID        string  `json:"snapshotCid"`
	StoredBy           string  `json:"storedBy"`
	StoredAt           string  `json:"storedAt"`
	NotaryEventHash    string  `json:"notaryEventHash"`
	NotaryPreviousHash *string `json:"notaryPreviousHash"`
	NotaryReceivedAt   string  `json:"notaryReceivedAt"`
}

func BuildArchiveTimestampEvidence(payload ArchiveNotaryPayload, receipt *ArchiveNotaryReceipt) (ArchiveTimestampEvidence, error) {
	if receipt == nil {
		return ArchiveTimestampEvidence{}, fmt.Errorf("archive notary receipt is required for TSA evidence")
	}
	if receipt.EventHash == "" {
		return ArchiveTimestampEvidence{}, fmt.Errorf("archive notary receipt event hash is required for TSA evidence")
	}
	return ArchiveTimestampEvidence{
		ArchiveEntryID:     payload.ArchiveEntryID,
		DID:                payload.DID,
		ContractVersion:    payload.ContractVersion,
		ContentHash:        payload.ContentHash,
		SnapshotCID:        payload.SnapshotCID,
		StoredBy:           payload.StoredBy,
		StoredAt:           payload.StoredAt.UTC().Format(time.RFC3339Nano),
		NotaryEventHash:    receipt.EventHash,
		NotaryPreviousHash: receipt.PreviousHash,
		NotaryReceivedAt:   receipt.ReceivedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

func CanonicalArchiveTimestampEvidence(evidence ArchiveTimestampEvidence) ([]byte, error) {
	data, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("marshal archive TSA evidence: %w", err)
	}
	return data, nil
}
