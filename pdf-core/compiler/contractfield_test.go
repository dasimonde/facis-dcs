package compiler

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// cleanContractFieldPayload is the ContractField model clean form as it reaches pdf-core after
// the DCS runs inlineContractFieldRenderText: contractData is a flat list of typed
// dcs:ContractField nodes; each clause references a field by a bare {"@id"}
// node onto which the DCS has copied the field's dcs:label and (once
// filled) its dcs:value. The renderer must show the filled value, never the @id.
const cleanContractFieldPayload = `{
  "@type":"dcs:Contract",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Clean ContractField Contract"},
  "dcs:contractFields":{"@list":[
    {"@id":"urn:c#f-amount","@type":"dcs:ContractField","dcs:label":"Amount","dcs:datatype":"xsd:decimal","dcs:value":15000},
    {"@id":"urn:c#f-term","@type":"dcs:ContractField","dcs:label":"Term","dcs:datatype":"xsd:integer","dcs:value":36}
  ]},
  "dcs:policies":{"@type":"odrl:Set","odrl:permission":[{"@type":"odrl:Permission","odrl:action":{"@id":"odrl:use"}}]},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:c#c1"}]}},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Clause","@id":"urn:c#c1","dcs:content":["The amount is ",{"@id":"urn:c#f-amount"}," EUR per ",{"@id":"urn:c#f-term"}," months."]}
    ]}
  }
}`

// cleanUnfilledPayload is the template form: the field carries a label but
// no value (unfilled). The renderer must show the empty slot, never the @id.
const cleanUnfilledPayload = `{
  "@type":"dcs:Contract",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Clean ContractField Template"},
  "dcs:contractFields":{"@list":[
    {"@id":"urn:c#f-amount","@type":"dcs:ContractField","dcs:label":"Amount","dcs:datatype":"xsd:decimal"}
  ]},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:c#c1"}]}},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Clause","@id":"urn:c#c1","dcs:content":["The amount is ",{"@id":"urn:c#f-amount"}," EUR."]}
    ]}
  }
}`

