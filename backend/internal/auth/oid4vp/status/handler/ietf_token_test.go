package handler_test

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/envelope"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"

	"github.com/stretchr/testify/require"
)

func TestIETFToken_Check_StandardCWTWithoutIss(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	const (
		kid     = "12"
		listURI = "http://status.example/statuslists/1"
	)
	now := time.Unix(1719129600, 0).UTC()

	lst := zlibCompress(t, make([]byte, 8))
	claims := map[string]any{
		"sub": listURI,
		"iat": now.Unix(),
		"exp": now.Add(24 * time.Hour).Unix(),
		"status_list": map[string]any{
			"bits": int64(1),
			"lst":  lst,
		},
	}
	signed, err := envelope.SignStatusListCWT(claims, privateKey, kid)
	require.NoError(t, err)

	trust := &status.TrustConfig{
		Issuers: map[string]status.TrustIssuerEntry{
			"http://status.example": {
				JWKS: status.TrustJWKS{Keys: []map[string]any{ecJWK(t, privateKey, kid)}},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/statuslist+cwt")
		_, _ = w.Write(signed)
	}))
	t.Cleanup(srv.Close)

	fetcher := &fetch.Client{HTTPClient: &http.Client{Transport: rewriteHostTransport{base: srv.URL}}}
	tokenHandler := &handler.IETFToken{
		Fetcher: fetcher,
		Trust:   trust,
		Now:     func() time.Time { return now },
	}

	result, err := tokenHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
		URI:       listURI,
		Index:     0,
		Mechanism: status.MechanismIETFToken,
	})
	require.NoError(t, err)
	require.Equal(t, status.StateValid, result.State)
	require.Equal(t, status.MechanismIETFToken, result.Mechanism)
}

func TestIETFToken_Check_RejectsWithoutTrust(t *testing.T) {
	t.Run("jwt", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/statuslist+jwt")
			_, _ = w.Write([]byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig"))
		}))
		t.Cleanup(srv.Close)

		tokenHandler := &handler.IETFToken{
			Fetcher: &fetch.Client{HTTPClient: &http.Client{Transport: rewriteHostTransport{base: srv.URL}}},
			Trust:   nil,
		}

		_, err := tokenHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
			URI:       "http://status.example/statuslists/1",
			Index:     0,
			Mechanism: status.MechanismIETFToken,
		})
		require.ErrorIs(t, err, status.ErrStatusTrustNotConfigured)
	})

	t.Run("cwt", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/statuslist+cwt")
			_, _ = w.Write([]byte{0xd2, 0x84, 0x01, 0x02, 0x03, 0x04})
		}))
		t.Cleanup(srv.Close)

		tokenHandler := &handler.IETFToken{
			Fetcher: &fetch.Client{HTTPClient: &http.Client{Transport: rewriteHostTransport{base: srv.URL}}},
			Trust:   nil,
		}

		_, err := tokenHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
			URI:       "http://status.example/statuslists/1",
			Index:     0,
			Mechanism: status.MechanismIETFToken,
		})
		require.ErrorIs(t, err, status.ErrStatusTrustNotConfigured)
	})
}

func TestIETFToken_Check_RejectsSubjectNotExactlyEqual(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	const kid = "12"
	now := time.Unix(1719129600, 0).UTC()
	refURI := "http://status.example/statuslists%2F1"
	tokenSub := "http://status.example/statuslists/1"

	lst := zlibCompress(t, make([]byte, 8))
	claims := map[string]any{
		"sub": tokenSub,
		"iat": now.Unix(),
		"exp": now.Add(24 * time.Hour).Unix(),
		"status_list": map[string]any{
			"bits": int64(1),
			"lst":  lst,
		},
	}
	signed, err := envelope.SignStatusListCWT(claims, privateKey, kid)
	require.NoError(t, err)

	trust := &status.TrustConfig{
		Issuers: map[string]status.TrustIssuerEntry{
			"http://status.example": {
				JWKS: status.TrustJWKS{Keys: []map[string]any{ecJWK(t, privateKey, kid)}},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/statuslist+cwt")
		_, _ = w.Write(signed)
	}))
	t.Cleanup(srv.Close)

	fetcher := &fetch.Client{HTTPClient: &http.Client{Transport: rewriteHostTransport{base: srv.URL}}}
	tokenHandler := &handler.IETFToken{
		Fetcher: fetcher,
		Trust:   trust,
		Now:     func() time.Time { return now },
	}

	_, err = tokenHandler.Check(context.Background(), status.VerifiedCredential{}, status.Reference{
		URI:       refURI,
		Index:     0,
		Mechanism: status.MechanismIETFToken,
	})
	require.ErrorIs(t, err, status.ErrStatusURIMismatch)
}

func zlibCompress(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func ecJWK(t *testing.T, key *ecdsa.PrivateKey, kid string) map[string]any {
	t.Helper()
	return map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"kid": kid,
		"x":   base64.RawURLEncoding.EncodeToString(key.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(key.Y.Bytes()),
	}
}

type rewriteHostTransport struct {
	base string
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(strings.TrimPrefix(t.base, "https://"), "http://")
	return http.DefaultTransport.RoundTrip(cloned)
}
