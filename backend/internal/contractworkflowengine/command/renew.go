package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

// ErrContractNotRenewable is returned when the original contract is not in a
// state from which a renewal makes sense (DCS-FR-CWE-11/22). Mapped to a
// client error by service.mapContractCommandError.
var ErrContractNotRenewable = errors.New("contract is not in a renewable state")

// renewableStates are the original-contract states from which a renewal
// instance may be created. A contract must at least have been agreed
// (APPROVED) or be in its performance/archival lifecycle (SIGNED, ACTIVE,
// TERMINATED, EXPIRED) — renewing a contract that never made it out of
// negotiation (DRAFT/OFFERED/NEGOTIATION/SUBMITTED/REVIEWED/REJECTED/
// WITHDRAWN) has no meaning.
var renewableStates = map[contractstate.ContractState]bool{
	contractstate.Approved:   true,
	contractstate.Signed:     true,
	contractstate.Active:     true,
	contractstate.Terminated: true,
	contractstate.Expired:    true,
}

// RenewCmd carries a new-instance renewal request (DCS-FR-CWE-11/22,
// DCS-FR-CSA-15). DID is the new renewal contract's DID, minted by the
// service layer before Handle is invoked (mirrors CreateCmd). OriginalDID is
// the contract being renewed; the original is read-only for this command and
// is never mutated.
type RenewCmd struct {
	DID                string             `json:"did"`
	OriginalDID        string             `json:"original_did"`
	RenewedBy          string             `json:"renewed_by"`
	HolderDID          string             `json:"holder_did"`
	UserRoles          userrole.UserRoles `json:"user_roles"`
	UpdatedAt          time.Time          `json:"updated_at"`
	NewStartDate       *time.Time         `json:"new_start_date"`
	NewExpDate         *time.Time         `json:"new_exp_date"`
	NewExpPolicy       *string            `json:"new_exp_policy"`
	NewExpNoticePeriod *int               `json:"new_exp_notice_period"`
}

// RenewResult reports back what the command derived from the original
// contract so the HTTP layer can echo it without a second read.
type RenewResult struct {
	OriginalContractVersion int
}

type Renewer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NTRepo      db.NegotiationTaskRepo
	DIDDocument identity.DIDDocument
}

// Handle creates a brand-new contract instance (its own single-writer
// aggregate, like Creator.Handle) that references the original via a
// dcs:renewsContract JSON-LD link. The original contract is only read here,
// never written — there is no cross-peer forwarding concern for the
// original, unlike Terminator.Handle, because renewal never mutates it.
func (h *Renewer) Handle(ctx context.Context, cmd RenewCmd) (*RenewResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	original, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.OriginalDID)
	if err != nil {
		return nil, fmt.Errorf("could not read original contract data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < original.UpdatedAt.Unix() {
		return nil, errors.New("contract was updated elsewhere, please reload")
	}

	originalState, err := contractstate.NewContractState(original.State)
	if err != nil {
		return nil, fmt.Errorf("could not parse original contract state: %w", err)
	}
	if !renewableStates[originalState] {
		return nil, fmt.Errorf("%w: original contract is in state %s", ErrContractNotRenewable, originalState)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return nil, fmt.Errorf("could not get DID: %w", err)
	}

	// Rebase the copied document's internal identity onto the new DID first,
	// then attach the dcs:renewsContract back-reference — attaching it before
	// rebasing would risk the rebase pass rewriting the very reference we
	// want to keep pointed at the original.
	normalizedContractData, err := validation.NormalizeContractDataForPersistence(original.ContractData, cmd.DID, false)
	if err != nil {
		return nil, fmt.Errorf("contract data validation failed: %w", err)
	}
	unsignedContractData, err := unsignRenewedDocument(normalizedContractData)
	if err != nil {
		return nil, fmt.Errorf("could not clear the previous term's signature attribution: %w", err)
	}
	renewalContractData, err := attachRenewsContractReference(unsignedContractData, cmd.OriginalDID, original.ContractVersion)
	if err != nil {
		return nil, fmt.Errorf("could not attach renewal reference: %w", err)
	}

	var resp db.Responsible
	if original.Responsible != nil {
		resp = db.Responsible{
			Creator:      localPeer,
			Reviewers:    original.Responsible.Reviewers,
			Approvers:    original.Responsible.Approvers,
			Negotiators:  original.Responsible.Negotiators,
			Counterparty: original.Responsible.Counterparty,
		}
	} else {
		resp = db.Responsible{Creator: localPeer}
	}

	// Establish the new term's parties and signature fields the same way a plain
	// create does (Creator.Handle), now that the previous term's attribution is
	// gone: the copy already declares the carried-over parties and their fields,
	// so this adds only what the original was missing.
	seeded, changed, err := SeedPartiesAndSignatureFields(*renewalContractData, resp.GetParties())
	if err != nil {
		return nil, fmt.Errorf("could not seed contract parties and signature fields: %w", err)
	}
	if changed {
		renewalContractData = &seeded
	}

	newStartDate := original.StartDate
	if cmd.NewStartDate != nil {
		newStartDate = cmd.NewStartDate
	}
	newExpDate := original.ExpDate
	if cmd.NewExpDate != nil {
		newExpDate = cmd.NewExpDate
	}
	newExpPolicy := original.ExpPolicy
	if cmd.NewExpPolicy != nil {
		newExpPolicy = cmd.NewExpPolicy
	}
	newExpNoticePeriod := original.ExpNoticePeriod
	if cmd.NewExpNoticePeriod != nil {
		newExpNoticePeriod = cmd.NewExpNoticePeriod
	}

	data := db.Contract{
		DID:             cmd.DID,
		Origin:          localPeer,
		CreatedBy:       cmd.RenewedBy,
		State:           contractstate.Draft.String(),
		ContractData:    renewalContractData,
		TemplateDID:     original.TemplateDID,
		TemplateVersion: original.TemplateVersion,
		Name:            original.Name,
		Description:     original.Description,
		Responsible:     &resp,
	}
	if err := h.CRepo.Create(ctx, tx, data); err != nil {
		return nil, fmt.Errorf("could not create renewal contract: %w", err)
	}

	// Term dates/policy are not accepted by Create (mirrors the plain
	// create endpoint, which also has no date fields); carry them over — or
	// apply the caller's override — via a follow-up partial update.
	if newStartDate != nil || newExpDate != nil || newExpPolicy != nil || newExpNoticePeriod != nil {
		update := db.ContractUpdateData{
			DID:             cmd.DID,
			StartDate:       newStartDate,
			ExpDate:         newExpDate,
			ExpPolicy:       newExpPolicy,
			ExpNoticePeriod: newExpNoticePeriod,
		}
		if err := h.CRepo.Update(ctx, tx, update); err != nil {
			return nil, fmt.Errorf("could not persist renewal term dates: %w", err)
		}
	}

	// Reuse Creator's task fan-out (same package) so review/approval/
	// negotiation tasks for the new instance follow the exact same rules as
	// a plain create — automatic metadata carryover (DCS-FR-CWE-22) extends
	// to who is responsible, not just contract fields.
	if err := createReviewAndApprovalTasks(ctx, tx, h.RTRepo, h.ATRepo, cmd.DID, cmd.RenewedBy, resp); err != nil {
		return nil, err
	}
	for _, negotiator := range resp.Negotiators {
		if err := mintNegotiationTask(ctx, tx, h.NTRepo, cmd.DID, negotiator, cmd.RenewedBy, initialContractVersion); err != nil {
			return nil, err
		}
	}

	evt := contractevents.RenewEvent{
		DID:                     cmd.DID,
		HolderDID:               cmd.HolderDID,
		RenewedBy:               cmd.RenewedBy,
		OriginalDID:             cmd.OriginalDID,
		OriginalContractVersion: original.ContractVersion,
		ContractData:            renewalContractData,
		OccurredAt:              time.Now().UTC(),
		UserRoles:               cmd.UserRoles,
		Responsible:             &resp,
	}
	if err := event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine); err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &RenewResult{OriginalContractVersion: original.ContractVersion}, nil
}

