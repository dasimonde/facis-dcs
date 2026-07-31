package template

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templatecatalogueintegration/client"
	catalogueevents "digital-contracting-service/internal/templatecatalogueintegration/event"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

type GetAllMetadataQry struct {
	Offset      int
	Limit       int
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type GetAllMetadataHandler struct {
	DB       *sqlx.DB
	FCClient *client.FederatedCatalogueClient
}

const retrieveTemplatesCountStatementTemplate = `
SELECT (COUNT(DISTINCT ?template_uuid) AS ?total) WHERE {
%s}
`

const retrieveTemplatesStatementTemplate = `
SELECT (?template_uuid AS ?did) ?name ?description ?version ?state ?template_uuid WHERE {
%s}
ORDER BY ?s
OFFSET %d
LIMIT %d
`

func (h *GetAllMetadataHandler) Handle(ctx context.Context, qry GetAllMetadataQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	countResp, err := h.FCClient.Query(ctx, client.QueryRequest{
		Statement: fmt.Sprintf(retrieveTemplatesCountStatementTemplate, coreFieldTriples()),
	})
	if err != nil {
		return nil, err
	}

	totalCount := countFromResults(countResp.Items, "total")

	limit := qry.Limit
	if limit < 1 {
		limit = totalCount
	}

	statement := fmt.Sprintf(retrieveTemplatesStatementTemplate, coreFieldTriples(), qry.Offset, limit)
	dataResp, err := h.FCClient.Query(ctx, client.QueryRequest{
		Statement: statement,
	})
	if err != nil {
		return nil, err
	}

	if h.DB != nil {
		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not create transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		evt := catalogueevents.RetrieveAllEvent{
			RetrievedBy: qry.RetrievedBy,
			OccurredAt:  time.Now().UTC(),
			HolderDID:   qry.HolderDID,
			UserRoles:   qry.UserRoles,
		}
		err = event.Create(ctx, tx, evt, componenttype.TemplateCatalogueIntegration)
		if err != nil {
			return nil, fmt.Errorf("could not create event: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}
	}

	items := make([]*templatecatalogueintegration.TemplateCatalogueItem, 0, len(dataResp.Items))
	for _, item := range dataResp.Items {
		if ct := projectionMap(item); ct != nil {
			if mapped := mapCatalogueItem(ct); mapped != nil {
				items = append(items, mapped)
			}
		}
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveResponse{
		TotalCount: totalCount,
		Items:      items,
	}, nil
}

// projectionMap unwraps a single query result row into its projected fields.
// SPARQL SELECT rows are already flat (variable name -> value); a nested map
// under a single key is the Neo4j/Cypher `RETURN {...} AS n` shape kept here
// only in case any caller still produces it.
func projectionMap(row map[string]interface{}) map[string]interface{} {
	if row == nil {
		return nil
	}
	for _, value := range row {
		if mapped, ok := value.(map[string]interface{}); ok {
			return mapped
		}
	}
	return row
}

func mapCatalogueItem(ct map[string]interface{}) *templatecatalogueintegration.TemplateCatalogueItem {
	if ct == nil {
		return nil
	}

	did := ptr.StringFromMap(ct, "did")
	if strings.TrimSpace(did) == "" {
		return nil
	}

	return &templatecatalogueintegration.TemplateCatalogueItem{
		Did:         did,
		Version:     ptr.Ref(ptr.IntFromMap(ct, "version")),
		Name:        ptr.Ref(ptr.StringFromMap(ct, "name")),
		Description: ptr.Ref(ptr.StringFromMap(ct, "description")),
	}
}
