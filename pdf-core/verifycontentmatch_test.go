package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// buildContentMatchBody builds the multipart body /verify/content-match takes:
// the submitted PDF and the reference PDF its pages must still reproduce.
func buildContentMatchBody(t *testing.T, submitted, reference []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, field := range []struct {
		name string
		data []byte
	}{{"pdf", submitted}, {"reference", reference}} {
		fw, err := mw.CreateFormField(field.name)
		if err != nil {
			t.Fatalf("multipart CreateFormField %s: %v", field.name, err)
		}
		if _, err := fw.Write(field.data); err != nil {
			t.Fatalf("multipart write %s: %v", field.name, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return &buf, "multipart/form-data; boundary=" + mw.Boundary()
}

func postContentMatch(t *testing.T, submitted, reference []byte) (bool, string, int) {
	t.Helper()
	body, ct := buildContentMatchBody(t, submitted, reference)
	rec := doRequest(http.MethodPost, "/verify/content-match", body, ct)
	if rec.Code != http.StatusOK {
		return false, rec.Body.String(), rec.Code
	}
	var decoded struct {
		Match    bool   `json:"match"`
		Mismatch string `json:"mismatch"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode verify/content-match response: %v", err)
	}
	return decoded.Match, decoded.Mismatch, rec.Code
}

func TestVerifyContentMatch_SameDocumentMatches(t *testing.T) {
	pdf := compilePDF(t)

	match, mismatch, code := postContentMatch(t, pdf, pdf)
	if code != http.StatusOK {
		t.Fatalf("verify/content-match: status %d: %s", code, mismatch)
	}
	if !match {
		t.Errorf("a document must match itself, got mismatch: %s", mismatch)
	}
}

// The check compares against a reference the caller holds, not against the
// submission's own embedded payload — so a submission that renders a DIFFERENT
// contract is refused even though it is internally self-consistent.
func TestVerifyContentMatch_DifferentDocumentDiverges(t *testing.T) {
	reference := compilePDF(t)
	otherRec := doRequest(http.MethodPost, "/render",
		strings.NewReader(strings.Replace(minimalPayload, "clause one", "clause TWO", 1)),
		"application/ld+json")
	other := signPrepared(t, otherRec)

	match, mismatch, code := postContentMatch(t, other, reference)
	if code != http.StatusOK {
		t.Fatalf("verify/content-match: status %d: %s", code, mismatch)
	}
	if match {
		t.Fatal("a PDF rendering different contract text must not match the reference")
	}
	if !strings.Contains(mismatch, "page 1 content does not match") {
		t.Errorf("mismatch must name what diverged, got: %q", mismatch)
	}
}

func TestVerifyContentMatch_ReferenceRequired(t *testing.T) {
	pdf := compilePDF(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormField("pdf")
	if err != nil {
		t.Fatalf("multipart CreateFormField pdf: %v", err)
	}
	if _, err := fw.Write(pdf); err != nil {
		t.Fatalf("multipart write pdf: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}

	rec := doRequest(http.MethodPost, "/verify/content-match", &buf,
		"multipart/form-data; boundary="+mw.Boundary())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without a reference, got %d", rec.Code)
	}
}

func TestVerifyContentMatch_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/verify/content-match",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}
