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
)

type SearchQry struct {
	DID         string
	Version     int
	Name        string
	Description string
	Offset      int
	Limit       int
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type SearchHandler struct {
	DB       *sqlx.DB
	FCClient *client.FederatedCatalogueClient
}

const searchTemplatesCountStatementTemplate = `
SELECT (COUNT(DISTINCT ?template_uuid) AS ?total) WHERE {
%s%s}
`

const searchTemplatesStatementTemplate = `
SELECT (?template_uuid AS ?did) ?name ?description ?version ?state ?template_uuid WHERE {
%s%s}
ORDER BY ?s
OFFSET %d
LIMIT %d
`

func (h *SearchHandler) Handle(ctx context.Context, qry SearchQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	filters := buildSearchFilters(qry)

	countStatement := fmt.Sprintf(searchTemplatesCountStatementTemplate, coreFieldTriples(), filters)
	countResp, err := h.FCClient.Query(ctx, client.QueryRequest{
		Statement: countStatement,
	})
	if err != nil {
		return nil, err
	}

	totalCount := countFromResults(countResp.Items, "total")

	limit := qry.Limit
	if limit < 1 {
		limit = totalCount
	}

	statement := fmt.Sprintf(searchTemplatesStatementTemplate, coreFieldTriples(), filters, qry.Offset, limit)
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

		evt := catalogueevents.SearchEvent{
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

// buildSearchFilters renders SPARQL FILTER clauses for the search query's
// optional conditions. Values are inlined (escaped) rather than bound as
// query parameters — FC's SPARQL backend does not support parameterized
// queries the way the old Neo4j/Cypher backend did (see sparql.go).
func buildSearchFilters(qry SearchQry) string {
	var b strings.Builder

	if value := strings.TrimSpace(qry.DID); value != "" {
		fmt.Fprintf(&b, "  FILTER(CONTAINS(LCASE(?template_uuid), LCASE(\"%s\")))\n", sparqlEscapeString(value))
	}
	if qry.Version > 0 {
		fmt.Fprintf(&b, "  FILTER(?version = \"%d\")\n", qry.Version)
	}
	if value := strings.TrimSpace(qry.Name); value != "" {
		fmt.Fprintf(&b, "  FILTER(CONTAINS(LCASE(?name), LCASE(\"%s\")))\n", sparqlEscapeString(value))
	}
	if value := strings.TrimSpace(qry.Description); value != "" {
		fmt.Fprintf(&b, "  FILTER(CONTAINS(LCASE(?description), LCASE(\"%s\")))\n", sparqlEscapeString(value))
	}

	return b.String()
}
