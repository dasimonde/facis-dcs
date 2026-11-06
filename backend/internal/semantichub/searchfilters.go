package semantichub

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/tggo/goRDFlib/shacl"
)

type SearchFilterSource struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Version int    `json:"version"`
}

type SearchFilter struct {
	ID       string             `json:"id"`
	Label    string             `json:"label"`
	FieldIRI string             `json:"field_iri"`
	Source   SearchFilterSource `json:"source"`
}

type SearchFilterConfig struct {
	Revision int64          `json:"revision"`
	Filters  []SearchFilter `json:"filters"`
}

var ErrSearchFilterConflict = errors.New("search filter configuration was modified")
var ErrInvalidSearchFilter = errors.New("invalid search filter configuration")

func LoadSearchFilters(ctx context.Context, tx *sqlx.Tx) (SearchFilterConfig, error) {
	var revision int64
	var raw []byte
	if err := tx.QueryRowxContext(ctx, `SELECT revision, filters FROM semantic_search_filter_config WHERE singleton`).Scan(&revision, &raw); err != nil {
		return SearchFilterConfig{}, err
	}
	config := SearchFilterConfig{Revision: revision}
	if err := json.Unmarshal(raw, &config.Filters); err != nil {
		return SearchFilterConfig{}, err
	}
	return config, nil
}

func SaveSearchFilters(ctx context.Context, tx *sqlx.Tx, config SearchFilterConfig) (SearchFilterConfig, error) {
	seen := map[string]bool{}
	contextSchema, err := (Repo{}).Get(ctx, tx, ContextName, "context", 0)
	if err != nil {
		return SearchFilterConfig{}, err
	}
	var contextDoc struct {
		Context map[string]any `json:"@context"`
	}
	if err := json.Unmarshal([]byte(contextSchema.Content), &contextDoc); err != nil {
		return SearchFilterConfig{}, err
	}
	dcs, ok := contextDoc.Context["dcs"].(string)
	if !ok || dcs == "" {
		return SearchFilterConfig{}, errors.New("active context has no dcs namespace")
	}
	for _, filter := range config.Filters {
		if strings.TrimSpace(filter.ID) == "" || strings.TrimSpace(filter.Label) == "" || seen[filter.ID] {
			return SearchFilterConfig{}, fmt.Errorf("%w: filter identifier and label must be non-empty and identifiers unique", ErrInvalidSearchFilter)
		}
		seen[filter.ID] = true
		if filter.Source.Version <= 0 || (filter.Source.Kind != "ontology" && filter.Source.Kind != "shapes") {
			return SearchFilterConfig{}, fmt.Errorf("%w: invalid source for filter %q", ErrInvalidSearchFilter, filter.ID)
		}
		schema, err := (Repo{}).Get(ctx, tx, filter.Source.Name, filter.Source.Kind, filter.Source.Version)
		if err != nil {
			if errors.Is(err, ErrSchemaNotFound) {
				return SearchFilterConfig{}, fmt.Errorf("%w: filter %q source: %v", ErrInvalidSearchFilter, filter.ID, err)
			}
			return SearchFilterConfig{}, fmt.Errorf("filter %q source: %w", filter.ID, err)
		}
		graph, err := shacl.LoadTurtleString(schema.Content, "urn:dcs:search-filter:source")
		if err != nil {
			return SearchFilterConfig{}, err
		}
		field := shacl.IRI(filter.FieldIRI)
		var matches bool
		if filter.Source.Kind == "ontology" {
			rdfType := shacl.IRI(shacl.RDFType)
			domainField := shacl.IRI(dcs + "DomainField")
			for _, object := range graph.Objects(field, rdfType) {
				if object == domainField {
					matches = true
					break
				}
			}
		} else {
			path := shacl.IRI(shacl.SH + "path")
			matches = len(graph.All(nil, &path, &field)) > 0
		}
		if !matches {
			return SearchFilterConfig{}, fmt.Errorf("%w: field %q is not defined by its source", ErrInvalidSearchFilter, filter.FieldIRI)
		}
	}
	raw, err := json.Marshal(config.Filters)
	if err != nil {
		return SearchFilterConfig{}, err
	}
	err = tx.QueryRowxContext(ctx, `UPDATE semantic_search_filter_config SET filters=$1, revision=revision+1 WHERE singleton AND revision=$2 RETURNING revision`, string(raw), config.Revision).Scan(&config.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return SearchFilterConfig{}, ErrSearchFilterConflict
	}
	return config, err
}
