package test

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/testutil"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func searchFederatedCatalogue(t *testing.T, ctx context.Context, fc *fcclient.FederatedCatalogueClient, qry templatequery.SearchQry) *templatecatalogueintegration.TemplateCatalogueRetrieveResponse {
	t.Helper()

	searchHandler := templatequery.SearchHandler{
		Ctx:      ctx,
		FCClient: fc,
	}
	result, err := searchHandler.Handle(qry)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func catalogueItemByDID(items []*templatecatalogueintegration.TemplateCatalogueItem, did string) *templatecatalogueintegration.TemplateCatalogueItem {
	for _, item := range items {
		if item != nil && item.Did == did {
			return item
		}
	}
	return nil
}

func assertCatalogueItemMatchesSeed(t *testing.T, item *templatecatalogueintegration.TemplateCatalogueItem, seed testutil.TemplateSeed, expectedName string) {
	t.Helper()

	require.NotNil(t, item)
	assert.Equal(t, seed.DID, item.Did)
	require.NotNil(t, item.Version)
	assert.Equal(t, seed.Version, *item.Version)
	if seed.DocumentNumber != "" {
		require.NotNil(t, item.DocumentNumber)
		assert.Equal(t, seed.DocumentNumber, *item.DocumentNumber)
	}
	if expectedName != "" {
		require.NotNil(t, item.Name)
		assert.Equal(t, expectedName, *item.Name)
	}
}

func publishApprovedTemplateToFC(t *testing.T, ctx context.Context, db *sqlx.DB, fc *fcclient.FederatedCatalogueClient, repo *TestRepo, did string, name string, templateData *datatype.JSON) {
	t.Helper()

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal(*templateData, &templateDataMap); err != nil {
		t.Fatalf("unmarshal template data failed: %v", err)
	}

	const createdBy = "Test User"
	const description = "Test Description"
	createTestContractTemplateWithData(t, db, repo, &did, contracttemplatestate.Approved, createdBy, name, description, templateDataMap)

	participantID := getParticipantID()
	publishHandler := command.Publisher{
		DB:       db,
		CTRepo:   repo.CTRepo,
		FCClient: fc,
	}
	err := publishHandler.Handle(ctx, command.PublishCmd{
		DID:           did,
		UpdatedAt:     time.Now().UTC(),
		PublishedBy:   "Test User",
		ParticipantID: participantID,
	})
	require.NoError(t, err)
}

func TestSearch_ReturnsTemplatesPostedToFederatedCatalogue(t *testing.T) {
	fc := setupTestFC(t)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	participantID := getParticipantID()
	templateData := loadExampleTemplateData(t)

	didOne, err := base.GetDID(datatype.TemplateResourceType)
	require.NoError(t, err)
	didTwo, err := base.GetDID(datatype.TemplateResourceType)
	require.NoError(t, err)

	const nameOne = "Catalogue Search Template One"
	const nameTwo = "Catalogue Search Template Two"

	seedOne := seedFCTemplateResource(t, ctx, fc, participantID, *didOne, 1, "document-number-a", contracttemplatetype.FrameContract.String(), nameOne, templateData)
	seedTwo := seedFCTemplateResource(t, ctx, fc, participantID, *didTwo, 1, "document-number-b", contracttemplatetype.FrameContract.String(), nameTwo, templateData)

	result := searchFederatedCatalogue(t, ctx, fc, templatequery.SearchQry{
		Offset: 0,
		Limit:  0, // set to 0 to retrieve all results
	})

	assert.Equal(t, 2, result.TotalCount)
	require.Len(t, result.Items, 2)

	itemOne := catalogueItemByDID(result.Items, seedOne.DID)
	itemTwo := catalogueItemByDID(result.Items, seedTwo.DID)
	assertCatalogueItemMatchesSeed(t, itemOne, seedOne, nameOne)
	assertCatalogueItemMatchesSeed(t, itemTwo, seedTwo, nameTwo)
}

