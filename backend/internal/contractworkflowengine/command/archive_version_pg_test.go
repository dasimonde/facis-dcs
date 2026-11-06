package command

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// DCS-FR-CSA-12: this instance signed first and archived its version; the
// counterparty's countersigned copy increases the contract version. The
// archived version stays listed with its own version number.
func TestArchivedVersionStaysListedAfterAPeerVersionBump(t *testing.T) {
	signedByB := withSignature(unsignedBase, "B")
	receiver, database := signingConflictReceiver(t, signedByB)
	ctx := context.Background()
	_, err := database.Exec(`UPDATE contracts SET state = 'SIGNED' WHERE did = $1`, taskContractDID)
	require.NoError(t, err)

	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	contract, err := receiver.CRepo.ReadDataByDID(ctx, tx, taskContractDID)
	require.NoError(t, err)
	entry, err := BuildArchiveEntry(contract, nil, "Org B", []ArchiveSigningEvidence{{
		Signer: "did:jwk:b", CredentialType: "AES", CeremonyID: "c1", Field: localPeerDID,
		PDFCID: "cid", PDFHash: "abc", SignedAt: time.Now(),
	}}, nil)
	require.NoError(t, err)
	entry.SnapshotCID = "bafy-snapshot"
	require.NoError(t, receiver.CRepo.StoreArchiveEntry(ctx, tx, entry))
	require.NoError(t, tx.Commit())

	require.NoError(t, receiver.Handle(ctx, shipOf(withSignature(signedByB, "A"))))
	var version int
	require.NoError(t, database.Get(&version, `SELECT contract_version FROM contracts WHERE did = $1`, taskContractDID))
	require.Greater(t, version, entry.ContractVersion, "the peer ship increases the contract version")

	tx, err = database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	rows, err := receiver.CRepo.ReadArchivedContractsByFilter(contractreadpolicy.WithSystemScope(ctx), tx, db.SearchValues{DID: taskContractDID})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, entry.ContractVersion, rows[0].ContractVersion, "the entry reports its own archived version")
}
