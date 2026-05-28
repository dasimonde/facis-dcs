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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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
		DID: *did,

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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
		DID: *did,

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_UpdateContractTemplateDataInPublishedState(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Published, creator)

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_UpdateNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.UpdateManageCmd{
		DID:       *did,
		UpdatedAt: time.Now().UTC(),
		UpdatedBy: "Test User 1",
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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
		DID: *did,

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromDraftToPublished(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)
	newState := contracttemplatestate.Published

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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
		DID: *did,

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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
		DID: *did,

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

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	reviewTasksExist, err := repo.RTRepo.TaskExist(ctx, tx, cmd.DID)
	if err != nil {
		t.Fatalf("could not check existing review tasks: %v", err)
	}

	approvalTaskExists, err := repo.ATRepo.TaskExists(ctx, tx, cmd.DID)
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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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
		DID: *did,

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

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	reviewTasksExist, err := repo.RTRepo.TaskExist(ctx, tx, cmd.DID)
	if err != nil {
		t.Fatalf("could not check existing review tasks: %v", err)
	}

	approvalTaskExists, err := repo.ATRepo.TaskExists(ctx, tx, cmd.DID)
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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

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
		DID: *did,

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

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	reviewTasksExist, err := repo.RTRepo.ReadAllByDID(ctx, tx, cmd.DID)
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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromPublishedToDraft(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Published, creator)
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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromPublishedToSubmitted(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Published, creator)
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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromPublishedToApproved(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Published, creator)
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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromPublishedToArchived(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Published, creator)
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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

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

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdateManage_SetContractTemplateStateFromArchivedToPublished(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deleted, creator)
	newState := contracttemplatestate.Published

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
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.UpdateManager{

		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
