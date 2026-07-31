package command

import (
	"testing"

	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

// The registrant has to be able to act on the refusal: which library, which
// version, and which hub published it.
func TestMissingShapeLibrariesErrorNamesLibraryVersionAndOrigin(t *testing.T) {
	err := missingShapeLibrariesError([]validation.DeclaredShapeLibrary{
		{Name: "e2e-sla-shapes", Version: 3, Anchor: "https://dcs-a.example.org/semantic/shapes/e2e-sla-shapes?version=3"},
		{Name: "gaia-x-trust", Version: 0},
	})
	require.ErrorContains(t, err, `"e2e-sla-shapes" (version 3)`)
	require.ErrorContains(t, err, "https://dcs-a.example.org/semantic/shapes/e2e-sla-shapes?version=3")
	require.ErrorContains(t, err, `"gaia-x-trust" (no pinned version)`)
	require.ErrorContains(t, err, "Semantic Hub")
}
