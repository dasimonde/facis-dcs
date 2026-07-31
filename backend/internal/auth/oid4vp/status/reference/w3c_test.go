package reference_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/reference"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtract_W3CBitstring(t *testing.T) {
	credential := status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: map[string]any{
			"credentialStatus": map[string]any{
				"type":                 "BitstringStatusListEntry",
				"statusPurpose":        "revocation",
				"statusListIndex":      "94567",
				"statusListCredential": "https://issuer.example/status/3",
			},
		},
	}

	refs, err := reference.Extract(credential)
	require.NoError(t, err)
	require.Len(t, refs, 1)

	ref := refs[0]
	assert.Equal(t, status.MechanismW3CBitstring, ref.Mechanism)
	assert.Equal(t, "https://issuer.example/status/3", ref.URI)
	assert.Equal(t, uint64(94567), ref.Index)
	assert.Equal(t, "revocation", ref.Purpose)
	assert.Equal(t, uint(1), ref.StatusSize)
	assert.Equal(t, "BitstringStatusListEntry", ref.EntryType)
}

func TestExtract_W3CMultipleEntries(t *testing.T) {
	credential := status.VerifiedCredential{
		Format: "json-ld",
		Claims: map[string]any{
			"credentialStatus": []any{
				map[string]any{
					"type":                 "BitstringStatusListEntry",
					"statusPurpose":        "revocation",
					"statusListCredential": "https://issuer.example/status/1",
					"statusListIndex":      "42",
				},
				map[string]any{
					"type":                 "BitstringStatusListEntry",
					"statusPurpose":        "suspension",
					"statusListCredential": "https://issuer.example/status/2",
					"statusListIndex":      "43",
				},
			},
		},
	}

	refs, err := reference.Extract(credential)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	assert.Equal(t, "revocation", refs[0].Purpose)
	assert.Equal(t, uint64(43), refs[1].Index)
}

func TestExtract_RequiresType(t *testing.T) {
	credential := status.VerifiedCredential{
		Claims: map[string]any{
			"credentialStatus": map[string]any{
				"statusListCredential": "https://issuer.example/status/1",
				"statusListIndex":      "1",
			},
		},
	}

	_, err := reference.Extract(credential)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentialStatus.type is required")
}

func TestExtract_RejectsUnknownW3CType(t *testing.T) {
	credential := status.VerifiedCredential{
		Claims: map[string]any{
			"credentialStatus": map[string]any{
				"type":                 "UnknownStatusEntry",
				"statusListCredential": "https://issuer.example/status/1",
				"statusListIndex":      "1",
			},
		},
	}

	_, err := reference.Extract(credential)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported credentialStatus.type")
}

func TestExtract_W3CStringIndex(t *testing.T) {
	credential := status.VerifiedCredential{
		Claims: map[string]any{
			"credentialStatus": map[string]any{
				"type":                 "BitstringStatusListEntry",
				"statusPurpose":        "revocation",
				"statusListCredential": "https://issuer.example/status/1",
				"statusListIndex":      "60538",
			},
		},
	}

	refs, err := reference.Extract(credential)
	require.NoError(t, err)
	assert.Equal(t, uint64(60538), refs[0].Index)
}
