package validation

import (
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func documentDeclaring(t *testing.T, shapesGraph any) *datatype.JSON {
	t.Helper()
	raw, err := datatype.NewJSON(map[string]any{"sh:shapesGraph": shapesGraph})
	require.NoError(t, err)
	return &raw
}

// A template authored on another deployment carries that deployment's hub
// URL, so the consumer can say where a library it does not hold lives.
func TestDeclaredShapeLibrariesKeepsTheOriginatingAnchor(t *testing.T) {
	document := documentDeclaring(t, []any{
		map[string]any{"@id": "https://dcs-a.example.org/semantic/shapes/facis-dcs?version=1"},
		map[string]any{"@id": "https://dcs-a.example.org/semantic/shapes/e2e-sla-shapes?version=3"},
		SchemaSHACLShapesV1,
	})

	libraries, err := DeclaredShapeLibraries(document)
	require.NoError(t, err)
	require.Equal(t, []DeclaredShapeLibrary{
		{Name: "facis-dcs", Version: 1, Anchor: "https://dcs-a.example.org/semantic/shapes/facis-dcs?version=1"},
		{Name: "e2e-sla-shapes", Version: 3, Anchor: "https://dcs-a.example.org/semantic/shapes/e2e-sla-shapes?version=3"},
	}, libraries)
}

func TestDeclaredShapeLibrariesReadsTheSingleAndUnpinnedForms(t *testing.T) {
	single := documentDeclaring(t, map[string]any{"@id": "/semantic/shapes/e2e-sla-shapes"})
	libraries, err := DeclaredShapeLibraries(single)
	require.NoError(t, err)
	require.Equal(t, []DeclaredShapeLibrary{
		{Name: "e2e-sla-shapes", Version: 0, Anchor: "/semantic/shapes/e2e-sla-shapes"},
	}, libraries)

	none, err := DeclaredShapeLibraries(documentDeclaring(t, SchemaSHACLShapesV1))
	require.NoError(t, err)
	require.Empty(t, none)

	empty, err := DeclaredShapeLibraries(nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
