package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// approvedTemplateForPublish creates a contract template in the database with the given state and other parameters, ready for publishing tests.
func approvedTemplateForPublish(t *testing.T, db *sqlx.DB, repo *TestRepo, did *string, createdBy string, templateData *datatype.JSON) {
	t.Helper()

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal(*templateData, &templateDataMap); err != nil {
		t.Fatalf("unmarshal template data failed: %v", err)
	}

	name := "Test Template"
	description := "Test Description"
	createTestContractTemplateWithData(t, db, repo, did, contracttemplatestate.Approved, createdBy, name, description, templateDataMap)
}

func TestPublish_ApprovedTemplatePublishesToFCAndUpdatesState(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	participantID := getParticipantID()
	templateData := loadExampleTemplateData(t)

	did, err := base.GetDID()
	require.NoError(t, err)

	approvedTemplateForPublish(t, db, repo, did, "Test User", templateData)

	handler := command.Publisher{
		DB:       db,
		CTRepo:   repo.CTRepo,
		FCClient: fc,
	}
	err = handler.Handle(ctx, command.PublishCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		PublishedBy:   "Test User",
		ParticipantID: participantID,
	})
	require.NoError(t, err)

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	processData, err := repo.CTRepo.ReadProcessData(ctx, tx, *did)
	require.NoError(t, err)
	assert.Equal(t, contracttemplatestate.Published.String(), processData.State)

	fcQueryHandler := templatequery.GetByIDHandler{
		Ctx:      ctx,
		FCClient: fc,
	}
	fcTemplate, err := fcQueryHandler.Handle(templatequery.GetByIDQry{
		DID:     *did,
		Version: processData.Version,
	})
	require.NoError(t, err)
	require.NotNil(t, fcTemplate)
	assert.Equal(t, *did, fcTemplate.Did)
}

func TestPublish_FailsWhenTemplateNotApproved(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	did, err := base.GetDID()
	require.NoError(t, err)

	createContractTemplate(t, db, repo, did, contracttemplatestate.Draft, "Test User")

	handler := command.Publisher{
		DB:       db,
		CTRepo:   repo.CTRepo,
		FCClient: fc,
	}
	err = handler.Handle(ctx, command.PublishCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		PublishedBy:   "Test User",
		ParticipantID: getParticipantID(),
	})
	require.Error(t, err)

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	processData, err := repo.CTRepo.ReadProcessData(ctx, tx, *did)
	require.NoError(t, err)
	assert.Equal(t, contracttemplatestate.Draft.String(), processData.State)

	fcQueryHandler := templatequery.GetByIDHandler{
		Ctx:      ctx,
		FCClient: fc,
	}
	fcTemplate, err := fcQueryHandler.Handle(templatequery.GetByIDQry{
		DID:     *did,
		Version: processData.Version,
	})
	require.NoError(t, err)
	assert.Nil(t, fcTemplate)
}

func TestPublish_PublishesWhenFCTemplateAlreadyExists(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	participantID := getParticipantID()
	templateData := loadExampleTemplateData(t)

	did, err := base.GetDID()
	require.NoError(t, err)

	approvedTemplateForPublish(t, db, repo, did, "Test User", templateData)

	handler := command.Publisher{
		DB:       db,
		CTRepo:   repo.CTRepo,
		FCClient: fc,
	}
	cmd := command.PublishCmd{
		DID:           *did,
		UpdatedAt:     time.Now().UTC(),
		PublishedBy:   "Test User",
		ParticipantID: participantID,
	}

	require.NoError(t, handler.Handle(ctx, cmd))

	// Simulate local state still approved while FC already has the SD.
	updateStatement := `UPDATE contract_templates SET state = $2 WHERE did = $1`
	_, err = db.Exec(updateStatement, *did, contracttemplatestate.Approved)
	require.NoError(t, err)

	cmd.UpdatedAt = time.Now().UTC()
	require.NoError(t, handler.Handle(ctx, cmd))

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	processData, err := repo.CTRepo.ReadProcessData(ctx, tx, *did)
	require.NoError(t, err)
	assert.Equal(t, contracttemplatestate.Published.String(), processData.State)
}
