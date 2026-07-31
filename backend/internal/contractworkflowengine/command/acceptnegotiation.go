// Package command implements the write-side CQRS use cases for the contract
// workflow engine: one file per Goa endpoint, each following the same shape
// (BeginTx -> forward-if-not-origin -> validate -> mutate -> event.Create ->
// Commit). Two cross-cutting rules recur across most handlers here:
//
//  1. Single-writer-per-aggregate: a contract's Origin peer is its sole
//     writer. A handler that finds it is not running on the Origin peer
//     forwards the exact same command, unmutated, to the Origin via a
//     signed did:web RPC (contractworkflowengine/remotesync/remoteaction)
//     instead of applying the change locally (see ADR-0005).
//  2. Optimistic concurrency: state-mutating commands carry a client-supplied
//     UpdatedAt that is compared against the stored value before any
//     mutation, rejecting stale writes (see ADR-0007). Not every handler in
//     this package requires it (e.g. AcceptNegotiation/RejectNegotiation
//     currently don't), which is a known inconsistency, not an oversight to
//     replicate elsewhere.
package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/identity"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type AcceptNegotiationCmd struct {
	ID         string             `json:"id"`
	DID        string             `json:"did"`
	AcceptedBy string             `json:"accepted_by"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
	CauserDID  string             `json:"causer_did"`
}

type NegotiationAcceptor struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *NegotiationAcceptor) Handle(ctx context.Context, cmd AcceptNegotiationCmd) error {

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

	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventAcceptNegotiation); err != nil {
		return err
	}

	isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID)
	if err != nil {
		return fmt.Errorf("could not validate negotiator: %w", err)
	}

	if !isValidNegotiator {
		return ErrNotAParty
	}

	// Conflict-of-interest guard (FR-CWE-07): the identity that authored this
	// negotiation's change_request may not be the same identity now accepting
	// it. created_by/AcceptedBy are both the caller's participant identity
	// (middleware.GetParticipantID — the organization claim from the OID4VP
	// credential, see internal/middleware/oidc.go), independent of the
	// peer-DID-scoped CauserDID checked above.
	createdBy, err := h.NRepo.ReadCreatedByByNegotiationID(ctx, tx, cmd.ID)
	if err != nil {
		return fmt.Errorf("could not read negotiation author: %w", err)
	}
	if createdBy != "" && createdBy == cmd.AcceptedBy {
		return ErrConflictOfInterest
	}

	err = h.NRepo.Accept(ctx, tx, cmd.ID, cmd.CauserDID)
	if err != nil {
		return fmt.Errorf("could not accept negotiation: %w", err)
	}

	evt := contractevents.AcceptNegotiationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		UserRoles:       cmd.UserRoles,
		AcceptedBy:      cmd.AcceptedBy,
		HolderDID:       cmd.HolderDID,
		OccurredAt:      time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
