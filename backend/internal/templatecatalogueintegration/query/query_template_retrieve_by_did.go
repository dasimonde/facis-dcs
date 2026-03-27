package query

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

type RetrieveTemplateByIDQry struct {
	Token string
	DID   string
}

type RetrieveTemplateByIDHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const retrieveTemplateByIDStatement = `
MATCH (ct:ContractTemplate)
WHERE ct.did = $did
OPTIONAL MATCH (ct)-[:operatedBy]->(p:Participant)
OPTIONAL MATCH (p)-[:headquarterAddress]->(hq)
OPTIONAL MATCH (p)-[:TermsAndConditions]->(tc)
RETURN {
  did: ct.did,
  document_number: ct.documentNumber,
  version: ct.version,
  name: ct.name,
  description: ct.description,
  template_type: ct.templateType,
  participant_id: p.uri,
  participant: {
    legal_name: p.legalName,
    registration_number: p.registrationNumber,
    lei_code: p.leiCode,
    headquarter_address: {
      country: hq.country,
      locality: hq.locality
    },
    terms_and_conditions: tc.url
  },
  created_at: ct.createdAt,
  updated_at: ct.updatedAt
} AS n
LIMIT 1
`

func (h *RetrieveTemplateByIDHandler) Handle(qry RetrieveTemplateByIDQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.DID == "" {
		return nil, fmt.Errorf("did is empty")
	}

	resp, err := h.FCClient.Query(h.Ctx, qry.Token, client.QueryRequest{
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

	var n map[string]interface{}
	for _, v := range resp.Items[0] {
		if m, ok := v.(map[string]interface{}); ok {
			n = m
			break
		}
	}
	if n == nil {
		return nil, fmt.Errorf("query projection missing projected map for did=%s", qry.DID)
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse{
		Did:            derefString(n, "did"),
		DocumentNumber: stringPtr(derefString(n, "document_number")),
		Version:        intPtr(derefInt(n, "version")),
		Name:           stringPtr(derefString(n, "name")),
		Description:    stringPtr(derefString(n, "description")),
		TemplateType:   stringPtr(derefString(n, "template_type")),
		Participant:    mapTemplateParticipantSummary(n),
		CreatedAt:      stringPtr(derefString(n, "created_at")),
		UpdatedAt:      stringPtr(derefString(n, "updated_at")),
	}, nil
}

func mapTemplateParticipantSummary(n map[string]interface{}) *templatecatalogueintegration.TemplateCatalogueParticipantSummary {
	participantRaw, ok := n["participant"].(map[string]interface{})
	if !ok || participantRaw == nil {
		// Optional participant summary
		return nil
	}

	headquarterRaw, _ := participantRaw["headquarter_address"].(map[string]interface{})

	return &templatecatalogueintegration.TemplateCatalogueParticipantSummary{
		LegalName:          stringPtr(derefString(participantRaw, "legal_name")),
		RegistrationNumber: stringPtr(derefString(participantRaw, "registration_number")),
		LeiCode:            stringPtr(derefString(participantRaw, "lei_code")),
		HeadquarterAddress: &templatecatalogueintegration.TemplateCatalogueParticipantHeadquarterSummary{
			Country:  stringPtr(derefString(headquarterRaw, "country")),
			Locality: stringPtr(derefString(headquarterRaw, "locality")),
		},
		TermsAndConditions: stringPtr(derefString(participantRaw, "terms_and_conditions")),
	}
}
