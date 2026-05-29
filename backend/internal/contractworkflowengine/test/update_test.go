package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpdate_UpdateContractDataInDraftState(t *testing.T) {

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

	contractData := map[string]interface{}{
		"test": "update",
	}
	jsonContractData, err := datatype.NewJSON(contractData)
	if err != nil {
		t.Fatalf("Failed to create JSON  data: %v", err)
	}

	name := "Updated Contract"
	description := "Updated Description"

	cmd := command.UpdateCmd{
		DID:          *did,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		ContractData: &jsonContractData,
	}
	handler := command.Updater{
		DB:    db,
		CRepo: repo.CRepo,
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

	assert.Equal(t, *did, result.DID)
	assert.Equal(t, name, *result.Name)
	assert.Equal(t, description, *result.Description)
	//assert.Equal(t, jsonContractData, result.ContractData)
}

func TestUpdate_UpdateNonExistingContract(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	did, err := base.GetDID(datatype.ContractResourceType)
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
		DB:    db,
		CRepo: repo.CRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractDataInDraftStateWithInvalidUser(t *testing.T) {

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

	contractData := map[string]interface{}{
		"test": "update",
	}
	jsonContractData, err := datatype.NewJSON(contractData)
	if err != nil {
		t.Fatalf("Failed to create JSON data: %v", err)
	}

	name := "Updated Contract"
	description := "Updated Description"

	cmd := command.UpdateCmd{
		DID:          *did,
		UpdatedBy:    "Test User 1",
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		ContractData: &jsonContractData,
	}
	handler := command.Updater{
		DB:    db,
		CRepo: repo.CRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestUpdate_UpdateContractDataInInvalidState(t *testing.T) {

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

	contractData := map[string]interface{}{
		"test": "update",
	}
	jsonContractData, err := datatype.NewJSON(contractData)
	if err != nil {
		t.Fatalf("Failed to create JSON data: %v", err)
	}

	name := "Updated Contract"
	description := "Updated Description"

	cmd := command.UpdateCmd{
		DID:          *did,
		UpdatedBy:    creator,
		UpdatedAt:    time.Now().UTC(),
		Name:         &name,
		Description:  &description,
		ContractData: &jsonContractData,
	}
	handler := command.Updater{
		DB:    db,
		CRepo: repo.CRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}
