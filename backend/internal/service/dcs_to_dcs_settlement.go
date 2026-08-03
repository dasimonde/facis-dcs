package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	trustgate "digital-contracting-service/internal/dcstodcs"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

// settlementClockSkew is how far into the future a peer's claimed settlement
// time may sit before the artifact is refused: enough for two instances whose
// clocks are merely not synchronized, not enough to pre-date a settlement of a
// document that does not exist yet. A settlement never expires — it is voided
// by the document changing, not by time.
const settlementClockSkew = 5 * time.Minute

// PostSettlement receives the counterparty's evidence that it reached its own
// settled state on a named version of this contract. Signing claims both
// parties agreed the same document, and ADR-13 keeps intrinsic state local, so
// the peer's agreement is knowable here only as an artifact it signed and
// shipped — held locally, re-verifiable, and bound to the document this
// instance itself holds. Every refusal names the check that failed.
func (s *dcsToDcssrvc) PostSettlement(ctx context.Context, req *dcstodcs.DCSToDCSContractSettlementRequest) (res *dcstodcs.DCSToDCSContractSettlementResponse, err error) {
	remoteDIDDocument, err := fetchPeerDIDDocument(req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := remoteDIDDocument.VerifyPeerChallenge(s.TrustPool, []byte(req.SecretValue), req.SecretHash); err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_settlement rejected: peer %s did not authenticate: %w", req.FromPeerDid, err))
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if identity.SameDIDWeb(req.FromPeerDid, localPeer) {
		return nil, contractworkflowengine.MakeBadRequest(
			errors.New("post_settlement rejected: a settlement shipped by this instance to itself is no counterparty evidence"))
	}

	// Federation trust gate (ADR-19), exactly as the PDF ship applies it: a
	// peer this instance may not federate with cannot deposit evidence here.
	if err := s.TrustGate.Check(ctx, req.FromPeerDid, trustgate.Inbound, req.ContractIri, ""); err != nil {
		var gateErr *trustgate.GateError
		if errors.As(err, &gateErr) {
			if incidentErr := trustgate.RecordDenialIncident(ctx, s.DB, req.ContractIri, trustgate.Inbound, gateErr); incidentErr != nil {
				log.Printf(ctx, "could not record trust gate denial incident for %s: %v", req.ContractIri, incidentErr)
			}
		}
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_settlement rejected: peer %s does not pass the federation trust gate: %w", req.FromPeerDid, err))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	contract, err := s.CRepo.ReadDataByDID(ctx, tx, req.ContractIri)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_settlement rejected: this instance holds no copy of contract %s: %w", req.ContractIri, err))
	}
	documentDigest, err := jades.ContractDocumentDigest(contractDocumentOf(contract))
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	var parties []string
	if contract.Responsible != nil {
		parties = contract.Responsible.GetParties()
	}
	previous, err := s.SRepo.GetSettlement(ctx, tx, req.ContractIri, req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	superseded, err := s.supersededDocumentDigests(ctx, tx, req.ContractIri, localPeer)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	settlement, err := verifyShippedSettlement(req.SettlementJades, req.FromPeerDid, remoteDIDDocument, localSettlementContext{
		ContractIRI:      req.ContractIri,
		LocalPeer:        localPeer,
		DocumentDigest:   documentDigest,
		SupersededDigest: superseded,
		Parties:          parties,
		Previous:         previous,
		Now:              time.Now().UTC(),
	})
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("post_settlement rejected: %w", err))
	}

	if err := s.SRepo.UpsertSettlement(ctx, tx, *settlement); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSContractSettlementResponse{FromPeerDid: localPeer}, nil
}

func contractDocumentOf(contract *cwedb.Contract) []byte {
	if contract.ContractData != nil && contract.ContractData.IsNotNullValue() {
		return []byte(*contract.ContractData)
	}
	return []byte(`{}`)
}

