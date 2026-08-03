package sdjwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// mintTestCert issues a certificate for pub, signed by signer/signerCert, for
// chain-validation tests.
func mintTestCert(t *testing.T, cn string, pub *ecdsa.PublicKey, isCA bool, signer *ecdsa.PrivateKey, signerCert *x509.Certificate) *x509.Certificate {
	t.Helper()

	return mintTestCertFrom(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: cn},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}, pub, signer, signerCert)
}

// mintTestCertFrom issues a certificate from a partially filled template, so a
// test can decide the names and usages that resolution actually reads.
func mintTestCertFrom(t *testing.T, template *x509.Certificate, pub *ecdsa.PublicKey, signer *ecdsa.PrivateKey, signerCert *x509.Certificate) *x509.Certificate {
	t.Helper()

	template.SerialNumber = big.NewInt(time.Now().UnixNano())
	template.NotBefore = time.Now().Add(-time.Hour)
	template.NotAfter = time.Now().Add(time.Hour)

	cn := template.Subject.CommonName
	der, err := x509.CreateCertificate(rand.Reader, template, signerCert, pub, signer)
	if err != nil {
		t.Fatalf("create certificate %q: %v", cn, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate %q: %v", cn, err)
	}
	return cert
}

func mintSelfSignedCA(t *testing.T, cn string) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	return key, cert
}

func x5cHeaderValue(certs ...*x509.Certificate) any {
	out := make([]any, 0, len(certs))
	for _, c := range certs {
		out = append(out, base64.StdEncoding.EncodeToString(c.Raw))
	}
	return out
}

func TestVerificationKeyFromX5C_TrustedChainReturnsLeafKey(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Test Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafCert := mintTestCert(t, "Test Issuer", &leafKey.PublicKey, false, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	key, err := verificationKeyFromX5C(x5cHeaderValue(leafCert), roots, "Test Issuer")
	if err != nil {
		t.Fatalf("expected the chain to verify, got: %v", err)
	}
	pub, ok := key.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("expected an *ecdsa.PublicKey, got %T", key)
	}
	if pub.X.Cmp(leafKey.X) != 0 || pub.Y.Cmp(leafKey.Y) != 0 {
		t.Fatalf("returned key does not match the leaf certificate's public key")
	}
}

func TestVerificationKeyFromX5C_UntrustedChainIsRefused(t *testing.T) {
	// The leaf is issued by a REAL CA, but the trust pool only knows about a
	// DIFFERENT, unrelated CA — this must fail, not fall back to trusting the
	// leaf on its own say-so.
	caKey, caCert := mintSelfSignedCA(t, "Issuing CA (not trusted)")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafCert := mintTestCert(t, "Test Issuer", &leafKey.PublicKey, false, caKey, caCert)

	_, unrelatedCert := mintSelfSignedCA(t, "Unrelated Trusted Root")
	roots := x509.NewCertPool()
	roots.AddCert(unrelatedCert)

	if _, err := verificationKeyFromX5C(x5cHeaderValue(leafCert), roots, "Test Issuer"); err == nil {
		t.Fatal("expected an untrusted certificate chain to be refused")
	}
}

func TestVerificationKeyFromX5C_NoTrustAnchorsConfiguredIsRefused(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Some CA")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafCert := mintTestCert(t, "Test Issuer", &leafKey.PublicKey, false, caKey, caCert)

	// roots == nil: an x5c-bearing credential with nothing configured to
	// verify it against must be refused, never silently trusted.
	if _, err := verificationKeyFromX5C(x5cHeaderValue(leafCert), nil, "Test Issuer"); err == nil {
		t.Fatal("expected a nil trust pool to refuse the credential")
	}
}

func TestVerificationKeyFromX5C_IntermediateChainVerifiesAgainstRoot(t *testing.T) {
	rootKey, rootCert := mintSelfSignedCA(t, "Root CA")
	intermediateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate intermediate key: %v", err)
	}
	intermediateCert := mintTestCert(t, "Intermediate CA", &intermediateKey.PublicKey, true, rootKey, rootCert)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafCert := mintTestCert(t, "Test Issuer", &leafKey.PublicKey, false, intermediateKey, intermediateCert)

	roots := x509.NewCertPool()
	roots.AddCert(rootCert)

	// x5c is leaf-first (RFC 7517 §4.7): [leaf, intermediate].
	if _, err := verificationKeyFromX5C(x5cHeaderValue(leafCert, intermediateCert), roots, "Test Issuer"); err != nil {
		t.Fatalf("expected leaf -> intermediate -> root to verify, got: %v", err)
	}
}

