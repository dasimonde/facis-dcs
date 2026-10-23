package semantichub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const minimalShapesTurtle = `
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix ex: <https://example.org/> .
ex:Shape a sh:NodeShape ;
  sh:targetClass ex:Thing .
`

const minimalOntologyTurtle = `
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix ex: <https://example.org/> .
ex:Term a rdfs:Class ;
  rdfs:label "term" .
`

const minimalProfileYAML = `
id: admission.test
version: "1"
rules:
  - id: something-exists
    type: exists
    severity: error
    where:
      "@type": https://example.org/Thing
`

const minimalContextJSON = `{"@context":{"ex":"https://example.org/"}}`

func TestValidateSchemaContentAcceptsGenesisAssets(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		content string
	}{
		{"context", "context", string(genesisContext)},
		{"shapes", "shapes", string(genesisShapes)},
		{"clause-catalog", "shapes", string(genesisClauseCatalog)},
		{"profile", "profile", string(genesisProfile)},
		{"sla-ontology", "ontology", string(genesisSLAOntology)},
		{"odrl-ontology", "ontology", string(genesisODRLProfile)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, ValidateSchemaContent(tc.kind, tc.content))
		})
	}
}

func TestValidateSchemaContentAcceptsMinimalValidDocuments(t *testing.T) {
	cases := []struct {
		kind    string
		content string
	}{
		{"shapes", minimalShapesTurtle},
		{"ontology", minimalOntologyTurtle},
		{"profile", minimalProfileYAML},
		{"context", minimalContextJSON},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			require.NoError(t, ValidateSchemaContent(tc.kind, tc.content))
		})
	}
}

func TestValidateSchemaContentRejectsBrokenInput(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		content string
		want    string
	}{
		{"empty", "shapes", "", "must not be empty"},
		{"whitespace-only", "ontology", "  \n\t  ", "must not be empty"},
		{"shapes-not-turtle", "shapes", "this is not turtle", "not valid Turtle"},
		{"ontology-broken-turtle", "ontology", "@prefix broken", "not valid Turtle"},
		{"shapes-turtle-without-shape", "shapes", minimalOntologyTurtle, "at least one sh:NodeShape or sh:PropertyShape"},
		{"ontology-empty-graph", "ontology", "@prefix ex: <https://example.org/> .\n", "at least one RDF triple"},
		{"ontology-invalid-parameter-type", "ontology", `
@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
@prefix odrl: <http://www.w3.org/ns/odrl/2/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
odrl:eq a odrl:Operator ;
  rdfs:label "eq" ;
  dcs:appliesToParameterType "string", "enum1" .
`, `invalid appliesToParameterType "enum1"`},
		{"ontology-invalid-parameter-type-other-ns", "ontology", `
@prefix dcs: <https://w3id.org/facis/my-dcs/ontology/v1#> .
@prefix odrl: <http://www.w3.org/ns/odrl/2/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
odrl:eq a odrl:Operator ;
  rdfs:label "eq" ;
  dcs:appliesToParameterType "enum1" .
`, `invalid appliesToParameterType "enum1"`},
		{"shapes-property-without-path", "shapes", `
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix ex: <https://example.org/> .
ex:S a sh:NodeShape ;
  sh:targetClass ex:T ;
  sh:property [ sh:minCount 1 ] .
`, "sh:property without sh:path"},
		{"shapes-non-integer-mincount", "shapes", `
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix ex: <https://example.org/> .
ex:S a sh:NodeShape ;
  sh:targetClass ex:T ;
  sh:property [ sh:path ex:p ; sh:minCount "x" ] .
`, `non-integer sh:minCount`},
		{"profile-incomplete-yaml", "profile", "id: only\n", "not a valid validation profile"},
		{"profile-missing-rules", "profile", "id: broken\nversion: \"1\"\nrules: []\n", "not a valid validation profile"},
		{"profile-rule-missing-target", "profile", "id: broken\nversion: \"1\"\nrules:\n  - id: missing-target\n    type: comparison\n    operator: gt\n    value: 0\n", "not a valid validation profile"},
		{"context-empty-object", "context", `{"@context":{}}`, "non-empty @context"},
		{"context-missing-key", "context", `{"name":"no-context"}`, "non-empty @context"},
		{"context-not-json", "context", `not-json`, "non-empty @context"},
		{"context-invalid-container-set1", "context", `{
			"@context": {
				"signatureFields": {
					"@id": "https://example.org/signatureFields",
					"@type": "@id",
					"@container": "@set1"
				}
			}
		}`, "not a usable JSON-LD context"},
		{"unknown-kind", "unknown", "x", `unsupported schema kind "unknown"`},
		{"shapes-as-profile", "profile", minimalShapesTurtle, "not a valid validation profile"},
		{"profile-as-shapes", "shapes", minimalProfileYAML, "not valid Turtle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSchemaContent(tc.kind, tc.content)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestValidateSchemaContentRejectsOdrlProfileWithEnum1(t *testing.T) {
	mutated := strings.Replace(string(genesisODRLProfile), `"enum"`, `"enum1"`, 1)
	require.Contains(t, mutated, `"enum1"`)
	err := ValidateSchemaContent("ontology", mutated)
	require.Error(t, err)
	require.ErrorContains(t, err, `invalid appliesToParameterType "enum1"`)
}

func TestValidateSchemaContentRejectsGenesisContextWithSet1Container(t *testing.T) {
	mutated := mutateFirstSetContainer(t, string(genesisContext), "@set1")
	err := ValidateSchemaContent("context", mutated)
	require.Error(t, err)
	require.ErrorContains(t, err, "not a usable JSON-LD context")
	require.ErrorContains(t, err, "invalid container mapping")
}

func mutateFirstSetContainer(t *testing.T, content, replacement string) string {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(content), &doc))

	var found bool
	var walk func(any)
	walk = func(v any) {
		if found {
			return
		}
		switch node := v.(type) {
		case map[string]any:
			if c, ok := node["@container"].(string); ok && c == "@set" {
				node["@container"] = replacement
				found = true
				return
			}
			for _, child := range node {
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(doc["@context"])
	require.True(t, found, "genesis context should contain at least one @container:@set")

	out, err := json.Marshal(doc)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(out), `"@container":"`+replacement+`"`))
	return string(out)
}
