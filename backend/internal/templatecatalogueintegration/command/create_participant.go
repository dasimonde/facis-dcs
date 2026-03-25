package command

import (
	"context"
	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"digital-contracting-service/internal/templatecatalogueintegration/query"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
)

type CreateParticipantCmd struct {
	Token       string
	Participant selfdescription.ParticipantSdInput
}

type CreateParticipantResult struct {
	ID string
}

type CreateParticipant struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

// ErrParticipantAlreadyExists indicates that a participant with the same participantID
var ErrParticipantAlreadyExists = errors.New("participant already exists")

func (h *CreateParticipant) Handle(cmd CreateParticipantCmd) (*CreateParticipantResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.Participant.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	// Check if the participant already exists.
	existsHandler := query.ParticipantExistsHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	existsResp, err := existsHandler.Handle(query.ParticipantExistsQry{
		ParticipantID: cmd.Participant.ParticipantID,
		Token:         cmd.Token,
	})
	if err != nil {
		return nil, err
	}
	if existsResp != nil && existsResp.Exists {
		return nil, ErrParticipantAlreadyExists
	}

	// Build self-description and create the participant in the Federated Catalogue.
	jsonLD := selfdescription.BuildParticipantSelfDescription(cmd.Participant)

	body, err := json.Marshal(jsonLD)
	if err != nil {
		return nil, fmt.Errorf("marshal participant template payload failed: %w", err)
	}

	resp, err := h.FCClient.Post(h.Ctx, client.ParticipantsEndpointPath, cmd.Token, nil, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create participant failed with status %d", resp.StatusCode)
	}

	var fcResp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(resp.Body, &fcResp); err != nil {
		return nil, fmt.Errorf("parse create participant response failed: %w", err)
	}
	if fcResp.ID == "" {
		return nil, fmt.Errorf("create participant response id is empty")
	}

	return &CreateParticipantResult{ID: fcResp.ID}, nil
}
