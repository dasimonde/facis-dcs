package command

import (
	"context"
	"errors"
	"fmt"

	"digital-contracting-service/internal/contractworkflowengine/db"
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

// requireUnsettledAgreement refuses a caller that is about to persist a NEW
// contract document for did when the stored artifact already carries a
// signature. It gates the document changing, not the command: local RBAC
// progress (review, approval), free-text negotiation notes and the signing path
// itself leave contract_data's document in step with the artifact and are not
// its business.
//
// The artifact is the authoritative signal (ADR-13: federation state is
// derivable from artifacts alone). The ODRL seal in the document — dcs:policies
// retyped from odrl:Offer to odrl:Agreement at the first signature — travels on
// the same ship, but it only exists for a contract whose policy set is a node
// to retype, so a policy-free contract would settle without it.
func requireUnsettledAgreement(ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo, did string) error {
	pdfState, err := cRepo.ReadPDFState(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read the stored artifact's lifecycle state for %s: %w", did, err)
	}
	if provenance.ArtifactCarriesSignature(pdfState.C2PAState) {
		return ErrAgreementSettled
	}
	return nil
}
