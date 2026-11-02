package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/identity"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"

	"github.com/jmoiron/sqlx"
)

type NegotiationCmd struct {
	DID           string             `json:"did"`
	NegotiatedBy  string             `json:"negotiated_by"`
	ChangeRequest *datatype.JSON     `json:"change_request"`
	UpdatedAt     time.Time          `json:"updated_at"`
	HolderDID     string             `json:"holder_did"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
	CauserDID     string             `json:"causer_did"`
}

type Negotiator struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	NORepo      db.NegotiationOutboundRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Negotiator) Handle(ctx context.Context, cmd NegotiationCmd) error {
	var change negotiationmerging.ChangeRequest
	if cmd.ChangeRequest != nil {
		var err error
		if change, _, err = negotiationmerging.Decode(*cmd.ChangeRequest); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not process core data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	// Optimistic concurrency (lost-update guard, ADR-0007): reject only if the
	// contract's CONTENT changed after the caller's view — compare against
	// content_updated_at, which moves solely on a real contract_data edit, not on
	// benign writes (a state transition or a background artifact write) that nudge
	// updated_at without changing content and would otherwise false-trip this.
	if cmd.UpdatedAt.Unix() < processData.ContentUpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return fmt.Errorf("contract %w, please force synchronisation and reload", base.ErrUpdatedElsewhere)
		}
		return fmt.Errorf("contract %w, please reload", base.ErrUpdatedElsewhere)
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventNegotiate); err != nil {
		return err
	}

	// Authorization splits on who owns the contract (SRS §4 Contract Negotiation
	// & Review: the Responder reviews an offered contract and may accept,
	// negotiate, or refuse it). For an INBOUND offer (Origin != localPeer) this
	// instance is the Responder/counterparty, and its right to negotiate derives
	// from being the designated counterparty — not from a local negotiator-task
	// assignment. Local negotiator RBAC governs only contracts this instance
	// itself authored (Origin == localPeer).
	if processData.Origin == localPeer {
		isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID, processData.ContractVersion)
		if err != nil {
			return fmt.Errorf("could not validate negotiator: %w", err)
		}
		if !isValidNegotiator {
			return ErrNotAParty
		}
	} else {
		// Proposing a redline on an inbound offer is engaging with the round just
		// as accepting it is, and it may be the counterparty's first act on a
		// contract whose receipt minted nothing. Without the task, the decision
		// rows created below name no negotiator and this instance's own submit
		// has no evidence that it ever took part.
		if err := mintNegotiationTask(ctx, tx, h.NTRepo, cmd.DID, localPeer, cmd.NegotiatedBy, processData.ContractVersion); err != nil {
			return err
		}
	}

	negotiators, err := h.NTRepo.ReadNegotiatorsForDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read negotiators: %w", err)
	}
	data := db.NegotiationCreateData{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ChangeRequest:   cmd.ChangeRequest,
		CreatedBy:       cmd.NegotiatedBy,
	}
	negotiationID, _, err := h.NRepo.Create(ctx, tx, data, negotiators)
	if err != nil {
		return fmt.Errorf("could not create negotiation: %w", err)
	}

	// Ship-proposal-as-PDF-exchange (Arne 2026-07-20): a counter-offer used to
	// apply its redline to contract_data immediately, so the negotiated PDF
	// re-rendered with the proposed value and re-shipped to the peer over the
	// PDF exchange (ADR-13) — the peer reviewed the redline in the received
	// document and agreed at settle. Changing the content is what made the
	// background regenerator produce and ship a new PDF, growing the C2PA chain
	// on both parties. That immediate apply is withdrawn: the change request is
	// still recorded above for the negotiation audit trail (DCS-IR-CWE-03)
	// against the current formal version, and accepted changes become formal
	// only when Submit merges them (submit.go). A change request is either
	// free-text (a comment / redline note kept only for the negotiation audit
	// trail — stored raw above) or a structured ChangeRequest carrying a
	// contract_data redline. Only a structured redline is validated here; a
	// free-text note has nothing to validate, so it is skipped.
	if cmd.ChangeRequest != nil {
		if change.ContractData != nil {
			// A redline on a settled agreement is the wedge: the document changes
			// but the artifact is the peer's signed PDF and cannot be re-rendered,
			// so the proposal would ship the OLD signed bytes while this copy's own
			// document moved on. Before any signature exists the same redline is
			// this instance rewriting the version it told the counterparty it
			// agreed to. Refuse both here — a free-text note (no
			// change.ContractData) leaves the document alone and still carries this
			// copy out of OFFERED towards its own countersignature.
			if err := requireUnsettledAgreement(ctx, tx, h.CRepo, h.SRepo, localPeer, cmd.DID); err != nil {
				return err
			}
			proposed := datatype.JSON(*change.ContractData)
			normalized, err := validation.NormalizeContractDataForPersistence(&proposed, cmd.DID, true)
			if err != nil {
				return fmt.Errorf("proposed contract data validation failed: %w", err)
			}
			stored, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
			if err != nil {
				return fmt.Errorf("could not read contract data: %w", err)
			}
			// The redline is the document the proposing client assembled, so the
			// bundle this copy is pinned to travels from the stored document
			// (CarrySemanticBundle). Validated here; persisted only on Submit.
			_, err = validation.CarrySemanticBundle(stored.ContractData, normalized)
			if err != nil {
				return fmt.Errorf("could not carry the pinned Semantic Hub bundle forward: %w", err)
			}
		}
	}

	// Propose does not replace the document or bump the version, so the round
	// is unchanged and reopening is all that is owed.
	err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID, processData.ContractVersion)
	if err != nil {
		return fmt.Errorf("could not reopen negotiation: %w", err)
	}

	err = enqueueNegotiationOutbound(ctx, tx, h.CRepo, h.NORepo, localPeer,
		db.NegotiationMessageProposal, negotiationID, nil, cmd.DID, processData.ContractVersion)
	if err != nil {
		return err
	}

	// The Responder choosing to negotiate an offered contract starts the
	// negotiation phase (SRS §4; transition.go Offered -> Negotiation via
	// EventNegotiate). Later redlines happen within NEGOTIATION (a self-loop) and
	// leave the state unchanged.
	currentState := contractstate.ContractState(processData.State)
	if currentState == contractstate.Offered {
		if err := contractstate.ValidateOutcome(currentState, contractstate.EventNegotiate, contractstate.Negotiation); err != nil {
			return err
		}
		if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, processData.State, contractstate.Negotiation.String()); err != nil {
			return fmt.Errorf("could not persist negotiation state: %w", err)
		}
	}

	// Proposing consumes the author's private staged draft (SRS §3.1.1 "Save
	// draft" vs "Propose change"): what was staged is now on the table.
	if err := h.NRepo.DeleteDraft(ctx, tx, cmd.DID, cmd.NegotiatedBy); err != nil {
		return fmt.Errorf("could not clear negotiation draft: %w", err)
	}

	evt := contractevents.NegotiationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ChangeRequest:   cmd.ChangeRequest,
		NegotiatedBy:    cmd.NegotiatedBy,
		Negotiators:     negotiators,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// SeedSignatureFields adds one dcs:SignatureField per participating DCS
// instance to the contract document, its dcs:signatoryName set to that
// instance's DID — the value pdf-core renders as the AcroForm field's /T name,
// which the wallet-driven signing ceremony targets (ADR-12). An explicit
// declaration wins: a contract that already declares any signature fields is
// signed against exactly those and is left untouched, so an authored
// multi-signatory contract is never augmented. Otherwise it auto-seeds one
// field per instance and is idempotent — re-running over its own output adds
// nothing. It reports whether the document changed so the caller only persists
// real additions.
func SeedSignatureFields(raw datatype.JSON, instanceDIDs []string) (datatype.JSON, bool, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("decode contract data: %w", err)
	}

	fields, _ := doc["dcs:signatureFields"].([]any)
	// Explicit declaration wins: a contract that already declares its signature
	// fields (e.g. a multi-signatory contract naming each signer) is signed
	// against exactly those, so the per-party auto-seed does not add an extra
	// instance-DID field on top of them.
	if len(fields) > 0 {
		return raw, false, nil
	}

	present := map[string]bool{}
	docID, _ := doc["@id"].(string)
	changed := false
	for _, did := range instanceDIDs {
		if present[did] {
			continue
		}
		digest := sha256.Sum256([]byte(did))
		fields = append(fields, map[string]any{
			"@id":               fmt.Sprintf("%s#signature-field-%s", docID, hex.EncodeToString(digest[:8])),
			"@type":             "dcs:SignatureField",
			"dcs:signatoryName": did,
		})
		present[did] = true
		changed = true
	}
	if !changed {
		return raw, false, nil
	}

	doc["dcs:signatureFields"] = fields
	encoded, err := datatype.NewJSON(doc)
	if err != nil {
		return nil, false, fmt.Errorf("encode contract data: %w", err)
	}
	return encoded, true, nil
}
