package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// keywordPayload is a contract whose clauses say the words a PDF parser looks
// for. Clause text reaches the page content stream verbatim (only `\`, parens
// and newlines are escaped) and reaches the contract.jsonld attachment
// byte-for-byte, so any of these words would be a structural marker to a
// scanner that trusts text.
const keywordPayload = `{
  "@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#", "dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@id": "urn:doc:keyword-text",
  "@type": "ContractTemplate",
  "metadata": {"@type": "TemplateMetadata", "title": "Keyword Text"},
  "documentStructure": {
    "@type": "DocumentStructure",
    "layout": [
      {"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:keyword-text#s1"]},
      {"@type": "LayoutNode", "@id": "urn:doc:keyword-text#s1", "children": ["urn:doc:keyword-text#c1", "urn:doc:keyword-text#c2", "urn:doc:keyword-text#c3"]}
    ],
    "blocks": [
      {"@type": "Section", "@id": "urn:doc:keyword-text#s1", "title": "1. Keywords"},
      {"@type": "Clause", "@id": "urn:doc:keyword-text#c1", "content": ["The parties agree that the escrow record terminates at the endobj keyword."]},
      {"@type": "Clause", "@id": "urn:doc:keyword-text#c2", "content": ["The data feed is closed at the endstream keyword, and reopened at the stream keyword."]},
      {"@type": "Clause", "@id": "urn:doc:keyword-text#c3", "content": ["An obj referenced here (with an unbalanced paren and a stray ) bracket) stays inert."]}
    ]
  }
}`

// compileKeywordPDF compiles keywordPayload and returns the PDF together with
// the exact payload bytes the attachment must give back.
func compileKeywordPDF(t *testing.T) ([]byte, []byte) {
	t.Helper()
	payload := []byte(keywordPayload)
	pdf, err := CompilePDF(testSigningContext(), payload, CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	return pdf, payload
}

// The page content stream carries the clause text, so "endobj" inside it must
// not end the content object. When it does, the object is reported as having no
// stream at all and the /verify content gate rejects the document.
func TestPageContentSurvivesKeywordsInClauseText(t *testing.T) {
	pdf, payload := compileKeywordPDF(t)

	recompiled, err := CompilePDF(testSigningContext(), payload, CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if err := MatchPageContent(pdf, recompiled); err != nil {
		t.Fatalf("page content must match a recompile of the same payload: %v", err)
	}

	streams, err := extractPageContentStreams(pdf)
	if err != nil {
		t.Fatalf("extract page content: %v", err)
	}
	var all []byte
	for _, s := range streams {
		all = append(all, s...)
	}
	for _, word := range []string{"endobj", "endstream", "stream keyword", "An obj"} {
		if !bytes.Contains(all, []byte(word)) {
			t.Fatalf("page content stream is truncated: %q is missing", word)
		}
	}
}

// The attachment is embedded verbatim, so extraction must return every byte of
// it. The dangerous failure is silent: a truncated stream returns err == nil and
// the peer stores unparseable JSON-LD as the contract.
func TestEmbeddedJSONLDIsByteExactWithKeywordsInClauseText(t *testing.T) {
	pdf, payload := compileKeywordPDF(t)

	for _, tc := range []struct {
		name string
		last bool
	}{{"first", false}, {"last", true}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractJSONLDStream(pdf, tc.last)
			if err != nil {
				t.Fatalf("extractJSONLDStream: %v", err)
			}
			if len(got) != len(payload) {
				t.Fatalf("extracted %d of %d bytes", len(got), len(payload))
			}
			if !bytes.Equal(got, payload) {
				t.Fatal("extracted bytes differ from the embedded payload")
			}
			var doc map[string]any
			if err := json.Unmarshal(got, &doc); err != nil {
				t.Fatalf("extracted payload does not parse: %v", err)
			}
		})
	}

	start, length, err := findEmbeddedJSONLDStreamRange(pdf)
	if err != nil {
		t.Fatalf("findEmbeddedJSONLDStreamRange: %v", err)
	}
	if !bytes.Equal(pdf[start:start+length], payload) {
		t.Fatalf("stream range covers %d of %d payload bytes", length, len(payload))
	}
}

