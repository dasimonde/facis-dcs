package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreate_CreateNewContract(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}

	name := "Test Contract"
	description := "Test Description"

	contractData := map[string]interface{}{}
	templateData, err := datatype.NewJSON(contractData)
	if err != nil {
		t.Fatalf("Failed to create JSON data: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	creator := "Test User"

	cmd := command.CreateCmd{
		DID:          *did,
		CreatedBy:    creator,
		Name:         &name,
		Description:  &description,
		ContractData: &templateData,
	}
	createHandler := command.Creator{
		DB:    db,
		CRepo: repo.CRepo,
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create contract: %v", err)
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

	assert.Equal(t, *did, result.DID)
	assert.Equal(t, name, *result.Name)
	assert.Equal(t, description, *result.Description)
	// assert.Equal(t, jsonMetaData, *result.ContractData)
}
