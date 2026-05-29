package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/query"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubmit_SubmitContractTemplateInDraftState(t *testing.T) {

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

	approver := "Test User 5"
	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
		Comments:    nil,
		Reviewers: []string{
			"Test User 2",
			"Test User 3",
			"Test User 4",
		},
		Approver: &approver,
	}
	handler := command.Submitter{
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
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

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	queryReviewTasks := query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerReviewTasks := query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err := handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	for _, reviewTask := range reviewTasks {
		assert.Equal(t, reviewtaskstate.Open, reviewTask.State)

		if !slices.Contains(cmd.Reviewers, reviewTask.Reviewer) {
			t.Fatalf("Reviewer not found in review tasks: %v", reviewTask)
		}
	}
}

func TestSubmit_SubmitContractTemplateInDraftStateWithInvalidUser(t *testing.T) {

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

	approver := "Test User 5"
	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 6",
		ActionFlag:  nil,
		Comments:    nil,
		Reviewers:   reviewers,
		Approver:    &approver,
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_OneReviewerApprovedContractTemplateInSubmittedState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	verifyCmd := command.VerifyCmd{
		DID:        *did,
		VerifiedBy: reviewers[0],
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

	actionFlag := actionflag.Approval

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
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

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)
}

func TestSubmit_ApproveContractTemplateInSubmittedStateWithInvalidUser(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	actionFlag := actionflag.Approval

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 4",
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_ApproveContractTemplateInSubmittedStateWithoutVerifying(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	actionFlag := actionflag.Approval

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 1",
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_RejectContractTemplateInSubmittedStateWithInvalidUser(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	actionFlag := actionflag.Draft

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 4",
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_AllReviewersApprovedContractTemplateInSubmittedState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	/**
	All reviewers verify contract template
	*/
	for _, reviewer := range reviewers {
		verifyCmd := command.VerifyCmd{
			DID:        *did,
			VerifiedBy: reviewer,
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
	}

	actionFlag := actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
			Comments:    []string{},
		}
		handler := command.Submitter{

			DB:     db,
			CTRepo: repo.CTRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract template: %v", err)
		}
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

	assert.Equal(t, contracttemplatestate.Reviewed, contractTemplate.State)
}

func TestSubmit_OneReviewerDeclinesContractTemplateInSubmittedState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	actionFlag := actionflag.Draft

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy := "Test User"

	qry := contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler := contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Rejected, contractTemplate.State)
}

