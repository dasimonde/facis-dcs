package participant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
)

type UpdateCmd struct {
	Participant selfdescription.ParticipantSdInput
}

// UpdateParticipant handler updates the current participant in the Federated Catalogue.
type Updater struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {
	if h.FCClient == nil {
		return client.ErrFederatedCatalogueNotConfigured
	}
	if cmd.Participant.ParticipantID == "" {
		return fmt.Errorf("participant id is empty")
	}

	jsonLD := selfdescription.BuildParticipantSelfDescription(cmd.Participant)

	body, err := json.Marshal(jsonLD)
	if err != nil {
		return fmt.Errorf("marshal participant template payload failed: %w", err)
	}

	path := client.ParticipantsEndpointPath + "/" + url.PathEscape(cmd.Participant.ParticipantID)
	resp, err := h.FCClient.Put(h.Ctx, path, nil, body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update participant failed with status %d", resp.StatusCode)
	}
	return nil
}
