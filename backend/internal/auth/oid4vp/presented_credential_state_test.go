package oid4vp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp/status"
)

// The compliance viewer reads the live status of the Power of Attorney a
// signatory presented. The same stored presentation must follow the list:
// valid before the issuer revokes it, revoked after.
func TestPresentedCredentialState_FollowsTheLiveStatusList(t *testing.T) {
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	state, err := PresentedCredentialState(context.Background(), poa.Presentation, trust, PurposePeer)
	require.NoError(t, err)
	assert.Equal(t, status.StateValid, state)

	list.Revoke(testPoAStatusIndex)

	state, err = PresentedCredentialState(context.Background(), poa.Presentation, trust, PurposePeer)
	require.NoError(t, err)
	assert.Equal(t, status.StateInvalid, state)
}

// The compliance check reads the live status of the PID a signatory presented
// (DCS-FR-SM-21). A PID carries no roles and no delegation, and its status
// follows the issuer's list like any other credential's.
func TestPresentedCredentialState_ReadsThePIDStatus(t *testing.T) {
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	pid := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, map[string]any{"roles": nil})
	trust := peerTrust(t, issuerKey, []Purpose{PurposePID}, []string{testPoAParty}, list)

	state, err := PresentedCredentialState(context.Background(), pid.Presentation, trust, PurposePID)
	require.NoError(t, err)
	assert.Equal(t, status.StateValid, state)

	list.Revoke(testPoAStatusIndex)

	state, err = PresentedCredentialState(context.Background(), pid.Presentation, trust, PurposePID)
	require.NoError(t, err)
	assert.Equal(t, status.StateInvalid, state)
}

// A list that cannot be fetched gives no state. The caller reports UNKNOWN; a
// default of "valid" would hide a revocation for as long as the outage lasts.
func TestPresentedCredentialState_UnreachableListIsAnError(t *testing.T) {
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, map[string]any{
		"status": map[string]any{"status_list": map[string]any{"uri": "http://127.0.0.1:1/status-list/1", "idx": 1}},
	})
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	state, err := PresentedCredentialState(context.Background(), poa.Presentation, trust, PurposePeer)
	require.Error(t, err)
	assert.Empty(t, state)
}

// The status entry is the credential author's pointer, so it is followed only
// for a credential an issuer trusted for the purpose signed.
func TestPresentedCredentialState_UntrustedIssuerIsNotFollowed(t *testing.T) {
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposeLogin}, []string{testPoAParty}, list)

	_, err := PresentedCredentialState(context.Background(), poa.Presentation, trust, PurposePeer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not trusted")
}

func TestPresentedCredentialState_ChecksEveryReference(t *testing.T) {
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)
	entry := func(index string) map[string]any {
		return map[string]any{"type": "TokenStatusList", "statusListCredential": list.URL, "statusListIndex": index, "statusPurpose": "revocation"}
	}
	list.Revoke(2)
	for _, indices := range [][]string{{"1", "2"}, {"2", "1"}} {
		t.Run(indices[0]+"_"+indices[1], func(t *testing.T) {
			poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, map[string]any{
				"status": nil, "credentialStatus": []any{entry(indices[0]), entry(indices[1])},
			})
			state, err := PresentedCredentialState(context.Background(), poa.Presentation, trust, PurposePeer)
			require.NoError(t, err)
			assert.Equal(t, status.StateInvalid, state)
		})
	}
}

func TestPresentedCredentialState_RequiresEveryResultAndPolicyToAccept(t *testing.T) {
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	for _, state := range []status.State{status.StateInvalid, status.StateSuspended, status.StateUnknown, status.StateApplicationSpecific, status.StateRefreshRequired} {
		for position := range 3 {
			results := []status.Result{{State: status.StateValid}, {State: status.StateValid}, {State: status.StateValid}}
			results[position].State = state
			statusListVerifier.Policy = presentedStatePolicy{CredentialVerificationResult: status.CredentialVerificationResult{Accepted: state == status.StateRefreshRequired, StatusResults: results}}
			actual, err := PresentedCredentialState(context.Background(), poa.Presentation, trust, PurposePeer)
			require.NoError(t, err)
			assert.Equal(t, state, actual, "non-valid result at position %d", position)
		}
	}
	for _, tc := range []struct {
		name      string
		accepted  bool
		results   []status.Result
		wantError bool
	}{
		{"all valid", true, []status.Result{{State: status.StateValid}, {State: status.StateValid}}, false},
		{"policy rejected", false, []status.Result{{State: status.StateValid}}, true},
		{"no results", false, nil, true},
		{"empty accepted result", true, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statusListVerifier.Policy = presentedStatePolicy{CredentialVerificationResult: status.CredentialVerificationResult{Accepted: tc.accepted, StatusResults: tc.results, Reason: "test policy verdict"}}
			actual, err := PresentedCredentialState(context.Background(), poa.Presentation, trust, PurposePeer)
			if tc.wantError {
				require.Error(t, err)
				assert.Empty(t, actual)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, status.StateValid, actual)
		})
	}
}

type presentedStatePolicy struct {
	status.StrictPolicy
	status.CredentialVerificationResult
}

func (p presentedStatePolicy) Evaluate(status.VerifiedCredential, []status.Result) (status.CredentialVerificationResult, error) {
	return p.CredentialVerificationResult, nil
}

func TestMostSevereStatus_RevokedOverSuspendedOverOthers(t *testing.T) {
	for _, tc := range []struct {
		states []status.State
		want   status.State
	}{
		{[]status.State{status.StateSuspended, status.StateInvalid}, status.StateInvalid},
		{[]status.State{status.StateUnknown, status.StateSuspended, status.StateValid}, status.StateSuspended},
		{[]status.State{status.StateValid, status.StateUnknown}, status.StateUnknown},
		{[]status.State{status.StateValid, status.StateValid}, status.StateValid},
	} {
		results := make([]status.Result, len(tc.states))
		for i, state := range tc.states {
			results[i] = status.Result{State: state, Index: uint64(i)}
		}
		assert.Equal(t, tc.want, mostSevereStatus(results).State, "%v", tc.states)
	}
	err := mapStatusListRejection(status.CredentialVerificationResult{StatusResults: []status.Result{{State: status.StateValid, Index: 4}, {State: status.StateInvalid, Index: 7}}})
	assert.EqualError(t, err, "credential status list index 7 is revoked")
}
