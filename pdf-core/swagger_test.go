package main

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// Minimal types for parsing an OpenAPI 3.0 document.

type swaggerDoc struct {
	Paths map[string]map[string]*swaggerOp `yaml:"paths"`
}

type swaggerOp struct {
	Parameters  []swaggerParam  `yaml:"parameters"`
	RequestBody *swaggerReqBody `yaml:"requestBody"`
}

type swaggerParam struct {
	Name    string      `yaml:"name"`
	In      string      `yaml:"in"`
	Example interface{} `yaml:"example"`
}

type swaggerReqBody struct {
	Required bool                      `yaml:"required"`
	Content  map[string]*swaggerMedia  `yaml:"content"`
}

type swaggerMedia struct {
	Schema  map[string]interface{} `yaml:"schema"`
	Example interface{}            `yaml:"example"`
}

func loadSwagger(t *testing.T) swaggerDoc {
	t.Helper()
	data, err := os.ReadFile("api/openapi3.yaml")
	if err != nil {
		t.Fatalf("read api/openapi3.yaml: %v", err)
	}
	var doc swaggerDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse swagger: %v", err)
	}
	return doc
}

// TestSwaggerPostEndpointsHaveRequestBody verifies that every POST endpoint
// documents a requestBody with the correct primary content type.
func TestSwaggerPostEndpointsHaveRequestBody(t *testing.T) {
	doc := loadSwagger(t)

	endpoints := []struct {
		path        string
		contentType string
	}{
		{"/download", "application/ld+json"},
		{"/verify", "application/pdf"},
		{"/update", "multipart/form-data"},
		{"/claim", "multipart/form-data"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			pathItem, ok := doc.Paths[ep.path]
			if !ok {
				t.Fatalf("path %q not found in swagger", ep.path)
			}
			op, ok := pathItem["post"]
			if !ok {
				t.Fatalf("no POST operation at %q", ep.path)
			}
			if op.RequestBody == nil {
				t.Fatalf("POST %q has no requestBody", ep.path)
			}
			if op.RequestBody.Content[ep.contentType] == nil {
				t.Fatalf("POST %q requestBody missing content type %q; have: %v",
					ep.path, ep.contentType, keys(op.RequestBody.Content))
			}
		})
	}
}

// TestSwaggerUpdateMultipartHasPdfAndPayloadFields verifies that /update
// documents multipart/form-data fields "pdf" and "payload".
func TestSwaggerUpdateMultipartHasPdfAndPayloadFields(t *testing.T) {
	doc := loadSwagger(t)

	pathItem := doc.Paths["/update"]
	if pathItem == nil {
		t.Fatal("/update path not in swagger")
	}
	op := pathItem["post"]
	if op == nil || op.RequestBody == nil {
		t.Fatal("POST /update has no requestBody")
	}
	media := op.RequestBody.Content["multipart/form-data"]
	if media == nil {
		t.Fatal("POST /update requestBody missing multipart/form-data content")
	}
	props, ok := media.Schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("multipart/form-data schema has no properties; schema: %v", media.Schema)
	}
	for _, field := range []string{"pdf", "payload"} {
		if _, ok := props[field]; !ok {
			t.Errorf("multipart/form-data schema missing field %q", field)
		}
	}
}

// TestSwaggerContentTypeExamplesAreValidMediaTypes checks that Content-Type
// header parameter examples are valid MIME types, not generated Lorem Ipsum.
func TestSwaggerContentTypeExamplesAreValidMediaTypes(t *testing.T) {
	doc := loadSwagger(t)

	for path, pathItem := range doc.Paths {
		op, ok := pathItem["post"]
		if !ok {
			continue
		}
		for _, param := range op.Parameters {
			if param.Name != "Content-Type" {
				continue
			}
			example, ok := param.Example.(string)
			if !ok || example == "" {
				continue // no example set – acceptable
			}
			mediaType, _, err := mime.ParseMediaType(example)
			if err != nil || mediaType == "" {
				t.Errorf("POST %q Content-Type example %q is not a valid media type: %v",
					path, example, err)
			}
		}
	}
}

// TestDownloadBodyIsJSONNotBinary verifies that the /download requestBody schema
// does not carry format: binary (which makes Swagger UI render a file picker
// instead of a JSON text editor).
func TestDownloadBodyIsJSONNotBinary(t *testing.T) {
	doc := loadSwagger(t)
	op := doc.Paths["/download"]["post"]
	if op == nil || op.RequestBody == nil {
		t.Fatal("POST /download has no requestBody")
	}
	media := op.RequestBody.Content["application/ld+json"]
	if media == nil {
		t.Fatal("POST /download requestBody missing application/ld+json content")
	}
	if fmt, ok := media.Schema["format"]; ok && fmt == "binary" {
		t.Error("POST /download schema has format: binary — Swagger UI will render a file picker instead of a text editor")
	}
}

// TestDownloadHasNoContentTypeHeaderParam verifies that the Content-Type header
// is not exposed as a manual parameter on /download — the requestBody content
// type already communicates it.
func TestDownloadHasNoContentTypeHeaderParam(t *testing.T) {
	doc := loadSwagger(t)
	op := doc.Paths["/download"]["post"]
	if op == nil {
		t.Fatal("POST /download not found")
	}
	for _, p := range op.Parameters {
		if p.Name == "Content-Type" {
			t.Error("POST /download exposes Content-Type as a header parameter; it should be implicit from the requestBody")
		}
	}
}

// TestDownloadRequestBodyHasExample verifies that /download carries an example
// JSON-LD payload so Swagger UI users know what to submit.
func TestDownloadRequestBodyHasExample(t *testing.T) {
	doc := loadSwagger(t)
	op := doc.Paths["/download"]["post"]
	if op == nil || op.RequestBody == nil {
		t.Fatal("POST /download has no requestBody")
	}
	media := op.RequestBody.Content["application/ld+json"]
	if media == nil {
		t.Fatal("POST /download requestBody missing application/ld+json content")
	}
	if media.Example == nil {
		t.Error("POST /download application/ld+json requestBody has no example")
	}
}

// TestSwaggerServerURLRespectsPublicURL verifies that GET /swagger.json
// substitutes the hardcoded localhost URL with DCS_PDF_CORE_PUBLIC_URL when set.
func TestSwaggerServerURLRespectsPublicURL(t *testing.T) {
	const publicURL = "https://abc123.ngrok-free.app"
	t.Setenv("DCS_PDF_CORE_PUBLIC_URL", publicURL)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger.json", nil)
	newServer().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /swagger.json returned %d", rec.Code)
	}

	var doc struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
		t.Fatalf("decode swagger.json: %v", err)
	}
	if len(doc.Servers) == 0 {
		t.Fatal("swagger.json has no servers")
	}
	if got := doc.Servers[0].URL; got != publicURL {
		t.Errorf("servers[0].url = %q, want %q", got, publicURL)
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
