package participant

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
)

type DeleteCmd struct {
	ID string
}

// DeleteParticipant handler deletes a participant from the Federated Catalogue.
type Deleter struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *Deleter) Handle(ctx context.Context, cmd DeleteCmd) error {
	if h.FCClient == nil {
		return fcclient.ErrFederatedCatalogueNotConfigured
	}
	if cmd.ID == "" {
		return fmt.Errorf("participant id is empty")
	}
	// The participant graph node won't be deleted if other SDs depend on it.
	if err := h.deleteOtherSelfDescriptionsByIDs(cmd.ID); err != nil {
		return err
	}
	path := fcclient.ParticipantsEndpointPath + "/" + url.PathEscape(cmd.ID)

	resp, err := h.FCClient.Delete(h.Ctx, path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("delete participant failed with status %d", resp.StatusCode)
	}

	return nil
}

// deleteOtherSelfDescriptionsByIDs deletes all SDs except the participant's own SD.
func (h *Deleter) deleteOtherSelfDescriptionsByIDs(participantID string) error {
	sdResp, err := h.FCClient.GetSelfDescriptions(h.Ctx, fcclient.GetSelfDescriptionsRequest{
		WithContent: false,
	})
	if err != nil {
		return err
	}

	for _, item := range sdResp.Items {
		if item.Meta.ID == participantID {
			continue
		}
		sdHash := item.Meta.SdHash
		if sdHash == "" {
			continue
		}

		path := fcclient.SelfDescriptionsEndpointPath + "/" + url.PathEscape(sdHash)
		delResp, err := h.FCClient.Delete(h.Ctx, path, nil)
		if err != nil {
			return err
		}
		if delResp.StatusCode == http.StatusNotFound {
			continue
		}
		if delResp.StatusCode != http.StatusOK && (delResp.StatusCode < 200 || delResp.StatusCode >= 300) {
			return fmt.Errorf("delete self-description failed with status %d", delResp.StatusCode)
		}
	}

	return nil
}
