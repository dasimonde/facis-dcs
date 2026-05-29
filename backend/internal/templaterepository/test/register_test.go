package test

import (
	"context"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	database "digital-contracting-service/internal/templaterepository/db"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister_RegisterContractTemplateFromFederatedCatalogue(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	participantID := getParticipantID()
	templateData := loadExampleTemplateData(t)

	sourceDID, err := base.GetDID()
	require.NoError(t, err)

	version := 1
	documentNumber := ""
	name := "Test Template"
	seedFCTemplateResource(t, ctx, fc, participantID, *sourceDID, version, documentNumber, contracttemplatetype.SubContract.String(), name, templateData)

	newDID, err := base.GetDID()
	require.NoError(t, err)

	creator := "Test User"
	handler := command.Registrar{
		DB:       db,
		CTRepo:   repo.CTRepo,
		FCClient: fc,
	}
	err = handler.Handle(ctx, command.RegisterCmd{
		DID:          *sourceDID,
		NewDID:       *newDID,
		Version:      version,
		RegisteredBy: creator,
	})
	require.NoError(t, err)

	qry := contracttemplate.GetByIDQry{
		DID:         *newDID,
		RetrievedBy: creator,
	}
	queryHandler := contracttemplate.GetByIDHandler{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	contractTemplate, err := queryHandler.Handle(ctx, qry)
	require.NoError(t, err)

	assert.Equal(t, *newDID, contractTemplate.DID)
	assert.Equal(t, contracttemplatestate.Draft, contractTemplate.State)
	assert.Equal(t, 1, contractTemplate.Version)
	require.NotNil(t, contractTemplate.TemplateData)
}

func TestRegister_FailsWhenTemplateNotInFederatedCatalogue(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	sourceDID, err := base.GetDID()
	require.NoError(t, err)
	newDID, err := base.GetDID()
	require.NoError(t, err)

	handler := command.Registrar{
		DB:       db,
		CTRepo:   repo.CTRepo,
		FCClient: fc,
	}
	err = handler.Handle(ctx, command.RegisterCmd{
		DID:          *sourceDID,
		NewDID:       *newDID,
		Version:      1,
		RegisteredBy: "Test User",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, fcclient.ErrTemplateNotFoundInFederatedCatalogue))

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	_, err = repo.CTRepo.ReadDataByID(ctx, tx, *newDID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, database.ErrContractTemplateNotFound))

	fcQueryHandler := templatequery.GetByIDHandler{
		Ctx:      ctx,
		FCClient: fc,
	}
	fcTemplate, err := fcQueryHandler.Handle(templatequery.GetByIDQry{
		DID:     *sourceDID,
		Version: 1,
	})
	require.NoError(t, err)
	assert.Nil(t, fcTemplate)
}

func TestRegister_FailsWhenFederatedCatalogueNotConfigured(t *testing.T) {
	db := setupTestDB(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()

	sourceDID, err := base.GetDID()
	require.NoError(t, err)
	newDID, err := base.GetDID()
	require.NoError(t, err)

	handler := command.Registrar{
		DB:     db,
		CTRepo: repo.CTRepo,
	}
	err = handler.Handle(ctx, command.RegisterCmd{
		DID:          *sourceDID,
		NewDID:       *newDID,
		Version:      1,
		RegisteredBy: "Test User",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, fcclient.ErrFederatedCatalogueNotConfigured))

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	_, err = repo.CTRepo.ReadDataByID(ctx, tx, *newDID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, database.ErrContractTemplateNotFound))
}
