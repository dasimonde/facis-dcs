package status_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapIETFResult_ReservedValues(t *testing.T) {
	ref := status.Reference{Mechanism: status.MechanismIETFToken}
	assert.Equal(t, status.StateApplicationSpecific, status.MapIETFResult(ref, 3).State)
	assert.Equal(t, status.StateUnknown, status.MapIETFResult(ref, 4).State)
}

func TestStrictPolicy_AcceptsValid(t *testing.T) {
	policy := status.StrictPolicy{}
	credential := status.VerifiedCredential{Format: "sd-jwt"}
	result, err := policy.Evaluate(credential, []status.Result{{
		State: status.StateValid,
	}})
	require.NoError(t, err)
	require.True(t, result.Accepted)
}

func TestMapIETFResult(t *testing.T) {
	ref := status.Reference{Mechanism: status.MechanismIETFToken}
	assert.Equal(t, status.StateValid, status.MapIETFResult(ref, 0).State)
	assert.Equal(t, status.StateInvalid, status.MapIETFResult(ref, 1).State)
	assert.Equal(t, status.StateSuspended, status.MapIETFResult(ref, 2).State)
}

func TestMapW3CResult(t *testing.T) {
	ref := status.Reference{Purpose: "revocation"}
	assert.Equal(t, status.StateValid, status.MapW3CResult(ref, 0).State)
	assert.Equal(t, status.StateInvalid, status.MapW3CResult(ref, 1).State)
}

func TestStrictPolicy_RejectsUnknownState(t *testing.T) {
	policy := status.StrictPolicy{}
	result, err := policy.Evaluate(status.VerifiedCredential{}, []status.Result{{
		State: status.StateUnknown,
	}})
	require.NoError(t, err)
	require.False(t, result.Accepted)
}
