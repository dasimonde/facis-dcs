package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

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

// OfferCmd carries the inputs for transmitting a draft contract to its
// counterparty for the first time (SRS 2.2.6, DRAFT -> OFFERED).
type OfferCmd struct {
	DID       string             `json:"did"`
	UpdatedAt time.Time          `json:"updated_at"`
	OfferedBy string             `json:"offered_by"`
	HolderDID string             `json:"holder_did"`
	UserRoles userrole.UserRoles `json:"user_roles"`
	CauserDID string             `json:"causer_did"`
}

// Offerer handles the OfferCmd command.
type Offerer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	DIDDocument identity.DIDDocument
}

func (h *Offerer) Handle(ctx context.Context, cmd OfferCmd) error {

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
		return fmt.Errorf("could not read process data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if !cmd.UserRoles.HasRoles(userrole.ContractCreator, userrole.SystemContractCreator) {
		return errors.New("invalid user permission")
	}

	// This avoids that state changes on different DCS are possible
	if cmd.CauserDID == localPeer && cmd.OfferedBy != processData.CreatedBy {
		return errors.New("invalid participant")
	}

	currentState := contractstate.ContractState(processData.State)
	if err := contractstate.ValidateTransition(currentState, contractstate.EventOffer); err != nil {
		return err
	}

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract data: %w", err)
	}
	if err := validateOfferReady(ctx, contract.ContractData); err != nil {
		return err
	}

	err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Offered.String())
	if err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
	}

	evt := contractevents.OfferEvent{
		DID:             cmd.DID,
		HolderDID:       cmd.HolderDID,
		ContractVersion: processData.ContractVersion,
		OfferedBy:       cmd.OfferedBy,
		OccurredAt:      time.Now().UTC(),
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// validateOfferReady gates the offer on the same content checks submission
// runs, plus contract closedness: the offer is the first transmission to the
// counterparty, and SRS 2.2.2 requires the filled-out contract to be ready to
// be sent — an offer must be a definite proposal (SRS 1.2), so a draft still
// carrying unresolved placeholders must not leave the instance.
func validateOfferReady(ctx context.Context, contractData *datatype.JSON) error {
	if contractData == nil {
		return fmt.Errorf("%w: contract has no contract data", validation.ErrContractNotClosed)
	}
	if err := validation.ValidateContractSemantics(contractData); err != nil {
		return fmt.Errorf("contract semantic validation failed: %w", err)
	}
	if err := validation.ValidateContractClosed(*contractData); err != nil {
		return err
	}
	if err := validation.RequireHubConformance(ctx, contractData); err != nil {
		return fmt.Errorf("contract offer blocked: %w", err)
	}
	return nil
}
