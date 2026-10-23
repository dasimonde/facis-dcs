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
	"digital-contracting-service/internal/pdfgeneration/provenance"

	"github.com/jmoiron/sqlx"
)

// ErrContractNotRenewable is returned when the original contract is not in a
// state from which a renewal makes sense (DCS-FR-CWE-11/22). Mapped to a
// client error by service.mapContractCommandError.
var ErrContractNotRenewable = errors.New("contract is not in a renewable state")

// ErrInvalidRenewalTerm is returned when the renewal's term dates are
// inconsistent. Mapped to a client error by service.mapContractCommandError.
var ErrInvalidRenewalTerm = errors.New("invalid renewal term")

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
		return nil, fmt.Errorf("contract %w, please reload", base.ErrUpdatedElsewhere)
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
	priorSignatures, err := signatureReferences(original.ContractData)
	if err != nil {
		return nil, fmt.Errorf("could not read the previous term's signatures: %w", err)
	}
	originalPDF, err := h.CRepo.ReadPDFState(ctx, tx, cmd.OriginalDID)
	if err != nil {
		return nil, fmt.Errorf("could not read the original contract's PDF state: %w", err)
	}
	renewalContractData, err := attachRenewsContractReference(unsignedContractData, cmd.OriginalDID, original.ContractVersion,
		signedDocumentReference(originalPDF), priorSignatures)
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

	newStartDate, newExpDate, err := renewalTerm(original.StartDate, original.ExpDate, cmd.NewStartDate, cmd.NewExpDate, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	newExpPolicy := original.ExpPolicy
	if cmd.NewExpPolicy != nil {
		newExpPolicy = cmd.NewExpPolicy
	}
	newExpNoticePeriod := original.ExpNoticePeriod
	if cmd.NewExpNoticePeriod != nil {
		newExpNoticePeriod = cmd.NewExpNoticePeriod
	}

	renewalContractData, err = stampRenewalTerm(renewalContractData, sharedContractDetails{
		StartDate:       newStartDate,
		ExpDate:         newExpDate,
		ExpPolicy:       newExpPolicy,
		ExpNoticePeriod: newExpNoticePeriod,
	})
	if err != nil {
		return nil, err
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

// signatureReferences returns one dcs:signatures entry per party node of the
// original that carries a signatory: the party, its signatory and the Power of
// Attorney they signed under. These are the predicates unsignRenewedDocument
// clears from the copy. A countersignature applied to an already signed PDF
// stamps no party node (apply.go contractDocumentForSignature), so only the
// signed PDF holds it: see signedDocumentReference.
func signatureReferences(raw *datatype.JSON) ([]any, error) {
	if raw == nil {
		return nil, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	nodes, _ := doc["dcs:parties"].([]any)
	var refs []any
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok || node["dcs:hasSignatory"] == nil {
			continue
		}
		ref := map[string]any{
			"dcs:party":        map[string]any{"@id": node["@id"]},
			"dcs:hasSignatory": node["dcs:hasSignatory"],
		}
		if poa, ok := node["dcs:hasPowerOfAttorney"]; ok {
			ref["dcs:hasPowerOfAttorney"] = poa
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// signedDocumentReference returns the IPFS reference of the original's stored
// PDF when that artifact holds a signature, else "". The PDF carries every
// PAdES signature of the original, the counterparty's included.
func signedDocumentReference(state *db.ContractPDFState) string {
	if state == nil || state.IPFSCID == "" || !provenance.ArtifactCarriesSignature(state.C2PAState) {
		return ""
	}
	return "ipfs://" + state.IPFSCID
}

// attachRenewsContractReference adds the dcs:renewsContract back-reference
// (DCS-FR-CWE-11: "retaining linked metadata and signatures", "reference
// links"; DCS-FR-CSA-15: "retain references to the prior contract's version,
// ID, and signatures") to an already-normalized contract document. It is a
// plain top-level JSON-LD property, distinct from dcs:parentContract (ADR-7's
// frame-agreement hierarchy link, which must stay child->parent only and
// singular) — a renewal is not a sub-contract of the original, it supersedes
// it. dcs:signedDocument and dcs:signatures are omitted when empty.
func attachRenewsContractReference(raw *datatype.JSON, originalDID string, originalVersion int, signedDocument string, signatures []any) (*datatype.JSON, error) {
	var doc map[string]any
	if err := json.Unmarshal(*raw, &doc); err != nil {
		return nil, fmt.Errorf("could not decode contract data: %w", err)
	}
	reference := map[string]any{
		"@id":         base.ResourceIRI("contract", originalDID),
		"dcs:version": originalVersion,
	}
	if signedDocument != "" {
		reference["dcs:signedDocument"] = map[string]any{"@id": signedDocument}
	}
	if len(signatures) > 0 {
		reference["dcs:signatures"] = signatures
	}
	doc["dcs:renewsContract"] = reference
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("could not encode contract data: %w", err)
	}
	return &encoded, nil
}

// renewalTerm returns the renewal's start and expiry. Given dates win. Without
// a given expiry, an original whose expiry has passed moves forward by whole
// terms of its own length until the expiry lies ahead: a copied past expiry
// makes the new DRAFT read EXPIRED at once (contracts_effective view). Without
// both original dates the length is unknown, so the expiry stays empty.
func renewalTerm(origStart, origExp, newStart, newExp *time.Time, now time.Time) (*time.Time, *time.Time, error) {
	start, exp := origStart, origExp
	if newStart != nil {
		start = newStart
	}
	if newExp != nil {
		exp = newExp
	} else if origExp != nil && !origExp.After(now) {
		exp = nil
		if origStart != nil && origExp.After(*origStart) {
			length := origExp.Sub(*origStart)
			if newStart == nil {
				s := origExp.Add(now.Sub(*origExp) / length * length)
				start = &s
			}
			e := start.Add(length)
			exp = &e
		} else if newStart == nil {
			start = nil
		}
	}
	if exp != nil && !exp.After(now) {
		return nil, nil, fmt.Errorf("%w: expiration date must be in the future", ErrInvalidRenewalTerm)
	}
	if start != nil && exp != nil && !exp.After(*start) {
		return nil, nil, fmt.Errorf("%w: expiration date must be after start date", ErrInvalidRenewalTerm)
	}
	return start, exp, nil
}

// stampRenewalTerm writes the renewal's term into dcs:metadata. The copy
// carries the original's term, so an empty date removes the original's key.
func stampRenewalTerm(payload *datatype.JSON, term sharedContractDetails) (*datatype.JSON, error) {
	stamped, _, err := stampSharedDetailsIntoContractData(payload, term)
	if err != nil {
		return nil, err
	}
	if stamped == nil || (term.StartDate != nil && term.ExpDate != nil) {
		return stamped, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(*stamped), &doc); err != nil {
		return nil, fmt.Errorf("could not parse contract data for the renewal term: %w", err)
	}
	meta, _ := doc["dcs:metadata"].(map[string]any)
	if meta == nil {
		return stamped, nil
	}
	if term.StartDate == nil {
		delete(meta, metaStartDate)
	}
	if term.ExpDate == nil {
		delete(meta, metaExpirationDate)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("could not marshal contract data with the renewal term: %w", err)
	}
	cleared := datatype.JSON(raw)
	return &cleared, nil
}
