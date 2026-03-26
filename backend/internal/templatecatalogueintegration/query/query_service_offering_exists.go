package query

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// ServiceOfferingExistsQry checks whether a service offering node exists by serviceOfferingId.
type ServiceOfferingExistsQry struct {
	ServiceOfferingID string
	Token             string
}

type ServiceOfferingExistsResponse struct {
	Exists bool
}

// ServiceOfferingExistsHandler fetches a service offering existence projection from FC.
type ServiceOfferingExistsHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

// Query to check if a service offering node exists in the Federated Catalogue by serviceOfferingId.
const serviceOfferingExistsStatement = `
MATCH (so:ServiceOffering)
WHERE so.uri = $serviceOfferingId
RETURN { exists: true } AS n
LIMIT 1
`

func (h *ServiceOfferingExistsHandler) Handle(qry ServiceOfferingExistsQry) (*ServiceOfferingExistsResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if qry.ServiceOfferingID == "" {
		return nil, fmt.Errorf("service offering id is empty")
	}

	reqBody := client.QueryRequest{
		Statement: serviceOfferingExistsStatement,
		Parameters: map[string]string{
			"serviceOfferingId": qry.ServiceOfferingID,
		},
	}

	queryResp, err := h.FCClient.Query(h.Ctx, qry.Token, reqBody)
	if err != nil {
		return nil, err
	}

	exists := !(queryResp.TotalCount == 0 || len(queryResp.Items) == 0)
	return &ServiceOfferingExistsResponse{Exists: exists}, nil
}
