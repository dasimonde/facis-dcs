package oid4vp

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const statusListJWTTyp = "statuslist+jwt"

// ietfStatusList serves an IETF Token Status List exactly as the issuer flow
// deployed beside this service serves its own (ADR-34,
// deployment/helm/charts/orce/flows-issuer/flow-statuslist.json): ES256 over typ
// statuslist+jwt, iss the issuer's base URL, sub the list's own URL, the bits
// zlib-deflated under status_list.lst, and an x5c chain — leaf then root — whose
// leaf carries the issuer identifier as a URI SAN.
//
// The chain is what makes this fixture the deployed shape. A statuslist+jwt
// media type routes to handler.IETFToken, and its x5c branch is the path a
// login credential's status list actually takes; a fixture that published a
// bundled JWKS instead would leave that path uncovered, which is how an
// x5c-signed status list first reached production unverified. keyByJWKS covers
// the other branch, for an issuer that publishes no certificate.
type ietfStatusList struct {
	// URL is what a credential's status.status_list.uri points at.
	URL string
	// Issuer is the iss the token carries, and the identifier the leaf must
	// name for its chain to be usable.
	Issuer string
	// Roots are the anchors an instance verifying this list must be configured
	// with; without them the chain in the token verifies against nothing.
	Roots *x509.CertPool

	key    *ecdsa.PrivateKey
	chain  []*x509.Certificate
	jwks   json.RawMessage
	caKey  *ecdsa.PrivateKey
	caCert *x509.Certificate

	mu   sync.Mutex
	bits []byte
}

// keyByJWKS publishes the list's signing key as a bundled JWKS and stops
// sending the chain, for an issuer identified by key rather than certificate.
func keyByJWKS(l *ietfStatusList) {
	l.chain = nil
}

func newIETFStatusList(t *testing.T, size int, opts ...func(*ietfStatusList)) *ietfStatusList {
	t.Helper()

	list := &ietfStatusList{key: newECKey(t), bits: make([]byte, size)}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is the only fetch: its media type routes the reference to the
		// IETF handler, which then reads the body the router already holds.
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))

		token, err := list.token()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/statuslist+jwt")
		_, _ = w.Write([]byte(token))
	}))
	list.Issuer = "http://" + srv.Listener.Addr().String()
	list.URL = list.Issuer + "/status-list/1"
	srv.Start()
	t.Cleanup(srv.Close)

	issuerURI, err := url.Parse(list.Issuer)
	require.NoError(t, err)

	list.caKey, list.caCert = mintStatusListCA(t)
	leaf := mintStatusListLeaf(t, issuerURI, &list.key.PublicKey, list.caKey, list.caCert)
	list.chain = []*x509.Certificate{leaf, list.caCert}
	list.Roots = x509.NewCertPool()
	list.Roots.AddCert(list.caCert)
	list.jwks = statusListJWKS(t, list.key)

	for _, opt := range opts {
		opt(list)
	}
	return list
}

// leafNamingNoIssuer re-mints the leaf without the URI SAN, which is what the
// issuer's boot path used to produce before the public URL was known: a
// certificate that chains to the anchor and identifies nobody. It is the shape
// that made a correctly-signed production status list unusable.
func leafNamingNoIssuer(t *testing.T) func(*ietfStatusList) {
	t.Helper()
	return func(l *ietfStatusList) {
		template := &x509.Certificate{
			SerialNumber:          big.NewInt(time.Now().UnixNano()),
			Subject:               pkix.Name{CommonName: "FACIS Demo Issuer"},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			KeyUsage:              x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
		}
		l.chain = []*x509.Certificate{
			createCertificate(t, template, l.caCert, &l.key.PublicKey, l.caKey),
			l.caCert,
		}
	}
}

// Entry is the status claim a credential carries to point at this list.
func (l *ietfStatusList) Entry(index uint64) map[string]any {
	return map[string]any{"status_list": map[string]any{"uri": l.URL, "idx": index}}
}

// Revoke sets an index's bit, standing in for the /admin endpoint the deployed
// issuer flow exposes. A list that can be changed after a credential has
// verified against it is what makes a revocation test prove revocation rather
// than a list that failed to load.
func (l *ietfStatusList) Revoke(index uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bits[index/8] |= 1 << (index % 8)
}

func (l *ietfStatusList) token() (string, error) {
	l.mu.Lock()
	compressed, err := deflateZLIB(l.bits)
	l.mu.Unlock()
	if err != nil {
		return "", err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": l.Issuer,
		"sub": l.URL,
		"iat": time.Now().Add(-time.Minute).Unix(),
		"status_list": map[string]any{
			"bits": 1,
			"lst":  base64.RawURLEncoding.EncodeToString(compressed),
		},
	})
	token.Header["typ"] = statusListJWTTyp
	if len(l.chain) > 0 {
		token.Header["x5c"] = x5cChain(l.chain...)
	}
	return token.SignedString(l.key)
}

