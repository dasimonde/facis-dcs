package dcstodcs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/pgtest"
)

type memoryNonces map[string]bool

func (n memoryNonces) Use(_ context.Context, fromDID, nonce string, _ time.Time) (bool, error) {
	if n[fromDID+"|"+nonce] {
		return false, nil
	}
	n[fromDID+"|"+nonce] = true
	return true, nil
}

const (
	challengeFrom     = "did:web:a.example"
	challengeTo       = "did:web:b.example"
	challengeContract = "did:web:a.example:contract:1"
)

// A peer challenge authenticates exactly the request it was signed for (ADR-13).
func TestAPeerChallengeAuthenticatesOnlyTheRequestItWasSignedFor(t *testing.T) {
	signer := eraserTestDIDDocument(t, "a.example")
	value, hash, err := SignPeerChallenge(signer, OpTerminate, challengeFrom, challengeTo, challengeContract, []byte("breach"))
	require.NoError(t, err)
	signed := PeerRequest{Op: OpTerminate, FromDID: challengeFrom, ContractIRI: challengeContract, Body: []byte("breach"), SecretValue: value, SecretHash: hash}
	verify := func(req PeerRequest, localPeer string, now time.Time) error {
		return VerifyPeerRequest(context.Background(), memoryNonces{}, signer, nil, localPeer, req, now)
	}

	require.NoError(t, verify(signed, challengeTo, time.Now()))
	// The receiver compares did:web identifiers by what they denote.
	require.NoError(t, verify(signed, "did:web:B.example", time.Now()))

	for name, c := range map[string]struct {
		mutate    func(*PeerRequest)
		localPeer string
		now       time.Time
		refusal   string
	}{
		"another operation":   {mutate: func(r *PeerRequest) { r.Op = OpErase }, refusal: "signed for terminate"},
		"another contract":    {mutate: func(r *PeerRequest) { r.ContractIRI = "did:web:a.example:contract:2" }, refusal: "names contract"},
		"another sender":      {mutate: func(r *PeerRequest) { r.FromDID = "did:web:c.example" }, refusal: "signed by"},
		"another receiver":    {localPeer: "did:web:c.example", refusal: "addressed to"},
		"another body":        {mutate: func(r *PeerRequest) { r.Body = []byte("mutual agreement") }, refusal: "does not bind the body"},
		"an iat too old":      {now: time.Now().Add(PeerChallengeWindow + 5*time.Second), refusal: "check NTP"},
		"an iat too far on":   {now: time.Now().Add(-PeerChallengeWindow - 5*time.Second), refusal: "check NTP"},
		"another signature":   {mutate: func(r *PeerRequest) { r.SecretHash = append([]byte{}, hash[:len(hash)-1]...) }, refusal: "verifies against none"},
		"a bare random value": {mutate: func(r *PeerRequest) { r.SecretValue = "challenge" }, refusal: "not a dcs-peer-v1 challenge"},
		"an edited value": {mutate: func(r *PeerRequest) {
			r.SecretValue = strings.Replace(r.SecretValue, challengeTo, "did:web:c.example", 1)
		}, localPeer: "did:web:c.example", refusal: "verifies against none"},
	} {
		t.Run(name, func(t *testing.T) {
			req := signed
			if c.mutate != nil {
				c.mutate(&req)
			}
			localPeer, now := c.localPeer, c.now
			if localPeer == "" {
				localPeer = challengeTo
			}
			if now.IsZero() {
				now = time.Now()
			}
			err := verify(req, localPeer, now)
			require.ErrorIs(t, err, ErrPeerChallengeRefused)
			require.ErrorContains(t, err, c.refusal)
		})
	}
}

func TestAPeerChallengeIsAcceptedOnce(t *testing.T) {
	signer := eraserTestDIDDocument(t, "a.example")
	value, hash, err := SignPeerChallenge(signer, OpErase, challengeFrom, challengeTo, challengeContract, nil)
	require.NoError(t, err)
	req := PeerRequest{Op: OpErase, FromDID: challengeFrom, ContractIRI: challengeContract, SecretValue: value, SecretHash: hash}
	nonces := memoryNonces{}

	require.NoError(t, VerifyPeerRequest(context.Background(), nonces, signer, nil, challengeTo, req, time.Now()))
	err = VerifyPeerRequest(context.Background(), nonces, signer, nil, challengeTo, req, time.Now())
	require.ErrorIs(t, err, ErrPeerChallengeRefused)
	require.ErrorContains(t, err, "already used")
}

