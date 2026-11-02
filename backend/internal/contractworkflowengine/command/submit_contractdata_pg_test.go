package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// DCS-FR-CWE-08, DCS-FR-CWE-02: a submit that carries contract_data saves the
// document like an update. The replaced version goes to the history with the
// actor, and a parent that loops back to the contract is refused.
func TestSubmitWithContractDataKeepsTheVersionAndChecksTheParentPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`
		INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES
		('a','did:web:local','Org A','DRAFT','tpl',1,'a','{}'),
		('b','did:web:local','Org A','DRAFT','tpl',1,'b','{"dcs:parentContract":{"@id":"a"}}')`)
	require.NoError(t, err)
	h := &Submitter{DB: database, CRepo: &cwepg.PostgresContractRepo{}}
	ctx := context.Background()
	submit := func(parent string) error {
		var data map[string]any
		require.NoError(t, json.Unmarshal(minimalCanonicalContractData(t, "a"), &data))
		if parent != "" {
			data["dcs:parentContract"] = map[string]any{"@id": parent}
		}
		raw, err := datatype.NewJSON(data)
		require.NoError(t, err)
		tx, err := database.BeginTxx(ctx, nil)
		require.NoError(t, err)
		_, err = h.contractDataForSemanticValidation(ctx, tx, SubmitCmd{DID: "a", SubmittedBy: "Org A", ContractData: &raw})
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}

	err = submit("b")
	require.ErrorIs(t, err, ErrContractHierarchyCycle)

	require.NoError(t, submit(""))
	var history []struct {
		ModifiedBy string `db:"modified_by"`
	}
	require.NoError(t, database.Select(&history, `SELECT modified_by FROM contract_history WHERE did = 'a'`))
	require.Len(t, history, 1)
	require.Equal(t, "Org A", history[0].ModifiedBy)
}

// DCS-IR-CWE-03: a Contract Manager who negotiates a round may also close it.
// Without an engagement in the round the submit is refused as not settled,
// not as a missing permission.
func TestContractManagerSubmitsANegotiationRoundPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES
		('a',$1,'Org A','NEGOTIATION','tpl',1,'a','{}')`, localPeerDID)
	require.NoError(t, err)
	h := &Submitter{
		DB: database, CRepo: &cwepg.PostgresContractRepo{}, NTRepo: &cwepg.PostgresNegotiationTaskRepo{},
		DIDDocument: *taskTestDIDDocument(t),
	}
	err = h.Handle(context.Background(), SubmitCmd{
		DID: "a", UpdatedAt: time.Now().Add(time.Hour), SubmittedBy: "Org A", CauserDID: localPeerDID,
		UserRoles: userrole.UserRoles{userrole.ContractManager},
	})
	require.ErrorIs(t, err, ErrNegotiationNotSettled)
}
