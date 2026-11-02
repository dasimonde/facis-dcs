package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/middleware"
)

// DCS-FR-CWE-15: the approver's comments reach the approval command both when
// the approval runs at once and when the approval gate resumes it after review
// from the stored continuation.
func TestApprovalKeepsDecisionNotesDirectAndResumed(t *testing.T) {
	ctx := middleware.InjectAuthContext(context.Background(), []string{userrole.ContractApprover.String()}, "holder", "Org A")
	updatedAt := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	continuation := approvalContinuation(ctx, "did:web:local", []string{"clause 4 checked"})

	stored, err := json.Marshal(continuation)
	require.NoError(t, err)
	var resumed map[string]any
	require.NoError(t, json.Unmarshal(stored, &resumed))

	for name, c := range map[string]map[string]any{"direct": continuation, "resumed": resumed} {
		cmd := approveCmd("did:contract:1", updatedAt, c)
		require.Equal(t, []string{"clause 4 checked"}, cmd.DecisionNotes, name)
		require.Equal(t, "Org A", cmd.ApprovedBy, name)
		require.Equal(t, "holder", cmd.HolderDID, name)
		require.Equal(t, "did:web:local", cmd.CauserDID, name)
		require.Equal(t, userrole.UserRoles{userrole.ContractApprover}, cmd.UserRoles, name)
	}
}
