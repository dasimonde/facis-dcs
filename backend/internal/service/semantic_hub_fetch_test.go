package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/safehttp"
)

// allowLoopbackSchemaFetch lets the fetch reach the httptest servers on
// 127.0.0.1; every other address rule stays.
func allowLoopbackSchemaFetch(t *testing.T) {
	t.Helper()
	previous := schemaFetchClient
	schemaFetchClient = schemaClient(safehttp.Policy{AllowLoopback: true, DenyPrivate: true})
	t.Cleanup(func() { schemaFetchClient = previous })
}

// DCS-FR-TR-03: a source_url on the host itself is refused.
func TestFetchSchemaFromURLRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("internal"))
	}))
	defer srv.Close()
	_, err := fetchSchemaFromURL(context.Background(), srv.URL)
	require.ErrorContains(t, err, "loopback")
}

// DCS-FR-TR-03: a redirect does not lead the fetch to a link-local or a
// private address.
func TestFetchSchemaFromURLRefusesRedirectToInternalAddress(t *testing.T) {
	allowLoopbackSchemaFetch(t)
	for target, reason := range map[string]string{
		"http://169.254.169.254/latest/meta-data/": "link-local",
		"http://10.0.0.1/admin":                    "private",
	} {
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target, http.StatusFound)
		}))
		_, err := fetchSchemaFromURL(context.Background(), redirector.URL)
		redirector.Close()
		require.ErrorContains(t, err, reason)
	}
}

func TestFetchSchemaFromURLFollowsRedirects(t *testing.T) {
	allowLoopbackSchemaFetch(t)
	const body = "@prefix ex: <http://example.org/> . ex:s a ex:Shape ."
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer final.Close()
	// A w3id/purl-style redirect to the real document.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	got, err := fetchSchemaFromURL(context.Background(), redirector.URL)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

// The anchor a template declares in sh:shapesGraph is a hub URL that serves a
// SemanticSchemaItem envelope, so installing the peer's library from exactly
// that anchor has to store the shapes graph — not the JSON around it.
func TestFetchSchemaFromURLUnwrapsAPeerHubAnchor(t *testing.T) {
	allowLoopbackSchemaFetch(t)
	const shapes = "@prefix ex: <http://example.org/> . ex:s a ex:Shape ."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"e2e-sla-shapes","version":3,"kind":"shapes",` +
			`"media_type":"text/turtle","content":` + strconv.Quote(shapes) +
			`,"active":true,"created_by":"someone","created_at":"2026-07-29T00:00:00Z"}`))
	}))
	defer srv.Close()

	got, err := fetchSchemaFromURL(context.Background(), srv.URL+"/semantic/shapes/e2e-sla-shapes?version=3")
	require.NoError(t, err)
	require.Equal(t, shapes, got)
}

// Outside a hub path, and without the envelope's full shape, the fetched bytes
// are the schema — a JSON-LD context with a "content" term stays verbatim.
func TestFetchSchemaFromURLKeepsANonHubJSONDocumentVerbatim(t *testing.T) {
	allowLoopbackSchemaFetch(t)
	const doc = `{"@context":{"content":"http://example.org/content"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(doc))
	}))
	defer srv.Close()

	got, err := fetchSchemaFromURL(context.Background(), srv.URL+"/semantic/context/partner")
	require.NoError(t, err)
	require.Equal(t, doc, got)
}

func TestFetchSchemaFromURLRejectsNonHTTPScheme(t *testing.T) {
	_, err := fetchSchemaFromURL(context.Background(), "file:///etc/passwd")
	require.ErrorContains(t, err, "http or https")
}

func TestFetchSchemaFromURLRejectsNon200(t *testing.T) {
	allowLoopbackSchemaFetch(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := fetchSchemaFromURL(context.Background(), srv.URL)
	require.ErrorContains(t, err, "HTTP 404")
}
