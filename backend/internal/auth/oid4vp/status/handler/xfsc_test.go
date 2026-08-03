package handler_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnwrapStatusListJWTBody(t *testing.T) {
	raw := []byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig")
	quoted := []byte(`"eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig"`)

	require.Equal(t, raw, handler.UnwrapStatusListJWTBody(raw))
	require.Equal(t, raw, handler.UnwrapStatusListJWTBody(quoted))
}

func makeXFSCUnsignedBody(bitstring []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(bitstring)
	_ = w.Close()
	body, _ := json.Marshal(map[string]any{
		"tenantId": "default",
		"listId":   1,
		"list":     base64.RawStdEncoding.EncodeToString(buf.Bytes()),
	})
	return body
}

func TestXFSC_Check_RefusesAnUnsignedListEvenWhenPrefetched(t *testing.T) {
	var signedRequests atomic.Int32
	unsignedBody := makeXFSCUnsignedBody(make([]byte, 16))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			signedRequests.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.Fatalf("unexpected request headers: Accept=%q Content-Type=%q", r.Header.Get("Accept"), r.Header.Get("Content-Type"))
	}))
	t.Cleanup(srv.Close)

	fetcher := &fetch.Client{HTTPClient: &http.Client{Transport: localRoundTripper{base: srv.URL}}}
	xfscHandler := &handler.XFSC{Fetcher: fetcher}

	result, err := xfscHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
		Mechanism: status.MechanismXFSC,
		URI:       srv.URL,
		Index:     0,
		Prefetched: &fetch.Response{
			ContentType: "application/json",
			Body:        unsignedBody,
		},
	})
	// An unsigned list states a revocation status nobody vouched for, and
	// anyone who can answer the URL can write it. Having one already in hand is
	// not a reason to believe it.
	require.Error(t, err, "an unsigned status list must never decide a credential's validity")
	assert.NotEqual(t, status.StateValid, result.State)
	assert.Equal(t, int32(1), signedRequests.Load(), "XFSC must still attempt signed retrieval")
}

type localRoundTripper struct {
	base string
}

func (t localRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(strings.TrimPrefix(t.base, "https://"), "http://")
	return http.DefaultTransport.RoundTrip(cloned)
}
