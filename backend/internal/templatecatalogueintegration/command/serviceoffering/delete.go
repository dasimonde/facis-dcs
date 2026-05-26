package serviceoffering

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	selfdescriptionquery "digital-contracting-service/internal/templatecatalogueintegration/query/selfdescription"
	serviceofferingid "digital-contracting-service/internal/templatecatalogueintegration/serviceoffering"
)

type DeleteCmd struct {
	ParticipantID string
}

// DeleteServiceOffering handler deletes the service offering in the Federated Catalogue.
type Deleter struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

type DeleteResult struct {
	ID string
}

func (h *Deleter) Handle(ctx context.Context, cmd DeleteCmd) (*DeleteResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}
	serviceOfferingID, err := serviceofferingid.BuildID(cmd.ParticipantID)
	if err != nil {
		return nil, err
	}

	// 1. Get the self-description hash by service offering id
	hashHandler := selfdescriptionquery.GetSelfDescriptionsMetaByIDsHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	hashResult, err := hashHandler.Handle(selfdescriptionquery.GetSelfDescriptionsMetaByIDsQry{
		IDs: []string{serviceOfferingID},
	})
	if err != nil {
		return nil, err
	}
	if hashResult == nil {
		return &DeleteResult{ID: serviceOfferingID}, nil
	}
	sdHash := hashResult.SdHashByID[serviceOfferingID]
	if sdHash == "" {
		return &DeleteResult{ID: serviceOfferingID}, nil
	}

	// 2. Delete the service offering
	path := client.SelfDescriptionsEndpointPath + "/" + url.PathEscape(sdHash)

	resp, err := h.FCClient.Delete(h.Ctx, path, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		if resp.StatusCode == http.StatusNotFound {
			return &DeleteResult{ID: serviceOfferingID}, nil
		}
		return nil, fmt.Errorf("delete service offering failed with status %d", resp.StatusCode)
	}

	return &DeleteResult{ID: serviceOfferingID}, nil
}
