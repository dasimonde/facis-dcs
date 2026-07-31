package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/tsa"
)

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
