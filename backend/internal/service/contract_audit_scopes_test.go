package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"goa.design/goa/v3/security"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
)

// DCS-NFR-BR-05: the audit trail of a contract is read by the audit roles and
// the Contract Manager, not by a machine role (least privilege).
func TestContractAuditTrailIsNotReadByMachineRoles(t *testing.T) {
	scopes := requiredScopes(t, func(auth security.AuthJWTFunc) func(context.Context, any) (any, error) {
		return contractworkflowengine.NewAuditEndpoint(nil, auth)
	}, &contractworkflowengine.ContractAuditRequest{})
	require.ElementsMatch(t, []string{"Auditor", "Contract Manager", "Compliance Officer"}, scopes)
}
