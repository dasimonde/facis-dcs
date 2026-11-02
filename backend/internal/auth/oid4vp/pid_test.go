package oid4vp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DCS-NFR-SEC-18, DCS-NFR-SQ-04: the PID request names the claims the DCS
// uses, and every claim set ends with the names alone.
func TestPIDQueryRequestsOnlyTheClaimsTheDCSUses(t *testing.T) {
	raw, err := json.Marshal(DefaultPIDDCQLQuery())
	require.NoError(t, err)
	var query struct {
		Credentials []struct {
			Claims []struct {
				ID   string   `json:"id"`
				Path []string `json:"path"`
			} `json:"claims"`
			ClaimSets [][]string `json:"claim_sets"`
		} `json:"credentials"`
	}
	require.NoError(t, json.Unmarshal(raw, &query))
	require.Len(t, query.Credentials, 2)
	for _, credential := range query.Credentials {
		var paths [][]string
		ids := map[string]bool{}
		for _, claim := range credential.Claims {
			paths = append(paths, claim.Path)
			ids[claim.ID] = true
		}
		assert.Equal(t, PIDClaimPaths, paths)
		require.NotEmpty(t, credential.ClaimSets)
		for _, set := range credential.ClaimSets {
			for _, id := range set {
				assert.True(t, ids[id], "claim set names an unknown claim %q", id)
			}
		}
		assert.Equal(t, []string{"given_name", "family_name"}, credential.ClaimSets[len(credential.ClaimSets)-1])
	}
}

// A wallet that discloses more than was requested has the rest dropped
// before the claims are stored.
func TestVerifiedPIDKeepsOnlyTheRequestedClaims(t *testing.T) {
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	pid := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, map[string]any{
		"given_name":  "Erika",
		"family_name": "Mustermann",
		"birth_date":  "1964-08-12",
		"nationality": "DE",
		"address":     map[string]any{"street_address": "Heidestrasse 17", "locality": "Koeln"},
	})
	trust := peerTrust(t, issuerKey, []Purpose{PurposePID}, []string{testPoAParty}, list)

	verified, err := NewVerifier(trust, PurposePID).VerifyPID(pid.Presentation, PresentationContext{
		Nonce:    "ceremony-nonce",
		ClientID: "https://the-signing-instance.example",
	})
	require.NoError(t, err)
	assert.Equal(t, pid.SignatoryDID, verified.SubjectDID)

	var claims map[string]any
	require.NoError(t, json.Unmarshal(verified.RawClaims, &claims))
	assert.Equal(t, map[string]any{
		"given_name":  "Erika",
		"family_name": "Mustermann",
		"birth_date":  "1964-08-12",
		"address":     map[string]any{"street_address": "Heidestrasse 17"},
	}, claims)
}
