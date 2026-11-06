package semantichub

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
)

func TestSearchFilterConfigurationPersistsAndRejectsStaleUpdates(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, Seed(ctx, db))
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	config, err := LoadSearchFilters(ctx, tx)
	require.NoError(t, err)
	require.Len(t, config.Filters, 4)
	stale := config
	config.Filters = config.Filters[1:]
	saved, err := SaveSearchFilters(ctx, tx, config)
	require.NoError(t, err)
	require.Equal(t, config.Revision+1, saved.Revision)
	require.NoError(t, tx.Commit())

	tx, err = db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = SaveSearchFilters(ctx, tx, stale)
	require.ErrorIs(t, err, ErrSearchFilterConflict)
	require.NoError(t, tx.Rollback())
	require.NoError(t, Seed(ctx, db))
	tx, err = db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	loaded, err := LoadSearchFilters(ctx, tx)
	require.NoError(t, err)
	require.Equal(t, saved, loaded)
	filters, err := ResolveSearchFilters(ctx, tx, `[{"filter_id":"industry","value":"finance"}]`)
	require.NoError(t, err)
	require.Len(t, filters, 1)
	require.True(t, filters[0].Text)
	require.Equal(t, "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry", filters[0].FieldIRI)
	for _, invalid := range []string{
		`[{"filter_id":"unknown","value":"x"}]`,
		`[{"filter_id":"industry","value":"x","extra":true}]`,
		`[{"filter_id":"industry","value":"x"},{"filter_id":"industry","value":"y"}]`,
		`[{"filter_id":"industry","value":false}]`,
	} {
		_, err := ResolveSearchFilters(ctx, tx, invalid)
		require.ErrorIs(t, err, ErrInvalidSearchFilter, invalid)
	}
	_, err = tx.ExecContext(ctx, `CREATE TEMP TABLE search_projection_test (search_projection jsonb)`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO search_projection_test VALUES ('{"urn:field:text":["100%_safe"],"urn:field:number":["1.00"]}'), ('{"urn:field:text":["100xxsafe"],"urn:field:number":[2]}')`)
	require.NoError(t, err)
	condition, args := SearchFilterSQL([]ResolvedSearchFilter{
		{FieldIRI: "urn:field:text", Value: "%_", Text: true},
		{FieldIRI: "urn:field:number", Value: float64(1), Datatype: "http://www.w3.org/2001/XMLSchema#decimal"},
	}, "search_projection", 1)
	var count int
	require.NoError(t, tx.GetContext(ctx, &count, `SELECT count(*) FROM search_projection_test WHERE `+condition, args...))
	require.Equal(t, 1, count)
	invalid := loaded
	invalid.Filters = append([]SearchFilter(nil), loaded.Filters...)
	invalid.Filters[0].FieldIRI = "urn:unknown"
	_, err = SaveSearchFilters(ctx, tx, invalid)
	require.Error(t, err)
	unchanged, err := LoadSearchFilters(ctx, tx)
	require.NoError(t, err)
	require.Equal(t, loaded, unchanged)
}

func TestTypedSearchComparisonsPostgres(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`CREATE TEMP TABLE typed_search_projection (projection jsonb)`)
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO typed_search_projection VALUES
		('{"urn:number":["1.00"],"urn:date":["2026-11-04"],"urn:zoned-date":["2026-11-04+01:00"],"urn:time":["2026-11-04T12:00:00.123456789+01:00"]}'),
		('{"urn:number":["invalid","1e99999999"],"urn:date":["2026-99-99"],"urn:time":["invalid"]}')`)
	require.NoError(t, err)
	for _, filter := range []ResolvedSearchFilter{
		{FieldIRI: "urn:number", Value: json.Number("1"), Datatype: xsdNamespace + "nonNegativeInteger"},
		{FieldIRI: "urn:number", Value: "1", Datatype: xsdNamespace + "nonNegativeInteger"},
		{FieldIRI: "urn:date", Value: "2026-11-04", Datatype: xsdNamespace + "date"},
		{FieldIRI: "urn:zoned-date", Value: "2026-11-04+01:00", Datatype: xsdNamespace + "date"},
		{FieldIRI: "urn:time", Value: "2026-11-04T11:00:00.123456789Z", Datatype: xsdNamespace + "dateTime"},
	} {
		condition, args := SearchFilterSQL([]ResolvedSearchFilter{filter}, "projection", 1)
		var count int
		require.NoError(t, tx.GetContext(ctx, &count, `SELECT count(*) FROM typed_search_projection WHERE `+condition, args...))
		require.Equal(t, 1, count, filter.Datatype)
	}
	condition, args := SearchFilterSQL([]ResolvedSearchFilter{{FieldIRI: "urn:time", Value: "2026-11-04T11:00:00.123456788Z", Datatype: xsdNamespace + "dateTime"}}, "projection", 1)
	var count int
	require.NoError(t, tx.GetContext(ctx, &count, `SELECT count(*) FROM typed_search_projection WHERE `+condition, args...))
	require.Zero(t, count)
}

