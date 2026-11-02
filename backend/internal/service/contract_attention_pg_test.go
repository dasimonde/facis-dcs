package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
)

// DCS-FR-CSA-23: the attention list names the readable contracts whose notice
// period has started and those that expired or were terminated recently.
func TestAttentionListsRenewalDueExpiredAndTerminatedContractsPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data,exp_date,exp_notice_period) VALUES
		('due','','Org A','ACTIVE','tpl',1,'Due','{}',now() + interval '5 days',10),
		('later','','Org A','ACTIVE','tpl',1,'Later','{}',now() + interval '60 days',10),
		('expired','','Org A','ACTIVE','tpl',1,'Expired','{}',now() - interval '2 days',10),
		('long-expired','','Org A','EXPIRED','tpl',1,'Old','{}',now() - interval '60 days',10),
		('terminated','','Org A','TERMINATED','tpl',1,'Ended','{}',NULL,NULL),
		('foreign','','Org B','ACTIVE','tpl',1,'Foreign','{}',now() + interval '5 days',10)`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_terminations (did,reason,effective_at,terminated_by,initiating_role) VALUES ('terminated','breach',now() - interval '1 day','Org A','Contract Manager')`)
	require.NoError(t, err)
	engine := &contractWorkflowEnginesrvc{DB: database, DIDDocument: *jadesTestDIDDocument(t, "local")}
	ctx := middleware.InjectAuthContext(context.Background(), []string{userrole.ContractManager.String()}, "holder", "Org A")

	items, err := engine.Attention(ctx, &cwe.ContractAttentionRequest{})
	require.NoError(t, err)

	reasons := map[string]string{}
	for _, item := range items {
		reasons[item.Did] = item.Reason
	}
	require.Equal(t, map[string]string{"due": "renewal_due", "expired": "expired", "terminated": "terminated"}, reasons)
}
