package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearch_SearchContractTemplatesWithoutSearchValue(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	templateData := map[string]interface{}{}

	did, _ := base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "Test1", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "Test1", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "Test1", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "Test1", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "Test1", "Test1", templateData)

	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy: creator,
	}
	queryHandler := contracttemplate.GetAllMetaDataByFilterHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, 5, len(contractTemplate))
}

func TestSearch_SearchContractTemplatesByDID(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID()
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	name := "Test Contract Template"
	description := "Test Description"

	templateDataMap := map[string]interface{}{}
	templateData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		t.Fatalf("Failed to create JSON data: %v", err)
	}

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	cmd := command.CreateCmd{
		DID:          *did,
		CreatedBy:    creator,
		TemplateType: contracttemplatetype.FrameContract,
		Name:         &name,
		Description:  &description,
		TemplateData: &templateData,
	}
	createHandler := command.Creator{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create contract template: %v", err)
	}

	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy: creator,
		DID:         *did,
	}
	queryHandler := contracttemplate.GetAllMetaDataByFilterHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, 1, len(contractTemplate))
}

func TestSearch_SearchContractTemplatesByName(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	templateData := map[string]interface{}{}

	did, _ := base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 1 --", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 1.2 --", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 1.3 --", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 2 --", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 2.2 --", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 2.3 --", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 3 --", "Test1", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 3.2 --", "Test1", templateData)

	searchName := "Test 2." // The search is case-insensitive
	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy: creator,
		Name:        searchName,
	}
	queryHandler := contracttemplate.GetAllMetaDataByFilterHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, 2, len(contractTemplate))
}

func TestSearch_SearchContractTemplatesByDescript(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	templateData := map[string]interface{}{}

	did, _ := base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 1 --", "a long test1 description", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 1.2 --", "a long test2 description", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 1.3 --", "a long test2.2 description", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 2 --", "a long test2.3 description", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 2.2 --", "a long test3 description", templateData)

	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test 2.3 --", "a long test4 description", templateData)

	searchDescription := "Test2." // The search is case-insensitive
	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy: creator,
		Description: searchDescription,
	}
	queryHandler := contracttemplate.GetAllMetaDataByFilterHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, 2, len(contractTemplate))
}

func TestSearch_SearchContractTemplatesByTemplateData(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	templateData := map[string]interface{}{
		"name":        "-- test1 --",
		"description": "a long test1 description",
	}
	did, _ := base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test1 --", "a long test1 description", templateData)

	templateData = map[string]interface{}{
		"name":        "-- test1.2 --",
		"description": "a long test2 description",
	}
	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test1.2 --", "a long test2 description", templateData)

	templateData = map[string]interface{}{
		"name":        "-- test1.3 --",
		"description": "a long test2.2 description",
	}
	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test1.3 --", "a long test2.2 description", templateData)

	templateData = map[string]interface{}{
		"name":        "-- test2 --",
		"description": "a long test2.3 description",
	}
	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test2 --", "a long test2.3 description", templateData)

	templateData = map[string]interface{}{
		"name":        "-- test2.2 --",
		"description": "a long test3 description",
	}
	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test2.2 --", "a long test3 description", templateData)

	templateData = map[string]interface{}{
		"name":        "-- test2.3 --",
		"description": "a long test4 description",
	}
	did, _ = base.GetDID()
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Reviewed, creator, "-- test2.3 --", "a long test4 description", templateData)

	templateDataFilter := "Test2.2" // The search is case-insensitive
	qry := contracttemplate.GetAllMetadataByFilterQry{
		RetrievedBy:  creator,
		TemplateData: templateDataFilter,
	}
	queryHandler := contracttemplate.GetAllMetaDataByFilterHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract template: %v", err)
	}

	assert.Equal(t, 2, len(contractTemplate))
}