func TestVerificationKeyFromX5C_EmptyHeaderIsRejected(t *testing.T) {
	roots := x509.NewCertPool()
	if _, err := verificationKeyFromX5C([]any{}, roots, "Test Issuer"); err == nil {
		t.Fatal("expected an empty x5c header to be rejected")
	}
}

// A chain proves an anchor vouched for the certificate; it says nothing about
// WHOSE certificate it is. Without binding the leaf to the claimed issuer, any
// certificate under any configured anchor — a TLS server certificate included —
// signs credentials asserting any issuer identity.
func TestVerificationKeyFromX5C_LeafMustIdentifyTheClaimedIssuer(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	// A perfectly valid certificate under the anchor, belonging to someone else.
	leafCert := mintTestCert(t, "Some Other Service", &leafKey.PublicKey, false, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	if _, err := verificationKeyFromX5C(x5cHeaderValue(leafCert), roots, "https://victim.example/issuer"); err == nil {
		t.Fatal("a chain that verifies but names a different subject must be refused")
	}

	// The same certificate is fine for the identity it actually carries.
	if _, err := verificationKeyFromX5C(x5cHeaderValue(leafCert), roots, "Some Other Service"); err != nil {
		t.Fatalf("the leaf must be accepted for its own identity: %v", err)
	}
}

// A did:web or https issuer is often deployed as a host that also holds a TLS
// certificate under the same anchor, and that certificate's DNS SAN matches the
// issuer's authority — so the DNS branch is reachable and needs its own cover.
func TestLeafIdentifiesIssuer_SANDNSMatchesIssuerAuthority(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leaf := mintTestCertFrom(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "PID Issuer"},
		DNSNames:              []string{"issuer.example"},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}, &leafKey.PublicKey, caKey, caCert)

	for _, iss := range []string{"did:web:issuer.example:pid", "https://issuer.example/pid"} {
		if _, err := leafIdentifiesIssuer(leaf, iss); err != nil {
			t.Errorf("dns san %v should identify %q: %v", leaf.DNSNames, iss, err)
		}
	}

	// The DNS name must match the issuer's OWN authority, not merely appear.
	if _, err := leafIdentifiesIssuer(leaf, "did:web:other.example:pid"); err == nil {
		t.Error("a dns san for a different authority must not identify the issuer")
	}
}

// A did:web authority carrying a port arrives percent-encoded and decodes to
// host:port, which no DNS SAN can equal — the dev and demo PID issuers
// therefore name themselves with a URI SAN holding the DID itself.
func TestLeafIdentifiesIssuer_SANURICarriesTheDID(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	const iss = "did:web:dev.example:issuer:pid-x5c"
	didURI, err := url.Parse(iss)
	if err != nil {
		t.Fatalf("parse did uri: %v", err)
	}
	leaf := mintTestCertFrom(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "DCS Dev PID Issuer (x5c, DEV ONLY)"},
		URIs:                  []*url.URL{didURI},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}, &leafKey.PublicKey, caKey, caCert)

	if _, err := leafIdentifiesIssuer(leaf, iss); err != nil {
		t.Fatalf("a uri san holding the did must identify the issuer: %v", err)
	}
	if _, err := leafIdentifiesIssuer(leaf, "did:web:dev.example:issuer:other"); err == nil {
		t.Error("a uri san holding a different did must not identify the issuer")
	}
}