// Signing evidence is attached the same way and is arbitrary JSON, so the same
// words can appear in a party name or a credential claim.
func TestSigningEvidenceIsByteExactWithKeywordsInIt(t *testing.T) {
	pdf, _ := compileKeywordPDF(t)

	evidence := []byte(`{"note":"closed at the endstream keyword, filed under endobj (see stream)","n":1}`)
	embedded, err := EmbedSigningEvidence(pdf, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	got, found, err := ExtractSigningEvidence(embedded)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if !found {
		t.Fatal("evidence attachment not found")
	}
	if !bytes.Equal(got, evidence) {
		t.Fatalf("extracted %d of %d evidence bytes: %q", len(got), len(evidence), got)
	}
}

// A C2PA manifest is binary JUMBF: its last byte is whatever the box framing
// and the COSE signature make it, so one manifest in 256 ends in 0x0D. The
// writer emits `data + "\nendstream"`, and a reader that trims an end-of-line
// off the data eats that byte, handing /render/amendment a manifest one byte
// short — "invalid BMFF box framing", with no unusual contract text anywhere.
// The extent of a stream is its /Length, and nothing is trimmed off it.
//
// The condition is a property of the manifest bytes, not of the payload, so
// this compiles payload variants until it holds and then drives the real
// amendment path over that document.
func TestAC2PAManifestEndingInCarriageReturnSurvivesAnAmendment(t *testing.T) {
	const attempts = 4000
	var pdf, manifest []byte
	for i := 0; i < attempts && manifest == nil; i++ {
		payload := strings.Replace(minimalPayloadBase, "Original clause text.", fmt.Sprintf("Clause variant %d.", i), 1)
		candidate, err := CompilePDF(testSigningContext(), []byte(payload), CanonicalCompiledAt)
		if err != nil {
			t.Fatalf("compile variant %d: %v", i, err)
		}
		written := writtenC2PAManifest(t, candidate)
		if written[len(written)-1] == '\r' {
			pdf, manifest = candidate, written
		}
	}
	if manifest == nil {
		t.Fatalf("no compiled manifest ended in 0x0D within %d variants", attempts)
	}

	// The manifest the amendment path re-reads must be the one the compiler
	// wrote, to the byte: renderVerificationManifestStore parses it as BMFF.
	read, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		t.Fatalf("extract the manifest: %v", err)
	}
	if !bytes.Equal(read, manifest) {
		t.Fatalf("read %d of the %d manifest bytes written", len(read), len(manifest))
	}

	// The C2PA hard binding excludes the manifest's own stream; a window that is
	// not exactly it hashes bytes the claim is supposed to leave out.
	start, length, ok := findLastObjectStreamRange(pdf, 9)
	if !ok {
		t.Fatal("the manifest object's stream range must be found")
	}
	if !bytes.Equal(pdf[start:start+length], manifest) {
		t.Fatalf("the exclusion window covers %d of the manifest's %d bytes", length, len(manifest))
	}

	amended, err := UpdatePDFWithOptions(testSigningContext(), pdf, []byte(strings.Replace(minimalPayloadBase,
		"Original clause text.", "Amended clause text.", 1)), nil, "https://example.org/manifest", CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("amend a document whose manifest ends in 0x0D: %v", err)
	}
	if _, err := extractEmbeddedStreamByFileSpecName(amended, "content_credential.c2pa"); err != nil {
		t.Fatalf("re-read the amended manifest: %v", err)
	}
}

// c2paManifestObjectRE matches the C2PA embedded-file object's declared length
// and the start of its data.
var c2paManifestObjectRE = regexp.MustCompile(`/Subtype /application#2Fc2pa /Length (\d+) >>\nstream\n`)

// writtenC2PAManifest returns the manifest bytes as WRITTEN — taken from the
// object's declared /Length by this test alone, so it is an oracle independent
// of the extraction under test. Reading the manifest back through that
// extraction would hide the defect: it eats the last byte and then reports a
// manifest that never ends in the byte the test is looking for.
func writtenC2PAManifest(t *testing.T, pdf []byte) []byte {
	t.Helper()
	m := c2paManifestObjectRE.FindSubmatchIndex(pdf)
	if m == nil {
		t.Fatal("the compiled PDF carries no C2PA embedded-file object")
	}
	length, err := strconv.Atoi(string(pdf[m[2]:m[3]]))
	if err != nil {
		t.Fatalf("declared manifest length: %v", err)
	}
	return pdf[m[1] : m[1]+length]
}
