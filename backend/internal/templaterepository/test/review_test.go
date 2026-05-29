package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	db2 "digital-contracting-service/internal/templaterepository/db"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReview_CreateReviewTasks(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	assignees := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	for _, assignee := range assignees {
		reviewTask := db2.ReviewTaskData{
			DID:       *did,
			Reviewer:  assignee,
			State:     reviewtaskstate.Open.String(),
			CreatedBy: creator,
		}
		_, err = repo.RTRepo.Create(ctx, tx, reviewTask)
		if err != nil {
			t.Fatalf("Failed to create review task: %v", err)
		}
	}

	exists, err := repo.RTRepo.AnyTasksInState(ctx, tx, *did, reviewtaskstate.Open.String())
	if err != nil {
		t.Fatalf("Failed to check if review task exists: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.True(t, exists)
}

func TestReview_CreateReviewTasksAndApproveThem(t *testing.T) {

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

	createContractTemplate(t, db, repo, did, contracttemplatestate.Submitted, creator)

	assignees := []string{
		"Test User 1",
		"Test User 2",
		"Test User 3",
	}

	tx, err := db.BeginTxx(ctx, nil)
	defer tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	for _, assignee := range assignees {
		reviewTask := db2.ReviewTaskData{
			DID:       *did,
			Reviewer:  assignee,
			State:     reviewtaskstate.Open.String(),
			CreatedBy: creator,
		}
		_, err = repo.RTRepo.Create(ctx, tx, reviewTask)
		if err != nil {
			t.Fatalf("Failed to create review task: %v", err)
		}
	}

	for _, assignee := range assignees {
		err := repo.RTRepo.UpdateState(ctx, tx, *did, assignee, contracttemplatestate.Approved.String())
		if err != nil {
			t.Fatalf("Failed to approve review task: %v", err)
		}
	}

	exists, err := repo.RTRepo.AnyTasksInState(ctx, tx, *did, reviewtaskstate.Open.String())
	if err != nil {
		t.Fatalf("Failed to check if review task exists: %v", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	assert.False(t, exists)
}
