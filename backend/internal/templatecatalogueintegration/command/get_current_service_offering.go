package command

import (
	"context"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
)

type GetCurrentServiceOfferingCmd struct {
	ParticipantID string
	Token         string
}

type GetCurrentServiceOfferingResult struct {
	Keywords           []string
	Description        string
	EndPointURL        string
	TermsAndConditions string
}

// GetCurrentServiceOffering handler fetches the current service offering projection.
type GetCurrentServiceOffering struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *GetCurrentServiceOffering) Handle(cmd GetCurrentServiceOfferingCmd) (*GetCurrentServiceOfferingResult, error) {
	handler := query.GetServiceOfferingByParticipantHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	result, err := handler.Handle(query.GetServiceOfferingByParticipantQry{
		ParticipantID: cmd.ParticipantID,
		Token:         cmd.Token,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &GetCurrentServiceOfferingResult{
		Keywords:           result.Keywords,
		Description:        result.Description,
		EndPointURL:        result.EndPointURL,
		TermsAndConditions: result.TermsAndConditions,
	}, nil
}
