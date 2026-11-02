package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/tsa"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/middleware"
)

// archiveManager is the organization of the Archive Manager; it created the
// seeded contracts, so the contract read scope admits them.
const archiveManager = "did:web:creator"

func archiveTestService(t *testing.T, db *sqlx.DB) *contractStorageArchivesrvc {
	return &contractStorageArchivesrvc{DB: db, CRepo: &pg.PostgresContractRepo{}, DIDDocument: *eraseTestDIDDocument(t, "dcs.example")}
}

func archiveManagerContext() context.Context {
	return middleware.InjectAuthContext(context.Background(), []string{"ARCHIVE_MANAGER"}, archiveManager, archiveManager)
}

// seedArchivedContract writes a contract in the given state with an archive
// entry for its current version.
func seedArchivedContract(t *testing.T, db *sqlx.DB, did, state string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, name, contract_data)
		VALUES ($1, 'did:web:dcs.example', 'did:web:creator', $2, 'did:web:tpl', 1, $3, '{"clause":"terms"}')`, did, state, "Archived "+did)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, evidence)
		VALUES ($1, 1, 'did:web:creator', '{"did":"x"}', 'sha256:`+zeroHash+`', 'bafy-snapshot',
		        '{"source":"SIGNING_WORKFLOW_COMPLETION","snapshot_hash_algorithm":"SHA-256"}')`, did)
	require.NoError(t, err)
}

const zeroHash = "0000000000000000000000000000000000000000000000000000000000000000"

// archiveEvents returns the event payloads of the given type, oldest first.
func archiveEvents(t *testing.T, db *sqlx.DB, eventType string) []map[string]any {
	t.Helper()
	var rows []string
	require.NoError(t, db.Select(&rows, `SELECT event_data::text FROM outbox_events WHERE event_type = $1 ORDER BY id`, eventType))
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var data map[string]any
		require.NoError(t, json.Unmarshal([]byte(row), &data))
		out = append(out, data)
	}
	return out
}

func TestArchiveRetrievalIsAuditedWithTheContractIDs(t *testing.T) {
	db := pgtest.Open(t)
	seedArchivedContract(t, db, "did:web:dcs.example:contract:a", "SIGNED")
	svc := archiveTestService(t, db)

	_, err := svc.Retrieve(archiveManagerContext(), &contractstoragearchive.ArchiveRetrieveRequest{})
	require.NoError(t, err)
	retrieved := archiveEvents(t, db, "RETRIEVE_ARCHIVED_CONTRACTS")
	require.Len(t, retrieved, 1)
	require.Equal(t, archiveManager, retrieved[0]["retrieved_by"])
	require.Equal(t, []any{"did:web:dcs.example:contract:a"}, retrieved[0]["contract_dids"])
	// DCS-FR-CWE-21: the retrieval names the holder and the roles, not only the organization.
	require.Equal(t, archiveManager, retrieved[0]["holder_did"])
	require.Contains(t, retrieved[0], "user_roles")

	did := "did:web:dcs.example:contract:a"
	_, err = svc.Search(archiveManagerContext(), &contractstoragearchive.ArchiveSearchRequest{Did: &did})
	require.NoError(t, err)
	searched := archiveEvents(t, db, "SEARCH_ARCHIVED_CONTRACTS")
	require.Len(t, searched, 1)
	require.Equal(t, []any{did}, searched[0]["contract_dids"])
	require.Equal(t, map[string]any{"did": did}, searched[0]["criteria"])
	require.Equal(t, archiveManager, searched[0]["holder_did"])
	require.Contains(t, searched[0], "user_roles")
}

// DCS-FR-CSA-10: the archive search filters by jurisdiction, and the audit
// entry records the filter.
func TestArchiveSearchFiltersByJurisdiction(t *testing.T) {
	db := pgtest.Open(t)
	for did, jurisdiction := range map[string]string{"did:web:dcs.example:contract:de": "Germany", "did:web:dcs.example:contract:fr": "France"} {
		seedArchivedContract(t, db, did, "SIGNED")
		_, err := db.Exec(`UPDATE contracts SET contract_data = jsonb_build_object('dcs:contractFields', jsonb_build_array(jsonb_build_object(
			'dcs:shape', jsonb_build_object('@id', 'https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction'), 'dcs:value', $2::text))) WHERE did = $1`, did, jurisdiction)
		require.NoError(t, err)
	}
	svc := archiveTestService(t, db)

	jurisdiction := "germ"
	found, err := svc.Search(archiveManagerContext(), &contractstoragearchive.ArchiveSearchRequest{Jurisdiction: &jurisdiction})
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "did:web:dcs.example:contract:de", found[0].Did)
	searched := archiveEvents(t, db, "SEARCH_ARCHIVED_CONTRACTS")
	require.Len(t, searched, 1)
	require.Equal(t, map[string]any{"jurisdiction": "germ"}, searched[0]["criteria"])
}

