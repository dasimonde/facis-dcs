package query

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// ParticipantExistsQry checks whether a participant exists in the FC graph.
type ParticipantExistsQry struct {
	ParticipantID string
	Token         string
}

type ParticipantExistsResponse struct {
	Exists bool
}

// ParticipantExistsHandler fetches a participant existence projection from FC.
type ParticipantExistsHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

// Query to check if a participant exists in the Federated Catalogue.
const participantExistsStatement = `
MATCH (p:Participant)
WHERE p.uri = $participantId
RETURN { exists: true } AS n
LIMIT 1
`

func (h *ParticipantExistsHandler) Handle(qry ParticipantExistsQry) (*ParticipantExistsResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	reqBody := client.QueryRequest{
		Statement: participantExistsStatement,
		Parameters: map[string]string{
			"participantId": qry.ParticipantID,
		},
	}

	queryResp, err := h.FCClient.Query(h.Ctx, qry.Token, reqBody)
	if err != nil {
		return nil, err
	}

	exists := !(queryResp.TotalCount == 0 || len(queryResp.Items) == 0)
	return &ParticipantExistsResponse{Exists: exists}, nil
}

