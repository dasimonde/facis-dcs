package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"digital-contracting-service/internal/base/identity"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

// eraseTestDIDDocument builds a DIDDocument with a self-signed x5c leaf whose
// SAN matches the did:web host, so the peer-auth layers of the erase handler
// (certificate + challenge-response) run against it like against a fetched
// peer document.
func eraseTestDIDDocument(t *testing.T, host string) *identity.DIDDocument {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	// A conformant shape this deployment does not itself emit: the identity key
	// is NOT the first verification method, and the authentication relationship
	// names it with a relative DID URL.
	didJSON := map[string]any{
		"id": "did:web:" + host,
		"verificationMethod": []map[string]any{
			{
				"id": "did:web:" + host + "#unrelated-encryption-key",
				"publicKeyJwk": map[string]any{
					"kty": "EC",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(otherKey.X.FillBytes(make([]byte, 32))),
					"y":   base64.RawURLEncoding.EncodeToString(otherKey.Y.FillBytes(make([]byte, 32))),
				},
			},
			{
				"id": "did:web:" + host + "#key-1",
				"publicKeyJwk": map[string]any{
					"kty": "EC",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
					"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
					"x5c": []string{base64.StdEncoding.EncodeToString(certDER)},
				},
			},
		},
		"authentication": []string{"#key-1"},
		"keyAgreement":   []string{"#unrelated-encryption-key"},
	}
	raw, err := json.Marshal(didJSON)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "did.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := identity.NewDIDDocument(path, key)
	if err != nil {
		t.Fatalf("NewDIDDocument: %v", err)
	}
	return doc
}

type recordingShredder struct {
	calls []struct{ iri, actor, reason string }
	err   error
}

func (r *recordingShredder) Shred(_ context.Context, iri, actor, reason string) (int64, error) {
	r.calls = append(r.calls, struct{ iri, actor, reason string }{iri, actor, reason})
	return 1, r.err
}

type staticParties struct {
	parties []string
	err     error
}

func (s *staticParties) Parties(context.Context, string) ([]string, error) { return s.parties, s.err }

const eraseTestIRI = "did:web:local.example:contract:42"

func newEraseTestService(t *testing.T, peerDoc *identity.DIDDocument, parties []string) (*dcsToDcssrvc, *recordingShredder, func()) {
	t.Helper()
	localDoc := eraseTestDIDDocument(t, "local.example")
	shredder := &recordingShredder{}
	svc := &dcsToDcssrvc{
		DIDDocument: *localDoc,
		Shredder:    shredder,
		Parties:     &staticParties{parties: parties},
	}
	previous := fetchPeerDIDDocument
	fetchPeerDIDDocument = func(string) (*identity.DIDDocument, error) { return peerDoc, nil }
	return svc, shredder, func() { fetchPeerDIDDocument = previous }
}

func TestEraseRejectsFailedChallenge(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{"did:web:peer.example"})
	defer restore()

	// The secret is signed by a key that is NOT the peer's published one.
	wrongDoc := eraseTestDIDDocument(t, "peer.example")
	secret := "challenge"
	hash, err := wrongDoc.Sign([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: "did:web:peer.example",
		ContractIri: eraseTestIRI,
		SecretValue: secret,
		SecretHash:  hash,
	})
	if err == nil {
		t.Fatal("a failed challenge must reject the erase")
	}
	if len(shredder.calls) != 0 {
		t.Fatal("nothing may be shredded on a failed challenge")
	}
}

func TestEraseRejectsNonPartyPeer(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{"did:web:someone-else.example"})
	defer restore()

	secret := "challenge"
	hash, err := peerDoc.Sign([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: "did:web:peer.example",
		ContractIri: eraseTestIRI,
		SecretValue: secret,
		SecretHash:  hash,
	})
	if err == nil {
		t.Fatal("a peer that is not a party of the contract must be rejected")
	}
	if len(shredder.calls) != 0 {
		t.Fatal("nothing may be shredded for a non-party peer")
	}
}

func TestEraseShredsForAuthenticatedParty(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{"did:web:local.example", "did:web:peer.example"})
	defer restore()

	secret := "challenge"
	hash, err := peerDoc.Sign([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: "did:web:peer.example",
		ContractIri: eraseTestIRI,
		SecretValue: secret,
		SecretHash:  hash,
	})
	if err != nil {
		t.Fatalf("Erase: %v", err)
	}
	if res.FromPeerDid != "did:web:local.example" {
		t.Fatalf("unexpected confirming peer %q", res.FromPeerDid)
	}
	if len(shredder.calls) != 1 {
		t.Fatalf("expected one shred, got %+v", shredder.calls)
	}
	call := shredder.calls[0]
	if call.iri != eraseTestIRI || call.actor != "did:web:peer.example" {
		t.Fatalf("unexpected shred attribution: %+v", call)
	}
}

func TestEraseShredFailureIsInternalError(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{"did:web:peer.example"})
	defer restore()
	shredder.err = errors.New("db down")

	secret := "challenge"
	hash, err := peerDoc.Sign([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: "did:web:peer.example",
		ContractIri: eraseTestIRI,
		SecretValue: secret,
		SecretHash:  hash,
	}); err == nil {
		t.Fatal("a shred failure must propagate as an error")
	}
}
