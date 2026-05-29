package participant

import (
	"context"
	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	participantquery "digital-contracting-service/internal/templatecatalogueintegration/query/participant"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
)

type CreateCmd struct {
	Participant selfdescription.ParticipantSdInput
}

type Creator struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

// ErrParticipantAlreadyExists indicates that a participant with the same participantID
var ErrParticipantAlreadyExists = errors.New("participant already exists")

func (h *Creator) Handle(ctx context.Context, cmd CreateCmd) error {
	if h.FCClient == nil {
		return client.ErrFederatedCatalogueNotConfigured
	}
	if cmd.Participant.ParticipantID == "" {
		return fmt.Errorf("participant id is empty")
	}

	// Check if the participant already exists.
	existsHandler := participantquery.ParticipantExistsHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}
	existsResp, err := existsHandler.Handle(participantquery.ParticipantExistsQry{
		ParticipantID: cmd.Participant.ParticipantID,
	})
	if err != nil {
		return err
	}
	if existsResp != nil && existsResp.Exists {
		return ErrParticipantAlreadyExists
	}

	// Build self-description and create the participant in the Federated Catalogue.
	jsonLD := selfdescription.BuildParticipantSelfDescription(cmd.Participant)

	body, err := json.Marshal(jsonLD)
	if err != nil {
		return fmt.Errorf("marshal participant template payload failed: %w", err)
	}

	resp, err := h.FCClient.Post(h.Ctx, client.ParticipantsEndpointPath, nil, body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("create participant failed with status %d", resp.StatusCode)
	}

	var fcResp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(resp.Body, &fcResp); err != nil {
		return fmt.Errorf("parse create participant response failed: %w", err)
	}
	if fcResp.ID == "" {
		return fmt.Errorf("create participant response id is empty")
	}

	return nil
}