func TestCleanContractFieldRendersFilledValue(t *testing.T) {
	pdf, err := CompilePDF(WithSigner(context.Background(), NewCapturingSigner()), []byte(cleanContractFieldPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("clean field contract must compile: %v", err)
	}
	text := renderedText(t, pdf)
	if !bytes.Contains(text, []byte("15000")) {
		t.Fatalf("filled field value 15000 not rendered:\n%s", text)
	}
	if !bytes.Contains(text, []byte("36")) {
		t.Fatalf("filled field value 36 not rendered:\n%s", text)
	}
	if bytes.Contains(text, []byte("urn:c#f-amount")) {
		t.Fatalf("field @id leaked into the visible render instead of its value:\n%s", text)
	}
}

func TestCleanContractFieldUnfilledRendersEmptySlot(t *testing.T) {
	pdf, err := CompilePDF(WithSigner(context.Background(), NewCapturingSigner()), []byte(cleanUnfilledPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("clean field template must compile: %v", err)
	}
	text := renderedText(t, pdf)
	if bytes.Contains(text, []byte("urn:c#f-amount")) {
		t.Fatalf("unfilled field @id leaked into the visible render:\n%s", text)
	}
	if !bytes.Contains(text, []byte("_____")) {
		t.Fatalf("unfilled field did not render the empty slot:\n%s", text)
	}
}

// TestCleanContractFieldUpdateMatchesFreshCompile is the determinism contract for
// field-bearing documents: an amended contract regenerated via UpdatePDF
// must have byte-identical page content to a fresh CompilePDF of the same
// payload (otherwise a peer's /verify rejects with a content mismatch).
func TestCleanContractFieldUpdateMatchesFreshCompile(t *testing.T) {
	ctx := context.Background()
	base, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(cleanContractFieldPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("base compile: %v", err)
	}
	amended := strings.Replace(cleanContractFieldPayload, "15000", "12000", -1)
	updated, err := UpdatePDF(WithSigner(ctx, NewCapturingSigner()), base, []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("updatePDF: %v", err)
	}
	reference, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("reference compile: %v", err)
	}
	if err := MatchPageContent(updated, reference); err != nil {
		t.Fatalf("clean field updatePDF page content diverges from fresh CompilePDF: %v", err)
	}
}

// cleanContractFieldVerifyBase mirrors the minimal update-test payload conventions
// (bare @vocab terms, top-level @id) but carries a clean ContractField model field in
// both contractData and the clause content, so the byte-prefix /verify contract
// is exercised with a filled field present.
const cleanContractFieldVerifyBase = `{
  "@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#", "dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@id": "urn:doc:clean-ph",
  "@type": "ContractTemplate",
  "metadata": {"@type": "TemplateMetadata", "title": "Clean PH Verify"},
  "contractFields": [
    {"@id":"urn:doc:clean-ph#f-amount","@type":"ContractField","label":"Amount","datatype":"xsd:decimal","value":15000}
  ],
  "documentStructure": {
    "@type": "DocumentStructure",
    "layout": [
      {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:clean-ph#s1"]},
      {"@type": "LayoutNode", "@id": "urn:doc:clean-ph#s1", "children": ["urn:doc:clean-ph#c1"]}
    ],
    "blocks": [
      {"@type": "Section", "@id": "urn:doc:clean-ph#s1", "title": "1. Terms"},
      {"@type": "Clause", "@id": "urn:doc:clean-ph#c1", "content": ["The amount is ", {"@id":"urn:doc:clean-ph#f-amount","label":"Amount","value":15000}, " EUR."]}
    ]
  }
}`

// TestCleanContractDataPassesSHACL proves the /render SHACL gate accepts the
// clean ContractField model format: a flat dcs:contractFields list of typed dcs:ContractField
// nodes (this is the exact shape that used to 400 with a ClassConstraintComponent
// violation because the LinkML shape declared contractData items as
// dcs:ContractField). dcs:value is numeric, which must validate against the
// open ContractField shape.
func TestCleanContractDataPassesSHACL(t *testing.T) {
	loadSHACLForTest(t)
	clean := []byte(`{
		"@context": {"dcs":"https://w3id.org/facis/dcs/ontology/v1#","xsd":"http://www.w3.org/2001/XMLSchema#"},
		"@id":"urn:doc:clean-shacl",
		"@type":"dcs:ContractTemplate",
		"dcs:metadata":{"@id":"urn:doc:clean-shacl#m","@type":"dcs:TemplateMetadata","dcs:title":"Clean SHACL"},
		"dcs:contractFields":[
			{"@id":"urn:doc:clean-shacl#f-amount","@type":"dcs:ContractField","dcs:label":"Payment Amount","dcs:datatype":"xsd:decimal","dcs:shape":{"@id":"urn:shape:PaymentClauseShape"},"dcs:required":true,"dcs:value":15000}
		],
		"dcs:documentStructure":{"@id":"urn:doc:clean-shacl#ds","@type":"dcs:DocumentStructure",
			"dcs:blocks":{"@list":[{"@id":"urn:doc:clean-shacl#c1","@type":"dcs:Clause","dcs:content":{"@list":["Amount ",{"@id":"urn:doc:clean-shacl#f-amount"}]}}]},
			"dcs:layout":[{"@id":"urn:doc:clean-shacl#root","@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:doc:clean-shacl#c1"}]}}]
		}
	}`)
	if err := ValidatePayloadSHACL(clean); err != nil {
		t.Fatalf("clean dcs:ContractField contractData must pass SHACL, got: %v", err)
	}
}

// TestCleanContractFieldRecompileMatchesPrefix is the /verify contract: the
// original compile of a field document must reproduce byte-for-byte from
// its own embedded payload across an amendment chain.
func TestCleanContractFieldRecompileMatchesPrefix(t *testing.T) {
	base, err := CompilePDF(testSigningContext(), []byte(cleanContractFieldVerifyBase), time.Now())
	if err != nil {
		t.Fatalf("base compile: %v", err)
	}
	amended := strings.Replace(cleanContractFieldVerifyBase, "15000", "12000", -1)
	updated, err := UpdatePDF(testSigningContext(), base, []byte(amended), time.Now())
	if err != nil {
		t.Fatalf("updatePDF: %v", err)
	}
	if _, err := VerifyIncrementalUpdate(testSigningContext(), updated); err != nil {
		t.Fatalf("VerifyIncrementalUpdate rejected an honestly amended field PDF: %v", err)
	}
	// The recompiled visible content must carry the filled value, not the @id.
	if !bytes.Contains(renderedText(t, updated), []byte("12000")) {
		t.Fatalf("amended field value 12000 not rendered")
	}
}
