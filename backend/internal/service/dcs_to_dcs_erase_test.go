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
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/pgtest"
	trustgate "digital-contracting-service/internal/dcstodcs"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/ecutil"
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
					"x":   ecutil.MustJWKX(ecutil.AsPublic(otherKey)),
					"y":   ecutil.MustJWKY(ecutil.AsPublic(otherKey)),
				},
			},
			{
				"id": "did:web:" + host + "#key-1",
				"publicKeyJwk": map[string]any{
					"kty": "EC",
					"crv": "P-256",
					"x":   ecutil.MustJWKX(ecutil.AsPublic(key)),
					"y":   ecutil.MustJWKY(ecutil.AsPublic(key)),
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

// partiesErasure allows the erase to the named parties, unless refusal is set.
type partiesErasure struct {
	parties []string
	refusal error
}

func (e *partiesErasure) AuthorizeErase(_ context.Context, contractIRI, peerDID string) error {
	if !slices.Contains(e.parties, peerDID) {
		return fmt.Errorf("%w: peer %s is not a party of %s", trustgate.ErrNoSharedCopy, peerDID, contractIRI)
	}
	return e.refusal
}

// memoryNonces is the nonce store of one test receiver.
type memoryNonces map[string]bool

func (n memoryNonces) Use(_ context.Context, fromDID, nonce string, _ time.Time) (bool, error) {
	if n[fromDID+"|"+nonce] {
		return false, nil
	}
	n[fromDID+"|"+nonce] = true
	return true, nil
}

// stubGate answers every trust gate check with err.
type stubGate struct{ err error }

func (g stubGate) Check(context.Context, string, trustgate.Direction, string, string) error {
	return g.err
}

const (
	eraseTestIRI   = "did:web:local.example:contract:42"
	eraseTestLocal = "did:web:local.example"
	eraseTestPeer  = "did:web:peer.example"
)

func newEraseTestService(t *testing.T, peerDoc *identity.DIDDocument, parties []string) (*dcsToDcssrvc, *recordingShredder, func()) {
	t.Helper()
	localDoc := eraseTestDIDDocument(t, "local.example")
	shredder := &recordingShredder{}
	svc := &dcsToDcssrvc{
		DIDDocument: *localDoc,
		Shredder:    shredder,
		Erasure:     &partiesErasure{parties: parties},
		Nonces:      memoryNonces{},
		TrustGate:   stubGate{},
	}
	previous := fetchPeerDIDDocument
	fetchPeerDIDDocument = func(string) (*identity.DIDDocument, error) { return peerDoc, nil }
	return svc, shredder, func() { fetchPeerDIDDocument = previous }
}

// signedFor signs a challenge of fromDID for op on contractIRI toward toDID.
func signedFor(t *testing.T, signer *identity.DIDDocument, op trustgate.PeerOp, fromDID, toDID, contractIRI string, body []byte) (string, []byte) {
	t.Helper()
	value, hash, err := trustgate.SignPeerChallenge(signer, op, fromDID, toDID, contractIRI, body)
	require.NoError(t, err)
	return value, hash
}

func eraseRequest(t *testing.T, signer *identity.DIDDocument, contractIRI string) *dcstodcs.DCSToDCSContractEraseRequest {
	t.Helper()
	value, hash := signedFor(t, signer, trustgate.OpErase, eraseTestPeer, eraseTestLocal, contractIRI, nil)
	return &dcstodcs.DCSToDCSContractEraseRequest{FromPeerDid: eraseTestPeer, ContractIri: contractIRI, SecretValue: value, SecretHash: hash}
}

func requireServiceError(t *testing.T, err error, name string) {
	t.Helper()
	var serviceErr *goa.ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, name, serviceErr.Name, err.Error())
}

func TestEraseRejectsFailedChallenge(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{eraseTestPeer})
	defer restore()

	// The challenge is signed by a key that is NOT the peer's published one.
	_, err := svc.Erase(context.Background(), eraseRequest(t, eraseTestDIDDocument(t, "peer.example"), eraseTestIRI))
	requireServiceError(t, err, "bad_request")
	require.Empty(t, shredder.calls, "nothing may be shredded on a failed challenge")
}

