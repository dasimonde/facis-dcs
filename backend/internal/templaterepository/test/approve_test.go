package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApprove_ApproveContractTemplateInReviewedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Reviewed, creator)

	approver := "Test User 1"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	verifyCmd := command.VerifyCmd{
		DID:        *did,
		VerifiedBy: approver,
	}
	verifyHandler := command.Verifier{
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
	}
	err = verifyHandler.Handle(ctx, verifyCmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    approver,
		DecisionNotes: []string{},
	}
	handler := command.Approver{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry := contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler := contracttemplate.GetByIDHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Approved, contractTemplate.State)
}

func TestApprove_ApproveNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractTemplateInReviewedStateWithInvalidUser(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	creator := "Test User"

	createContractTemplate(t, db, repo, did, contracttemplatestate.Reviewed, creator)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 1")

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 2",
		DecisionNotes: []string{},
	}
	handler := command.Approver{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestApprove_ApproveContractTemplateInDraftState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractTemplateInApprovedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Approved, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractTemplateAfterUpdate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Reviewed, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().Add(-5 * time.Second),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
