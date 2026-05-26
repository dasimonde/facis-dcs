package serviceoffering

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

// GetByParticipantQry fetches a service offering by participant-id.
type GetByParticipantQry struct {
	ParticipantID string
}

type GetByParticipantResult struct {
	URI                string
	Keywords           []string
	Description        string
	EndPointURL        string
	TermsAndConditions string
}

// GetByParticipantHandler fetches a service offering projection by participant-id.
type GetByParticipantHandler struct {
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

func (h *GetByParticipantHandler) Handle(qry GetByParticipantQry) (*GetByParticipantResult, error) {
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

	queryResp, err := h.FCClient.Query(h.Ctx, reqBody)
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

	return &GetByParticipantResult{
		URI:                ptr.StringFromMap(offering, "uri"),
		Keywords:           ptr.StringSliceFromMap(offering, "keywords"),
		Description:        ptr.StringFromMap(offering, "description"),
		EndPointURL:        ptr.StringFromMap(offering, "end_point_url"),
		TermsAndConditions: ptr.StringFromMap(offering, "terms_and_conditions"),
	}, nil
}
