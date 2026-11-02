package dcstodcs

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
)

const erasedDID = "did:web:dcs.example:contract:erase-1"

// seedErasableContract writes a contract with plaintext content in every
// Postgres place that holds it, plus its live wrapped CEK.
func seedErasableContract(t *testing.T, db *sqlx.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO contracts (did, created_by, state, template_did, template_version, contract_data)
		 VALUES ($1, 'did:web:creator', 'SIGNED', 'did:web:tpl', 1, '{"clause":"secret terms"}')`,
		`INSERT INTO contract_history (did, created_by, state, template_did, template_version, contract_data)
		 VALUES ($1, 'did:web:creator', 'DRAFT', 'did:web:tpl', 1, '{"clause":"older secret terms"}')`,
		`INSERT INTO contract_negotiations (did, contract_version, change_request, created_by)
		 VALUES ($1, 1, '{"clause":"proposed secret terms"}', 'did:web:creator')`,
		`INSERT INTO contract_negotiation_drafts (contract_did, saved_by, change_request)
		 VALUES ($1, 'did:web:creator', '{"clause":"draft secret terms"}')`,
		`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, evidence)
		 VALUES ($1, 1, 'did:web:creator', '{"contract_data":{"clause":"secret terms"}}',
		         'sha256:` + "0000000000000000000000000000000000000000000000000000000000000000" + `', 'bafy-snapshot',
		         '{"source":"SIGNING_WORKFLOW_COMPLETION","snapshot_hash_algorithm":"SHA-256"}')`,
		`INSERT INTO signature_ceremonies (id, contract_did, field_name, requested_by, nonce, expires_at, pinned_payload)
		 VALUES (gen_random_uuid(), $1, 'sig', 'did:web:creator', 'n', now(), 'secret terms'::bytea)`,
		`INSERT INTO content_encryption_keys (scope_kind, scope_id, recipient_did, wrapped_cek)
		 VALUES ('contract', $1, 'did:web:dcs.example', '{"alg":"ECDH-ES+A256KW","encrypted_key":"x"}')`,
	}
	for _, statement := range statements {
		_, err := db.Exec(statement, erasedDID)
		require.NoError(t, err, statement)
	}
}

func TestShredDestroysTheWrappedKeyAndErasesPlaintextContent(t *testing.T) {
	db := pgtest.Open(t)
	seedErasableContract(t, db)

	shredder := &AuditedShredder{
		DB:        db,
		Keys:      &artifactstore.PostgresCEKRepo{DB: db},
		Artifacts: artifactstore.New(nil, nil, nil, "", "", nil),
	}
	affected, err := shredder.Shred(context.Background(), erasedDID, "did:web:archive-manager", "Art. 17 request")
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)

	var liveKeys int
	require.NoError(t, db.Get(&liveKeys, `SELECT count(*) FROM content_encryption_keys WHERE scope_id = $1 AND wrapped_cek IS NOT NULL`, erasedDID))
	require.Zero(t, liveKeys, "a shredded scope must not keep any wrapped CEK")

	var plaintext int
	require.NoError(t, db.Get(&plaintext, `
		SELECT (SELECT count(*) FROM contracts WHERE did = $1 AND contract_data::text LIKE '%secret%')
		     + (SELECT count(*) FROM contract_history WHERE did = $1 AND contract_data::text LIKE '%secret%')
		     + (SELECT count(*) FROM contract_negotiations WHERE did = $1 AND change_request::text LIKE '%secret%')
		     + (SELECT count(*) FROM contract_negotiation_drafts WHERE contract_did = $1)
		     + (SELECT count(*) FROM contract_archive_entries WHERE did = $1 AND contract_snapshot::text LIKE '%secret%')
		     + (SELECT count(*) FROM signature_ceremonies WHERE contract_did = $1 AND pinned_payload IS NOT NULL)`, erasedDID))
	require.Zero(t, plaintext, "plaintext contract content survived the erasure")

	var shredEvents int
	require.NoError(t, db.Get(&shredEvents, `SELECT count(*) FROM outbox_events WHERE did = $1 AND event_type = 'KEY_SHREDDED'`, erasedDID))
	require.Equal(t, 1, shredEvents)
}

// The archive snapshot can only be erased, and only after the contract's keys
// are shredded: any other change stays refused by the database.
func TestArchiveSnapshotIsErasableOnlyAfterTheKeysAreShredded(t *testing.T) {
	db := pgtest.Open(t)
	seedErasableContract(t, db)

	_, err := db.Exec(`UPDATE contract_archive_entries SET contract_snapshot = '{}'::jsonb WHERE did = $1`, erasedDID)
	require.ErrorContains(t, err, "immutable", "the snapshot was erasable while the keys were live")

	_, err = db.Exec(`UPDATE content_encryption_keys SET wrapped_cek = NULL, shredded_at = now() WHERE scope_id = $1`, erasedDID)
	require.NoError(t, err)

	_, err = db.Exec(`UPDATE contract_archive_entries SET contract_snapshot = '{"forged":true}'::jsonb WHERE did = $1`, erasedDID)
	require.ErrorContains(t, err, "immutable", "a shredded scope let the snapshot be replaced, not erased")

	_, err = db.Exec(`UPDATE contract_archive_entries SET contract_snapshot = '{}'::jsonb WHERE did = $1`, erasedDID)
	require.NoError(t, err)
}

// A shredded record without its key is the only shape the table accepts: a
// shred that left the wrapped CEK behind is refused.
func TestShreddedKeyRowMustNotKeepItsWrappedKey(t *testing.T) {
	db := pgtest.Open(t)
	seedErasableContract(t, db)

	_, err := db.Exec(`UPDATE content_encryption_keys SET shredded_at = now() WHERE scope_id = $1`, erasedDID)
	require.ErrorContains(t, err, "chk_cek_shredded_has_no_key")
}

func shredForTest(t *testing.T, db *sqlx.DB) {
	t.Helper()
	shredder := &AuditedShredder{DB: db, Keys: &artifactstore.PostgresCEKRepo{DB: db}, Artifacts: artifactstore.New(nil, nil, nil, "", "", nil)}
	_, err := shredder.Shred(context.Background(), erasedDID, "did:web:archive-manager", "Art. 17 request")
	require.NoError(t, err)
}

// The shred also erases the copies of the contract and of the signers'
// personal data outside the contract tables: the outbox bodies that the audit
// trail erases with the key, the payload of every JAdES signature, and the
// PID claims, presentations and prepared PDF of the signing ceremonies. The
// erasure records and the bodies of other scopes stay.
func TestShredErasesResidualCopiesOfTheContentAndPersonalData(t *testing.T) {
	db := pgtest.Open(t)
	seedErasableContract(t, db)
	const jws = "aGVhZGVy.c2VjcmV0IHRlcm1z.c2ln"
	statements := []string{
		`UPDATE signature_ceremonies SET pid_claims = '{"given_name":"secret Anna"}', vp_token = 'secret-vp',
		 poa_vp_token = 'secret-poa', prepared_pdf = 'secret pdf'::bytea WHERE contract_did = $1`,
		`INSERT INTO contract_sync_signatures (did, contract_version, from_peer_did, jades_signature)
		 VALUES ($1, 1, 'did:web:peer', '` + jws + `')`,
		`INSERT INTO contract_signatures (contract_did, signer_did, status, jades_signature)
		 VALUES ($1, 'did:jwk:anna', 'SIGNED', '` + jws + `')`,
		`INSERT INTO outbox_events (component, event_type, event_data, did) VALUES
		 ('CONTRACT_STORAGE_ARCHIVE', 'STORE_ARCHIVED_CONTRACT', '{"note":"secret terms"}', $1),
		 ('CONTRACT_WORKFLOW_ENGINE', 'TERMINATE_CONTRACT', '{"reason":"secret reason"}', $1),
		 ('CONTRACT_STORAGE_ARCHIVE', 'DELETE_ARCHIVED_CONTRACT', '{"justification":"kept record"}', $1),
		 ('PROCESS_AUDIT_AND_COMPLIANCE', 'PAC_INCIDENT_REPORT', '{"case_id":"kept case"}', $1)`,
	}
	for _, statement := range statements {
		_, err := db.Exec(statement, erasedDID)
		require.NoError(t, err, statement)
	}

	shredForTest(t, db)

	var personal int
	require.NoError(t, db.Get(&personal, `SELECT count(*) FROM signature_ceremonies WHERE contract_did = $1
		AND num_nonnulls(pid_claims, vp_token, poa_vp_token, prepared_pdf) > 0`, erasedDID))
	require.Zero(t, personal, "a signing ceremony kept personal data or the prepared PDF")

	var signatures []string
	require.NoError(t, db.Select(&signatures, `SELECT jades_signature FROM contract_sync_signatures WHERE did = $1
		UNION ALL SELECT jades_signature FROM contract_signatures WHERE contract_did = $1`, erasedDID))
	require.Equal(t, []string{"aGVhZGVy..c2ln", "aGVhZGVy..c2ln"}, signatures, "a JAdES signature kept its contract payload")

	var bodies []string
	require.NoError(t, db.Select(&bodies, `SELECT event_data::text FROM outbox_events
		WHERE did = $1 AND event_type <> 'KEY_SHREDDED' ORDER BY id`, erasedDID))
	require.Equal(t, []string{`{"erased": true}`, `{"erased": true}`, `{"justification": "kept record"}`, `{"case_id": "kept case"}`}, bodies)
}

// NFR-17: the free text users entered about the contract is erased too: its
// name and description now and in its history, the reasons of rejected
// changes, and the termination reasons.
func TestShredErasesTheFreeTextOfTheContract(t *testing.T) {
	db := pgtest.Open(t)
	seedErasableContract(t, db)
	statements := []string{
		`UPDATE contracts SET name = 'secret name', description = 'secret description' WHERE did = $1`,
		`UPDATE contract_history SET name = 'secret old name', description = 'secret old description' WHERE did = $1`,
		`INSERT INTO contract_negotiation_decisions (negotiation_id, negotiator, decision, rejection_reason)
		 SELECT id, 'did:web:peer', 'REJECTED', 'secret objection' FROM contract_negotiations WHERE did = $1`,
		`INSERT INTO contract_terminations (did, reason, effective_at, terminated_by, initiating_role)
		 VALUES ($1, 'secret termination reason', now(), 'Org A', 'Contract Manager')`,
		`INSERT INTO contract_peer_terminations (did, peer_did, reason) VALUES ($1, 'did:web:peer', 'secret termination reason')`,
	}
	for _, statement := range statements {
		_, err := db.Exec(statement, erasedDID)
		require.NoError(t, err, statement)
	}

	shredForTest(t, db)

	var freeText int
	require.NoError(t, db.Get(&freeText, `
		SELECT (SELECT count(*) FROM contracts WHERE did = $1 AND num_nonnulls(name, description) > 0)
		     + (SELECT count(*) FROM contract_history WHERE did = $1 AND num_nonnulls(name, description) > 0)
		     + (SELECT count(*) FROM contract_negotiation_decisions d JOIN contract_negotiations n ON n.id = d.negotiation_id
		         WHERE n.did = $1 AND d.rejection_reason IS NOT NULL)
		     + (SELECT count(*) FROM contract_terminations WHERE did = $1 AND reason <> $2)
		     + (SELECT count(*) FROM contract_peer_terminations WHERE did = $1 AND reason <> $2)`, erasedDID, ErasedReason))
	require.Zero(t, freeText, "free text about the contract survived the erasure")
}

// A peer erase goes only to a party that holds a copy: the origin this
// instance received the contract from, or a peer a revision was delivered to.
func TestErasePartiesAreThePeersThatHoldACopy(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	parties := &DBContractParties{DB: db, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}}
	insert := func(did, origin, counterparty string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, responsible)
			VALUES ($1, $2::text, $2::text, 'TERMINATED', 'tpl', 1, jsonb_build_object('creator', $2::text, 'counterparty', $3::text))`, did, origin, counterparty)
		require.NoError(t, err)
	}
	insert("never-shipped", "did:web:local", "did:web:peer")
	insert("shipped", "did:web:local", "did:web:peer")
	insert("received", "did:web:peer", "did:web:local")
	_, err := db.Exec(`INSERT INTO contract_pdf_deliveries (did, peer_did, contract_state, pdf_cid) VALUES ('shipped', 'did:web:peer', 'OFFERED', 'cid')`)
	require.NoError(t, err)

	for did, want := range map[string][]string{
		"never-shipped": {"did:web:local"},
		"shipped":       {"did:web:local", "did:web:peer"},
		"received":      {"did:web:peer"},
	} {
		got, err := parties.Parties(ctx, did)
		require.NoError(t, err)
		require.ElementsMatch(t, want, got, did)
	}
}

