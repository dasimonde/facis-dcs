package schema

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// CreateCmd uploads a new schema TTL to FC.
type CreateCmd struct {
	Content []byte
}

// CreateHandler runs POST /schemas.
type CreateHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *CreateHandler) Handle(_ context.Context, cmd CreateCmd) error {
	if h.FCClient == nil {
		return client.ErrFederatedCatalogueNotConfigured
	}
	if len(cmd.Content) == 0 {
		return fmt.Errorf("schema content is empty")
	}

	resp, err := h.FCClient.PostRaw(h.Ctx, client.SchemaEndpointPath, nil, client.RDFXMLContentType, cmd.Content)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return h.FCClient.SchemaHTTPError("create schema", resp)
}