func TestFailedArchiveOperationsAreAuditedWithTheirOutcome(t *testing.T) {
	db := pgtest.Open(t)
	svc := archiveTestService(t, db)

	_, err := svc.Delete(archiveManagerContext(), &contractstoragearchive.DeletePayload{Did: "did:web:dcs.example:contract:none", Justification: "retention over"})
	require.Error(t, err)
	bad := "not-a-date"
	_, err = svc.Search(archiveManagerContext(), &contractstoragearchive.ArchiveSearchRequest{ValidFrom: &bad})
	require.Error(t, err)
	unknownState := "PENDING"
	_, err = svc.Search(archiveManagerContext(), &contractstoragearchive.ArchiveSearchRequest{State: &unknownState})
	require.Equal(t, "bad_request", errorName(err), "an unknown state filter is a client error")

	failed := archiveEvents(t, db, "ARCHIVE_OPERATION_FAILED")
	require.Len(t, failed, 3)
	require.Equal(t, "delete", failed[0]["operation"])
	require.Equal(t, "did:web:dcs.example:contract:none", failed[0]["did"])
	require.Equal(t, archiveManager, failed[0]["actor"])
	require.Equal(t, "FAILURE", failed[0]["outcome"])
	require.Equal(t, "search", failed[1]["operation"])
}

// Access attempts are part of the immutable audit log: the database refuses
// to change or remove them.
func TestAccessAttemptsAreAppendOnly(t *testing.T) {
	db := pgtest.Open(t)
	_, err := db.Exec(`INSERT INTO access_attempts (attempt_by, ip_address, attempted_at, success, outcome, service, method) VALUES ('u', '10.0.0.1', now(), false, 'DENIED', 'ContractStorageArchive', 'retrieve')`)
	require.NoError(t, err)

	_, err = db.Exec(`UPDATE access_attempts SET success = true`)
	require.ErrorContains(t, err, "append-only")
	_, err = db.Exec(`DELETE FROM access_attempts`)
	require.ErrorContains(t, err, "append-only")
}

// DCS-FR-CSA-09: the identifiers of an archived contract are persistent and
// immutable.
func TestArchivedContractIdentifiersAreImmutable(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:ids"
	seedArchivedContract(t, db, did, "SIGNED")

	_, err := db.Exec(`UPDATE contract_archive_entries SET id = gen_random_uuid() WHERE did = $1`, did)
	require.ErrorContains(t, err, "immutable", "the archive entry UUID changed")
	_, err = db.Exec(`UPDATE contract_archive_entries SET did = 'did:web:dcs.example:contract:other' WHERE did = $1`, did)
	require.ErrorContains(t, err, "immutable", "the archive entry DID changed")
	_, err = db.Exec(`UPDATE contracts SET did = 'did:web:dcs.example:contract:other' WHERE did = $1`, did)
	require.Error(t, err, "the archived contract's DID changed")
}

// DCS-FR-CSA-16: a terminated contract stays in the archive, marked
// TERMINATED, and its entry stays read-only.
func TestTerminatedContractStaysInTheArchiveReadOnly(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:ended"
	seedArchivedContract(t, db, did, "SIGNED")
	_, err := db.Exec(`UPDATE contracts SET state = 'TERMINATED' WHERE did = $1`, did)
	require.NoError(t, err)

	res, err := archiveTestService(t, db).Retrieve(archiveManagerContext(), &contractstoragearchive.ArchiveRetrieveRequest{})
	require.NoError(t, err)
	require.Len(t, res.Contracts, 1)
	require.Equal(t, did, res.Contracts[0].Did)
	require.Equal(t, "TERMINATED", res.Contracts[0].State)

	_, err = db.Exec(`UPDATE contract_archive_entries SET contract_snapshot = '{"forged":true}'::jsonb WHERE did = $1`, did)
	require.ErrorContains(t, err, "immutable")
}

