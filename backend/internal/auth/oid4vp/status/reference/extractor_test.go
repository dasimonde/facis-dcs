package reference_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/reference"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtract_RejectsMixedModels(t *testing.T) {
	credential := status.VerifiedCredential{
		Claims: map[string]any{
			"credentialStatus": map[string]any{
				"type":                 "BitstringStatusListEntry",
				"statusListCredential": "https://issuer.example/status/1",
				"statusListIndex":      "1",
			},
			"status": map[string]any{
				"status_list": map[string]any{
					"uri": "https://issuer.example/status/2",
					"idx": 1,
				},
			},
		},
	}

	_, err := reference.Extract(credential)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incompatible")
}