// supersededDocumentDigests lists the versions of a contract document this
// instance HELD and no longer holds: the version its own settlement names, and
// every version snapshotted to contract_history.
//
// They are accepted alongside the current document because delivery is not
// instantaneous while the document is. A settlement is verified against the
// receiver at ARRIVAL, and the retry scheduler re-ships an artifact unchanged,
// so a settlement whose first delivery was lost arrives against a document that
// has since moved — the first signature seals odrl:Offer into odrl:Agreement and
// both instances persist the result — and would be refused for good, leaving
// the peer unable ever to prove it settled and this instance unable ever to
// countersign.
//
// The own settlement carries most of the weight here: the receive path replaces
// the stored document with the peer's shipped copy without snapshotting the one
// it replaces, so a version this instance agreed to and was then shipped past
// leaves no other trace. It is also exactly the version the signing gate
// compares against, so a settlement matching it names the document both sides
// stood behind.
//
// Everything outside the set is still refused. A settlement of a document
// neither side ever held authorizes nothing, and one naming a superseded
// version does not let a signature through either: assertCounterpartiesSettled
// compares the peer's digest against THIS instance's own settled digest, so a
// settlement of a version this instance has moved off is stored and simply
// never satisfies the gate.
func (s *dcsToDcssrvc) supersededDocumentDigests(ctx context.Context, tx *sqlx.Tx, did, localPeer string) ([]string, error) {
	var superseded []string

	own, err := s.SRepo.GetSettlement(ctx, tx, did, localPeer)
	if err != nil {
		return nil, err
	}
	if own != nil {
		superseded = append(superseded, own.DocumentDigest)
	}

	history, err := s.CRepo.ReadHistoryByDID(ctx, tx, did)
	if err != nil {
		return nil, err
	}
	for _, entry := range history {
		document := []byte(`{}`)
		if entry.ContractData != nil && entry.ContractData.IsNotNullValue() {
			document = []byte(*entry.ContractData)
		}
		digest, err := jades.ContractDocumentDigest(document)
		if err != nil {
			return nil, err
		}
		superseded = append(superseded, digest)
	}
	return superseded, nil
}

// localSettlementContext is what this instance itself knows about the contract
// a peer claims to have settled — the ground truth every claim is checked
// against, never the shipped artifact's own account of it.
type localSettlementContext struct {
	ContractIRI    string
	LocalPeer      string
	DocumentDigest string
	// SupersededDigest are the document versions this instance held before the
	// one it holds now (supersededDocumentDigests).
	SupersededDigest []string
	Parties          []string
	Previous         *db2.Settlement
	Now              time.Time
}

