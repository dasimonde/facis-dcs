package service

import (
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/dcstodcs"
	db2 "digital-contracting-service/internal/dcstodcs/db"
)

// localWithdrawalCtx is what the receiver knows: it holds the contract and the
// two parties. A withdrawal is a statement about a settlement, so no document
// version enters here.
func localWithdrawalCtx() localWithdrawalContext {
	return localWithdrawalContext{
		ContractIRI: settledIRI,
		LocalPeer:   receiver,
		Parties:     []string{receiver, settlingPeer},
		Now:         time.Now().UTC(),
	}
}

// shipWithdrawalFrom produces a withdrawal the way the withdrawing instance
// produces it — through the sender-side builder, so what the receiver is asked
// to accept is exactly what the ship path emits.
func shipWithdrawalFrom(t *testing.T, doc *identity.DIDDocument, document string, at time.Time) (db2.SettlementWithdrawal, string) {
	t.Helper()
	queued := db2.SettlementWithdrawal{
		DID:            settledIRI,
		FromPeerDID:    settlingPeer,
		ToPeerDID:      receiver,
		DocumentDigest: digestOf(t, document),
		WithdrawnAt:    at.UTC().Truncate(time.Microsecond),
	}
	signature, err := dcstodcs.BuildSettlementWithdrawal(doc, queued)
	if err != nil {
		t.Fatal(err)
	}
	return queued, signature
}

// heldSettlementOf is the row the receiver stored when the peer settled.
func heldSettlementOf(t *testing.T, document string, at time.Time) *db2.Settlement {
	t.Helper()
	return &db2.Settlement{
		DID:            settledIRI,
		FromPeerDID:    settlingPeer,
		ToPeerDID:      receiver,
		DocumentDigest: digestOf(t, document),
		SettledAt:      at.UTC().Truncate(time.Microsecond),
	}
}

func TestVerifyShippedWithdrawalAcceptsWhatTheProducerShips(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	queued, signature := shipWithdrawalFrom(t, peer, settledDocument, time.Now())

	withdrawal, err := verifyShippedWithdrawal(signature, settlingPeer, peer, localWithdrawalCtx())
	if err != nil {
		t.Fatalf("expected the shipped withdrawal to verify, got: %v", err)
	}
	if withdrawal.ContractDID != settledIRI || withdrawal.WithdrawnBy != settlingPeer || withdrawal.WithdrawnFrom != receiver {
		t.Fatalf("withdrawal read against the wrong parties: %+v", withdrawal)
	}
	if withdrawal.DocumentDigest != queued.DocumentDigest {
		t.Fatalf("withdrawal names document %s, want the settled %s", withdrawal.DocumentDigest, queued.DocumentDigest)
	}
	if !withdrawal.WithdrawnAt.Equal(queued.WithdrawnAt) {
		t.Fatalf("withdrawal dated %s, want the queued %s", withdrawal.WithdrawnAt, queued.WithdrawnAt)
	}
}

func TestVerifyShippedWithdrawalRefusesTamperedArtifact(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	_, signature := shipWithdrawalFrom(t, peer, settledDocument, time.Now())
	_, other := shipWithdrawalFrom(t, peer, `{"dcs:clause":"a different document"}`, time.Now())

	parts := strings.Split(signature, ".")
	tampered := parts[0] + "." + strings.Split(other, ".")[1] + "." + parts[2]

	_, err := verifyShippedWithdrawal(tampered, settlingPeer, peer, localWithdrawalCtx())
	if err == nil || !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("expected the tampered withdrawal to be refused, got: %v", err)
	}
}

// The whole point of the artifact: only the party that gave the agreement may
// take it back. A key the peer does not publish is anybody's key.
func TestVerifyShippedWithdrawalRefusesForeignKey(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	imposter := jadesTestDIDDocument(t, "dcs-evil.localhost")
	_, signature := shipWithdrawalFrom(t, imposter, settledDocument, time.Now())

	_, err := verifyShippedWithdrawal(signature, settlingPeer, peer, localWithdrawalCtx())
	if err == nil || !strings.Contains(err.Error(), "not published by peer") {
		t.Fatalf("expected a withdrawal signed by a key the peer does not publish to be refused, got: %v", err)
	}
}

// A settlement and a withdrawal of it are opposite statements over the same
// fields; neither may be read as the other.
func TestVerifyShippedWithdrawalRefusesASettlementReplay(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	settlement := shipSettlementFrom(t, peer, settledDocument, time.Now())

	_, err := verifyShippedWithdrawal(settlement.JadesSignature, settlingPeer, peer, localWithdrawalCtx())
	if err == nil || !strings.Contains(err.Error(), "not a dcs:ContractSettlementWithdrawal") {
		t.Fatalf("expected a replayed settlement to be refused as a withdrawal, got: %v", err)
	}
}

func TestVerifyShippedWithdrawalRefusesNonCanonicalPayload(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	at := time.Now().UTC().Truncate(time.Microsecond)
	jws := signSettlementPayload(t, peer, map[string]any{
		"@context":                   map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@type":                      jades.SettlementWithdrawalType,
		"dcs:contractDid":            settledIRI,
		"dcs:contractDocumentDigest": digestOf(t, settledDocument),
		"dcs:withdrawnBy":            settlingPeer,
		"dcs:withdrawnFrom":          receiver,
		"dcs:withdrawnAt":            at.Format(time.RFC3339Nano),
		"dcs:alsoRevokesEverything":  true,
	})

	_, err := verifyShippedWithdrawal(jws, settlingPeer, peer, localWithdrawalCtx())
	if err == nil || !strings.Contains(err.Error(), "not the canonical form") {
		t.Fatalf("expected an artifact carrying more than the withdrawal to be refused, got: %v", err)
	}
}

