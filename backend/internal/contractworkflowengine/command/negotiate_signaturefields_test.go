package command

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

// signatureFieldsOf decodes the dcs:signatureFields of a contract document as a
// map from signatory name (the instance DID) to the field's @id.
func signatureFieldsOf(t *testing.T, raw datatype.JSON) map[string]string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	out := map[string]string{}
	fields, _ := doc["dcs:signatureFields"].([]any)
	for _, rawField := range fields {
		node := rawField.(map[string]any)
		require.Equal(t, "dcs:SignatureField", node["@type"])
		id, _ := node["@id"].(string)
		require.NotEmpty(t, id)
		name, _ := node["dcs:signatoryName"].(string)
		if name == "" {
			name, _ = node["signatoryName"].(string)
		}
		out[name] = id
	}
	return out
}

// TestSeedSignatureFieldsDistinctDIDs proves each distinct participating
// instance DID yields exactly one dcs:SignatureField whose dcs:signatoryName is
// that DID.
func TestSeedSignatureFieldsDistinctDIDs(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{"@id": "urn:contract:1", "@type": "dcs:Contract"})
	require.NoError(t, err)

	seeded, changed, err := SeedSignatureFields(raw, []string{"did:web:origin", "did:web:peer"})
	require.NoError(t, err)
	require.True(t, changed)

	fields := signatureFieldsOf(t, seeded)
	require.Len(t, fields, 2)
	require.Contains(t, fields, "did:web:origin")
	require.Contains(t, fields, "did:web:peer")
	// @ids are document-anchored and distinct per instance.
	require.NotEqual(t, fields["did:web:origin"], fields["did:web:peer"])
	require.Contains(t, fields["did:web:origin"], "urn:contract:1#signature-field-")
}

// TestSeedSignatureFieldsSingleInstance proves a single participating DID gets
// exactly one field.
func TestSeedSignatureFieldsSingleInstance(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{"@id": "urn:contract:1", "@type": "dcs:Contract"})
	require.NoError(t, err)

	seeded, changed, err := SeedSignatureFields(raw, []string{"did:web:solo"})
	require.NoError(t, err)
	require.True(t, changed)

	fields := signatureFieldsOf(t, seeded)
	require.Len(t, fields, 1)
	require.Contains(t, fields, "did:web:solo")
}

// TestSeedSignatureFieldsIdempotent proves re-running the seed over its own
// output adds nothing and reports no change, with @ids stable across runs.
func TestSeedSignatureFieldsIdempotent(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{"@id": "urn:contract:1", "@type": "dcs:Contract"})
	require.NoError(t, err)

	dids := []string{"did:web:origin", "did:web:peer"}
	firstRun, changed, err := SeedSignatureFields(raw, dids)
	require.NoError(t, err)
	require.True(t, changed)
	first := signatureFieldsOf(t, firstRun)

	secondRun, changed, err := SeedSignatureFields(firstRun, dids)
	require.NoError(t, err)
	require.False(t, changed)

	second := signatureFieldsOf(t, secondRun)
	require.Equal(t, first, second)
}

// TestSeedSignatureFieldsExplicitDeclarationWins proves that a contract which
// already declares its signature fields is signed against exactly that
// declaration: the per-party auto-seed adds nothing on top (no extra
// instance-DID field) and reports no change, so an explicitly authored
// multi-signatory contract is never silently augmented.
func TestSeedSignatureFieldsExplicitDeclarationWins(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{
		"@id":   "urn:contract:1",
		"@type": "dcs:Contract",
		"dcs:signatureFields": []any{
			map[string]any{
				"@id":               "urn:uuid:template-field",
				"@type":             "dcs:SignatureField",
				"dcs:signatoryName": "did:web:origin",
			},
		},
	})
	require.NoError(t, err)

	seeded, changed, err := SeedSignatureFields(raw, []string{"did:web:origin", "did:web:peer"})
	require.NoError(t, err)
	require.False(t, changed)

	fields := signatureFieldsOf(t, seeded)
	require.Len(t, fields, 1)
	require.Equal(t, "urn:uuid:template-field", fields["did:web:origin"])
	require.NotContains(t, fields, "did:web:peer")
}
