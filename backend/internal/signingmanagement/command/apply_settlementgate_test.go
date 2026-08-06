package command

import (
	"context"
	"errors"
	"go/token"
	"testing"
	"time"

	"digital-contracting-service/internal/base/jades"
	dcsdb "digital-contracting-service/internal/dcstodcs/db"
	db "digital-contracting-service/internal/signingmanagement/db"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

const (
	gateLocalPeer = "did:web:dcs-a.localhost"
	gatePeer      = "did:web:dcs-b.localhost"
	settledDigest = "sha256:1e2d3c4b5a69788796a5b4c3d2e1f00112233445566778899aabbccddeeff001"
	otherDigest   = "sha256:ffeeddccbbaa99887766554433221100f0e1d2c3b4a5968778695a4b3c2d1e00"
)

// settlementStore answers the gate from an in-memory map keyed by the settling
// party, the way contract_settlements is keyed by (contract, settling party).
type settlementStore struct {
	rows map[string]*dcsdb.Settlement
	err  error
}

func (s settlementStore) GetSettlement(_ context.Context, _ *sqlx.Tx, _, fromPeerDID string) (*dcsdb.Settlement, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.rows[fromPeerDID], nil
}

func settlementOf(party, digest string, contractVersion int) *dcsdb.Settlement {
	return &dcsdb.Settlement{
		DID:             "did:web:dcs-a.localhost:contract:1",
		FromPeerDID:     party,
		ContractVersion: contractVersion,
		DocumentDigest:  digest,
		SettledAt:       time.Now().UTC(),
		JadesSignature:  "eyJ.header.signature",
	}
}

func bothParties() *db.Responsible {
	return &db.Responsible{Creator: gateLocalPeer, Counterparty: gatePeer}
}

func runGate(t *testing.T, store PeerSettlements, resp *db.Responsible, fields []string) error {
	t.Helper()
	return assertCounterpartiesSettled(context.Background(), nil, store,
		"did:web:dcs-a.localhost:contract:1", gateLocalPeer, resp, fields)
}

// A contract whose declared slots name no other party has no peer to hear
// from: the single-instance multi-signer flow names its fields per signatory,
// so it must pass without any settlement store at all.
func TestSettlementGateLeavesContractWithoutRemotePartyAlone(t *testing.T) {
	require.NoError(t, runGate(t, nil, bothParties(), []string{"signature-1", "signature-2"}))
	require.NoError(t, runGate(t, nil, &db.Responsible{Creator: gateLocalPeer}, []string{gateLocalPeer}))
	require.NoError(t, runGate(t, nil, nil, []string{gatePeer}))
	require.NoError(t, runGate(t, nil, bothParties(), nil))
}

// The defect (#40): local APPROVED was sufficient to sign while the peer was
// still in NEGOTIATION. Absence of the peer's settlement is not agreement.
func TestSettlementGateRefusesUnsettledCounterparty(t *testing.T) {
	store := settlementStore{rows: map[string]*dcsdb.Settlement{
		gateLocalPeer: settlementOf(gateLocalPeer, settledDigest, 3),
	}}

	err := runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer})
	require.ErrorIs(t, err, ErrCounterpartyNotSettled)
	require.Contains(t, err.Error(), gatePeer)
}

// A settlement for the version before a redline was merged must not authorise
// signing the version after it.
func TestSettlementGateRefusesStaleCounterpartySettlement(t *testing.T) {
	store := settlementStore{rows: map[string]*dcsdb.Settlement{
		gateLocalPeer: settlementOf(gateLocalPeer, settledDigest, 3),
		gatePeer:      settlementOf(gatePeer, otherDigest, 3),
	}}

	err := runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer})
	require.ErrorIs(t, err, ErrCounterpartyNotSettled)
	require.Contains(t, err.Error(), otherDigest)
	require.Contains(t, err.Error(), settledDigest)
}

