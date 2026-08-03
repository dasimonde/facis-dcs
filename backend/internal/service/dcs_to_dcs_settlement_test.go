package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gowebpki/jcs"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/dcstodcs"
	db2 "digital-contracting-service/internal/dcstodcs/db"
)

const (
	settlingPeer = "did:web:dcs-b.localhost"
	receiver     = "did:web:dcs-a.localhost"
	settledIRI   = "did:web:dcs-a.localhost:contract:42"
)

// settledContract is the contract both instances hold a copy of.
func settledContract(t *testing.T, document string) *cwedb.Contract {
	t.Helper()
	data := datatype.JSON(document)
	return &cwedb.Contract{
		DID:             settledIRI,
		ContractVersion: 3,
		ContractData:    &data,
		Responsible:     &cwedb.Responsible{Creator: receiver, Counterparty: settlingPeer},
	}
}

func digestOf(t *testing.T, document string) string {
	t.Helper()
	digest, err := jades.ContractDocumentDigest([]byte(document))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// localContext is what the receiver knows: it holds the contract, the document
// and the two parties.
func localContext(t *testing.T, document string) localSettlementContext {
	t.Helper()
	return localSettlementContext{
		ContractIRI:    settledIRI,
		LocalPeer:      receiver,
		DocumentDigest: digestOf(t, document),
		Parties:        []string{receiver, settlingPeer},
		Now:            time.Now().UTC(),
	}
}

// shipSettlementFrom produces a settlement the way the settling instance
// produces it — through the sender-side builder, so what the receiver is asked
// to accept is exactly what the ship path emits.
func shipSettlementFrom(t *testing.T, doc *identity.DIDDocument, document string, at time.Time) db2.Settlement {
	t.Helper()
	settlement, err := dcstodcs.BuildSettlement(doc, settledContract(t, document), settlingPeer, receiver, at)
	if err != nil {
		t.Fatal(err)
	}
	return settlement
}

// signSettlementPayload signs an arbitrary JCS-canonicalized payload as a
// settlement, for the artifacts a well-behaved producer never emits.
func signSettlementPayload(t *testing.T, doc *identity.DIDDocument, payload map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := jades.Sign(doc, canonical)
	if err != nil {
		t.Fatal(err)
	}
	return jws
}

const settledDocument = `{"dcs:metadata":{"dcs:title":"Peer Contract"},"dcs:clause":"the agreed terms"}`

func TestVerifyShippedSettlementAcceptsWhatTheProducerShips(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, settledDocument, time.Now())

	stored, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, localContext(t, settledDocument))
	if err != nil {
		t.Fatalf("expected the shipped settlement to verify, got: %v", err)
	}
	if stored.DID != settledIRI || stored.FromPeerDID != settlingPeer || stored.ToPeerDID != receiver {
		t.Fatalf("settlement stored against the wrong parties: %+v", stored)
	}
	if stored.DocumentDigest != digestOf(t, settledDocument) {
		t.Fatalf("settlement stored with digest %s, want %s", stored.DocumentDigest, digestOf(t, settledDocument))
	}
	if stored.ContractVersion != 3 {
		t.Fatalf("settlement stored with version %d, want the shipped 3", stored.ContractVersion)
	}
	if !stored.SettledAt.Equal(shipped.SettledAt) {
		t.Fatalf("settlement stored at %s, want the signed %s", stored.SettledAt, shipped.SettledAt)
	}
	if stored.JadesSignature != shipped.JadesSignature {
		t.Fatal("the artifact must be kept verbatim so it stays independently re-verifiable")
	}
}

// The receiver's copy of the document is stored through jsonb, which reorders
// keys and rewrites whitespace and number forms. The digest is taken over the
// JCS canonicalization precisely so that does not turn into a refusal.
func TestVerifyShippedSettlementSurvivesDocumentReserialization(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, `{"b":1.10,"a":{"nested":true}}`, time.Now())

	reserialized := localContext(t, "  {\"a\": {\"nested\": true},\n\"b\": 1.1}")
	if _, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, reserialized); err != nil {
		t.Fatalf("expected the re-serialized document to be recognized as the same version, got: %v", err)
	}
}

