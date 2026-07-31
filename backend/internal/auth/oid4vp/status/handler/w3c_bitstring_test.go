package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"

	"github.com/stretchr/testify/require"
)

func TestW3CBitstring_Check_RejectsWithoutTrust(t *testing.T) {
	t.Run("jwt", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/vc+jwt")
			_, _ = w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig"))
		}))
		t.Cleanup(srv.Close)

		bitstringHandler := newW3CHandlerWithoutTrust(t, srv.URL)
		_, err := bitstringHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
			URI:       srv.URL,
			Index:     0,
			Mechanism: status.MechanismW3CBitstring,
		})
		require.ErrorIs(t, err, status.ErrStatusTrustNotConfigured)
	})

	t.Run("data integrity with proof", func(t *testing.T) {
		body := []byte(`{
			"type": ["VerifiableCredential","BitstringStatusListCredential"],
			"credentialSubject": {
				"type": "BitstringStatusList",
				"encodedList": "uH4sIAAAAAAAAAAD6PwpGwSgYsQAQAAD//9T_OrgABAAA"
			},
			"proof": {"type":"DataIntegrityProof"}
		}`)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/ld+json")
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)

		bitstringHandler := newW3CHandlerWithoutTrust(t, srv.URL)
		_, err := bitstringHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
			URI:       srv.URL,
			Index:     0,
			Mechanism: status.MechanismW3CBitstring,
		})
		require.ErrorIs(t, err, status.ErrStatusTrustNotConfigured)
	})

	t.Run("cose", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/vc+cose")
			_, _ = w.Write([]byte{0xd2, 0x84, 0x01, 0x02, 0x03, 0x04})
		}))
		t.Cleanup(srv.Close)

		bitstringHandler := newW3CHandlerWithoutTrust(t, srv.URL)
		_, err := bitstringHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
			URI:       srv.URL,
			Index:     0,
			Mechanism: status.MechanismW3CBitstring,
		})
		require.ErrorIs(t, err, status.ErrStatusTrustNotConfigured)
	})
}

func newW3CHandlerWithoutTrust(t *testing.T, srvURL string) *handler.W3CBitstring {
	t.Helper()
	return &handler.W3CBitstring{
		Fetcher: &fetch.Client{HTTPClient: &http.Client{Transport: rewriteHostTransport{base: srvURL}}},
		Trust:   nil,
	}
}
