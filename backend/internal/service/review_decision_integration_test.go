package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/semantichub/hubtest"
)

// DCS-FR-CWE-15: rejection returns the contract to draft status. The rejected
// contract accepts edits like a draft.
func TestRejectedContractIsEditablePostgres(t *testing.T) {
	database := hubtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES ('c1',$1,'Org A','REJECTED','tpl',1,'old','{"@id":"c1","@type":"dcs:Contract"}')`, peer)
	require.NoError(t, err)

	name := "reworked"
	handler := command.Updater{DB: database, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: *document}
	require.NoError(t, handler.Handle(context.Background(), command.UpdateCmd{
		DID: "c1", UpdatedAt: time.Now().Add(time.Hour), UpdatedBy: "Org A", Name: &name, CauserDID: peer,
	}))

	var stored string
	require.NoError(t, database.Get(&stored, `SELECT name FROM contracts WHERE did='c1'`))
	require.Equal(t, "reworked", stored)
}

// An edit of a rejected contract follows the resubmission rule: a version a
// signature stands behind is not rewritten.
func TestRejectedContractEditKeepsTheSettlementGatePostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data,pdf_c2pa_state,pdf_signed) VALUES ('c1',$1,'Org A','REJECTED','tpl',1,'old','{"@id":"c1"}','active',true)`, peer)
	require.NoError(t, err)

	name := "reworked"
	handler := command.Updater{DB: database, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: *document}
	err = handler.Handle(context.Background(), command.UpdateCmd{
		DID: "c1", UpdatedAt: time.Now().Add(time.Hour), UpdatedBy: "Org A", Name: &name, CauserDID: peer,
	})
	require.ErrorIs(t, err, command.ErrAgreementSettled)
}
