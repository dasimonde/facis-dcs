package command

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
)

type DeleteParticipantCmd struct {
	ID    string
	Token string
}

type DeleteParticipantResult struct {
	ID string
}

// DeleteParticipant handler deletes a participant from the Federated Catalogue.
type DeleteParticipant struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *DeleteParticipant) Handle(cmd DeleteParticipantCmd) (*DeleteParticipantResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	path := fcclient.ParticipantsEndpointPath + "/" + url.PathEscape(cmd.ID)

	resp, err := h.FCClient.Delete(h.Ctx, path, cmd.Token, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("delete participant failed with status %d", resp.StatusCode)
	}

	return &DeleteParticipantResult{
		ID: cmd.ID,
	}, nil
}
