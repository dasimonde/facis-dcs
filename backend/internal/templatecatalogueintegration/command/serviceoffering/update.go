package serviceoffering

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
	serviceofferingid "digital-contracting-service/internal/templatecatalogueintegration/serviceoffering"
)

type UpdateCmd struct {
	ParticipantID      string
	EndPointURL        string
	TermsAndConditions string
	Keywords           []string
	Description        string
}

// UpdateServiceOffering handler updates the service offering in the Federated Catalogue.
type Updater struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

type UpdateResult struct {
	ID string
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) (*UpdateResult, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
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
	if len(cmd.Keywords) == 0 {
		return nil, fmt.Errorf("service offering keywords is empty")
	}
	if cmd.Description == "" {
		return nil, fmt.Errorf("service offering description is empty")
	}

	serviceOfferingID, err := serviceofferingid.BuildID(cmd.ParticipantID)
	if err != nil {
		return nil, err
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

	// Federated Catalogue will overwrite the existing self-description if the id is the same.
	resp, err := h.FCClient.Post(h.Ctx, client.SelfDescriptionsEndpointPath, nil, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return &UpdateResult{ID: serviceOfferingID}, nil
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("update service offering failed with status %d", resp.StatusCode)
	}

	return &UpdateResult{ID: serviceOfferingID}, nil
}
