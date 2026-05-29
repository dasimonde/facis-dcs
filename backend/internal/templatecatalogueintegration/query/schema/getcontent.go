package schema

import (
	"context"
	"fmt"
	"strings"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// GetContentQry loads one FC schema shape TTL by id.
type GetContentQry struct {
	ID string
}

// GetContentHandler fetches schema TTL from GET /schemas/{id}.
type GetContentHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *GetContentHandler) Handle(qry GetContentQry) ([]byte, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	id := strings.TrimSpace(qry.ID)
	if id == "" {
		return nil, fmt.Errorf("schema id is empty")
	}

	path := client.SchemaEndpointPath + "/" + id
	resp, err := h.FCClient.Get(h.Ctx, path, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get schema %s failed with status %d: %s", id, resp.StatusCode, h.FCClient.ExtractErrorMessage(resp.Body))
	}
	return resp.Body, nil
}
