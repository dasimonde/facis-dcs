package design

import (
	"reflect"
	"testing"

	"goa.design/goa/v3/codegen"
	"goa.design/goa/v3/http/codegen/openapi"
	openapiv2 "goa.design/goa/v3/http/codegen/openapi/v2"
	openapiv3 "goa.design/goa/v3/http/codegen/openapi/v3"
)

func TestResponseContentGenerationPreservesErrors(t *testing.T) {
	extensions := map[string]any{responseContentExtension: map[string]any{
		"application/json": map[string]any{"type": "object"},
		"text/csv":         map[string]any{"type": "string", "format": "binary"},
		"application/pdf":  map[string]any{"type": "string", "format": "binary"},
	}}
	v3Success := &openapiv3.Response{Extensions: extensions}
	v3Error := &openapiv3.Response{Content: map[string]*openapiv3.MediaType{"application/json": {Schema: &openapi.Schema{Type: "object"}}}}
	v3 := &openapiv3.OpenAPI{Paths: map[string]*openapiv3.PathItem{"/report": {Get: &openapiv3.Operation{Responses: map[string]*openapiv3.ResponseRef{
		"200": {Value: v3Success}, "500": {Value: v3Error},
	}}}}}
	v2Success := &openapiv2.Response{Extensions: extensions}
	v2Error := &openapiv2.Response{Schema: &openapi.Schema{Type: "object"}}
	v2Op := &openapiv2.Operation{Responses: map[string]*openapiv2.Response{"200": v2Success, "500": v2Error}}
	v2 := &openapiv2.V2{Paths: map[string]any{"/report": &openapiv2.Path{Get: v2Op}}}
	files := []*codegen.File{{SectionTemplates: []*codegen.SectionTemplate{{Data: v3}, {Data: v3}, {Data: v2}, {Data: v2}}}}
	if _, err := generateResponseContent("", nil, files); err != nil {
		t.Fatal(err)
	}
	if len(v3Success.Content) != 3 || v3Success.Content["application/json"].Schema.Type != "object" || v3Success.Content["application/pdf"].Schema.Format != "binary" || v3Success.Content["text/csv"].Schema.Format != "binary" {
		t.Fatalf("incorrect response content: %+v", v3Success.Content)
	}
	if !reflect.DeepEqual(v2Op.Produces, []string{"application/json", "application/pdf", "text/csv"}) || v2Success.Schema.Type != "file" {
		t.Fatalf("incorrect OAS2 download: %+v", v2Op)
	}
	if len(v3Error.Content) != 1 || v2Error.Schema.Type != "object" {
		t.Fatal("error schemas changed")
	}
}

func TestResponseContentRejectsInvalidDesignExtension(t *testing.T) {
	for _, value := range []any{"not an object", map[string]any{}, map[string]any{"text/csv": nil}} {
		if _, err := responseContentSchemas(map[string]any{responseContentExtension: value}); err == nil {
			t.Fatalf("accepted invalid metadata: %#v", value)
		}
	}
}
