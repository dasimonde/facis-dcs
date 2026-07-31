package compiler

import (
	"bytes"
	"testing"
)

func TestExtractDocumentModelWithBareUUIDIDs(t *testing.T) {
	payload := []byte(`{
  "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "odrl": "http://www.w3.org/ns/odrl/2/", "xsd": "http://www.w3.org/2001/XMLSchema#"},
  "@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3",
  "@type": "dcs:ContractTemplate",
  "dcs:contractData": [],
  "dcs:documentStructure": {
    "@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#document-structure",
    "@type": "dcs:DocumentStructure",
    "dcs:blocks": {"@list": [
      {"@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#block-9c33cbb9-6f61-4f21-b8b5-38e4939c0069", "@type": "dcs:Section"},
      {"@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#block-c68431a8-a91d-471b-8e53-1d83ad5f3e39", "@type": "dcs:TextBlock", "dcs:text": "fdsfsdfds"}
    ]},
    "dcs:layout": [
      {"@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#block-715bc2aa-83e9-4347-8de3-fdd2f9592155", "@type": "dcs:LayoutNode",
       "dcs:children": {"@list": [{"@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#block-9c33cbb9-6f61-4f21-b8b5-38e4939c0069"}]}, "dcs:isRoot": true},
      {"@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#block-9c33cbb9-6f61-4f21-b8b5-38e4939c0069", "@type": "dcs:LayoutNode",
       "dcs:children": {"@list": [{"@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#block-c68431a8-a91d-471b-8e53-1d83ad5f3e39"}]}}
    ]
  },
  "dcs:metadata": {"@id": "b1d0010b-52a1-4bc9-bc07-7934272767e3#metadata", "@type": "dcs:TemplateMetadata", "dcs:templateType": "dcs:Component", "dcs:title": "title123"},
  "dcs:policies": []
}`)

	doc := mustExtractFromPayload(t, payload)
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Heading != "" {
		t.Fatalf("expected empty section heading, got %q", doc.Sections[0].Heading)
	}

	var prose []string
	for _, section := range doc.Sections {
		for _, clause := range section.Clauses {
			for _, segment := range clause.Segments {
				if segment.Type == "prose" && segment.Text != "" {
					prose = append(prose, segment.Text)
				}
			}
		}
	}

	if len(prose) != 1 {
		t.Fatalf("expected exactly 1 prose segment, got %d (%v)", len(prose), prose)
	}
	if prose[0] != "fdsfsdfds" {
		t.Fatalf("expected prose segment %q, got %q", "fdsfsdfds", prose[0])
	}

	pdfBytes, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatalf("renderPDF: %v", err)
	}
	if !bytes.Contains(pdfBytes, []byte("fdsfsdfds")) {
		t.Fatalf("expected rendered PDF bytes to contain %q", "fdsfsdfds")
	}
}