// trustIETFStatusList wires this instance's status-list verifier the way a
// deployment wires it: off the trust config it loaded, whose x5c anchors are the
// ConfigMap of issuer roots and whose bundled JWKS entries are the trust
// document's own. A list signing by chain gets the anchors and no issuer entry —
// an x5c issuer bundles no key — and one signing by bundled key gets the entry
// and no anchors, so neither test can pass on the other's credentials.
func trustIETFStatusList(t *testing.T, list *ietfStatusList) {
	t.Helper()

	cfg := &TrustConfig{}
	if len(list.chain) > 0 {
		cfg.Issuers = map[string]TrustedIssuer{list.Issuer: {Mechanism: MechanismX5C}}
		cfg.SetX5CTrustRoots(list.Roots)
	} else {
		cfg.Issuers = map[string]TrustedIssuer{
			list.Issuer: {Mechanism: MechanismJWKS, JWKS: list.jwks},
		}
	}
	require.NoError(t, ConfigureStatusListVerification(cfg))
	t.Cleanup(func() { _ = ConfigureStatusListVerification(nil) })
}

// xfscStatusList serves a status list in the XFSC shape a third-party issuer
// may still serve: the unsigned {tenantId, listId, list} envelope on the plain
// probe, which is what routes the reference to the XFSC handler, and — only for
// the request that asks for statuslist+jwt — a token signed under the issuer's
// own certificate chain, leaf then root, with the leaf's URI SAN naming the
// issuer the token claims to be.
//
// The probe body decides nothing. XFSC refuses an unsigned list and fetches the
// signed one regardless of what it was handed.
type xfscStatusList struct {
	// URL is what a credential's status.status_list.uri points at.
	URL string
	// Roots are the anchors an instance verifying this list must be configured
	// with; without them the chain in the token verifies against nothing.
	Roots *x509.CertPool

	issuer string
	key    *ecdsa.PrivateKey
	chain  []*x509.Certificate

	mu   sync.Mutex
	bits []byte
}

func newXFSCStatusList(t *testing.T, size int) *xfscStatusList {
	t.Helper()

	list := &xfscStatusList{key: newECKey(t), bits: make([]byte, size)}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(list.unsignedEnvelope())
			return
		}
		token, err := list.token()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/statuslist+jwt")
		_, _ = w.Write([]byte(token))
	}))
	list.issuer = "http://" + srv.Listener.Addr().String()
	list.URL = list.issuer + "/status-list/1"
	srv.Start()
	t.Cleanup(srv.Close)

	issuerURI, err := url.Parse(list.issuer)
	require.NoError(t, err)

	caKey, caCert := mintStatusListCA(t)
	leaf := mintStatusListLeaf(t, issuerURI, &list.key.PublicKey, caKey, caCert)
	list.chain = []*x509.Certificate{leaf, caCert}
	list.Roots = x509.NewCertPool()
	list.Roots.AddCert(caCert)

	return list
}

func (l *xfscStatusList) Entry(index uint64) map[string]any {
	return map[string]any{"status_list": map[string]any{"uri": l.URL, "idx": index}}
}

func (l *xfscStatusList) Revoke(index uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bits[index/8] |= 1 << (index % 8)
}

func (l *xfscStatusList) token() (string, error) {
	l.mu.Lock()
	compressed, err := compressGZIP(l.bits)
	l.mu.Unlock()
	if err != nil {
		return "", err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": l.issuer,
		"sub": l.URL,
		"iat": time.Now().Add(-time.Minute).Unix(),
		"status_list": map[string]any{
			"bits": 1,
			"lst":  base64.RawURLEncoding.EncodeToString(compressed),
		},
	})
	token.Header["typ"] = statusListJWTTyp
	token.Header["x5c"] = x5cChain(l.chain...)
	return token.SignedString(l.key)
}

func (l *xfscStatusList) unsignedEnvelope() []byte {
	l.mu.Lock()
	compressed, err := compressGZIP(l.bits)
	l.mu.Unlock()
	if err != nil {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"tenantId": "default",
		"listId":   1,
		"list":     base64.RawStdEncoding.EncodeToString(compressed),
	})
	return body
}

func mintStatusListCA(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()

	key := newECKey(t)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "Status List Test Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	return key, createCertificate(t, template, template, &key.PublicKey, key)
}

// mintStatusListLeaf issues the issuer's signing certificate. The URI SAN holds
// the issuer identifier itself, so the leaf identifies the issuer the token
// names rather than merely chaining to an anchor — without which any
// certificate under any configured anchor could publish any issuer's revocation
// status.
func mintStatusListLeaf(
	t *testing.T,
	issuer *url.URL,
	pub *ecdsa.PublicKey,
	caKey *ecdsa.PrivateKey,
	caCert *x509.Certificate,
) *x509.Certificate {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "Status List Test Issuer"},
		URIs:                  []*url.URL{issuer},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	return createCertificate(t, template, caCert, pub, caKey)
}

func createCertificate(
	t *testing.T,
	template, parent *x509.Certificate,
	pub *ecdsa.PublicKey,
	signer *ecdsa.PrivateKey,
) *x509.Certificate {
	t.Helper()

	der, err := x509.CreateCertificate(rand.Reader, template, parent, pub, signer)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func x5cChain(certs ...*x509.Certificate) []string {
	out := make([]string, 0, len(certs))
	for _, cert := range certs {
		out = append(out, base64.StdEncoding.EncodeToString(cert.Raw))
	}
	return out
}

func statusListJWKS(t *testing.T, key *ecdsa.PrivateKey) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(map[string]any{"keys": []any{jwkMap(publicJWK(key))}})
	require.NoError(t, err)
	return raw
}

func deflateZLIB(bits []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(bits); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func compressGZIP(bits []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(bits); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