func TestSubmit_SubmitNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
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
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_SubmitContractTemplateInSubmittedStateWithoutActionFlag(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
		Comments:    []string{},
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestSubmit_SubmitContractTemplateInReviewedStateWithInvalidUser(t *testing.T) {

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

	approver := "Test User 1"

	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, approver)

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: "Test User 2",
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestSubmit_SubmitContractTemplateInSubmittedStateWithApproverUser(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

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
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.Error(t, err)
}

func TestSubmit_SubmitContractTemplateWithResubmission(t *testing.T) {

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

	/**
	Create and Submit the Draft
	*/
	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)

	approver := "Test User 4"
	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
		Comments:    nil,
		Reviewers:   reviewers,
		Approver:    &approver,
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
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

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	queryReviewTasks := query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerReviewTasks := query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err := handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(reviewTasks), 3)

	for _, reviewTask := range reviewTasks {
		assert.Equal(t, reviewtaskstate.Open, reviewTask.State)

		if !slices.Contains(cmd.Reviewers, reviewTask.Reviewer) {
			t.Fatalf("Reviewer not found in review tasks: %v", reviewTask)
		}
	}

	queryApprovalTasks := query.GetAllApprovalTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerApprovalTasks := query.GetAllApprovalTasksForDIDHandler{

		DB:     db,
		ATRepo: repo.ATRepo,
	}
	approvalTasks, err := handlerApprovalTasks.Handle(ctx, queryApprovalTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(approvalTasks), 1)
	assert.Equal(t, approvaltaskstate.Open, approvalTasks[0].State)

	/**
	First reviewer verifies contract template
	*/
	verifyCmd := command.VerifyCmd{
		DID:        *did,
		VerifiedBy: reviewers[0],
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

	/**
	First reviewer approves the contract template
	*/
	actionFlag := actionflag.Approval

	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	queryReviewTasks = query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerReviewTasks = query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err = handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(reviewTasks), 3)

	queryApprovalTasks = query.GetAllApprovalTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerApprovalTasks = query.GetAllApprovalTasksForDIDHandler{

		DB:     db,
		ATRepo: repo.ATRepo,
	}
	approvalTasks, err = handlerApprovalTasks.Handle(ctx, queryApprovalTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(approvalTasks), 1)

	/**
	Second reviewer declined the contract template
	*/
	actionFlag = actionflag.Draft

	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[2],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Rejected, contractTemplate.State)

	/**
	contract template creator submits it again
	*/
	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
		Comments:    nil,
		Approver:    &approver,
		Reviewers:   reviewers,
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	/**
	All reviewers verify contract template
	*/
	for _, reviewer := range reviewers {
		verifyCmd := command.VerifyCmd{
			DID:        *did,
			VerifiedBy: reviewer,
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
	}

	/**
	All reviewer approve the contract template
	*/
	actionFlag = actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
			Comments:    []string{},
		}
		handler := command.Submitter{

			DB:     db,
			CTRepo: repo.CTRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract template: %v", err)
		}
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Reviewed, contractTemplate.State)

	/**
	Approver resubmits reviewed contract template
	*/
	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: approver,
		ActionFlag:  nil,
		Comments:    []string{"Test Comment"},
		Reviewers:   nil,
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID: *did,

		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	/**
	All reviewers verify contract template
	*/
	for _, reviewer := range reviewers {
		verifyCmd := command.VerifyCmd{
			DID:        *did,
			VerifiedBy: reviewer,
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
	}

	/**
	All reviewer approve the contract template
	*/
	actionFlag = actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
			Comments:    []string{},
		}
		handler := command.Submitter{

			DB:     db,
			CTRepo: repo.CTRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract template: %v", err)
		}
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Reviewed, contractTemplate.State)

	/**
	Approver resubmits reviewed contract template
	*/
	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: approver,
		ActionFlag:  nil,
		Comments:    []string{"Test Comment"},
		Reviewers:   nil,
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)
}

func TestSubmit_SubmitContractTemplateWithApproving(t *testing.T) {

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

	/**
	Create and Submit the Draft
	*/
	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)

	approver := "Test User 4"
	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	cmd := command.SubmitCmd{
		DID: *did,

		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
		Comments:    nil,
		Reviewers:   reviewers,
		Approver:    &approver,
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry := contracttemplate.GetByIDQry{
		DID: *did,

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

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	queryReviewTasks := query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerReviewTasks := query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err := handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(reviewTasks), 3)

	for _, reviewTask := range reviewTasks {
		assert.Equal(t, reviewtaskstate.Open, reviewTask.State)

		if !slices.Contains(cmd.Reviewers, reviewTask.Reviewer) {
			t.Fatalf("Reviewer not found in review tasks: %v", reviewTask)
		}
	}

	queryApprovalTasks := query.GetAllApprovalTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerApprovalTasks := query.GetAllApprovalTasksForDIDHandler{

		DB:     db,
		ATRepo: repo.ATRepo,
	}
	approvalTasks, err := handlerApprovalTasks.Handle(ctx, queryApprovalTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(approvalTasks), 1)
	assert.Equal(t, approvaltaskstate.Open, approvalTasks[0].State)

	/**
	First reviewer verifies contract template
	*/
	verifyCmd := command.VerifyCmd{
		DID:        *did,
		VerifiedBy: reviewers[0],
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

	/**
	First reviewer approves the contract template
	*/
	actionFlag := actionflag.Approval

	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID: *did,

		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	queryReviewTasks = query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerReviewTasks = query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err = handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(reviewTasks), 3)

	queryApprovalTasks = query.GetAllApprovalTasksForDIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	handlerApprovalTasks = query.GetAllApprovalTasksForDIDHandler{

		DB:     db,
		ATRepo: repo.ATRepo,
	}
	approvalTasks, err = handlerApprovalTasks.Handle(ctx, queryApprovalTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(approvalTasks), 1)

	/**
	Second reviewer declined the contract template
	*/
	actionFlag = actionflag.Draft

	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[1],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Rejected, contractTemplate.State)

	/**
	contract template creator submits it again
	*/
	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: creator,
		ActionFlag:  nil,
		Comments:    nil,
		Approver:    &approver,
		Reviewers:   reviewers,
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	/**
	All reviewers verify contract template
	*/
	for _, reviewer := range reviewers {
		verifyCmd := command.VerifyCmd{
			DID:        *did,
			VerifiedBy: reviewer,
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
	}

	/**
	All reviewer approve the contract template
	*/
	actionFlag = actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
			Comments:    []string{},
		}
		handler := command.Submitter{

			DB:     db,
			CTRepo: repo.CTRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract template: %v", err)
		}
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Reviewed, contractTemplate.State)

	/**
	Approver resubmits reviewed contract template
	*/
	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: approver,
		ActionFlag:  nil,
		Comments:    []string{"Test Comment"},
		Reviewers:   nil,
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	/**
	All reviewers verify contract template
	*/
	for _, reviewer := range reviewers {
		verifyCmd := command.VerifyCmd{
			DID:        *did,
			VerifiedBy: reviewer,
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
	}

	/**
	All reviewer approve the contract template
	*/
	actionFlag = actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
			Comments:    []string{},
		}
		handler := command.Submitter{

			DB:     db,
			CTRepo: repo.CTRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract template: %v", err)
		}
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Reviewed, contractTemplate.State)

	/**
	Approver verifies reviewed contract template
	*/
	verifyCmd = command.VerifyCmd{
		DID:        *did,
		VerifiedBy: approver,
	}
	verifyHandler = command.Verifier{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
	}
	err = verifyHandler.Handle(ctx, verifyCmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	/**
	Approver approves reviewed contract template
	*/
	approveCmd := command.ApproveCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		ApprovedBy:    approver,
		DecisionNotes: []string{"Test"},
	}
	approveHandler := command.Approver{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = approveHandler.Handle(ctx, approveCmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Approved, contractTemplate.State)
}

