package sdjwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

// The point of pinning a login issuer's leaf: a certificate authority cannot
// introduce a login issuer for this deployment.
//
// Both leaves below are issued by the SAME CA and both name the same issuer
// identifier, so a chain check against that CA accepts either. Only the one
// carrying the key the operator wrote down is accepted here, which is what
// "my organization issued this credential" has to mean (ADR-35).
func TestPinnedLeaf_CACannotIntroduceALoginIssuer(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared PoA Root")
	const iss = "https://my.example/issuer"

	mine, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	// A second key the same CA is equally willing to certify for the same name.
	theirs, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate impostor key: %v", err)
	}

	mineCert := mintTestCert(t, iss, &mine.PublicKey, false, caKey, caCert)
	theirsCert := mintTestCert(t, iss, &theirs.PublicKey, false, caKey, caCert)

	pinned := [][]byte{mineCert.RawSubjectPublicKeyInfo}

	key, err := verificationKeyFromPinnedLeaf(x5cHeaderValue(mineCert), pinned, iss)
	if err != nil {
		t.Fatalf("the pinned issuer's own leaf must be accepted: %v", err)
	}
	if pub, ok := key.(*ecdsa.PublicKey); !ok || pub.X.Cmp(mine.X) != 0 {
		t.Fatal("resolution returned a key that is not the pinned one")
	}

	// The impostor's chain is perfectly valid under the shared CA. Pinning is
	// the only thing that refuses it, and it must.
	if _, err := verificationKeyFromPinnedLeaf(x5cHeaderValue(theirsCert), pinned, iss); err == nil {
		t.Fatal("a leaf under the same CA, naming the same issuer, was accepted for login")
	}
}

// Pinning does not excuse the leaf from naming its issuer. A credential whose
// leaf carries the pinned key but speaks for someone else is malformed, and
// reading the key out without looking would accept it.
func TestPinnedLeaf_StillRequiresTheLeafToNameTheIssuer(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Shared PoA Root")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	cert := mintTestCert(t, "Some Other Service", &key.PublicKey, false, caKey, caCert)
	pinned := [][]byte{cert.RawSubjectPublicKeyInfo}

	if _, err := verificationKeyFromPinnedLeaf(x5cHeaderValue(cert), pinned, "https://my.example/issuer"); err == nil {
		t.Fatal("a leaf carrying the pinned key but naming another issuer must be refused")
	}
}

// A login issuer consults no certificate authority at all, so a deployment with
// no anchors configured still verifies its own logins. This is what makes login
// independent of the PoA CA list rather than a stricter reading of it.
func TestPinnedLeaf_NeedsNoTrustAnchors(t *testing.T) {
	caKey, caCert := mintSelfSignedCA(t, "Some CA Nobody Anchors")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	const iss = "https://my.example/issuer"
	cert := mintTestCert(t, iss, &key.PublicKey, false, caKey, caCert)

	pinned := [][]byte{cert.RawSubjectPublicKeyInfo}
	if _, err := verificationKeyFromPinnedLeaf(x5cHeaderValue(cert), pinned, iss); err != nil {
		t.Fatalf("a pinned login issuer must verify without any anchors: %v", err)
	}
}
