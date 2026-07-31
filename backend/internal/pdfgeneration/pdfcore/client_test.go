package pdfcore_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

const testVersion = "1.0.1"

// testSign is the in-process dcs-c2pa stand-in: it returns a fixed 64-byte ES256
// r||s for any Sig_structure.
func testSign(_ []byte) ([]byte, error) {
	sig := make([]byte, 64)
	for i := range sig {
		sig[i] = byte(i)
	}
	return sig, nil
}

func newClient(url string) *pdfcore.Client { return pdfcore.New(url, testSign) }

func stubServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// writePrepared responds like pdf-core's prepare step: a JSON envelope carrying a
// PDF and one Sig_structure to sign.
func writePrepared(w http.ResponseWriter, preparedPDF []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-PDF-Core-Version", testVersion)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"pdf_base64":          base64.StdEncoding.EncodeToString(preparedPDF),
		"c2pa_sig_structures": []string{base64.StdEncoding.EncodeToString([]byte("sig-structure"))},
	})
}

// TestClientVersion verifies that Version calls GET /version and parses the
// version string from the JSON response body.
func TestClientVersion(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/version", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":"`+testVersion+`"}`)
	})

	c := newClient(srv.URL)
	v, err := c.Version(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testVersion, v)
}

// TestClientDownload verifies Download posts JSON-LD to /download (prepare), signs
// the returned Sig_structure, posts it to /c2pa/embed, and returns the embedded
// PDF plus the renderer version from the prepare header.
func TestClientDownload(t *testing.T) {
	fakePDF := []byte("%PDF-1.7 embedded")
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render":
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/ld+json", r.Header.Get("Content-Type"))
			body, _ := io.ReadAll(r.Body)
			assert.Equal(t, `{"@context":"test"}`, string(body))
			writePrepared(w, []byte("%PDF prepared"))
		case "/c2pa/embed":
			var req struct {
				C2PASignatures []string `json:"c2pa_signatures"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			assert.Len(t, req.C2PASignatures, 1, "one signature per Sig_structure")
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(fakePDF)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	c := newClient(srv.URL)
	pdf, ver, err := c.Download(context.Background(), []byte(`{"@context":"test"}`))
	require.NoError(t, err)
	assert.Equal(t, fakePDF, pdf)
	assert.Equal(t, testVersion, ver)
}

// TestClientUpdate verifies Update sends a multipart prepare request then embeds.
func TestClientUpdate(t *testing.T) {
	fakePDF := []byte("%PDF-1.7 updated")
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render/amendment":
			require.NoError(t, r.ParseMultipartForm(8<<20))
			assert.NotEmpty(t, r.FormValue("pdf"), "pdf field must be present")
			assert.NotEmpty(t, r.FormValue("payload"), "payload field must be present")
			assert.Empty(t, r.FormValue("vc"))
			assert.Empty(t, r.FormValue("manifest_url"))
			writePrepared(w, []byte("%PDF prepared update"))
		case "/c2pa/embed":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(fakePDF)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	c := newClient(srv.URL)
	pdf, ver, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), nil, "")
	require.NoError(t, err)
	assert.Equal(t, fakePDF, pdf)
	assert.Equal(t, testVersion, ver)
}

// TestClientUpdateWithManifestURL verifies Update sends a "manifest_url" field.
func TestClientUpdateWithManifestURL(t *testing.T) {
	const manifestURL = "https://dcs.example/c2pa/manifest/did:example:contract-1"
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render/amendment":
			require.NoError(t, r.ParseMultipartForm(8<<20))
			assert.Equal(t, manifestURL, r.FormValue("manifest_url"), "manifest_url field must match supplied URL")
			writePrepared(w, []byte("%PDF prepared"))
		case "/c2pa/embed":
			_, _ = w.Write([]byte("%PDF embedded"))
		}
	})

	c := newClient(srv.URL)
	_, _, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), nil, manifestURL)
	require.NoError(t, err)
}