func TestEraseRejectsNonPartyPeer(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{"did:web:someone-else.example"})
	defer restore()

	_, err := svc.Erase(context.Background(), eraseRequest(t, peerDoc, eraseTestIRI))
	requireServiceError(t, err, "not_found")
	require.Empty(t, shredder.calls, "nothing may be shredded for a non-party peer")
}

// DCS-FR-CWE-20: a copy in force or under retention is not erased on a peer's
// request. The peer may ask again later.
func TestEraseRefusedWhileTheCopyMayNotBeErased(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{eraseTestPeer})
	defer restore()
	svc.Erasure = &partiesErasure{parties: []string{eraseTestPeer}, refusal: fmt.Errorf("%w: contract is ACTIVE", trustgate.ErrNotErasable)}

	_, err := svc.Erase(context.Background(), eraseRequest(t, peerDoc, eraseTestIRI))
	requireServiceError(t, err, "bad_request")
	require.Empty(t, shredder.calls)
}

func TestEraseShredsForAuthenticatedParty(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{eraseTestLocal, eraseTestPeer})
	defer restore()
	svc.DB = pgtest.Open(t)

	res, err := svc.Erase(context.Background(), eraseRequest(t, peerDoc, eraseTestIRI))
	require.NoError(t, err)
	require.Equal(t, eraseTestLocal, res.FromPeerDid)
	require.Len(t, shredder.calls, 1)
	require.Equal(t, eraseTestIRI, shredder.calls[0].iri)
	require.Equal(t, eraseTestPeer, shredder.calls[0].actor)
	require.Len(t, archiveEvents(t, svc.DB, "PEER_EXCHANGE"), 1, "a received erase is audited (DCS-NFR-BR-08)")
}

func TestEraseShredFailureIsInternalError(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{eraseTestPeer})
	defer restore()
	shredder.err = errors.New("db down")

	_, err := svc.Erase(context.Background(), eraseRequest(t, peerDoc, eraseTestIRI))
	requireServiceError(t, err, "internal_error")
}

// A peer that the federation trust gate denies cannot erase or terminate a
// contract (ADR-19), and the denial is an incident.
func TestEraseAndTerminateRefusedWhenTrustGateDenies(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, shredder, restore := newEraseTestService(t, peerDoc, []string{eraseTestPeer})
	defer restore()
	database := pgtest.Open(t)
	svc.DB = database
	svc.TrustGate = stubGate{err: &trustgate.GateError{Kind: trustgate.AgreementFailure, PeerDID: eraseTestPeer, Err: errors.New("agreement credential withdrawn")}}
	_, err := database.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, responsible)
		VALUES ($1, $2::text, 'Org A', 'ACTIVE', 'tpl', 1, jsonb_build_object('creator', $2::text, 'counterparty', $3::text))`,
		eraseTestIRI, eraseTestLocal, eraseTestPeer)
	require.NoError(t, err)

	_, err = svc.Erase(context.Background(), eraseRequest(t, peerDoc, eraseTestIRI))
	requireServiceError(t, err, "bad_request")
	require.Empty(t, shredder.calls)

	reason := "breach"
	value, hash := signedFor(t, peerDoc, trustgate.OpTerminate, eraseTestPeer, eraseTestLocal, eraseTestIRI, []byte(reason))
	_, err = svc.Terminate(context.Background(), &dcstodcs.DCSToDCSContractTerminateRequest{
		FromPeerDid: eraseTestPeer, ContractIri: eraseTestIRI, Reason: &reason, SecretValue: value, SecretHash: hash,
	})
	requireServiceError(t, err, "bad_request")
	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did = $1`, eraseTestIRI))
	require.Equal(t, "ACTIVE", state)

	require.Equal(t, 2, outboxCount(t, database, "PAC_TRUST_GATE_DENIAL", eraseTestIRI))
	require.Zero(t, outboxCount(t, database, "PEER_EXCHANGE", eraseTestIRI), "a refused exchange is not audited as received")
}

func outboxCount(t *testing.T, database *sqlx.DB, eventType, did string) int {
	t.Helper()
	var count int
	require.NoError(t, database.Get(&count, `SELECT count(*) FROM outbox_events WHERE event_type = $1 AND did = $2`, eventType, did))
	return count
}
