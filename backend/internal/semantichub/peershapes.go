package semantichub

import (
	"context"
	"errors"
	"fmt"

	"digital-contracting-service/internal/base/validation"

	"github.com/jmoiron/sqlx"
)

// PinnedShapesHub is the exact-version shapes access the DCS-to-DCS exchange
// needs of a hub, on both sides of the wire. The two namespaces are separate
// operations, never one lookup: this instance's own entries (kind="shapes")
// and the ones a counterparty shipped (kind="peer-shapes").
type PinnedShapesHub interface {
	LocalShapesVersion(ctx context.Context, name string, version int) (*Schema, error)
	PeerShapesVersion(ctx context.Context, name string, version int) (*Schema, error)
	StorePeerShapes(ctx context.Context, entry Schema) error
}

// DBPinnedShapes is the Postgres-backed PinnedShapesHub.
type DBPinnedShapes struct {
	DB *sqlx.DB
}

func (d DBPinnedShapes) LocalShapesVersion(ctx context.Context, name string, version int) (*Schema, error) {
	return d.versionOfKind(ctx, name, ShapesKind, version)
}

func (d DBPinnedShapes) PeerShapesVersion(ctx context.Context, name string, version int) (*Schema, error) {
	return d.versionOfKind(ctx, name, PeerShapesKind, version)
}

func (d DBPinnedShapes) versionOfKind(ctx context.Context, name, kind string, version int) (*Schema, error) {
	tx, err := d.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	schema, err := (Repo{}).Get(ctx, tx, name, kind, version)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return schema, nil
}

// StorePeerShapes writes a shipped graph into the peer namespace at exactly the
// version its pin names. Never active: Register hardcodes active=FALSE on
// insert and the admin activate/rollback endpoints do not accept this kind, so
// no peer-written row can become what this instance validates its own new
// documents against.
func (d DBPinnedShapes) StorePeerShapes(ctx context.Context, entry Schema) error {
	tx, err := d.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := (Repo{}).Register(ctx, tx, entry.Name, PeerShapesKind, entry.MediaType, entry.Content, entry.CreatedBy, entry.Version, false); err != nil {
		return err
	}
	return tx.Commit()
}

// CollectPinnedShapes reads the exact graphs a set of pinned references names,
// so they can travel with the artifact that pins them. Envelope entries are
// skipped: every deployment holds its own and judges the envelope by it.
//
// A library is read from this instance's own entries or, failing that, from
// what an earlier ship installed — a contract received from one peer and passed
// on to another carries a pin whose graph this instance only ever held as peer
// evidence. A reference neither namespace resolves is an error naming it: the
// artifact is already unevaluable here, and shipping it would only move the
// failure.
func CollectPinnedShapes(ctx context.Context, hub PinnedShapesHub, refs []validation.VersionedShapeRef) ([]Schema, error) {
	entries := make([]Schema, 0, len(refs))
	for _, ref := range refs {
		if IsEnvelopeShapes(ref.Name) {
			continue
		}
		schema, err := hub.LocalShapesVersion(ctx, ref.Name, ref.Version)
		if errors.Is(err, ErrSchemaNotFound) {
			schema, err = hub.PeerShapesVersion(ctx, ref.Name, ref.Version)
		}
		if err != nil {
			return nil, fmt.Errorf("semantic hub: pinned shapes %s v%d: %w", ref.Name, ref.Version, err)
		}
		entries = append(entries, Schema{
			Name:      schema.Name,
			Version:   schema.Version,
			Kind:      ShapesKind,
			MediaType: schema.MediaType,
			Content:   schema.Content,
		})
	}
	return entries, nil
}

// PlanPinnedShapesImport decides which of the graphs a peer shipped this
// instance has to store, and refuses the exchange outright when the artifact's
// bundle cannot be assembled. It reads only, so a caller can settle the refusal
// before it changes any state and store the result once the rest of the
// exchange has been accepted.
//
// Four conditions refuse, each naming the entry: a shipped entry the artifact
// does not pin, a shipped entry under an envelope name (those are this
// instance's own invariants, never a peer's to supply), shipped content
// differing from what this hub already holds at that version, and a pinned
// library neither shipped nor already held.
func PlanPinnedShapesImport(ctx context.Context, hub PinnedShapesHub, createdBy string, pinned []validation.VersionedShapeRef, shipped []Schema) ([]Schema, error) {
	pinnedRefs := map[validation.VersionedShapeRef]bool{}
	for _, ref := range pinned {
		pinnedRefs[ref] = true
	}
	shippedByRef := map[validation.VersionedShapeRef]Schema{}
	// Iterated in wire order, so which entry a refusal names is the first
	// offending one rather than whichever a map hands back.
	for _, entry := range shipped {
		ref := validation.VersionedShapeRef{Name: entry.Name, Version: entry.Version}
		if IsEnvelopeShapes(ref.Name) {
			return nil, fmt.Errorf(
				"semantic hub: shipped shapes %s v%d name this instance's own envelope vocabulary, which no peer supplies",
				ref.Name, ref.Version)
		}
		if !pinnedRefs[ref] {
			return nil, fmt.Errorf("semantic hub: shipped shapes %s v%d are not pinned by the contract they accompany", ref.Name, ref.Version)
		}
		shippedByRef[ref] = entry
	}

	var install []Schema
	for _, ref := range pinned {
		if IsEnvelopeShapes(ref.Name) {
			continue
		}
		held, err := heldPinnedShapes(ctx, hub, ref)
		if err != nil {
			return nil, err
		}
		entry, carried := shippedByRef[ref]
		if held != nil {
			if carried && entry.Content != held.Content {
				return nil, fmt.Errorf(
					"semantic hub: shipped shapes %s v%d differ from the version this hub already holds under that name",
					ref.Name, ref.Version)
			}
			continue
		}
		if !carried {
			return nil, fmt.Errorf("%w: pinned shapes %s v%d were neither shipped with the contract nor registered here",
				ErrSchemaNotFound, ref.Name, ref.Version)
		}
		entry.Kind = PeerShapesKind
		entry.CreatedBy = createdBy
		install = append(install, entry)
	}
	return install, nil
}

// heldPinnedShapes returns the graph this hub already holds at a pinned
// reference, from either namespace, or nil when it holds none.
func heldPinnedShapes(ctx context.Context, hub PinnedShapesHub, ref validation.VersionedShapeRef) (*Schema, error) {
	held, err := hub.LocalShapesVersion(ctx, ref.Name, ref.Version)
	if err == nil {
		return held, nil
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		return nil, fmt.Errorf("semantic hub: look up pinned shapes %s v%d: %w", ref.Name, ref.Version, err)
	}
	held, err = hub.PeerShapesVersion(ctx, ref.Name, ref.Version)
	if err == nil {
		return held, nil
	}
	if !errors.Is(err, ErrSchemaNotFound) {
		return nil, fmt.Errorf("semantic hub: look up pinned shapes %s v%d: %w", ref.Name, ref.Version, err)
	}
	return nil, nil
}

// StorePinnedShapes installs a plan PlanPinnedShapesImport produced.
func StorePinnedShapes(ctx context.Context, hub PinnedShapesHub, install []Schema) error {
	for _, entry := range install {
		if err := hub.StorePeerShapes(ctx, entry); err != nil {
			return fmt.Errorf("semantic hub: import pinned shapes %s v%d: %w", entry.Name, entry.Version, err)
		}
	}
	return nil
}
