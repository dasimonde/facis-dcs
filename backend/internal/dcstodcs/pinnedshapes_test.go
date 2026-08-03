package dcstodcs

import (
	"context"
	"fmt"
	"testing"

	"digital-contracting-service/internal/semantichub"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"

	"github.com/stretchr/testify/require"
)

// contractPinnedTo is a shipped contract document reduced to the property the
// exchange reads: the immutable bundle its creation stamped on it.
func contractPinnedTo(anchors ...string) []byte {
	refs := ""
	for i, anchor := range anchors {
		if i > 0 {
			refs += ","
		}
		refs += fmt.Sprintf(`{"@id":%q}`, anchor)
	}
	return []byte(fmt.Sprintf(`{"@id":"urn:contract","dcs:effectiveShapes":[%s]}`, refs))
}

// fakeHub keeps this instance's own entries and the ones a peer shipped apart,
// exactly as the kind column does.
type fakeHub struct {
	local  map[string]semantichub.Schema
	peer   map[string]semantichub.Schema
	stored []semantichub.Schema
}

func newFakeHub(entries ...semantichub.Schema) *fakeHub {
	hub := &fakeHub{local: map[string]semantichub.Schema{}, peer: map[string]semantichub.Schema{}}
	for _, entry := range entries {
		hub.local[fmt.Sprintf("%s\x00%d", entry.Name, entry.Version)] = entry
	}
	return hub
}

func (f *fakeHub) LocalShapesVersion(_ context.Context, name string, version int) (*semantichub.Schema, error) {
	return lookupShapes(f.local, name, version)
}

func (f *fakeHub) PeerShapesVersion(_ context.Context, name string, version int) (*semantichub.Schema, error) {
	return lookupShapes(f.peer, name, version)
}

func lookupShapes(rows map[string]semantichub.Schema, name string, version int) (*semantichub.Schema, error) {
	entry, ok := rows[fmt.Sprintf("%s\x00%d", name, version)]
	if !ok {
		return nil, fmt.Errorf("%w: %s/shapes", semantichub.ErrSchemaNotFound, name)
	}
	return &entry, nil
}

func (f *fakeHub) StorePeerShapes(_ context.Context, entry semantichub.Schema) error {
	f.peer[fmt.Sprintf("%s\x00%d", entry.Name, entry.Version)] = entry
	f.stored = append(f.stored, entry)
	return nil
}

func acceptPinnedShapes(t *testing.T, hub *fakeHub, peerDID string, document []byte, wire []*dcstodcs.DCSToDCSPinnedShapes) error {
	t.Helper()
	install, err := PlanPinnedShapes(context.Background(), hub, peerDID, document, wire)
	if err != nil {
		return err
	}
	return InstallPinnedShapes(context.Background(), hub, install)
}

// The full round trip the vertical needs: the authoring instance reads the
// libraries its contract pins, they travel on the ship, and the counterparty
// ends up able to resolve the same pin. The envelope stays on both sides.
func TestPinnedShapesTravelWithTheContractThatPinsThem(t *testing.T) {
	author := newFakeHub(
		semantichub.Schema{Name: "facis-dcs", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "canonical", Active: true},
		semantichub.Schema{Name: "e2e-payment-shape", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "payment shapes", Active: true},
	)
	document := contractPinnedTo(
		"https://dcs-a.test/semantic/shapes/facis-dcs?version=1",
		"https://dcs-a.test/semantic/shapes/e2e-payment-shape?version=1",
	)

	entries, err := PinnedShapesForDocument(context.Background(), author, document)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the envelope is not the peer's to supply, so only the library travels")
	require.Equal(t, "e2e-payment-shape", entries[0].Name)

	peer := newFakeHub(semantichub.Schema{Name: "facis-dcs", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "canonical", Active: true})
	require.NoError(t, acceptPinnedShapes(t, peer, "did:web:dcs-a.test", document, WirePinnedShapes(entries)))

	require.Len(t, peer.stored, 1)
	require.Equal(t, "e2e-payment-shape", peer.stored[0].Name)
	require.Equal(t, semantichub.PeerShapesKind, peer.stored[0].Kind)
	require.Equal(t, "peer:did:web:dcs-a.test", peer.stored[0].CreatedBy)
}

