package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/contractworkflowengine/db"
	db2 "digital-contracting-service/internal/dcstodcs/db"
	"digital-contracting-service/internal/pdfgeneration/provenance"

	"github.com/jmoiron/sqlx"
)

// ErrAgreementSettled refuses an edit to the contract document of a copy whose
// agreement is already settled: a party signed it, and the stored artifact is
// that party's PAdES-signed bytes.
//
// Settlement is not intrinsic state. The receive path deliberately keeps the
// peer's workflow out of this instance's own (only a revocation overrides it,
// DCS-NFR-BR-06), so a copy can sit in OFFERED while the document it holds is
// signed — and the transition table, which knows only the local state, still
// offers the counterparty its designated way out of an offer. Editing the
// document there produces a copy whose contract_data no longer matches the
// document embedded in its own PDF, which cannot be re-rendered without
// destroying the peer's signature: every later ship is refused by the peer
// ("JAdES payload does not match the contract document embedded in the shipped
// PDF"), and the local signature can never reach it.
var ErrAgreementSettled = errors.New("this agreement is settled; a signed contract cannot be renegotiated")

// ErrOwnAgreementSettled refuses the same edit one step EARLIER: this instance
// stated, in a signed settlement artifact shipped to every counterparty, that
// it agrees to the document exactly as it stands. Nothing is signed yet, so it
// is a different answer from ErrAgreementSettled and says so — the party is
// being held to its own word, not to somebody's signature.
//
// The way out is the rejection the workflow already has: a reviewer rejecting a
// submission (SUBMITTED -> NEGOTIATION) or an approver rejecting the contract
// (REVIEWED -> REJECTED) reopens the round and withdraws this instance's
// settlement (withdrawOwnSettlement), after which the document is editable
// again and a later submit settles the new version.
var ErrOwnAgreementSettled = errors.New("this instance already agreed to this exact version of the contract; reopen the negotiation round by rejecting the submission before changing the document")

// OwnSettlements is the slice of the DCS-to-DCS settlement store the workflow
// engine needs: the settlement rows keyed by the settling party, read for this
// instance's own did:web and deleted when it withdraws its agreement. The peer
// rows in the same table belong to the signing gate
// (signingmanagement/command/apply.go) and are never touched from here.
type OwnSettlements interface {
	GetSettlement(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) (*db2.Settlement, error)
	GetSettlementsBy(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) ([]db2.Settlement, error)
	DeleteSettlementsBy(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) error
	UpsertSettlementWithdrawal(ctx context.Context, tx *sqlx.Tx, withdrawal db2.SettlementWithdrawal) error
}

// requireUnsettledAgreement refuses a caller that is about to persist a NEW
// contract document for did when this instance is already committed to the one
// it holds — either because the stored artifact carries a signature, or because
// this instance settled precisely that document. It gates the document
// changing, not the command: local RBAC progress (review, approval), free-text
// negotiation notes and the signing path itself leave contract_data's document
// in step with the artifact and are not its business.
//
// The two conditions cover consecutive windows of the same lifecycle and are
// both needed. The settlement covers "both parties agreed, nobody has signed
// yet" — the window this gate exists for, in which the artifact is still an
// unsigned draft render and says nothing. The artifact covers everything from
// the first signature on, including a copy that never settled locally at all:
// the peer's signed PDF arrives on a copy still sitting in OFFERED, whose own
// settlement row does not exist (ADR-13: federation state is derivable from
// artifacts alone).
//
// The settlement is compared BY DIGEST against the document as stored, not
// merely looked up. A settlement names one version (jades.ContractDocumentDigest
// over the JCS canonicalization); once the document this instance holds is a
// different one — the counterparty redlined and shipped its own, or the first
// signature sealed dcs:policies from odrl:Offer into odrl:Agreement — the
// settlement no longer describes anything this copy could be held to, and
// refusing on its mere existence would wedge a contract whose counterparty is
// still negotiating.
func requireUnsettledAgreement(
	ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo,
	settlements OwnSettlements, localPeer, did string,
) error {
	pdfState, err := cRepo.ReadPDFState(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read the stored artifact's lifecycle state for %s: %w", did, err)
	}
	if provenance.ArtifactCarriesSignature(pdfState.C2PAState) {
		return ErrAgreementSettled
	}

	settled, err := ownSettlementCoversStoredDocument(ctx, tx, cRepo, settlements, localPeer, did)
	if err != nil {
		return err
	}
	if settled {
		return ErrOwnAgreementSettled
	}
	return nil
}

