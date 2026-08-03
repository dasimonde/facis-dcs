package compiler

import (
	"strings"
	"testing"
)

// B's VerifyContent compares A's shipped contract PDF (rendered via updatePDF, the
// amendment path DCS uses to regenerate contracts) against a FRESH CompilePDF of
// the same payload. Their page content must be byte-identical, especially for a
// rich/filled payload — otherwise B rejects with "human-readable does not match".
func TestUpdatePDFPageContentMatchesFreshCompile(t *testing.T) {
	ctx := testChainContext()
	base, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("base compile: %v", err)
	}
	amended := strings.Replace(filledContractPayload, "15000", "12000", 1)
	updated, err := UpdatePDF(WithSigner(ctx, NewCapturingSigner()), base, []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("updatePDF: %v", err)
	}
	reference, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("reference compile: %v", err)
	}
	if err := MatchPageContent(updated, reference); err != nil {
		t.Fatalf("updatePDF page content diverges from fresh CompilePDF (B would reject): %v", err)
	}
}

const richFilledContractPayload = `{
  "@type":"dcs:Contract",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Rich Filled Contract"},
  "dcs:contractFields":{"@list":[
    {"@id":"urn:c#f-amount","@type":"dcs:ContractField","dcs:label":"Amount","dcs:datatype":"xsd:decimal","dcs:required":true,"dcs:value":15000},
    {"@id":"urn:c#f-term","@type":"dcs:ContractField","dcs:label":"Term","dcs:datatype":"xsd:integer","dcs:required":true,"dcs:value":36}
  ]},
  "dcs:contractData":[
    {"@id":"urn:c#payment","@type":"dcs:PaymentClause","dcs:amount":{"@id":"urn:c#f-amount"},"dcs:term":{"@id":"urn:c#f-term"}}
  ],
  "dcs:policies":{"@type":"odrl:Set","odrl:permission":[{"@type":"odrl:Permission","odrl:action":{"@id":"odrl:use"}}]},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@list":[
      {"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:c#s1"},{"@id":"urn:c#s2"}]}},
      {"@type":"dcs:LayoutNode","@id":"urn:c#s1","dcs:children":{"@list":[{"@id":"urn:c#c1"},{"@id":"urn:c#s1a"}]}},
      {"@type":"dcs:LayoutNode","@id":"urn:c#s1a","dcs:children":{"@list":[{"@id":"urn:c#c2"}]}},
      {"@type":"dcs:LayoutNode","@id":"urn:c#s2","dcs:children":{"@list":[{"@id":"urn:c#c3"}]}}
    ]},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Section","@id":"urn:c#s1","dcs:title":"1. Payment"},
      {"@type":"dcs:Clause","@id":"urn:c#c1","dcs:content":["The amount is ",{"@id":"urn:c#f-amount"}," EUR per ",{"@id":"urn:c#f-term"},"."]},
      {"@type":"dcs:Section","@id":"urn:c#s1a","dcs:title":"1.1 Term"},
      {"@type":"dcs:Clause","@id":"urn:c#c2","dcs:content":["The term is ",{"@id":"urn:c#f-term"}," months."]},
      {"@type":"dcs:Section","@id":"urn:c#s2","dcs:title":"2. Governing Law"},
      {"@type":"dcs:Clause","@id":"urn:c#c3","dcs:content":["This agreement is governed by the laws referenced in ",{"@id":"https://w3id.org/facis/dcs/ontology/v1#Jurisdiction","schema:name":"Jurisdiction"},"."]}
    ]}
  },
  "dcs:signatureFields":{"@list":[
    {"@type":"dcs:SignatureField","@id":"urn:c#sig-a","dcs:signatoryName":"did:web:dcs-a"},
    {"@type":"dcs:SignatureField","@id":"urn:c#sig-b","dcs:signatoryName":"did:web:dcs-b"}
  ]}
}`

func TestUpdatePDFRichPageContentMatchesFreshCompile(t *testing.T) {
	ctx := testChainContext()
	base, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(richFilledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("base compile: %v", err)
	}
	amended := strings.Replace(richFilledContractPayload, "15000", "12000", 1)
	updated, err := UpdatePDF(WithSigner(ctx, NewCapturingSigner()), base, []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("updatePDF: %v", err)
	}
	reference, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(amended), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("reference compile: %v", err)
	}
	if err := MatchPageContent(updated, reference); err != nil {
		t.Fatalf("RICH updatePDF page content diverges from fresh CompilePDF (B rejects): %v", err)
	}
}

func TestStackedAmendmentsMatchFreshCompile(t *testing.T) {
	ctx := testChainContext()
	v1 := richFilledContractPayload
	v2 := strings.Replace(v1, "15000", "10000", 1)
	v3 := strings.Replace(v1, "15000", "12500", 1)
	base, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(v1), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	u1, err := UpdatePDF(WithSigner(ctx, NewCapturingSigner()), base, []byte(v2), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	u2, err := UpdatePDF(WithSigner(ctx, NewCapturingSigner()), u1, []byte(v3), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(v3), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := MatchPageContent(u2, ref); err != nil {
		t.Fatalf("stacked amendments diverge from fresh compile (B rejects): %v", err)
	}
}
