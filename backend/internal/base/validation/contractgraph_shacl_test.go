package validation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// externalLibraryShapes is a vanilla third-party SHACL library: it targets
// its own classes and constrains plain instance data — it knows nothing
// about the DCS field indirection.
const externalLibraryShapes = `
@prefix sh:  <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix ex:  <https://example.org/vocab#> .

ex:LegalPersonShape a sh:NodeShape ;
    sh:targetClass ex:LegalPerson ;
    sh:property [
        sh:path ex:registrationNumber ;
        sh:datatype xsd:string ;
        sh:minCount 1 ;
        sh:maxCount 1
    ] ;
    sh:property [
        sh:path ex:legalAddress ;
        sh:class ex:Address ;
        sh:minCount 1
    ] .

ex:AddressShape a sh:NodeShape ;
    sh:targetClass ex:Address ;
    sh:property [
        sh:path ex:countryName ;
        sh:datatype xsd:string ;
        sh:minCount 1 ;
        sh:maxCount 1
    ] .
`

func swapExternalLibraryShapeSource(t *testing.T) func() {
	t.Helper()
	return swapShapeSource(t, fixtureShapeSource{
		shapesTTL: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-shapes.ttl") +
			"\n\n" + externalLibraryShapes,
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld"),
	})
}

func errorFindings(findings []PolicyFinding) []PolicyFinding {
	var errors []PolicyFinding
	for _, finding := range findings {
		if finding.Severity == "error" {
			errors = append(errors, finding)
		}
	}
	return errors
}

// A filled contract's object graph validates against the vanilla external
// library: the field reference on ex:countryName dereferences to its
// dcs:value, class traversal follows the in-document @id links, and fixed
// literals sit on their properties directly.
func TestExternalLibraryValidatesFilledContractGraph(t *testing.T) {
	restore := swapExternalLibraryShapeSource(t)
	defer restore()

	contract := nestedDomainContract(t)
	field := contract["dcs:contractFields"].([]any)[0].(map[string]any)
	field["dcs:value"] = "DEU"

	findings, _, err := validateAgainstHubShapes(context.Background(), contract)
	require.NoError(t, err)
	require.Empty(t, errorFindings(findings), fmt.Sprintf("expected a conformant graph, got: %+v", findings))
}

// While the referenced field is unfilled the property is absent, so the
// library's own cardinality constraint names exactly what negotiation still
// has to deliver.
func TestExternalLibraryFlagsUnfilledFieldProperty(t *testing.T) {
	restore := swapExternalLibraryShapeSource(t)
	defer restore()

	contract := nestedDomainContract(t)
	findings, _, err := validateAgainstHubShapes(context.Background(), contract)
	require.NoError(t, err)
	require.NotEmpty(t, findings)
	require.Contains(t, fmt.Sprintf("%+v", findings), "countryName")
}

// A fixed literal violating the library's datatype is reported — external
// semantics apply to the live document without any export step.
func TestExternalLibraryFlagsWrongLiteralDatatype(t *testing.T) {
	restore := swapExternalLibraryShapeSource(t)
	defer restore()

	contract := nestedDomainContract(t)
	field := contract["dcs:contractFields"].([]any)[0].(map[string]any)
	field["dcs:value"] = "DEU"
	hq := contract["dcs:contractData"].([]any)[2].(map[string]any)
	hq["ex:countryName"] = 276

	findings, _, err := validateAgainstHubShapes(context.Background(), contract)
	require.NoError(t, err)
	require.NotEmpty(t, errorFindings(findings))
	require.Contains(t, fmt.Sprintf("%+v", findings), "countryName")
}

func TestMaterializeKeepsOriginalUntouched(t *testing.T) {
	contract := nestedDomainContract(t)
	field := contract["dcs:contractFields"].([]any)[0].(map[string]any)
	field["dcs:value"] = "DEU"

	materialized := materializeContractDataFields(contract)

	legal := contract["dcs:contractData"].([]any)[1].(map[string]any)
	require.Equal(t, map[string]any{"@id": "urn:uuid:field-legal-country"}, legal["ex:countryName"],
		"the stored document must keep the field reference — materialization is validation-only")
	materializedLegal := materialized["dcs:contractData"].([]any)[1].(map[string]any)
	require.Equal(t, "DEU", materializedLegal["ex:countryName"])
}

// A fill written as a typed {"@value"} literal — the canonical serialization
// for "properly @value-linked" data — dereferences through materialization
// and satisfies the external library exactly like a bare scalar fill.
func TestExternalLibraryAcceptsTypedValueFill(t *testing.T) {
	restore := swapExternalLibraryShapeSource(t)
	defer restore()

	contract := nestedDomainContract(t)
	field := contract["dcs:contractFields"].([]any)[0].(map[string]any)
	field["dcs:value"] = map[string]any{"@value": "DEU", "@type": "xsd:string"}

	findings, _, err := validateAgainstHubShapes(context.Background(), contract)
	require.NoError(t, err)
	require.Empty(t, errorFindings(findings), fmt.Sprintf("expected a conformant graph, got: %+v", findings))
}
