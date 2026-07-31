package provenance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractCredentialStatus_ReadsTheEntry(t *testing.T) {
	ref, present, err := ExtractCredentialStatus([]byte(`{
		"credentialStatus": {
			"type": "TokenStatusList",
			"statusListCredential": "http://statuslist/v1/tenants/default/status/1",
			"statusListIndex": "4711"
		}
	}`))
	require.NoError(t, err)
	require.True(t, present)
	assert.Equal(t, "http://statuslist/v1/tenants/default/status/1", ref.StatusListCredential)
	assert.Equal(t, uint32(4711), ref.Index)
}

// A credential that advertises no status has nothing to check, which is not a
// finding.
func TestExtractCredentialStatus_ReportsAnAbsentEntryAsNothingToCheck(t *testing.T) {
	ref, present, err := ExtractCredentialStatus([]byte(`{"type":["VerifiableCredential"]}`))
	require.NoError(t, err)
	assert.False(t, present)
	assert.Zero(t, ref)
}

// The fail-open this closes: an entry present but unreadable used to be reported
// exactly like an absent one, so the caller skipped the revocation check without
// saying so and a revoked contract verified clean.
func TestExtractCredentialStatus_ReportsAnUnreadableEntryAsIndeterminate(t *testing.T) {
	for name, vc := range map[string]string{
		"not an object":      `{"credentialStatus": "revoked"}`,
		"no locator":         `{"credentialStatus": {"type": "TokenStatusList"}}`,
		"index not a number": `{"credentialStatus": {"statusListCredential": "http://statuslist/1", "statusListIndex": "first"}}`,
		"unreadable json":    `{`,
	} {
		t.Run(name, func(t *testing.T) {
			_, present, err := ExtractCredentialStatus([]byte(vc))
			require.Error(t, err)
			assert.True(t, present, "an entry that cannot be read is not an absent entry")
		})
	}
}
