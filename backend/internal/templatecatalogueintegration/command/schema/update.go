package schema

import (
	"context"
	"fmt"
	"strings"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// UpdateCmd replaces an existing FC schema shape TTL.
type UpdateCmd struct {
	ID      string
	Content []byte
}

// UpdateHandler runs PUT /schemas/{id}.
type UpdateHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *UpdateHandler) Handle(_ context.Context, cmd UpdateCmd) error {
	if h.FCClient == nil {
		return client.ErrFederatedCatalogueNotConfigured
	}
	id := strings.TrimSpace(cmd.ID)
	if id == "" {
		return fmt.Errorf("schema id is empty")
	}
	if len(cmd.Content) == 0 {
		return fmt.Errorf("schema content is empty")
	}

	path := client.SchemaEndpointPath + "/" + id
	resp, err := h.FCClient.PutRaw(h.Ctx, path, nil, client.RDFXMLContentType, cmd.Content)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return h.FCClient.SchemaHTTPError("update schema", resp)
}