// DCS-FR-CSA-16: the archive item of a terminated contract carries the
// recorded termination reason, the effective date and the initiating role. A
// termination a counterparty instance notified names the Counterparty.
func TestTerminatedArchiveItemCarriesReasonDateAndRole(t *testing.T) {
	db := pgtest.Open(t)
	byManager, byPeer := "did:web:dcs.example:contract:ended-here", "did:web:dcs.example:contract:ended-there"
	seedArchivedContract(t, db, byManager, "SIGNED")
	seedArchivedContract(t, db, byPeer, "SIGNED")
	svc := archiveTestService(t, db)
	local, err := svc.DIDDocument.GetID()
	require.NoError(t, err)
	handler := cwecommand.Terminator{DB: db, CRepo: &pg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: svc.DIDDocument}
	later := time.Now().Add(time.Hour)
	require.NoError(t, handler.Handle(context.Background(), cwecommand.TerminateCmd{
		DID: byManager, TerminatedBy: archiveManager, Reason: "breach of the SLA", UpdatedAt: later, CauserDID: local,
		UserRoles: userrole.UserRoles{userrole.ContractObserver, userrole.ArchiveManager},
	}))
	require.NoError(t, handler.Handle(context.Background(), cwecommand.TerminateCmd{
		DID: byPeer, TerminatedBy: "did:web:peer.example", Reason: "insolvency", UpdatedAt: later, CauserDID: "did:web:peer.example",
	}))

	res, err := svc.Retrieve(archiveManagerContext(), &contractstoragearchive.ArchiveRetrieveRequest{})
	require.NoError(t, err)
	items := map[string]*contractstoragearchive.ContractItem{}
	for _, item := range res.Contracts {
		items[item.Did] = item
	}
	require.Len(t, items, 2)
	for did, want := range map[string][2]string{byManager: {"breach of the SLA", "Archive Manager"}, byPeer: {"insolvency", "Counterparty"}} {
		item := items[did]
		require.Equal(t, "TERMINATED", item.State)
		require.Equal(t, want[0], *item.TerminationReason)
		require.Equal(t, want[1], *item.TerminationRole)
		effective, err := time.Parse(time.RFC3339, *item.TerminatedAt)
		require.NoError(t, err)
		require.WithinDuration(t, time.Now(), effective, time.Minute)
	}
}

// memoryArtifacts stands in for the encrypting IPFS store.
type memoryArtifacts struct {
	blobs    map[string][]byte
	shredded map[string]bool
}

func newMemoryArtifacts() *memoryArtifacts {
	return &memoryArtifacts{blobs: map[string][]byte{}, shredded: map[string]bool{}}
}

func (m *memoryArtifacts) Put(_ context.Context, _ artifactstore.Scope, plaintext []byte) (string, error) {
	cid := fmt.Sprintf("bafy-%d", len(m.blobs)+1)
	m.blobs[cid] = append([]byte(nil), plaintext...)
	return cid, nil
}

func (m *memoryArtifacts) Get(_ context.Context, scope artifactstore.Scope, cid string) ([]byte, error) {
	if m.shredded[scope.ID] {
		return nil, &artifactstore.ShreddedError{Scope: scope}
	}
	return m.blobs[cid], nil
}

