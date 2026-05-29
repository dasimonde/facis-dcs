package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApprove_ApproveContractInReviewedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContract(t, db, repo, did, contractstate.Reviewed, creator)

	approver := "Test User 1"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    approver,
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract : %v", err)
	}

	qry := contract.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
		NRepo: repo.NRepo,
	}
	contract, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract : %v", err)
	}

	assert.Equal(t, contractstate.Approved, contract.State)
}

func TestApprove_ApproveNonExistingContract(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
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
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractInReviewedStateWithInvalidUser(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	creator := "Test User"

	createContract(t, db, repo, did, contractstate.Reviewed, creator)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 1")

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 2",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestApprove_ApproveContractInDraftState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContract(t, db, repo, did, contractstate.Draft, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractInApprovedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContract(t, db, repo, did, contractstate.Approved, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestApprove_ApproveContractAfterUpdate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContract(t, db, repo, did, contractstate.Reviewed, creator)

	cmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().Add(-5 * time.Second),
		ApprovedBy:    "Test User 1",
		DecisionNotes: []string{},
	}
	handler := command.Approver{

		DB:     db,
		CRepo:  repo.CRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
