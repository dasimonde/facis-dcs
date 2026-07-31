// Package crl checks X.509 certificate serials against a DER-encoded
// Certificate Revocation List (RFC 5280). It is the shared mechanic behind the
// signature-revocation finding surfaced by /signature/validate and the
// cmd/crlcheck ops tool that materialises a revocation into
// contract_signatures.cert_revoked_at (DCS-OR-C2PA-007).
package crl

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
)

// IsRevoked reports whether the certificate with the given serial number is
// listed in the DER-encoded X.509 CRL.
func IsRevoked(crlDER []byte, serial *big.Int) (bool, error) {
	if serial == nil {
		return false, fmt.Errorf("nil serial")
	}
	rl, err := x509.ParseRevocationList(crlDER)
	if err != nil {
		return false, fmt.Errorf("parse CRL: %w", err)
	}
	for _, entry := range rl.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(serial) == 0 {
			return true, nil
		}
	}
	return false, nil
}

// LeafSerial parses the first certificate from a PEM chain (the leaf) and
// returns its serial number.
func LeafSerial(chainPEM []byte) (*big.Int, error) {
	block, _ := pem.Decode(chainPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("no leaf CERTIFICATE block in PEM chain")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse leaf certificate: %w", err)
	}
	return cert.SerialNumber, nil
}

// ParseCRLDER accepts either PEM ("X509 CRL") or raw DER CRL bytes and returns
// the DER encoding.
func ParseCRLDER(raw []byte) ([]byte, error) {
	if block, _ := pem.Decode(raw); block != nil && block.Type == "X509 CRL" {
		return block.Bytes, nil
	}
	// Assume the input is already DER; ParseRevocationList validates it.
	if _, err := x509.ParseRevocationList(raw); err != nil {
		return nil, fmt.Errorf("input is neither a PEM X509 CRL nor a valid DER CRL: %w", err)
	}
	return raw, nil
}
