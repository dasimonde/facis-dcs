package command

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/db/pg"
)

// A revocation that arrives while a signature holds the per-contract lock
// waits for that signature, and then revokes. The contract ends REVOKED, not
// SIGNED.
func TestRevocationWaitsForTheSignatureInProgressAndWins(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	const did, signerA = "did:example:race", "did:example:signer-a"
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version) VALUES ($1,'','Org A','SIGNED','tpl',1)`, did)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_signatures (contract_did, signer_did, credential_type, signature_bytes, status, field_name)
		VALUES ($1,$2,'AES','\x00','SIGNED','did:example:party-a')`, did, signerA)
	require.NoError(t, err)
	repo := &pg.PostgresContractRepo{}

	// The second signature of the contract, in submitSignature's order: the
	// per-contract lock, the state read under it, the state write.
	submit, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = submit.Rollback() }()
	require.NoError(t, acquireRegenerationLock(ctx, submit, did))
	current, err := repo.ReadProcessDataByDID(ctx, submit, did)
	require.NoError(t, err)
	require.NoError(t, contractstate.ValidateTransition(contractstate.ContractState(current.State), contractstate.EventSign))

	revokeDone := make(chan error, 1)
	go func() {
		revokeDone <- (&Revoker{DB: database, CRepo: repo}).Handle(ctx, RevokeCmd{DID: did, SignerDID: signerA, Reason: "signer credential revoked", RevokedBy: "admin"})
	}()
	select {
	case err := <-revokeDone:
		t.Fatalf("the revocation committed while the signature held the contract lock: %v", err)
	case <-time.After(time.Second):
	}

	require.NoError(t, repo.UpdateState(ctx, submit, did, current.State, contractstate.Signed.String()))
	require.NoError(t, submit.Commit())
	select {
	case err := <-revokeDone:
		require.NoError(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("the revocation did not finish after the signature committed")
	}

	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did = $1`, did))
	require.Equal(t, contractstate.Revoked.String(), state)
}