// The chain is verified with ExtKeyUsageAny, so the certificate's own usage
// extensions are the only thing standing between an anchor that also issues TLS
// certificates and a server certificate signing credentials for its host.
func TestVerificationKeyFromX5C_TLSCertificateIsRefused(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	// An ordinary TLS server certificate for the issuer's own host: it chains to
	// the anchor and its DNS SAN names the issuer's authority.
	leaf := mintTestCertFrom(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "issuer.example"},
		DNSNames:              []string{"issuer.example"},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}, &leafKey.PublicKey, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	if _, err := verificationKeyFromX5C(x5cHeaderValue(leaf), roots, "did:web:issuer.example:pid"); err == nil {
		t.Fatal("a TLS server certificate must not be accepted as a credential signer")
	}
}

func TestVerificationKeyFromX5C_LeafForbiddenToSignIsRefused(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	leaf := mintTestCertFrom(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "Test Issuer"},
		KeyUsage:              x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}, &leafKey.PublicKey, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	if _, err := verificationKeyFromX5C(x5cHeaderValue(leaf), roots, "Test Issuer"); err == nil {
		t.Fatal("a certificate whose key usage excludes digital signatures must be refused")
	}
}

// --- ResolveIssuerVerificationKey: the CONFIGURED mechanism picks the branch ---

type stubTrust struct {
	trusted map[string]bool
	usesX5C map[string]bool
	jwks    json.RawMessage
	roots   *x509.CertPool
}

func (s stubTrust) IssuerTrusted(iss string) bool { return s.trusted[iss] }
func (s stubTrust) VCTAllowed(string) bool        { return true }

func (s stubTrust) IssuerJWKS(iss string) (json.RawMessage, error) {
	if !s.trusted[iss] {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}
	// An x5c issuer publishes no bare key, exactly as trust_mechanism.go resolves it.
	if s.usesX5C[iss] {
		return nil, nil
	}
	return s.jwks, nil
}

func (s stubTrust) IssuerUsesX5C(iss string) (bool, error) {
	if !s.trusted[iss] {
		return false, fmt.Errorf("issuer %q is not trusted", iss)
	}
	return s.usesX5C[iss], nil
}

func (s stubTrust) X5CTrustRoots() *x509.CertPool { return s.roots }

func publicJWKSFor(t *testing.T, key *ecdsa.PublicKey) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"keys": []map[string]string{{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}}})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return raw
}

func tokenFor(iss string, header map[string]any) *jwt.Token {
	header["alg"] = "ES256"
	return &jwt.Token{Header: header, Claims: jwt.MapClaims{"iss": iss}}
}

// Letting the credential choose the branch would mean any certificate under any
// configured anchor could speak for an issuer whose key is published as a JWKS.
func TestResolveIssuerVerificationKey_X5CHeaderRefusedForJWKSIssuer(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	const iss = "did:web:jwks.example:issuer"
	leaf := mintTestCert(t, iss, &issuerKey.PublicKey, false, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	cfg := stubTrust{
		trusted: map[string]bool{iss: true},
		usesX5C: map[string]bool{},
		jwks:    publicJWKSFor(t, &issuerKey.PublicKey),
		roots:   roots,
	}

	// The chain verifies and the leaf even names the issuer — it is still the
	// wrong way for THIS issuer to publish a key.
	if _, err := ResolveIssuerVerificationKey(cfg, tokenFor(iss, map[string]any{"x5c": x5cHeaderValue(leaf)})); err == nil {
		t.Fatal("a jwks issuer must not be resolved through a certificate chain")
	}

	// The configured path still works.
	if _, err := ResolveIssuerVerificationKey(cfg, tokenFor(iss, map[string]any{})); err != nil {
		t.Fatalf("the configured jwks path must resolve: %v", err)
	}
}

func TestResolveIssuerVerificationKey_BareJWKRefusedForX5CIssuer(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared Trust Root")
	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	const iss = "did:web:x5c.example:issuer"
	leaf := mintTestCert(t, iss, &issuerKey.PublicKey, false, caKey, caCert)

	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	cfg := stubTrust{
		trusted: map[string]bool{iss: true},
		usesX5C: map[string]bool{iss: true},
		roots:   roots,
	}

	header := map[string]any{"jwk": map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(issuerKey.X.FillBytes(make([]byte, 32))),
		"y":   base64.RawURLEncoding.EncodeToString(issuerKey.Y.FillBytes(make([]byte, 32))),
	}}
	// The key is even the right one — but this issuer publishes certificates, so
	// a bare key arrives with nothing vouching for it.
	if _, err := ResolveIssuerVerificationKey(cfg, tokenFor(iss, header)); err == nil {
		t.Fatal("an x5c issuer must not be resolved from a bare jwk header")
	}

	key, err := ResolveIssuerVerificationKey(cfg, tokenFor(iss, map[string]any{"x5c": x5cHeaderValue(leaf)}))
	if err != nil {
		t.Fatalf("the configured x5c path must resolve: %v", err)
	}
	pub, ok := key.(*ecdsa.PublicKey)
	if !ok || pub.X.Cmp(issuerKey.X) != 0 {
		t.Fatal("x5c resolution must return the leaf certificate's key")
	}
}

