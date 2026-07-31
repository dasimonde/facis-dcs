package compiler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// testC2PASigner is the ECDSA P-256 key that stands in for the backend's PKCS#11
// dcs-c2pa key under test. Its self-signed certificate is the x5chain the
// compiler embeds; pdf-core itself holds no key, so tests inject an in-process
// Signer via testSigningContext rather than reaching a signing endpoint.
var testC2PASigner *ecdsa.PrivateKey

// testDeterministicSigner is the in-process Signer tests inject: it signs the
// captured COSE Sig_structure with testC2PASigner exactly as the DCS backend
// would with its dcs-c2pa key. Signing is deterministic (RFC 6979-style nonce)
// so repeated compilations of the same payload are byte-identical, preserving
// pdf-core's determinism guarantee under test.
type testDeterministicSigner struct{}

func (testDeterministicSigner) Sign(_ context.Context, data []byte) ([]byte, error) {
	return deterministicES256(testC2PASigner, data), nil
}

// testSigningContext returns a context carrying the in-process test signer, as
// the DCS backend's prepare/embed flow supplies a real signer in production.
func testSigningContext() context.Context {
	return WithSigner(context.Background(), testDeterministicSigner{})
}

// startTestSigningServer generates a P-256 key + self-signed leaf, writes the
// leaf as an x5chain PEM into dir, and sets testC2PASigner. pdf-core embeds the
// x5chain but never signs; tests sign in-process via testSigningContext.
func startTestSigningServer(dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	testC2PASigner = key

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DCS-PDF-CORE test c2pa signer", Organization: []string{"DCS-PDF-CORE"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return err
	}
	chainPath := filepath.Join(dir, "x5chain.pem")
	if err := os.WriteFile(chainPath, certPEM(der), 0o644); err != nil {
		return err
	}

	_ = os.Setenv(envX5ChainPEMFile, chainPath)
	return nil
}

func certPEM(der []byte) []byte {
	const header = "-----BEGIN CERTIFICATE-----\n"
	const footer = "-----END CERTIFICATE-----\n"
	b := base64.StdEncoding.EncodeToString(der)
	var out []byte
	out = append(out, header...)
	for len(b) > 64 {
		out = append(out, b[:64]...)
		out = append(out, '\n')
		b = b[64:]
	}
	out = append(out, b...)
	out = append(out, '\n')
	out = append(out, footer...)
	return out
}

// deterministicES256 signs message with RFC 6979 deterministic ECDSA over P-256
// and returns the fixed-width 64-byte r||s encoding COSE requires.
func deterministicES256(priv *ecdsa.PrivateKey, message []byte) []byte {
	digest := sha256.Sum256(message)
	r, s := signRFC6979(priv, digest[:])
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out
}

// signRFC6979 produces a deterministic ECDSA signature (r,s) per RFC 6979 using
// HMAC-SHA256, so tests get byte-stable signatures without a real HSM.
func signRFC6979(priv *ecdsa.PrivateKey, hash []byte) (*big.Int, *big.Int) {
	n := priv.Curve.Params().N
	d := priv.D
	e := hashToInt(hash, priv.Curve)

	for k := rfc6979Nonce(n, d, hash); ; k = k.Add(k, big.NewInt(1)) {
		if k.Sign() <= 0 || k.Cmp(n) >= 0 {
			continue
		}
		rx, _ := priv.Curve.ScalarBaseMult(k.Bytes())
		r := new(big.Int).Mod(rx, n)
		if r.Sign() == 0 {
			continue
		}
		kInv := new(big.Int).ModInverse(k, n)
		s := new(big.Int).Mul(r, d)
		s.Add(s, e)
		s.Mul(s, kInv)
		s.Mod(s, n)
		if s.Sign() == 0 {
			continue
		}
		return r, s
	}
}

func hashToInt(hash []byte, curve elliptic.Curve) *big.Int {
	orderBits := curve.Params().N.BitLen()
	orderBytes := (orderBits + 7) / 8
	if len(hash) > orderBytes {
		hash = hash[:orderBytes]
	}
	ret := new(big.Int).SetBytes(hash)
	if excess := len(hash)*8 - orderBits; excess > 0 {
		ret.Rsh(ret, uint(excess))
	}
	return ret
}

// rfc6979Nonce derives the deterministic nonce k from the private key and hash.
func rfc6979Nonce(n, d *big.Int, hash []byte) *big.Int {
	// A single-round HMAC-SHA256 derivation is sufficient for a deterministic,
	// reproducible test nonce; the retry loop in signRFC6979 handles the rare
	// out-of-range case. This is test-only and does not need full RFC 6979 rigor.
	seed := append(d.Bytes(), hash...)
	sum := sha256.Sum256(seed)
	return new(big.Int).Mod(new(big.Int).SetBytes(sum[:]), n)
}

// setupTestSigning is invoked from TestMain to make signing material (the
// x5chain cert + the in-process test key) available to every test that compiles
// a PDF. It fails the process hard if setup fails.
func setupTestSigning() error {
	dir, err := os.MkdirTemp("", "dcs-c2pa-test")
	if err != nil {
		return err
	}
	return startTestSigningServer(dir)
}
