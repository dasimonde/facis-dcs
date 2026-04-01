package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCreate_RejectContractTemplateInReviewedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	tmpCtx := context.Background()
	ctx, cancel := context.WithTimeout(tmpCtx, conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo(ctx)

	createContractTemplate(t, db, repo, did, contracttemplatestate.Reviewed, creator)

	approver := "Test User 1"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	cmd := command.RejectCmd{
		DID:        *did,
		UpdatedAt:  time.Now(),
		RejectedBy: approver,
		Reason:     "Test Reason",
	}
	handler := command.Rejecter{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy := "Test User"

	qry := contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler := contracttemplate.GetByIDHandler{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Rejected, contractTemplate.State)
}

func TestCreate_RejectContractTemplateInReviewedStateWithInvalidUser(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	tmpCtx := context.Background()
	ctx, cancel := context.WithTimeout(tmpCtx, conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo(ctx)

	createContractTemplate(t, db, repo, did, contracttemplatestate.Reviewed, creator)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 1")

	cmd := command.RejectCmd{
		DID:        *did,
		UpdatedAt:  time.Now(),
		RejectedBy: "Test User 2",
		Reason:     "Test Reason",
	}
	handler := command.Rejecter{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestCreate_RejectNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	tmpCtx := context.Background()
	ctx, cancel := context.WithTimeout(tmpCtx, conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo(ctx)

	cmd := command.RejectCmd{
		DID:        *did,
		UpdatedAt:  time.Now(),
		RejectedBy: "Test User 1",
	}
	handler := command.Rejecter{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestCreate_RejectContractTemplateInDraftState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	tmpCtx := context.Background()
	ctx, cancel := context.WithTimeout(tmpCtx, conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo(ctx)

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, "Test User")

	rejectedBy := "Test User"

	cmd := command.RejectCmd{
		DID:        *did,
		UpdatedAt:  time.Now(),
		RejectedBy: rejectedBy,
		Reason:     "Test Reason",
	}
	handler := command.Rejecter{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestCreate_RejectContractTemplateInApprovedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	tmpCtx := context.Background()
	ctx, cancel := context.WithTimeout(tmpCtx, conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo(ctx)

	createContractTemplate(t, db, repo, did, contracttemplatestate.Approved, "Test User")

	rejectedBy := "Test User"

	cmd := command.RejectCmd{
		DID:        *did,
		UpdatedAt:  time.Now(),
		RejectedBy: rejectedBy,
		Reason:     "Test Reason",
	}
	handler := command.Rejecter{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestCreate_RejectContractTemplateAfterUpdate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	tmpCtx := context.Background()
	ctx, cancel := context.WithTimeout(tmpCtx, conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo(ctx)

	createContractTemplate(t, db, repo, did, contracttemplatestate.Reviewed, "Test User")

	rejectedBy := "Test User"

	cmd := command.RejectCmd{
		DID:        *did,
		UpdatedAt:  time.Now().Add(-5 * time.Second),
		RejectedBy: rejectedBy,
		Reason:     "Test Reason",
	}
	handler := command.Rejecter{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}
