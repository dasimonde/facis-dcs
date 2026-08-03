package command

import (
	"testing"

	"digital-contracting-service/internal/semantichub"

	"github.com/stretchr/testify/require"
)

func TestEffectiveBundleRefsPinEverySelectedVersionExactly(t *testing.T) {
	old := semantichub.EffectiveBundle{
		ContextVersion: 2,
		ProfileVersion: 4,
		Shapes: []semantichub.Schema{
			{Name: semantichub.ShapesName, Version: 3},
			{Name: semantichub.ClauseCatalogName, Version: 5},
		},
		Libraries: []semantichub.Schema{{Name: "customer-library", Version: 7}},
	}
	activated := semantichub.EffectiveBundle{
		ContextVersion: old.ContextVersion,
		ProfileVersion: 5,
		Shapes: []semantichub.Schema{
			{Name: semantichub.ShapesName, Version: 4},
			{Name: semantichub.ClauseCatalogName, Version: 5},
		},
		Libraries: []semantichub.Schema{{Name: "customer-library", Version: 8}},
	}

	oldRefs, err := effectiveBundleRefs(old)
	require.NoError(t, err)
	newRefs, err := effectiveBundleRefs(activated)
	require.NoError(t, err)
	rollbackRefs, err := effectiveBundleRefs(old)
	require.NoError(t, err)

	require.Equal(t, []string{
		"/semantic/shapes/facis-dcs?version=4",
		"/semantic/shapes/clause-catalog?version=5",
	}, newRefs.Shapes)
	require.Equal(t, []string{"/semantic/shapes/customer-library?version=8"}, newRefs.Libraries)
	require.Equal(t,
		"/semantic/profile/facis.sla.basic?version=5",
		newRefs.Profile,
	)
	require.NotEqual(t, oldRefs.CanonicalShapes, newRefs.CanonicalShapes)
	require.NotEqual(t, oldRefs.Libraries[0], newRefs.Libraries[0])
	require.NotEqual(t, oldRefs.Profile, newRefs.Profile)
	require.Equal(t, oldRefs, rollbackRefs)
}

// The pin a contract carries is what its counterparty has to resolve and what
// its ship has to carry, so it names the envelope alone. A registered library
// reaches the pin only through the contract's own sh:shapesGraph, which is
// PinSemanticBundle's job.
func TestEffectiveBundleRefsPinTheEnvelopeAndNotEveryRegisteredLibrary(t *testing.T) {
	refs, err := effectiveBundleRefs(semantichub.EffectiveBundle{
		ContextVersion: 1,
		ProfileVersion: 1,
		Shapes: []semantichub.Schema{
			{Name: semantichub.ShapesName, Version: 1},
			{Name: semantichub.ClauseCatalogName, Version: 1},
		},
		Libraries: []semantichub.Schema{
			{Name: "e2e-erasure-shape-1785499278293", Version: 1},
			{Name: "e2e-payment-shape-1785499379922", Version: 1},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"/semantic/shapes/facis-dcs?version=1",
		"/semantic/shapes/clause-catalog?version=1",
	}, refs.Shapes)
}

func TestEffectiveBundleRefsRejectIncompleteVersions(t *testing.T) {
	_, err := effectiveBundleRefs(semantichub.EffectiveBundle{
		ContextVersion: 1,
		ProfileVersion: 1,
		Shapes: []semantichub.Schema{
			{Name: semantichub.ShapesName, Version: 0},
		},
	})
	require.ErrorContains(t, err, "canonical shapes")
}
