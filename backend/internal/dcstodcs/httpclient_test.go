package dcstodcs

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type recordingDoer struct {
	schemes []string
	bodies  []string
	// respond decides each attempt's outcome by 0-based attempt index.
	respond func(attempt int) (*http.Response, error)
}

func (d *recordingDoer) Do(req *http.Request) (*http.Response, error) {
	attempt := len(d.schemes)
	d.schemes = append(d.schemes, req.URL.Scheme)
	body := ""
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		body = string(b)
	}
	d.bodies = append(d.bodies, body)
	return d.respond(attempt)
}

// postRequest addresses a loopback peer, the kind of host did:web may be
// resolved from over plain http.
func postRequest(t *testing.T, payload string) *http.Request {
	t.Helper()
	return postRequestTo(t, "peer.localhost:8443", payload)
}

func postRequestTo(t *testing.T, host, payload string) *http.Request {
	t.Helper()
	// http.NewRequest populates GetBody for a *bytes.Reader, which is what makes
	// the request replayable — the same shape the Goa client encoder produces.
	req, err := http.NewRequest(http.MethodPost, "http://"+host+"/api/pdf", bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return req
}

// UC-IF-5: toward a peer that did:web resolves over https only, a TLS failure
// is an error. The PDF and its signed challenge never go out over plain http.
func TestSchemeFallbackDoerNeverDowngradesAnHTTPSOnlyPeer(t *testing.T) {
	t.Setenv("DCS_DIDWEB_INSECURE_HOSTS", "orce.internal:1880")
	for _, host := range []string{"peer.example", "peer.example:8443", "10.0.0.7:443"} {
		inner := &recordingDoer{respond: func(int) (*http.Response, error) {
			return nil, errors.New("tls: failed to verify certificate")
		}}
		d := &schemeFallbackDoer{inner: inner}

		if _, err := d.Do(postRequestTo(t, host, "payload")); err == nil {
			t.Fatalf("%s: expected the https error", host)
		}
		if want := []string{"https"}; !equal(inner.schemes, want) {
			t.Errorf("%s: schemes = %v, want %v", host, inner.schemes, want)
		}
	}

	// A host the deployment names as plain-http capable still falls back.
	inner := &recordingDoer{respond: func(attempt int) (*http.Response, error) {
		if attempt == 0 {
			return nil, errors.New("connection refused")
		}
		return okResponse(), nil
	}}
	if _, err := (&schemeFallbackDoer{inner: inner}).Do(postRequestTo(t, "orce.internal:1880", "payload")); err != nil {
		t.Fatalf("allow-listed host: %v", err)
	}
	if want := []string{"https", "http"}; !equal(inner.schemes, want) {
		t.Errorf("allow-listed host: schemes = %v, want %v", inner.schemes, want)
	}
}

// The same over real sockets: the peer peer.example answers https with a
// certificate this instance does not trust, and plain http on a listener that
// counts what it receives.
func TestAnUntrustedPeerCertificateSendsNothingInClearText(t *testing.T) {
	var plain atomic.Int32
	clear := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { plain.Add(1) }))
	defer clear.Close()
	tlsPeer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer tlsPeer.Close()

	dialer := &net.Dialer{}
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, strings.TrimPrefix(clear.URL, "http://"))
		},
		DialTLSContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, strings.TrimPrefix(tlsPeer.URL, "https://"))
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(conn, &tls.Config{ServerName: "peer.example", MinVersion: tls.VersionTLS13})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}}
	d := &schemeFallbackDoer{inner: client}

	if _, err := d.Do(postRequestTo(t, "peer.example", "payload")); err == nil {
		t.Fatal("a peer certificate this instance does not trust must fail the request")
	}
	if plain.Load() != 0 {
		t.Fatalf("%d request(s) went out over plain http", plain.Load())
	}
}

func okResponse() *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}
}

func TestSchemeFallbackDoerPrefersHTTPS(t *testing.T) {
	inner := &recordingDoer{respond: func(int) (*http.Response, error) { return okResponse(), nil }}
	d := &schemeFallbackDoer{inner: inner}

	if _, err := d.Do(postRequest(t, "payload")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"https"}; !equal(inner.schemes, want) {
		t.Errorf("schemes = %v, want %v (https must be tried first and suffice)", inner.schemes, want)
	}
}

func TestSchemeFallbackDoerFallsBackToHTTPOnTransportFailure(t *testing.T) {
	inner := &recordingDoer{respond: func(attempt int) (*http.Response, error) {
		if attempt == 0 {
			return nil, errors.New("connection refused")
		}
		return okResponse(), nil
	}}
	d := &schemeFallbackDoer{inner: inner}

	resp, err := d.Do(postRequest(t, "payload"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if want := []string{"https", "http"}; !equal(inner.schemes, want) {
		t.Errorf("schemes = %v, want %v", inner.schemes, want)
	}
	// The retry must carry the same body, or a plain-http peer would be shipped
	// an empty request.
	if want := []string{"payload", "payload"}; !equal(inner.bodies, want) {
		t.Errorf("bodies = %v, want %v", inner.bodies, want)
	}
}

// An HTTP response means the peer processed the request. Retrying it over http
// would ship a second copy of the same contract PDF.
func TestSchemeFallbackDoerDoesNotRetryOnHTTPErrorStatus(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusInternalServerError, http.StatusPermanentRedirect} {
		inner := &recordingDoer{respond: func(int) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: http.NoBody}, nil
		}}
		d := &schemeFallbackDoer{inner: inner}

		if _, err := d.Do(postRequest(t, "payload")); err != nil {
			t.Fatalf("status %d: unexpected error: %v", status, err)
		}
		if len(inner.schemes) != 1 {
			t.Errorf("status %d: made %d attempts (%v), want exactly 1", status, len(inner.schemes), inner.schemes)
		}
	}
}

func TestSchemeFallbackDoerReportsBothFailures(t *testing.T) {
	inner := &recordingDoer{respond: func(int) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}}
	d := &schemeFallbackDoer{inner: inner}

	_, err := d.Do(postRequest(t, "payload"))
	if err == nil {
		t.Fatal("expected an error when neither scheme reaches the peer")
	}
	for _, want := range []string{"https", "http", "connection refused"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
	if want := []string{"https", "http"}; !equal(inner.schemes, want) {
		t.Errorf("schemes = %v, want %v", inner.schemes, want)
	}
}

// A body with no GetBody cannot be rewound, so there is nothing honest to retry.
func TestSchemeFallbackDoerDoesNotRetryUnreplayableBody(t *testing.T) {
	inner := &recordingDoer{respond: func(int) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}}
	d := &schemeFallbackDoer{inner: inner}

	req := postRequest(t, "payload")
	req.GetBody = nil

	if _, err := d.Do(req); err == nil {
		t.Fatal("expected the https error to surface")
	}
	if len(inner.schemes) != 1 {
		t.Errorf("made %d attempts (%v), want exactly 1", len(inner.schemes), inner.schemes)
	}
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
