package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/identity"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/pdfgeneration/statuspublication"

	"github.com/jmoiron/sqlx"
)

// ErrTerminationReasonRequired is returned when a termination names no
// reason (DCS-FR-CWE-12). It is mapped to 400.
var ErrTerminationReasonRequired = errors.New("a termination needs a reason")

type TerminateCmd struct {
	DID          string             `json:"did"`
	TerminatedBy string             `json:"terminated_by"`
	Reason       string             `json:"reason"`
	UpdatedAt    time.Time          `json:"updated_at"`
	HolderDID    string             `json:"holder_did"`
	UserRoles    userrole.UserRoles `json:"user_roles"`
	CauserDID    string             `json:"causer_did"`
}

type Terminator struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Terminator) Handle(ctx context.Context, cmd TerminateCmd) error {
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	if cmd.Reason == "" {
		return ErrTerminationReasonRequired
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

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return fmt.Errorf("contract %w, please force synchronisation and reload", base.ErrUpdatedElsewhere)
		}
		return fmt.Errorf("contract %w, please reload", base.ErrUpdatedElsewhere)
	}

	// Terminate is allowed from any non-terminal state; there is no path back
	// out of TERMINATED. See contractstate.Transitions for the exact table.
	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventTerminate); err != nil {
		return err
	}

	role, err := initiatingRole(cmd, localPeer)
	if err != nil {
		return err
	}
	err = h.CRepo.UpdateState(ctx, tx, cmd.DID, processData.State, contractstate.Terminated.String())
	if err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
	}
	occurredAt := time.Now().UTC()
	// A concurrent termination that committed first holds the row. The
	// insert then adds nothing, and this one is refused as a transition from
	// TERMINATED.
	recorded, err := tx.ExecContext(ctx, `
		INSERT INTO contract_terminations (did, reason, effective_at, terminated_by, initiating_role)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (did) DO NOTHING`, cmd.DID, cmd.Reason, occurredAt, cmd.TerminatedBy, role)
	if err != nil {
		return fmt.Errorf("could not record the termination: %w", err)
	}
	if rows, err := recorded.RowsAffected(); err != nil || rows == 0 {
		return fmt.Errorf("%w: contract %s is already terminated", contractstate.ErrInvalidTransition, cmd.DID)
	}

	if err := closeOpenTasks(ctx, tx, cmd.DID); err != nil {
		return err
	}

	// A local termination notifies every counterparty instance
	// (DCS-NFR-BR-06). A termination received from a peer does not: the
	// terminating instance notifies all parties itself.
	if cmd.CauserDID == localPeer {
		if err := h.queuePeerTerminations(ctx, tx, cmd.DID, cmd.Reason, localPeer); err != nil {
			return err
		}
	}

	evt := contractevents.TerminateEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		TerminatedBy:    cmd.TerminatedBy,
		Reason:          cmd.Reason,
		OccurredAt:      occurredAt,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}
	if err := statuspublication.EnqueueTx(ctx, tx, cmd.DID, contractstate.Terminated.String(), cmd.Reason, occurredAt); err != nil {
		return err
	}

	return tx.Commit()
}

// CounterpartyTerminationRole is the initiating role of a termination that a
// counterparty instance notified.
const CounterpartyTerminationRole = "Counterparty"

// terminationRoles are the roles that may terminate a contract, in the order
// the initiating role is chosen from the caller's roles.
var terminationRoles = []userrole.UserRole{userrole.ContractManager, userrole.SystemContractManager, userrole.ArchiveManager}

// initiatingRole is the role recorded with a termination (DCS-FR-CSA-16).
func initiatingRole(cmd TerminateCmd, localPeer string) (string, error) {
	if cmd.CauserDID != localPeer {
		return CounterpartyTerminationRole, nil
	}
	for _, role := range terminationRoles {
		if slices.Contains(cmd.UserRoles, role) {
			return role.String(), nil
		}
	}
	return "", fmt.Errorf("terminating a contract needs one of the roles %v", terminationRoles)
}

// queuePeerTerminations records one termination notice per party instance
// other than this one that holds a copy of the contract;
// dcstodcs.TerminationNotifier delivers them. A party that never received
// the contract gets no notice.
func (h *Terminator) queuePeerTerminations(ctx context.Context, tx *sqlx.Tx, did, reason, localPeer string) error {
	contract, err := h.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read contract parties: %w", err)
	}
	if contract.Responsible == nil {
		return nil
	}
	deliveries, err := h.SRepo.GetPDFDeliveries(ctx, tx, did)
	if err != nil {
		return fmt.Errorf("could not read the peers that received contract %s: %w", did, err)
	}
	for _, party := range db2.PeersHoldingCopy(contract.Responsible.GetParties(), contract.Origin, deliveries) {
		if party == localPeer {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO contract_peer_terminations (did, peer_did, reason) VALUES ($1, $2, $3)
			ON CONFLICT (did, peer_did) DO NOTHING`, did, party, reason); err != nil {
			return fmt.Errorf("could not queue the termination notice toward %s: %w", party, err)
		}
	}
	return nil
}

// closeOpenTasks removes the contract's open negotiation, review and approval
// tasks: a terminated contract leaves every active workflow (DCS-FR-CWE-23).
// Decided tasks stay as the record of who decided what.
func closeOpenTasks(ctx context.Context, tx *sqlx.Tx, did string) error {
	for _, table := range []string{"contract_negotiation_task", "contract_review_task", "contract_approval_task"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE did = $1 AND state = 'OPEN'`, did); err != nil {
			return fmt.Errorf("could not close open tasks in %s: %w", table, err)
		}
	}
	return nil
}
