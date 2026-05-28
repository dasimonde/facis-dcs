package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpdate_UpdateContractTemplateDataInDraftState(t *testing.T) {

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

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to update contract template: %v", err)
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

func TestUpdate_UpdateNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.UpdateCmd{
		DID:       *did,
		UpdatedAt: time.Now().UTC(),
		UpdatedBy: "Test User 1",
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateDataInDraftStateWithInvalidUser(t *testing.T) {

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

	reviewers := []string{"Test User 2"}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateCmd{
		DID:          *did,
		UpdatedBy:    "Test User 1",
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateDataInSubmittedStateAsCreator(t *testing.T) {

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

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateDataInSubmittedStateAsReviewer(t *testing.T) {

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

	reviewers := []string{"Test User 2"}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    reviewers[0],
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to update contract template: %v", err)
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

func TestUpdate_UpdateContractTemplateDataInSubmittedStateWithInvalidUser(t *testing.T) {

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

	reviewers := []string{"Test User 2"}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    "Test User 1",
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateDataInDraftApprovedState(t *testing.T) {

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

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateDataInDraftPublishedState(t *testing.T) {

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

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateDataInDraftArchivedState(t *testing.T) {

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

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateDataInDraftApprovedStateWithInvalidUser(t *testing.T) {

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

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    "Test User 1",
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateAfterUpdate(t *testing.T) {

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

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    creator,
		UpdatedAt:    time.Now().Add(-5 * time.Second),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractTemplateAndReopenTasks(t *testing.T) {

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

	reviewers := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Approved, creator, reviewers)

	templateData := map[string]interface{}{
		"test": "update",
	}
	jsonTemplateData, err := datatype.NewJSON(templateData)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	name := "Updated Contract Template"
	description := "Updated Description"

	cmd := command.UpdateCmd{
		DID: *did,

		UpdatedBy:    reviewers[1],
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		TemplateData: &jsonTemplateData,
	}
	handler := command.Updater{

		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to update template: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	exists, err := repo.RTRepo.AnyTasksInState(ctx, tx, *did, contracttemplatestate.Approved.String(), reviewtaskstate.Verified.String(), contracttemplatestate.Rejected.String())
	if err != nil {
		t.Fatalf("Failed to check existence of review tasks: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatal("could not commit transaction: %w", err)
	}

	assert.False(t, exists)
}
