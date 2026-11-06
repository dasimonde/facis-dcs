package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
)

// DCS-FR-CSA-23: a contract webhook event names the creating organization and
// the responsible parties, so an ORCE flow can route it to the contract owner.
func TestContractEventOwnerNamesCreatorAndResponsiblePostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data,responsible) VALUES
		('did:web:a:contract:1','','did:web:org-a','ACTIVE','tpl',1,'{}','{"creator":"did:web:org-a","counterparty":"did:web:org-b","approvers":["did:key:approver"]}')`)
	require.NoError(t, err)
	owner := ContractEventOwner(database)

	var got struct {
		CreatedBy   string `json:"created_by"`
		Responsible struct {
			Creator      string   `json:"creator"`
			Counterparty string   `json:"counterparty"`
			Approvers    []string `json:"approvers"`
		} `json:"responsible"`
	}
	require.NoError(t, json.Unmarshal(owner(context.Background(), "did:web:a:contract:1"), &got))
	require.Equal(t, "did:web:org-a", got.CreatedBy)
	require.Equal(t, "did:web:org-b", got.Responsible.Counterparty)
	require.Equal(t, []string{"did:key:approver"}, got.Responsible.Approvers)
	require.Nil(t, owner(context.Background(), "did:web:a:contract:unknown"))
}
