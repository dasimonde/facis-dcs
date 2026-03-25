package command

import (
	"context"
	"fmt"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/query"
)

type GetCurrentParticipantCmd struct {
	ParticipantID string
	Token         string
}

type GetCurrentParticipantResult struct {
	LegalName           string
	RegistrationNumber  string
	LeiCode             string
	EthereumAddress     string
	HeadquarterCountry  string
	HeadquarterStreet   string
	HeadquarterPostal   string
	HeadquarterLocality string
	LegalCountry        string
	LegalStreet         string
	LegalPostal         string
	LegalLocality       string
	TermsAndConditions  string
}

// GetCurrentParticipant handler fetches the current participant from the Federated Catalogue.
type GetCurrentParticipant struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *GetCurrentParticipant) Handle(cmd GetCurrentParticipantCmd) (*GetCurrentParticipantResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	handler := query.GetCurrentParticipantHandler{
		Ctx:      h.Ctx,
		FCClient: h.FCClient,
	}

	response, err := handler.Handle(query.GetCurrentParticipantQry{
		ParticipantID: cmd.ParticipantID,
		Token:         cmd.Token,
	})
	if err != nil {
		return nil, err
	}
	if response == nil {
		// Not found
		return nil, nil
	}

	return &GetCurrentParticipantResult{
		LegalName:           response.LegalName,
		RegistrationNumber:  response.RegistrationNumber,
		LeiCode:             response.LeiCode,
		EthereumAddress:     response.EthereumAddress,
		HeadquarterCountry:  response.HeadquarterAddress.Country,
		HeadquarterStreet:   response.HeadquarterAddress.StreetAddress,
		HeadquarterPostal:   response.HeadquarterAddress.PostalCode,
		HeadquarterLocality: response.HeadquarterAddress.Locality,
		LegalCountry:        response.LegalAddress.Country,
		LegalStreet:         response.LegalAddress.StreetAddress,
		LegalPostal:         response.LegalAddress.PostalCode,
		LegalLocality:       response.LegalAddress.Locality,
		TermsAndConditions:  response.TermsAndConditions,
	}, nil
}
