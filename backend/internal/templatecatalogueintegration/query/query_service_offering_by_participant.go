package query

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// GetServiceOfferingByParticipantQry fetches a service offering by participant-id.
type GetServiceOfferingByParticipantQry struct {
	ParticipantID string
	Token         string
}

type ServiceOfferingByParticipantResponse struct {
	URI                string
	Keywords           []string
	Description        string
	EndPointURL        string
	TermsAndConditions string
}

// GetServiceOfferingByParticipantHandler fetches a service offering projection by participant-id.
type GetServiceOfferingByParticipantHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const getServiceOfferingByParticipantStatement = `
MATCH (so:ServiceOffering)-[:offeredBy]->(p:Participant)
WHERE p.uri = $participantId
OPTIONAL MATCH (so)-[:termsAndConditions]->(tc)
RETURN {
  uri: so.uri,
  end_point_url: so.endPointURL,
  terms_and_conditions: tc.content,
  keywords: so.keyword,
  description: so.description
} AS n
LIMIT 1
`

func (h *GetServiceOfferingByParticipantHandler) Handle(qry GetServiceOfferingByParticipantQry) (*ServiceOfferingByParticipantResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	reqBody := client.QueryRequest{
		Statement: getServiceOfferingByParticipantStatement,
		Parameters: map[string]string{
			"participantId": qry.ParticipantID,
		},
	}

	queryResp, err := h.FCClient.Query(h.Ctx, qry.Token, reqBody)
	if err != nil {
		return nil, err
	}
	if queryResp.TotalCount == 0 || len(queryResp.Items) == 0 {
		// Not found
		return nil, nil
	}

	var offering map[string]interface{}
	for _, v := range queryResp.Items[0] {
		if m, ok := v.(map[string]interface{}); ok {
			offering = m
			break
		}
	}
	if offering == nil {
		return nil, fmt.Errorf("query projection missing projected map for participantId=%s", qry.ParticipantID)
	}

	return &ServiceOfferingByParticipantResponse{
		URI:                derefString(offering, "uri"),
		Keywords:           derefStringSlice(offering, "keywords"),
		Description:        derefString(offering, "description"),
		EndPointURL:        derefString(offering, "end_point_url"),
		TermsAndConditions: derefString(offering, "terms_and_conditions"),
	}, nil
}

func derefStringSlice(m map[string]interface{}, key string) []string {
	if m == nil {
		return []string{}
	}
	v, ok := m[key]
	if !ok || v == nil {
		return []string{}
	}
	arr, ok := v.([]interface{})
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
