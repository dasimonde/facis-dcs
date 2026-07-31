package reference_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/reference"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtract_IETFTokenStatusList(t *testing.T) {
	credential := status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: map[string]any{
			"status": map[string]any{
				"status_list": map[string]any{
					"idx": 94567,
					"uri": "https://issuer.example/status/3",
				},
			},
		},
	}

	refs, err := reference.Extract(credential)
	require.NoError(t, err)
	require.Len(t, refs, 1)

	ref := refs[0]
	assert.Equal(t, status.MechanismIETFToken, ref.Mechanism)
	assert.Equal(t, "https://issuer.example/status/3", ref.URI)
	assert.Equal(t, uint64(94567), ref.Index)
}

func TestExtract_IETFTrimsURIWhitespace(t *testing.T) {
	credential := status.VerifiedCredential{
		Claims: map[string]any{
			"status": map[string]any{
				"status_list": map[string]any{
					"idx": 0,
					"uri": " https://issuer.example/status/3",
				},
			},
		},
	}

	refs, err := reference.Extract(credential)
	require.NoError(t, err)
	assert.Equal(t, "https://issuer.example/status/3", refs[0].URI)
}
