package participant

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

type GetOtherParticipantsQry struct {
	ParticipantID string
}

type GetOtherParticipantsHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const listOtherParticipantsStatement = `
MATCH (p:Participant)
WHERE p.uri <> $participantId
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
`

func (h *GetOtherParticipantsHandler) Handle(qry GetOtherParticipantsQry) ([]*templatecatalogueintegration.TemplateCatalogueParticipantSummary, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	reqBody := client.QueryRequest{
		Statement: listOtherParticipantsStatement,
		Parameters: map[string]string{
			"participantId": qry.ParticipantID,
		},
	}

	queryResp, err := h.FCClient.Query(h.Ctx, reqBody)
	if err != nil {
		return nil, err
	}

	items := make([]*templatecatalogueintegration.TemplateCatalogueParticipantSummary, 0, len(queryResp.Items))
	for _, item := range queryResp.Items {
		var participant map[string]interface{}
		for _, v := range item {
			if m, ok := v.(map[string]interface{}); ok {
				participant = m
				break
			}
		}
		if participant == nil {
			continue
		}

		hq, ok := participant["headquarter_address"].(map[string]interface{})
		if !ok {
			hq = map[string]interface{}{}
		}

		items = append(items, &templatecatalogueintegration.TemplateCatalogueParticipantSummary{
			LegalName:          ptr.Ref(ptr.StringFromMap(participant, "legal_name")),
			RegistrationNumber: ptr.Ref(ptr.StringFromMap(participant, "registration_number")),
			LeiCode:            ptr.Ref(ptr.StringFromMap(participant, "lei_code")),
			HeadquarterAddress: &templatecatalogueintegration.TemplateCatalogueParticipantHeadquarterSummary{
				Country:  ptr.Ref(ptr.StringFromMap(hq, "country")),
				Locality: ptr.Ref(ptr.StringFromMap(hq, "locality")),
			},
			TermsAndConditions: ptr.Ref(ptr.StringFromMap(participant, "terms_and_conditions")),
		})
	}

	return items, nil
}
