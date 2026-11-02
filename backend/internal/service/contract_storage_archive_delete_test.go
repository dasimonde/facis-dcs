package service

import (
	"errors"
	"testing"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

// recordingSnapshotStore captures DeleteFile calls and can fail on a chosen
// CID, standing in for the IPFS client in the DCS-NFR-SEC-13 delete path.
type recordingSnapshotStore struct {
	deleted []string
	failOn  string
}

func (r *recordingSnapshotStore) DeleteFile(cid string) error {
	if cid == r.failOn {
		return errors.New("ipfs unreachable")
	}
	r.deleted = append(r.deleted, cid)
	return nil
}

func TestSnapshotCIDsForDeletionSelectsEveryEntryOfTheDID(t *testing.T) {
	deletedAt := time.Now().UTC()
	entries := []db.ContractArchiveEntry{
		{DID: "did:web:a", ContractVersion: 1, SnapshotCID: "bafy-a-v1"},
		{DID: "did:web:a", ContractVersion: 2, SnapshotCID: "bafy-a-v2"},
		// Same CID twice (identical snapshot bytes are content-addressed to
		// one object) must yield one removal.
		{DID: "did:web:a", ContractVersion: 3, SnapshotCID: "bafy-a-v2"},
		// Already soft-deleted: a repeated delete removes what an earlier
		// attempt left.
		{DID: "did:web:a", ContractVersion: 0, SnapshotCID: "bafy-a-old", DeletedAt: &deletedAt},
		// Another contract's entry stays untouched.
		{DID: "did:web:b", ContractVersion: 1, SnapshotCID: "bafy-b-v1"},
		// No snapshot stored — nothing in IPFS to remove.
		{DID: "did:web:a", ContractVersion: 4, SnapshotCID: ""},
	}

	cids := snapshotCIDsForDeletion(entries, "did:web:a")

	want := []string{"bafy-a-v1", "bafy-a-v2", "bafy-a-old"}
	if len(cids) != len(want) {
		t.Fatalf("expected CIDs %v, got %v", want, cids)
	}
	for i, cid := range want {
		if cids[i] != cid {
			t.Fatalf("expected CIDs %v, got %v", want, cids)
		}
	}
}

func TestRemoveArchivedSnapshotsRemovesEveryCID(t *testing.T) {
	store := &recordingSnapshotStore{}
	if err := removeArchivedSnapshots(store, []string{"bafy-1", "bafy-2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.deleted) != 2 || store.deleted[0] != "bafy-1" || store.deleted[1] != "bafy-2" {
		t.Fatalf("expected both snapshots removed, got %v", store.deleted)
	}
}

func TestRemoveArchivedSnapshotsTriesEveryCIDAndReportsTheFailures(t *testing.T) {
	store := &recordingSnapshotStore{failOn: "bafy-2"}
	err := removeArchivedSnapshots(store, []string{"bafy-1", "bafy-2", "bafy-3"})
	if err == nil {
		t.Fatal("expected the removal error to be reported")
	}
	if len(store.deleted) != 2 || store.deleted[0] != "bafy-1" || store.deleted[1] != "bafy-3" {
		t.Fatalf("expected every other CID removed, got %v", store.deleted)
	}
}

func TestRetainedUntilBlocksOnlyLiveEntriesWithAFutureRetention(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	past, soon, later := now.AddDate(0, 0, -1), now.AddDate(1, 0, 0), now.AddDate(5, 0, 0)
	deletedAt := now.AddDate(0, 0, -2)

	if got := retainedUntil([]db.ContractArchiveEntry{
		{DID: "did:web:a"},
		{DID: "did:web:a", RetentionUntil: &past},
		{DID: "did:web:a", RetentionUntil: &later, DeletedAt: &deletedAt},
		{DID: "did:web:b", RetentionUntil: &later},
	}, "did:web:a", now); got != nil {
		t.Fatalf("deletion must be permitted, got retention until %s", got)
	}

	got := retainedUntil([]db.ContractArchiveEntry{
		{DID: "did:web:a", RetentionUntil: &soon},
		{DID: "did:web:a", RetentionUntil: &later},
	}, "did:web:a", now)
	if got == nil || !got.Equal(later) {
		t.Fatalf("expected retention until %s, got %v", later, got)
	}
}
