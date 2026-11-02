package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// DCS-FR-CWE-03: submit runs the required metadata check for a draft and for a
// rejected contract before the contract enters negotiation.
func TestSubmitRefusesContractWithoutRequiredMetadataPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES
		('draft',$1,'Org A','DRAFT','template',1,'Service Agreement','{}'),
		('rejected',$1,'Org A','REJECTED','template',1,'Service Agreement','{}')`, peer)
	require.NoError(t, err)

	submitter := &command.Submitter{DB: database, CRepo: &cwepg.PostgresContractRepo{}, DIDDocument: *document}
	for _, did := range []string{"draft", "rejected"} {
		err := submitter.Handle(context.Background(), command.SubmitCmd{
			DID:         did,
			UpdatedAt:   time.Now().Add(time.Hour),
			SubmittedBy: "Org A",
			UserRoles:   userrole.UserRoles{userrole.ContractCreator},
			CauserDID:   peer,
		})
		require.ErrorIs(t, err, command.ErrMissingRequiredMetadata, did)
		require.ErrorContains(t, err, "description is missing", did)
	}
}