// verifyShippedSettlement verifies a peer's settlement artifact and returns
// the record to store. It refuses — naming the failing check — a JAdES that
// does not verify, one signed by a key the peer does not publish for
// assertions, an artifact that is not a settlement, that names another
// contract, another audience or a non-party as settler, that does not
// re-canonicalize to the bytes that were signed, that settles a document other
// than the one this instance holds, or that is timestamped implausibly far
// ahead or behind a settlement already held from that peer.
func verifyShippedSettlement(jadesSignature, fromPeerDID string, remoteDIDDocument *identity.DIDDocument, local localSettlementContext) (*db2.Settlement, error) {
	payload, leafKey, err := jades.Verify(jadesSignature)
	if err != nil {
		return nil, fmt.Errorf("settlement JAdES does not verify: %w", err)
	}
	if !remoteDIDDocument.PublishesKeyFor(identity.PurposeAssertion, leafKey) {
		return nil, fmt.Errorf("settlement JAdES x5c leaf key is not published by peer %s as an %s key",
			fromPeerDID, identity.PurposeAssertion)
	}

	var claimed struct {
		Type            string `json:"@type"`
		ContractDID     string `json:"dcs:contractDid"`
		ContractVersion int    `json:"dcs:contractVersion"`
		DocumentDigest  string `json:"dcs:contractDocumentDigest"`
		SettledBy       string `json:"dcs:settledBy"`
		SettledWith     string `json:"dcs:settledWith"`
		SettledAt       string `json:"dcs:settledAt"`
	}
	if err := json.Unmarshal(payload, &claimed); err != nil {
		return nil, fmt.Errorf("could not decode the settlement payload: %w", err)
	}
	if claimed.Type != jades.SettlementType {
		return nil, fmt.Errorf("signed payload is a %q, not a %s", claimed.Type, jades.SettlementType)
	}
	for _, required := range []struct{ field, value string }{
		{"dcs:contractDid", claimed.ContractDID},
		{"dcs:contractDocumentDigest", claimed.DocumentDigest},
		{"dcs:settledBy", claimed.SettledBy},
		{"dcs:settledWith", claimed.SettledWith},
		{"dcs:settledAt", claimed.SettledAt},
	} {
		if strings.TrimSpace(required.value) == "" {
			return nil, fmt.Errorf("settlement is missing %s", required.field)
		}
	}

	settledAt, err := time.Parse(time.RFC3339Nano, claimed.SettledAt)
	if err != nil {
		return nil, fmt.Errorf("settlement dcs:settledAt %q is not an RFC3339 timestamp: %w", claimed.SettledAt, err)
	}

	// Re-derive the signed bytes from the claimed fields instead of trusting the
	// sender's serialization: anything the canonical form does not carry — an
	// extra property, a differently written timestamp, a foreign @context —
	// makes the artifact something other than the settlement it reads as.
	expected, err := jades.BuildSettlementPayload(jades.Settlement{
		ContractDID:     claimed.ContractDID,
		ContractVersion: claimed.ContractVersion,
		DocumentDigest:  claimed.DocumentDigest,
		SettledBy:       claimed.SettledBy,
		SettledWith:     claimed.SettledWith,
		SettledAt:       settledAt,
	})
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(payload, expected) {
		return nil, errors.New("settlement payload is not the canonical form of the fields it claims")
	}

	if claimed.ContractDID != local.ContractIRI {
		return nil, fmt.Errorf("settlement binds contract %s, not the shipped contract %s", claimed.ContractDID, local.ContractIRI)
	}
	if !identity.SameDIDWeb(claimed.SettledBy, fromPeerDID) {
		return nil, fmt.Errorf("settlement was made by %s but shipped by %s", claimed.SettledBy, fromPeerDID)
	}
	if !identity.SameDIDWeb(claimed.SettledWith, local.LocalPeer) {
		return nil, fmt.Errorf("settlement was made toward %s, not this instance %s", claimed.SettledWith, local.LocalPeer)
	}
	isParty := false
	for _, party := range local.Parties {
		if identity.SameDIDWeb(party, claimed.SettledBy) {
			isParty = true
			break
		}
	}
	if !isParty {
		return nil, fmt.Errorf("peer %s is not a party of contract %s", claimed.SettledBy, local.ContractIRI)
	}

	// The binding that matters. contract_version is a per-instance counter —
	// the receiver bumps it on every inbound ship, the sender only on merging a
	// redline — so the versions are not comparable across instances and the
	// digest of the document itself is what says "the same version". A
	// settlement of a document this instance never held authorizes nothing.
	//
	// Any version this instance HAS held is accepted, not only the one it holds
	// at arrival: see supersededDocumentDigests for why an in-flight settlement
	// outlives the document it names, and why accepting it does not let a stale
	// one through the signing gate.
	if claimed.DocumentDigest != local.DocumentDigest && !slices.Contains(local.SupersededDigest, claimed.DocumentDigest) {
		return nil, fmt.Errorf("settlement covers document %s but this instance holds %s for contract %s and has held no version matching it",
			claimed.DocumentDigest, local.DocumentDigest, local.ContractIRI)
	}

	if settledAt.After(local.Now.Add(settlementClockSkew)) {
		return nil, fmt.Errorf("settlement is dated %s, further ahead than the tolerated clock skew", settledAt.Format(time.RFC3339Nano))
	}
	if local.Previous != nil && settledAt.Before(local.Previous.SettledAt) {
		return nil, fmt.Errorf("settlement is dated %s, older than the settlement already held from %s (%s)",
			settledAt.Format(time.RFC3339Nano), fromPeerDID, local.Previous.SettledAt.Format(time.RFC3339Nano))
	}

	received := local.Now
	return &db2.Settlement{
		DID:             local.ContractIRI,
		FromPeerDID:     fromPeerDID,
		ToPeerDID:       local.LocalPeer,
		ContractVersion: claimed.ContractVersion,
		DocumentDigest:  claimed.DocumentDigest,
		SettledAt:       settledAt,
		JadesSignature:  jadesSignature,
		// Evidence in the hands of its audience: the delivery bookkeeping the
		// outbound queue uses is closed for an artifact that has arrived.
		DeliveredAt: &received,
	}, nil
}

