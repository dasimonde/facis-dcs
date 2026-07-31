package compiler

import (
	"encoding/json"
	"testing"
)

// TestCanonicalizePayload_ListPropertiesAreAlwaysArrays verifies that after
// stableCtx is annotated with @container:@list, single-item list properties
// do NOT collapse to scalars and explicit @list wrappers are normalized away.
func TestCanonicalizePayload_ListPropertiesAreAlwaysArrays(t *testing.T) {
	// Single content item would previously collapse to a scalar string.
	raw := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:array-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Array Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:array-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:array-test#s1", "children": ["urn:doc:array-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:array-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:array-test#c1", "content": ["Single item."]}
			]
		}
	}`)
	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(canonical, &doc); err != nil {
		t.Fatalf("unmarshal canonical: %v", err)
	}

	ds, _ := doc["documentStructure"].(map[string]any)
	if ds == nil {
		t.Fatal("documentStructure missing from canonical")
	}

	// children of each layout node must be an array, not a plain string
	layout, _ := ds["layout"].([]any)
	if layout == nil {
		t.Fatal("layout must be a JSON array in canonical form")
	}
	for i, item := range layout {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if ch, exists := node["children"]; exists {
			if _, isString := ch.(string); isString {
				t.Errorf("layout[%d].children is a scalar string; must be an array after @container:@list", i)
			}
		}
	}

	// content of each clause must be an array, not a plain string
	blocks, _ := ds["blocks"].([]any)
	if blocks == nil {
		t.Fatal("blocks must be a JSON array in canonical form")
	}
	for i, item := range blocks {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if ct, exists := node["content"]; exists {
			if _, isString := ct.(string); isString {
				t.Errorf("blocks[%d].content is a scalar string; must be an array after @container:@list", i)
			}
		}
	}
}

// TestCanonicalizePayload_ExplicitListBecomesPlainArray verifies that explicit
// {"@list": [...]} syntax in the input normalizes to a plain JSON array in the
// canonical form when @container:@list is in the stable context.
func TestCanonicalizePayload_ExplicitListBecomesPlainArray(t *testing.T) {
	raw := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:explicit-list-canon",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Explicit List Canon"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": {"@list": [
				{"@type": "LayoutNode", "isRoot": true, "children": {"@list": ["urn:doc:explicit-list-canon#s1"]}},
				{"@type": "LayoutNode", "@id": "urn:doc:explicit-list-canon#s1",
				 "children": {"@list": ["urn:doc:explicit-list-canon#c1"]}}
			]},
			"blocks": {"@list": [
				{"@type": "Section", "@id": "urn:doc:explicit-list-canon#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:explicit-list-canon#c1",
				 "content": {"@list": ["Explicit list item."]}}
			]}
		}
	}`)
	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(canonical, &doc); err != nil {
		t.Fatalf("unmarshal canonical: %v", err)
	}
	ds, _ := doc["documentStructure"].(map[string]any)
	if ds == nil {
		t.Fatal("documentStructure missing")
	}
	// blocks must be a plain JSON array, not {"@list": [...]}
	if _, isMap := ds["blocks"].(map[string]any); isMap {
		t.Error("blocks is a @list-wrapped object; canonical form must normalize to a plain array")
	}
	if _, isArr := ds["blocks"].([]any); !isArr {
		t.Error("blocks must be a plain JSON array in canonical form")
	}
	// layout must be a plain JSON array
	if _, isMap := ds["layout"].(map[string]any); isMap {
		t.Error("layout is a @list-wrapped object; canonical form must normalize to a plain array")
	}
	if _, isArr := ds["layout"].([]any); !isArr {
		t.Error("layout must be a plain JSON array in canonical form")
	}
}

// TestUnmarshalCanonicalIntoTypedStruct verifies that the canonical JSON-LD
// form unmarshals cleanly into ContractTemplate typed Go structs.
func TestUnmarshalCanonicalIntoTypedStruct(t *testing.T) {
	raw := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:struct-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Struct Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:struct-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:struct-test#s1", "children": ["urn:doc:struct-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:struct-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:struct-test#c1", "content": ["A clause text."]}
			]
		},
		"signatureFields": [
			{"@type": "SignatureField", "@id": "urn:doc:struct-test#sig1", "signatoryName": "Signer1"}
		]
	}`)
	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}

	var tmpl ContractTemplate
	if err := json.Unmarshal(canonical, &tmpl); err != nil {
		t.Fatalf("unmarshal into ContractTemplate: %v", err)
	}

	if tmpl.ID != "urn:doc:struct-test" {
		t.Errorf("ID = %q, want %q", tmpl.ID, "urn:doc:struct-test")
	}
	if tmpl.Metadata == nil || tmpl.Metadata.Title != "Struct Test" {
		t.Errorf("Metadata.Title = %v", tmpl.Metadata)
	}
	if tmpl.DocumentStructure == nil {
		t.Fatal("DocumentStructure is nil")
	}
	if len(tmpl.DocumentStructure.Layout) != 2 {
		t.Errorf("Layout len = %d, want 2", len(tmpl.DocumentStructure.Layout))
	}
	if len(tmpl.DocumentStructure.Blocks) != 2 {
		t.Errorf("Blocks len = %d, want 2", len(tmpl.DocumentStructure.Blocks))
	}
	if len(tmpl.SignatureFields) != 1 {
		t.Errorf("SignatureFields len = %d, want 1", len(tmpl.SignatureFields))
	}
	if tmpl.SignatureFields[0].SignatoryName != "Signer1" {
		t.Errorf("SignatureFields[0].SignatoryName = %q", tmpl.SignatureFields[0].SignatoryName)
	}

	// Layout node children must be string slices
	root := tmpl.DocumentStructure.Layout[0]
	if !root.IsRoot {
		t.Error("first layout node must be root")
	}
	if len(root.Children) != 1 || root.Children[0] != "urn:doc:struct-test#s1" {
		t.Errorf("root.Children = %v", root.Children)
	}
}