// TestClientUpdateWithVC verifies Update sends a "vc" multipart field.
func TestClientUpdateWithVC(t *testing.T) {
	vcBytes := []byte(`{"type":["VerifiableCredential"]}`)
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render/amendment":
			require.NoError(t, r.ParseMultipartForm(8<<20))
			assert.Equal(t, string(vcBytes), r.FormValue("vc"), "vc field must match supplied bytes")
			writePrepared(w, []byte("%PDF prepared"))
		case "/c2pa/embed":
			_, _ = w.Write([]byte("%PDF embedded"))
		}
	})

	c := newClient(srv.URL)
	_, _, err := c.Update(context.Background(),
		[]byte("%PDF existing"), []byte(`{"@context":"test"}`), vcBytes, "")
	require.NoError(t, err)
}

// TestClientHTTPErrorPropagated verifies that a non-2xx prepare response is
// returned as an error (hard-fail — no silent fallbacks).
func TestClientHTTPErrorPropagated(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"name":"internal_error","message":"boom"}`, http.StatusInternalServerError)
	})

	c := newClient(srv.URL)
	_, _, err := c.Download(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// TestClientMatchContent verifies MatchContent posts BOTH documents to
// /verify/content-match as the "pdf" and "reference" multipart fields, and
// returns the match verdict with its diagnostic.
func TestClientMatchContent(t *testing.T) {
	submitted := []byte("%PDF-1.7 submitted")
	reference := []byte("%PDF-1.7 prepared")
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/verify/content-match", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(8<<20))
		assert.Equal(t, string(submitted), r.FormValue("pdf"))
		assert.Equal(t, string(reference), r.FormValue("reference"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"match":false,"mismatch":"page 1 content does not match"}`)
	})

	c := newClient(srv.URL)
	match, mismatch, err := c.MatchContent(context.Background(), submitted, reference)
	require.NoError(t, err)
	assert.False(t, match)
	assert.Equal(t, "page 1 content does not match", mismatch)
}

// TestClientMatchContentErrorPropagated proves an unreachable or failing
// content-match is an error, never a silent pass: the caller refuses the
// signature rather than accepting it unchecked.
func TestClientMatchContentErrorPropagated(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"name":"internal_error","message":"boom"}`, http.StatusInternalServerError)
	})

	c := newClient(srv.URL)
	match, _, err := c.MatchContent(context.Background(), []byte("a"), []byte("b"))
	require.Error(t, err)
	assert.False(t, match)
}

// pdf-core reports the SHA-256 digests its match verdict was reached on. They
// were being dropped on the floor, which is why the three hash fields on the
// verify response encoded as "" on every single call.
func TestClientVerifyCarriesTheDigestsTheVerdictWasReachedOn(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/verify", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"match":                true,
			"c2pa_signature_valid": true,
			"vc_present":           false,
			"jsonld_hash":          "aa",
			"base_pdf_hash":        "bb",
			"stored_base_pdf_hash": "bb",
		})
	})

	result, err := newClient(srv.URL).Verify(context.Background(), []byte("%PDF-"))
	require.NoError(t, err)
	assert.Equal(t, "aa", result.JSONLDHash)
	assert.Equal(t, "bb", result.BasePDFHash)
	assert.Equal(t, "bb", result.StoredBasePDFHash)
}

// A content mismatch is exactly the case where the two PDF digests are worth
// reporting — they say WHICH side diverged. The 409 stays an error; the evidence
// rides along with it rather than being discarded with the body.
func TestClientVerifyKeepsTheDigestsFromAContentMismatch(t *testing.T) {
	srv := stubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":                 "conflict",
			"message":              "embedded payload does not reproduce the submitted PDF",
			"jsonld_hash":          "aa",
			"base_pdf_hash":        "bb",
			"stored_base_pdf_hash": "cc",
		})
	})

	result, err := newClient(srv.URL).Verify(context.Background(), []byte("%PDF-"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 409")
	assert.False(t, result.Match)
	assert.Equal(t, "aa", result.JSONLDHash)
	assert.Equal(t, "bb", result.BasePDFHash)
	assert.Equal(t, "cc", result.StoredBasePDFHash)
}
