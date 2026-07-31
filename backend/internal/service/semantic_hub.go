package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	semantichubgen "digital-contracting-service/gen/semantic_hub"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/semantichub"

	"github.com/jmoiron/sqlx"
)

const maxSchemaFetchBytes = 8 << 20 // 8 MiB

// fetchSchemaFromURL retrieves a schema document over http(s), following
// redirects (Gaia-X and other canonical schemas sit behind w3id/purl
// redirects). It is register-only and RBAC-gated (Template Manager); it bounds
// the scheme, response size, and time. The caller snapshots the bytes as an
// immutable version.
func fetchSchemaFromURL(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid source_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("source_url must be http or https, got %q", parsed.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/turtle, application/ld+json, application/rdf+xml, application/json, */*")

	// http.Client follows up to 10 redirects by default.
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", parsed.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: HTTP %d", parsed.Redacted(), resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSchemaFetchBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", parsed.Redacted(), err)
	}
	if len(body) > maxSchemaFetchBytes {
		return "", fmt.Errorf("fetched schema exceeds %d bytes", maxSchemaFetchBytes)
	}
	if len(body) == 0 {
		return "", fmt.Errorf("fetched schema from %s is empty", parsed.Redacted())
	}
	return unwrapHubSchemaItem(parsed, body), nil
}

// unwrapHubSchemaItem returns the schema document a DCS hub anchor serves.
//
// The anchor a template carries in sh:shapesGraph
// (/semantic/shapes/{name}?version=N) dereferences to a SemanticSchemaItem
// envelope, not to raw Turtle — so installing a peer's library straight from
// the anchor the template declares would otherwise snapshot the JSON envelope
// as the shapes graph. Recognized only on a /semantic/ path and only when the
// body carries the envelope's full shape, so a schema document that happens
// to have a "content" key is left verbatim.
func unwrapHubSchemaItem(source *url.URL, body []byte) string {
	if !strings.Contains(source.Path, "/semantic/") {
		return string(body)
	}
	var item struct {
		Name    *string `json:"name"`
		Kind    *string `json:"kind"`
		Version *int    `json:"version"`
		Content *string `json:"content"`
	}
	if err := json.Unmarshal(body, &item); err != nil {
		return string(body)
	}
	if item.Name == nil || item.Kind == nil || item.Version == nil || item.Content == nil {
		return string(body)
	}
	if strings.TrimSpace(*item.Content) == "" {
		return string(body)
	}
	return *item.Content
}

// SemanticHub service implementation (DCS-FR-TR-03, UC-02-08).
type semanticHubsrvc struct {
	DB   *sqlx.DB
	Repo semantichub.Repo
	auth.JWTAuthenticator
}

// NewSemanticHub returns the SemanticHub service implementation.
func NewSemanticHub(db *sqlx.DB, jwtAuth auth.JWTAuthenticator) semantichubgen.Service {
	return &semanticHubsrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
	}
}

func (s *semanticHubsrvc) Register(ctx context.Context, p *semantichubgen.RegisterPayload) (res *semantichubgen.SemanticSchemaRegisterResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	content := ""
	if p.Content != nil {
		content = *p.Content
	}
	// A schema may be given inline or fetched from a URL (Gaia-X and other
	// canonical schemas live behind w3id/purl redirects). The fetched bytes are
	// snapshotted as the version — never a live reference — so version pinning
	// (DCS-FR-TR-03, ADR-8) still holds.
	if strings.TrimSpace(content) == "" && p.SourceURL != nil && strings.TrimSpace(*p.SourceURL) != "" {
		fetched, err := fetchSchemaFromURL(ctx, *p.SourceURL)
		if err != nil {
			return nil, semantichubgen.MakeBadRequest(err)
		}
		content = fetched
	}

	if p.Kind == "context" {
		// A context version must at least parse as a JSON-LD document with
		// an @context object — a broken active context would break every
		// subsequent document normalization.
		var doc struct {
			Context map[string]any `json:"@context"`
		}
		if err := json.Unmarshal([]byte(content), &doc); err != nil || len(doc.Context) == 0 {
			return nil, semantichubgen.MakeBadRequest(
				fmt.Errorf("context schema content must be a JSON-LD document with a non-empty @context object"))
		}
	}
	if strings.TrimSpace(content) == "" {
		return nil, semantichubgen.MakeBadRequest(errors.New("schema content must not be empty (provide content or source_url)"))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	activate := p.Activate != nil && *p.Activate
	requestedVersion := 0
	if p.Version != nil {
		requestedVersion = *p.Version
	}
	version, err := s.Repo.Register(ctx, tx, p.Name, p.Kind, p.MediaType, content, middleware.GetParticipantID(ctx), requestedVersion, activate)
	if err != nil {
		// An occupied version number is the registrant's mistake to correct,
		// not a server fault: the hub already holds that immutable version.
		if errors.Is(err, semantichub.ErrVersionTaken) {
			return nil, semantichubgen.MakeBadRequest(err)
		}
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	if activate {
		if err := RefreshValidationAnchors(ctx, s.DB); err != nil {
			return nil, semantichubgen.MakeInternalError(fmt.Errorf("schema version %d activated but re-anchoring failed: %w", version, err))
		}
	}

	return &semantichubgen.SemanticSchemaRegisterResponse{
		Name:    p.Name,
		Version: version,
		Kind:    p.Kind,
		Active:  activate,
	}, nil
}

func (s *semanticHubsrvc) Rollback(ctx context.Context, p *semantichubgen.RollbackPayload) (res *semantichubgen.SemanticSchemaRegisterResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.Repo.Activate(ctx, tx, p.Name, p.Kind, p.Version); err != nil {
		if errors.Is(err, semantichub.ErrSchemaNotFound) {
			return nil, semantichubgen.MakeNotFound(err)
		}
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := RefreshValidationAnchors(ctx, s.DB); err != nil {
		return nil, semantichubgen.MakeInternalError(fmt.Errorf("schema version %d activated but re-anchoring failed: %w", p.Version, err))
	}

	return &semantichubgen.SemanticSchemaRegisterResponse{
		Name:    p.Name,
		Version: p.Version,
		Kind:    p.Kind,
		Active:  true,
	}, nil
}

func (s *semanticHubsrvc) Retrieve(ctx context.Context, p *semantichubgen.RetrievePayload) (res *semantichubgen.SemanticSchemaItem, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, p.Name, p.Kind, p.Version)
	if err != nil {
		return nil, err
	}
	return toSchemaItem(schema), nil
}

func (s *semanticHubsrvc) Versions(ctx context.Context, p *semantichubgen.VersionsPayload) (res []*semantichubgen.SemanticSchemaItem, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	schemas, err := s.Repo.Versions(ctx, tx, p.Name, p.Kind)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	out := make([]*semantichubgen.SemanticSchemaItem, 0, len(schemas))
	for i := range schemas {
		out = append(out, toSchemaItem(&schemas[i]))
	}
	return out, nil
}

// ResolveShapes serves SHACL shapes at the anchor path
// (/semantic/shapes/{name}?version=N) semantichub.AnchorURL embeds into
// every produced document's sh:shapesGraph.
func (s *semanticHubsrvc) ResolveShapes(ctx context.Context, p *semantichubgen.ResolveShapesPayload) (res *semantichubgen.SemanticSchemaItem, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, p.Name, "shapes", p.Version)
	if err != nil {
		return nil, err
	}
	return toSchemaItem(schema), nil
}

// ResolveOntology serves ontology documents at /semantic/ontology/{name} —
// the dereference target of the dcs: term IRIs via the w3id.org redirect.
func (s *semanticHubsrvc) ResolveOntology(ctx context.Context, p *semantichubgen.ResolveOntologyPayload) (res *semantichubgen.SemanticSchemaItem, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, p.Name, "ontology", p.Version)
	if err != nil {
		return nil, err
	}
	return toSchemaItem(schema), nil
}

// ResolveProfile serves a registered validation profile at
// /semantic/profile/{name}?version=N. Documents no longer carry a
// dcterms:conformsTo anchor pointing here — claiming conformance to one
// envelope profile said nothing about the vocabulary a contract's own data
// uses — but the profiles remain readable, and the rules still run at
// validation time.
func (s *semanticHubsrvc) ResolveProfile(ctx context.Context, p *semantichubgen.ResolveProfilePayload) (res *semantichubgen.SemanticSchemaItem, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, p.Name, "profile", p.Version)
	if err != nil {
		return nil, err
	}
	return toSchemaItem(schema), nil
}

func (s *semanticHubsrvc) ResolveContext(ctx context.Context, p *semantichubgen.ResolveContextPayload) (res any, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, p.Name, "context", p.Version)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal([]byte(schema.Content), &doc); err != nil {
		return nil, semantichubgen.MakeInternalError(fmt.Errorf("stored context %s v%d is not valid JSON: %w", schema.Name, schema.Version, err))
	}
	return doc, nil
}

// Clauses serves the pre-digested clause catalog form-schema derived from
// the same shapes graph validateAgainstHubShapes enforces.
func (s *semanticHubsrvc) Clauses(ctx context.Context) (res *semantichubgen.ClauseCatalogResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	schema, err := s.getSchema(ctx, semantichub.ClauseCatalogName, "shapes", nil)
	if err != nil {
		return nil, err
	}
	prefixes, _, err := semantichub.ActiveOntologyIRIs(ctx, s.DB)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(fmt.Errorf("load active hub context prefixes: %w", err))
	}
	entries, err := semantichub.ParseClauseCatalog(schema.Content, prefixes)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(fmt.Errorf("parse clause catalog v%d: %w", schema.Version, err))
	}

	clauses := make([]*semantichubgen.ClauseCatalogType, 0, len(entries))
	for _, entry := range entries {
		clauses = append(clauses, &semantichubgen.ClauseCatalogType{
			Type:  entry.Type,
			Label: entry.Label,
			Shape: entry.Shape,
		})
	}

	return &semantichubgen.ClauseCatalogResponse{
		Version: schema.Version,
		Clauses: clauses,
		Shapes:  schema.Content,
	}, nil
}

// List serves the hub's (name, kind) inventory for the management UI.
func (s *semanticHubsrvc) List(ctx context.Context) (res []*semantichubgen.SemanticSchemaListEntry, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	entries, err := s.Repo.List(ctx, tx)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	out := make([]*semantichubgen.SemanticSchemaListEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &semantichubgen.SemanticSchemaListEntry{
			Name:          e.Name,
			Kind:          e.Kind,
			MediaType:     e.MediaType,
			ActiveVersion: e.ActiveVersion,
			LatestVersion: e.LatestVersion,
			UpdatedAt:     e.UpdatedAt,
		})
	}
	return out, nil
}

// RefreshValidationAnchors re-points the validation layer's process-wide
// schema anchors at the hub's current active versions. Called at startup
// and after every activation (register-with-activate, rollback).
func RefreshValidationAnchors(ctx context.Context, db *sqlx.DB) error {
	hubIRIs, contextVersion, err := semantichub.ActiveOntologyIRIs(ctx, db)
	if err != nil {
		return fmt.Errorf("load active hub context: %w", err)
	}
	shapesVersion, err := semantichub.ActiveVersion(ctx, db, semantichub.ShapesName, "shapes")
	if err != nil {
		return fmt.Errorf("load active hub shapes version: %w", err)
	}
	// The class -> library index a produced document's sh:shapesGraph
	// declares its registered shape libraries from (ADR-23): validation
	// honours the declaration, so a library must be declared to be enforced.
	libraryClasses, err := semantichub.ActiveShapeLibraryClasses(ctx, db)
	if err != nil {
		return fmt.Errorf("index active hub shape libraries: %w", err)
	}
	libraryAnchors := make(map[string]validation.ShapeLibraryAnchor, len(libraryClasses))
	for class, library := range libraryClasses {
		libraryAnchors[class] = validation.ShapeLibraryAnchor{
			Name: library.Name,
			URL:  semantichub.AnchorURL("shapes", library.Name, library.Version),
		}
	}
	validation.SetShapeLibraryAnchors(libraryAnchors)
	validation.SetCanonicalOntologyIRIs(hubIRIs)
	validation.ResetDomainOntologyCache()
	validation.SetSchemaAnchorRefs(
		semantichub.AnchorURL("context", semantichub.ContextName, contextVersion),
		semantichub.AnchorURL("shapes", semantichub.ShapesName, shapesVersion),
	)
	return nil
}

func (s *semanticHubsrvc) getSchema(ctx context.Context, name, kind string, version *int) (*semantichub.Schema, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	v := 0
	if version != nil {
		v = *version
	}
	schema, err := s.Repo.Get(ctx, tx, name, kind, v)
	if err != nil {
		if errors.Is(err, semantichub.ErrSchemaNotFound) {
			return nil, semantichubgen.MakeNotFound(err)
		}
		return nil, semantichubgen.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, semantichubgen.MakeInternalError(err)
	}
	return schema, nil
}

func toSchemaItem(s *semantichub.Schema) *semantichubgen.SemanticSchemaItem {
	return &semantichubgen.SemanticSchemaItem{
		Name:      s.Name,
		Version:   s.Version,
		Kind:      s.Kind,
		MediaType: s.MediaType,
		Content:   s.Content,
		Active:    s.Active,
		CreatedBy: s.CreatedBy,
		CreatedAt: s.CreatedAt,
	}
}
