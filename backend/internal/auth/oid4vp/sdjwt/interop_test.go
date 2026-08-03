package sdjwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// RFC 7515 §4.1.9 lets typ carry the full media type, and the SD-JWT VC subtype
// was renamed after most deployed issuers had shipped: a credential is refused
// for what it says, not for how its issuer's library spells the header.
func TestCredentialHeaderAcceptsEverySDJWTVCSpelling(t *testing.T) {
	for _, typ := range []string{"dc+sd-jwt", "application/dc+sd-jwt", "vc+sd-jwt", "application/vc+sd-jwt"} {
		if err := validateCredentialHeader(&jwt.Token{Header: map[string]any{"typ": typ}}); err != nil {
			t.Errorf("typ %q was refused: %v", typ, err)
		}
	}
	if err := validateCredentialHeader(&jwt.Token{Header: map[string]any{"typ": "JWT"}}); err == nil {
		t.Error("a plain JWT was accepted as a credential")
	}
}

func TestKBHeaderAcceptsTheFullMediaType(t *testing.T) {
	for _, typ := range []string{"kb+jwt", "application/kb+jwt"} {
		if err := validateKBHeader(&jwt.Token{Header: map[string]any{"typ": typ}}); err != nil {
			t.Errorf("typ %q was refused: %v", typ, err)
		}
	}
}

func holderCNF(t *testing.T) (jwt.MapClaims, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate holder key: %v", err)
	}
	jwk := JWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64URL(key.X),
		Y:   base64URL(key.Y),
	}
	did, err := DIDJWKFromPublicJWK(jwk)
	if err != nil {
		t.Fatalf("did:jwk: %v", err)
	}
	return jwt.MapClaims{"cnf": map[string]any{"jwk": map[string]any{
		"kty": jwk.Kty, "crv": jwk.Crv, "x": jwk.X, "y": jwk.Y,
	}}}, did
}

// SD-JWT VC makes sub OPTIONAL and its value arbitrary — cnf is the holder
// binding. A third-party issuer that names its own subject identifier, or none
// at all, still yields a verifiable credential.
func TestHolderSubject(t *testing.T) {
	claims, bindingDID := holderCNF(t)

	got, err := HolderSubject(claims)
	if err != nil {
		t.Fatalf("a credential without sub was refused: %v", err)
	}
	if got != bindingDID {
		t.Fatalf("sub was not derived from cnf.jwk: %q", got)
	}

	claims["sub"] = "urn:eudi:pid:de:1:2f9a"
	got, err = HolderSubject(claims)
	if err != nil {
		t.Fatalf("an issuer-chosen sub was refused: %v", err)
	}
	if got != "urn:eudi:pid:de:1:2f9a" {
		t.Fatalf("issuer-chosen sub was rewritten to %q", got)
	}

	other, _ := holderCNF(t)
	otherDID, err := HolderSubject(other)
	if err != nil {
		t.Fatalf("derive other did: %v", err)
	}
	claims["sub"] = otherDID
	if _, err := HolderSubject(claims); err == nil {
		t.Fatal("a did:jwk sub naming a different key than cnf.jwk was accepted")
	}
}

func base64URL(v *big.Int) string {
	return base64.RawURLEncoding.EncodeToString(v.FillBytes(make([]byte, 32)))
}

// --- x5c leaves as real issuers actually carry them ---

func mintRSALeaf(t *testing.T, template *x509.Certificate, signer *ecdsa.PrivateKey, signerCert *x509.Certificate) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	template.SerialNumber = big.NewInt(time.Now().UnixNano())
	template.NotBefore = time.Now().Add(-time.Hour)
	template.NotAfter = time.Now().Add(time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, template, signerCert, &key.PublicKey, signer)
	if err != nil {
		t.Fatalf("create rsa certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse rsa certificate: %v", err)
	}
	return cert
}

// Real PID and QTSP issuer certificates are frequently RSA; refusing them for
// the key type alone rejects credentials that are otherwise fully verifiable.
func TestVerificationKeyFromX5C_RSALeafIsAccepted(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leaf := mintRSALeaf(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "Real PID Issuer"},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	key, err := verificationKeyFromX5C(x5cHeaderValue(leaf), roots, "Real PID Issuer")
	if err != nil {
		t.Fatalf("an rsa issuer certificate was refused: %v", err)
	}
	if _, ok := key.(*rsa.PublicKey); !ok {
		t.Fatalf("expected an *rsa.PublicKey, got %T", key)
	}
}

func TestVerificationKeyFromX5C_P384LeafIsAccepted(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate p-384 key: %v", err)
	}
	leaf := mintTestCertFromAnyKey(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "Real PID Issuer"},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}, &leafKey.PublicKey, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	if _, err := verificationKeyFromX5C(x5cHeaderValue(leaf), roots, "Real PID Issuer"); err != nil {
		t.Fatalf("a p-384 issuer certificate was refused: %v", err)
	}
}

// A certificate that names the issuer identifier itself is not a server
// certificate for that host, whatever its extended key usages say — and issuer
// certificates cut from web PKI carry serverAuth/clientAuth as a matter of
// course.
func TestVerificationKeyFromX5C_WebPKIEKUAcceptedWhenTheLeafNamesTheIssuer(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	const iss = "did:web:issuer.example:pid"
	didURI, err := url.Parse(iss)
	if err != nil {
		t.Fatalf("parse did uri: %v", err)
	}
	leaf := mintTestCertFrom(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "issuer.example"},
		DNSNames:              []string{"issuer.example"},
		URIs:                  []*url.URL{didURI},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}, &leafKey.PublicKey, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	if _, err := verificationKeyFromX5C(x5cHeaderValue(leaf), roots, iss); err != nil {
		t.Fatalf("a web-pki issuer certificate naming the issuer was refused: %v", err)
	}
}

// mintTestCertFromAnyKey is mintTestCertFrom for a public key of any type.
func mintTestCertFromAnyKey(t *testing.T, template *x509.Certificate, pub any, signer *ecdsa.PrivateKey, signerCert *x509.Certificate) *x509.Certificate {
	t.Helper()
	template.SerialNumber = big.NewInt(time.Now().UnixNano())
	template.NotBefore = time.Now().Add(-time.Hour)
	template.NotAfter = time.Now().Add(time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, template, signerCert, pub, signer)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}
