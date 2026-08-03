package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/validation"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	semanticmapper "digital-contracting-service/internal/semantic/mapper"
)

type ApproveCmd struct {
	DID           string             `json:"did"`
	UpdatedAt     time.Time          `json:"updated_at"`
	ApprovedBy    string             `json:"approved_by"`
	DecisionNotes []string           `json:"decision_notes"`
	HolderDID     string             `json:"holder_did"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
	CauserDID     string             `json:"causer_did"`
}

type Approver struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	ATRepo      db.ApprovalTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

type ArchiveSnapshotStorer interface {
	Put(ctx context.Context, scope artifactstore.Scope, plaintext []byte) (string, error)
}

type ArchiveNotary interface {
	NotarizeArchiveEntry(ctx context.Context, payload ArchiveNotaryPayload) (*ArchiveNotaryReceipt, error)
}

func (h *Approver) Handle(ctx context.Context, cmd ApproveCmd) error {

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
	// older than what's stored (see package doc / ADR-0007). The distinct
	// messages tell a local caller to simply reload vs. a forwarded/remote
	// caller to force a full peer re-sync first.
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventApprove); err != nil {
		return err
	}

	// IsValidApprover checks CauserDID (a peer DID) against the task's assigned
	// approver — task ownership is peer-scoped, not individual-user-scoped;
	// per-user permission was already checked locally via UserRoles upstream.
	valid, err := h.ATRepo.IsValidApprover(ctx, tx, cmd.DID, cmd.CauserDID)
	if err != nil {
		return err
	}

	if !valid {
		return errors.New("invalid user")
	}

	contractForPolicyValidation, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read contract for policy validation: %w", err)
	}
	if contractForPolicyValidation.ContractData == nil {
		return fmt.Errorf("contract %s has no contract data for policy validation", cmd.DID)
	}
	if err := validation.ValidateContractPolicySatisfaction(
		*contractForPolicyValidation.ContractData,
		validation.ContractContentAuditMetadata{
			ContractDID:     cmd.DID,
			ContractVersion: fmt.Sprint(processData.ContractVersion),
			AuditedBy:       cmd.ApprovedBy,
			HolderDID:       cmd.HolderDID,
		},
	); err != nil {
		return err
	}

	// SRS Contract Approval verifies schema completeness: an approved contract
	// must be closed — no unresolved placeholders (negotiated boundaries,
	// required fields, prose placeholders).
	if err := validation.ValidateContractClosed(*contractForPolicyValidation.ContractData); err != nil {
		return err
	}

	err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, cmd.CauserDID, approvaltaskstate.Approved.String())
	if err != nil {
		return fmt.Errorf("could not update approval task state: %w", err)
	}

	existOpenTasks, err := h.ATRepo.AnyTasksInState(ctx, tx, processData.DID, approvaltaskstate.Open.String())
	if err != nil {
		return fmt.Errorf("could not check if review task exists: %w", err)
	}

	if !existOpenTasks {
		err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Approved.String())
		if err != nil {
			return fmt.Errorf("could not update current template state: %w", err)
		}

		approvedContract, err := h.CRepo.ReadDataByDID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read approved contract for archive storage: %w", err)
		}
		finalContractData, err := semanticmapper.BuildContractJSONLD(*approvedContract)
		if err != nil {
			return fmt.Errorf("could not materialize approved contract JSON-LD: %w", err)
		}
		finalContractJSON, err := datatype.NewJSON(finalContractData)
		if err != nil {
			return fmt.Errorf("could not encode approved contract JSON-LD: %w", err)
		}
		approvedContract.ContractData = &finalContractJSON
		if err := h.CRepo.Update(ctx, tx, db.ContractUpdateData{
			DID:          cmd.DID,
			ContractData: approvedContract.ContractData,
		}); err != nil {
			return fmt.Errorf("could not persist approved contract JSON-LD: %w", err)
		}
	}

	evt := contractevents.ApproveEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ApprovedBy:      cmd.ApprovedBy,
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