// PostSettlementWithdrawal receives the counterparty taking back a settlement
// it shipped earlier. Its own gate stopped it signing the moment it dropped its
// settlement locally; this is what stops THIS instance signing a version the
// peer no longer agrees to, which the mutual guarantee otherwise only holds
// until the peer refuses the signature that comes back.
//
// Authentication, self-check and federation trust gate are post_settlement's,
// because a withdrawal is the same statement in reverse and must be no easier
// to inject than the settlement it revokes.
func (s *dcsToDcssrvc) PostSettlementWithdrawal(ctx context.Context, req *dcstodcs.DCSToDCSSettlementWithdrawalRequest) (res *dcstodcs.DCSToDCSSettlementWithdrawalResponse, err error) {
	remoteDIDDocument, err := fetchPeerDIDDocument(req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := remoteDIDDocument.VerifyPeerChallenge(s.TrustPool, []byte(req.SecretValue), req.SecretHash); err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_settlement_withdrawal rejected: peer %s did not authenticate: %w", req.FromPeerDid, err))
	}

	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if identity.SameDIDWeb(req.FromPeerDid, localPeer) {
		return nil, contractworkflowengine.MakeBadRequest(
			errors.New("post_settlement_withdrawal rejected: this instance cannot withdraw its own settlement through the peer channel"))
	}

	if err := s.TrustGate.Check(ctx, req.FromPeerDid, trustgate.Inbound, req.ContractIri, ""); err != nil {
		var gateErr *trustgate.GateError
		if errors.As(err, &gateErr) {
			if incidentErr := trustgate.RecordDenialIncident(ctx, s.DB, req.ContractIri, trustgate.Inbound, gateErr); incidentErr != nil {
				log.Printf(ctx, "could not record trust gate denial incident for %s: %v", req.ContractIri, incidentErr)
			}
		}
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_settlement_withdrawal rejected: peer %s does not pass the federation trust gate: %w", req.FromPeerDid, err))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	contract, err := s.CRepo.ReadDataByDID(ctx, tx, req.ContractIri)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_settlement_withdrawal rejected: this instance holds no copy of contract %s: %w", req.ContractIri, err))
	}
	var parties []string
	if contract.Responsible != nil {
		parties = contract.Responsible.GetParties()
	}
	held, err := s.SRepo.GetSettlement(ctx, tx, req.ContractIri, req.FromPeerDid)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	withdrawal, err := verifyShippedWithdrawal(req.WithdrawalJades, req.FromPeerDid, remoteDIDDocument, localWithdrawalContext{
		ContractIRI: req.ContractIri,
		LocalPeer:   localPeer,
		Parties:     parties,
		Now:         time.Now().UTC(),
	})
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("post_settlement_withdrawal rejected: %w", err))
	}

	applies, reason := withdrawalTakesBack(*withdrawal, held)
	if !applies {
		log.Printf(ctx, "settlement withdrawal from %s for %s removed nothing: %s", req.FromPeerDid, req.ContractIri, reason)
		return &dcstodcs.DCSToDCSSettlementWithdrawalResponse{FromPeerDid: localPeer, Withdrawn: false}, nil
	}

	if err := s.SRepo.DeleteSettlementsBy(ctx, tx, req.ContractIri, req.FromPeerDid); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	return &dcstodcs.DCSToDCSSettlementWithdrawalResponse{FromPeerDid: localPeer, Withdrawn: true}, nil
}