// This instance holding no settlement of its own means it never stated which
// version it agreed to and never shipped that statement, so the counterparty
// is refusing it in the same way.
func TestSettlementGateRefusesWhenThisInstanceHasNotSettled(t *testing.T) {
	store := settlementStore{rows: map[string]*dcsdb.Settlement{
		gatePeer: settlementOf(gatePeer, settledDigest, 3),
	}}

	err := runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer})
	require.ErrorIs(t, err, ErrCounterpartyNotSettled)
}

// Both parties settled the same document: the counters differ, because
// contract_version is a per-instance counter (the sender bumps it on merging a
// redline, the receiver on every inbound ship). Only the digest binds.
func TestSettlementGateAcceptsMatchingDigestsAcrossDifferentCounters(t *testing.T) {
	store := settlementStore{rows: map[string]*dcsdb.Settlement{
		gateLocalPeer: settlementOf(gateLocalPeer, settledDigest, 3),
		gatePeer:      settlementOf(gatePeer, settledDigest, 7),
	}}

	require.NoError(t, runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer}))
}

// A federated contract with no settlement store configured must fail loudly
// rather than wave the signature through — and not as the sentinel, which the
// API answers with "waiting for the counterparty".
func TestSettlementGateHardFailsWithoutAStore(t *testing.T) {
	err := runGate(t, nil, bothParties(), []string{gateLocalPeer, gatePeer})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrCounterpartyNotSettled)
	require.Contains(t, err.Error(), "no settlement store is configured")
}

// A store that cannot answer is not an absent settlement: reporting it as one
// would turn a database outage into "the counterparty has not settled".
func TestSettlementGateReportsStoreFailureAsItself(t *testing.T) {
	store := settlementStore{err: errors.New("connection refused")}

	err := runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrCounterpartyNotSettled)
	require.Contains(t, err.Error(), "connection refused")
}

// Evidence is only evidence about the party that made it. A settlement held
// from a third instance — verified, current, naming the very document this
// instance settled — says nothing about the counterparty, and a gate that
// counted any settlement would be satisfied by a peer the contract does not
// name.
func TestSettlementGateRefusesASettlementFromSomebodyElse(t *testing.T) {
	store := settlementStore{rows: map[string]*dcsdb.Settlement{
		gateLocalPeer:                settlementOf(gateLocalPeer, settledDigest, 3),
		"did:web:dcs-c.localhost":    settlementOf("did:web:dcs-c.localhost", settledDigest, 3),
		"did:web:dcs-evil.localhost": settlementOf("did:web:dcs-evil.localhost", settledDigest, 3),
	}}

	err := runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer})
	require.ErrorIs(t, err, ErrCounterpartyNotSettled)
	require.Contains(t, err.Error(), gatePeer)
}

func documentDigest(t *testing.T, document string) string {
	t.Helper()
	digest, err := jades.ContractDocumentDigest([]byte(document))
	require.NoError(t, err)
	return digest
}

// The version binding, driven by the documents themselves rather than by
// digests written out by hand: a redline merged after the counterparty settled
// produces a different document, so the statement the counterparty signed is
// about a version that no longer exists. Signing it anyway is exactly how an
// adjustment nobody agreed to would slip under a signature.
//
// Neither instance's contract_version says this. That counter is per-instance —
// the sender bumps it on merging a redline, the receiver on every inbound ship —
// which is why the settlement the counterparty is finally held to here carries a
// counter LOWER than the stale one it replaces.
func TestSettlementGateRefusesTheDocumentSettledBeforeARedlineWasMerged(t *testing.T) {
	negotiated := documentDigest(t, `{"dcs:clause":"the supplier is paid 10000 EUR"}`)
	redlined := documentDigest(t, `{"dcs:clause":"the supplier is paid 15000 EUR"}`)
	require.NotEqual(t, negotiated, redlined)

	// This instance merged the redline and settled the document it produced;
	// the counterparty's held settlement still names the document from before.
	store := settlementStore{rows: map[string]*dcsdb.Settlement{
		gateLocalPeer: settlementOf(gateLocalPeer, redlined, 4),
		gatePeer:      settlementOf(gatePeer, negotiated, 9),
	}}

	err := runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer})
	require.ErrorIs(t, err, ErrCounterpartyNotSettled)
	require.Contains(t, err.Error(), negotiated)
	require.Contains(t, err.Error(), redlined)

	// The wait ends when the counterparty settles the merged document — and on
	// nothing else, its own counter still reading lower than ours.
	store.rows[gatePeer] = settlementOf(gatePeer, redlined, 9)
	require.NoError(t, runGate(t, store, bothParties(), []string{gateLocalPeer, gatePeer}))
}

