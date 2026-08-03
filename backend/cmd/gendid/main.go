// Command gendid regenerates a DID document whose verificationMethod publishes
// the ECDSA P-256 public key of the HSM dcs-did token key, together with a
// self-signed x5c certificate binding that same key. It is
// invoked by the dev-stack provisioning after the token keys are created.
package main

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"digital-contracting-service/internal/base/hsm"
)

func main() {
	out := flag.String("out", "", "output path for the DID document JSON")
	did := flag.String("did", "", "did:web identifier, e.g. did:web:localhost%3A8991")
	endpoint := flag.String("endpoint", "", "DigitalContractingService serviceEndpoint URL")
	flag.Parse()

	if *out == "" || *did == "" {
		fmt.Fprintln(os.Stderr, "gendid: -out and -did are required")
		os.Exit(1)
	}

	host, err := didWebHost(*did)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: %v\n", err)
		os.Exit(1)
	}

	h, err := hsm.Open(hsm.ConfigFromEnv())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: open token: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = h.Close() }()

	signer, err := h.Signer(hsm.KeyLabelDID())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: load did key: %v\n", err)
		os.Exit(1)
	}
	pub, ok := signer.Public().(*ecdsa.PublicKey)
	if !ok {
		fmt.Fprintln(os.Stderr, "gendid: did key is not ECDSA")
		os.Exit(1)
	}

	certDER, err := selfSignedCert(host, pub, signer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: create certificate: %v\n", err)
		os.Exit(1)
	}

	jwk := hsm.ECPublicKeyJWK(pub)
	jwk["kid"] = "dev-key-1"
	jwk["alg"] = "ES256"
	jwk["x5c"] = []string{base64.StdEncoding.EncodeToString(certDER)}

	// The federation agreement credential (ADR-19) is signed by a SEPARATE
	// HSM key, published here as its own verificationMethod so a verifier can
	// tell the eIDAS/JAdES identity key (#dev-key-1, above) apart from the VC
	// signing key. Which is which is stated by the relationships below, not by
	// the order of this list.
	vcLabel := hsm.KeyLabelVC()
	vcSigner, err := h.Signer(vcLabel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: load vc key: %v\n", err)
		os.Exit(1)
	}
	vcPub, ok := vcSigner.Public().(*ecdsa.PublicKey)
	if !ok {
		fmt.Fprintln(os.Stderr, "gendid: vc key is not ECDSA")
		os.Exit(1)
	}
	vcCertDER, err := selfSignedCert(host, vcPub, vcSigner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: create vc certificate: %v\n", err)
		os.Exit(1)
	}
	vcJWK := hsm.ECPublicKeyJWK(vcPub)
	vcJWK["kid"] = vcLabel
	vcJWK["alg"] = "ES256"
	vcJWK["x5c"] = []string{base64.StdEncoding.EncodeToString(vcCertDER)}

	// Key-agreement key: peers wrap content-encryption keys to this public
	// key. Published without x5c — it never signs anything.
	ecdhLabel := hsm.KeyLabelECDH()
	ecdhJWK, err := h.PublicJWK(ecdhLabel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: load ecdh key: %v\n", err)
		os.Exit(1)
	}
	ecdhJWK["kid"] = ecdhLabel
	ecdhJWK["use"] = "enc"

	doc := map[string]any{
		"@context": []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/jws-2020/v1",
		},
		"id": *did,
		"verificationMethod": []map[string]any{
			{
				"id":           *did + "#dev-key-1",
				"type":         "JsonWebKey2020",
				"controller":   *did,
				"publicKeyJwk": jwk,
			},
			{
				"id":           *did + "#" + vcLabel,
				"type":         "JsonWebKey2020",
				"controller":   *did,
				"publicKeyJwk": vcJWK,
			},
			{
				"id":           *did + "#" + ecdhLabel,
				"type":         "JsonWebKey2020",
				"controller":   *did,
				"publicKeyJwk": ecdhJWK,
			},
		},
		// The identity key authenticates this instance to a peer (the DCS-to-DCS
		// challenge-response) and asserts (JAdES, JAR); the VC key only asserts;
		// the key-agreement key does neither. A consumer resolves a key by the
		// relationship its use requires, so each one has to be stated.
		"authentication":  []string{*did + "#dev-key-1"},
		"assertionMethod": []string{*did + "#dev-key-1", *did + "#" + vcLabel},
		"keyAgreement":    []string{*did + "#" + ecdhLabel},
	}
	if *endpoint != "" {
		doc["services"] = []map[string]any{
			{
				"id":              *did + "#dcs",
				"type":            "DigitalContractingService",
				"serviceEndpoint": *endpoint,
			},
		}
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendid: marshal: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gendid: write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (EC P-256 DID key for %s)\n", *out, *did)
}

// didWebHost extracts the host component (with port) from a did:web identifier.
func didWebHost(did string) (string, error) {
	const prefix = "did:web:"
	if !strings.HasPrefix(did, prefix) {
		return "", fmt.Errorf("not a did:web identifier: %q", did)
	}
	hostEncoded, _, _ := strings.Cut(strings.TrimPrefix(did, prefix), ":")
	host, err := url.QueryUnescape(hostEncoded)
	if err != nil {
		return "", fmt.Errorf("decode did:web host: %w", err)
	}
	return host, nil
}

func selfSignedCert(host string, pub *ecdsa.PublicKey, priv any) ([]byte, error) {
	dnsName := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		dnsName = h
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   host,
			Organization: []string{"DCS Dev"},
			Country:      []string{"DE"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		BasicConstraintsValid: true,
		DNSNames:              []string{dnsName},
	}
	return x509.CreateCertificate(rand.Reader, template, template, pub, priv)
}
