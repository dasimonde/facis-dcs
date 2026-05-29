package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetrieve_RetrieveContractTemplateById(t *testing.T) {

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
	assert.Equal(t, contracttemplatestate.Draft, contractTemplate.State)
}

func TestRetrieve_RetrieveNonExistingContractTemplateById(t *testing.T) {

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

	did2, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get another DID: %v", err)
	}

	qry := contracttemplate.GetByIDQry{
		DID:         *did2,
		RetrievedBy: creator,
	}
	queryHandler := contracttemplate.GetByIDHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	_, err = queryHandler.Handle(ctx, qry)

	assert.NotNil(t, err)
}

func TestRetrieve_RetrieveAllContractTemplates(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	dids := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		did, err := base.GetDID(datatype.TemplateResourceType)
		if err != nil {
			t.Fatalf("Failed to get new DID: %v", err)
		}
		dids = append(dids, *did)
		createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, creator)

		reviewers := []string{
			creator,
		}

		createReviewTasks(t, ctx, db, repo, *did, reviewtaskstate.Open, creator, reviewers)

		createApprovalTasks(t, ctx, db, repo, *did, approvaltaskstate.Open, creator, creator)

	}
	sort.Strings(dids)

	qry := contracttemplate.GetAllMetadataQry{
		RetrievedBy: creator,
	}
	queryHandler := contracttemplate.GetAllMetadataHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
		ATRepo: repo.ATRepo,
		RTRepo: repo.RTRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.NotEmpty(t, result.ContractTemplates)
	assert.NotEmpty(t, result.ReviewerTasks)
	assert.NotEmpty(t, result.ApprovalTasks)

	for _, ct := range result.ContractTemplates {
		assert.Equal(t, contracttemplatestate.Draft, ct.State)

		if !slices.Contains(dids, ct.DID) {
			t.Errorf("DID not found in retrieved contract template: %v", ct.DID)
		}
	}
}
