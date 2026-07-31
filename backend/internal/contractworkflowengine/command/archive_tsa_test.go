package command

import (
	"testing"
	"time"
)

func TestCanonicalArchiveTimestampEvidenceIsDeterministic(t *testing.T) {
	previousHash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := ArchiveNotaryPayload{
		EventType:       "ARCHIVE_STORED",
		ArchiveEntryID:  "did:example:contract:1#1",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ContentHash:     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SnapshotCID:     "bafy-one",
		StoredBy:        "alice",
		StoredAt:        time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
	}
	receipt := &ArchiveNotaryReceipt{
		ReceiptType:    "ARCHIVE_NOTARY_RECEIPT",
		ArchiveEntryID: payload.ArchiveEntryID,
		EventHash:      "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		PreviousHash:   &previousHash,
		ReceivedAt:     time.Date(2026, 6, 8, 12, 0, 1, 0, time.UTC),
	}

	evidence, err := BuildArchiveTimestampEvidence(payload, receipt)
	if err != nil {
		t.Fatalf("BuildArchiveTimestampEvidence returned error: %v", err)
	}
	first, err := CanonicalArchiveTimestampEvidence(evidence)
	if err != nil {
		t.Fatalf("CanonicalArchiveTimestampEvidence returned error: %v", err)
	}
	second, err := CanonicalArchiveTimestampEvidence(evidence)
	if err != nil {
		t.Fatalf("CanonicalArchiveTimestampEvidence returned error: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("expected deterministic archive TSA evidence")
	}

	changed := evidence
	changed.SnapshotCID = "bafy-two"
	third, err := CanonicalArchiveTimestampEvidence(changed)
	if err != nil {
		t.Fatalf("CanonicalArchiveTimestampEvidence returned error: %v", err)
	}
	if string(first) == string(third) {
		t.Fatal("expected changed archive evidence to produce different canonical bytes")
	}
}

func TestBuildArchiveTimestampEvidenceRequiresNotaryReceipt(t *testing.T) {
	_, err := BuildArchiveTimestampEvidence(ArchiveNotaryPayload{}, nil)
	if err == nil {
		t.Fatal("expected missing notary receipt error")
	}
}
