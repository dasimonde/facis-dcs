package compiler

import (
	"bytes"
	"testing"
	"time"
)

const minimalPayloadBase = `{
  "@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#", "dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@id": "urn:doc:update-test",
  "@type": "ContractTemplate",
  "metadata": {"@type": "TemplateMetadata", "title": "Update Test"},
  "documentStructure": {
    "@type": "DocumentStructure",
    "layout": [
      {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:update-test#s1"]},
      {"@type": "LayoutNode", "@id": "urn:doc:update-test#s1", "children": ["urn:doc:update-test#c1"]}
    ],
    "blocks": [
      {"@type": "Section", "@id": "urn:doc:update-test#s1", "title": "1. Terms"},
      {"@type": "Clause", "@id": "urn:doc:update-test#c1", "content": ["Original clause text."]}
    ]
  }
}`

const minimalPayloadAmended = `{
  "@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#", "dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@id": "urn:doc:update-test",
  "@type": "ContractTemplate",
  "metadata": {"@type": "TemplateMetadata", "title": "Update Test"},
  "documentStructure": {
    "@type": "DocumentStructure",
    "layout": [
      {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:update-test#s1"]},
      {"@type": "LayoutNode", "@id": "urn:doc:update-test#s1", "children": ["urn:doc:update-test#c1", "urn:doc:update-test#c2"]}
    ],
    "blocks": [
      {"@type": "Section", "@id": "urn:doc:update-test#s1", "title": "1. Terms"},
      {"@type": "Clause", "@id": "urn:doc:update-test#c1", "content": ["Original clause text."]},
      {"@type": "Clause", "@id": "urn:doc:update-test#c2", "content": ["Newly added clause text."]}
    ]
  }
}`

// TestUpdatePDFProducesIncrementalReplace verifies that UpdatePDF appends a
// PDF incremental update that replaces the visible page content with a freshly
// compiled version of the new payload.
//
// The old behavior was to append a separate "AMENDMENT" page showing the RDF
// diff. The correct behavior is a page recompile: readers see the new content,
// but the original bytes are preserved as a prefix so C2PA / digital signatures
// can still verify over the original byte range.
func TestUpdatePDFProducesIncrementalReplace(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF(base): %v", err)
	}

	result, err := UpdatePDF(testSigningContext(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}

	// Original bytes must be preserved as a prefix (signature invariant).
	if !bytes.HasPrefix(result, original) {
		t.Error("updated PDF must start with the original bytes unchanged")
	}

	increment := result[len(original):]

	// The incremental section must NOT contain an "AMENDMENT" page dump.
	if bytes.Contains(increment, []byte("AMENDMENT")) {
		t.Error("incremental section must not contain an AMENDMENT page; UpdatePDF should recompile pages, not append a diff page")
	}

	// The new clause text must appear in the incremental section as compiled
	// page content, not as part of an amendment annotation.
	if !bytes.Contains(increment, []byte("Newly added clause text.")) {
		t.Error("incremental section must contain new clause text compiled into page content streams")
	}
}

