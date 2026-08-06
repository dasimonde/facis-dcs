package pdfcore_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

const testChainPEM = "-----BEGIN CERTIFICATE-----\ndGVzdA==\n-----END CERTIFICATE-----\n"

// TestClientSendsSigningChain proves this instance names the certificate its
// manifests are signed under on every request that renders or verifies one.
// pdf-core holds no signing material — not the dcs-c2pa key, and since it is
// shared by several instances, not the certificate naming it either — so a
// request that omits the chain is one pdf-core cannot serve honestly.
func TestClientSendsSigningChain(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		call func(*pdfcore.Client) error
	}{
		{
			name: "render",
			path: "/render",
			call: func(c *pdfcore.Client) error {
				_, _, err := c.Download(context.Background(), []byte(`{"@context":"test"}`))
				return err
			},
		},
		{
			name: "amendment",
			path: "/render/amendment",
			call: func(c *pdfcore.Client) error {
				_, _, err := c.Update(context.Background(), []byte("%PDF-1.7"), []byte(`{"@context":"test"}`), nil, "")
				return err
			},
		},
		{
			name: "reanchor",
			path: "/render/reanchor",
			call: func(c *pdfcore.Client) error {
				_, err := c.Reanchor(context.Background(), []byte("%PDF-1.7"), "")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case tc.path:
					got = r.Header.Get("X-DCS-C2PA-X5Chain")
					writePrepared(w, []byte("%PDF prepared"))
				case "/c2pa/embed":
					w.Header().Set("Content-Type", "application/pdf")
					_, _ = w.Write([]byte("%PDF-1.7 embedded"))
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			})

			c := pdfcore.New(srv.URL, testSign).WithSigningChain([]byte(testChainPEM))
			require.NoError(t, tc.call(c))
			assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(testChainPEM)), got,
				"pdf-core must be told which certificate this render is signed under")
		})
	}
}

// TestClientSendsSigningChainOnVerify covers the verify side of the same input.
// The replay of a stored manifest runs under the certificate the artifact
// carries; this chain is the separate question of whose signature was expected,
// and it also signs the verification witness.
func TestClientSendsSigningChainOnVerify(t *testing.T) {
	var got string
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-DCS-C2PA-X5Chain")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"match":true,"c2pa_signature_valid":true}`))
	})

	c := pdfcore.New(srv.URL, testSign).WithSigningChain([]byte(testChainPEM))
	_, err := c.Verify(context.Background(), []byte("%PDF-1.7"))
	require.NoError(t, err)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(testChainPEM)), got,
		"a verify must name the identity it expected to find")
}