// ownSettlementCoversStoredDocument reports whether this instance holds its own
// settlement of did naming the document currently stored for it.
func ownSettlementCoversStoredDocument(
	ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo,
	settlements OwnSettlements, localPeer, did string,
) (bool, error) {
	if settlements == nil {
		return false, fmt.Errorf("could not check this instance's settlement of %s: no settlement store is configured", did)
	}
	own, err := settlements.GetSettlement(ctx, tx, did, localPeer)
	if err != nil {
		return false, fmt.Errorf("could not read this instance's settlement of %s: %w", did, err)
	}
	if own == nil {
		return false, nil
	}

	contract, err := cRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return false, fmt.Errorf("could not read the contract document of %s: %w", did, err)
	}
	var document []byte
	if contract.ContractData != nil && contract.ContractData.IsNotNullValue() {
		document = []byte(*contract.ContractData)
	}
	// Same digest the settlement artifact was built over (dcstodcs.BuildSettlement),
	// so an unchanged document compares equal across the jsonb round trip.
	digest, err := jades.ContractDocumentDigest(document)
	if err != nil {
		return false, fmt.Errorf("could not digest the contract document of %s: %w", did, err)
	}
	return digest == own.DocumentDigest, nil
}

// withdrawOwnSettlement drops the settlements this instance produced for did,
// which is what makes its document editable again after requireUnsettledAgreement
// refused. It belongs on exactly the transitions that undo the one that settled:
// NEGOTIATION -> SUBMITTED records the settlement, so a reviewer sending the
// submission back (SUBMITTED -> NEGOTIATION) and an approver rejecting the
// contract (REVIEWED -> REJECTED) — the two edges that reopen the negotiation
// tasks — take it back.
//
// Deleting rather than superseding keeps the signing gate honest in the
// meantime: with no own settlement, assertCounterpartiesSettled refuses to sign
// with "this instance has not settled", which is the truth between the
// withdrawal and the next submit. The next settle writes a fresh, undelivered
// artifact for the new version and the retry scheduler ships it.
//
// Deleting locally only makes THIS instance stop signing. The counterparty
// holds the settlement too, as the evidence ITS gate reads, so every row
// dropped here queues a withdrawal toward the audience it was shipped to
// (dcstodcs ships and retries it). The withdrawal names the digest the deleted
// settlement covered, so it takes back exactly the agreement that was given and
// cannot land on a later one.
func withdrawOwnSettlement(ctx context.Context, tx *sqlx.Tx, settlements OwnSettlements, localPeer, did string) error {
	if settlements == nil {
		return fmt.Errorf("could not withdraw this instance's settlement of %s: no settlement store is configured", did)
	}
	given, err := settlements.GetSettlementsBy(ctx, tx, did, localPeer)
	if err != nil {
		return fmt.Errorf("could not read this instance's settlements of %s: %w", did, err)
	}
	// Truncated as BuildSettlement truncates settled_at, so the timestamp the
	// artifact is signed over is the one this row keeps across re-deliveries.
	withdrawnAt := time.Now().UTC().Truncate(time.Microsecond)
	for _, settlement := range given {
		if err := settlements.UpsertSettlementWithdrawal(ctx, tx, db2.SettlementWithdrawal{
			DID:            did,
			FromPeerDID:    localPeer,
			ToPeerDID:      settlement.ToPeerDID,
			DocumentDigest: settlement.DocumentDigest,
			WithdrawnAt:    withdrawnAt,
		}); err != nil {
			return fmt.Errorf("could not queue the withdrawal of this instance's settlement of %s toward %s: %w",
				did, settlement.ToPeerDID, err)
		}
	}
	if err := settlements.DeleteSettlementsBy(ctx, tx, did, localPeer); err != nil {
		return fmt.Errorf("could not withdraw this instance's settlement of %s: %w", did, err)
	}
	return nil
}
