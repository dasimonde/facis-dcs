package processauditandcompliance

import (
	"testing"

	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

func TestContractContentPolicyFindingEventDataIncludesPolicyDetails(t *testing.T) {
	data := ContractContentPolicyFindingEventData(validation.PolicyFinding{
		PolicySetID:   "policy-set",
		PolicyVersion: "v1",
		RuleID:        "availability-minimum",
		Message:       "Service availability must satisfy policy minimum.",
		Requirement:   "service.sla.availability must be >= 99.9",
		ActualValue:   99.5,
		ExpectedValue: 99.9,
		Operator:      "gte",
	}, validation.ContractContentAuditMetadata{
		ContractDID: "did:example:contract",
		AuditedBy:   "tester",
	})

	require.Equal(t, "service.sla.availability must be >= 99.9", data["requirement"])
	require.Equal(t, 99.5, data["actualValue"])
	require.Equal(t, 99.9, data["expectedValue"])
	require.Equal(t, "gte", data["operator"])
}
