// Package contracttemplate implements read-side CQRS use cases scoped to a
// single template (as opposed to the parent query package's cross-cutting
// task queries).
package contracttemplate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type GetAllMetadataQry struct {
	RetrievedBy string
	HolderDID   string
	Pagination  datatype.Pagination
	UserRoles   userrole.UserRoles
}

type MetadataItem struct {
	DID          string
	Version      int
	State        contracttemplatestate.ContractTemplateState
	TemplateType contracttemplatetype.ContractTemplateType
	Name         *string
	Description  *string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MetaData     datatype.JSON
	LatestDID    *string
}

type ReviewTaskItem struct {
	DID       string
	Version   int
	State     reviewtaskstate.ReviewTaskState
	Reviewer  string
	CreatedAt time.Time
}

type ApprovalTaskItem struct {
	DID       string
	Version   int
	State     approvaltaskstate.ApprovalTaskState
	Approver  string
	CreatedAt time.Time
}

type GetAllMetadataResult struct {
	ContractTemplates []MetadataItem
	ReviewerTasks     []ReviewTaskItem
	ApprovalTasks     []ApprovalTaskItem
}

type GetAllMetadataHandler struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *GetAllMetadataHandler) Handle(ctx context.Context, query GetAllMetadataQry) (*GetAllMetadataResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var contractTemplates []db.ContractTemplateMetadata
	if query.Pagination.Limit >= 0 {
		contractTemplates, err = h.CTRepo.ReadAllMetaData(ctx, tx, query.Pagination)
		if err != nil {
			return nil, fmt.Errorf("could not read all contract templates: %w", err)
		}
	}

	evt := templateevents.RetrieveAllEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	reviewerTasks, err := h.RTRepo.ReadAllByReviewer(ctx, tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	approvalTasks, err := h.ATRepo.ReadAllByApprover(ctx, tx, query.RetrievedBy)
	if err != nil {
		return nil, fmt.Errorf("could not read all review tasks: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	didToMetadata := make(map[string]MetadataItem)
	var contractTemplatesItems []MetadataItem
	for _, data := range contractTemplates {

		state, err := contracttemplatestate.NewContractTemplateState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template state: %w", err)
		}

		templateType, err := contracttemplatetype.NewContractTemplateType(data.TemplateType)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template type: %w", err)
		}

		metadata := MetadataItem{
			DID:          data.DID,
			Version:      data.Version,
			State:        state,
			TemplateType: templateType,
			Name:         data.Name,
			Description:  data.Description,
			CreatedBy:    data.CreatedBy,
			CreatedAt:    data.CreatedAt,
			UpdatedAt:    data.UpdatedAt,
			LatestDID:    data.LatestDID,
		}
		contractTemplatesItems = append(contractTemplatesItems, metadata)

		didToMetadata[data.DID] = metadata
	}

	var reviewTaskItems []ReviewTaskItem
	for _, data := range reviewerTasks {

		state, err := reviewtaskstate.NewReviewTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template state: %w", err)
		}

		metadata, exists := didToMetadata[data.DID]
		var version int
		if exists {
			version = metadata.Version
		}

		reviewTaskItems = append(reviewTaskItems, ReviewTaskItem{
			DID:       data.DID,
			State:     state,
			Version:   version,
			Reviewer:  data.Reviewer,
			CreatedAt: data.CreatedAt,
		})
	}

	var approvalTasksItems []ApprovalTaskItem
	for _, data := range approvalTasks {

		state, err := approvaltaskstate.NewApprovalTaskState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template state: %w", err)
		}

		metadata, exists := didToMetadata[data.DID]
		var version int
		if exists {
			version = metadata.Version
		}

		approvalTasksItems = append(approvalTasksItems, ApprovalTaskItem{
			DID:       data.DID,
			Version:   version,
			State:     state,
			Approver:  data.Approver,
			CreatedAt: data.CreatedAt,
		})
	}

	return &GetAllMetadataResult{
		ContractTemplates: contractTemplatesItems,
		ReviewerTasks:     reviewTaskItems,
		ApprovalTasks:     approvalTasksItems,
	}, nil
}
