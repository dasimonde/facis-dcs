package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// DCS-FR-CWE-15: the approval is logged with the approver's comments.
func TestApproveEventCarriesDecisionNotes(t *testing.T) {
	evt := ApproveCmd{DID: "did:web:c1", ApprovedBy: "Org A", DecisionNotes: []string{"clause 4 checked"}}.event(3)

	require.Equal(t, []string{"clause 4 checked"}, evt.DecisionNotes)
	require.Equal(t, 3, evt.ContractVersion)
}
