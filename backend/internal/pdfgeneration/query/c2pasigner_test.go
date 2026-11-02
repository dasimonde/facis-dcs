package query

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// testCA issues C2PA signing leaves the way a deployment's provisioning does.
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newTestCA(t *testing.T) testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "C2PA test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testCA{cert: cert, key: key}
}

// signer returns the manifest signer of a leaf this CA issued to issuerDID.
func (ca testCA) signer(t *testing.T, manifest, issuerDID string) pdfcore.ManifestSigner {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uri, err := url.Parse(issuerDID)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "c2pa signer", Organization: []string{"FACIS"}},
		URIs:         []*url.URL{uri},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return pdfcore.ManifestSigner{Manifest: manifest, Subject: "CN=c2pa signer", Chain: []string{base64.StdEncoding.EncodeToString(der)}}
}

const (
	partyA = "did:web:dcs-a.example"
	partyB = "did:web:dcs-b.example"
)

// DCS-OR-C2PA-007: the signer of every manifest is anchored. Its chain must
// verify against the deployment's anchors and its leaf must identify an
// instance party to the document; the chain a manifest carries does not
// vouch for itself.
func TestEveryManifestSignerMustBeAnchoredToAParty(t *testing.T) {
	ca := newTestCA(t)
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	anchors := SignerAnchors{Roots: roots, Issuers: []string{partyA, partyB}}

	status, detail := anchorSigners([]pdfcore.ManifestSigner{
		ca.signer(t, "urn:c2pa:1", partyA),
		ca.signer(t, "urn:c2pa:2", partyB),
	}, anchors)
	if status != provenance.CheckValid || !strings.Contains(detail, "urn:c2pa:2 signed by "+partyB) {
		t.Fatalf("both parties' manifests: %s (%s), want valid", status, detail)
	}

	for name, foreign := range map[string]pdfcore.ManifestSigner{
		"a leaf naming no party":      ca.signer(t, "urn:c2pa:3", "did:web:dcs-x.example"),
		"a leaf under another anchor": newTestCA(t).signer(t, "urn:c2pa:3", partyA),
	} {
		t.Run(name, func(t *testing.T) {
			status, detail := anchorSigners([]pdfcore.ManifestSigner{ca.signer(t, "urn:c2pa:1", partyA), foreign}, anchors)
			if status != provenance.CheckInvalid || !strings.Contains(detail, "urn:c2pa:3") {
				t.Fatalf("got %s (%s), want invalid naming the manifest", status, detail)
			}
		})
	}

	if status, _ := anchorSigners([]pdfcore.ManifestSigner{ca.signer(t, "urn:c2pa:1", partyA)}, SignerAnchors{Issuers: []string{partyA}}); status != provenance.CheckIndeterminate {
		t.Fatalf("without trust anchors: %s, want indeterminate", status)
	}
}

// The verifier reports the anchoring of the signers pdf-core names.
func TestRunVerifyReportsTheSignerAnchoring(t *testing.T) {
	ca := newTestCA(t)
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": true,
		"signers":              []pdfcore.ManifestSigner{ca.signer(t, "urn:c2pa:1", "did:web:dcs-x.example")},
	})

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, nil, nil, nil,
		SignerAnchors{Roots: roots, Issuers: []string{partyA}}, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.C2paSignerStatus != provenance.CheckInvalid || result.C2paSignerDetail == nil {
		t.Fatalf("c2pa_signer_status = %q (%v), want invalid with the reason", result.C2paSignerStatus, result.C2paSignerDetail)
	}
}
