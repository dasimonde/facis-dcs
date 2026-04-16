package contract

import (
	"context"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	events "digital-contracting-service/internal/contractworkflowengine/event"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type GetAllMetadataQry struct {
	RetrievedBy string
}

type MetadataItem struct {
	DID             string
	ContractVersion *int
	Name            *string
	Description     *string
	State           contractstate.ContractState
	CreatedAt       time.Time
	UpdatedAt       time.Time
	MetaData        datatype.JSON
	CreatedBy       string
}

type ReviewTaskItem struct {
	DID             string
	ContractVersion *int
	State           reviewtaskstate.ReviewTaskState
	Reviewer        string
	CreatedAt       time.Time
}

type ApprovalTaskItem struct {
	DID             string
	ContractVersion *int
	State           approvaltaskstate.ApprovalTaskState
	Approver        string
	CreatedAt       time.Time
}

type NegotiatorTaskItem struct {
	DID             string
	ContractVersion *int
	State           negotiationtaskstate.NegotiationTaskState
	Negotiator      string
	CreatedAt       time.Time
}

type GetAllMetadataResult struct {
	Contracts       []MetadataItem
	ReviewerTasks   []ReviewTaskItem
	ApprovalTasks   []ApprovalTaskItem
	NegotiatorTasks []NegotiatorTaskItem
}

type GetAllMetadataHandler struct {
	Ctx    context.Context
	DB     *sqlx.DB
	CRepo  db.ContractRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
	NTRepo db.NegotiationTaskRepo
}

func (h *GetAllMetadataHandler) Handle(query GetAllMetadataQry) (*GetAllMetadataResult, error) {

	ctx, cancel := context.WithTimeout(h.Ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer tx.Rollback()

	contractsMetadata, err := h.CRepo.ReadAllMetaData(tx)
	if err != nil {
		return nil, fmt.Errorf("could not read all contracts: %w", err)
	}

	negotiationTasks, err := h.NTRepo.ReadAllByNegotiator(tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all negotiation tasks: %w", err)
	}

	reviewerTasks, err := h.RTRepo.ReadAllByReviewer(tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	approvalTasks, err := h.ATRepo.ReadAllByApprover(tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	evt := events.RetrieveAllEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now(),
	}
	err = event.Create(h.Ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	didToMetadata := make(map[string]MetadataItem)
	var contractItems []MetadataItem
	for _, data := range contractsMetadata {

		state, err := contractstate.NewContractState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract state: %w", err)
		}

		metadata := MetadataItem{
			DID:             data.DID,
			ContractVersion: data.ContractVersion,
			State:           state,
			Name:            data.Name,
			Description:     data.Description,
			CreatedBy:       data.CreatedBy,
			CreatedAt:       data.CreatedAt,
			UpdatedAt:       data.UpdatedAt,
		}
		contractItems = append(contractItems, metadata)

		didToMetadata[data.DID] = metadata
	}

	var reviewTaskItems []ReviewTaskItem
	for _, data := range reviewerTasks {

		state, err := reviewtaskstate.NewReviewTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create review task state: %w", err)
		}

		metadata, exists := didToMetadata[data.DID]
		var contractVersion *int
		if exists {
			contractVersion = metadata.ContractVersion
		}

		reviewTaskItems = append(reviewTaskItems, ReviewTaskItem{
			DID:             data.DID,
			State:           state,
			ContractVersion: contractVersion,
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

		metadata, exists := didToMetadata[data.DID]
		var contractVersion *int
		if exists {
			contractVersion = metadata.ContractVersion
		}

		negotiationTaskItems = append(negotiationTaskItems, NegotiatorTaskItem{
			DID:             data.DID,
			State:           state,
			ContractVersion: contractVersion,
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

		metadata, exists := didToMetadata[data.DID]
		var contractVersion *int
		if exists {
			contractVersion = metadata.ContractVersion
		}

		approvalTasksItems = append(approvalTasksItems, ApprovalTaskItem{
			DID:             data.DID,
			ContractVersion: contractVersion,
			State:           state,
			Approver:        data.Approver,
			CreatedAt:       data.CreatedAt,
		})
	}

	return &GetAllMetadataResult{
		Contracts:       contractItems,
		ReviewerTasks:   reviewTaskItems,
		ApprovalTasks:   approvalTasksItems,
		NegotiatorTasks: negotiationTaskItems,
	}, nil
}
