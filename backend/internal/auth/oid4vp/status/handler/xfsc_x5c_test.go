package handler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/url"
	"testing"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/golang-jwt/jwt/v5"
)

// A PID issuer that publishes its key by certificate chain bundles no JWKS, so
// the status list it signs can only be verified from the chain the token itself
// carries. Nothing exercised that: the BDD credentials carrying x5c point their
// status list at a third-party URL, so no test ever fetched a status list signed
// by an x5c issuer, and the demo deployment was the first place the two met.
const x5cStatusIssuer = "https://issuer.example/pid-issuer"

func mintCA(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Status Test Root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	return key, cert
}

// mintLeaf issues a signing leaf naming issuerURI in a URI SAN, the way the
// demo PID issuer names itself.
func mintLeaf(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, issuerURI string) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	san, err := url.Parse(issuerURI)
	if err != nil {
		t.Fatalf("parse issuer uri: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Status Test PID Issuer"},
		URIs:                  []*url.URL{san},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	return key, cert
}

func signStatusListJWT(t *testing.T, key *ecdsa.PrivateKey, chain []*x509.Certificate, issuer string) []byte {
	t.Helper()
	x5c := make([]string, 0, len(chain))
	for _, cert := range chain {
		x5c = append(x5c, base64.StdEncoding.EncodeToString(cert.Raw))
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": issuer,
		"sub": issuer + "/status-list/1",
		"iat": time.Now().Add(-time.Minute).Unix(),
		"status_list": map[string]any{
			"bits": 1,
			"lst":  "eNrbuQAAAfsB9Q", // one cleared bit, deflate-compressed
		},
	})
	token.Header["typ"] = "statuslist+jwt"
	token.Header["x5c"] = x5c
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign status list: %v", err)
	}
	return []byte(signed)
}

func trustWithAnchor(t *testing.T, anchor *x509.Certificate) *status.TrustConfig {
	t.Helper()
	pool := x509.NewCertPool()
	if anchor != nil {
		pool.AddCert(anchor)
	}
	// Deliberately no bundled JWKS: an x5c issuer publishes no key here, which
	// is precisely the configuration that used to make its status list
	// unverifiable.
	return &status.TrustConfig{
		Issuers:  map[string]status.TrustIssuerEntry{},
		X5CRoots: pool,
	}
}

func TestXFSCVerifiesAStatusListSignedByAnX5CIssuer(t *testing.T) {
	caKey, caCert := mintCA(t)
	leafKey, leafCert := mintLeaf(t, caKey, caCert, x5cStatusIssuer)
	body := signStatusListJWT(t, leafKey, []*x509.Certificate{leafCert, caCert}, x5cStatusIssuer)

	h := &XFSC{Trust: trustWithAnchor(t, caCert)}
	verified, err := h.verifyStatusListJWT(body)
	if err != nil {
		t.Fatalf("an x5c issuer's status list must verify from its own chain: %v", err)
	}
	claims, err := json.Marshal(verified.Claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	if len(claims) == 0 {
		t.Fatal("verified status list carried no claims")
	}
}

// The chain must prove something. Without an anchor it proves only that
// whoever signed it also minted the certificate.
func TestXFSCRefusesAnX5CStatusListWithoutAnchors(t *testing.T) {
	caKey, caCert := mintCA(t)
	leafKey, leafCert := mintLeaf(t, caKey, caCert, x5cStatusIssuer)
	body := signStatusListJWT(t, leafKey, []*x509.Certificate{leafCert, caCert}, x5cStatusIssuer)

	h := &XFSC{Trust: &status.TrustConfig{Issuers: map[string]status.TrustIssuerEntry{}}}
	if _, err := h.verifyStatusListJWT(body); err == nil {
		t.Fatal("a chain that verifies against no anchor must be refused")
	}
}

// A certificate under a trusted anchor says nothing about WHICH issuer it
// belongs to. Without this, any leaf the anchor ever signed could publish the
// revocation status of every other issuer.
func TestXFSCRefusesAnX5CStatusListWhoseLeafNamesAnotherIssuer(t *testing.T) {
	caKey, caCert := mintCA(t)
	leafKey, leafCert := mintLeaf(t, caKey, caCert, "https://someone-else.example/pid-issuer")
	body := signStatusListJWT(t, leafKey, []*x509.Certificate{leafCert, caCert}, x5cStatusIssuer)

	h := &XFSC{Trust: trustWithAnchor(t, caCert)}
	if _, err := h.verifyStatusListJWT(body); err == nil {
		t.Fatal("a leaf naming a different issuer must not verify that issuer's status list")
	}
}
