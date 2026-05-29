package fcschemas

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	schemacmd "digital-contracting-service/internal/templatecatalogueintegration/command/schema"
	schemaquery "digital-contracting-service/internal/templatecatalogueintegration/query/schema"
)

type remoteSchema struct {
	id      string
	content []byte
}

// Sync aligns embedded schemas with Federated Catalogue.
// Unknown remote shapes are ignored. Schemas are never deleted from FC.
func Sync(ctx context.Context, fc *fcclient.FederatedCatalogueClient) error {
	if fc == nil {
		return fcclient.ErrFederatedCatalogueNotConfigured
	}

	bundles, err := LoadBundles()
	if err != nil {
		return err
	}

	remote, err := indexRemoteSchemas(ctx, fc, bundles)
	if err != nil {
		return err
	}

	createHandler := schemacmd.CreateHandler{Ctx: ctx, FCClient: fc}
	updateHandler := schemacmd.UpdateHandler{Ctx: ctx, FCClient: fc}

	for _, bundle := range bundles {
		if err := syncBundle(ctx, createHandler, updateHandler, bundle, bundles, fc, remote); err != nil {
			return fmt.Errorf("schema %s v%d: %w", bundle.Type, bundle.Version, err)
		}
	}

	return nil
}

// syncBundle ensures that the given bundle is present in FC with the same content, creating or updating as needed.
func syncBundle(
	ctx context.Context,
	createHandler schemacmd.CreateHandler,
	updateHandler schemacmd.UpdateHandler,
	bundle Bundle,
	all []Bundle,
	fc *fcclient.FederatedCatalogueClient,
	remote map[string]map[int]remoteSchema,
) error {
	byVersion := remote[bundle.Type]
	current, ok := byVersion[bundle.Version]
	if ok && bytes.Equal(current.content, bundle.Content) {
		log.Printf("fc schema %s v%d: up to date (%s)", bundle.Type, bundle.Version, bundle.File)
		return nil
	}
	if ok {
		log.Printf("fc schema %s v%d: updating %s", bundle.Type, bundle.Version, current.id)
		return updateHandler.Handle(ctx, schemacmd.UpdateCmd{ID: current.id, Content: bundle.Content})
	}

	log.Printf("fc schema %s v%d: creating from %s", bundle.Type, bundle.Version, bundle.File)
	if err := createHandler.Handle(ctx, schemacmd.CreateCmd{Content: bundle.Content}); err != nil {
		if !isCreateConflict(err) {
			return err
		}
		log.Printf("fc schema %s v%d: create conflict, resolving via update", bundle.Type, bundle.Version)
		remote, scanErr := indexRemoteSchemas(ctx, fc, all)
		if scanErr != nil {
			return scanErr
		}
		found, ok := remote[bundle.Type][bundle.Version]
		if !ok {
			return fmt.Errorf("create conflict but no remote schema matched %s v%d: %w", bundle.Type, bundle.Version, err)
		}
		return updateHandler.Handle(ctx, schemacmd.UpdateCmd{ID: found.id, Content: bundle.Content})
	}
	return nil
}

// indexRemoteSchemas loads all remote schemas and matches them to the given bundles by content.
func indexRemoteSchemas(ctx context.Context, fc *fcclient.FederatedCatalogueClient, bundles []Bundle) (map[string]map[int]remoteSchema, error) {
	listHandler := schemaquery.ListShapeIDsHandler{Ctx: ctx, FCClient: fc}
	getHandler := schemaquery.GetContentHandler{Ctx: ctx, FCClient: fc}

	shapeIDs, err := listHandler.Handle(schemaquery.ListShapeIDsQry{})
	if err != nil {
		return nil, err
	}

	remote := make(map[string]map[int]remoteSchema)
	for _, id := range shapeIDs {
		body, err := getHandler.Handle(schemaquery.GetContentQry{ID: id})
		if err != nil {
			return nil, err
		}

		var matched *Bundle
		for i := range bundles {
			b := &bundles[i]
			if !b.MatchesRemote(body) {
				continue
			}
			if matched != nil && (matched.Type != b.Type || matched.Version != b.Version) {
				return nil, fmt.Errorf("remote schema %s matches multiple DCS schemas (%s v%d and %s v%d)",
					id, matched.Type, matched.Version, b.Type, b.Version)
			}
			matched = b
		}
		if matched == nil {
			continue
		}

		byVersion := remote[matched.Type]
		if byVersion == nil {
			byVersion = make(map[int]remoteSchema)
			remote[matched.Type] = byVersion
		}
		if existing, ok := byVersion[matched.Version]; ok && existing.id != id {
			return nil, fmt.Errorf("multiple remote schemas for %s v%d (%s and %s)",
				matched.Type, matched.Version, existing.id, id)
		}
		byVersion[matched.Version] = remoteSchema{id: id, content: body}
	}
	return remote, nil
}

// isCreateConflict determines whether the given error indicates a create conflict, which can be resolved by an update.
func isCreateConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Schema redefines existing terms")
}