// BLOCKING (MEDIUM): two deployments built from different image digests seed
// different genesis content at version 1. The ship must still go through — the
// alternative is that no offer, counter-offer, signature or DCS-NFR-BR-06
// revocation can ever cross between them, with no repair path in the hub API.
func TestShipSucceedsBetweenInstancesWithDifferentGenesisContentAtTheSameVersion(t *testing.T) {
	author := newFakeHub(
		semantichub.Schema{Name: "facis-dcs", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "A's genesis envelope"},
		semantichub.Schema{Name: "clause-catalog", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "A's genesis catalog"},
	)
	document := contractPinnedTo(
		"https://dcs-a.test/semantic/shapes/facis-dcs?version=1",
		"https://dcs-a.test/semantic/shapes/clause-catalog?version=1",
	)
	entries, err := PinnedShapesForDocument(context.Background(), author, document)
	require.NoError(t, err)
	require.Empty(t, entries)

	receiver := newFakeHub(
		semantichub.Schema{Name: "facis-dcs", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "B's genesis envelope, built from another digest"},
		semantichub.Schema{Name: "clause-catalog", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "B's genesis catalog, built from another digest"},
	)
	require.NoError(t, acceptPinnedShapes(t, receiver, "did:web:dcs-a.test", document, WirePinnedShapes(entries)))
	require.Empty(t, receiver.stored)
}

// BLOCKING (HIGH): the envelope graphs are what RequireHubConformance blocks
// on at offer, submit and signing. A peer shipping content under one of their
// names is refused, so no row a peer wrote is ever reachable from those names.
func TestAcceptPinnedShapesRefusesAPeerShippingTheEnvelopeVocabulary(t *testing.T) {
	receiver := newFakeHub(
		semantichub.Schema{Name: "facis-dcs", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "this instance's envelope"})
	document := contractPinnedTo("https://dcs-a.test/semantic/shapes/facis-dcs?version=7")

	err := acceptPinnedShapes(t, receiver, "did:web:dcs-a.test", document,
		[]*dcstodcs.DCSToDCSPinnedShapes{{Name: "facis-dcs", Version: 7, MediaType: "text/turtle", Content: "the peer's envelope"}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "facis-dcs v7")
	require.Empty(t, receiver.stored)
	require.Empty(t, receiver.peer)
}

// The defect this closes, stated as the invariant it restores: a ship that does
// not carry a pin the receiver cannot resolve is refused at the boundary, so no
// contract is ever stored in a state where every workflow transition blocks.
func TestAcceptPinnedShapesRefusesAShipMissingAPinTheReceiverLacks(t *testing.T) {
	peer := newFakeHub(semantichub.Schema{Name: "facis-dcs", Version: 1, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "canonical"})
	document := contractPinnedTo(
		"https://dcs-a.test/semantic/shapes/facis-dcs?version=1",
		"https://dcs-a.test/semantic/shapes/e2e-erasure-shape-1785499278293?version=1",
	)

	err := acceptPinnedShapes(t, peer, "did:web:dcs-a.test", document, nil)
	require.ErrorIs(t, err, semantichub.ErrSchemaNotFound)
	require.Contains(t, err.Error(), "e2e-erasure-shape-1785499278293 v1")
}

func TestAcceptPinnedShapesRefusesShippedContentContradictingAHeldVersion(t *testing.T) {
	peer := newFakeHub(semantichub.Schema{Name: "partner-library", Version: 2, Kind: semantichub.ShapesKind, MediaType: "text/turtle", Content: "this hub's graph"})
	document := contractPinnedTo("https://dcs-a.test/semantic/shapes/partner-library?version=2")

	err := acceptPinnedShapes(t, peer, "did:web:dcs-a.test", document,
		[]*dcstodcs.DCSToDCSPinnedShapes{{Name: "partner-library", Version: 2, MediaType: "text/turtle", Content: "the peer's graph"}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "partner-library v2")
	require.Empty(t, peer.stored)
}

// A contract predating the pin carries no bundle; the gate refuses it on its
// own terms, and the exchange has nothing to install.
func TestAcceptPinnedShapesIsANoOpForADocumentWithoutAPin(t *testing.T) {
	peer := newFakeHub()
	require.NoError(t, acceptPinnedShapes(t, peer, "did:web:dcs-a.test", []byte(`{"@id":"urn:contract"}`), nil))
	require.Empty(t, peer.stored)
}
