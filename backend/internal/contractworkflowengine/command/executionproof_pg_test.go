package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/db/pg"
)

type recordingNotary struct{ payloads []ArchiveNotaryPayload }

func (n *recordingNotary) NotarizeArchiveEntry(_ context.Context, payload ArchiveNotaryPayload) (*ArchiveNotaryReceipt, error) {
	n.payloads = append(n.payloads, payload)
	return &ArchiveNotaryReceipt{ReceiptType: "ARCHIVE_NOTARY_RECEIPT", ArchiveEntryID: payload.ArchiveEntryID, EventHash: "sha256:receipt", ReceivedAt: time.Now().UTC()}, nil
}

type fixedStorer struct{}

func (fixedStorer) Put(context.Context, artifactstore.Scope, []byte) (string, error) {
	return "bafy-execution-proof", nil
}

// DCS-FR-SM-10, DCS-FR-UC-13-1: on the instance that signed first, the archive
// entry is written before the counterparty signs. When the counterparty's
// signature completes the contract, a second, notarized proof of execution
// covers it and the fully signed PDF. It is written once, and not while a
// field is still unsigned.
func TestTheLastCounterpartySignatureGetsANotarizedExecutionProof(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	const did, local, peer = "did:web:dcs-a.example:contract:1", "did:web:dcs-a.example", "did:web:dcs-b.example"
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,responsible,contract_data)
		VALUES ($1,'',$2,'SIGNED','tpl',1,$3,$4)`, did, local,
		`{"creator":"`+local+`","counterparty":"`+peer+`"}`,
		`{"dcs:signatureFields":[{"dcs:signatoryName":"`+local+`"},{"dcs:signatoryName":"`+peer+`"}]}`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, signature_metadata, evidence)
		VALUES ($1, 1, 'Org A', '{"did":"x"}', 'sha256:'||repeat('a',64), 'bafy-snapshot',
		        '{"status":"SIGNED","signatures":[{"field":"`+local+`","signer":"did:jwk:a"}]}', '{"source":"SIGNING_WORKFLOW_COMPLETION","snapshot_hash_algorithm":"SHA-256"}')`, did)
	require.NoError(t, err)
	notary := &recordingNotary{}
	recorder := ExecutionProofRecorder{CRepo: &pg.PostgresContractRepo{}, Storer: fixedStorer{}, Notary: notary}
	record := func() bool {
		t.Helper()
		tx, err := database.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		stored, err := recorder.Record(ctx, tx, ExecutionProofInput{DID: did, LocalPeer: local, PeerDID: peer, PeerJAdES: "eyJ.peer.sig", SignedPDF: []byte("%PDF-both-signatures"), StoredBy: peer})
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
		return stored
	}

	require.False(t, record(), "a proof was written while the local field is unsigned")
	_, err = database.Exec(`INSERT INTO contract_signatures (contract_did, signer_did, credential_type, signature_bytes, status, field_name) VALUES ($1,'did:jwk:a','AES','\x00','SIGNED',$2)`, did, local)
	require.NoError(t, err)

	require.True(t, record())
	require.False(t, record(), "the same proof was written twice")

	var row struct {
		ContentHash string `db:"content_hash"`
		Proof       []byte `db:"proof"`
		Receipt     []byte `db:"notary_receipt"`
	}
	require.NoError(t, database.Get(&row, `SELECT content_hash, proof, notary_receipt FROM contract_execution_proofs WHERE did = $1`, did))
	var proof map[string]any
	require.NoError(t, json.Unmarshal(row.Proof, &proof))
	require.Equal(t, peer, proof["peer_signature"].(map[string]any)["signer"])
	require.Equal(t, "sha256:"+strings.Repeat("a", 64), proof["archive_content_hash"])
	require.Equal(t, []any{map[string]any{"field": local, "signer": "did:jwk:a"}}, proof["signatures"])
	require.NotEmpty(t, proof["pdf_hash"])
	require.NotEmpty(t, row.Receipt)
	require.Len(t, notary.payloads, 2)
	require.Equal(t, "EXECUTION_PROOF", notary.payloads[0].EventType)
	require.Equal(t, row.ContentHash, notary.payloads[0].ContentHash)

	var audited int
	require.NoError(t, database.Get(&audited, `SELECT count(*) FROM outbox_events WHERE event_type = 'STORE_EXECUTION_PROOF' AND did = $1`, did))
	require.Equal(t, 1, audited)
}
