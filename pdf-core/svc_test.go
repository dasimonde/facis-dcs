package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	compiler "example.com/m/V2/compiler"
)

// minimalPayload is a valid JSON-LD payload that CompilePDF can process.
// The @vocab entry ensures all terms expand to dcs ontology IRIs and therefore
// appear in the URDNA2015 N-Quads used for determinism checks and change detection.
const minimalPayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:svc-test",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "Service test"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:svc-test#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:svc-test#s1", "children": ["urn:doc:svc-test#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:svc-test#s1", "title": "1. Test"},
			{"@type": "Clause", "@id": "urn:doc:svc-test#c1", "content": ["clause one"]}
		]
	}
}`

// minimalPayloadAmended adds a second clause to minimalPayload. The new clause
// produces additional N-Quads, making the two payloads semantically distinct.
const minimalPayloadAmended = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:svc-test",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "Service test"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:svc-test#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:svc-test#s1", "children": ["urn:doc:svc-test#c1", "urn:doc:svc-test#c2"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:svc-test#s1", "title": "1. Test"},
			{"@type": "Clause", "@id": "urn:doc:svc-test#c1", "content": ["clause one"]},
			{"@type": "Clause", "@id": "urn:doc:svc-test#c2", "content": ["clause two"]}
		]
	}
}`

const minimalPayloadFlavorPrefixed = `{
	"@context": {
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:svc-test",
	"@type": "dcs:ContractTemplate",
	"dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "Service test"},
	"dcs:documentStructure": {
		"@type": "dcs:DocumentStructure",
		"dcs:layout": [
			{"@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": ["urn:doc:svc-test#s1"]},
			{"@type": "dcs:LayoutNode", "@id": "urn:doc:svc-test#s1", "dcs:children": ["urn:doc:svc-test#c1"]}
		],
		"dcs:blocks": [
			{"@type": "dcs:Section", "@id": "urn:doc:svc-test#s1", "dcs:title": "1. Test"},
			{"@type": "dcs:Clause", "@id": "urn:doc:svc-test#c1", "dcs:content": ["clause one"]}
		]
	}
}`

const minimalPayloadFlavorExpanded = `{
	"@context": {},
	"@id": "urn:doc:svc-test",
	"@type": "https://w3id.org/facis/dcs/ontology/v1#ContractTemplate",
	"https://w3id.org/facis/dcs/ontology/v1#metadata": [{
		"@type": "https://w3id.org/facis/dcs/ontology/v1#TemplateMetadata",
		"https://w3id.org/facis/dcs/ontology/v1#title": [{"@value": "Service test"}]
	}],
	"https://w3id.org/facis/dcs/ontology/v1#documentStructure": [{
		"@type": "https://w3id.org/facis/dcs/ontology/v1#DocumentStructure",
		"https://w3id.org/facis/dcs/ontology/v1#layout": [
			{"@type": "https://w3id.org/facis/dcs/ontology/v1#LayoutNode",
			 "https://w3id.org/facis/dcs/ontology/v1#isRoot": [{"@value": true}],
			 "https://w3id.org/facis/dcs/ontology/v1#children": [{"@value": "urn:doc:svc-test#s1"}]},
			{"@type": "https://w3id.org/facis/dcs/ontology/v1#LayoutNode",
			 "@id": "urn:doc:svc-test#s1",
			 "https://w3id.org/facis/dcs/ontology/v1#children": [{"@value": "urn:doc:svc-test#c1"}]}
		],
		"https://w3id.org/facis/dcs/ontology/v1#blocks": [
			{"@type": "https://w3id.org/facis/dcs/ontology/v1#Section",
			 "@id": "urn:doc:svc-test#s1",
			 "https://w3id.org/facis/dcs/ontology/v1#title": [{"@value": "1. Test"}]},
			{"@type": "https://w3id.org/facis/dcs/ontology/v1#Clause",
			 "@id": "urn:doc:svc-test#c1",
			 "https://w3id.org/facis/dcs/ontology/v1#content": [{"@value": "clause one"}]}
		]
	}]
}`