// CWE-5: a peer erases this instance's copy only when the copy is no longer
// in force and no retention period of its archive entries runs.
func TestPeerEraseNeedsAClosedCopyOutsideRetention(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	erasure := DBPeerErasure{DB: db, CRepo: &cwepg.PostgresContractRepo{}}
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, responsible)
		VALUES ($1, 'did:web:peer', 'did:web:peer', 'ACTIVE', 'tpl', 1, jsonb_build_object('creator', 'did:web:peer', 'counterparty', 'did:web:local'))`, erasedDID)
	require.NoError(t, err)

	require.ErrorIs(t, erasure.AuthorizeErase(ctx, "did:web:dcs.example:contract:unknown", "did:web:peer"), ErrNoSharedCopy)
	require.ErrorIs(t, erasure.AuthorizeErase(ctx, erasedDID, "did:web:stranger"), ErrNoSharedCopy)
	require.ErrorIs(t, erasure.AuthorizeErase(ctx, erasedDID, "did:web:peer"), ErrNotErasable, "an ACTIVE copy was erasable")

	_, err = db.Exec(`UPDATE contracts SET state = 'TERMINATED' WHERE did = $1`, erasedDID)
	require.NoError(t, err)
	archive := func(version int, retention string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, evidence, retention_until)
			VALUES ($1, $2, 'did:web:local', '{}', 'sha256:`+"0000000000000000000000000000000000000000000000000000000000000000"+`', 'bafy',
			        '{"source":"SIGNING_WORKFLOW_COMPLETION","snapshot_hash_algorithm":"SHA-256"}', now() + $3::interval)`, erasedDID, version, retention)
		require.NoError(t, err)
	}
	archive(1, "-1 day")
	require.NoError(t, erasure.AuthorizeErase(ctx, erasedDID, "did:web:peer"), "an ended retention kept the copy")

	archive(2, "1 year")
	err = erasure.AuthorizeErase(ctx, erasedDID, "did:web:peer")
	require.ErrorIs(t, err, ErrNotErasable, "a copy under retention was erasable")
	require.ErrorContains(t, err, "retained until")
}

