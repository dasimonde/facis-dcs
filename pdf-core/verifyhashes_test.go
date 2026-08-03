package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	compiler "example.com/m/V2/compiler"
)

// verifyHashes is the hash triple /verify reports, on both the 200 body and the
// 409 error body.
type verifyHashes struct {
	JSONLDHash        string `json:"jsonld_hash"`
	BasePDFHash       string `json:"base_pdf_hash"`
	StoredBasePDFHash string `json:"stored_base_pdf_hash"`
}

func decodeHashes(t *testing.T, body []byte) verifyHashes {
	t.Helper()
	var h verifyHashes
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode verify hashes: %v (body: %s)", err, body)
	}
	return h
}

// wantSHA256Hex is the test's own digest, computed independently of the
// production helper so the assertion checks the value and not just the call.
func wantSHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestVerify_ReturnsTheHashesTheVerdictWasReachedOn proves /verify reports the
// digests its own match decision is made of, instead of leaving the caller with
// a bare boolean it cannot audit. jsonld_hash is the embedded payload; the two
// PDF hashes are the deterministic re-render and the stored bytes it was
// compared against, so on an intact artifact they are equal.
func TestVerify_ReturnsTheHashesTheVerdictWasReachedOn(t *testing.T) {
	pdf := compilePDF(t)

	rec := doRequest(http.MethodPost, "/verify", bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}
	hashes := decodeHashes(t, rec.Body.Bytes())

	payload, err := compiler.ExtractEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("extract embedded payload: %v", err)
	}
	if want := wantSHA256Hex(payload); hashes.JSONLDHash != want {
		t.Errorf("jsonld_hash = %q, want the SHA-256 of the embedded payload %q", hashes.JSONLDHash, want)
	}
	if len(hashes.BasePDFHash) != 64 {
		t.Errorf("base_pdf_hash = %q, want a 64-hex SHA-256 digest", hashes.BasePDFHash)
	}
	if len(hashes.StoredBasePDFHash) != 64 {
		t.Errorf("stored_base_pdf_hash = %q, want a 64-hex SHA-256 digest", hashes.StoredBasePDFHash)
	}
	if hashes.BasePDFHash != hashes.StoredBasePDFHash {
		t.Errorf("an intact artifact must re-render to its stored base bytes: base_pdf_hash=%q stored_base_pdf_hash=%q",
			hashes.BasePDFHash, hashes.StoredBasePDFHash)
	}
}

// TestVerify_TamperedPDFReportsDivergingBaseHashes edits only the visible page
// content stream, leaving the embedded machine-readable payload untouched. The
// verdict stays a mismatch (409), and the two PDF hashes now differ — which is
// the evidence FOR the verdict, not merely a restatement of it. jsonld_hash is
// unchanged, naming the untouched payload as the side that still holds.
func TestVerify_TamperedPDFReportsDivergingBaseHashes(t *testing.T) {
	pdf := compilePDF(t)

	tampered := bytes.Replace(pdf, []byte("(clause one) Tj"), []byte("(clause TWO) Tj"), 1)
	if bytes.Equal(tampered, pdf) {
		t.Fatal("test setup: page-content clause literal not found to tamper")
	}

	rec := doRequest(http.MethodPost, "/verify", bytes.NewReader(tampered), "application/pdf")
	if rec.Code != http.StatusConflict {
		t.Fatalf("verify of a tampered PDF: status %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if name := errorName(t, rec.Body.Bytes()); name != "conflict" {
		t.Fatalf("error name = %q, want conflict", name)
	}

	hashes := decodeHashes(t, rec.Body.Bytes())
	payload, err := compiler.ExtractEmbeddedJSONLD(tampered)
	if err != nil {
		t.Fatalf("extract embedded payload: %v", err)
	}
	if want := wantSHA256Hex(payload); hashes.JSONLDHash != want {
		t.Errorf("jsonld_hash = %q, want %q — the payload was not tampered with", hashes.JSONLDHash, want)
	}
	if len(hashes.BasePDFHash) != 64 || len(hashes.StoredBasePDFHash) != 64 {
		t.Fatalf("a mismatch must still report both digests: base=%q stored=%q",
			hashes.BasePDFHash, hashes.StoredBasePDFHash)
	}
	if hashes.BasePDFHash == hashes.StoredBasePDFHash {
		t.Error("tampered page content must make the stored base diverge from its re-render")
	}
}

// TestVerify_AmendedPDFReturnsTheHashesOfTheReplayedChain proves the hashes are
// reported for an incrementally updated artifact too — the branch that replays
// the amendment chain rather than recompiling a bare base — and that they agree
// on an honestly amended document.
func TestVerify_AmendedPDFReturnsTheHashesOfTheReplayedChain(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pdfPart, _ := mw.CreateFormField("pdf")
	_, _ = pdfPart.Write(compilePDF(t))
	payloadPart, _ := mw.CreateFormField("payload")
	_, _ = payloadPart.Write([]byte(minimalPayloadAmended))
	_ = mw.Close()

	amendRec := doRequest(http.MethodPost, "/render/amendment", &buf, mw.FormDataContentType())
	if amendRec.Code != http.StatusOK {
		t.Fatalf("amend: status %d: %s", amendRec.Code, amendRec.Body.String())
	}
	amended := signPrepared(t, amendRec)

	rec := doRequest(http.MethodPost, "/verify", bytes.NewReader(amended), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}
	hashes := decodeHashes(t, rec.Body.Bytes())

	payload, err := compiler.ExtractLatestEmbeddedJSONLD(amended)
	if err != nil {
		t.Fatalf("extract latest embedded payload: %v", err)
	}
	if want := wantSHA256Hex(payload); hashes.JSONLDHash != want {
		t.Errorf("jsonld_hash = %q, want the SHA-256 of the LATEST embedded payload %q", hashes.JSONLDHash, want)
	}
	if hashes.BasePDFHash != hashes.StoredBasePDFHash {
		t.Errorf("an honestly amended artifact must replay to its stored bytes: base=%q stored=%q",
			hashes.BasePDFHash, hashes.StoredBasePDFHash)
	}
}