// localWithdrawalContext is what this instance knows about the contract whose
// settlement a peer is taking back. It carries no document digest: a withdrawal
// is a statement about a SETTLEMENT, and the settlement it names is the one
// held from that peer, whatever document the contract has moved on to.
type localWithdrawalContext struct {
	ContractIRI string
	LocalPeer   string
	Parties     []string
	Now         time.Time
}

// verifyShippedWithdrawal verifies a peer's settlement-withdrawal artifact.
// It refuses — naming the failing check — a JAdES that does not verify, one
// signed by a key the peer does not publish for assertions, an artifact that is
// not a withdrawal, that names another contract, another audience or a
// non-party as withdrawing, that does not re-canonicalize to the bytes that
// were signed, or that is dated implausibly far ahead.
//
// Whether the withdrawal actually removes anything is decided separately
// (withdrawalTakesBack): a well-formed withdrawal that matches no held
// settlement is not an error, it is a delivery that has nothing left to do.
func verifyShippedWithdrawal(jadesSignature, fromPeerDID string, remoteDIDDocument *identity.DIDDocument, local localWithdrawalContext) (*jades.SettlementWithdrawal, error) {
	payload, leafKey, err := jades.Verify(jadesSignature)
	if err != nil {
		return nil, fmt.Errorf("settlement withdrawal JAdES does not verify: %w", err)
	}
	if !remoteDIDDocument.PublishesKeyFor(identity.PurposeAssertion, leafKey) {
		return nil, fmt.Errorf("settlement withdrawal JAdES x5c leaf key is not published by peer %s as an %s key",
			fromPeerDID, identity.PurposeAssertion)
	}

	var claimed struct {
		Type           string `json:"@type"`
		ContractDID    string `json:"dcs:contractDid"`
		DocumentDigest string `json:"dcs:contractDocumentDigest"`
		WithdrawnBy    string `json:"dcs:withdrawnBy"`
		WithdrawnFrom  string `json:"dcs:withdrawnFrom"`
		WithdrawnAt    string `json:"dcs:withdrawnAt"`
	}
	if err := json.Unmarshal(payload, &claimed); err != nil {
		return nil, fmt.Errorf("could not decode the settlement withdrawal payload: %w", err)
	}
	if claimed.Type != jades.SettlementWithdrawalType {
		return nil, fmt.Errorf("signed payload is a %q, not a %s", claimed.Type, jades.SettlementWithdrawalType)
	}
	for _, required := range []struct{ field, value string }{
		{"dcs:contractDid", claimed.ContractDID},
		{"dcs:contractDocumentDigest", claimed.DocumentDigest},
		{"dcs:withdrawnBy", claimed.WithdrawnBy},
		{"dcs:withdrawnFrom", claimed.WithdrawnFrom},
		{"dcs:withdrawnAt", claimed.WithdrawnAt},
	} {
		if strings.TrimSpace(required.value) == "" {
			return nil, fmt.Errorf("settlement withdrawal is missing %s", required.field)
		}
	}

	withdrawnAt, err := time.Parse(time.RFC3339Nano, claimed.WithdrawnAt)
	if err != nil {
		return nil, fmt.Errorf("settlement withdrawal dcs:withdrawnAt %q is not an RFC3339 timestamp: %w", claimed.WithdrawnAt, err)
	}

	withdrawal := jades.SettlementWithdrawal{
		ContractDID:    claimed.ContractDID,
		DocumentDigest: claimed.DocumentDigest,
		WithdrawnBy:    claimed.WithdrawnBy,
		WithdrawnFrom:  claimed.WithdrawnFrom,
		WithdrawnAt:    withdrawnAt,
	}
	expected, err := jades.BuildSettlementWithdrawalPayload(withdrawal)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(payload, expected) {
		return nil, errors.New("settlement withdrawal payload is not the canonical form of the fields it claims")
	}

	if claimed.ContractDID != local.ContractIRI {
		return nil, fmt.Errorf("settlement withdrawal binds contract %s, not the shipped contract %s", claimed.ContractDID, local.ContractIRI)
	}
	if !identity.SameDIDWeb(claimed.WithdrawnBy, fromPeerDID) {
		return nil, fmt.Errorf("settlement withdrawal was made by %s but shipped by %s", claimed.WithdrawnBy, fromPeerDID)
	}
	if !identity.SameDIDWeb(claimed.WithdrawnFrom, local.LocalPeer) {
		return nil, fmt.Errorf("settlement withdrawal was made toward %s, not this instance %s", claimed.WithdrawnFrom, local.LocalPeer)
	}
	isParty := false
	for _, party := range local.Parties {
		if identity.SameDIDWeb(party, claimed.WithdrawnBy) {
			isParty = true
			break
		}
	}
	if !isParty {
		return nil, fmt.Errorf("peer %s is not a party of contract %s", claimed.WithdrawnBy, local.ContractIRI)
	}

	if withdrawnAt.After(local.Now.Add(settlementClockSkew)) {
		return nil, fmt.Errorf("settlement withdrawal is dated %s, further ahead than the tolerated clock skew",
			withdrawnAt.Format(time.RFC3339Nano))
	}

	return &withdrawal, nil
}

