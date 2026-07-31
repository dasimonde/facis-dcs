package crl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func newCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen ca key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CRL CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca cert: %v", err)
	}
	return caCert, key
}

func buildCRL(t *testing.T, ca *x509.Certificate, key *ecdsa.PrivateKey, revoked ...*big.Int) []byte {
	t.Helper()
	entries := make([]x509.RevocationListEntry, 0, len(revoked))
	for _, s := range revoked {
		entries = append(entries, x509.RevocationListEntry{SerialNumber: s, RevocationTime: time.Now()})
	}
	tmpl := &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                time.Now(),
		NextUpdate:                time.Now().Add(time.Hour),
		RevokedCertificateEntries: entries,
	}
	der, err := x509.CreateRevocationList(rand.Reader, tmpl, ca, key)
	if err != nil {
		t.Fatalf("create CRL: %v", err)
	}
	return der
}

func TestIsRevoked(t *testing.T) {
	ca, key := newCA(t)
	revokedSerial := big.NewInt(42)
	crlDER := buildCRL(t, ca, key, revokedSerial)

	got, err := IsRevoked(crlDER, big.NewInt(42))
	if err != nil {
		t.Fatalf("IsRevoked revoked: %v", err)
	}
	if !got {
		t.Fatal("expected serial 42 to be reported revoked")
	}

	got, err = IsRevoked(crlDER, big.NewInt(7))
	if err != nil {
		t.Fatalf("IsRevoked non-revoked: %v", err)
	}
	if got {
		t.Fatal("expected serial 7 to be reported not revoked")
	}
}

func TestParseCRLDERAcceptsPEM(t *testing.T) {
	ca, key := newCA(t)
	crlDER := buildCRL(t, ca, key, big.NewInt(9))
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlDER})

	der, err := ParseCRLDER(pemBytes)
	if err != nil {
		t.Fatalf("ParseCRLDER pem: %v", err)
	}
	got, err := IsRevoked(der, big.NewInt(9))
	if err != nil || !got {
		t.Fatalf("expected serial 9 revoked, got=%v err=%v", got, err)
	}
}
