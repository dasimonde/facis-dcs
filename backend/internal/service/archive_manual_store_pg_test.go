package service

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/pgtest"
)

// seedLocalSignature records a signature this instance applied to did under
// a ceremony that kept its presentation.
func seedLocalSignature(t *testing.T, db *sqlx.DB, did string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO signature_ceremonies (id, contract_did, field_name, requested_by, status, nonce, vp_token, expires_at)
		VALUES ('33333333-3333-3333-3333-333333333333', $1, 'did:web:creator', 'did:web:creator', 'verified', 'n', 'vp', now() + interval '1 hour')`, did)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contract_signatures (contract_did, signer_did, credential_type, status, field_name, ceremony_id, pdf_hash)
		VALUES ($1, 'did:key:jane', 'AES', 'SIGNED', 'did:web:creator', '33333333-3333-3333-3333-333333333333', 'abc')`, did)
	require.NoError(t, err)
}

// DCS-FR-CSA-06/-07/-08: an Archive Manager archives only a signed contract,
// and the entry records its signatures and credential hashes, so the
// signature policy rule passes for it.
func TestArchiveManagerArchivesASignedContractWithItsSignatures(t *testing.T) {
	db := pgtest.Open(t)
	unsigned, signed := "did:web:dcs.example:contract:expired-draft", "did:web:dcs.example:contract:signed"
	for _, did := range []string{unsigned, signed} {
		_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data)
			VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'EXPIRED', 'did:web:tpl', 1, '{}')`, did)
		require.NoError(t, err)
	}
	seedLocalSignature(t, db, signed)
	svc := archiveTestService(t, db)
	svc.Artifacts = newMemoryArtifacts()

	_, err := svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "contract", Did: unsigned})
	require.ErrorContains(t, err, "never signed")
	_, err = svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "contract", Did: signed})
	require.NoError(t, err)

	items, err := svc.Compliance(archiveManagerContext(), &contractstoragearchive.CompliancePayload{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	for _, check := range items[0].Checks {
		if check.Rule == "signature_policy" {
			require.True(t, check.Passed, "signature policy: %v", check.Reason)
		}
	}
}

// DCS-FR-CSA-18, DCS-FR-CSA-03: a store waits for the per-contract lock, so
// a concurrent store of the same version is refused, not dropped silently.
func TestArchiveManagerStoreWaitsForTheContractLock(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:raced"
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data)
		VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'TERMINATED', 'did:web:tpl', 1, '{}')`, did)
	require.NoError(t, err)
	seedLocalSignature(t, db, did)
	svc := archiveTestService(t, db)
	artifacts := newMemoryArtifacts()
	svc.Artifacts = artifacts

	other, err := db.Beginx()
	require.NoError(t, err)
	defer func() { _ = other.Rollback() }()
	_, err = other.Exec(`SELECT pg_advisory_xact_lock(hashtext($1))`, did)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "contract", Did: did})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("store returned while another store held the contract: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	_, err = other.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, evidence)
		VALUES ($1, 1, 'did:web:creator', '{}', 'sha256:`+zeroHash+`', 'bafy-other', '{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, did)
	require.NoError(t, err)
	require.NoError(t, other.Commit())

	select {
	case err := <-done:
		require.ErrorContains(t, err, "already archived")
	case <-time.After(10 * time.Second):
		t.Fatal("store did not finish")
	}
	require.Empty(t, artifacts.blobs, "no orphan snapshot in IPFS")
}