func TestSearchFilterTextTypesAndReferencedCatalogPostgres(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, Seed(ctx, db))
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	ontology := `@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
<urn:field:token> a dcs:DomainField ; rdfs:range xsd:token .
<urn:field:catalog> a dcs:DomainField ; rdfs:range xsd:string ; dcs:hasValueConstraint <urn:constraint> .
<urn:constraint> dcs:allowedValuesRef "ISO 3166-1 alpha-3" .
<urn:field:iri> a dcs:DomainField ; rdfs:range xsd:anyURI .`
	version, err := (Repo{}).Register(ctx, tx, "search-text-test", "ontology", "text/turtle", ontology, "test", 1, true)
	require.NoError(t, err)
	config, err := LoadSearchFilters(ctx, tx)
	require.NoError(t, err)
	for _, id := range []string{"token", "catalog", "iri"} {
		config.Filters = append(config.Filters, SearchFilter{ID: id, Label: id, FieldIRI: "urn:field:" + id, Source: SearchFilterSource{Name: "search-text-test", Kind: "ontology", Version: version}})
	}
	_, err = SaveSearchFilters(ctx, tx, config)
	require.NoError(t, err)
	_, err = tx.Exec(`CREATE TEMP TABLE text_projection (projection jsonb)`)
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO text_projection VALUES ('{"urn:field:token":["Hello World"],"urn:field:catalog":["DEU"],"urn:field:iri":["https://example.org/value"]}')`)
	require.NoError(t, err)
	for _, tc := range []struct {
		id, value string
		expected  int
	}{{"token", "WORLD", 1}, {"catalog", "de", 0}, {"catalog", "DEU", 1}, {"iri", "example", 0}, {"iri", "https://example.org/value", 1}} {
		raw, err := json.Marshal([]map[string]string{{"filter_id": tc.id, "value": tc.value}})
		require.NoError(t, err)
		filters, err := ResolveSearchFilters(ctx, tx, string(raw))
		require.NoError(t, err)
		condition, args := SearchFilterSQL(filters, "projection", 1)
		var count int
		require.NoError(t, tx.GetContext(ctx, &count, `SELECT count(*) FROM text_projection WHERE `+condition, args...))
		require.Equal(t, tc.expected, count, tc.id)
	}
}

func TestSeededJurisdictionUsesExactCodesAndIndustryRemainsText(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, Seed(ctx, db))
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`CREATE TEMP TABLE catalog_search_projection (projection jsonb)`)
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO catalog_search_projection VALUES
		('{"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction":["DEU"],"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry":["Automotive"]}'),
		('{"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction":["AUT"],"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry":["Finance"]}')`)
	require.NoError(t, err)
	for _, tc := range []struct {
		id, value string
		text      bool
		count     int
	}{
		{"jurisdiction", "DEU", false, 1},
		{"jurisdiction", "AUT", false, 1},
		{"jurisdiction", "DE", false, 0},
		{"jurisdiction", "deu", false, 0},
		{"jurisdiction", "Germany", false, 0},
		{"industry", "MOT", true, 1},
	} {
		t.Run(tc.id+"/"+tc.value, func(t *testing.T) {
			raw, err := json.Marshal([]map[string]string{{"filter_id": tc.id, "value": tc.value}})
			require.NoError(t, err)
			filters, err := ResolveSearchFilters(ctx, tx, string(raw))
			require.NoError(t, err)
			require.Len(t, filters, 1)
			require.Equal(t, tc.text, filters[0].Text)
			condition, args := SearchFilterSQL(filters, "projection", 1)
			var count int
			require.NoError(t, tx.GetContext(ctx, &count, `SELECT count(*) FROM catalog_search_projection WHERE `+condition, args...))
			require.Equal(t, tc.count, count)
		})
	}
}
