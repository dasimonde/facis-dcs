package validation

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

// nestedDomainContract builds a contract whose dcs:contractData is a small
// object graph in the shape any external SHACL library produces: a typed
// root object holding a fixed literal, a negotiable-field reference, and two
// references to a nested object type (the gx:LegalPerson legalAddress /
// headquarterAddress pattern, vocabulary-neutral here).
func nestedDomainContract(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"@context": map[string]any{
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"ex":  "https://example.org/vocab#",
			"xsd": "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   "urn:uuid:contract-nested-graph-1",
		"@type": "dcs:Contract",
		"dcs:metadata": map[string]any{
			"@type":     "dcs:ContractMetadata",
			"dcs:title": "Nested domain graph",
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   "urn:uuid:block-clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{
						"Registered in ",
						map[string]any{"@id": "urn:uuid:field-legal-country"},
					}},
				},
			}},
			"dcs:layout": map[string]any{"@list": []any{
				map[string]any{
					"@id":          "urn:uuid:block-root",
					"@type":        "dcs:LayoutNode",
					"dcs:isRoot":   true,
					"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "urn:uuid:block-clause-1"}}},
				},
			}},
		},
		"dcs:contractFields": []any{
			map[string]any{
				"@id":          "urn:uuid:field-legal-country",
				"@type":        "dcs:ContractField",
				"dcs:label":    "Legal seat country",
				"dcs:datatype": "xsd:string",
				"dcs:required": true,
			},
		},
		"dcs:contractData": []any{
			map[string]any{
				"@id":                   "urn:uuid:object-provider",
				"@type":                 "ex:LegalPerson",
				"ex:registrationNumber": map[string]any{"@value": "HRB 12345", "@type": "xsd:string"},
				"ex:legalAddress":       map[string]any{"@id": "urn:uuid:object-legal-address"},
				"ex:headquarterAddress": map[string]any{"@id": "urn:uuid:object-hq-address"},
			},
			map[string]any{
				"@id":            "urn:uuid:object-legal-address",
				"@type":          "ex:Address",
				"ex:countryName": map[string]any{"@id": "urn:uuid:field-legal-country"},
			},
			map[string]any{
				"@id":            "urn:uuid:object-hq-address",
				"@type":          "ex:Address",
				"ex:countryName": "DEU",
			},
		},
		"dcs:policies": []any{},
	}
}

func normalizeNested(t *testing.T, data map[string]any) error {
	t.Helper()
	raw, err := datatype.NewJSON(data)
	require.NoError(t, err)
	_, err = NormalizeContractData(&raw, false)
	return err
}

func TestContractDataAcceptsNestedObjectGraph(t *testing.T) {
	require.NoError(t, normalizeNested(t, nestedDomainContract(t)))
}

func TestContractDataAcceptsScalarAndTypedLiterals(t *testing.T) {
	data := nestedDomainContract(t)
	object := data["dcs:contractData"].([]any)[0].(map[string]any)
	object["ex:employeeCount"] = 250
	object["ex:active"] = true
	require.NoError(t, normalizeNested(t, data))
}

func TestContractDataRejectsUnresolvedReference(t *testing.T) {
	data := nestedDomainContract(t)
	object := data["dcs:contractData"].([]any)[0].(map[string]any)
	object["ex:legalAddress"] = map[string]any{"@id": "urn:uuid:object-nowhere"}
	err := normalizeNested(t, data)
	require.ErrorContains(t, err, "urn:uuid:object-nowhere")
}

// An sh:nodeKind sh:IRI leaf holds an absolute IRI naming a resource outside
// the document (a Gaia-X provider, a vocabulary individual) — no declaration
// inside the document backs it.
func TestContractDataAcceptsExternalResourceIRI(t *testing.T) {
	data := nestedDomainContract(t)
	object := data["dcs:contractData"].([]any)[0].(map[string]any)
	object["ex:registrationIssuer"] = map[string]any{"@id": "https://registry.example.org/issuers/42"}
	object["ex:providedBy"] = map[string]any{"@id": "did:web:provider.example.org"}
	require.NoError(t, normalizeNested(t, data))
}

func TestContractDataRejectsUnresolvedDocumentScopedReference(t *testing.T) {
	data := nestedDomainContract(t)
	data["@id"] = "did:web:example.org:contracts:1"
	object := data["dcs:contractData"].([]any)[0].(map[string]any)
	object["ex:legalAddress"] = map[string]any{"@id": "did:web:example.org:contracts:1#object-gone"}
	err := normalizeNested(t, data)
	require.ErrorContains(t, err, "#object-gone")
}

func TestContractDataRejectsEmbeddedBlankObject(t *testing.T) {
	data := nestedDomainContract(t)
	object := data["dcs:contractData"].([]any)[0].(map[string]any)
	object["ex:legalAddress"] = map[string]any{
		"@type":          "ex:Address",
		"ex:countryName": "DEU",
	}
	err := normalizeNested(t, data)
	require.ErrorContains(t, err, "ex:legalAddress")
}

func TestContractDataRequiresObjectIdentity(t *testing.T) {
	data := nestedDomainContract(t)
	objects := data["dcs:contractData"].([]any)
	address := objects[1].(map[string]any)
	delete(address, "@id")
	err := normalizeNested(t, data)
	require.ErrorContains(t, err, "@id")
}

// The two-level shape (every property a field ref) must remain valid — it is
// the depth-one special case of the general graph.
func TestContractDataTwoLevelShapeStillValid(t *testing.T) {
	data := nestedDomainContract(t)
	data["dcs:contractData"] = []any{
		map[string]any{
			"@id":            "urn:uuid:object-payment",
			"@type":          "ex:PaymentClause",
			"ex:countryName": map[string]any{"@id": "urn:uuid:field-legal-country"},
		},
	}
	require.NoError(t, normalizeNested(t, data))
}

func TestContractDataGraphSurvivesPersistenceRebase(t *testing.T) {
	raw, err := datatype.NewJSON(nestedDomainContract(t))
	require.NoError(t, err)
	stored, err := NormalizeContractDataForPersistence(&raw, "11111111-2222-3333-4444-555555555555", false)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(*stored, &doc))
	objects := doc["dcs:contractData"].([]any)
	root := objects[0].(map[string]any)
	legalRef := root["ex:legalAddress"].(map[string]any)["@id"].(string)
	address := objects[1].(map[string]any)
	// The rebase must keep the object graph closed: the rewritten reference
	// still names the rewritten nested object.
	require.Equal(t, address["@id"].(string), legalRef)
	countryRef := address["ex:countryName"].(map[string]any)["@id"].(string)
	fieldID := doc["dcs:contractFields"].([]any)[0].(map[string]any)["@id"].(string)
	require.Equal(t, fieldID, countryRef)
}

// A dcs:Contract document whose metadata node still carries the template's
// type must fail at submission — the render gate's SHACL would otherwise
// reject it asynchronously, long after the API call succeeded.
func TestContractRejectsTemplateMetadataType(t *testing.T) {
	data := nestedDomainContract(t)
	data["dcs:metadata"].(map[string]any)["@type"] = "dcs:TemplateMetadata"
	err := normalizeNested(t, data)
	require.ErrorContains(t, err, "dcs:ContractMetadata")
}
