package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

func devKeyPEM(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func signES256(t *testing.T, key *ecdsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodES256, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// UC-IF-20 (DCS-IR-CI-05): the BDD stack checks access token signatures. Its
// expired-credential scenarios sign their tokens with a development key that
// DCS_DEV_ACCESS_TOKEN_KEY names, so the expiry is what refuses them. The key
// is accepted only on a stack that declares dev trust, and a token signed with
// any other key stays refused.
func TestADevelopmentKeySignsTheTestTokensOfADevStackOnly(t *testing.T) {
	t.Setenv("JWT_ALG_NONE_SUPPORTED", "")
	srv, providerKey := testProvider(t)
	devKey, devPEM := devKeyPEM(t)
	config := HydraJWTConfig{PublicIssuerURL: srv.URL, ClientID: "dcs-client"}

	t.Setenv("DCS_DEV_ACCESS_TOKEN_KEY", devPEM)
	t.Setenv("DCS_ALLOW_DEV_TRUST", "")
	if _, err := NewHydraJWTValidator(context.Background(), config); err == nil {
		t.Fatal("a development token key was accepted without DCS_ALLOW_DEV_TRUST")
	}

	t.Setenv("DCS_ALLOW_DEV_TRUST", "true")
	validator, err := NewHydraJWTValidator(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	expired := accessTokenClaims(srv.URL, "Template Creator")
	expired["exp"] = time.Now().Add(-time.Hour).Unix()
	_, err = validator.ValidateToken(context.Background(), signES256(t, devKey, expired))
	var expiredErr *oidc.TokenExpiredError
	if !errors.As(err, &expiredErr) {
		t.Fatalf("an expired token signed with the development key = %v, want TokenExpiredError", err)
	}
	if sub := validator.ExpiredTokenSubject(context.Background(), signES256(t, devKey, expired)); sub != "did:example:holder" {
		t.Fatalf("expired token subject = %q, want did:example:holder", sub)
	}
	if _, err := validator.ValidateToken(context.Background(), signRS256(t, providerKey, accessTokenClaims(srv.URL, "Template Creator"))); err != nil {
		t.Fatalf("a token the provider signed was refused: %v", err)
	}
	otherKey, _ := devKeyPEM(t)
	if info, err := validator.ValidateToken(context.Background(), signES256(t, otherKey, accessTokenClaims(srv.URL, "Sys. Administrator"))); err == nil {
		t.Fatalf("a token signed with another key was accepted as %+v", info)
	}
	none := segment(t, map[string]string{"alg": "none", "typ": "JWT"}) + "." + segment(t, accessTokenClaims(srv.URL, "Sys. Administrator")) + "."
	if _, err := validator.ValidateToken(context.Background(), none); err == nil {
		t.Fatal("an unsigned token was accepted")
	}
}
