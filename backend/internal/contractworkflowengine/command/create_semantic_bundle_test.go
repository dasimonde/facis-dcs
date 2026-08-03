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
			{Name: "customer-library", Version: 7},
		},
	}
	activated := semantichub.EffectiveBundle{
		ContextVersion: old.ContextVersion,
		ProfileVersion: 5,
		Shapes: []semantichub.Schema{
			{Name: semantichub.ShapesName, Version: 4},
			{Name: semantichub.ClauseCatalogName, Version: 5},
			{Name: "customer-library", Version: 8},
		},
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
		"/semantic/shapes/customer-library?version=8",
	}, newRefs.Shapes)
	require.Equal(t,
		"/semantic/profile/facis.sla.basic?version=5",
		newRefs.Profile,
	)
	require.NotEqual(t, oldRefs.CanonicalShapes, newRefs.CanonicalShapes)
	require.NotEqual(t, oldRefs.Shapes[2], newRefs.Shapes[2])
	require.NotEqual(t, oldRefs.Profile, newRefs.Profile)
	require.Equal(t, oldRefs, rollbackRefs)
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
