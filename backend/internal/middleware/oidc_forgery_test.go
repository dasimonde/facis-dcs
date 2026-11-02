package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testProvider is an OpenID provider that publishes discovery metadata and
// one RSA key in its JWKS, like Hydra does.
func testProvider(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"jwks_uri":                              srv.URL + "/jwks",
			"authorization_endpoint":                srv.URL + "/oauth2/auth",
			"token_endpoint":                        srv.URL + "/oauth2/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA",
			"kid": "hydra-key",
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	return srv, key
}

func accessTokenClaims(issuer string, roles ...string) jwt.MapClaims {
	return jwt.MapClaims{
		"iss":       issuer,
		"sub":       "did:example:holder",
		"client_id": "dcs-client",
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(time.Hour).Unix(),
		"ext":       map[string]any{"iss": "did:web:participant.example", "roles": roles},
	}
}

func signRS256(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "hydra-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func segment(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// The validator accepts only tokens the provider signed. JWT_ALG_NONE_SUPPORTED
// is unset here, as in every deployment outside the BDD stack.
func TestValidateTokenRefusesForgedTokens(t *testing.T) {
	t.Setenv("JWT_ALG_NONE_SUPPORTED", "")
	srv, providerKey := testProvider(t)
	validator, err := NewHydraJWTValidator(context.Background(), HydraJWTConfig{
		PublicIssuerURL: srv.URL,
		ClientID:        "dcs-client",
	})
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}

	genuine := signRS256(t, providerKey, accessTokenClaims(srv.URL, "Template Creator"))
	info, err := validator.ValidateToken(context.Background(), genuine)
	if err != nil {
		t.Fatalf("a token the provider signed was refused: %v", err)
	}
	if info.HolderDID != "did:example:holder" || len(info.Roles) != 1 || info.Roles[0] != "Template Creator" {
		t.Fatalf("unexpected identity from a genuine token: %+v", info)
	}

	attackerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(genuine, ".")
	elevated := accessTokenClaims(srv.URL, "Sys. Administrator")

	forged := map[string]string{
		"signed with another key under the provider's kid": signRS256(t, attackerKey, elevated),
		"payload changed under the genuine signature":      parts[0] + "." + segment(t, elevated) + "." + parts[2],
		"alg none":           segment(t, map[string]string{"alg": "none", "typ": "JWT"}) + "." + segment(t, elevated) + ".",
		"signature stripped": parts[0] + "." + parts[1] + ".",
		"HS256 keyed with the provider's public modulus": func() string {
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, elevated)
			token.Header["kid"] = "hydra-key"
			signed, err := token.SignedString(providerKey.N.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			return signed
		}(),
	}
	for name, token := range forged {
		t.Run(name, func(t *testing.T) {
			if info, err := validator.ValidateToken(context.Background(), token); err == nil {
				t.Fatalf("forged token accepted as %+v", info)
			}
		})
	}
}

// The provider metadata names the JWKS under the public issuer URL, which a
// pod need not reach. The validator fetches it from the internal issuer URL.
func TestTheJWKSIsFetchedFromTheInternalIssuerURL(t *testing.T) {
	t.Setenv("JWT_ALG_NONE_SUPPORTED", "")
	t.Setenv("DCS_DEV_ACCESS_TOKEN_KEY", "")
	const public = "http://public.invalid"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	internal := httptest.NewServer(mux)
	t.Cleanup(internal.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                public,
			"jwks_uri":                              public + "/.well-known/jwks.json",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": "hydra-key", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	validator, err := NewHydraJWTValidator(context.Background(), HydraJWTConfig{
		PublicIssuerURL: public, InternalIssuerURL: internal.URL, ClientID: "dcs-client",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.ValidateToken(context.Background(), signRS256(t, key, accessTokenClaims(public, "Template Creator"))); err != nil {
		t.Fatalf("a token the provider signed was refused: %v", err)
	}
}
