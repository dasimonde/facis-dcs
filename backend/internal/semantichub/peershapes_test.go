package semantichub

import (
	"context"
	"fmt"
	"testing"

	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

// fakeHub stands in for one instance's semantic_schemas table, keeping the two
// namespaces the exchange touches apart exactly as the kind column does.
type fakeHub struct {
	local  map[string]Schema
	peer   map[string]Schema
	stored []Schema
}

func newFakeHub(entries ...Schema) *fakeHub {
	hub := &fakeHub{local: map[string]Schema{}, peer: map[string]Schema{}}
	for _, entry := range entries {
		hub.local[shapesKey(entry.Name, entry.Version)] = entry
	}
	return hub
}

func (f *fakeHub) withPeerEntry(entry Schema) *fakeHub {
	f.peer[shapesKey(entry.Name, entry.Version)] = entry
	return f
}

func shapesKey(name string, version int) string {
	return fmt.Sprintf("%s\x00%d", name, version)
}

func (f *fakeHub) LocalShapesVersion(_ context.Context, name string, version int) (*Schema, error) {
	return lookupShapes(f.local, name, version)
}

func (f *fakeHub) PeerShapesVersion(_ context.Context, name string, version int) (*Schema, error) {
	return lookupShapes(f.peer, name, version)
}

func lookupShapes(rows map[string]Schema, name string, version int) (*Schema, error) {
	entry, ok := rows[shapesKey(name, version)]
	if !ok {
		return nil, fmt.Errorf("%w: %s/shapes", ErrSchemaNotFound, name)
	}
	return &entry, nil
}

func (f *fakeHub) StorePeerShapes(_ context.Context, entry Schema) error {
	if _, taken := f.peer[shapesKey(entry.Name, entry.Version)]; taken {
		return fmt.Errorf("%w: %s/%s version %d", ErrVersionTaken, entry.Name, PeerShapesKind, entry.Version)
	}
	f.peer[shapesKey(entry.Name, entry.Version)] = entry
	f.stored = append(f.stored, entry)
	return nil
}

func ref(name string, version int) validation.VersionedShapeRef {
	return validation.VersionedShapeRef{Name: name, Version: version}
}

func importPinned(t *testing.T, hub *fakeHub, pinned []validation.VersionedShapeRef, shipped []Schema) error {
	t.Helper()
	install, err := PlanPinnedShapesImport(context.Background(), hub, "peer:did:web:dcs-a.test", pinned, shipped)
	if err != nil {
		return err
	}
	return StorePinnedShapes(context.Background(), hub, install)
}

// A shape library published on the authoring instance alone exists nowhere
// else, so the contract pinned to it can only be evaluated on the counterparty
// once the graph travels with it — at the version the pin names, into the peer
// namespace, and without becoming part of the receiving hub's own vocabulary.
func TestImportPinnedShapesInstallsAShippedLibraryAtItsPinnedVersion(t *testing.T) {
	hub := newFakeHub(Schema{Name: "facis-dcs", Version: 1, Kind: ShapesKind, Content: "canonical"})

	require.NoError(t, importPinned(t, hub,
		[]validation.VersionedShapeRef{ref("facis-dcs", 1), ref("e2e-erasure-shape", 3)},
		[]Schema{{Name: "e2e-erasure-shape", Version: 3, MediaType: "text/turtle", Content: "erasure shapes"}}))

	require.Len(t, hub.stored, 1)
	imported := hub.stored[0]
	require.Equal(t, "e2e-erasure-shape", imported.Name)
	require.Equal(t, 3, imported.Version, "the entry must land at the version the pin names, not this hub's next number")
	require.Equal(t, PeerShapesKind, imported.Kind, "a peer's graph never lands in the namespace this instance publishes from")
	require.Equal(t, "text/turtle", imported.MediaType)
	require.Equal(t, "erasure shapes", imported.Content)
	require.Equal(t, "peer:did:web:dcs-a.test", imported.CreatedBy)

	_, err := hub.LocalShapesVersion(context.Background(), "e2e-erasure-shape", 3)
	require.ErrorIs(t, err, ErrSchemaNotFound, "the import must not be visible as an entry of this instance's own")
	resolved, err := hub.PeerShapesVersion(context.Background(), "e2e-erasure-shape", 3)
	require.NoError(t, err)
	require.Equal(t, "erasure shapes", resolved.Content)
}

// BLOCKING (HIGH): facis-dcs and clause-catalog are this instance's own
// envelope invariants and the graphs RequireHubConformance blocks on at offer,
// submit and signing. A peer naming one of them in a ship is trying to write
// what our own gates apply, and the version number it names is one we may
// simply not hold — so the refusal cannot depend on a content comparison.
func TestImportPinnedShapesRefusesAPeerSupplyingTheEnvelopeVocabulary(t *testing.T) {
	for _, name := range []string{ShapesName, ClauseCatalogName} {
		t.Run(name, func(t *testing.T) {
			hub := newFakeHub(Schema{Name: name, Version: 1, Kind: ShapesKind, Content: "this instance's envelope"})

			err := importPinned(t, hub,
				[]validation.VersionedShapeRef{ref(name, 9)},
				[]Schema{{Name: name, Version: 9, MediaType: "text/turtle", Content: "the peer's envelope"}})

			require.Error(t, err)
			require.Contains(t, err.Error(), fmt.Sprintf("%s v9", name))
			require.Contains(t, err.Error(), "envelope")
			require.Empty(t, hub.stored, "no peer-supplied row may exist under an envelope name")
			held, err := hub.LocalShapesVersion(context.Background(), name, 1)
			require.NoError(t, err)
			require.Equal(t, "this instance's envelope", held.Content)
		})
	}
}

// BLOCKING (MEDIUM): two deployments built from different image digests hold
// different genesis content at the same version number. The envelope is not
// peer evidence — each instance judges it by its own graph — so a pin naming it
// is neither carried nor compared, and every ship between such a pair still
// goes through: offers, counter-offers, the signed agreement, the REVOKED ship
// DCS-NFR-BR-06 requires the receiver to adopt, and the wrapped CEK.
func TestImportPinnedShapesAcceptsAShipFromAnInstanceWithDifferentGenesisContent(t *testing.T) {
	hub := newFakeHub(
		Schema{Name: ShapesName, Version: 1, Kind: ShapesKind, Content: "this deployment's genesis shapes"},
		Schema{Name: ClauseCatalogName, Version: 1, Kind: ShapesKind, Content: "this deployment's genesis catalog"},
	)

	require.NoError(t, importPinned(t, hub,
		[]validation.VersionedShapeRef{ref(ShapesName, 1), ref(ClauseCatalogName, 1)},
		nil))
	require.Empty(t, hub.stored)

	// The same holds when the peer's envelope sits at a version this hub never
	// assigned, which is what an asset drift on one side alone produces.
	require.NoError(t, importPinned(t, hub,
		[]validation.VersionedShapeRef{ref(ShapesName, 4), ref(ClauseCatalogName, 2)},
		nil))
	require.Empty(t, hub.stored)
}

// Fail-closed: an unresolvable library pin must block, never fall back to the
// receiving hub's own shapes — two instances would otherwise reach different
// verdicts on the same document, which is what pinning exists to prevent.
func TestImportPinnedShapesRefusesAPinItCanNeitherResolveNorReceive(t *testing.T) {
	hub := newFakeHub(Schema{Name: "facis-dcs", Version: 1, Kind: ShapesKind, Content: "canonical"})

	err := importPinned(t, hub,
		[]validation.VersionedShapeRef{ref("facis-dcs", 1), ref("e2e-payment-shape", 2)},
		nil)

	require.ErrorIs(t, err, ErrSchemaNotFound)
	require.Contains(t, err.Error(), "e2e-payment-shape v2", "the refusal must name the bundle entry that could not be assembled")
	require.Empty(t, hub.stored)
}

// Hub versions are immutable: if this instance already published a library at
// the pinned version, the pin names two graphs and the exchange is refused
// rather than validated against either. Unlike the envelope this is an
// operator-created name collision, and republishing under a free version
// resolves it.
func TestImportPinnedShapesRefusesContentThatContradictsTheHeldVersion(t *testing.T) {
	hub := newFakeHub(Schema{Name: "partner-library", Version: 4, Kind: ShapesKind, Content: "this hub's graph"})

	err := importPinned(t, hub,
		[]validation.VersionedShapeRef{ref("partner-library", 4)},
		[]Schema{{Name: "partner-library", Version: 4, MediaType: "text/turtle", Content: "the peer's graph"}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "partner-library v4")
	require.Contains(t, err.Error(), "already holds")
	require.Empty(t, hub.stored, "a contradicted version is never overwritten")
	held, err := hub.LocalShapesVersion(context.Background(), "partner-library", 4)
	require.NoError(t, err)
	require.Equal(t, "this hub's graph", held.Content)
}

// A repeated ship carries the identical bundle, so the import has to absorb it
// — every negotiation round re-ships the same libraries.
func TestImportPinnedShapesIsIdempotentForAnAlreadyImportedLibrary(t *testing.T) {
	hub := newFakeHub().withPeerEntry(Schema{Name: "partner-library", Version: 4, Kind: PeerShapesKind, Content: "the peer's graph"})

	require.NoError(t, importPinned(t, hub,
		[]validation.VersionedShapeRef{ref("partner-library", 4)},
		[]Schema{{Name: "partner-library", Version: 4, MediaType: "text/turtle", Content: "the peer's graph"}}))
	require.Empty(t, hub.stored)
}

// The ship may install exactly what the contract it accompanies pins, and
// nothing else: a peer has no standing to push vocabulary into this hub under
// cover of a contract exchange. The refusal names the first offending entry in
// wire order, so the message does not depend on map iteration.
func TestImportPinnedShapesRefusesAnEntryTheContractDoesNotPin(t *testing.T) {
	hub := newFakeHub()

	for i := 0; i < 8; i++ {
		err := importPinned(t, hub, nil, []Schema{
			{Name: "unsolicited-a", Version: 1, MediaType: "text/turtle", Content: "shapes"},
			{Name: "unsolicited-b", Version: 1, MediaType: "text/turtle", Content: "shapes"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsolicited-a v1")
		require.Empty(t, hub.stored)
	}
}

func TestCollectPinnedShapesShipsTheLibrariesAndLeavesTheEnvelopeBehind(t *testing.T) {
	hub := newFakeHub(
		Schema{Name: "facis-dcs", Version: 2, Kind: ShapesKind, MediaType: "text/turtle", Content: "canonical"},
		Schema{Name: "clause-catalog", Version: 2, Kind: ShapesKind, MediaType: "text/turtle", Content: "catalog"},
		Schema{Name: "e2e-payment-shape", Version: 1, Kind: ShapesKind, MediaType: "text/turtle", Content: "payment shapes"},
	)

	entries, err := CollectPinnedShapes(context.Background(), hub,
		[]validation.VersionedShapeRef{ref("facis-dcs", 2), ref("clause-catalog", 2), ref("e2e-payment-shape", 1)})
	require.NoError(t, err)
	require.Equal(t, []Schema{
		{Name: "e2e-payment-shape", Version: 1, Kind: ShapesKind, MediaType: "text/turtle", Content: "payment shapes"},
	}, entries)
}

// A contract received from one peer and passed on to another carries a pin
// whose graph this instance only ever held as peer evidence.
func TestCollectPinnedShapesReadsALibraryAnEarlierShipInstalled(t *testing.T) {
	hub := newFakeHub().withPeerEntry(
		Schema{Name: "partner-library", Version: 3, Kind: PeerShapesKind, MediaType: "text/turtle", Content: "the first peer's graph"})

	entries, err := CollectPinnedShapes(context.Background(), hub, []validation.VersionedShapeRef{ref("partner-library", 3)})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "the first peer's graph", entries[0].Content)
}

// The pin is written from this hub's own inventory at creation, so a miss here
// means the contract is already unevaluable on the instance that authored it —
// shipping it without the bundle would only move the failure to the peer.
func TestCollectPinnedShapesFailsOnAPinTheShippingHubCannotResolve(t *testing.T) {
	hub := newFakeHub(Schema{Name: "facis-dcs", Version: 2, Kind: ShapesKind, Content: "canonical"})

	_, err := CollectPinnedShapes(context.Background(), hub,
		[]validation.VersionedShapeRef{ref("facis-dcs", 2), ref("gone", 7)})
	require.ErrorIs(t, err, ErrSchemaNotFound)
	require.Contains(t, err.Error(), "gone v7")
}
