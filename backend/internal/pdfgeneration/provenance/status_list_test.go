package provenance

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerminalStatesSetTheBit: every terminal state — including the uppercase
// forms the CWE emits (DCS-OR-C2PA-005 Gap 1) — revokes the contract's entry.
func TestTerminalStatesSetTheBit(t *testing.T) {
	for _, state := range []string{
		"terminated", "expired", "replaced", "suspended",
		"TERMINATED", "EXPIRED", "REPLACED", "SUSPENDED",
	} {
		t.Run(state, func(t *testing.T) {
			p, revocations := newTestPublisher()
			ref, err := p.PublishStatus(context.Background(), "did:example:contract-"+state, state, "", time.Now())
			require.NoError(t, err)
			assert.Equal(t, []uint32{ref.Index}, revocations.indices(DefaultListID),
				"state %q must set the contract's bit", state)
		})
	}
}

// TestNonTerminalStatesLeaveTheBitClear: a contract still in force is not
// revoked, and publishing its state must not read as one.
func TestNonTerminalStatesLeaveTheBitClear(t *testing.T) {
	for _, state := range []string{"active", "draft", "approved", "amended", "ACTIVE"} {
		t.Run(state, func(t *testing.T) {
			p, revocations := newTestPublisher()
			_, err := p.PublishStatus(context.Background(), "did:example:contract-"+state, state, "", time.Now())
			require.NoError(t, err)
			assert.Empty(t, revocations.indices(DefaultListID))
		})
	}
}

// TestPublishedEntryIsTheEntryARevocationFlips: the reference a credential
// carries and the bit a revocation sets come from the same allocation, so a
// verifier that follows the credential lands on the bit that was flipped.
func TestPublishedEntryIsTheEntryARevocationFlips(t *testing.T) {
	p, revocations := newTestPublisher()

	advertised, err := p.PublishStatus(context.Background(), "did:example:contract-flip", "active", "", time.Now())
	require.NoError(t, err)

	revoked, err := p.RevokeStatus(context.Background(), "did:example:contract-flip")
	require.NoError(t, err)

	assert.Equal(t, advertised, revoked)
	assert.Equal(t, []uint32{advertised.Index}, revocations.indices(DefaultListID),
		"the revocation must flip the entry the credential advertises")
}

// TestARevocationKeepsTheMomentItFirstHappened: republishing a terminal state
// must not move the answer to "when did this stop being valid".
func TestARevocationKeepsTheMomentItFirstHappened(t *testing.T) {
	p, revocations := newTestPublisher()

	_, err := p.RevokeStatus(context.Background(), "did:example:contract-twice")
	require.NoError(t, err)
	first := revocations.revokedAt["did:example:contract-twice"]

	_, err = p.PublishStatus(context.Background(), "did:example:contract-twice", "terminated", "", time.Now())
	require.NoError(t, err)

	assert.Equal(t, first, revocations.revokedAt["did:example:contract-twice"])
}

// TestStatusListURIIsTheOriginRootPath: the URI a credential names, the URI a
// verifier fetches and the token's own `sub` are the same string. A verifier
// refuses the list outright when they differ, so the format is pinned.
func TestStatusListURIIsTheOriginRootPath(t *testing.T) {
	assert.Equal(t, "https://dcs.example.org/status-list/1",
		StatusListURI("https://dcs.example.org", 1))
	assert.Equal(t, "https://dcs.example.org/status-list/2",
		StatusListURI("https://dcs.example.org/", 2))
}

// TestTheIssuerIdentifierDropsTheAPIPath: the list is served at the origin root,
// the way did.json is, because a verifier holding only a credential cannot be
// asked to know this deployment's API prefix.
func TestTheIssuerIdentifierDropsTheAPIPath(t *testing.T) {
	issuer, err := StatusListIssuerURL("https://dcs-ionos.facis.cloud/api")
	require.NoError(t, err)
	assert.Equal(t, "https://dcs-ionos.facis.cloud", issuer)

	issuer, err = StatusListIssuerURL("http://localhost:8991/api")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8991", issuer)
}

// TestAnUnusableIssuerIdentifierIsRefused: an origin that names no reachable
// host would produce credentials advertising a URI nothing resolves, which reads
// to every verifier as an unavailable revocation state.
func TestAnUnusableIssuerIdentifierIsRefused(t *testing.T) {
	for _, raw := range []string{"", "   ", "/api"} {
		_, err := StatusListIssuerURL(raw)
		assert.Error(t, err, "%q must not yield an issuer identifier", raw)
	}
}

// TestAPublisherWithoutARevocationStoreRefusesToRevoke: a terminal state that
// sets no bit is the failure this path exists to prevent, and it is invisible —
// every credential keeps advertising an entry that stays clear.
func TestAPublisherWithoutARevocationStoreRefusesToRevoke(t *testing.T) {
	allocator, _ := newTestAllocator(ListSize)
	p := NewDCSStatusListPublisher(
		func(listID int) string { return StatusListURI("https://dcs.example.org", listID) },
		allocator, nil)

	_, err := p.RevokeStatus(context.Background(), "did:example:contract")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revocation store")
}
