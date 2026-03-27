package query

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

type RetrieveTemplatesQry struct {
	Token  string
	Offset int
	Limit  int
}

type RetrieveTemplatesHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const retrieveTemplatesCountStatement = `
MATCH (n:ContractTemplate)
RETURN count(n) AS total
`

const retrieveTemplatesStatementTemplate = `
MATCH (ct:ContractTemplate)
RETURN {
  did: ct.did,
  document_number: ct.documentNumber,
  version: ct.version,
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

func (h *RetrieveTemplatesHandler) Handle(qry RetrieveTemplatesQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}
	if qry.Limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}

	countResp, err := h.FCClient.Query(h.Ctx, qry.Token, client.QueryRequest{
		Statement:  retrieveTemplatesCountStatement,
		Parameters: map[string]string{},
	})
	if err != nil {
		return nil, err
	}

	totalCount := countResp.TotalCount

	statement := fmt.Sprintf(retrieveTemplatesStatementTemplate, qry.Offset, qry.Limit)
	dataResp, err := h.FCClient.Query(h.Ctx, qry.Token, client.QueryRequest{
		Statement:  statement,
		Parameters: map[string]string{},
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
			Did:            derefString(ct, "did"),
			DocumentNumber: stringPtr(derefString(ct, "document_number")),
			Version:        intPtr(derefInt(ct, "version")),
			Name:           stringPtr(derefString(ct, "name")),
			Description:    stringPtr(derefString(ct, "description")),
			TemplateType:   stringPtr(derefString(ct, "template_type")),
			ParticipantID:  stringPtr(derefString(ct, "participant_id")),
			CreatedAt:      stringPtr(derefString(ct, "created_at")),
			UpdatedAt:      stringPtr(derefString(ct, "updated_at")),
		})
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveResponse{
		TotalCount: totalCount,
		Items:      items,
	}, nil
}

func derefInt(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return 0
	}
}

func stringPtr(v string) *string {
	return &v
}

func intPtr(v int) *int {
	return &v
}
