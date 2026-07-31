package pdfcore_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

const testIssuerDID = "did:web:dcs-ionos.facis.cloud"

// TestClientSendsLifecycleAuthority proves this instance tells pdf-core who is
// asserting the provenance it writes. Without it the embedded C2PA lifecycle
// assertion names the contract but never the party claiming it, which is what
// the deployment chart already describes signing.issuerDID as doing.
func TestClientSendsLifecycleAuthority(t *testing.T) {
	for _, tc := range []struct {
		name        string
		preparePath string
		call        func(*pdfcore.Client) error
	}{
		{
			name:        "render",
			preparePath: "/render",
			call: func(c *pdfcore.Client) error {
				_, _, err := c.Download(context.Background(), []byte(`{"@context":"test"}`))
				return err
			},
		},
		{
			name:        "amendment",
			preparePath: "/render/amendment",
			call: func(c *pdfcore.Client) error {
				_, _, err := c.Update(context.Background(), []byte("%PDF-1.7"), []byte(`{"@context":"test"}`), nil, "")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case tc.preparePath:
					got = r.Header.Get("X-DCS-Lifecycle-Authority")
					writePrepared(w, []byte("%PDF prepared"))
				case "/c2pa/embed":
					w.Header().Set("Content-Type", "application/pdf")
					_, _ = w.Write([]byte("%PDF-1.7 embedded"))
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			})

			c := pdfcore.NewWithAuthority(srv.URL, testSign, testIssuerDID)
			require.NoError(t, tc.call(c))
			assert.Equal(t, testIssuerDID, got,
				"pdf-core must be told which instance asserts this render's lifecycle events")
		})
	}
}

// TestClientOmitsEmptyLifecycleAuthority keeps the header off the wire entirely
// when no DID is configured, rather than asserting an empty authority.
func TestClientOmitsEmptyLifecycleAuthority(t *testing.T) {
	present := true
	srv := stubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render":
			_, present = r.Header["X-Dcs-Lifecycle-Authority"]
			writePrepared(w, []byte("%PDF prepared"))
		case "/c2pa/embed":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.7 embedded"))
		}
	})

	c := pdfcore.NewWithAuthority(srv.URL, testSign, "")
	_, _, err := c.Download(context.Background(), []byte(`{"@context":"test"}`))
	require.NoError(t, err)
	assert.False(t, present, "no authority configured must mean no header, not an empty one")
}
