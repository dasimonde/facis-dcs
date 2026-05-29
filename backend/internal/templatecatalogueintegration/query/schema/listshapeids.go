package schema

import (
	"context"
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

type listShapesResponse struct {
	Shapes []string `json:"shapes"`
}

// ListShapeIDsQry lists FC schema shape ids from GET /schemas.
type ListShapeIDsQry struct{}

// ListShapeIDsHandler loads schema shape ids from FC.
type ListShapeIDsHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *ListShapeIDsHandler) Handle(_ ListShapeIDsQry) ([]string, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}

	resp, err := h.FCClient.Get(h.Ctx, client.SchemaEndpointPath, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list schemas failed with status %d: %s", resp.StatusCode, h.FCClient.ExtractErrorMessage(resp.Body))
	}

	var list listShapesResponse
	if err := json.Unmarshal(resp.Body, &list); err != nil {
		return nil, fmt.Errorf("unmarshal schemas list failed: %w", err)
	}
	return list.Shapes, nil
}
