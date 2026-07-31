package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func hubSLAOntologyTTL() string {
	return mustReadRepoFile("backend/internal/semantichub/assets/facis-sla-ontology.ttl")
}

func TestParseDomainOntologyIndexesFieldsByIRIOnly(t *testing.T) {
	ontology, err := parseDomainOntology(hubSLAOntologyTTL())
	require.NoError(t, err)

	country, ok := ontology.fields["https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"]
	require.True(t, ok)
	require.NotNil(t, country.Constraint)
	require.Contains(t, country.Constraint.AllowedValues, "DEU")

	payment, ok := ontology.fields["https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-amount"]
	require.True(t, ok)
	require.Nil(t, payment.Constraint)

	// dotted semantic paths and prefixed subjects are no longer identities
	_, ok = ontology.fields["company.location.country"]
	require.False(t, ok)
	_, ok = ontology.fields["dcst:field-company-location-country"]
	require.False(t, ok)

	// the contract-party role stays a workflow concern, not a domain field
	_, ok = ontology.fields["https://w3id.org/facis/dcs/taxonomy/v1#field-company-role"]
	require.False(t, ok)
}

func TestParseDomainOntologyDerivesEntityRolePrefix(t *testing.T) {
	ontology, err := parseDomainOntology(hubSLAOntologyTTL())
	require.NoError(t, err)

	require.Equal(t, "https://w3id.org/facis/dcs/taxonomy/v1#role-", ontology.entityRolePrefix)
}

func TestParseDomainOntologyResolvesValueOptions(t *testing.T) {
	ontology, err := parseDomainOntology(hubSLAOntologyTTL())
	require.NoError(t, err)

	currency := ontology.fields["https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-currency"]
	require.NotNil(t, currency.Constraint)
	options := map[string]valueOption{}
	for _, option := range currency.Constraint.ValueOptions {
		options[option.Value] = option
	}
	euro, ok := options["EUR"]
	require.True(t, ok)
	require.Equal(t, "Euro", euro.Label)
	require.Equal(t, "€", euro.Symbol)
	require.Equal(t, "https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR", euro.IRI)
}

func TestParseDomainOntologyRequiresReferencedConstraints(t *testing.T) {
	_, err := parseDomainOntology(`
@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix dcst: <https://w3id.org/facis/dcs/taxonomy/v1#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .

dcst:field-company-role a dcs:DomainField ;
  rdfs:label "Company Contract Role"@en ;
  rdfs:range xsd:string ;
  dcs:hasValueConstraint dcst:missing-constraint .
`)

	require.ErrorContains(t, err, "unknown value constraint")
}

func TestParseDomainOntologyRequiresLabelAndRange(t *testing.T) {
	_, err := parseDomainOntology(`
@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix dcst: <https://w3id.org/facis/dcs/taxonomy/v1#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

dcst:field-unlabeled a dcs:DomainField .
`)
	require.ErrorContains(t, err, "requires rdfs:label")

	_, err = parseDomainOntology(`
@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix dcst: <https://w3id.org/facis/dcs/taxonomy/v1#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

dcst:field-rangeless a dcs:DomainField ;
  rdfs:label "Rangeless"@en .
`)
	require.ErrorContains(t, err, "requires rdfs:range")
}
