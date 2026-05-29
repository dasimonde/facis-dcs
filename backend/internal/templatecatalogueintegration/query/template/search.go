package template

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"
	"strconv"
	"strings"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

type SearchQry struct {
	DID            string
	DocumentNumber string
	Version        int
	Name           string
	Description    string
	Offset         int
	Limit          int
}

type SearchHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const searchTemplatesCountStatementTemplate = `
MATCH (ct:ContractTemplate)
%s
RETURN count(ct) AS total
`

const searchTemplatesStatementTemplate = `
MATCH (ct:ContractTemplate)
%s
RETURN {
  did: ct.did,
  document_number: ct.documentNumber,
  version: ct.version,
  schema_version: ct.schemaVersion,
  name: ct.name,
  description: ct.description,
  template_type: ct.templateType,
  participant_id: ct.participantId,
  created_at: ct.createdAt,
  updated_at: ct.updatedAt
} AS n
SKIP %d
LIMIT %d
`

func (h *SearchHandler) Handle(qry SearchQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	whereClause, params := buildSearchWhereClause(qry)
	where := formatSearchWhereSection(whereClause)

	countStatement := fmt.Sprintf(searchTemplatesCountStatementTemplate, where)
	countResp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
		Statement:  countStatement,
		Parameters: params,
	})
	if err != nil {
		return nil, err
	}

	totalCount := countResp.TotalCount

	limit := qry.Limit
	if limit < 1 {
		limit = totalCount
	}

	statement := fmt.Sprintf(searchTemplatesStatementTemplate, where, qry.Offset, limit)
	dataResp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
		Statement:  statement,
		Parameters: params,
	})
	if err != nil {
		return nil, err
	}

	items := make([]*templatecatalogueintegration.TemplateCatalogueItem, 0, len(dataResp.Items))
	for _, item := range dataResp.Items {
		var ct map[string]interface{}
		// Extract the template projection map from the item
		for _, v := range item {
			if m, ok := v.(map[string]interface{}); ok {
				ct = m
				break
			}
		}
		if ct == nil {
			continue
		}
		items = append(items, &templatecatalogueintegration.TemplateCatalogueItem{
			Did:            ptr.StringFromMap(ct, "did"),
			DocumentNumber: ptr.Ref(ptr.StringFromMap(ct, "document_number")),
			Version:        ptr.Ref(ptr.IntFromMap(ct, "version")),
			SchemaVersion:  ptr.Ref(ptr.IntFromMap(ct, "schema_version")),
			Name:           ptr.Ref(ptr.StringFromMap(ct, "name")),
			Description:    ptr.Ref(ptr.StringFromMap(ct, "description")),
			TemplateType:   ptr.Ref(ptr.StringFromMap(ct, "template_type")),
			ParticipantID:  ptr.Ref(ptr.StringFromMap(ct, "participant_id")),
			CreatedAt:      ptr.Ref(ptr.StringFromMap(ct, "created_at")),
			UpdatedAt:      ptr.Ref(ptr.StringFromMap(ct, "updated_at")),
		})
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveResponse{
		TotalCount: totalCount,
		Items:      items,
	}, nil
}

func formatSearchWhereSection(whereClause string) string {
	if whereClause == "" {
		return ""
	}
	return "WHERE " + whereClause + "\n"
}

func buildSearchWhereClause(qry SearchQry) (string, map[string]string) {
	conditions := make([]string, 0, 5)
	params := make(map[string]string)

	if value := strings.TrimSpace(qry.DID); value != "" {
		conditions = append(conditions, "toLower(ct.did) CONTAINS toLower($did)")
		params["did"] = value
	}
	if value := strings.TrimSpace(qry.DocumentNumber); value != "" {
		conditions = append(conditions, "toLower(ct.documentNumber) CONTAINS toLower($document_number)")
		params["document_number"] = value
	}
	if qry.Version > 0 {
		conditions = append(conditions, "ct.version = $version")
		params["version"] = strconv.Itoa(qry.Version)
	}
	if value := strings.TrimSpace(qry.Name); value != "" {
		conditions = append(conditions, "toLower(ct.name) CONTAINS toLower($name)")
		params["name"] = value
	}
	if value := strings.TrimSpace(qry.Description); value != "" {
		conditions = append(conditions, "toLower(ct.description) CONTAINS toLower($description)")
		params["description"] = value
	}

	return strings.Join(conditions, " AND "), params
}
