package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
)

type countingEraser struct {
	erased []string
	fail   error
}

func (e *countingEraser) EraseContract(_ context.Context, did, _, _ string) error {
	e.erased = append(e.erased, did)
	return e.fail
}

func archiveStatus(t *testing.T, svc *contractStorageArchivesrvc, did string) string {
	t.Helper()
	var status string
	require.NoError(t, svc.DB.Get(&status, `SELECT archive_status FROM contract_archive_entries WHERE did = $1`, did))
	return status
}

// DCS-FR-CSA-17: a contract in force keeps its archive entry and its keys.
func TestArchiveDeleteRefusesAContractInForce(t *testing.T) {
	for _, state := range []string{"SIGNED", "ACTIVE", "APPROVED", "DRAFT"} {
		t.Run(state, func(t *testing.T) {
			db := pgtest.Open(t)
			did := "did:web:dcs.example:contract:" + state
			seedArchivedContract(t, db, did, state)
			eraser, store := &countingEraser{}, &recordingSnapshotStore{}
			svc := archiveTestService(t, db)
			svc.Eraser, svc.SnapshotStore = eraser, store

			_, err := svc.Delete(archiveManagerContext(), &contractstoragearchive.DeletePayload{Did: did, Justification: "cleanup"})
			require.Equal(t, "bad_request", errorName(err))
			require.Empty(t, eraser.erased)
			require.Empty(t, store.deleted)
			require.NotEqual(t, "DELETED", archiveStatus(t, svc, did))
		})
	}
	for _, state := range []string{"TERMINATED", "EXPIRED", "REVOKED", "REJECTED", "WITHDRAWN"} {
		t.Run(state, func(t *testing.T) {
			db := pgtest.Open(t)
			did := "did:web:dcs.example:contract:" + state
			seedArchivedContract(t, db, did, state)
			eraser := &countingEraser{}
			svc := archiveTestService(t, db)
			svc.Eraser, svc.SnapshotStore = eraser, &recordingSnapshotStore{}

			n, err := svc.Delete(archiveManagerContext(), &contractstoragearchive.DeletePayload{Did: did, Justification: "retention over"})
			require.NoError(t, err)
			require.Equal(t, 1, n)
			require.Equal(t, []string{did}, eraser.erased)
		})
	}
}

// DCS-FR-CSA-02, ADR-31: an Archive Manager of another organization on the
// same instance may not annotate, store into or delete the archive of a
// contract it may not read.
func TestForeignArchiveManagerCannotWriteTheArchive(t *testing.T) {
	db := pgtest.Open(t)
	const did = "did:web:dcs.example:contract:y"
	seedArchivedContract(t, db, did, "TERMINATED")
	eraser, store := &countingEraser{}, &recordingSnapshotStore{}
	svc := archiveTestService(t, db)
	svc.Eraser, svc.SnapshotStore = eraser, store
	foreign := middleware.InjectAuthContext(context.Background(), []string{"ARCHIVE_MANAGER"}, "holder-x", "Org X")

	summary := "annotated by Org X"
	_, err := svc.Annotate(foreign, &contractstoragearchive.AnnotatePayload{Did: did, Summary: &summary})
	require.Equal(t, "forbidden", errorName(err), "annotate")
	_, err = svc.Delete(foreign, &contractstoragearchive.DeletePayload{Did: did, Justification: "x"})
	require.Equal(t, "forbidden", errorName(err), "delete")
	name, media, section := "note", "text/plain", "urn:c#payment"
	for _, payload := range []*contractstoragearchive.StorePayload{
		{Kind: "contract", Did: did},
		{Kind: "evidence", Did: did, Name: &name, MediaType: &media, Content: []byte("x")},
		{Kind: "section", Did: did, SectionID: &section, Parties: []string{"Org X"}},
	} {
		_, err = svc.Store(foreign, payload)
		require.Equal(t, "forbidden", errorName(err), "store kind=%s", payload.Kind)
	}

	require.Empty(t, eraser.erased)
	require.Empty(t, store.deleted)
	require.Equal(t, "STORED", archiveStatus(t, svc, did))
	var annotated *string
	require.NoError(t, db.Get(&annotated, `SELECT summary FROM contract_archive_entries WHERE did = $1`, did))
	require.Nil(t, annotated)
}

// DCS-FR-CSA-17, DCS-IR-CSA-03: the keys are shredded although the IPFS
// removal fails, and a delete of an entry whose erasure failed runs it again.
func TestArchiveDeleteShredsFirstAndCanBeRepeated(t *testing.T) {
	db := pgtest.Open(t)
	const did = "did:web:dcs.example:contract:erase"
	seedArchivedContract(t, db, did, "TERMINATED")
	eraser, store := &countingEraser{}, &recordingSnapshotStore{failOn: "bafy-snapshot"}
	svc := archiveTestService(t, db)
	svc.Eraser, svc.SnapshotStore = eraser, store

	n, err := svc.Delete(archiveManagerContext(), &contractstoragearchive.DeletePayload{Did: did, Justification: "gdpr"})
	require.NoError(t, err, "an IPFS removal failure does not fail a committed delete")
	require.Equal(t, 1, n)
	require.Equal(t, []string{did}, eraser.erased)
	require.Equal(t, "DELETED", archiveStatus(t, svc, did))
	require.Empty(t, archiveEvents(t, db, "ARCHIVE_OPERATION_FAILED"))

	eraser.fail = errors.New("peer unreachable")
	_, err = svc.Delete(archiveManagerContext(), &contractstoragearchive.DeletePayload{Did: did, Justification: "gdpr"})
	require.Error(t, err)
	require.Len(t, eraser.erased, 2, "a repeated delete runs the erasure again")

	eraser.fail, store.failOn = nil, ""
	n, err = svc.Delete(archiveManagerContext(), &contractstoragearchive.DeletePayload{Did: did, Justification: "gdpr"})
	require.NoError(t, err)
	require.Zero(t, n, "no entry is marked twice")
	require.Len(t, eraser.erased, 3)
	require.Contains(t, store.deleted, "bafy-snapshot", "the repeated delete removes what the first one left")
	require.Len(t, archiveEvents(t, db, "DELETE_ARCHIVED_CONTRACT"), 1)
}
