package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
)

type UpdateServiceOfferingCmd struct {
	Token              string
	ParticipantID      string
	EndPointURL        string
	TermsAndConditions string
	Keywords           []string
	Description        string
}

type UpdateServiceOfferingResult struct {
	ID string
}

// UpdateServiceOffering handler updates the service offering in the Federated Catalogue.
type UpdateServiceOffering struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *UpdateServiceOffering) Handle(cmd UpdateServiceOfferingCmd) (*UpdateServiceOfferingResult, error) {
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
	if len(cmd.Keywords) == 0 {
		return nil, fmt.Errorf("service offering keywords is empty")
	}
	if cmd.Description == "" {
		return nil, fmt.Errorf("service offering description is empty")
	}

	serviceOfferingID := strings.ReplaceAll(cmd.ParticipantID, "participant", "service-offering")
	if serviceOfferingID == "" {
		return nil, fmt.Errorf("service offering id is empty")
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
	resp, err := h.FCClient.Post(h.Ctx, client.SelfDescriptionsEndpointPath, cmd.Token, nil, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("update service offering failed with status %d", resp.StatusCode)
	}

	return &UpdateServiceOfferingResult{ID: serviceOfferingID}, nil
}
