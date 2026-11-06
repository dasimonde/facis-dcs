package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/pgtest"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/semantichub"
)

// DCS-IR-CWE-07 and DCS-FR-CWE-13: contract search filters by party, template
// and jurisdiction.
func TestContractSearchFiltersByPartyTemplateAndJurisdictionPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`
		INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,responsible,contract_data) VALUES
		('de','','Org A','DRAFT','tpl-1',1,'{"creator":"did:web:a","counterparty":"did:web:b"}',
		 '{"dcs:contractFields":[{"@id":"de#j","dcs:shape":{"@id":"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"},"dcs:value":"DEU"}]}'),
		('fr','','Org A','DRAFT','tpl-2',1,'{"creator":"did:web:a","counterparty":"did:web:c"}',
		 '{"dcs:contractFields":[{"@id":"fr#j","dcs:shape":{"@id":"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"},"dcs:value":{"@value":"FRA"}}]}')`)
	require.NoError(t, err)
	_, err = database.Exec(`UPDATE contracts SET search_projection = jsonb_build_object('https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction', jsonb_build_array(COALESCE(contract_data->'dcs:contractFields'->0->'dcs:value'->>'@value', contract_data->'dcs:contractFields'->0->>'dcs:value')))`)
	require.NoError(t, err)
	ctx := contractreadpolicy.WithSystemScope(context.Background())
	search := func(values cwedb.SearchValues) []string {
		tx, err := database.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		rows, err := (&cwepg.PostgresContractRepo{}).ReadAllMetaDataByFilter(ctx, tx, values, datatype.Pagination{})
		require.NoError(t, err)
		dids := []string{}
		for _, row := range rows {
			dids = append(dids, row.DID)
		}
		return dids
	}

	require.Equal(t, []string{"fr"}, search(cwedb.SearchValues{Party: "did:web:c"}))
	require.Equal(t, []string{"de"}, search(cwedb.SearchValues{TemplateDID: "tpl-1"}))
	require.Equal(t, []string{"de"}, search(cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction", Value: "DEU"}}}))
	require.Equal(t, []string{"fr"}, search(cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction", Value: "FRA"}}, Party: "did:web:a"}))
}

// DCS-FR-CWE-13: a wildcard in the industry or name filter matches as text.
func TestContractSearchMatchesWildcardsAsTextPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`
		INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES
		('de','','Org A','DRAFT','tpl-1',1,'Supply',
		 '{"dcs:contractFields":[{"@id":"de#j","dcs:shape":{"@id":"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry"},"dcs:value":"Automotive"}]}'),
		('pct','','Org A','DRAFT','tpl-1',1,'Supply 100%',
		 '{"dcs:contractFields":[{"@id":"pct#j","dcs:shape":{"@id":"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry"},"dcs:value":"Energy_DE"}]}')`)
	require.NoError(t, err)
	_, err = database.Exec(`UPDATE contracts SET search_projection = jsonb_build_object('https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry', jsonb_build_array(COALESCE(contract_data->'dcs:contractFields'->0->'dcs:value'->>'@value', contract_data->'dcs:contractFields'->0->>'dcs:value')))`)
	require.NoError(t, err)
	ctx := contractreadpolicy.WithSystemScope(context.Background())
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	search := func(values cwedb.SearchValues) []string {
		rows, err := (&cwepg.PostgresContractRepo{}).ReadAllMetaDataByFilter(ctx, tx, values, datatype.Pagination{})
		require.NoError(t, err)
		dids := []string{}
		for _, row := range rows {
			dids = append(dids, row.DID)
		}
		return dids
	}

	require.Equal(t, []string{"pct"}, search(cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry", Value: "_", Text: true}}}))
	require.Empty(t, search(cwedb.SearchValues{SemanticFilters: []semantichub.ResolvedSearchFilter{{FieldIRI: "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry", Value: "%x", Text: true}}}))
	require.Equal(t, []string{"pct"}, search(cwedb.SearchValues{Name: "%"}))
}

// DCS-IR-CWE-11: the state filter and the sort apply before the page is cut,
// so a matching contract on a later page is found.
func TestContractListFiltersAndSortsBeforeThePagePostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`
		INSERT INTO contracts (did,origin,created_by,created_at,state,template_did,template_version,name) VALUES
		('d1','','Org A',now() - interval '1 minute','DRAFT','tpl',1,'B'),
		('d2','','Org A',now() - interval '2 minute','DRAFT','tpl',1,'C'),
		('d3','','Org A',now() - interval '3 minute','DRAFT','tpl',1,'D'),
		('t1','','Org A',now() - interval '4 minute','TERMINATED','tpl',1,'A'),
		('n1','','Org A',now() - interval '5 minute','NEGOTIATION','tpl',1,'E')`)
	require.NoError(t, err)
	ctx := contractreadpolicy.WithSystemScope(context.Background())
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	repo := &cwepg.PostgresContractRepo{}
	dids := func(rows []cwedb.ContractMetadata) []string {
		out := []string{}
		for _, row := range rows {
			out = append(out, row.DID)
		}
		return out
	}

	page := datatype.Pagination{Offset: 1, Limit: 2}
	rows, err := repo.ReadAllMetaData(ctx, tx, page, cwedb.ListOptions{States: []string{"TERMINATED", "NEGOTIATION"}})
	require.NoError(t, err)
	require.Equal(t, []string{"t1", "n1"}, dids(rows))

	rows, err = repo.ReadAllMetaData(ctx, tx, page, cwedb.ListOptions{SortBy: "name", Ascending: true})
	require.NoError(t, err)
	require.Equal(t, []string{"t1", "d1"}, dids(rows))

	_, err = repo.ReadAllMetaData(ctx, tx, page, cwedb.ListOptions{SortBy: "did; DROP TABLE contracts"})
	require.Error(t, err)

	rows, err = repo.ReadAllMetaDataByFilter(ctx, tx, cwedb.SearchValues{States: []string{"terminated", "NEGOTIATION"}}, datatype.Pagination{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"t1", "n1"}, dids(rows))
}
