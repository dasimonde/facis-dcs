package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/templatestructure"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"
)

const legalShapesLibrary = `
@prefix sh:  <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix ex:  <https://example.org/legal#> .

ex:LegalPersonShape a sh:NodeShape ;
    sh:targetClass ex:LegalPerson ;
    sh:property [ sh:path ex:registrationNumber ; sh:datatype xsd:string ; sh:minCount 1 ; sh:maxCount 1 ] ;
    sh:property [ sh:path ex:legalForm ; sh:in ( "GmbH" "AG" "SE" ) ; sh:maxCount 1 ] ;
    sh:property [ sh:path ex:website ; sh:nodeKind sh:IRI ; sh:maxCount 1 ] ;
    sh:property [ sh:path ex:legalAddress ; sh:class ex:Address ; sh:minCount 1 ; sh:maxCount 1 ] .

ex:AddressShape a sh:NodeShape ;
    sh:targetClass ex:Address ;
    sh:property [ sh:path ex:countryName ; sh:datatype xsd:string ; sh:minCount 1 ; sh:maxCount 1 ] .
`

// dataObjectTemplateData is the template the builder emits for
// frontend/ClientApp/e2e/semantic-data-objects.spec.ts: a LegalPerson with a
// nested Address whose country is negotiable, and an inlined component clause
// whose prose has no placeholder and whose rule constrains Payment Amount.
const dataObjectTemplateData = `{
  "@type": "dcs:ContractTemplate",
  "dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "Legal Data Template",
    "dcs:description": "Fixture", "dcs:templateType": "dcs:ContractTemplate"},
  "dcs:documentStructure": {
    "@type": "dcs:DocumentStructure",
    "dcs:blocks": {"@list": [{"@type": "dcs:Clause", "@id": "urn:uuid:block-payment",
      "dcs:content": {"@list": ["The provider invoices the agreed payment amount."]}, "dcs:title": "Payment terms"}]},
    "dcs:layout": {"@list": [{"@id": "urn:uuid:block-root", "@type": "dcs:LayoutNode", "dcs:isRoot": true,
      "dcs:children": {"@list": [{"@id": "urn:uuid:block-payment"}]}}]}
  },
  "dcs:contractFields": [
    {"@id": "urn:uuid:field-country", "@type": "dcs:ContractField", "dcs:label": "countryName",
      "dcs:datatype": "xsd:string", "dcs:required": true},
    {"@id": "urn:uuid:block-payment-amount", "@type": "dcs:ContractField", "dcs:label": "Payment Amount",
      "dcs:datatype": "xsd:decimal", "dcs:required": true,
      "dcs:shape": {"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-amount"}}
  ],
  "dcs:contractData": [
    {"@id": "urn:uuid:object-person", "@type": "https://example.org/legal#LegalPerson",
      "https://example.org/legal#registrationNumber": "HRB 4711",
      "https://example.org/legal#legalForm": "GmbH",
      "https://example.org/legal#website": {"@id": "https://musterfirma.example.org/"},
      "https://example.org/legal#legalAddress": {"@id": "urn:uuid:object-address"}},
    {"@id": "urn:uuid:object-address", "@type": "https://example.org/legal#Address",
      "https://example.org/legal#countryName": {"@id": "urn:uuid:field-country"}}
  ],
  "dcs:policies": {
    "@id": "urn:uuid:policy-set", "@type": "odrl:Offer",
    "odrl:profile": {"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
    "odrl:permission": [{
      "@id": "urn:uuid:block-rule", "@type": "odrl:Permission",
      "odrl:action": {"@id": "odrl:use"},
      "odrl:assigner": {"@id": "urn:uuid:party-https%3A%2F%2Fw3id.org%2Ffacis%2Fdcs%2Ftaxonomy%2Fv1%23role-provider"},
      "odrl:assignee": {"@id": "urn:uuid:party-https%3A%2F%2Fw3id.org%2Ffacis%2Fdcs%2Ftaxonomy%2Fv1%23role-customer"},
      "odrl:target": {"@id": "urn:uuid:pending-target"},
      "dcs:prose": {"@id": "urn:uuid:block-payment"},
      "odrl:constraint": [{"@type": "odrl:Constraint",
        "odrl:leftOperand": {"@id": "urn:uuid:block-payment-amount"},
        "odrl:operator": {"@id": "odrl:lteq"},
        "odrl:rightOperand": {"@value": "500", "@type": "xsd:integer"}}]
    }]
  }
}`

// DCS-FR-TR-07, ADR-23: a template authored in the builder with a data object
// of a registered shapes library is COMPLIANT and a contract can be created
// from it.
func TestDataObjectTemplateIsCompliantPostgres(t *testing.T) {
	f := newContractsFromTemplates(t)
	tx := f.database.MustBegin()
	_, err := (semantichub.Repo{}).Register(f.ctx, tx, "e2e-legal-shapes", "shapes", "text/turtle", legalShapesLibrary, "org", 0, true)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, RefreshValidationAnchors(f.ctx, f.database))
	t.Cleanup(func() { validation.SetShapeLibraryAnchors(nil) })

	createStructuredTemplate(t, f.creator, "tpl-data-object", dataObjectTemplateData, templatestructure.Agreement)

	var stored []byte
	require.NoError(t, f.database.Get(&stored, `SELECT template_data FROM contract_templates WHERE did = 'tpl-data-object'`))
	raw := datatype.JSON(stored)
	findings, err := validation.AuditTemplatePolicies(&raw, validation.TemplatePolicyAuditMetadata{
		DID: "tpl-data-object", TemplateType: "CONTRACT_TEMPLATE", State: "APPROVED",
	})
	require.NoError(t, err)
	for _, finding := range findings {
		require.NotEqual(t, validation.SeverityError, finding.Severity, "%s: %s", finding.RuleID, finding.Message)
	}

	require.Equal(t, "COMPLIANT", f.approveAndRegister("tpl-data-object"))
	require.NoError(t, f.createContract("c-data-object", "tpl-data-object"))
}
