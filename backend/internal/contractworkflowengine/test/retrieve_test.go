package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetrieve_RetrieveContractById(t *testing.T) {

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
	contractItem, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractItem.DID, *did)
	assert.Equal(t, contractstate.Draft, contractItem.State)
}

func TestRetrieve_RetrieveNonExistingContractById(t *testing.T) {

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

	did2, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}
	qry := contract.GetByIDQry{
		DID:         *did2,
		RetrievedBy: creator,
	}
	queryHandler := contract.GetByIDHandler{
		Ctx:   ctx,
		DB:    db,
		CRepo: repo.CRepo,
	}
	_, err = queryHandler.Handle(ctx, qry)

	assert.NotNil(t, err)
}

func TestRetrieve_RetrieveAllContracts(t *testing.T) {

	db := setupTestDB(t)

	cleanupContractTable(t, db)

	creator := "Test User"

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	dids := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		did, err := base.GetDID(datatype.ContractResourceType)
		if err != nil {
			t.Fatalf("Failed to get new DID: %v", err)
		}
		dids = append(dids, *did)
		createContract(t, db, repo, did, contractstate.Draft, creator)
	}
	sort.Strings(dids)

	state := contractstate.Draft
	qry := contract.GetAllMetadataByFilterQry{
		RetrievedBy: creator,
		State:       &state,
	}
	queryHandler := contract.GetAllMetaDataByFilterHandler{
		DB:    db,
		CRepo: repo.CRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	for _, ct := range result {
		assert.Equal(t, contractstate.Draft, ct.State)

		if !slices.Contains(dids, ct.DID) {
			t.Errorf("DID not found in retrieved contract: %v", ct.DID)
		}
	}
}
