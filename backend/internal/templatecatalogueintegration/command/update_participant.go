package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
)

type UpdateParticipantCmd struct {
	Token       string
	Participant selfdescription.ParticipantSdInput
}

type UpdateParticipantResult struct {
	ID string
}

// UpdateParticipant handler updates the current participant in the Federated Catalogue.
type UpdateParticipant struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *UpdateParticipant) Handle(cmd UpdateParticipantCmd) (*UpdateParticipantResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.Participant.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	jsonLD := selfdescription.BuildParticipantSelfDescription(cmd.Participant)

	body, err := json.Marshal(jsonLD)
	if err != nil {
		return nil, fmt.Errorf("marshal participant template payload failed: %w", err)
	}

	path := client.ParticipantsEndpointPath + "/" + url.PathEscape(cmd.Participant.ParticipantID)
	resp, err := h.FCClient.Put(h.Ctx, path, cmd.Token, nil, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update participant failed with status %d", resp.StatusCode)
	}

	var fcResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Body, &fcResp); err != nil {
		return &UpdateParticipantResult{ID: cmd.Participant.ParticipantID}, nil
	}

	if fcResp.ID == "" {
		return &UpdateParticipantResult{ID: cmd.Participant.ParticipantID}, nil
	}
	return &UpdateParticipantResult{ID: fcResp.ID}, nil
}
