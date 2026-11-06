package hsm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestECPublicKeyJWKEncodesP256Coordinates(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwk, err := ECPublicKeyJWK(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, coord := range []string{"x", "y"} {
		raw, err := base64.RawURLEncoding.DecodeString(jwk[coord].(string))
		if err != nil || len(raw) != 32 {
			t.Fatalf("%s: %d bytes, err %v", coord, len(raw), err)
		}
	}
}

func TestECPublicKeyJWKRefusesKeysItCannotEncode(t *testing.T) {
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, pub := range map[string]*ecdsa.PublicKey{"nil": nil, "P-384": &p384.PublicKey} {
		if jwk, err := ECPublicKeyJWK(pub); err == nil {
			t.Errorf("%s: got %v, want error", name, jwk)
		}
	}
}
