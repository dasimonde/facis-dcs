package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/reviewtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/query"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubmit_SubmitContractInDraftState(t *testing.T) {

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

	approvers := []string{"Test User 5"}
	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
		Reviewers: []string{
			"Test User 2",
			"Test User 3",
			"Test User 4",
		},
		Negotiators: []string{
			"Test User 2",
			"Test User 3",
			"Test User 4",
		},
		Approvers: approvers,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NRepo:  repo.NRepo,
		NTRepo: repo.NTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract: %v", err)
	}

	qry := contract.GetByIDQry{
		DID: *did,

		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
		NRepo: repo.NRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractstate.Negotiation, result.State)

	queryReviewTasks := query.GetAllReviewTasksForDIDQry{
		DID: *did,

		RetrievedBy: creator,
	}
	handlerReviewTasks := query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err := handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query contract review tasks: %v", err)
	}

	for _, reviewTask := range reviewTasks {
		assert.Equal(t, reviewtaskstate.Open, reviewTask.State)

		if !slices.Contains(cmd.Reviewers, reviewTask.Reviewer) {
			t.Fatalf("Reviewer not found in review tasks: %v", reviewTask)
		}
	}
}

func TestSubmit_SubmitContractInDraftStateWithInvalidUser(t *testing.T) {

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

	approvers := []string{"Test User 5"}
	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 6",
		ActionFlag:  nil,
		Reviewers: []string{
			"Test User 2",
			"Test User 3",
			"Test User 4",
		},
		Approvers: approvers,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_SubmitContractInNegotiationState(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{"Test User 1"}
	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: negotiators[0],
		ActionFlag:  nil,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract: %v", err)
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
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractstate.Submitted, result.State)
	assert.Equal(t, result.ContractVersion, 1)
}

func TestSubmit_SubmitContractInNegotiationStateWithOpenNegotiations(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  negotiators[0],
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create negotiation: %v", err)
	}

	submitCmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: negotiators[0],
	}
	submitHandler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = submitHandler.Handle(ctx, submitCmd)

	assert.NotNil(t, err)
}

func TestSubmit_SubmitContractInNegotiationStateWithRejectedNegotiations(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  negotiators[0],
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create negotiation: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	negotiations, err := repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	if err != nil {
		t.Fatalf("Failed to read all negotiations for did: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	rejectionReason := "RejectionReason"
	rejectionCmd := command.RejectNegotiationCmd{
		DID:             *did,
		ID:              negotiations[0].ID,
		RejectionReason: &rejectionReason,
		RejectedBy:      negotiations[0].Negotiator,
	}
	rejectionHandler := command.NegotiationRejector{

		DB:     db,
		CRepo:  repo.CRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = rejectionHandler.Handle(ctx, rejectionCmd)
	if err != nil {
		t.Fatalf("Failed to reject negotiation: %v", err)
	}

	submitCmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: negotiators[0],
	}
	submitHandler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = submitHandler.Handle(ctx, submitCmd)
	if err != nil {
		t.Fatalf("Failed to submit contract: %v", err)
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
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractstate.Submitted, result.State)
}

func TestSubmit_SubmitContractInNegationStateWithInvalidUser(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	approvers := []string{"Test User 5"}
	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 6",
		ActionFlag:  nil,
		Reviewers: []string{
			"Test User 2",
			"Test User 3",
			"Test User 4",
		},
		Approvers: approvers,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_SubmitContractInReviewedStateWithVerifying(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	actionFlag := actionflag.Approval

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract: %v", err)
	}

	qry := contract.GetByIDQry{
		DID: *did,

		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
		NRepo: repo.NRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractstate.Submitted, result.State)
}

func TestSubmit_OneReviewerApprovedContractInSubmittedState(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	actionFlag := actionflag.Approval

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract: %v", err)
	}

	qry := contract.GetByIDQry{
		DID: *did,

		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
		NRepo: repo.NRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractstate.Submitted, result.State)
}

func TestSubmit_ApproveContractInSubmittedStateWithInvalidUser(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	actionFlag := actionflag.Approval

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 4",
		ActionFlag:  &actionFlag,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_RejectContractInSubmittedStateWithInvalidUser(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	actionFlag := actionflag.Reject

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 4",
		ActionFlag:  &actionFlag,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_AllReviewersApprovedContractInSubmittedState(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	actionFlag := actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
		}
		handler := command.Submitter{

			DB:     db,
			CRepo:  repo.CRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract: %v", err)
		}
	}

	qry := contract.GetByIDQry{
		DID: *did,

		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
		NRepo: repo.NRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractstate.Reviewed, result.State)
}

func TestSubmit_OneReviewerDeclinesContractInSubmittedState(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	actionFlag := actionflag.Reject

	cmd := command.SubmitCmd{
		DID: *did,

		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract: %v", err)
	}

	retrievedBy := "Test User"

	qry := contract.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
		NRepo: repo.NRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractstate.Negotiation, result.State)
}

func TestSubmit_SubmitNonExistingContract(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 1",
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_SubmitContractInSubmittedStateWithoutActionFlag(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_SubmitContractInReviewedStateWithInvalidUser(t *testing.T) {

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

	approver := "Test User 1"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 2",
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestSubmit_SubmitContractInSubmittedStateWithApproverUser(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	approver := "Test User 4"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	aFlag := actionflag.Approval
	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: approver,
		ActionFlag:  &aFlag,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestSubmit_SubmitContractReviewedState(t *testing.T) {

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

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	approver := "Test User 4"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	aFlag := actionflag.Approval
	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: approver,
		ActionFlag:  &aFlag,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestSubmit_SubmitContractTemplateAfterUpdate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContract(t, db, repo, did, contractstate.Draft, "Test User")

	submittedBy := "Test User"
	approvers := []string{"Test User 5"}
	cmd := command.SubmitCmd{
		DID: *did,

		UpdatedAt:   time.Now().Add(-5 * time.Minute),
		SubmittedBy: submittedBy,
		ActionFlag:  nil,
		Reviewers: []string{
			"Test User 2",
			"Test User 3",
			"Test User 4",
		},
		Approvers: approvers,
	}
	handler := command.Submitter{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
		NTRepo: repo.NTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
