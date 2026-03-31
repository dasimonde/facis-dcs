package command

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"fmt"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
)

type ListOtherParticipantsCmd struct {
	ParticipantID string
	Token         string
}

type ListOtherParticipants struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *ListOtherParticipants) Handle(cmd ListOtherParticipantsCmd) ([]*templatecatalogueintegration.TemplateCatalogueParticipantSummary, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	handler := query.ListOtherParticipantsHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}

	queryItems, err := handler.Handle(query.ListOtherParticipantsQry{
		ParticipantID: cmd.ParticipantID,
		Token:         cmd.Token,
	})
	if err != nil {
		return nil, err
	}

	return queryItems, nil
}
