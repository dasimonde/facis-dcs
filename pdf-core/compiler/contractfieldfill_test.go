package compiler

import (
	"bytes"
	"testing"
)

// The filled-contract shape declares a ContractField with its negotiated value;
// the clause references that field by @id. An ODRL policy sits alongside
// and MUST be ignored by the render (attachment-only) but preserved verbatim.
const filledContractPayload = `{
  "@type":"dcs:Contract",
  "dcs:metadata":{"@type":"dcs:TemplateMetadata","dcs:title":"Filled Contract"},
  "dcs:contractFields":{"@list":[
    {"@id":"urn:c#field-amount","@type":"dcs:ContractField","dcs:label":"Amount","dcs:datatype":"xsd:decimal","dcs:required":true,"dcs:value":15000}
  ]},
  "dcs:contractData":[{"@id":"urn:c#payment","@type":"dcs:PaymentClause","dcs:amount":{"@id":"urn:c#field-amount"}}],
  "dcs:policies":{"@type":"odrl:Set","odrl:permission":[{"@type":"odrl:Permission","odrl:action":{"@id":"odrl:use"}}]},
  "dcs:documentStructure":{"@type":"dcs:DocumentStructure",
    "dcs:layout":{"@type":"dcs:LayoutNode","dcs:isRoot":true,"dcs:children":{"@list":[{"@id":"urn:c#c1"}]}},
    "dcs:blocks":{"@list":[
      {"@type":"dcs:Clause","@id":"urn:c#c1","dcs:content":["Amount ",{"@id":"urn:c#field-amount"}," EUR"]}
    ]}
  }
}`

func renderedText(t *testing.T, pdf []byte) []byte {
	t.Helper()
	streams, err := extractPageContentStreams(pdf)
	if err != nil {
		t.Fatalf("extract page content: %v", err)
	}
	var all []byte
	for _, s := range streams {
		all = append(all, s...)
	}
	return all
}

func TestContractRendersFilledContractFieldValue(t *testing.T) {
	pdf, err := CompilePDF(WithSigner(testChainContext(), NewCapturingSigner()), []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("filled contract must compile: %v", err)
	}
	text := renderedText(t, pdf)
	if !bytes.Contains(text, []byte("15000")) {
		t.Fatalf("filled contract field value 15000 not rendered (got the empty slot instead)")
	}
	if bytes.Contains(text, []byte("odrl")) || bytes.Contains(text, []byte("Permission")) {
		t.Fatalf("ODRL policy leaked into the visible render")
	}
	embedded, err := ExtractEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("extract embedded: %v", err)
	}
	if !bytes.Contains(embedded, []byte("odrl:Set")) {
		t.Fatalf("ODRL policy must be preserved verbatim in the attachment")
	}
}