// A settlement of a document version neither side ever held authorizes nothing,
// whatever the delivery story: the digest binding is what makes the artifact
// evidence rather than an assertion.
func TestVerifyShippedSettlementRefusesADocumentVersionNeverHeld(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, settledDocument, time.Now())

	renegotiated := localContext(t, `{"dcs:metadata":{"dcs:title":"Peer Contract"},"dcs:clause":"the amended terms"}`)
	renegotiated.SupersededDigest = []string{digestOf(t, `{"dcs:clause":"some third document"}`)}
	_, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, renegotiated)
	if err == nil || !strings.Contains(err.Error(), "but this instance holds") {
		t.Fatalf("expected the settlement of an unknown version to be refused naming both documents, got: %v", err)
	}
}

// The defect this closes (#44): a settlement whose first delivery was lost is
// re-shipped unchanged, and by then the document has moved — the first
// signature seals odrl:Offer into odrl:Agreement on both copies. Refusing it
// against the document held AT ARRIVAL made the artifact permanently
// undeliverable and the exchange permanently unfinishable.
func TestVerifyShippedSettlementAcceptsAVersionThisInstanceHasHeld(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, settledDocument, time.Now())

	sealed := localContext(t, `{"dcs:metadata":{"dcs:title":"Peer Contract"},"dcs:policies":{"@type":"odrl:Agreement"}}`)
	sealed.SupersededDigest = []string{digestOf(t, settledDocument)}

	stored, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, sealed)
	if err != nil {
		t.Fatalf("expected the late settlement of a version this instance held to verify, got: %v", err)
	}
	if stored.DocumentDigest != digestOf(t, settledDocument) {
		t.Fatalf("settlement stored against %s, want the version it names (%s)",
			stored.DocumentDigest, digestOf(t, settledDocument))
	}
}

func TestVerifyShippedSettlementRefusesTamperedArtifact(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, settledDocument, time.Now())

	parts := strings.Split(shipped.JadesSignature, ".")
	forged := signSettlementPayload(t, jadesTestDIDDocument(t, "dcs-b.localhost"), map[string]any{"x": 1})
	tampered := parts[0] + "." + strings.Split(forged, ".")[1] + "." + parts[2]

	_, err := verifyShippedSettlement(tampered, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("expected the tampered artifact to be refused, got: %v", err)
	}
}

func TestVerifyShippedSettlementRefusesForeignKey(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	imposter := jadesTestDIDDocument(t, "dcs-evil.localhost")
	shipped := shipSettlementFrom(t, imposter, settledDocument, time.Now())

	_, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "not published by peer") {
		t.Fatalf("expected a settlement signed by a key the peer does not publish to be refused, got: %v", err)
	}
}

// A contract signature the peer legitimately shipped is a different statement
// and may not be replayed as a settlement.
func TestVerifyShippedSettlementRefusesAContractSignatureReplay(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	contractPayload, err := jades.BuildContractPayload(settledIRI, 3, []byte(settledDocument))
	if err != nil {
		t.Fatal(err)
	}
	jws, err := jades.Sign(peer, contractPayload)
	if err != nil {
		t.Fatal(err)
	}

	_, err = verifyShippedSettlement(jws, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "not a dcs:ContractSettlement") {
		t.Fatalf("expected a replayed contract signature to be refused, got: %v", err)
	}
}

