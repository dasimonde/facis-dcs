package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	pac "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	basepg "digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/pgtest"
)

// DCS-IR-CSA-04, DCS-FR-CSA-18: /archive/audit returns the archive
// operations per contract, limited to the readable contracts and the filter.
func TestArchiveOperationLogKeepsTheReadableOperations(t *testing.T) {
	entry := func(id int64, did, eventType string) datatype.AuditLogEntry {
		return datatype.AuditLogEntry{ID: id, Component: "CONTRACT_STORAGE_ARCHIVE", EventType: eventType, DID: &did, CreatedAt: time.Now()}
	}
	chains := [][]datatype.AuditLogEntry{
		{entry(1, "a", "STORE_ARCHIVED_CONTRACT"), entry(2, "a", "DELETE_ARCHIVED_CONTRACT")},
		{entry(3, "b", "ANNOTATE_ARCHIVED_CONTRACT")},
		{entry(4, "*", "RETRIEVE_ARCHIVED_CONTRACTS"), entry(5, "*", "RETRIEVE_SOMETHING")},
	}
	ids := func(log map[string][]*pac.PACResourceAuditTrailEntry) map[string][]int64 {
		out := map[string][]int64{}
		for did, entries := range log {
			for _, e := range entries {
				out[did] = append(out[did], e.ID)
			}
		}
		return out
	}

	require.Equal(t, map[string][]int64{"a": {1, 2}, "b": {3}, "*": {4}}, ids(archiveOperationLog(chains, "", nil)))
	require.Equal(t, map[string][]int64{"a": {1, 2}}, ids(archiveOperationLog(chains, "", map[string]bool{"a": true})))
	require.Equal(t, map[string][]int64{"b": {3}}, ids(archiveOperationLog(chains, "b", nil)))
}

// countingObjects counts the IPFS fetches of the integrity checks.
type countingObjects struct{ fetched []string }

func (o *countingObjects) CreateFile(context.Context, any) (*ipfs.IPFSResult, error) {
	result := &ipfs.IPFSResult{}
	result.Identifier.Value = "key-cid"
	return result, nil
}
func (o *countingObjects) FetchFile(cid string) (*ipfs.IPFSResult, error) {
	o.fetched = append(o.fetched, cid)
	return &ipfs.IPFSResult{}, nil
}

// DCS-IR-CSA-04: a DID filter applies before the integrity checks, so the
// audit of one contract fetches the snapshot of that contract only.
func TestArchiveAuditChecksOnlyTheFilteredContract(t *testing.T) {
	db := pgtest.Open(t)
	for _, did := range []string{"did:web:dcs.example:contract:one", "did:web:dcs.example:contract:two"} {
		seedArchivedContract(t, db, did, "SIGNED")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	objects := &countingObjects{}
	store := artifactstore.New(objects, &artifactstore.PostgresCEKRepo{DB: db}, reportAgreement{key}, "did:web:dcs.example", "did:web:dcs.example#agreement", &key.PublicKey)
	for _, did := range []string{"did:web:dcs.example:contract:one", "did:web:dcs.example:contract:two"} {
		_, err := store.Put(context.Background(), artifactstore.ContractScope(did), []byte("key"))
		require.NoError(t, err)
	}
	svc := archiveTestService(t, db)
	svc.ATrailReader = base.AuditTrailReader{ARepo: &basepg.PostgresAuditTrailRepository{}, Artifacts: store}

	did := "did:web:dcs.example:contract:one"
	trail, err := svc.Audit(archiveManagerContext(), &contractstoragearchive.AuditPayload{Did: &did, Justification: "audit"})
	require.NoError(t, err)
	require.Len(t, trail, 1)
	require.Len(t, objects.fetched, 1, "the snapshot of the other contract is not fetched")
}
