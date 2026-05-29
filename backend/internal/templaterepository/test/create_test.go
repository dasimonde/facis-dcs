package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreate_CreateNewContractTemplate(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	did, err := base.GetDID(datatype.TemplateResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	name := "Test Contract Template"
	description := "Test Description"

	templateDataMap := map[string]interface{}{}
	templateData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		t.Fatalf("Failed to create JSON template data: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	creator := "Test User"

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

	assert.Equal(t, *did, contractTemplate.DID)
	assert.Equal(t, name, *contractTemplate.Name)
	assert.Equal(t, description, *contractTemplate.Description)
	assert.Equal(t, templateData, *contractTemplate.TemplateData)
}
