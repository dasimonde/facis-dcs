package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestArchive_ArchiveContractTemplateDataInDraftState(t *testing.T) {

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

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
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

	assert.Equal(t, contractTemplate.DID, *did)
	assert.Equal(t, contracttemplatestate.Deleted, contractTemplate.State)
}

func TestArchive_ArchiveNonExistingContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.ArchiveCmd{
		DID:        *did,
		UpdatedAt:  time.Now().UTC(),
		ArchivedBy: "Test User 1",
	}
	handler := command.Archiver{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestArchive_ArchiveContractTemplateDataInSubmittedState(t *testing.T) {

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

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
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

	assert.Equal(t, contractTemplate.DID, *did)
	assert.Equal(t, contracttemplatestate.Deleted, contractTemplate.State)
}

func TestArchive_ArchiveContractTemplateDataInRejectedState(t *testing.T) {

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

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
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

	assert.Equal(t, contractTemplate.DID, *did)
	assert.Equal(t, contracttemplatestate.Deleted, contractTemplate.State)
}

func TestArchive_ArchiveContractTemplateDataInReviewedState(t *testing.T) {

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

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
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

	assert.Equal(t, contractTemplate.DID, *did)
	assert.Equal(t, contracttemplatestate.Deleted, contractTemplate.State)
}

func TestArchive_ArchiveContractTemplateDataInApprovedState(t *testing.T) {

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

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
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

	assert.Equal(t, contractTemplate.DID, *did)
	assert.Equal(t, contracttemplatestate.Deprecated, contractTemplate.State)
}

func TestArchive_ArchiveContractTemplateDataInPublishedState(t *testing.T) {

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

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
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

	assert.Equal(t, contractTemplate.DID, *did)
	assert.Equal(t, contracttemplatestate.Deprecated, contractTemplate.State)
}

func TestArchive_ArchiveContractTemplateDataInDeletedState(t *testing.T) {

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

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestArchive_ArchiveContractTemplateDataInDeprecatedState(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Deprecated, creator)

	cmd := command.ArchiveCmd{
		DID:        *did,
		ArchivedBy: creator,
		UpdatedAt:  time.Now().UTC(),
	}
	handler := command.Archiver{
		DB:     db,
		CTRepo: repo.CTRepo,
		RTRepo: repo.RTRepo,
		ATRepo: repo.ATRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
