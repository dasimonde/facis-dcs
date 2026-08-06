package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

// AcceptOfferCmd accepts an INBOUND offer as-is, with no redline. It is the
// counterparty's plain "yes, let us negotiate this document" — distinct from
// AcceptNegotiationCmd (POST /contract/respond, action_flag ACCEPTING), which
// decides one already-proposed change request.
type AcceptOfferCmd struct {
	DID        string             `json:"did"`
	AcceptedBy string             `json:"accepted_by"`
	UpdatedAt  time.Time          `json:"updated_at"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
	CauserDID  string             `json:"causer_did"`
}

type OfferAcceptor struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	NTRepo      db.NegotiationTaskRepo
	DIDDocument identity.DIDDocument
}

// Handle mints this instance's negotiation task for the offer's current round
// and carries the contract from OFFERED into NEGOTIATION (contractstate
// Transitions: Offered --Negotiate--> Negotiation, SRS §4 — the Responder may
// accept, negotiate or refuse). Receiving an offer mints nothing; accepting it
// is the act that makes "a party engaged with this round" true, which is what
// submit's settlement gate reads.
func (h *OfferAcceptor) Handle(ctx context.Context, cmd AcceptOfferCmd) error {

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

	// Optimistic concurrency against content_updated_at, as in negotiate: an
	// acceptance is a decision about a specific document.
	if cmd.UpdatedAt.Unix() < processData.ContentUpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return fmt.Errorf("contract %w, please force synchronisation and reload", base.ErrUpdatedElsewhere)
		}
		return fmt.Errorf("contract %w, please reload", base.ErrUpdatedElsewhere)
	}

	// A repeat acceptance is legal and inert: EventNegotiate is declared both
	// from Offered and as a Negotiation self-loop, and the mint below is
	// idempotent for the round.
	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventNegotiate); err != nil {
		return err
	}

	// Only an inbound offer can be accepted. The Responder's authority is being
	// the designated counterparty (ADR-13; see command/negotiate.go's inbound
	// branch) — deliberately not IsValidNegotiator, since the task this command
	// mints is precisely what does not exist yet.
	if processData.Origin == localPeer {
		return ErrNotAParty
	}

	if err := mintNegotiationTask(ctx, tx, h.NTRepo, cmd.DID, localPeer, cmd.AcceptedBy, processData.ContractVersion); err != nil {
		return err
	}

	// Entering NEGOTIATION finalizes the participating parties, so the signable
	// structure is materialized here as it is on a redline (negotiate.go): one
	// party node per instance and the dcs:SignatureField naming it (ADR-12).
	if err := reseedPartiesAndSignatureFields(ctx, tx, h.CRepo, cmd.DID); err != nil {
		return err
	}

	currentState := contractstate.ContractState(processData.State)
	if currentState == contractstate.Offered {
		if err := contractstate.ValidateOutcome(currentState, contractstate.EventNegotiate, contractstate.Negotiation); err != nil {
			return err
		}
		if err := h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Negotiation.String()); err != nil {
			return fmt.Errorf("could not persist negotiation state: %w", err)
		}
	}

	evt := contractevents.AcceptOfferEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		AcceptedBy:      cmd.AcceptedBy,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
		OccurredAt:      time.Now().UTC(),
	}
	if err := event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
