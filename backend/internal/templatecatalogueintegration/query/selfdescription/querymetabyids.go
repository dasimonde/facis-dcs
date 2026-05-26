package selfdescription

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// GetSelfDescriptionsMetaByIDsQry fetches FC /self-descriptions metadata (meta.sdHash) by SelfDescription ids.
type GetSelfDescriptionsMetaByIDsQry struct {
	IDs []string
}

type GetSelfDescriptionsMetaByIDsResult struct {
	// SdHashByID maps SelfDescription id -> meta.sdHash.
	SdHashByID map[string]string
}

// GetSelfDescriptionsMetaByIDsHandler reads self-description metadata by ids.
type GetSelfDescriptionsMetaByIDsHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

func (h *GetSelfDescriptionsMetaByIDsHandler) Handle(qry GetSelfDescriptionsMetaByIDsQry) (*GetSelfDescriptionsMetaByIDsResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if len(qry.IDs) == 0 {
		return nil, fmt.Errorf("self-description ids is empty")
	}

	resp, err := h.FCClient.GetSelfDescriptions(h.Ctx, client.GetSelfDescriptionsRequest{
		IDs: qry.IDs,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.TotalCount == 0 || len(resp.Items) == 0 {
		return nil, nil
	}

	out := make(map[string]string)
	for _, item := range resp.Items {
		if item.Meta.ID == "" {
			continue
		}
		if item.Meta.SdHash == "" {
			continue
		}
		out[item.Meta.ID] = item.Meta.SdHash
	}
	if len(out) == 0 {
		return nil, nil
	}

	return &GetSelfDescriptionsMetaByIDsResult{SdHashByID: out}, nil
}
