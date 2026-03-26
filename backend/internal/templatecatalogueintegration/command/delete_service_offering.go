package command

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
)

type DeleteServiceOfferingCmd struct {
	Token         string
	ParticipantID string
}

type DeleteServiceOfferingResult struct {
	ID string
}

// DeleteServiceOffering handler deletes the service offering in the Federated Catalogue.
type DeleteServiceOffering struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *DeleteServiceOffering) Handle(cmd DeleteServiceOfferingCmd) (*DeleteServiceOfferingResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	// 1. Get the service offering by participant id
	handler := query.GetServiceOfferingByParticipantHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	serviceOffering, err := handler.Handle(query.GetServiceOfferingByParticipantQry{
		ParticipantID: cmd.ParticipantID,
		Token:         cmd.Token,
	})
	if err != nil {
		return nil, err
	}
	if serviceOffering == nil || serviceOffering.URI == "" {
		return nil, nil
	}

	// 2. Get the self-description hash by service offering id
	hashHandler := query.GetSelfDescriptionsMetaByIDsHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	hashResult, err := hashHandler.Handle(query.GetSelfDescriptionsMetaByIDsQry{
		IDs:   []string{serviceOffering.URI},
		Token: cmd.Token,
	})
	if err != nil {
		return nil, err
	}
	if hashResult == nil {
		return nil, nil
	}
	sdHash := hashResult.SdHashByID[serviceOffering.URI]
	if sdHash == "" {
		return nil, nil
	}

	// 3. Delete the service offering
	path := client.SelfDescriptionsEndpointPath + "/" + url.PathEscape(sdHash)

	resp, err := h.FCClient.Delete(h.Ctx, path, cmd.Token, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("delete service offering failed with status %d", resp.StatusCode)
	}

	return &DeleteServiceOfferingResult{
		ID: serviceOffering.URI,
	}, nil
}
