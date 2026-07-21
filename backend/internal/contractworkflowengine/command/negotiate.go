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
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Negotiator) Handle(ctx context.Context, cmd NegotiationCmd) error {

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
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
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
		isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID)
		if err != nil {
			return fmt.Errorf("could not validate negotiator: %w", err)
		}
		if !isValidNegotiator {
			return ErrNotAParty
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
	_, err = h.NRepo.Create(ctx, tx, data, negotiators)
	if err != nil {
		return fmt.Errorf("could not create negotiation: %w", err)
	}

	// Ship-proposal-as-PDF-exchange (Arne 2026-07-20): a counter-offer applies its
	// redline to contract_data immediately, so the negotiated PDF re-renders with
	// the proposed value and re-ships to the peer over the PDF exchange (ADR-13) —
	// the peer reviews the redline in the received document and agrees at settle.
	// Changing the content is what makes the background regenerator produce and
	// ship a new PDF, growing the C2PA chain on both parties. The change request is
	// still recorded above for the negotiation audit trail (DCS-IR-CWE-03).
	// A change request is either free-text (a comment / redline note kept only
	// for the negotiation audit trail — stored raw above) or a structured
	// ChangeRequest carrying a contract_data redline. Only a structured redline
	// is applied immediately and re-shipped as a PDF; a free-text note (which
	// does not decode into the struct) has nothing to apply, so it is skipped.
	if cmd.ChangeRequest != nil {
		var change negotiationmerging.ChangeRequest
		if err := json.Unmarshal(*cmd.ChangeRequest, &change); err == nil && change.ContractData != nil {
			proposed := datatype.JSON(*change.ContractData)
			normalized, err := validation.NormalizeContractDataForPersistence(&proposed, cmd.DID, true)
			if err != nil {
				return fmt.Errorf("proposed contract data validation failed: %w", err)
			}
			if err := h.CRepo.Update(ctx, tx, db.ContractUpdateData{
				DID:             cmd.DID,
				ContractData:    normalized,
				ContractVersion: processData.ContractVersion + 1,
			}); err != nil {
				return fmt.Errorf("could not apply proposed change to contract data: %w", err)
			}
		}
	}

	err = h.NTRepo.ReopenTasks(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not reopen negotiation: %w", err)
	}

	// Negotiation is where the participating DCS instances are finalized, so it
	// is where the contract's signature fields are materialized: one
	// dcs:SignatureField per instance, the AcroForm field the wallet-driven
	// signing ceremony signs (ADR-12). Without a pre-placed field, the
	// deterministic two-call remote signing has nothing to sign.
	contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract for signature-field seeding: %w", err)
	}
	seeded, changed, err := seedSignatureFields(*contract.ContractData, contract.Responsible.GetParties())
	if err != nil {
		return fmt.Errorf("could not seed signature fields: %w", err)
	}
	if changed {
		if err := h.CRepo.Update(ctx, tx, db.ContractUpdateData{DID: cmd.DID, ContractData: &seeded}); err != nil {
			return fmt.Errorf("could not persist seeded signature fields: %w", err)
		}
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
		if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Negotiation.String()); err != nil {
			return fmt.Errorf("could not persist negotiation state: %w", err)
		}
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

// seedSignatureFields adds one dcs:SignatureField per participating DCS
// instance to the contract document, its dcs:signatoryName set to that
// instance's DID — the value pdf-core renders as the AcroForm field's /T name,
// which the wallet-driven signing ceremony targets (ADR-12). An explicit
// declaration wins: a contract that already declares any signature fields is
// signed against exactly those and is left untouched, so an authored
// multi-signatory contract is never augmented. Otherwise it auto-seeds one
// field per instance and is idempotent — re-running over its own output adds
// nothing. It reports whether the document changed so the caller only persists
// real additions.
func seedSignatureFields(raw datatype.JSON, instanceDIDs []string) (datatype.JSON, bool, error) {
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
