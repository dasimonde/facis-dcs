package command

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
)

type RetrieveTemplatesCmd struct {
	Token  string
	Offset int
	Limit  int
}

type RetrieveTemplates struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *RetrieveTemplates) Handle(cmd RetrieveTemplatesCmd) (*templatecatalogueintegration.TemplateCatalogueRetrieveResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}

	handler := query.RetrieveTemplatesHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}

	resp, err := handler.Handle(query.RetrieveTemplatesQry{
		Token:  cmd.Token,
		Offset: cmd.Offset,
		Limit:  cmd.Limit,
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}