// After the erasure the contract keeps the organization names of its
// parties, so the read policy still admits a named party. Nothing else of a
// party node stays.
func TestErasedContractStaysReadableForItsNamedParties(t *testing.T) {
	db := pgtest.Open(t)
	seedErasableContract(t, db)
	_, err := db.Exec(`UPDATE contracts SET contract_data =
		'{"clause":"secret terms","dcs:parties":[{"dcs:legalName":"Org B","dcs:email":"secret@b.example"}]}' WHERE did = $1`, erasedDID)
	require.NoError(t, err)

	shredForTest(t, db)

	var data string
	require.NoError(t, db.Get(&data, `SELECT contract_data::text FROM contracts WHERE did = $1`, erasedDID))
	require.Equal(t, `{"dcs:parties": [{"dcs:legalName": "Org B"}]}`, data)

	ctx := contractreadpolicy.WithScope(context.Background(),
		contractreadpolicy.Scope{Organization: "Org B", Roles: userrole.UserRoles{userrole.ContractObserver}})
	params := []any{erasedDID}
	var readable int
	require.NoError(t, db.Get(&readable, `SELECT count(*) FROM contracts WHERE did = $1 AND `+
		contractreadpolicy.SQL(ctx, "contracts.did", &params), params...))
	require.Equal(t, 1, readable, "the named party lost read access to the erased contract")
}
