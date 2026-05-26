package serviceoffering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	serviceofferingquery "digital-contracting-service/internal/templatecatalogueintegration/query/serviceoffering"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
	serviceofferingid "digital-contracting-service/internal/templatecatalogueintegration/serviceoffering"
)

type CreateCmd struct {
	ParticipantID      string
	EndPointURL        string
	TermsAndConditions string
	Keywords           []string
	Description        string
}

// CreateServiceOffering handler creates a service offering in the Federated Catalogue.
type Creator struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

// ErrServiceOfferingAlreadyExists indicates that a serviceOffering with the same serviceOfferingID
var ErrServiceOfferingAlreadyExists = errors.New("ServiceOffering already exists")

type CreateResult struct {
	ID string
}

func (h *Creator) Handle(ctx context.Context, cmd CreateCmd) (*CreateResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}
	if cmd.EndPointURL == "" {
		return nil, fmt.Errorf("service offering endpoint url is empty")
	}
	if cmd.TermsAndConditions == "" {
		return nil, fmt.Errorf("service offering terms and conditions is empty")
	}
	serviceOfferingID, err := serviceofferingid.BuildID(cmd.ParticipantID)
	if err != nil {
		return nil, err
	}

	// Check if the service offering already exists by serviceOfferingID.
	existsHandler := serviceofferingquery.ServiceOfferingExistsHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	existsResp, err := existsHandler.Handle(serviceofferingquery.ServiceOfferingExistsQry{
		ServiceOfferingID: serviceOfferingID,
	})
	if err != nil {
		return nil, err
	}
	if existsResp != nil && existsResp.Exists {
		return nil, ErrServiceOfferingAlreadyExists
	}

	jsonLD := selfdescription.BuildServiceOfferingSelfDescription(selfdescription.ServiceOfferingSdInput{
		ServiceOfferingID:  serviceOfferingID,
		ParticipantID:      cmd.ParticipantID,
		EndPointURL:        cmd.EndPointURL,
		TermsAndConditions: cmd.TermsAndConditions,
		Keywords:           cmd.Keywords,
		Description:        cmd.Description,
	})

	body, err := json.Marshal(jsonLD)
	if err != nil {
		return nil, fmt.Errorf("marshal service offering payload failed: %w", err)
	}

	resp, err := h.FCClient.Post(h.Ctx, client.SelfDescriptionsEndpointPath, nil, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create service offering failed with status %d", resp.StatusCode)
	}

	var fcResp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(resp.Body, &fcResp); err != nil {
		return nil, fmt.Errorf("parse create service offering response failed: %w", err)
	}

	if fcResp.ID == "" {
		return nil, fmt.Errorf("create service offering response id is empty")
	}

	return &CreateResult{ID: serviceOfferingID}, nil
}
