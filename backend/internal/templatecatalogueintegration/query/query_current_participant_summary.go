package query

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

type GetCurrentParticipantSummaryQry struct {
	ParticipantID string
	Token         string
}

type GetCurrentParticipantSummaryHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const getCurrentParticipantSummaryStatement = `
MATCH (p:Participant)
WHERE p.uri = $participantId
OPTIONAL MATCH (p)-[:headquarterAddress]->(hq)
OPTIONAL MATCH (p)-[:TermsAndConditions]->(tc)
RETURN {
  legal_name: p.legalName,
  registration_number: p.registrationNumber,
  lei_code: p.leiCode,
  headquarter_address: {
    country: hq.country,
    locality: hq.locality
  },
  terms_and_conditions: tc.url
} AS n
LIMIT 1
`

func (h *GetCurrentParticipantSummaryHandler) Handle(qry GetCurrentParticipantSummaryQry) (*templatecatalogueintegration.TemplateCatalogueParticipantSummary, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	reqBody := client.QueryRequest{
		Statement: getCurrentParticipantSummaryStatement,
		Parameters: map[string]string{
			"participantId": qry.ParticipantID,
		},
	}

	queryResp, err := h.FCClient.Query(h.Ctx, qry.Token, reqBody)
	if err != nil {
		return nil, err
	}
	if queryResp.TotalCount == 0 || len(queryResp.Items) == 0 {
		return nil, nil
	}

	var participant map[string]interface{}
	for _, v := range queryResp.Items[0] {
		if m, ok := v.(map[string]interface{}); ok {
			participant = m
			break
		}
	}
	if participant == nil {
		return nil, nil
	}

	hq, ok := participant["headquarter_address"].(map[string]interface{})
	if !ok {
		hq = map[string]interface{}{}
	}

	return &templatecatalogueintegration.TemplateCatalogueParticipantSummary{
		LegalName:          summaryStringPtr(derefString(participant, "legal_name")),
		RegistrationNumber: summaryStringPtr(derefString(participant, "registration_number")),
		LeiCode:            summaryStringPtr(derefString(participant, "lei_code")),
		HeadquarterAddress: &templatecatalogueintegration.TemplateCatalogueParticipantHeadquarterSummary{
			Country:  summaryStringPtr(derefString(hq, "country")),
			Locality: summaryStringPtr(derefString(hq, "locality")),
		},
		TermsAndConditions: summaryStringPtr(derefString(participant, "terms_and_conditions")),
	}, nil
}
