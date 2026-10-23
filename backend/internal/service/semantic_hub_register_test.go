package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	semantichubgen "digital-contracting-service/gen/semantic_hub"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"
)

const (
	registerTestShapes = `
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix ex: <https://example.org/admission/> .
ex:Shape a sh:NodeShape ;
  sh:targetClass ex:Thing .
`
	registerTestOntology = `
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix ex: <https://example.org/admission/> .
ex:Term a rdfs:Class ;
  rdfs:label "term" .
`
	registerTestProfile = `
id: admission.register.test
version: "1"
rules:
  - id: something-exists
    type: exists
    severity: error
    where:
      "@type": https://example.org/admission/Thing
`
)

func requireBadRequest(t *testing.T, err error, wantSubstring string) {
	t.Helper()
	require.Error(t, err)
	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, "bad_request", serviceErr.Name)
	require.Contains(t, serviceErr.Message, wantSubstring)
}

func TestRegisterRejectsInvalidContentAsBadRequest(t *testing.T) {
	s := &semanticHubsrvc{}
	cases := []struct {
		name    string
		kind    string
		content string
		want    string
	}{
		{"empty", "shapes", "", "must not be empty"},
		{"whitespace", "ontology", " \n\t ", "must not be empty"},
		{"shapes-garbage", "shapes", "this is not turtle", "not valid Turtle"},
		{"ontology-broken-prefix", "ontology", "@prefix broken", "not valid Turtle"},
		{"profile-yaml-incomplete", "profile", "id: only\n", "not a valid validation profile"},
		{"profile-no-rules", "profile", "id: broken\nversion: \"1\"\nrules: []\n", "not a valid validation profile"},
		{"context-empty-context", "context", `{"@context":{}}`, "non-empty @context"},
		{"context-missing-context", "context", `{"name":"x"}`, "non-empty @context"},
		{"context-not-json", "context", "not-json", "non-empty @context"},
		{"context-invalid-container-set1", "context", `{"@context":{"signatureFields":{"@id":"https://example.org/sf","@type":"@id","@container":"@set1"}}}`, "not a usable JSON-LD context"},
		{"shapes-turtle-without-shape", "shapes", registerTestOntology, "at least one sh:NodeShape or sh:PropertyShape"},
		{"shapes-property-without-path", "shapes", `
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix ex: <https://example.org/> .
ex:S a sh:NodeShape ;
  sh:targetClass ex:T ;
  sh:property [ sh:minCount 1 ] .
`, "sh:property without sh:path"},
		{"ontology-invalid-parameter-type", "ontology", `
@prefix dcs: <https://w3id.org/facis/my-dcs/ontology/v1#> .
@prefix odrl: <http://www.w3.org/ns/odrl/2/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
odrl:eq a odrl:Operator ;
  rdfs:label "eq" ;
  dcs:appliesToParameterType "string", "enum1" .
`, `invalid appliesToParameterType "enum1"`},
		{"unknown-kind", "not-a-kind", registerTestShapes, `unsupported schema kind`},
		{"shapes-payload-as-profile", "profile", registerTestShapes, "not a valid validation profile"},
		{"profile-payload-as-shapes", "shapes", registerTestProfile, "not valid Turtle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := tc.content
			_, err := s.Register(context.Background(), &semantichubgen.RegisterPayload{
				Name:      "admission-reject",
				Kind:      tc.kind,
				MediaType: "text/plain",
				Content:   &content,
			})
			requireBadRequest(t, err, tc.want)
		})
	}
}

// Admission runs before any DB write, so a bad source_url body is rejected
// without needing postgres.
func TestRegisterRejectsInvalidSourceURLBodyAsBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("definitely not turtle"))
	}))
	t.Cleanup(srv.Close)

	s := &semanticHubsrvc{}
	source := srv.URL
	_, err := s.Register(context.Background(), &semantichubgen.RegisterPayload{
		Name:      "admission-url-bad",
		Kind:      "shapes",
		MediaType: "text/turtle",
		SourceURL: &source,
	})
	requireBadRequest(t, err, "not valid Turtle")
}
