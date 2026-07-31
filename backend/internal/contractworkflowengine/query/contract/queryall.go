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
		contractsMetadata, err = h.CRepo.ReadAllMetaData(ctx, tx, query.Pagination)
		if err != nil {
			return nil, fmt.Errorf("could not read all contracts: %w", err)
		}
	}

	// Full-scope hierarchy filter: keep only children whose
	// dcs:parentContract references the requested parent DID. ReadAllMetaData
	// already extracts parent_contract_did, so this is an in-memory narrowing.
	if query.ParentDID != "" {
		filtered := contractsMetadata[:0]
		for _, c := range contractsMetadata {
			if c.ParentContractDID != nil && *c.ParentContractDID == query.ParentDID {
				filtered = append(filtered, c)
			}
		}
		contractsMetadata = filtered
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

	didToVersion := make(map[string]int, len(contractsMetadata))
	for _, c := range contractsMetadata {
		didToVersion[c.DID] = c.ContractVersion
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
			ContractVersion: didToVersion[data.DID],
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
		negotiationTaskItems = append(negotiationTaskItems, NegotiatorTaskItem{
			DID:             data.DID,
			State:           state,
			ContractVersion: didToVersion[data.DID],
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
			ContractVersion: didToVersion[data.DID],
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
