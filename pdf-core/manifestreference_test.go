package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"
)

// stripC2PAAttachment removes the embedded C2PA manifest from a copy the way
// the BDD strip-then-verify step does: every name of the attachment is
// overwritten with a name of the same length, so no reader finds the manifest
// and the xref offsets stay valid.
func stripC2PAAttachment(pdf []byte) []byte {
	return bytes.ReplaceAll(pdf, []byte("(content_credential.c2pa)"), []byte("(xxxxxxxxxxxxxxxxxxxxxxx)"))
}

func multipartFields(t *testing.T, fields map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range fields {
		fw, err := mw.CreateFormField(name)
		if err != nil {
			t.Fatalf("multipart field %s: %v", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatalf("multipart write %s: %v", name, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// A copy without its embedded manifest still names the remote manifest in its
// XMP metadata, and still carries the pages and the payload the remote manifest
// describes (DCS-OR-C2PA-008).
func TestStrippedCopyKeepsTheRemoteManifestReference(t *testing.T) {
	const manifestURL = "https://dcs.example/api/c2pa/manifest/did:example:contract-42"

	body, ct := multipartFields(t, map[string][]byte{
		"pdf":          compilePDF(t),
		"payload":      []byte(minimalPayloadAmended),
		"manifest_url": []byte(manifestURL),
	})
	amended := signPrepared(t, doRequest(http.MethodPost, "/render/amendment", body, ct))

	stripped := stripC2PAAttachment(amended)
	if bytes.Equal(stripped, amended) {
		t.Fatal("test setup: the PDF has no content_credential.c2pa attachment to strip")
	}

	if rec := doRequest(http.MethodPost, "/manifest/extract", bytes.NewReader(stripped), "application/pdf"); rec.Code == http.StatusOK {
		t.Fatalf("the stripped copy still yields an embedded manifest: %d", rec.Code)
	}

	rec := doRequest(http.MethodPost, "/manifest/reference", bytes.NewReader(stripped), "application/pdf")
	if rec.Code != http.StatusOK || rec.Body.String() != manifestURL {
		t.Fatalf("manifest reference = %d %q, want 200 %q", rec.Code, rec.Body.String(), manifestURL)
	}

	if rec := doRequest(http.MethodPost, "/payload/extract", bytes.NewReader(stripped), "application/pdf"); rec.Code != http.StatusOK {
		t.Fatalf("payload of the stripped copy: %d %s", rec.Code, rec.Body.String())
	}

	matchBody, matchCT := multipartFields(t, map[string][]byte{"pdf": stripped, "reference": amended})
	rec = doRequest(http.MethodPost, "/verify/content-match", matchBody, matchCT)
	var match struct {
		Match    bool   `json:"match"`
		Mismatch string `json:"mismatch"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &match); err != nil || !match.Match {
		t.Fatalf("the stripped copy does not render the pages of the original: %d %s", rec.Code, rec.Body.String())
	}
}

// A PDF without an XMP provenance reference has no remote manifest to name.
func TestManifestReferenceIsNotFoundWithoutXMPProvenance(t *testing.T) {
	rec := doRequest(http.MethodPost, "/manifest/reference", bytes.NewReader(compilePDF(t)), "application/pdf")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a PDF without a remote manifest reference, got %d: %s", rec.Code, rec.Body.String())
	}
}
