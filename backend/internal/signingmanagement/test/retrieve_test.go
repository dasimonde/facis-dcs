package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/signingmanagement/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/query"
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

	createContract(t, db, repo, did, contractstate.Approved, creator)

	qry := query.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler := query.GetByIDHandler{
		DB:    db,
		CRepo: repo.CRepo,
	}
	contractItem, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, contractItem.DID, *did)
	assert.Equal(t, contractstate.Approved, contractItem.State)
}

func TestRetrieve_RetrieveContractByIdInInvalidState(t *testing.T) {

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

	qry := query.GetByIDQry{
		DID:         *did,
		RetrievedBy: creator,
	}
	queryHandler := query.GetByIDHandler{
		DB:    db,
		CRepo: repo.CRepo,
	}
	_, err = queryHandler.Handle(ctx, qry)

	assert.NotNil(t, err)
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

	createContract(t, db, repo, did, contractstate.Approved, creator)

	did2, err := base.GetDID(datatype.ContractResourceType)
	if err != nil {
		t.Fatalf("Failed to get new DID: %v", err)
	}
	qry := query.GetByIDQry{
		DID:         *did2,
		RetrievedBy: creator,
	}
	queryHandler := query.GetByIDHandler{
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

	tmpCtx := context.Background()
	ctx, cancel := context.WithTimeout(tmpCtx, conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	dids := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		did, err := base.GetDID(datatype.ContractResourceType)
		if err != nil {
			t.Fatalf("Failed to get new DID: %v", err)
		}
		dids = append(dids, *did)

		if i%2 == 0 {
			createContract(t, db, repo, did, contractstate.Reviewed, creator)
		} else {
			createContract(t, db, repo, did, contractstate.Approved, creator)
		}
	}
	sort.Strings(dids)

	qry := query.GetAllMetadataQry{
		RetrievedBy: creator,
	}
	queryHandler := query.GetAllMetadataHandler{
		DB:    db,
		CRepo: repo.CRepo,
	}
	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		t.Fatalf("Failed to query contract: %v", err)
	}

	assert.Equal(t, 5, len(result.Contracts))

	for _, ct := range result.Contracts {
		assert.Equal(t, contractstate.Approved, ct.State)

		if !slices.Contains(dids, ct.DID) {
			t.Errorf("DID not found in retrieved contract: %v", ct.DID)
		}
	}
}