// A gate nothing calls refuses nothing. That is not hypothetical here: the
// negotiation guard this wave also replaces (#38) shipped as a no-op precisely
// because its own unit test passed while the flow never consulted it. So pin
// that both signing entry points reach this one — Prepare, which produces the
// to-be-signed document, and SubmitSignature, which records the signature.
func TestBothSigningEntryPointsReachTheSettlementGate(t *testing.T) {
	graph := packageCallGraph(t)

	for _, entry := range []string{"Prepare", "SubmitSignature"} {
		if !reaches(graph, entry, "assertCounterpartiesSettled") {
			t.Errorf("%s does not reach assertCounterpartiesSettled: an instance can sign a version the counterparty never agreed to", entry)
		}
		// The settlement gate is an ADDITIONAL condition (plan WP4). A refactor
		// that swapped one for the other would trade a peer-evidence check for a
		// local-state check, or the reverse — both regressions, neither visible
		// in a test that only asserts the new gate.
		if !reaches(graph, entry, "ValidateTransition") {
			t.Errorf("%s no longer validates the state transition: the settlement gate replaced it instead of adding to it", entry)
		}
	}
}

// Where the gate sits inside prepare: a refused signature must leave no trace,
// so the check runs before the first write. prepare seals the offer into an
// agreement and persists it — a contract the counterparty has not settled must
// not come back sealed.
func TestPrepareRefusesAnUnsettledCounterpartyBeforeItWritesAnything(t *testing.T) {
	body := functionBody(t, "apply.go", "prepare")

	gate := lastCallPos(body, "assertCounterpartiesSettled")
	if gate == token.NoPos {
		t.Fatal("prepare no longer calls assertCounterpartiesSettled: local APPROVED is sufficient to sign again (#40)")
	}
	for name, write := range map[string]token.Pos{
		"UpdateContractData": lastCallPos(body, "UpdateContractData"),
		"RecordSummaryVC":    lastCallPos(body, "RecordSummaryVC"),
	} {
		if write == token.NoPos {
			t.Fatalf("prepare no longer calls %s: this test no longer checks what it claims", name)
		}
		if gate > write {
			t.Errorf("the settlement gate runs after %s: a refused prepare has already written", name)
		}
	}
}

// And inside SubmitSignature, which is not a repeat of prepare's answer: the
// window between prepare and submit is long enough for the counterparty to
// reopen the negotiation and settle a different document, leaving the pinned
// bytes covering a version nobody agrees to any more. A signature binds the
// moment it is made, so the re-check has to precede the writes that record it.
func TestSubmitSignatureRechecksTheSettlementBeforeRecordingTheSignature(t *testing.T) {
	body := functionBody(t, "apply.go", "SubmitSignature")

	gate := lastCallPos(body, "assertCounterpartiesSettled")
	if gate == token.NoPos {
		t.Fatal("SubmitSignature no longer calls assertCounterpartiesSettled: a signature can be recorded for a version the counterparty never agreed to")
	}
	for name, write := range map[string]token.Pos{
		"MarkCeremonyConsumed": lastCallPos(body, "MarkCeremonyConsumed"),
		"finalize":             lastCallPos(body, "finalize"),
	} {
		if write == token.NoPos {
			t.Fatalf("SubmitSignature no longer calls %s: this test no longer checks what it claims", name)
		}
		if gate > write {
			t.Errorf("the settlement gate runs after %s: the signature is recorded before the counterparty's agreement is checked", name)
		}
	}
}