// withdrawalTakesBack reports whether a verified withdrawal removes the
// settlement currently held from that peer, and why it does not when it does
// not. Both refusals exist so a withdrawal cannot be replayed into a later
// round to delete an agreement that was given after it:
//
//   - the digest must be the one the held settlement covers. A withdrawal names
//     the version it takes back, so one held back and re-delivered after the
//     peer settled a new version matches nothing.
//   - it must not predate the settlement it would remove. That closes the case
//     the digest alone cannot: a peer that rejects and then re-settles the very
//     same document produces a settlement with the same digest and a later
//     settled_at, and the old withdrawal must not reach it.
//
// Neither is an error. The withdrawal was authentic and correctly addressed;
// there is simply nothing of that peer's left to take back, and answering
// "delivered" is what stops the sender retrying forever.
//
// Note what is NOT here: a settlement is never dropped because the document
// moved. The first signature seals odrl:Offer into odrl:Agreement, so on the
// receive path a digest mismatch is the NORMAL state of a counterparty that has
// signed — deleting on it would destroy the very evidence needed to countersign.
// A settlement goes only when the party that made it says so.
func withdrawalTakesBack(withdrawal jades.SettlementWithdrawal, held *db2.Settlement) (bool, string) {
	if held == nil {
		return false, "no settlement from that peer is held"
	}
	if held.DocumentDigest != withdrawal.DocumentDigest {
		return false, fmt.Sprintf("it withdraws the settlement of document %s while the settlement held covers %s",
			withdrawal.DocumentDigest, held.DocumentDigest)
	}
	if withdrawal.WithdrawnAt.Before(held.SettledAt) {
		return false, fmt.Sprintf("it is dated %s, before the settlement it would remove (%s)",
			withdrawal.WithdrawnAt.Format(time.RFC3339Nano), held.SettledAt.Format(time.RFC3339Nano))
	}
	return true, ""
}
