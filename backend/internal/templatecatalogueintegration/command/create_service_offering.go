package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
)

type CreateServiceOfferingCmd struct {
	Token              string
	ParticipantID      string
	EndPointURL        string
	TermsAndConditions string
	Keywords           []string
	Description        string
}

type CreateServiceOfferingResult struct {
	ID string
}

// CreateServiceOffering handler creates a service offering in the Federated Catalogue.
type CreateServiceOffering struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

// ErrServiceOfferingAlreadyExists indicates that a serviceOffering with the same serviceOfferingID
var ErrServiceOfferingAlreadyExists = errors.New("ServiceOffering already exists")

func (h *CreateServiceOffering) Handle(cmd CreateServiceOfferingCmd) (*CreateServiceOfferingResult, error) {
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
	serviceOfferingID := strings.ReplaceAll(cmd.ParticipantID, "participant", "service-offering")
	if serviceOfferingID == "" {
		return nil, fmt.Errorf("service offering id is empty")
	}

	// Check if the service offering already exists by serviceOfferingID.
	existsHandler := query.ServiceOfferingExistsHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	existsResp, err := existsHandler.Handle(query.ServiceOfferingExistsQry{
		ServiceOfferingID: serviceOfferingID,
		Token:             cmd.Token,
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

	resp, err := h.FCClient.Post(h.Ctx, client.SelfDescriptionsEndpointPath, cmd.Token, nil, body)
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

	return &CreateServiceOfferingResult{ID: fcResp.ID}, nil
}
