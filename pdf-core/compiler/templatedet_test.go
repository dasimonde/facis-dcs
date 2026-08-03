package compiler

import (
	"testing"
)

// A template/component whose documentStructure has a SINGLE layout node (and a
// single block) serializes them as objects, not 1-element arrays — JSON-LD's
// single-vs-array ambiguity. pdf-core must accept both shapes.
// The REAL DCS shape (see backend flatten_test.go): a single layout node
// (serialized as a bare object, not a 1-element array) whose dcs:children is an
// @list of @id-REFERENCE OBJECTS ({"@id":"c1"}), not bare strings — the model's
// LayoutNode.Children is []string, so both the cardinality AND the element shape
// must be normalized.
const templateSingleNodePayload = `{
  "@type":"dcs:ContractTemplate",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Single Node Template"},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:t#s1"}]}},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Section","@id":"urn:t#s1","dcs:title":"1. Payment"},
      {"@type":"dcs:Clause","@id":"urn:t#c1","dcs:content":["The amount is ","_____","."]}
    ]}
  }
}`

func TestTemplateSingleLayoutNodeCompiles(t *testing.T) {
	_, err := CompilePDF(WithSigner(testChainContext(), NewCapturingSigner()), []byte(templateSingleNodePayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("template with single layout node must compile: %v", err)
	}
}

func TestTemplateDoubleCompileDeterministic(t *testing.T) {
	ctx := testChainContext()
	p1, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(templateSingleNodePayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		p2, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(templateSingleNodePayload), CanonicalCompiledAt)
		if err != nil {
			t.Fatal(err)
		}
		if err := MatchPageContent(p1, p2); err != nil {
			t.Fatalf("template double-compile page content differs (run %d): %v", i, err)
		}
	}
}