func TestArchiveManagerStoresAFinalContractOnce(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:terminated"
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data)
		VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'TERMINATED', 'did:web:tpl', 1, '{"clause":"terms"}')`, did)
	require.NoError(t, err)
	seedLocalSignature(t, db, did)
	svc := archiveTestService(t, db)
	artifacts := newMemoryArtifacts()
	svc.Artifacts = artifacts

	res, err := svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "contract", Did: did})
	require.NoError(t, err)
	require.Equal(t, 1, *res.ContractVersion)

	var entry struct {
		StoredBy    string `db:"stored_by"`
		SnapshotCID string `db:"snapshot_cid"`
		Source      string `db:"source"`
	}
	require.NoError(t, db.Get(&entry, `SELECT stored_by, snapshot_cid, evidence->>'source' AS source FROM contract_archive_entries WHERE did = $1`, did))
	require.Equal(t, archiveManager, entry.StoredBy)
	require.Equal(t, "ARCHIVE_MANAGER_STORE", entry.Source)
	require.Contains(t, string(artifacts.blobs[entry.SnapshotCID]), `"clause":"terms"`)
	require.Len(t, archiveEvents(t, db, "STORE_ARCHIVED_CONTRACT"), 1)

	_, err = svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "contract", Did: did})
	require.ErrorContains(t, err, "already archived")
}

type stubArchiveNotary struct {
	payload cwecommand.ArchiveNotaryPayload
}

func (n *stubArchiveNotary) NotarizeArchiveEntry(_ context.Context, payload cwecommand.ArchiveNotaryPayload) (*cwecommand.ArchiveNotaryReceipt, error) {
	n.payload = payload
	return &cwecommand.ArchiveNotaryReceipt{ReceiptType: "ARCHIVE_NOTARY_RECEIPT", ArchiveEntryID: payload.ArchiveEntryID, EventHash: "sha256:" + zeroHash}, nil
}

type stubArchiveTSA struct{}

func (stubArchiveTSA) Enabled() bool { return true }
func (stubArchiveTSA) TimestampBytes(context.Context, []byte) (*tsa.Receipt, error) {
	return &tsa.Receipt{Token: "tsa-token", TokenEncoding: "base64", HashAlgorithm: "SHA-256", MessageImprint: "imprint"}, nil
}

// A contract the Archive Manager stores gets the notary receipt and the TSA
// receipt the signing workflow gives its archive entry, so the evidence
// compliance check passes.
func TestArchiveManagerStoreNotarizesAndTimestampsTheEntry(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:notarized"
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data)
		VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'TERMINATED', 'did:web:tpl', 1, '{"clause":"terms"}')`, did)
	require.NoError(t, err)
	seedLocalSignature(t, db, did)
	svc := archiveTestService(t, db)
	svc.Artifacts = newMemoryArtifacts()
	notary := &stubArchiveNotary{}
	svc.ArchiveNotary, svc.ArchiveTSA = notary, stubArchiveTSA{}

	_, err = svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "contract", Did: did})
	require.NoError(t, err)
	require.Equal(t, did+"#1", notary.payload.ArchiveEntryID)

	var token string
	require.NoError(t, db.Get(&token, `SELECT tsa_receipt->>'token' FROM contract_archive_entries WHERE did = $1`, did))
	require.Equal(t, "tsa-token", token)
	stored := archiveEvents(t, db, "STORE_ARCHIVED_CONTRACT")
	require.Len(t, stored, 1)
	require.Equal(t, "sha256:"+zeroHash, stored[0]["notary_receipt"].(map[string]any)["eventHash"])
	require.Equal(t, "tsa-token", stored[0]["tsa_receipt"].(map[string]any)["token"])

	items, err := svc.Compliance(archiveManagerContext(), &contractstoragearchive.CompliancePayload{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	for _, check := range items[0].Checks {
		if check.Rule == "evidence_completeness" {
			require.True(t, check.Passed, "evidence check: %v", check.Reason)
		}
	}
}

func TestArchiveManagerCannotArchiveAContractInWork(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:draft"
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version) VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'DRAFT', 'did:web:tpl', 1)`, did)
	require.NoError(t, err)
	svc := archiveTestService(t, db)
	svc.Artifacts = newMemoryArtifacts()

	_, err = svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "contract", Did: did})
	require.ErrorContains(t, err, "only SIGNED, TERMINATED or EXPIRED")
	require.Len(t, archiveEvents(t, db, "ARCHIVE_OPERATION_FAILED"), 1)
}

func TestArchiveManagerStoresEvidenceThatIsReadableUntilErasure(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:evidence"
	seedArchivedContract(t, db, did, "SIGNED")
	svc := archiveTestService(t, db)
	artifacts := newMemoryArtifacts()
	svc.Artifacts = artifacts

	name, mediaType := "delivery-note.pdf", "application/pdf"
	res, err := svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{
		Kind: "evidence", Did: did, Name: &name, MediaType: &mediaType, Content: []byte("%PDF-1.7 note"),
	})
	require.NoError(t, err)
	require.NotNil(t, res.EvidenceID)
	require.Len(t, archiveEvents(t, db, "STORE_ARCHIVE_EVIDENCE"), 1)

	items, err := svc.Evidence(archiveManagerContext(), &contractstoragearchive.EvidencePayload{Did: did})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, []byte("%PDF-1.7 note"), items[0].Content)
	require.Equal(t, res.ContentHash, items[0].ContentHash)

	artifacts.shredded[did] = true
	items, err = svc.Evidence(archiveManagerContext(), &contractstoragearchive.EvidencePayload{Did: did})
	require.NoError(t, err)
	require.Nil(t, items[0].Content, "erased evidence content must not be returned")

	_, err = db.Exec(`DELETE FROM contract_archive_evidence`)
	require.ErrorContains(t, err, "append-only")
}

func TestEvidenceNeedsALiveArchiveEntry(t *testing.T) {
	db := pgtest.Open(t)
	svc := archiveTestService(t, db)
	svc.Artifacts = newMemoryArtifacts()
	name, mediaType := "note.txt", "text/plain"
	did := "did:web:dcs.example:contract:unarchived"
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version) VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'SIGNED', 'did:web:tpl', 1)`, did)
	require.NoError(t, err)

	_, err = svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{
		Kind: "evidence", Did: did, Name: &name, MediaType: &mediaType, Content: []byte("x"),
	})
	require.ErrorContains(t, err, "no live archive entry")
	_, err = svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{
		Kind: "evidence", Did: "did:web:dcs.example:contract:none", Name: &name, MediaType: &mediaType, Content: []byte("x"),
	})
	require.Equal(t, "not_found", errorName(err))
}
