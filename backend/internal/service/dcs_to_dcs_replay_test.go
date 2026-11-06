package service

// Peer challenge replay (ADR-13). Instance A (a.example) is the honest sender.
// Instance B (b.example) legitimately receives requests from A. Instance C
// (c.example) holds a different contract with A. Whatever B captured from A
// does not authenticate B as A anywhere else, nor twice.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/pgtest"
	trustgate "digital-contracting-service/internal/dcstodcs"
)

const (
	replayA   = "did:web:a.example"
	replayB   = "did:web:b.example"
	replayC   = "did:web:c.example"
	contractB = "did:web:a.example:contract:with-b"
	contractC = "did:web:a.example:contract:with-c"
)

func replayReceiver(t *testing.T, host string, parties []string) (*dcsToDcssrvc, *recordingShredder) {
	t.Helper()
	local := eraseTestDIDDocument(t, host)
	shredder := &recordingShredder{}
	return &dcsToDcssrvc{
		DIDDocument: *local, Shredder: shredder, Erasure: &partiesErasure{parties: parties},
		Nonces: memoryNonces{}, TrustGate: stubGate{},
	}, shredder
}

func withPeerDocument(t *testing.T, doc *identity.DIDDocument) {
	t.Helper()
	previous := fetchPeerDIDDocument
	fetchPeerDIDDocument = func(string) (*identity.DIDDocument, error) { return doc, nil }
	t.Cleanup(func() { fetchPeerDIDDocument = previous })
}

// A's erase request to B, replayed by B to C with the IRI of the A-C
// contract, is refused by C. B refuses the same request a second time.
func TestAnEraseChallengeSignedForBIsRefusedByCAndByBTwice(t *testing.T) {
	aDoc := eraseTestDIDDocument(t, "a.example")
	withPeerDocument(t, aDoc)
	receiverB, shredB := replayReceiver(t, "b.example", []string{replayA, replayB})
	receiverB.DB = pgtest.Open(t)
	receiverC, shredC := replayReceiver(t, "c.example", []string{replayA, replayC})

	value, hash := signedFor(t, aDoc, trustgate.OpErase, replayA, replayB, contractB, nil)
	_, err := receiverB.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: replayA, ContractIri: contractB, SecretValue: value, SecretHash: hash,
	})
	require.NoError(t, err, "the honest request to B succeeds")
	require.Len(t, shredB.calls, 1)

	for _, iri := range []string{contractC, contractB} {
		_, err = receiverC.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
			FromPeerDid: replayA, ContractIri: iri, SecretValue: value, SecretHash: hash,
		})
		requireServiceError(t, err, "bad_request")
	}
	require.Empty(t, shredC.calls, "C shredded on a challenge A signed for B")

	_, err = receiverB.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: replayA, ContractIri: contractB, SecretValue: value, SecretHash: hash,
	})
	requireServiceError(t, err, "bad_request")
	require.Len(t, shredB.calls, 1, "B accepted the same challenge twice")
}

// A termination notice is bound to the terminate operation: as an erase it is
// refused, even by the receiver it was signed for.
func TestATerminationNoticeIsRefusedAsAnErase(t *testing.T) {
	aDoc := eraseTestDIDDocument(t, "a.example")
	withPeerDocument(t, aDoc)
	receiverC, shredC := replayReceiver(t, "c.example", []string{replayA, replayC})

	reason := "breach"
	value, hash := signedFor(t, aDoc, trustgate.OpTerminate, replayA, replayC, contractC, []byte(reason))
	_, err := receiverC.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: replayA, ContractIri: contractC, SecretValue: value, SecretHash: hash,
	})
	requireServiceError(t, err, "bad_request")
	require.Empty(t, shredC.calls)
}

// The termination reason is part of what A signed: a relay that changes it is
// refused.
func TestATerminationNoticeWithAChangedReasonIsRefused(t *testing.T) {
	aDoc := eraseTestDIDDocument(t, "a.example")
	withPeerDocument(t, aDoc)
	receiverC, _ := replayReceiver(t, "c.example", []string{replayA, replayC})

	value, hash := signedFor(t, aDoc, trustgate.OpTerminate, replayA, replayC, contractC, []byte("breach"))
	changed := "mutual agreement"
	_, err := receiverC.Terminate(context.Background(), &dcstodcs.DCSToDCSContractTerminateRequest{
		FromPeerDid: replayA, ContractIri: contractC, Reason: &changed, SecretValue: value, SecretHash: hash,
	})
	requireServiceError(t, err, "bad_request")
	require.ErrorContains(t, err, "does not bind the body")
}

// A challenge in the format used before dcs-peer-v1 (a bare random string) is
// refused.
func TestAnUnboundChallengeIsRefused(t *testing.T) {
	aDoc := eraseTestDIDDocument(t, "a.example")
	withPeerDocument(t, aDoc)
	receiverC, shredC := replayReceiver(t, "c.example", []string{replayA, replayC})

	hash, err := aDoc.Sign([]byte("challenge"))
	require.NoError(t, err)
	_, err = receiverC.Erase(context.Background(), &dcstodcs.DCSToDCSContractEraseRequest{
		FromPeerDid: replayA, ContractIri: contractC, SecretValue: "challenge", SecretHash: hash,
	})
	requireServiceError(t, err, "bad_request")
	require.Empty(t, shredC.calls)
}
