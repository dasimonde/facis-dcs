package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

// SaveNegotiationDraftCmd carries a party-scoped staged change request (SRS
// §3.1.1 Contract Negotiation UI "Save draft"). Saving a draft changes no
// contract state, bumps no version, and emits no event — SavedBy is the
// party's participant ID, so any authorized negotiator of the same party
// continues the party's staged position, and nothing leaves the instance
// until proposed via the negotiate command.
type SaveNegotiationDraftCmd struct {
	DID           string             `json:"did"`
	SavedBy       string             `json:"saved_by"`
	ChangeRequest *datatype.JSON     `json:"change_request"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
}

type DeleteNegotiationDraftCmd struct {
	DID       string             `json:"did"`
	SavedBy   string             `json:"saved_by"`
	UserRoles userrole.UserRoles `json:"user_roles"`
}

type NegotiationDraftSaver struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
	NRepo db.NegotiationRepo
}

func (h *NegotiationDraftSaver) Handle(ctx context.Context, cmd SaveNegotiationDraftCmd) error {

	if !cmd.UserRoles.HasRoles(negotiationDraftRoles...) {
		return errors.New("invalid user permission")
	}

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

	// A draft may be staged exactly while proposing would be allowed
	// (Offered/Negotiation — the states the Negotiate view serves).
	if !contractstate.EventAllowed(contractstate.ContractState(processData.State), contractstate.EventNegotiate) {
		return fmt.Errorf("%w: a negotiation draft can only be saved while the contract is negotiable (state %s)",
			contractstate.ErrInvalidTransition, processData.State)
	}

	if err := h.NRepo.UpsertDraft(ctx, tx, cmd.DID, cmd.SavedBy, cmd.ChangeRequest); err != nil {
		return fmt.Errorf("could not save negotiation draft: %w", err)
	}

	return tx.Commit()
}

type NegotiationDraftDeleter struct {
	DB    *sqlx.DB
	NRepo db.NegotiationRepo
}

func (h *NegotiationDraftDeleter) Handle(ctx context.Context, cmd DeleteNegotiationDraftCmd) error {

	if !cmd.UserRoles.HasRoles(negotiationDraftRoles...) {
		return errors.New("invalid user permission")
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	if err := h.NRepo.DeleteDraft(ctx, tx, cmd.DID, cmd.SavedBy); err != nil {
		return fmt.Errorf("could not delete negotiation draft: %w", err)
	}

	return tx.Commit()
}

// negotiationDraftRoles mirrors the negotiate endpoint's role set: whoever may
// propose a change may stage one.
var negotiationDraftRoles = []userrole.UserRole{
	userrole.ContractCreator,
	userrole.SystemContractCreator,
	userrole.ContractNegotiator,
	userrole.ContractReviewer,
	userrole.SystemContractReviewer,
	userrole.ContractManager,
}
