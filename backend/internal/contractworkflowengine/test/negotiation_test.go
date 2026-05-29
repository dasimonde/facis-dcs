package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationaction"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNegotiation_CreateNegotiation(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  negotiators[0],
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create negotiation: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	result, err := repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	if err != nil {
		t.Fatalf("Failed to read all negotiations for did: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, len(result), 3)
}

func TestNegotiation_CreateNegotiationWithInvalidUser(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  "Test User",
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)

	assert.NotNil(t, err)
}

func TestNegotiation_AllNegotiatorsAcceptChangeRequest(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  negotiators[0],
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create negotiation: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	negotiations, err := repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	if err != nil {
		t.Fatalf("Failed to read all negotiations for did: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	for _, negotiation := range negotiations {
		acceptCmd := command.AcceptNegotiationCmd{
			DID:        *did,
			ID:         negotiation.ID,
			AcceptedBy: negotiation.Negotiator,
		}
		acceptHandler := command.NegotiationAcceptor{

			DB:     db,
			CRepo:  repo.CRepo,
			NTRepo: repo.NTRepo,
			NRepo:  repo.NRepo,
		}
		err := acceptHandler.Handle(ctx, acceptCmd)
		if err != nil {
			t.Fatalf("Failed to accept negotiation: %v", err)
		}
	}

	tx, err = db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	acceptAmount := 0
	rejectAmount := 0
	closeAmount := 0
	negotiations, err = repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	for _, negotiation := range negotiations {
		if *negotiation.Decision == negotiationdescision.Accepted.String() {
			acceptAmount++
		} else if *negotiation.Decision == negotiationdescision.Rejected.String() {
			rejectAmount++
		} else if *negotiation.Decision == negotiationdescision.Closed.String() {
			closeAmount++
		}
	}
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, acceptAmount, 3)
	assert.Equal(t, rejectAmount, 0)
	assert.Equal(t, closeAmount, 0)
}

func TestNegotiation_OneNegotiatorRejectChangeRequest(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  negotiators[0],
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create negotiation: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	negotiations, err := repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	if err != nil {
		t.Fatalf("Failed to read all negotiations for did: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	rejectionReason := "RejectionReason"
	rejectionCmd := command.RejectNegotiationCmd{
		DID:             *did,
		ID:              negotiations[0].ID,
		RejectionReason: &rejectionReason,
		RejectedBy:      negotiations[0].Negotiator,
	}
	rejectionHandler := command.NegotiationRejector{

		DB:     db,
		CRepo:  repo.CRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = rejectionHandler.Handle(ctx, rejectionCmd)
	if err != nil {
		t.Fatalf("Failed to reject negotiation: %v", err)
	}

	tx, err = db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	acceptAmount := 0
	rejectAmount := 0
	closeAmount := 0
	negotiations, err = repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	for _, negotiation := range negotiations {
		if *negotiation.Decision == negotiationdescision.Accepted.String() {
			acceptAmount++
		} else if *negotiation.Decision == negotiationdescision.Rejected.String() {
			rejectAmount++
		} else if *negotiation.Decision == negotiationdescision.Closed.String() {
			closeAmount++
		}
	}
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, acceptAmount, 0)
	assert.Equal(t, rejectAmount, 1)
	assert.Equal(t, closeAmount, 2)
}

func TestNegotiation_OneAcceptionOneRejectionOfChangeRequest(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  negotiators[0],
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create negotiation: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	negotiations, err := repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	if err != nil {
		t.Fatalf("Failed to read all negotiations for did: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	acceptCmd := command.AcceptNegotiationCmd{
		DID:        *did,
		ID:         negotiations[0].ID,
		AcceptedBy: negotiations[0].Negotiator,
	}
	acceptHandler := command.NegotiationAcceptor{

		DB:     db,
		CRepo:  repo.CRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = acceptHandler.Handle(ctx, acceptCmd)
	if err != nil {
		t.Fatalf("Failed to accept negotiation: %v", err)
	}

	rejectionReason := "RejectionReason"
	rejectionCmd := command.RejectNegotiationCmd{
		DID:             *did,
		ID:              negotiations[1].ID,
		RejectionReason: &rejectionReason,
		RejectedBy:      negotiations[1].Negotiator,
	}
	rejectionHandler := command.NegotiationRejector{

		DB:     db,
		CRepo:  repo.CRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = rejectionHandler.Handle(ctx, rejectionCmd)
	if err != nil {
		t.Fatalf("Failed to reject negotiation: %v", err)
	}

	tx, err = db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	acceptAmount := 0
	rejectAmount := 0
	closeAmount := 0
	negotiations, err = repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	for _, negotiation := range negotiations {
		if *negotiation.Decision == negotiationdescision.Accepted.String() {
			acceptAmount++
		} else if *negotiation.Decision == negotiationdescision.Rejected.String() {
			rejectAmount++
		} else if *negotiation.Decision == negotiationdescision.Closed.String() {
			closeAmount++
		}
	}
	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, acceptAmount, 1)
	assert.Equal(t, rejectAmount, 1)
	assert.Equal(t, closeAmount, 1)
}

func TestNegotiation_TestForOpenNegotiationDecisions(t *testing.T) {

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

	createContract(t, db, repo, did, contractstate.Negotiation, creator)

	negotiators := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	createNegotiationTasks(t, ctx, db, repo, *did, negotiationtaskstate.Open, creator, negotiators)

	var changeRequest map[string]interface{}
	jsonChangeRequest, err := datatype.NewJSON(changeRequest)
	cmd := command.NegotiationCmd{
		DID:           *did,
		NegotiatedBy:  negotiators[0],
		ChangeRequest: &jsonChangeRequest,
		UpdatedAt:     time.Now().UTC(),
	}
	handler := command.Negotiator{

		DB:     db,
		CRepo:  repo.CRepo,
		RTRepo: repo.RTRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = handler.Handle(ctx, cmd)
	if err != nil {
		t.Fatalf("Failed to create negotiation: %v", err)
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	negotiations, err := repo.NRepo.ReadAllByContractDID(ctx, tx, *did)
	if err != nil {
		t.Fatalf("Failed to read all negotiations for did: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	acceptCmd := command.AcceptNegotiationCmd{
		DID:        *did,
		ID:         negotiations[0].ID,
		AcceptedBy: negotiations[0].Negotiator,
	}
	acceptHandler := command.NegotiationAcceptor{
		DB:     db,
		CRepo:  repo.CRepo,
		NTRepo: repo.NTRepo,
		NRepo:  repo.NRepo,
	}
	err = acceptHandler.Handle(ctx, acceptCmd)
	if err != nil {
		t.Fatalf("Failed to accept negotiation: %v", err)
	}

	tx, err = db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	hasOpenNegotiationDecisions, err := repo.NRepo.HasOpenNegotiationDecisions(ctx, tx, *did, 1, negotiations[0].Negotiator)
	if err != nil {
		t.Fatalf("Failed to check for open negotiation decisions %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.Equal(t, hasOpenNegotiationDecisions, true)
}
