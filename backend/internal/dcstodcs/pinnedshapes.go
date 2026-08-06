package dcstodcs

import (
	"context"
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

// PinnedShapesForDocument reads the shapes graphs a contract document pins in
// dcs:effectiveShapes, so they travel with the ship. Envelope graphs stay
// behind — the receiver holds its own.
func PinnedShapesForDocument(ctx context.Context, hub semantichub.PinnedShapesHub, document []byte) ([]semantichub.Schema, error) {
	refs, err := pinnedShapeRefs(document)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}
	return semantichub.CollectPinnedShapes(ctx, hub, refs)
}

// PlanPinnedShapes checks that the bundle a shipped contract pins can be
// assembled here and returns the entries to install. Read-only: the caller
// refuses the exchange on error before anything is stored.
func PlanPinnedShapes(ctx context.Context, hub semantichub.PinnedShapesHub, peerDID string, document []byte, wire []*dcstodcs.DCSToDCSPinnedShapes) ([]semantichub.Schema, error) {
	refs, err := pinnedShapeRefs(document)
	if err != nil {
		return nil, err
	}
	return semantichub.PlanPinnedShapesImport(ctx, hub, "peer:"+peerDID, refs, ReceivedPinnedShapes(wire))
}

// InstallPinnedShapes writes a plan into the hub's peer namespace.
func InstallPinnedShapes(ctx context.Context, hub semantichub.PinnedShapesHub, install []semantichub.Schema) error {
	return semantichub.StorePinnedShapes(ctx, hub, install)
}

func pinnedShapeRefs(document []byte) ([]validation.VersionedShapeRef, error) {
	var content map[string]any
	if err := json.Unmarshal(document, &content); err != nil {
		return nil, fmt.Errorf("decode contract document for its pinned shapes: %w", err)
	}
	refs, err := validation.EffectiveShapeRefs(content)
	if err != nil {
		return nil, fmt.Errorf("read pinned shapes of contract document: %w", err)
	}
	return refs, nil
}

// WirePinnedShapes converts hub entries to the ship payload.
func WirePinnedShapes(entries []semantichub.Schema) []*dcstodcs.DCSToDCSPinnedShapes {
	if len(entries) == 0 {
		return nil
	}
	wire := make([]*dcstodcs.DCSToDCSPinnedShapes, 0, len(entries))
	for _, entry := range entries {
		wire = append(wire, &dcstodcs.DCSToDCSPinnedShapes{
			Name:      entry.Name,
			Version:   entry.Version,
			MediaType: entry.MediaType,
			Content:   entry.Content,
		})
	}
	return wire
}

// ReceivedPinnedShapes converts a ship payload back to hub entries.
func ReceivedPinnedShapes(wire []*dcstodcs.DCSToDCSPinnedShapes) []semantichub.Schema {
	if len(wire) == 0 {
		return nil
	}
	entries := make([]semantichub.Schema, 0, len(wire))
	for _, entry := range wire {
		if entry == nil {
			continue
		}
		entries = append(entries, semantichub.Schema{
			Name:      entry.Name,
			Version:   entry.Version,
			MediaType: entry.MediaType,
			Content:   entry.Content,
		})
	}
	return entries
}
