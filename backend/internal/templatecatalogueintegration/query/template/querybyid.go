package template

import (
	"context"
	"database/sql"
	"encoding/json"
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

type GetByIDQry struct {
	DID         string
	Version     int
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type GetByIDHandler struct {
	DB       *sqlx.DB
	FCClient *client.FederatedCatalogueClient
}

const retrieveTemplateByIDStatement = `
MATCH (ct)
WHERE ct.templateUuid IS NOT NULL
  AND head(ct.claimsGraphUri) = $did
RETURN {
  did: head(ct.claimsGraphUri),
  name: ct.name,
  description: ct.description,
  version: ct.version,
  state: ct.state,
  template_uuid: ct.templateUuid,
  template_type: ct.templateType,
  template_data_string: ct.templateDataString
} AS n
LIMIT 1
`

func (h *GetByIDHandler) Handle(ctx context.Context, qry GetByIDQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.DID == "" {
		return nil, fmt.Errorf("did is empty")
	}
	if qry.Version < 1 {
		return nil, fmt.Errorf("version must be greater than 0")
	}

	resp, err := h.FCClient.Query(ctx, client.QueryRequest{
		Statement: retrieveTemplateByIDStatement,
		Parameters: map[string]string{
			"did": qry.DID,
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.TotalCount == 0 || len(resp.Items) == 0 {
		return nil, nil
	}

	n := projectionMap(resp.Items[0])
	if n == nil {
		return nil, fmt.Errorf("query projection missing projected map for did=%s", qry.DID)
	}

	result := mapCatalogueDetail(n)
	if result == nil {
		return nil, nil
	}

	if result.Version == nil || *result.Version != qry.Version {
		return nil, nil
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

		evt := catalogueevents.RetrieveByIDEvent{
			DID:         qry.DID,
			Version:     qry.Version,
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

	templateData, err := templateDataFromString(ptr.StringFromMap(n, "template_data_string"))
	if err != nil {
		return nil, err
	}

	result.TemplateData = templateData
	return result, nil
}

func mapCatalogueDetail(n map[string]interface{}) *templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse {
	if n == nil {
		return nil
	}

	did := ptr.StringFromMap(n, "did")
	if strings.TrimSpace(did) == "" {
		return nil
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse{
		Did:          did,
		Version:      ptr.Ref(ptr.IntFromMap(n, "version")),
		Name:         ptr.Ref(ptr.StringFromMap(n, "name")),
		Description:  ptr.Ref(ptr.StringFromMap(n, "description")),
		TemplateType: ptr.Ref(ptr.StringFromMap(n, "template_type")),
	}
}

func templateDataFromString(raw string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("template data is missing from Federated Catalogue")
	}

	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &templateData); err != nil {
		return nil, fmt.Errorf("parse template data from Federated Catalogue: %w", err)
	}

	return templateData, nil
}
