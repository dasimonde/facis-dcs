package compiler

import (
	"bytes"
	"testing"
)

func TestRenderPDFUsesCborContentBoxForSignature(t *testing.T) {
	doc := documentModel{
		Title: "sig fields",
		Sections: []sectionData{
			{Clauses: []clauseData{
				{Segments: []clauseSegment{{Type: "prose", Text: "Clause text."}}},
			}},
		},
		SignatureFields: []sigFieldDef{{Name: "SignerOne", Label: "Signer One"}},
		NamespaceMap:    map[string]string{},
		EmbeddedPayload:   []byte(`{}`),
		PayloadHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		FileID:          "0123456789abcdef0123456789abcdef",
	}

	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	contentCredential, ok := extractEmbeddedStreamByFileSpecNameForTest(pdf, "content_credential.c2pa")
	if !ok {
		t.Fatalf("embedded C2PA stream not found")
	}

	label := []byte("c2pa.signature")
	labelPos := -1
	searchStart := 0
	for {
		idx := bytes.Index(contentCredential[searchStart:], label)
		if idx < 0 {
			break
		}
		idx += searchStart
		if idx >= 25 && idx >= 21 && string(contentCredential[idx-21:idx-17]) == "jumd" {
			labelPos = idx
			break
		}
		searchStart = idx + 1
	}
	if labelPos < 0 {
		t.Fatalf("signature JUMBF label not found")
	}
	descStart := labelPos - 25
	descSize := int(bytesToUint32ForTest(contentCredential[descStart : descStart+4]))
	if descStart+descSize > len(contentCredential) {
		t.Fatalf("signature description box is truncated")
	}
	contentStart := descStart + descSize
	if contentStart+8 > len(contentCredential) {
		t.Fatalf("signature content box is truncated")
	}
	contentBoxType := string(contentCredential[contentStart+4 : contentStart+8])
	if contentBoxType != "cbor" {
		t.Fatalf("signature content box type = %q, want %q", contentBoxType, "cbor")
	}
	if contentStart+10 > len(contentCredential) {
		t.Fatalf("signature content payload is truncated")
	}
	contentBoxPayload := contentCredential[contentStart+8 : contentStart+10]
	if !bytes.Equal(contentBoxPayload, []byte{0xD2, 0x84}) {
		t.Fatalf("signature content payload does not look like a COSE_Sign1 tag")
	}
}

func TestExtractDocumentModelParsesSignatureFields(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:sig-fields-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Sig Fields Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:sig-fields-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:sig-fields-test#s1", "children": ["urn:doc:sig-fields-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:sig-fields-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:sig-fields-test#c1", "content": ["Clause."]}
			]
		},
		"signatureFields": [
			{"@type": "SignatureField", "@id": "urn:doc:sig-fields-test#SignerOne", "signatoryName": "SignerOne"},
			{"@type": "SignatureField", "@id": "urn:doc:sig-fields-test#SignerTwo", "signatoryName": "SignerTwo"}
		]
	}`)

	model := mustExtractFromPayload(t, payload)

	if len(model.SignatureFields) != 2 {
		t.Fatalf("signature field count = %d, want 2", len(model.SignatureFields))
	}
	if model.SignatureFields[0].Name != "SignerOne" || model.SignatureFields[0].Label != "SignerOne" {
		t.Fatalf("first signature field = %#v", model.SignatureFields[0])
	}
	if model.SignatureFields[1].Name != "SignerTwo" || model.SignatureFields[1].Label != "SignerTwo" {
		t.Fatalf("second signature field = %#v", model.SignatureFields[1])
	}
}

// TestExtractDocumentModelParsesSignatureFieldWithTitle verifies that when
// dcs:title is present alongside dcs:signatoryName, the title is used as the
// human-readable display label while signatoryName remains the AcroForm /T value.
func TestExtractDocumentModelParsesSignatureFieldWithTitle(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:sig-with-title",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Sig With Title"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:sig-with-title#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:sig-with-title#s1", "children": ["urn:doc:sig-with-title#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:sig-with-title#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:sig-with-title#c1", "content": ["Clause."]}
			]
		},
		"signatureFields": [
			{"@type": "SignatureField", "@id": "urn:doc:sig-with-title#sig-provider", "signatoryName": "sig-provider", "title": "Service Provider"}
		]
	}`)

	model := mustExtractFromPayload(t, payload)

	if len(model.SignatureFields) != 1 {
		t.Fatalf("signature field count = %d, want 1", len(model.SignatureFields))
	}
	if model.SignatureFields[0].Name != "sig-provider" {
		t.Errorf("Name = %q, want %q", model.SignatureFields[0].Name, "sig-provider")
	}
	if model.SignatureFields[0].Label != "Service Provider" {
		t.Errorf("Label = %q, want %q", model.SignatureFields[0].Label, "Service Provider")
	}
}

func TestRenderPDFIncludesAcroFormAndSigWidgets(t *testing.T) {
	doc := documentModel{
		Title: "sig fields",
		Sections: []sectionData{
			{Clauses: []clauseData{
				{Segments: []clauseSegment{{Type: "prose", Text: "Clause text."}}},
			}},
		},
		SignatureFields: []sigFieldDef{
			{Name: "SignerOne", Label: "Signer One"},
			{Name: "SignerTwo", Label: "Signer Two"},
		},
		NamespaceMap:  map[string]string{},
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		FileID:        "0123456789abcdef0123456789abcdef",
	}

	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}

	for _, marker := range [][]byte{
		[]byte("/AcroForm"),
		[]byte("/FT /Sig"),
		[]byte("/T (SignerOne)"),
		[]byte("/T (SignerTwo)"),
	} {
		if !bytes.Contains(pdf, marker) {
			t.Fatalf("PDF missing marker %q", string(marker))
		}
	}
}

// TestOutlineDestinationUsesLeftMarginX verifies that PDF outline entries use
// the left margin X coordinate (54.00) in their /XYZ destination, not 0.
func TestOutlineDestinationUsesLeftMarginX(t *testing.T) {
	doc := sectionDoc([]sectionData{{
		Heading: "1. Test",
		Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Clause."}}}},
	}})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/XYZ 54.00")) {
		t.Error("PDF outline destination must use /XYZ 54.00 (left margin), not /XYZ 0")
	}
}

func extractEmbeddedStreamByFileSpecNameForTest(pdf []byte, name string) ([]byte, bool) {
	needle := []byte("/F (" + name + ")")
	pos := bytes.Index(pdf, needle)
	if pos < 0 {
		return nil, false
	}
	efPos := bytes.Index(pdf[pos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, false
	}
	efPos += pos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, false
	}
	objNumber := pdf[efPos : efPos+refEnd]
	objMarker := append([]byte(string(objNumber)), []byte(" 0 obj")...)
	objPos := bytes.Index(pdf, objMarker)
	if objPos < 0 {
		return nil, false
	}
	streamPos := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamPos < 0 {
		return nil, false
	}
	streamPos += objPos + len("stream\n")
	endPos := bytes.Index(pdf[streamPos:], []byte("\nendstream"))
	if endPos < 0 {
		return nil, false
	}
	return pdf[streamPos : streamPos+endPos], true
}

func bytesToUint32ForTest(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
