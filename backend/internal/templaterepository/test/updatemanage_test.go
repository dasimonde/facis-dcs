package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpdateManage_UpdateContractTemplateDataInDraftState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID:          *did,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, name, *contractTemplate.Name)
	assert.Equal(t, description, *contractTemplate.Description)
	//assert.Equal(t, jsonTemplateData, contractTemplate.TemplateData)
}

func TestUpdateManage_UpdateContractTemplateDataInSubmitState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID:          *did,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, name, *contractTemplate.Name)
	assert.Equal(t, description, *contractTemplate.Description)
	//assert.Equal(t, jsonTemplateData, contractTemplate.TemplateData)
}

func TestUpdateManage_UpdateContractTemplateDataInRejectedState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Rejected, creator)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID:          *did,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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
		DID: *did,

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

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, name, *contractTemplate.Name)
	assert.Equal(t, description, *contractTemplate.Description)
	//assert.Equal(t, jsonTemplateData, contractTemplate.TemplateData)
}

func TestUpdateManage_UpdateContractTemplateDataInReviewedState(t *testing.T) {

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

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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
		DID: *did,

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

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, name, *contractTemplate.Name)
	assert.Equal(t, description, *contractTemplate.Description)
	//assert.Equal(t, jsonTemplateData, contractTemplate.TemplateData)
}

func TestUpdateManage_UpdateContractTemplateDataInApproveState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Approved, creator)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_UpdateContractTemplateDataInRegisteredState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Registered, creator)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_UpdateContractTemplateDataInArchiveState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deleted, creator)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_UpdateNonExistingContractTemplate(t *testing.T) {

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

	cmd := command.UpdateManageCmd{
		DID:       *did,
		UpdatedAt: time.Now(),
		UpdatedBy: "Test User 1",
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToDraft(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Draft

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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
		DID: *did,

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

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, newState, contractTemplate.State)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToSubmitted(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Submitted

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToRejected(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Rejected

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToReviewed(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Reviewed

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToApproved(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Approved

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToRegistered(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Registered

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToArchive(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Deleted

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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
		DID: *did,

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

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, newState, contractTemplate.State)
}

func TestUpdateManage_SetContractTemplateStateFromSubmittedToDraft(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)
	newState := contracttemplatestate.Draft

	reviewers := []string{"Test User 1", "Test User 2", "Test User 3"}
	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)
	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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
		DID: *did,

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

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	reviewTasksExist, err := repo.RTRepo.TaskExist(tx, cmd.DID)
	if err != nil {
		t.Fatalf("could not check existing review tasks: %v", err)
	}

	approvalTaskExists, err := repo.ATRepo.TaskExists(tx, cmd.DID)
	if err != nil {
		t.Fatalf("could not check existing approval tasks: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, newState, contractTemplate.State)
	assert.False(t, reviewTasksExist)
	assert.False(t, approvalTaskExists)
}

func TestUpdateManage_SetContractTemplateStateFromReviewedToDraft(t *testing.T) {

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
	newState := contracttemplatestate.Draft

	reviewers := []string{"Test User 1", "Test User 2", "Test User 3"}
	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)
	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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
		DID: *did,

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

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	reviewTasksExist, err := repo.RTRepo.TaskExist(tx, cmd.DID)
	if err != nil {
		t.Fatalf("could not check existing review tasks: %v", err)
	}

	approvalTaskExists, err := repo.ATRepo.TaskExists(tx, cmd.DID)
	if err != nil {
		t.Fatalf("could not check existing approval tasks: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, newState, contractTemplate.State)
	assert.False(t, reviewTasksExist)
	assert.False(t, approvalTaskExists)
}

func TestUpdateManage_SetContractTemplateStateFromApprovedToDraft(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Approved, creator)
	newState := contracttemplatestate.Draft

	reviewers := []string{"Test User 1", "Test User 2", "Test User 3"}
	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)
	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromReviewedToSubmitted(t *testing.T) {

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
	newState := contracttemplatestate.Submitted

	reviewers := []string{"Test User 1", "Test User 2", "Test User 3"}
	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Approved, creator, reviewers)
	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, "Test User 4")

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
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
		DID: *did,

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

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	reviewTasksExist, err := repo.RTRepo.ReadAllByDID(tx, cmd.DID)
	if err != nil {
		t.Fatalf("could not check existing review tasks: %v", err)
	}

	tasksAreOpen := true
	for _, reviewTask := range reviewTasksExist {
		if reviewTask.State != reviewtaskstate.Open.String() {
			tasksAreOpen = false
		}
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, newState, contractTemplate.State)
	assert.True(t, tasksAreOpen)
}

func TestUpdateManage_SetContractTemplateStateFromApprovedToSubmitted(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Approved, creator)
	newState := contracttemplatestate.Submitted

	reviewers := []string{"Test User 1", "Test User 2", "Test User 3"}
	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Approved, creator, reviewers)

	approver := "Test User 4"
	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Approved, creator, approver)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromApprovedToReviewed(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Approved, creator)
	newState := contracttemplatestate.Reviewed

	reviewers := []string{"Test User 1", "Test User 2", "Test User 3"}
	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Approved, creator, reviewers)

	approver := "Test User 4"
	createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Approved, creator, approver)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromRegisteredToDraft(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Registered, creator)
	newState := contracttemplatestate.Submitted

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromRegisteredToSubmitted(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Registered, creator)
	newState := contracttemplatestate.Submitted

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromRegisteredToApproved(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Registered, creator)
	newState := contracttemplatestate.Approved

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromRegisteredToArchived(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Registered, creator)
	newState := contracttemplatestate.Deleted

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromArchivedToDraft(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deleted, creator)
	newState := contracttemplatestate.Submitted

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromArchivedToSubmitted(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deleted, creator)
	newState := contracttemplatestate.Submitted

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromArchivedToApproved(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deleted, creator)
	newState := contracttemplatestate.Approved

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromArchivedToRegistered(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deleted, creator)
	newState := contracttemplatestate.Registered

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateManageCmd{
		DID: *did,

		State:        &newState,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{
		Ctx:    ctx,
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(cmd)

	assert.NotNil(t, err)
}
