// Package contract implements read-side CQRS use cases scoped to a single
// contract (as opposed to the parent query package's cross-cutting task
// queries).
package contract

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/identity"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	events "digital-contracting-service/internal/contractworkflowengine/event"
)

type GetAllMetadataQry struct {
	RetrievedBy string
	HolderDID   string
	ParentDID   string
	States      []string
	SortBy      string
	Ascending   bool
	Pagination  datatype.Pagination
	UserRoles   userrole.UserRoles
	DIDDocument identity.DIDDocument
}

type ReviewTaskItem struct {
	DID             string
	ContractVersion int
	State           reviewtaskstate.ReviewTaskState
	Reviewer        string
	CreatedAt       time.Time
}

type ApprovalTaskItem struct {
	DID             string
	ContractVersion int
	State           approvaltaskstate.ApprovalTaskState
	Approver        string
	CreatedAt       time.Time
}

type NegotiatorTaskItem struct {
	DID             string
	ContractVersion int
	State           negotiationtaskstate.NegotiationTaskState
	Negotiator      string
	CreatedAt       time.Time
}

type GetAllMetadataResult struct {
	Contracts       []db.ContractMetadata
	ReviewerTasks   []ReviewTaskItem
	ApprovalTasks   []ApprovalTaskItem
	NegotiatorTasks []NegotiatorTaskItem
}

type GetAllMetadataHandler struct {
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
}

func (h *GetAllMetadataHandler) Handle(ctx context.Context, query GetAllMetadataQry) (*GetAllMetadataResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	did, err := query.DIDDocument.GetID()
	if err != nil {
		return nil, fmt.Errorf("could not get DID: %w", err)
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var contractsMetadata []db.ContractMetadata
	if query.Pagination.Limit >= 0 {
		contractsMetadata, err = h.CRepo.ReadAllMetaData(ctx, tx, query.Pagination, db.ListOptions{
			States:    query.States,
			ParentDID: query.ParentDID,
			SortBy:    query.SortBy,
			Ascending: query.Ascending,
		})
		if err != nil {
			return nil, fmt.Errorf("could not read all contracts: %w", err)
		}
	}

	negotiationTasks, err := h.NTRepo.ReadAllByNegotiator(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("could not read all negotiation tasks: %w", err)
	}

	reviewerTasks, err := h.RTRepo.ReadAllByReviewer(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	approvalTasks, err := h.ATRepo.ReadAllByApprover(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	evt := events.RetrieveAllEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	var reviewTaskItems []ReviewTaskItem
	for _, data := range reviewerTasks {
		state, err := reviewtaskstate.NewReviewTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create review task state: %w", err)
		}
		reviewTaskItems = append(reviewTaskItems, ReviewTaskItem{
			DID:             data.DID,
			State:           state,
			ContractVersion: data.ContractVersion,
			Reviewer:        data.Reviewer,
			CreatedAt:       data.CreatedAt,
		})
	}

	var negotiationTaskItems []NegotiatorTaskItem
	for _, data := range negotiationTasks {
		state, err := negotiationtaskstate.NewNegotiationTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create negotiation task state: %w", err)
		}
		// The task's own round, not the contract's current version: a task is
		// minted per negotiation round and stays on the round it was minted for
		// until it is carried forward.
		negotiationTaskItems = append(negotiationTaskItems, NegotiatorTaskItem{
			DID:             data.DID,
			State:           state,
			ContractVersion: data.ContractVersion,
			Negotiator:      data.Negotiator,
			CreatedAt:       data.CreatedAt,
		})
	}

	var approvalTasksItems []ApprovalTaskItem
	for _, data := range approvalTasks {
		state, err := approvaltaskstate.NewApprovalTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create approval task state: %w", err)
		}
		approvalTasksItems = append(approvalTasksItems, ApprovalTaskItem{
			DID:             data.DID,
			ContractVersion: data.ContractVersion,
			State:           state,
			Approver:        data.Approver,
			CreatedAt:       data.CreatedAt,
		})
	}

	return &GetAllMetadataResult{
		Contracts:       contractsMetadata,
		ReviewerTasks:   reviewTaskItems,
		ApprovalTasks:   approvalTasksItems,
		NegotiatorTasks: negotiationTaskItems,
	}, nil
}