// buildMultipartBody constructs an io.Reader and Content-Type header for a
// multipart/form-data body containing "pdf" and "payload" fields.
func buildMultipartBody(t *testing.T, pdf []byte, payload string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	boundary := mw.Boundary()
	fw, err := mw.CreateFormField("pdf")
	if err != nil {
		t.Fatalf("multipart CreateFormField pdf: %v", err)
	}
	if _, err := fw.Write(pdf); err != nil {
		t.Fatalf("multipart write pdf: %v", err)
	}
	fw2, err := mw.CreateFormField("payload")
	if err != nil {
		t.Fatalf("multipart CreateFormField payload: %v", err)
	}
	if _, err := fw2.Write([]byte(payload)); err != nil {
		t.Fatalf("multipart write payload: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return &buf, "multipart/form-data; boundary=" + boundary
}

// buildMultipartBodyWithVC is like buildMultipartBody but also includes a "vc" field.
func buildMultipartBodyWithVC(t *testing.T, pdf []byte, payload string, vc []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	boundary := mw.Boundary()
	for name, data := range map[string][]byte{"pdf": pdf, "payload": []byte(payload), "vc": vc} {
		fw, err := mw.CreateFormField(name)
		if err != nil {
			t.Fatalf("multipart CreateFormField %s: %v", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatalf("multipart write %s: %v", name, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	return &buf, "multipart/form-data; boundary=" + boundary
}

// errorName parses the "name" field from a JSON error response body.
func errorName(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("parse error body: %v (body: %s)", err, body)
	}
	return v.Name
}

// doRequest performs a request against newServer() and returns the recorder.
func doRequest(method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	newServer().ServeHTTP(rec, req)
	return rec
}

// signPrepared drives the stateless two-step signing a prepare recorder began:
// it decodes the prepared PDF + Sig_structures, signs each with the test key (as
// the DCS backend signs with dcs-c2pa), posts them to /c2pa/embed, and returns
// the final signed PDF. Use it for every success-path /render or /render/amendment call.
func signPrepared(t *testing.T, rec *httptest.ResponseRecorder) []byte {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare: status %d, body: %s", rec.Code, rec.Body.String())
	}
	var prepared preparedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &prepared); err != nil {
		t.Fatalf("decode prepared response: %v", err)
	}
	signatures := make([]string, len(prepared.C2PASigStructures))
	for i, s := range prepared.C2PASigStructures {
		sigStructure, decErr := base64.StdEncoding.DecodeString(s)
		if decErr != nil {
			t.Fatalf("decode sig_structure %d: %v", i, decErr)
		}
		signatures[i] = base64.StdEncoding.EncodeToString(signSigStructure(sigStructure))
	}
	body, err := json.Marshal(embedRequest{PDFBase64: prepared.PDFBase64, C2PASignatures: signatures})
	if err != nil {
		t.Fatalf("marshal embed request: %v", err)
	}
	embedRec := doRequest(http.MethodPost, "/c2pa/embed", bytes.NewReader(body), "application/json")
	if embedRec.Code != http.StatusOK {
		t.Fatalf("embed: status %d, body: %s", embedRec.Code, embedRec.Body.String())
	}
	return embedRec.Body.Bytes()
}

// compilePDF is a test helper that compiles minimalPayload and returns the final
// signed PDF bytes (prepare -> sign -> embed).
func compilePDF(t *testing.T) []byte {
	t.Helper()
	rec := doRequest(http.MethodPost, "/render",
		bytes.NewBufferString(minimalPayload), "application/ld+json")
	return signPrepared(t, rec)
}

// ---- Download ---------------------------------------------------------------

func TestDownload_ValidPayload(t *testing.T) {
	rec := doRequest(http.MethodPost, "/render",
		bytes.NewBufferString(minimalPayload), "application/ld+json")
	pdf := signPrepared(t, rec)
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("result does not start with PDF header")
	}
}

func TestDownload_ApplicationJSON(t *testing.T) {
	rec := doRequest(http.MethodPost, "/render",
		bytes.NewBufferString(minimalPayload), "application/json")
	if !bytes.HasPrefix(signPrepared(t, rec), []byte("%PDF-")) {
		t.Fatal("result does not start with PDF header")
	}
}

func TestDownload_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/render",
		bytes.NewBufferString("hello"), "text/plain")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

func TestDownload_InvalidPayload(t *testing.T) {
	rec := doRequest(http.MethodPost, "/render",
		bytes.NewBufferString("not valid json-ld"), "application/ld+json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}

func TestDownload_CarriesJSONLDAttachmentVerbatim(t *testing.T) {
	// pdf-core carries the JSON-LD attachment VERBATIM — each compiled PDF embeds
	// exactly the bytes submitted, byte-preserved, whatever the serialization
	// flavor. Canonicalizing the OUTPUT so distinct flavors collapse to one form
	// is NOT pdf-core's concern anymore (DCS canonicalizes before sending); a
	// renderer must not silently rewrite the document it carries. pdf-core's own
	// guarantee — the visible render reproduces from documentStructure — is
	// covered by determinism.feature.
	for i, payload := range []string{minimalPayload, minimalPayloadFlavorPrefixed} {
		rec := doRequest(http.MethodPost, "/render",
			bytes.NewBufferString(payload), "application/ld+json")
		if rec.Code != http.StatusOK {
			t.Fatalf("Download flavor %d failed: status %d", i, rec.Code)
		}
		var env struct {
			PDFBase64 string `json:"pdf_base64"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("flavor %d: decode prepared envelope: %v", i, err)
		}
		pdf, err := base64.StdEncoding.DecodeString(env.PDFBase64)
		if err != nil {
			t.Fatalf("flavor %d: decode pdf: %v", i, err)
		}
		embedded, err := compiler.ExtractEmbeddedJSONLD(pdf)
		if err != nil {
			t.Fatalf("flavor %d: extract embedded: %v", i, err)
		}
		if strings.TrimSpace(string(embedded)) != strings.TrimSpace(payload) {
			t.Fatalf("flavor %d: embedded attachment is not the verbatim submitted payload\ngot:  %.120s\nwant: %.120s", i, embedded, payload)
		}
	}
}

// TestDownload_RejectsExpandedJSONLD proves pdf-core reads only the compact
// dcs:/bare-term shape DCS sends. An expanded-IRI serialization is not the
// expected documentStructure shape and is rejected — pdf-core no longer runs a
// json-gold expand/compact pass to accept arbitrary flavors (that round trip
// reordered rich content non-deterministically).
func TestDownload_RejectsExpandedJSONLD(t *testing.T) {
	rec := doRequest(http.MethodPost, "/render",
		bytes.NewBufferString(minimalPayloadFlavorExpanded), "application/ld+json")
	if rec.Code == http.StatusOK {
		t.Fatalf("expanded JSON-LD flavor must be rejected, got status 200")
	}
}

func TestDownload_MalformedPayloadReportsValidationDetails(t *testing.T) {
	// A ContractTemplate without the required dcs:metadata must fail SHACL validation
	// with a MinCountConstraintComponent violation on dcs:metadata.
	malformed := `{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:svc-bad",
		"@type": "ContractTemplate"
	}`
	rec := doRequest(http.MethodPost, "/render",
		bytes.NewBufferString(malformed), "application/ld+json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
	var v struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	msg := v.Message
	if !strings.Contains(msg, "path=<https://w3id.org/facis/dcs/ontology/v1#metadata>") ||
		!strings.Contains(msg, "component=<http://www.w3.org/ns/shacl#MinCountConstraintComponent>") {
		t.Fatalf("expected detailed validation report with paths, got: %s", msg)
	}
}

// ---- Verify -----------------------------------------------------------------

func TestVerify_ValidPDF(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Error("expected match=true for a valid compiled PDF")
	}
}

func TestVerify_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

// ---- Verify content --------------------------------------------------------

// verifyContent posts a PDF to /verify/content and returns the decoded match.
func verifyContentMatch(t *testing.T, pdf []byte) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(http.MethodPost, "/verify/content",
		bytes.NewReader(pdf), "application/pdf")
}

// TestVerifyContent_SignedPDFMatches proves the content-only check accepts a
// fully signed (C2PA-embedded) PDF: unlike /verify's byte-prefix reproduction,
// /verify/content compares only the page content streams, so the appended
// signature and provenance layers do not make a legitimate artifact diverge.
func TestVerifyContent_SignedPDFMatches(t *testing.T) {
	pdf := compilePDF(t)

	rec := verifyContentMatch(t, pdf)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify/content: status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Match bool `json:"match"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode verify/content response: %v", err)
	}
	if !body.Match {
		t.Error("expected match=true for a signed PDF whose page content renders from its embedded payload")
	}
}

// TestVerifyContent_TamperedPageContentRejected edits only the visible page
// content stream (leaving the embedded machine-readable payload untouched) and
// asserts the endpoint reports match=false. This is the legal guarantee PostPdf
// relies on: a human-readable form that no longer matches its embedded payload
// must be refused. The clause literal "clause one" appears twice — once in the
// embedded JSON-LD (["clause one"]) and once in the page stream ((clause one)
// Tj); only the page-stream copy is swapped, for an equal-length divergence
// that keeps the xref table valid.
func TestVerifyContent_TamperedPageContentRejected(t *testing.T) {
	pdf := compilePDF(t)

	tampered := bytes.Replace(pdf, []byte("(clause one) Tj"), []byte("(clause TWO) Tj"), 1)
	if bytes.Equal(tampered, pdf) {
		t.Fatal("test setup: page-content clause literal not found to tamper")
	}

	rec := verifyContentMatch(t, tampered)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify/content: status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Match bool `json:"match"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode verify/content response: %v", err)
	}
	if body.Match {
		t.Error("expected match=false: page content was tampered but the embedded payload was not")
	}
}

func TestVerifyContent_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/verify/content",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

// ---- Update -----------------------------------------------------------------

func TestUpdate_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/render/amendment",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

func TestUpdate_MissingPDFField(t *testing.T) {
	multipartBody := []byte("--boundary\r\nContent-Disposition: form-data; name=\"payload\"\r\n\r\nhello\r\n--boundary--\r\n")
	rec := doRequest(http.MethodPost, "/render/amendment",
		bytes.NewReader(multipartBody), "multipart/form-data; boundary=boundary")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}

// ---- OntologyContext --------------------------------------------------------

func TestOntologyContext(t *testing.T) {
	rec := doRequest(http.MethodGet, "/ontology/dcs-pdf-core", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	res := rec.Body.Bytes()
	if len(res) == 0 {
		t.Fatal("expected non-empty ontology context")
	}
	if !json.Valid(res) {
		t.Fatal("ontology context is not valid JSON")
	}
}

// TestVersionEndpointReturnsVersion verifies that GET /version returns a JSON
// object containing the renderer version string.
func TestVersionEndpointReturnsVersion(t *testing.T) {
	rec := doRequest(http.MethodGet, "/version", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json content type, got %q", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	v, ok := body["version"]
	if !ok || v == "" {
		t.Fatalf("response body must contain non-empty \"version\" field, got %v", body)
	}
}

// TestDownloadResponseCarriesVersionHeader verifies that POST /download
// includes an X-PDF-Core-Version header in the response.
func TestDownloadResponseCarriesVersionHeader(t *testing.T) {
	rec := doRequest(http.MethodPost, "/render",
		strings.NewReader(minimalPayload), "application/ld+json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if v := rec.Header().Get("X-PDF-Core-Version"); v == "" {
		t.Error("POST /download response must include X-PDF-Core-Version header")
	}
}

// TestUpdateResponseCarriesVersionHeader verifies that POST /update
// includes an X-PDF-Core-Version header in the response.
func TestUpdateResponseCarriesVersionHeader(t *testing.T) {
	// First compile a base PDF.
	baseRec := doRequest(http.MethodPost, "/render",
		strings.NewReader(minimalPayload), "application/ld+json")
	if baseRec.Code != http.StatusOK {
		t.Fatalf("compile base PDF: expected 200, got %d", baseRec.Code)
	}
	basePDF := signPrepared(t, baseRec)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(basePDF)
	payloadPart, _ := mw.CreateFormField("payload")
	_, _ = payloadPart.Write([]byte(minimalPayloadAmended))
	mw.Close()

	rec := doRequest(http.MethodPost, "/render/amendment", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if v := rec.Header().Get("X-PDF-Core-Version"); v == "" {
		t.Error("POST /update response must include X-PDF-Core-Version header")
	}
}

// TestUpdateWithVCEmbedsAttachment verifies that POST /update with an optional
// "vc" multipart field embeds the VC bytes as "contract-lifecycle-vc.json" in
// the returned PDF.
func TestUpdateWithVCEmbedsAttachment(t *testing.T) {
	baseRec := doRequest(http.MethodPost, "/render",
		strings.NewReader(minimalPayload), "application/ld+json")
	if baseRec.Code != http.StatusOK {
		t.Fatalf("compile base PDF: %d", baseRec.Code)
	}
	basePDF := signPrepared(t, baseRec)

	vcBytes := []byte(`{"type":["VerifiableCredential"],"id":"urn:dcs:vc:test"}`)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(basePDF)
	payloadPart, _ := mw.CreateFormField("payload")
	_, _ = payloadPart.Write([]byte(minimalPayloadAmended))
	vcPart, _ := mw.CreateFormField("vc")
	_, _ = vcPart.Write(vcBytes)
	mw.Close()

	rec := doRequest(http.MethodPost, "/render/amendment", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(signPrepared(t, rec), []byte("contract-lifecycle-vc.json")) {
		t.Error("response PDF must contain the VC attachment name")
	}
}

// TestUpdateWithVCUnchangedPayloadProceeds verifies that POST /update with a
// "vc" field succeeds even when the payload is identical to the current
// embedded one, because the VC attachment is itself a provenance event.
func TestUpdateWithVCUnchangedPayloadProceeds(t *testing.T) {
	baseRec := doRequest(http.MethodPost, "/render",
		strings.NewReader(minimalPayload), "application/ld+json")
	if baseRec.Code != http.StatusOK {
		t.Fatalf("compile base PDF: %d", baseRec.Code)
	}
	basePDF := signPrepared(t, baseRec)

	vcBytes := []byte(`{"type":["VerifiableCredential"],"id":"urn:dcs:vc:genesis"}`)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(basePDF)
	payloadPart, _ := mw.CreateFormField("payload")
	_, _ = payloadPart.Write([]byte(minimalPayload)) // same payload as base
	vcPart, _ := mw.CreateFormField("vc")
	_, _ = vcPart.Write(vcBytes)
	mw.Close()

	rec := doRequest(http.MethodPost, "/render/amendment", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (VC-only update), got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(signPrepared(t, rec), []byte("contract-lifecycle-vc.json")) {
		t.Error("response PDF must contain the VC attachment name")
	}
}

// TestOntologyContextSubstitutesBaseURL verifies that when
// DCS_PDF_CORE_ONTOLOGY_BASE_URL is set, the context endpoint replaces the
// hardcoded default base URL with the configured one.
func TestOntologyContextIsValidJSONLD(t *testing.T) {
	rec := doRequest(http.MethodGet, "/ontology/dcs-pdf-core", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	res := rec.Body.Bytes()
	if len(res) == 0 {
		t.Fatal("expected non-empty context bytes")
	}
	if !json.Valid(res) {
		t.Fatal("ontology context response must be valid JSON")
	}
	if !bytes.Contains(res, []byte("w3id.org/facis/dcs/ontology")) {
		t.Error("ontology context must reference the canonical dcs ontology IRI")
	}
}

// ---- Verify (amended document) ----------------------------------------------

// TestVerify_AmendedDocument proves that /verify accepts a PDF produced by
// /update and appends a verification witness.
func TestVerify_AmendedDocument(t *testing.T) {
	original := compilePDF(t)

	body, ct := buildMultipartBody(t, original, minimalPayloadAmended)
	recUpdate := doRequest(http.MethodPost, "/render/amendment", body, ct)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update: status %d: %s", recUpdate.Code, recUpdate.Body.String())
	}
	amended := signPrepared(t, recUpdate)
	if !bytes.HasPrefix(amended, original) {
		t.Fatal("amended PDF must preserve original bytes as prefix (C2PA invariant)")
	}

	recVerify := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(amended), "application/pdf")
	if recVerify.Code != http.StatusOK {
		t.Fatalf("verify amended: status %d: %s", recVerify.Code, recVerify.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(recVerify.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Error("expected match=true for valid amended PDF")
	}
	// An amendment carries the previous manifest forward verbatim next to a newly
	// signed one, so both claim signatures must verify.
	if !result.C2PASignatureValid {
		t.Errorf("expected c2pa_signature_valid=true for a valid amended PDF, got error %q", result.C2PASignatureError)
	}
}

// TestVerify_AmendedDocumentRejectsCorruption proves that /verify rejects an
// amended PDF whose incremental section has been tampered with.
func TestVerify_AmendedDocumentRejectsCorruption(t *testing.T) {
	original := compilePDF(t)

	body, ct := buildMultipartBody(t, original, minimalPayloadAmended)
	recUpdate := doRequest(http.MethodPost, "/render/amendment", body, ct)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update: status %d", recUpdate.Code)
	}
	amended := signPrepared(t, recUpdate)

	corrupted := append([]byte(nil), amended...)
	corrupted[len(original)+50] ^= 0xFF

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(corrupted), "application/pdf")
	if rec.Code == http.StatusOK {
		t.Fatal("expected Verify to reject a corrupted amended document")
	}
}

// ---- Claim ------------------------------------------------------------------

// compileAndStripPayload compiles minimalPayload and strips the embedded JSON-LD.
func compileAndStripPayload(t *testing.T) (fullPDF, strippedPDF []byte) {
	t.Helper()
	full := compilePDF(t)
	stripped, err := compiler.StripEmbeddedJSONLD(full)
	if err != nil {
		t.Fatalf("StripEmbeddedJSONLD: %v", err)
	}
	return full, stripped
}

// TestClaim_MatchingPayload verifies that /claim accepts a stripped PDF paired
// with the correct JSON-LD.
func TestClaim_MatchingPayload(t *testing.T) {
	_, stripped := compileAndStripPayload(t)

	body, ct := buildMultipartBody(t, stripped, minimalPayload)
	rec := doRequest(http.MethodPost, "/claim", body, ct)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim: status %d: %s", rec.Code, rec.Body.String())
	}
	result := rec.Body.Bytes()
	if !bytes.HasPrefix(result, []byte("%PDF-")) {
		t.Fatal("result must be a PDF")
	}
	baseline, _ := compiler.CompilePDF(context.Background(), []byte(minimalPayload), time.Now())
	if len(result) <= len(baseline) {
		t.Fatal("result must include a witness appendix and be larger than a bare compilation")
	}
}

// TestClaim_WrongContentType verifies that a non-multipart Content-Type is
// rejected with 415.
func TestClaim_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/claim",
		bytes.NewBufferString("{}"), "application/json")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "unsupported_media_type" {
		t.Fatalf("expected unsupported_media_type, got %q", name)
	}
}

// TestClaim_MismatchedPayload verifies that /claim rejects a payload whose
// compiled output does not match the submitted PDF's page content.
func TestClaim_MismatchedPayload(t *testing.T) {
	_, stripped := compileAndStripPayload(t)

	body, ct := buildMultipartBody(t, stripped, minimalPayloadAmended)
	rec := doRequest(http.MethodPost, "/claim", body, ct)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "conflict" {
		t.Fatalf("expected conflict, got %q", name)
	}
}

// TestClaim_MissingPDFField verifies that /claim rejects a request without
// the pdf field.
func TestClaim_MissingPDFField(t *testing.T) {
	multipartBody := []byte("--boundary\r\nContent-Disposition: form-data; name=\"payload\"\r\n\r\nhello\r\n--boundary--\r\n")
	rec := doRequest(http.MethodPost, "/claim",
		bytes.NewReader(multipartBody), "multipart/form-data; boundary=boundary")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if name := errorName(t, rec.Body.Bytes()); name != "bad_request" {
		t.Fatalf("expected bad_request, got %q", name)
	}
}

// ---- Verify JSON response ---------------------------------------------------

// verifyResult matches the JSON body returned by POST /verify.
type verifyResult struct {
	Match              bool   `json:"match"`
	C2PASignatureValid bool   `json:"c2pa_signature_valid"`
	C2PASignatureError string `json:"c2pa_signature_error,omitempty"`
	VCBytes            string `json:"vc_bytes,omitempty"` // base64-encoded VC JSON
	VCPresent          bool   `json:"vc_present"`
	Artifact           string `json:"artifact"` // base64-encoded verification-witness PDF
}

// TestVerify_ReturnsJSON verifies that POST /verify returns application/json
// with a match=true body for a valid compiled PDF.
func TestVerify_ReturnsJSON(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json response, got %q", ct)
	}

	var result verifyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Error("expected match=true for a valid compiled PDF")
	}
	if !result.C2PASignatureValid {
		t.Errorf("expected c2pa_signature_valid=true for a valid compiled PDF, got error %q", result.C2PASignatureError)
	}
}

// TestVerify_ReportsForgedC2PASignature is the behaviour the field's old
// hardcoded true hid: /verify now runs the COSE claim-signature check, so a PDF
// whose signature bytes were replaced is reported as invalid with the reason.
func TestVerify_ReportsForgedC2PASignature(t *testing.T) {
	pdf := compilePDF(t)
	idx := bytes.Index(pdf, []byte{0xA0, 0xF6, 0x58, 0x40})
	if idx < 0 {
		t.Fatal("compiled PDF carries no COSE signature to forge")
	}
	forged := append([]byte(nil), pdf...)
	start := idx + 4
	for i := start; i < start+64; i++ {
		forged[i] ^= 0xFF
	}

	rec := doRequest(http.MethodPost, "/verify", bytes.NewReader(forged), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if result.C2PASignatureValid {
		t.Error("expected c2pa_signature_valid=false for a PDF with a forged COSE signature")
	}
	if result.C2PASignatureError == "" {
		t.Error("expected c2pa_signature_error to name the reason the signature failed")
	}
}

// TestVerify_ReportsVCPresenceNotProofValidity pins that pdf-core reports only
// what it can establish about an embedded lifecycle credential. Verifying the
// proof needs the issuer's assertion key, which pdf-core cannot resolve, so the
// response carries presence plus the bytes and leaves the verdict to the caller.
func TestVerify_ReportsVCPresenceNotProofValidity(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/verify", bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()
	if bytes.Contains(body, []byte("vc_proof_valid")) {
		t.Error("pdf-core must not report a vc_proof_valid verdict it cannot establish")
	}
	var result verifyResult
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if result.VCPresent {
		t.Error("expected vc_present=false for a compiled PDF carrying no lifecycle credential")
	}
}

// TestVerify_ArtifactFieldContainsWitnessPDF verifies that POST /verify
// embeds the verification-witness PDF (base64-encoded) in the "artifact"
// JSON field (DCS-OR-C2PA-008 witness requirement).
func TestVerify_ArtifactFieldContainsWitnessPDF(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}

	var result verifyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if result.Artifact == "" {
		t.Fatal("expected non-empty artifact field in /verify JSON response")
	}
	artifactBytes, err := base64.StdEncoding.DecodeString(result.Artifact)
	if err != nil {
		t.Fatalf("base64 decode artifact: %v", err)
	}
	if !bytes.HasPrefix(artifactBytes, []byte("%PDF-")) {
		t.Errorf("artifact is not a PDF (got %q...)", artifactBytes[:min(10, len(artifactBytes))])
	}
	if len(artifactBytes) <= len(pdf) {
		t.Error("artifact (witness PDF) must be larger than the original compiled PDF")
	}
}

// TestVerify_JSONIncludesVCBytesWhenPresent verifies that when a PDF contains
// a contract-lifecycle-vc.json attachment /verify returns its base64 bytes in
// the vc_bytes field so the backend can check the status list without parsing
// PDF bytes itself.
func TestVerify_JSONIncludesVCBytesWhenPresent(t *testing.T) {
	original := compilePDF(t)
	vcJSON := []byte(`{"type":["VerifiableCredential"],"credentialSubject":{"status":"active"}}`)

	body, ct := buildMultipartBodyWithVC(t, original, minimalPayloadAmended, vcJSON)
	recUpdate := doRequest(http.MethodPost, "/render/amendment", body, ct)
	if recUpdate.Code != http.StatusOK {
		t.Fatalf("update: status %d: %s", recUpdate.Code, recUpdate.Body.String())
	}
	withVC := signPrepared(t, recUpdate)

	recVerify := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(withVC), "application/pdf")
	if recVerify.Code != http.StatusOK {
		t.Fatalf("verify with VC: status %d: %s", recVerify.Code, recVerify.Body.String())
	}

	var result verifyResult
	if err := json.NewDecoder(recVerify.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if result.VCBytes == "" {
		t.Error("expected vc_bytes to be non-empty when PDF contains contract-lifecycle-vc.json")
	}
	if !result.VCPresent {
		t.Error("expected vc_present=true when PDF contains contract-lifecycle-vc.json")
	}
	if !result.C2PASignatureValid {
		t.Errorf("expected c2pa_signature_valid=true for a VC-carrying amended PDF, got error %q", result.C2PASignatureError)
	}
}

// ---- POST /manifest/extract -------------------------------------------------

// TestExtractManifest_ReturnsManifestBytes verifies that POST /manifest/extract
// on a compiled PDF returns non-empty JUMBF bytes starting with the "jumb" marker.
func TestExtractManifest_ReturnsManifestBytes(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/manifest/extract",
		bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("expected application/octet-stream, got %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("expected non-empty manifest bytes")
	}
	if !bytes.Contains(body, []byte("jumb")) {
		t.Error("manifest bytes do not contain JUMBF marker 'jumb'")
	}
}

// TestExtractManifest_WrongContentType verifies that a non-PDF Content-Type
// returns 415.
func TestExtractManifest_WrongContentType(t *testing.T) {
	rec := doRequest(http.MethodPost, "/manifest/extract",
		strings.NewReader("hello"), "text/plain")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
}

// TestExtractManifest_InvalidPDF verifies that a request body that is not a
// valid PDF returns 400.
func TestExtractManifest_InvalidPDF(t *testing.T) {
	rec := doRequest(http.MethodPost, "/manifest/extract",
		bytes.NewReader([]byte("%PDF-garbage")), "application/pdf")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestVerify_JSONMatchFalseOnMismatch verifies that tampered content returns
// match=false and status 409 instead of 200.
func TestVerify_JSONMatchFalseOnMismatch(t *testing.T) {
	pdf := compilePDF(t)
	// Flip a byte inside the page content stream to break the match.
	corrupted := append([]byte(nil), pdf...)
	// Find "clause" text in the content stream and flip a byte.
	idx := bytes.Index(corrupted, []byte("("))
	if idx > 0 {
		corrupted[idx+1] ^= 0x01
	}

	rec := doRequest(http.MethodPost, "/verify",
		bytes.NewReader(corrupted), "application/pdf")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for tampered PDF, got %d", rec.Code)
	}
}

// TestEmbedEvidenceAttachesAndRoundTrips verifies POST /evidence/embed attaches
// the posted evidence to the PDF WITHOUT signing (the attach-only seam a remote
// DSS signer needs: embed evidence first so the PAdES ByteRange covers it, then
// have the DSS sign the returned PDF). The result must carry the evidence
// filespec and extract back byte-for-byte via /evidence/extract.
func TestEmbedEvidenceAttachesAndRoundTrips(t *testing.T) {
	baseRec := doRequest(http.MethodPost, "/render",
		strings.NewReader(minimalPayload), "application/ld+json")
	if baseRec.Code != http.StatusOK {
		t.Fatalf("compile base PDF: %d", baseRec.Code)
	}
	basePDF := signPrepared(t, baseRec)

	evidence := []byte(`{"type":["VerifiablePresentation"],"id":"urn:dcs:evidence:test"}`)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(basePDF)
	evPart, _ := mw.CreateFormField("evidence")
	_, _ = evPart.Write(evidence)
	mw.Close()

	rec := doRequest(http.MethodPost, "/evidence/embed", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("expected application/pdf, got %q", ct)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("signing-evidence.json")) {
		t.Error("embedded PDF must contain the signing-evidence.json filespec")
	}

	// The embedded PDF is unsigned (no PAdES dictionary yet — a DSS signs it next).
	if bytes.Contains(rec.Body.Bytes(), []byte("/SubFilter/ETSI.CAdES.detached")) {
		t.Error("/evidence/embed must NOT sign the PDF")
	}

	// Round-trips back out.
	extractRec := doRequest(http.MethodPost, "/evidence/extract",
		bytes.NewReader(rec.Body.Bytes()), "application/pdf")
	if extractRec.Code != http.StatusOK {
		t.Fatalf("extract expected 200, got %d: %s", extractRec.Code, extractRec.Body.String())
	}
	if !bytes.Equal(bytes.TrimSpace(extractRec.Body.Bytes()), evidence) {
		t.Errorf("extracted evidence mismatch: got %q", extractRec.Body.String())
	}
}
