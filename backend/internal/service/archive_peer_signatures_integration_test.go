package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncdb "digital-contracting-service/internal/dcstodcs/db"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
)

// DCS-FR-SM-10: the counterparty signature that completes an archived
// contract leaves a proof of execution over every signature.
func TestPeerSignatureCompletingAnArchivedContractLeavesAnExecutionProofPostgres(t *testing.T) {
	database := pgtest.Open(t)
	const did, local, peer = "did:web:dcs-a.localhost:contract:proof", "did:web:dcs-a.localhost", "did:web:dcs-b.localhost"
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,responsible,contract_data)
		VALUES ($1,'',$2,'SIGNED','tpl',1,$3,$4)`, did, local, `{"creator":"`+local+`","counterparty":"`+peer+`"}`,
		`{"dcs:signatureFields":[{"dcs:signatoryName":"`+local+`"},{"dcs:signatoryName":"`+peer+`"}]}`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_signatures (contract_did, signer_did, credential_type, signature_bytes, status, field_name) VALUES ($1,'did:jwk:a','AES','\x00','SIGNED',$2)`, did, local)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, signature_metadata, evidence)
		VALUES ($1, 1, 'Org A', '{"did":"x"}', 'sha256:'||repeat('a',64), 'bafy-snapshot', '{"status":"SIGNED","signatures":[]}',
		        '{"source":"SIGNING_WORKFLOW_COMPLETION","snapshot_hash_algorithm":"SHA-256"}')`, did)
	require.NoError(t, err)

	svc := &dcsToDcssrvc{DB: database, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}}
	require.NoError(t, svc.recordSyncSignature(context.Background(), syncdb.SyncSignature{DID: did, ContractVersion: 1, FromPeerDID: peer, JadesSignature: "eyJ.e.s"}, local, []byte("%PDF-signed")))

	var proofs int
	require.NoError(t, database.Get(&proofs, `SELECT count(*) FROM contract_execution_proofs WHERE did = $1`, did))
	require.Equal(t, 1, proofs)
}

// DCS-FR-CSA-08: this instance archived the contract when its own signature
// completed. The counterparty signature that arrives afterwards is added to
// that archive entry, and the archive trigger permits only that append.
func TestPeerSignatureArrivingAfterArchiveIsAddedToTheEntryPostgres(t *testing.T) {
	database := pgtest.Open(t)
	const did = "did:web:dcs-a.localhost:contract:archived"
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data)
		VALUES ($1,'','Org A','SIGNED','tpl',1,'{}')`, did)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_archive_entries
		(did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, signature_metadata, evidence)
		VALUES ($1, 1, 'Org A', '{"did":"x"}', 'sha256:'||repeat('a',64), 'bafy-snapshot',
		        '{"status":"SIGNED","signatures":[{"field":"did:web:dcs-a.localhost"}]}',
		        '{"source":"SIGNING_WORKFLOW_COMPLETION","snapshot_hash_algorithm":"SHA-256"}')`, did)
	require.NoError(t, err)

	svc := &dcsToDcssrvc{DB: database, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}}
	record := func(evidence string) {
		t.Helper()
		require.NoError(t, svc.recordSyncSignature(context.Background(), syncdb.SyncSignature{
			DID: did, ContractVersion: 1, FromPeerDID: "did:web:dcs-b.localhost", JadesSignature: "eyJ.e.s", PoAEvidence: []byte(evidence),
		}, "did:web:dcs-a.localhost", nil))
	}
	metadata := func() map[string]any {
		t.Helper()
		var raw []byte
		require.NoError(t, database.Get(&raw, `SELECT signature_metadata FROM contract_archive_entries WHERE did = $1`, did))
		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	peerB := map[string]any{"party": "did:web:dcs-b.localhost", "signatory": "did:jwk:b"}
	record(`[{"party":"did:web:dcs-b.localhost","signatory":"did:jwk:b"}]`)
	got := metadata()
	require.Equal(t, []any{peerB}, got["peer_signatures"])
	require.Equal(t, "SIGNED", got["status"])

	peerC := map[string]any{"party": "did:web:dcs-c.localhost", "signatory": "did:jwk:c"}
	record(`[{"party":"did:web:dcs-b.localhost","signatory":"did:jwk:b"},{"party":"did:web:dcs-c.localhost","signatory":"did:jwk:c"}]`)
	require.Equal(t, []any{peerB, peerC}, metadata()["peer_signatures"])

	for name, update := range map[string]string{
		"other key":         `UPDATE contract_archive_entries SET signature_metadata = signature_metadata || '{"status":"REVOKED"}' WHERE did = $1`,
		"removal":           `UPDATE contract_archive_entries SET signature_metadata = jsonb_set(signature_metadata, '{peer_signatures}', '[]') WHERE did = $1`,
		"reorder":           `UPDATE contract_archive_entries SET signature_metadata = jsonb_set(signature_metadata, '{peer_signatures}', jsonb_build_array(signature_metadata->'peer_signatures'->1, signature_metadata->'peer_signatures'->0)) WHERE did = $1`,
		"replace with more": `UPDATE contract_archive_entries SET signature_metadata = jsonb_set(signature_metadata, '{peer_signatures}', '[{"party":"x"},{"party":"y"},{"party":"z"}]') WHERE did = $1`,
	} {
		_, err := database.Exec(update, did)
		require.ErrorContains(t, err, "immutable contract archive entry fields cannot be updated", name)
	}
}
