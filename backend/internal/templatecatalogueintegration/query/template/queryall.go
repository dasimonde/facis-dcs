package template

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

type GetAllMetadataQry struct {
	Offset int
	Limit  int
}

type GetAllMetadataHandler struct {
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

func (h *GetAllMetadataHandler) Handle(qry GetAllMetadataQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}
	if qry.Limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}

	countResp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
		Statement:  retrieveTemplatesCountStatement,
		Parameters: map[string]string{},
	})
	if err != nil {
		return nil, err
	}

	totalCount := countResp.TotalCount

	statement := fmt.Sprintf(retrieveTemplatesStatementTemplate, qry.Offset, qry.Limit)
	dataResp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
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
