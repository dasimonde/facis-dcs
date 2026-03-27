package command

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
)

type RetrieveTemplateByIDCmd struct {
	Token string
	DID   string
}

type RetrieveTemplateByID struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *RetrieveTemplateByID) Handle(cmd RetrieveTemplateByIDCmd) (*templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}

	handler := query.RetrieveTemplateByIDHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}

	resp, err := handler.Handle(query.RetrieveTemplateByIDQry{
		Token: cmd.Token,
		DID:   cmd.DID,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}

	return resp, nil
}