func TestResolveIssuerVerificationKey_UntrustedIssuerIsRefused(t *testing.T) {
	cfg := stubTrust{trusted: map[string]bool{}, usesX5C: map[string]bool{}}
	if _, err := ResolveIssuerVerificationKey(cfg, tokenFor("did:web:nobody.example:issuer", map[string]any{})); err == nil {
		t.Fatal("an issuer absent from the trust configuration must be refused")
	}
}

func TestIssuerAuthority(t *testing.T) {
	cases := map[string]string{
		"did:web:example.com:issuer":             "example.com",
		"did:web:dcs-b.localhost%3A18080:issuer": "dcs-b.localhost:18080",
		"https://example.com/issuer":             "example.com",
		"urn:something:else":                     "",
	}
	for iss, want := range cases {
		if got := issuerAuthority(iss); got != want {
			t.Errorf("%s → %q, want %q", iss, got, want)
		}
	}
}

// The ORCE issuer flow mints ONE leaf and signs two things with it: the Power
// of Attorney credential, whose `iss` is the issuer's did:web identifier, and
// the status list that credential points at, whose `iss` is the issuer's base
// URL. Both must be identifiable from the same certificate, or ADR-34's
// arrangement — same issuer, same key, identified the same way — falls apart at
// one of the two.
//
// The dev stack makes it awkward on purpose: the issuer is reached on a
// NodePort, so its authority carries a port, which arrives percent-encoded in
// the DID and decodes to host:port. No DNS SAN can equal that, and the http
// base URL is not the DID either, so both identifiers are carried explicitly
// (deployment/helm/charts/orce/flows-issuer/flow-pki.json ensureIssuerCertFor
// writes URI:<did>, URI:<baseUrl>, DNS:<hostname>).
func TestLeafIdentifiesIssuer_ORCEIssuerLeafNamesBothItsIdentifiers(t *testing.T) {
	const (
		baseURL  = "http://localhost:30181"
		issuerID = "did:web:localhost%3A30181"
		hostname = "localhost"
	)
	caKey, caCert := mintSelfSignedCA(t, "FACIS Demo Root CA")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	didURI, err := url.Parse(issuerID)
	if err != nil {
		t.Fatalf("parse did uri: %v", err)
	}
	baseURI, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	leaf := mintTestCertFrom(t, &x509.Certificate{
		Subject:               pkix.Name{CommonName: "FACIS Demo Issuer"},
		URIs:                  []*url.URL{didURI, baseURI},
		DNSNames:              []string{hostname},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}, &leafKey.PublicKey, caKey, caCert)

	// The percent-encoded port has to survive parsing and re-serialisation, or
	// the SAN the issuer wrote is not the string compared against `iss`.
	for _, iss := range []string{issuerID, baseURL} {
		binding, err := leafIdentifiesIssuer(leaf, iss)
		if err != nil {
			t.Fatalf("leaf must identify %q: %v", iss, err)
		}
		if binding != bindingURI {
			t.Errorf("issuer %q was identified by %v, not by the URI SAN the flow writes", iss, binding)
		}
	}

	// The same issuer published on a different port is a different issuer, and
	// the shared DNS name must not make one speak for the other.
	if _, err := leafIdentifiesIssuer(leaf, "did:web:localhost%3A18080:issuer"); err == nil {
		t.Error("a leaf minted for one base URL must not identify the issuer at another")
	}
}