// TestUpdatePDFModifiedClause verifies the behavior when clause text is changed
// rather than added: the original text must still exist in the preserved prefix
// (for provenance) while the incremental section contains only the new text.
func TestUpdatePDFModifiedClause(t *testing.T) {
	base := []byte(`{
  "@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@id": "urn:doc:mod-test",
  "@type": "ContractTemplate",
  "metadata": {"@type": "TemplateMetadata", "title": "Mod Test"},
  "documentStructure": {
    "@type": "DocumentStructure",
    "layout": [
      {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:mod-test#s1"]},
      {"@type": "LayoutNode", "@id": "urn:doc:mod-test#s1", "children": ["urn:doc:mod-test#c1"]}
    ],
    "blocks": [
      {"@type": "Section", "@id": "urn:doc:mod-test#s1", "title": "1. Terms"},
      {"@type": "Clause", "@id": "urn:doc:mod-test#c1", "content": ["The original wording of clause one."]}
    ]
  }
}`)
	modified := []byte(`{
  "@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@id": "urn:doc:mod-test",
  "@type": "ContractTemplate",
  "metadata": {"@type": "TemplateMetadata", "title": "Mod Test"},
  "documentStructure": {
    "@type": "DocumentStructure",
    "layout": [
      {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:mod-test#s1"]},
      {"@type": "LayoutNode", "@id": "urn:doc:mod-test#s1", "children": ["urn:doc:mod-test#c1"]}
    ],
    "blocks": [
      {"@type": "Section", "@id": "urn:doc:mod-test#s1", "title": "1. Terms"},
      {"@type": "Clause", "@id": "urn:doc:mod-test#c1", "content": ["The revised wording of clause one."]}
    ]
  }
}`)

	original, err := CompilePDF(testSigningContext(), base, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	result, err := UpdatePDF(testSigningContext(), original, modified, time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}

	if !bytes.HasPrefix(result, original) {
		t.Error("updated PDF must preserve original bytes as prefix")
	}

	// Original wording must still exist in the preserved prefix bytes —
	// this is what content-credentials / signatures verify over.
	if !bytes.Contains(original, []byte("The original wording of clause one.")) {
		t.Error("original PDF must contain the original clause wording")
	}

	// The incremental section must show the new wording.
	increment := result[len(original):]
	if !bytes.Contains(increment, []byte("The revised wording of clause one.")) {
		t.Error("incremental section must contain the revised clause wording")
	}

	// The original wording must NOT appear in the incremental section —
	// it was replaced, not appended alongside.
	if bytes.Contains(increment, []byte("The original wording of clause one.")) {
		t.Error("incremental section must not repeat original clause wording; it was replaced")
	}
}

// TestUpdatePDFIncrementalTrailerHasID verifies that the incremental update
// trailer written by UpdatePDF includes the /ID key. ISO 19005-3:2012 clause
// 6.1.3 requires the trailer of every cross-reference section to carry an /ID
// array, including the trailers of incremental updates. Without /ID, veraPDF
// rejects the amended PDF.
func TestUpdatePDFIncrementalTrailerHasID(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	result, err := UpdatePDF(testSigningContext(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	increment := result[len(original):]
	if !bytes.Contains(increment, []byte("/ID [")) {
		t.Error("incremental update trailer missing /ID key; ISO 19005-3 clause 6.1.3 requires /ID in every trailer")
	}
}

// TestUpdatePDFIdenticalPayloadIsRejected verifies that submitting an unchanged
// payload returns an error (the HTTP layer maps this to 409 Conflict).
func TestUpdatePDFIdenticalPayloadIsRejected(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	_, err = UpdatePDF(testSigningContext(), original, []byte(minimalPayloadBase), time.Now())
	if err == nil {
		t.Error("UpdatePDF with identical payload must return an error")
	}
}

// TestUpdatePDFPreservesSigFieldWidgets verifies that when UpdatePDF is called
// on a document that originally had signature fields, the updated PDF still
// contains valid widget annotation references on the new pages (not null
// "0 0 R" references). Specifically:
//   - The incremental section must contain /FT /Sig (widget objects).
//   - The incremental section must NOT contain the pattern "0 0 R" in an
//     /Annots array (which would mean the widget references are null/broken).
//   - The updated /AcroForm must reference valid (non-zero) object IDs.
func TestUpdatePDFPreservesSigFieldWidgets(t *testing.T) {
	basePayload := []byte(`{
	"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
	"@id": "urn:doc:sig-field-test",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "Sig Field Update Test"},
	"signatureFields": [{"@type": "SignatureField", "@id": "urn:doc:sig-field-test#Signer1", "signatoryName": "Signer1"}],
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:sig-field-test#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:sig-field-test#s1", "children": ["urn:doc:sig-field-test#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:sig-field-test#s1", "title": "1. Terms"},
			{"@type": "Clause", "@id": "urn:doc:sig-field-test#c1", "content": ["Original text."]}
		]
	}
}`)
	amendedPayload := []byte(`{
	"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
	"@id": "urn:doc:sig-field-test",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "Sig Field Update Test"},
	"signatureFields": [{"@type": "SignatureField", "@id": "urn:doc:sig-field-test#Signer1", "signatoryName": "Signer1"}],
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:sig-field-test#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:sig-field-test#s1", "children": ["urn:doc:sig-field-test#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:sig-field-test#s1", "title": "1. Terms"},
			{"@type": "Clause", "@id": "urn:doc:sig-field-test#c1", "content": ["Amended text."]}
		]
	}
}`)

	base, err := CompilePDF(testSigningContext(), basePayload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	result, err := UpdatePDF(testSigningContext(), base, amendedPayload, time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}

	increment := result[len(base):]

	// The incremental section must contain a proper sig field widget.
	if !bytes.Contains(increment, []byte("/FT /Sig")) {
		t.Error("incremental section missing /FT /Sig; signature field widget must be re-emitted on update")
	}

	// The incremental section must not reference null objects (0 0 R) in /Annots.
	// A "0 0 R" in an Annots array means the widget link is broken.
	if bytes.Contains(increment, []byte("/Annots [0 0 R")) {
		t.Error("incremental section contains null annotation reference (0 0 R); sig field widget ObjectID was not assigned")
	}

	// The AcroForm in the incremental section must reference a non-zero object.
	if bytes.Contains(increment, []byte("/Fields [0 0 R")) {
		t.Error("incremental AcroForm contains null field reference (0 0 R); AcroForm was not updated with new widget IDs")
	}
}

// TestVerifyIncrementalUpdate_Valid checks that a legitimately amended PDF passes
// VerifyIncrementalUpdate — both the original prefix and the amendment must
// reproduce deterministically.
func TestVerifyIncrementalUpdate_Valid(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDF(testSigningContext(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	if _, err := VerifyIncrementalUpdate(testSigningContext(), amended); err != nil {
		t.Errorf("VerifyIncrementalUpdate on valid amended PDF: %v", err)
	}
}

// TestVerifyIncrementalUpdate_CorruptedIncrement checks that tampering with the
// incremental section causes VerifyIncrementalUpdate to return an error.
func TestVerifyIncrementalUpdate_CorruptedIncrement(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDF(testSigningContext(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	corrupted := append([]byte(nil), amended...)
	corrupted[len(original)+50] ^= 0xFF
	if _, err := VerifyIncrementalUpdate(testSigningContext(), corrupted); err == nil {
		t.Error("expected VerifyIncrementalUpdate to reject a corrupted incremental section")
	}
}

// TestVerifyIncrementalUpdate_PlainPDF checks that a plain compiled PDF (no
// incremental update marker) returns an error from VerifyIncrementalUpdate.
func TestVerifyIncrementalUpdate_PlainPDF(t *testing.T) {
	plain, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if _, err := VerifyIncrementalUpdate(testSigningContext(), plain); err == nil {
		t.Error("expected VerifyIncrementalUpdate to reject a plain (non-incremental) PDF")
	}
}

// TestVerifyIncrementalUpdate_SignedThenAmended checks that VerifyIncrementalUpdate
// accepts an amended PDF whose original was PAdES-signed before the amendment.
// PAdES signing appends bytes after %%EOF; VerifyIncrementalUpdate must treat the
// compiled portion as the deterministic root and allow append-only extras before
// and after the dcs-pdf-core incremental update.
func TestVerifyIncrementalUpdate_SignedThenAmended(t *testing.T) {
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	// Simulate a PAdES-style append: add a fake external signature appendix.
	// The real pyhanko signer does the same — appends bytes after %%EOF.
	fakeSignature := []byte("\n% external-signature-appendix\nstartxref\n0\n%%EOF\n")
	signed := append(append([]byte(nil), compiled...), fakeSignature...)

	amended, err := UpdatePDF(testSigningContext(), signed, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF on signed PDF: %v", err)
	}
	if _, err := VerifyIncrementalUpdate(testSigningContext(), amended); err != nil {
		t.Errorf("VerifyIncrementalUpdate on signed-then-amended PDF: %v", err)
	}

	// Also verify that amending after a PAdES-re-signed amended PDF passes.
	fakeSignature2 := []byte("\n% external-signature-appendix-2\nstartxref\n0\n%%EOF\n")
	reSigned := append(append([]byte(nil), amended...), fakeSignature2...)
	if _, err := VerifyIncrementalUpdate(testSigningContext(), reSigned); err != nil {
		t.Errorf("VerifyIncrementalUpdate on re-signed amended PDF: %v", err)
	}
}

// TestVerifyAfterUpdateNoCyclicC2PA checks that calling AppendVerificationWitness
// on an already-updated (amended) PDF produces a C2PA manifest store whose
// manifest labels are distinct — i.e. the witness manifest does not collide with
// the update manifest label, which would cause c2patool to report a cyclic ingredient.
func TestVerifyAfterUpdateNoCyclicC2PA(t *testing.T) {
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDF(testSigningContext(), compiled, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	amendedPayload, err := ExtractLatestEmbeddedJSONLD(amended)
	if err != nil {
		t.Fatalf("ExtractLatestEmbeddedJSONLD: %v", err)
	}
	witnessed, err := AppendVerificationWitness(testSigningContext(), amended, amendedPayload)
	if err != nil {
		t.Fatalf("AppendVerificationWitness: %v", err)
	}

	// Extract all top-level manifest labels from the witnessed PDF's C2PA store.
	c2paBytes, err := extractEmbeddedStreamByFileSpecName(witnessed, "content_credential.c2pa")
	if err != nil {
		t.Fatalf("extract C2PA from witnessed PDF: %v", err)
	}
	boxes, err := extractTopLevelManifestBoxes(c2paBytes)
	if err != nil {
		t.Fatalf("extractTopLevelManifestBoxes: %v", err)
	}
	labels := make(map[string]int)
	for _, box := range boxes {
		label, err := extractJUMBFLabel(box)
		if err != nil {
			t.Fatalf("extractJUMBFLabel: %v", err)
		}
		labels[label]++
	}
	for label, count := range labels {
		if count > 1 {
			t.Errorf("duplicate manifest label %q (appears %d times) — c2patool will report a cycle", label, count)
		}
	}
	if len(labels) < 3 {
		t.Errorf("expected at least 3 manifests (compiled, update, witness), got %d", len(labels))
	}
}
