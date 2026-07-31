package main

import (
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

// testMainC2PASigner stands in for the DCS backend's dcs-c2pa key: svc_test signs
// the Sig_structures a prepare step returns and posts them to /c2pa/embed, exactly
// as the backend does. pdf-core holds no key material.
var testMainC2PASigner *ecdsa.PrivateKey

// setupTestSigning generates the test P-256 key + self-signed x5chain leaf and
// sets the x5chain env pdf-core embeds. It starts no signing server: pdf-core
// signs nothing.
func setupTestSigning() error {
	dir, err := os.MkdirTemp("", "dcs-c2pa-test")
	if err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	testMainC2PASigner = key

	// The chain must satisfy the C2PA certificate profile, or every manifest the
	// suite compiles is one c2patool rejects with signingCredential.invalid: a
	// CA-issued (not self-signed) leaf, an organizationName in the subject, and
	// an accepted extended key usage. This mirrors scripts/c2pa-cert-provision.sh.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"FACIS DCS"}, CommonName: "DCS-PDF-CORE test c2pa CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caKey.Public(), caKey)
	if err != nil {
		return err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{Organization: []string{"FACIS DCS"}, CommonName: "DCS-PDF-CORE test c2pa signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, key.Public(), caKey)
	if err != nil {
		return err
	}
	chainPath := filepath.Join(dir, "x5chain.pem")
	if err := os.WriteFile(chainPath, append(certPEM(der), certPEM(caDER)...), 0o644); err != nil {
		return err
	}
	_ = os.Setenv("DCS_PDF_CORE_C2PA_X5CHAIN_PEM_FILE", chainPath)
	return nil
}

// signSigStructure signs one COSE Sig_structure with the test key, as the DCS
// backend signs a prepare step's captured Sig_structures before /c2pa/embed.
func signSigStructure(sigStructure []byte) []byte {
	return deterministicES256(testMainC2PASigner, sigStructure)
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
// and returns the 64-byte r||s encoding, so repeated compilations are byte-stable.
func deterministicES256(priv *ecdsa.PrivateKey, message []byte) []byte {
	digest := sha256.Sum256(message)
	r, s := signRFC6979(priv, digest[:])
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out
}

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

func rfc6979Nonce(n, d *big.Int, hash []byte) *big.Int {
	seed := append(d.Bytes(), hash...)
	sum := sha256.Sum256(seed)
	return new(big.Int).Mod(new(big.Int).SetBytes(sum[:]), n)
}