// Everything the canonical form does not carry must not ride along unnoticed.
func TestVerifyShippedSettlementRefusesNonCanonicalPayload(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	at := time.Now().UTC().Truncate(time.Microsecond)
	jws := signSettlementPayload(t, peer, map[string]any{
		"@context":                   map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@type":                      jades.SettlementType,
		"dcs:contractDid":            settledIRI,
		"dcs:contractVersion":        3,
		"dcs:contractDocumentDigest": digestOf(t, settledDocument),
		"dcs:settledBy":              settlingPeer,
		"dcs:settledWith":            receiver,
		"dcs:settledAt":              at.Format(time.RFC3339Nano),
		"dcs:alsoRevokesEverything":  true,
	})

	_, err := verifyShippedSettlement(jws, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "not the canonical form") {
		t.Fatalf("expected an artifact carrying more than the settlement to be refused, got: %v", err)
	}
}

func TestVerifyShippedSettlementRefusesWrongContract(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	other := settledContract(t, settledDocument)
	other.DID = "did:web:dcs-a.localhost:contract:OTHER"
	shipped, err := dcstodcs.BuildSettlement(peer, other, settlingPeer, receiver, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	_, err = verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "binds contract") {
		t.Fatalf("expected a settlement of another contract to be refused, got: %v", err)
	}
}

func TestVerifyShippedSettlementRefusesSettlementMadeByAnotherPeer(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped, err := dcstodcs.BuildSettlement(peer, settledContract(t, settledDocument),
		"did:web:dcs-c.localhost", receiver, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	_, err = verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "but shipped by") {
		t.Fatalf("expected a settlement made by somebody other than the shipper to be refused, got: %v", err)
	}
}

// A settlement made toward one instance must not be relayed as evidence to a
// third one.
func TestVerifyShippedSettlementRefusesForeignAudience(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped, err := dcstodcs.BuildSettlement(peer, settledContract(t, settledDocument),
		settlingPeer, "did:web:dcs-c.localhost", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	_, err = verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "not this instance") {
		t.Fatalf("expected a settlement made toward another instance to be refused, got: %v", err)
	}
}

func TestVerifyShippedSettlementRefusesNonParty(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, settledDocument, time.Now())

	local := localContext(t, settledDocument)
	local.Parties = []string{receiver, "did:web:dcs-c.localhost"}

	_, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, local)
	if err == nil || !strings.Contains(err.Error(), "is not a party") {
		t.Fatalf("expected a settlement from a non-party to be refused, got: %v", err)
	}
}

func TestVerifyShippedSettlementRefusesFutureDatedSettlement(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, settledDocument, time.Now().Add(time.Hour))

	_, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, localContext(t, settledDocument))
	if err == nil || !strings.Contains(err.Error(), "clock skew") {
		t.Fatalf("expected a future-dated settlement to be refused, got: %v", err)
	}
}

func TestVerifyShippedSettlementRefusesRollbackToAnOlderSettlement(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	older := time.Now().Add(-2 * time.Hour)
	shipped := shipSettlementFrom(t, peer, settledDocument, older)

	local := localContext(t, settledDocument)
	local.Previous = &db2.Settlement{SettledAt: time.Now().UTC().Add(-time.Minute)}

	_, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, local)
	if err == nil || !strings.Contains(err.Error(), "older than the settlement already held") {
		t.Fatalf("expected a replayed older settlement to be refused, got: %v", err)
	}
}

// A re-delivery of the settlement already held is the retry path, not a
// rollback: the identical artifact must still be accepted.
func TestVerifyShippedSettlementAcceptsARedeliveryOfTheHeldSettlement(t *testing.T) {
	peer := jadesTestDIDDocument(t, "dcs-b.localhost")
	shipped := shipSettlementFrom(t, peer, settledDocument, time.Now())

	local := localContext(t, settledDocument)
	local.Previous = &db2.Settlement{SettledAt: shipped.SettledAt}

	if _, err := verifyShippedSettlement(shipped.JadesSignature, settlingPeer, peer, local); err != nil {
		t.Fatalf("expected the re-delivered settlement to verify, got: %v", err)
	}
}