// signatureAttributionPredicates are the assertions the act of signing stamps
// onto a contract's party node: who signed for that party
// (signingmanagement/command/apply.go recordSignatory writes dcs:hasSignatory
// and the dcs:hasPowerOfAttorney that authorized them) and the ODRL role the
// seal assigned when the offer was accepted (sealAgreementForSigning writes
// odrl:function — odrl:contractedParty asserts that this party accepted).
var signatureAttributionPredicates = []string{
	"dcs:hasSignatory",
	"dcs:hasPowerOfAttorney",
	"odrl:function",
}

// unsignRenewedDocument strips the previous term's signature evidence from the
// document a renewal copies, so a renewal draft that nobody has signed does not
// assert in its machine-readable contract that named people signed it under
// named authority — an assertion that would travel into the rendered PDF, to
// every peer, and into the Power-of-Attorney compliance findings
// (signingmanagement/command/compliance.go reads exactly these predicates).
//
// It clears predicates and never removes a party node. The nodes are also the
// read-authorization records (attachContractParties' #party-N nodes and their
// dcs:legalName, which gate query/contract/querybyid.go), and CreateCmd.Parties
// is not persisted anywhere — a dropped node could not be rebuilt.
//
// The signature FIELDS are left in place: they declare where the new term is to
// be signed, not that it was, and an authored multi-signatory contract names
// them itself, so they are part of the terms the renewal inherits.
func unsignRenewedDocument(raw *datatype.JSON) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}

	nodes, _ := doc["dcs:parties"].([]any)
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		for _, predicate := range signatureAttributionPredicates {
			delete(node, predicate)
		}
	}

	// Break the seal: the first signature retyped the offered policy set into
	// the odrl:Agreement those signatures bind. A renewal is a fresh offer of
	// the same terms, and nobody has accepted it yet.
	if policies, ok := doc["dcs:policies"].(map[string]any); ok {
		if policyType, _ := policies["@type"].(string); strings.HasSuffix(policyType, "Agreement") {
			policies["@type"] = "odrl:Offer"
		}
	}

	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}

// attachRenewsContractReference adds the dcs:renewsContract back-reference
// (DCS-FR-CWE-11: "reference links"; DCS-FR-CSA-15: "retain references to
// the prior contract's version [and] ID") to an already-normalized contract
// document. It is a plain top-level JSON-LD property, distinct from
// dcs:parentContract (ADR-7's frame-agreement hierarchy link, which must
// stay child->parent only and singular) — a renewal is not a sub-contract of
// the original, it supersedes it.
func attachRenewsContractReference(raw *datatype.JSON, originalDID string, originalVersion int) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	doc["dcs:renewsContract"] = map[string]any{
		"@id":         base.ResourceIRI("contract", originalDID),
		"dcs:version": originalVersion,
	}
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}
