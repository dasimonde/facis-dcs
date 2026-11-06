package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	db2 "digital-contracting-service/internal/dcstodcs/db"
)

// ErrProposalAlreadyAccepted refuses withdraw once any counterparty has
// accepted the change request.
var ErrProposalAlreadyAccepted = errors.New("proposal has already been accepted")

// ErrNotProposalAuthor refuses withdraw by anyone other than the proposal's
// created_by (the participant that authored the change request).
var ErrNotProposalAuthor = errors.New("only the proposal author may withdraw it")

type WithdrawNegotiationCmd struct {
	ID          string             `json:"id"`
	DID         string             `json:"did"`
	WithdrawnBy string             `json:"withdrawn_by"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
	CauserDID   string             `json:"causer_did"`
}

type NegotiationWithdrawer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	NORepo      db.NegotiationOutboundRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *NegotiationWithdrawer) Handle(ctx context.Context, cmd WithdrawNegotiationCmd) error {
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

	err = contractstate.ValidateTransition(
		contractstate.ContractState(processData.State),
		contractstate.EventWithdrawNegotiation,
	)
	if err != nil {
		return err
	}

	createdBy, err := h.NRepo.ReadCreatedByByNegotiationID(ctx, tx, cmd.ID)
	if err != nil {
		return fmt.Errorf("could not read negotiation author: %w", err)
	}

	if createdBy == "" || createdBy != cmd.WithdrawnBy {
		return ErrNotProposalAuthor
	}

	accepted, err := h.NRepo.HasAcceptedDecision(ctx, tx, cmd.ID)
	if err != nil {
		return fmt.Errorf("could not check accepted decisions: %w", err)
	}

	if accepted {
		return ErrProposalAlreadyAccepted
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	if h.NORepo != nil {
		contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read contract parties for withdraw: %w", err)
		}
		if contract.Responsible != nil {
			for _, peer := range contract.Responsible.GetParties() {
				if identity.SameDIDWeb(peer, localPeer) {
					continue
				}
				err = h.NORepo.CancelUndeliveredProposal(ctx, tx, cmd.ID, peer)
				if err != nil {
					return fmt.Errorf("could not cancel undelivered proposal toward %s: %w", peer, err)
				}
			}
		}
	}

	err = enqueueNegotiationOutbound(ctx, tx, h.CRepo, h.NORepo, localPeer,
		db.NegotiationMessageWithdraw, cmd.ID, nil, cmd.DID, processData.ContractVersion)
	if err != nil {
		return err
	}

	// Tombstone before DELETE so a late Accept that lands after this commit
	// (partition race) can ACK as withdrawn instead of retrying forever.
	err = h.NRepo.RecordWithdrawn(ctx, tx, cmd.ID, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not record negotiation withdraw tombstone: %w", err)
	}

	err = h.NRepo.DeleteByID(ctx, tx, cmd.ID)
	if err != nil {
		return fmt.Errorf("could not delete withdrawn negotiation: %w", err)
	}

	evt := contractevents.WithdrawNegotiationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		NegotiationID:   cmd.ID,
		WithdrawnBy:     cmd.WithdrawnBy,
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
