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

func TestSnapshotCIDsForDeletionSelectsLiveEntriesOfTheDID(t *testing.T) {
	deletedAt := time.Now().UTC()
	entries := []db.ContractArchiveEntry{
		{DID: "did:web:a", ContractVersion: 1, SnapshotCID: "bafy-a-v1"},
		{DID: "did:web:a", ContractVersion: 2, SnapshotCID: "bafy-a-v2"},
		// Same CID twice (identical snapshot bytes are content-addressed to
		// one object) must yield one removal.
		{DID: "did:web:a", ContractVersion: 3, SnapshotCID: "bafy-a-v2"},
		// Already soft-deleted: its snapshot was disposed of by the earlier
		// delete, not this one.
		{DID: "did:web:a", ContractVersion: 0, SnapshotCID: "bafy-a-old", DeletedAt: &deletedAt},
		// Another contract's entry stays untouched.
		{DID: "did:web:b", ContractVersion: 1, SnapshotCID: "bafy-b-v1"},
		// No snapshot stored — nothing in IPFS to remove.
		{DID: "did:web:a", ContractVersion: 4, SnapshotCID: ""},
	}

	cids := snapshotCIDsForDeletion(entries, "did:web:a")

	want := []string{"bafy-a-v1", "bafy-a-v2"}
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

func TestRemoveArchivedSnapshotsHardFailsOnRemovalError(t *testing.T) {
	store := &recordingSnapshotStore{failOn: "bafy-2"}
	err := removeArchivedSnapshots(store, []string{"bafy-1", "bafy-2", "bafy-3"})
	if err == nil {
		t.Fatal("expected the removal error to propagate")
	}
	if len(store.deleted) != 1 || store.deleted[0] != "bafy-1" {
		t.Fatalf("expected removal to stop at the failing CID, got %v", store.deleted)
	}
}