func TestVerifyShippedWithdrawalRefusesWrongContractAudienceAndNonParty(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	_, signature := shipWithdrawalFrom(t, peer, settledDocument, time.Now())

	elsewhere := localWithdrawalCtx()
	elsewhere.ContractIRI = "did:web:dcs-a.localhost:contract:OTHER"
	if _, err := verifyShippedWithdrawal(signature, settlingPeer, peer, elsewhere); err == nil ||
		!strings.Contains(err.Error(), "binds contract") {
		t.Fatalf("expected a withdrawal of another contract to be refused, got: %v", err)
	}

	thirdParty := localWithdrawalCtx()
	thirdParty.LocalPeer = "did:web:dcs-c.localhost"
	if _, err := verifyShippedWithdrawal(signature, settlingPeer, peer, thirdParty); err == nil ||
		!strings.Contains(err.Error(), "not this instance") {
		t.Fatalf("expected a withdrawal made toward another instance to be refused, got: %v", err)
	}

	stranger := localWithdrawalCtx()
	stranger.Parties = []string{receiver, "did:web:dcs-c.localhost"}
	if _, err := verifyShippedWithdrawal(signature, settlingPeer, peer, stranger); err == nil ||
		!strings.Contains(err.Error(), "is not a party") {
		t.Fatalf("expected a withdrawal from a non-party to be refused, got: %v", err)
	}
}

func TestVerifyShippedWithdrawalRefusesFutureDatedArtifact(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	_, signature := shipWithdrawalFrom(t, peer, settledDocument, time.Now().Add(time.Hour))

	_, err := verifyShippedWithdrawal(signature, settlingPeer, peer, localWithdrawalCtx())
	if err == nil || !strings.Contains(err.Error(), "clock skew") {
		t.Fatalf("expected a future-dated withdrawal to be refused, got: %v", err)
	}
}

// The defect this closes (#46): a party that rejects and redlines has taken its
// agreement back, and until the peer is told, the peer's signing gate still
// reads the settlement it holds as evidence and lets a signature through.
func TestWithdrawalTakesBackTheSettlementItNames(t *testing.T) {
	settledAt := time.Now().Add(-time.Minute)
	withdrawal := jades.SettlementWithdrawal{
		DocumentDigest: digestOf(t, settledDocument),
		WithdrawnAt:    time.Now(),
	}

	applies, reason := withdrawalTakesBack(withdrawal, heldSettlementOf(t, settledDocument, settledAt))
	if !applies {
		t.Fatalf("expected the withdrawal of the held settlement to remove it, got: %s", reason)
	}
}

// A withdrawal held back and re-delivered into a later round names the version
// it took back, which is no longer the version the peer stands behind. Deleting
// on arrival alone would let a replay strip a fresh agreement.
func TestWithdrawalDoesNotTakeBackASettlementOfAnotherVersion(t *testing.T) {
	fresh := heldSettlementOf(t, `{"dcs:clause":"the renegotiated terms"}`, time.Now().Add(-time.Minute))
	stale := jades.SettlementWithdrawal{
		DocumentDigest: digestOf(t, settledDocument),
		WithdrawnAt:    time.Now().Add(-time.Hour),
	}

	applies, reason := withdrawalTakesBack(stale, fresh)
	if applies {
		t.Fatal("a withdrawal naming an earlier version must not remove the settlement of a later one")
	}
	if !strings.Contains(reason, "while the settlement held covers") {
		t.Fatalf("the refusal must name both versions, got: %s", reason)
	}
}

// The case the digest alone cannot catch: a party that rejects and then
// re-settles the SAME document produces a settlement with the same digest.
// Without the ordering check, the earlier withdrawal would delete it.
func TestWithdrawalDoesNotTakeBackASettlementMadeAfterIt(t *testing.T) {
	resettled := heldSettlementOf(t, settledDocument, time.Now())
	stale := jades.SettlementWithdrawal{
		DocumentDigest: digestOf(t, settledDocument),
		WithdrawnAt:    time.Now().Add(-time.Hour),
	}

	applies, reason := withdrawalTakesBack(stale, resettled)
	if applies {
		t.Fatal("a withdrawal dated before the settlement it names must not remove it")
	}
	if !strings.Contains(reason, "before the settlement it would remove") {
		t.Fatalf("the refusal must say it predates the settlement, got: %s", reason)
	}
}

// Delivery is retried until the peer accepts, so a withdrawal that has already
// been applied must not be an error — or the sender re-attempts forever.
func TestWithdrawalOfNothingIsNotAFailure(t *testing.T) {
	withdrawal := jades.SettlementWithdrawal{
		DocumentDigest: digestOf(t, settledDocument),
		WithdrawnAt:    time.Now(),
	}

	applies, reason := withdrawalTakesBack(withdrawal, nil)
	if applies {
		t.Fatal("there is no settlement to remove")
	}
	if !strings.Contains(reason, "no settlement from that peer is held") {
		t.Fatalf("the reason must say nothing was held, got: %s", reason)
	}
}