// A nonce is recorded only for a challenge whose signature verified.
func TestAnUnsignedChallengeRecordsNoNonce(t *testing.T) {
	signer := eraserTestDIDDocument(t, "a.example")
	value, _, err := SignPeerChallenge(signer, OpErase, challengeFrom, challengeTo, challengeContract, nil)
	require.NoError(t, err)
	nonces := memoryNonces{}
	err = VerifyPeerRequest(context.Background(), nonces, signer, nil, challengeTo,
		PeerRequest{Op: OpErase, FromDID: challengeFrom, ContractIRI: challengeContract, SecretValue: value, SecretHash: []byte("forged")}, time.Now())
	require.ErrorIs(t, err, ErrPeerChallengeRefused)
	require.Empty(t, nonces)
}

func TestTheChallengeBuilderRefusesTheSeparator(t *testing.T) {
	_, _, err := SignPeerChallenge(eraserTestDIDDocument(t, "a.example"), OpErase, challengeFrom, challengeTo, "did:web:a.example:contract:1|x", nil)
	require.ErrorContains(t, err, "separator")
}

// The negotiation message body is canonical, so the receiver computes the
// digest the sender signed from what it decoded.
func TestTheNegotiationMessageBodyIgnoresTheChallengeAndKeyOrder(t *testing.T) {
	decision := "ACCEPTED"
	sent := &dcstodcs.DCSToDCSNegotiationMessageRequest{
		FromPeerDid: challengeFrom, ContractIri: challengeContract, MessageID: "m1", Kind: "PROPOSAL",
		ChangeRequest: map[string]any{"b": 1.0, "a": []any{"x"}}, Decision: &decision,
	}
	received := *sent
	received.ChangeRequest = map[string]any{"a": []any{"x"}, "b": 1.0}
	received.SecretValue, received.SecretHash = "signed", []byte("signature")

	sentBody, err := NegotiationMessageBody(sent)
	require.NoError(t, err)
	receivedBody, err := NegotiationMessageBody(&received)
	require.NoError(t, err)
	require.Equal(t, sentBody, receivedBody)

	received.MessageID = "m2"
	changed, err := NegotiationMessageBody(&received)
	require.NoError(t, err)
	require.NotEqual(t, sentBody, changed)
}

// The nonce store is shared by every replica through Postgres, and expired
// nonces leave it.
func TestPeerChallengeNoncesAreSharedAndExpire(t *testing.T) {
	db := pgtest.Open(t)
	replicaA, replicaB := DBPeerNonces{DB: db}, DBPeerNonces{DB: db}
	ctx := context.Background()

	fresh, err := replicaA.Use(ctx, challengeFrom, "n1", time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, fresh)
	fresh, err = replicaB.Use(ctx, challengeFrom, "n1", time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.False(t, fresh, "another replica accepted a nonce already used")
	fresh, err = replicaB.Use(ctx, "did:web:c.example", "n1", time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, fresh, "nonces are per sender")

	_, err = db.Exec(`INSERT INTO peer_challenge_nonces (from_did, nonce, expires_at) VALUES ($1, 'old', now() - INTERVAL '1 second')`, challengeFrom)
	require.NoError(t, err)
	_, err = replicaA.Use(ctx, challengeFrom, "n2", time.Now().Add(time.Minute))
	require.NoError(t, err)
	var left int
	require.NoError(t, db.Get(&left, `SELECT count(*) FROM peer_challenge_nonces WHERE nonce = 'old'`))
	require.Zero(t, left)
}

func TestAFinalPeerRefusalIsNotFoundOnly(t *testing.T) {
	require.True(t, IsFinalPeerRefusal(dcstodcs.MakeNotFound(errors.New("no copy"))))
	require.False(t, IsFinalPeerRefusal(dcstodcs.MakeBadRequest(errors.New("retention runs"))))
	require.False(t, IsFinalPeerRefusal(errors.New("connection refused")))
	require.False(t, IsFinalPeerRefusal(nil))
}