func TestSearch_FiltersByVersionAfterPostingSDToFederatedCatalogue(t *testing.T) {
	fc := setupTestFC(t)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	participantID := getParticipantID()
	templateData := loadExampleTemplateData(t)

	didV1, err := base.GetDID(datatype.TemplateResourceType)
	require.NoError(t, err)
	didV2, err := base.GetDID(datatype.TemplateResourceType)
	require.NoError(t, err)
	const nameOne = "Catalogue Search Template V1"
	const nameTwo = "Catalogue Search Template V2"
	const versionOne = 1
	const versionTwo = 2

	seedV1 := seedFCTemplateResource(t, ctx, fc, participantID, *didV1, versionOne, "document-number-v1", contracttemplatetype.FrameContract.String(), nameOne, templateData)
	seedV2 := seedFCTemplateResource(t, ctx, fc, participantID, *didV2, versionTwo, "document-number-v2", contracttemplatetype.FrameContract.String(), nameTwo, templateData)

	result := searchFederatedCatalogue(t, ctx, fc, templatequery.SearchQry{
		Version: versionOne,
		Offset:  0,
		Limit:   0,
	})

	assert.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Items, 1)
	assertCatalogueItemMatchesSeed(t, result.Items[0], seedV1, nameOne)
	assert.Nil(t, catalogueItemByDID(result.Items, seedV2.DID))
}

func TestSearch_FindsPublishedTemplateByMatchingName(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	templateData := loadExampleTemplateData(t)

	did, err := base.GetDID(datatype.TemplateResourceType)
	require.NoError(t, err)

	const publishedName = "Catalogue Search Published Template"
	publishApprovedTemplateToFC(t, ctx, db, fc, repo, *did, publishedName, templateData)

	result := searchFederatedCatalogue(t, ctx, fc, templatequery.SearchQry{
		Name:   "published template",
		Offset: 0,
		Limit:  0,
	})

	assert.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Items, 1)
	assert.Equal(t, *did, result.Items[0].Did)
	require.NotNil(t, result.Items[0].Name)
	assert.Equal(t, publishedName, *result.Items[0].Name)
}

func TestSearch_DoesNotFindPublishedTemplateByNonMatchingName(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	templateData := loadExampleTemplateData(t)

	did, err := base.GetDID(datatype.TemplateResourceType)
	require.NoError(t, err)

	const publishedName = "Catalogue Search Published Template"
	publishApprovedTemplateToFC(t, ctx, db, fc, repo, *did, publishedName, templateData)

	result := searchFederatedCatalogue(t, ctx, fc, templatequery.SearchQry{
		Name:   "name-that-does-not-exist-in-fc",
		Offset: 0,
		Limit:  0,
	})

	assert.Equal(t, 0, result.TotalCount)
	assert.Empty(t, result.Items)
	assert.Nil(t, catalogueItemByDID(result.Items, *did))
}

func TestSearch_FindsTemplatePublishedToFederatedCatalogue(t *testing.T) {
	db := setupTestDB(t)
	fc := setupTestFC(t)

	cleanupContractTemplateTable(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), conf.TransactionTimeout())
	defer cancel()

	repo := NewTestRepo()
	templateData := loadExampleTemplateData(t)

	did, err := base.GetDID(datatype.TemplateResourceType)
	require.NoError(t, err)

	const publishedName = "Catalogue Search Published By DID"
	publishApprovedTemplateToFC(t, ctx, db, fc, repo, *did, publishedName, templateData)

	result := searchFederatedCatalogue(t, ctx, fc, templatequery.SearchQry{
		DID:    *did,
		Offset: 0,
		Limit:  0,
	})

	assert.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Items, 1)
	assert.Equal(t, *did, result.Items[0].Did)
	require.NotNil(t, result.Items[0].Version)
	assert.Equal(t, 1, *result.Items[0].Version)
	require.NotNil(t, result.Items[0].Name)
	assert.Equal(t, publishedName, *result.Items[0].Name)
}
