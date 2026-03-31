package command

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
)

type GetCurrentParticipantSummaryCmd struct {
	ParticipantID string
	Token         string
}

type GetCurrentParticipantSummary struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *GetCurrentParticipantSummary) Handle(cmd GetCurrentParticipantSummaryCmd) (*templatecatalogueintegration.TemplateCatalogueParticipantSummary, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	handler := query.GetCurrentParticipantSummaryHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}

	return handler.Handle(query.GetCurrentParticipantSummaryQry{
		ParticipantID: cmd.ParticipantID,
		Token:         cmd.Token,
	})
}
