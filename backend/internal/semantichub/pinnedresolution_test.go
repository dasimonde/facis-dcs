package semantichub

import (
	"context"
	"fmt"
	"testing"

	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

// fakeRows stands in for semantic_schemas, keyed the way the table is: name,
// kind and version together.
type fakeRows struct {
	rows           map[string]string
	activeVersions map[string]int
}

func newFakeRows() *fakeRows {
	return &fakeRows{rows: map[string]string{}, activeVersions: map[string]int{}}
}

func (f *fakeRows) put(name, kind string, version int, content string) *fakeRows {
	f.rows[fmt.Sprintf("%s\x00%s\x00%d", name, kind, version)] = content
	return f
}

func (f *fakeRows) activate(name, kind string, version int) *fakeRows {
	f.activeVersions[name+"\x00"+kind] = version
	return f
}

func (f *fakeRows) versionContent(_ context.Context, name, kind string, version int) (string, error) {
	content, ok := f.rows[fmt.Sprintf("%s\x00%s\x00%d", name, kind, version)]
	if !ok {
		return "", fmt.Errorf("%w: %s/%s", ErrSchemaNotFound, name, kind)
	}
	return content, nil
}

func (f *fakeRows) active(ctx context.Context, name, kind string) (string, int, error) {
	version, ok := f.activeVersions[name+"\x00"+kind]
	if !ok {
		return "", 0, fmt.Errorf("%w: %s/%s", ErrSchemaNotFound, name, kind)
	}
	content, err := f.versionContent(ctx, name, kind, version)
	return content, version, err
}

// BLOCKING (HIGH): validateAgainstShapeSource resolves the pinned bundle, and
// RequireHubConformance blocks offer, submit and signing on the verdict. A row
// a peer wrote must be unreachable from an envelope name however the peer got
// it stored, or the counterparty chooses the SHACL our own gates apply.
func TestPinnedEnvelopeShapesNeverResolveToAPeerWrittenRow(t *testing.T) {
	rows := newFakeRows().
		put(ShapesName, ShapesKind, 1, "this instance's envelope").
		activate(ShapesName, ShapesKind, 1).
		put(ShapesName, PeerShapesKind, 9, "a peer's envelope")

	content, err := pinnedShapesContent(context.Background(), rows, validation.VersionedShapeRef{Name: ShapesName, Version: 9})
	require.NoError(t, err)
	require.Equal(t, "this instance's envelope", content)
}

// BLOCKING (MEDIUM): two deployments seed their own genesis, so a version
// number the pin names may be one this hub never assigned. Resolving to the
// active envelope graph is what keeps the pair able to evaluate each other's
// contracts at all; failing here would block every transition on every
// federated copy.
func TestPinnedEnvelopeShapesFallBackToTheActiveGraphOnAVersionThisHubNeverAssigned(t *testing.T) {
	rows := newFakeRows().
		put(ClauseCatalogName, ShapesKind, 3, "this instance's catalog").
		activate(ClauseCatalogName, ShapesKind, 3)

	content, err := pinnedShapesContent(context.Background(), rows, validation.VersionedShapeRef{Name: ClauseCatalogName, Version: 1})
	require.NoError(t, err)
	require.Equal(t, "this instance's catalog", content)
}

// A library is the authoring instance's vocabulary: this hub's own entry
// answers where it published one, and the peer namespace a ship installed into
// answers otherwise.
func TestPinnedLibraryPrefersThisHubsEntryAndFallsBackToTheImportedOne(t *testing.T) {
	rows := newFakeRows().
		put("partner-library", ShapesKind, 2, "this hub's graph").
		put("partner-library", PeerShapesKind, 2, "a peer's graph").
		put("e2e-payment-shape", PeerShapesKind, 1, "the shipped graph")

	content, err := pinnedShapesContent(context.Background(), rows, validation.VersionedShapeRef{Name: "partner-library", Version: 2})
	require.NoError(t, err)
	require.Equal(t, "this hub's graph", content)

	content, err = pinnedShapesContent(context.Background(), rows, validation.VersionedShapeRef{Name: "e2e-payment-shape", Version: 1})
	require.NoError(t, err)
	require.Equal(t, "the shipped graph", content)
}

// A library pin neither namespace holds blocks: falling back to this hub's own
// shapes would let the two instances reach different verdicts on one document.
func TestPinnedLibraryThatResolvesNowhereFails(t *testing.T) {
	rows := newFakeRows().put(ShapesName, ShapesKind, 1, "envelope").activate(ShapesName, ShapesKind, 1)

	_, err := pinnedShapesContent(context.Background(), rows, validation.VersionedShapeRef{Name: "absent-library", Version: 4})
	require.ErrorIs(t, err, ErrSchemaNotFound)
}