func TestSubmit_SubmitContractTemplateWithRejecting(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	/**
	Create and Submit the Draft
	*/
	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, "Test User")

	submittedBy := "Test User"
	approver := "Test User 4"
	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: submittedBy,
		ActionFlag:  nil,
		Comments:    nil,
		Reviewers:   reviewers,
		Approver:    &approver,
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy := "Test User"

	qry := contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler := contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	queryReviewTasks := query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	handlerReviewTasks := query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err := handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(reviewTasks), 3)

	for _, reviewTask := range reviewTasks {
		assert.Equal(t, reviewtaskstate.Open, reviewTask.State)

		if !slices.Contains(cmd.Reviewers, reviewTask.Reviewer) {
			t.Fatalf("Reviewer not found in review tasks: %v", reviewTask)
		}
	}

	queryApprovalTasks := query.GetAllApprovalTasksForDIDQry{
		DID: *did,

		RetrievedBy: retrievedBy,
	}
	handlerApprovalTasks := query.GetAllApprovalTasksForDIDHandler{

		DB:     db,
		ATRepo: repo.ATRepo,
	}
	approvalTasks, err := handlerApprovalTasks.Handle(ctx, queryApprovalTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(approvalTasks), 1)
	assert.Equal(t, approvaltaskstate.Open, approvalTasks[0].State)

	/**
	First reviewer verifies contract template
	*/
	verifyCmd := command.VerifyCmd{
		DID:        *did,
		VerifiedBy: reviewers[0],
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

	/**
	First reviewer approves the contract template
	*/
	actionFlag := actionflag.Approval

	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[0],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy = "Test User"

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	queryReviewTasks = query.GetAllReviewTasksForDIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	handlerReviewTasks = query.GetAllReviewTasksForDIDHandler{

		DB:     db,
		RTRepo: repo.RTRepo,
	}
	reviewTasks, err = handlerReviewTasks.Handle(ctx, queryReviewTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(reviewTasks), 3)

	queryApprovalTasks = query.GetAllApprovalTasksForDIDQry{
		DID: *did,

		RetrievedBy: retrievedBy,
	}
	handlerApprovalTasks = query.GetAllApprovalTasksForDIDHandler{

		DB:     db,
		ATRepo: repo.ATRepo,
	}
	approvalTasks, err = handlerApprovalTasks.Handle(ctx, queryApprovalTasks)
	if err != nil {
		t.Fatalf("Failed to query template review tasks: %v", err)
	}

	assert.Equal(t, len(approvalTasks), 1)

	/**
	Second reviewer declined the contract template
	*/
	actionFlag = actionflag.Draft

	cmd = command.SubmitCmd{
		DID: *did,

		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: reviewers[1],
		ActionFlag:  &actionFlag,
		Comments:    []string{},
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy = "Test User"

	qry = contracttemplate.GetByIDQry{
		DID: *did,

		RetrievedBy: retrievedBy,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Rejected, contractTemplate.State)

	/**
	contract template creator submits it again
	*/
	approver = "Test User 4"
	reviewers = []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	submittedBy = "Test User"
	cmd = command.SubmitCmd{
		DID: *did,

		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: submittedBy,
		ActionFlag:  nil,
		Comments:    nil,
		Approver:    &approver,
		Reviewers:   reviewers,
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy = "Test User"

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	/**
	All reviewers verify contract template
	*/
	for _, reviewer := range reviewers {
		verifyCmd := command.VerifyCmd{
			DID:        *did,
			VerifiedBy: reviewer,
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
	}

	/**
	All reviewer approve the contract template
	*/
	actionFlag = actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
			Comments:    []string{},
		}
		handler := command.Submitter{

			DB:     db,
			CTRepo: repo.CTRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract template: %v", err)
		}
	}

	retrievedBy = "Test User"

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Reviewed, contractTemplate.State)

	/**
	Approver resubmits reviewed contract template
	*/
	cmd = command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().UTC(),
		SubmittedBy: approver,
		ActionFlag:  nil,
		Comments:    []string{"Test Comment"},
		Reviewers:   nil,
	}
	handler = command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy = "Test User"

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Submitted, contractTemplate.State)

	/**
	All reviewers verify contract template
	*/
	for _, reviewer := range reviewers {
		verifyCmd := command.VerifyCmd{
			DID:        *did,
			VerifiedBy: reviewer,
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
	}

	/**
	All reviewer approve the contract template
	*/
	actionFlag = actionflag.Approval

	for _, reviewer := range reviewers {
		cmd := command.SubmitCmd{
			DID:         *did,
			UpdatedAt:   time.Now().UTC(),
			SubmittedBy: reviewer,
			ActionFlag:  &actionFlag,
			Comments:    []string{},
		}
		handler := command.Submitter{

			DB:     db,
			CTRepo: repo.CTRepo,
			RTRepo: repo.RTRepo,
			ATRepo: repo.ATRepo,
		}
		err = handler.Handle(ctx, cmd)
		if err != nil {
			t.Fatalf("Failed to submit contract template: %v", err)
		}
	}

	retrievedBy = "Test User"

	qry = contracttemplate.GetByIDQry{
		DID: *did,

		RetrievedBy: retrievedBy,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Reviewed, contractTemplate.State)

	/**
	Approver rejects reviewed contract template
	*/
	rejectCmd := command.RejectCmd{
		DID:        *did,
		UpdatedAt:  time.Now().UTC(),
		RejectedBy: approver,
		Reason:     "Test",
	}
	rejectHandler := command.Rejecter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = rejectHandler.Handle(ctx, rejectCmd)
	if err != nil {
		t.Fatalf("Failed to submit contract template: %v", err)
	}

	retrievedBy = "Test User"

	qry = contracttemplate.GetByIDQry{
		DID:         *did,
		RetrievedBy: retrievedBy,
	}
	queryHandler = contracttemplate.GetByIDHandler{

		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err = queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, contracttemplatestate.Rejected, contractTemplate.State)
}

func TestSubmit_SubmitContractTemplateAfterUpdate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, "Test User")

	submittedBy := "Test User"
	approver := "Test User 5"
	cmd := command.SubmitCmd{
		DID:         *did,
		UpdatedAt:   time.Now().Add(-5 * time.Minute),
		SubmittedBy: submittedBy,
		ActionFlag:  nil,
		Comments:    nil,
		Reviewers: []string{
			"Test User 2",
			"Test User 3",
			"Test User 4",
		},
		Approver: &approver,
	}
	handler := command.Submitter{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
