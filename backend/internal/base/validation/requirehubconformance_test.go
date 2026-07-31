package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The submission/signing gate: error-severity hub-shape findings block the
// document; a conformant one passes (DCS-FR-TR-20, DCS-FR-PACM-03).
func TestRequireHubConformance(t *testing.T) {
	restore := swapGaiaXShapeSource(t)
	defer restore()

	conformant := canonicalAuditContract()
	conformant["dcs:typedClause"] = gaiaXParticipantInstance()
	require.NoError(t, RequireHubConformance(context.Background(), conformant))

	violating := canonicalAuditContract()
	participant := gaiaXParticipantInstance()
	delete(participant, "https://w3id.org/gaia-x/development#legalName")
	violating["dcs:typedClause"] = participant

	err := RequireHubConformance(context.Background(), violating)
	require.Error(t, err)
	require.Contains(t, err.Error(), "violates Semantic Hub shapes")
	require.Contains(t, err.Error(), "legalName")
}
