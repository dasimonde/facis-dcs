package compiler

import (
	"bytes"
	"fmt"
	"testing"
)

const minimalPDFForEvidence = "%PDF-1.7\n" +
	"1 0 obj\n<< /Type /Catalog >>\nendobj\n" +
	"xref\n0 2\n0000000000 65535 f \n0000000009 00000 n \n" +
	"trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n40\n%%EOF\n"

func TestEmbedAndExtractSigningEvidenceRoundTrip(t *testing.T) {
	base := []byte(minimalPDFForEvidence)
	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)

	embedded, err := EmbedSigningEvidence(base, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	if !bytes.HasPrefix(embedded, base) {
		t.Fatal("embedded PDF must preserve the original bytes as a prefix so a later signature's ByteRange covers the evidence")
	}
	if !bytes.Contains(embedded, evidence) {
		t.Fatal("verbatim evidence bytes must appear in the embedded PDF")
	}

	got, err := ExtractSigningEvidence(embedded)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 embedded evidence attachment, got %d", len(got))
	}
	if !bytes.Equal(got[0], evidence) {
		t.Fatalf("extracted evidence mismatch:\n got %q\nwant %q", got[0], evidence)
	}
}

func TestEmbedSigningEvidenceEmptyIsNoop(t *testing.T) {
	base := []byte(minimalPDFForEvidence)
	out, err := EmbedSigningEvidence(base, nil)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence(nil): %v", err)
	}
	if !bytes.Equal(out, base) {
		t.Fatal("empty evidence must return the PDF unchanged")
	}
	got, err := ExtractSigningEvidence(base)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a PDF without evidence must yield no attachments, got %d", len(got))
	}
}

// Each signing party in a DCS-to-DCS federation embeds ITS OWN evidence before
// applying its signature, so a contract accumulates one attachment per signer.
// They must coexist under distinct names — a name tree holds exactly one entry
// per name, so re-using signing-evidence.json would leave every earlier
// attachment unreachable by name — and all of them must come back out.
func TestEmbedSigningEvidenceRepeatedlyKeepsEveryAttachment(t *testing.T) {
	pdf := []byte(minimalPDFForEvidence)
	evidences := [][]byte{
		[]byte(`{"party":"a","type":["ContractSigningSummaryCredential"]}`),
		[]byte(`{"party":"b","type":["ContractSigningSummaryCredential"]}`),
		[]byte(`{"party":"c","type":["VerifiablePresentation"],"poa":"eyJ.ccc"}`),
	}
	for i, evidence := range evidences {
		next, err := EmbedSigningEvidence(pdf, evidence)
		if err != nil {
			t.Fatalf("EmbedSigningEvidence #%d: %v", i+1, err)
		}
		if !bytes.HasPrefix(next, pdf) {
			t.Fatalf("embed #%d rewrote earlier bytes; a signature over them would break", i+1)
		}
		pdf = next
	}

	for i, name := range []string{"signing-evidence.json", "signing-evidence-2.json", "signing-evidence-3.json"} {
		if !bytes.Contains(pdf, []byte(fmt.Sprintf("/F (%s)", name))) {
			t.Errorf("attachment #%d is not filed under %s", i+1, name)
		}
	}

	got, err := ExtractSigningEvidence(pdf)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if len(got) != len(evidences) {
		t.Fatalf("extracted %d attachments, want %d", len(got), len(evidences))
	}
	for i := range evidences {
		if !bytes.Equal(got[i], evidences[i]) {
			t.Errorf("attachment %d (oldest-first) mismatch:\n got %s\nwant %s", i, got[i], evidences[i])
		}
	}
}

// The counterparty embeds its evidence into a PDF that already carries the
// first party's evidence, PAdES signature and validation data. The signed
// catalog — superseded by the signer to carry the /AcroForm holding the signed
// field — is the one that must be patched: patching a stale definition would
// drop the signature field the reader resolves.
func TestEmbedSigningEvidenceOverASignedRevision(t *testing.T) {
	ctx := WithSigner(testChainContext(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	first := []byte(`{"party":"a","type":["ContractSigningSummaryCredential"]}`)
	prepared, err := EmbedSigningEvidence(fresh, first)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence (first party): %v", err)
	}

	signed := appendPAdESRevision(t, prepared, "Signature1")
	signed = appendDSSRevision(t, signed)

	second := []byte(`{"party":"b","type":["ContractSigningSummaryCredential"]}`)
	countersigned, err := EmbedSigningEvidence(signed, second)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence (counterparty): %v", err)
	}
	if !bytes.HasPrefix(countersigned, signed) {
		t.Fatal("the first party's signed bytes must remain a prefix")
	}

	got, err := ExtractSigningEvidence(countersigned)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("extracted %d attachments, want 2 (one per signing party)", len(got))
	}
	if !bytes.Equal(got[0], first) || !bytes.Equal(got[1], second) {
		t.Fatalf("attachments out of embed order:\n got %s | %s\nwant %s | %s", got[0], got[1], first, second)
	}

	root, ok := currentRootObjID(countersigned)
	if !ok {
		t.Fatal("no /Root in the trailer after the counterparty embed")
	}
	catalog := objectBody(t, countersigned, root)
	if !bytes.Contains([]byte(catalog), []byte("/AcroForm")) {
		t.Fatal("the superseded catalog dropped the signer's /AcroForm; the signed field would no longer resolve")
	}
	if !bytes.Contains([]byte(catalog), []byte("/DSS")) {
		t.Fatal("the superseded catalog dropped the /DSS validation store the LTV layer added")
	}
	for _, name := range []string{"signing-evidence.json", "signing-evidence-2.json"} {
		if !bytes.Contains([]byte(catalog), []byte("("+name+")")) {
			t.Errorf("%s missing from the current catalog's /EmbeddedFiles name tree", name)
		}
	}
}

// A signer may publish a FRESH catalog object rather than supersede object 1,
// leaving /Root pointing elsewhere. Embedding must follow the trailer: patching
// a hardcoded object 1 supersedes a catalog no reader resolves, so the
// attachment ends up unlisted (ISO 19005-3 clause 6.8) and unreachable by name.
func TestEmbedSigningEvidenceFollowsARelocatedCatalog(t *testing.T) {
	ctx := WithSigner(testChainContext(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	relocated, newRoot := appendRelocatedCatalogRevision(t, fresh)

	evidence := []byte(`{"type":["ContractSigningSummaryCredential"],"id":"urn:uuid:relocated"}`)
	embedded, err := EmbedSigningEvidence(relocated, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}

	root, ok := currentRootObjID(embedded)
	if !ok {
		t.Fatal("no /Root in the trailer after embedding")
	}
	if root != newRoot {
		t.Fatalf("embedding moved /Root from %d to %d", newRoot, root)
	}
	catalog := objectBody(t, embedded, root)
	if !bytes.Contains([]byte(catalog), []byte(signingEvidenceFileName)) {
		t.Fatalf("the catalog a reader resolves (obj %d) does not name the evidence attachment: %s", root, catalog)
	}
	if got := bytes.Count([]byte(catalog), []byte("0 R")); got < 3 {
		t.Fatalf("catalog %s lost its associated files", catalog)
	}

	extracted, err := ExtractSigningEvidence(embedded)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if len(extracted) != 1 || !bytes.Equal(extracted[0], evidence) {
		t.Fatalf("evidence not recoverable after a relocated-catalog embed: %q", extracted)
	}
}
