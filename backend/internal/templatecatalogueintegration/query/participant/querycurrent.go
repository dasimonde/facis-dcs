package participant

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

// GetCurrentParticipantQry represents the input required to fetch the current participant projection.
type GetCurrentParticipantQry struct {
	ParticipantID string
}

type AddressResult struct {
	Country       string
	StreetAddress string
	PostalCode    string
	Locality      string
}

// GetCurrentParticipantResult is the FC /query projection result consumed by the service layer.
type GetCurrentParticipantResult struct {
	LegalName          string
	RegistrationNumber string
	LeiCode            string
	EthereumAddress    string
	HeadquarterAddress AddressResult
	LegalAddress       AddressResult
	TermsAndConditions string
}

// GetCurrentParticipantHandler fetches the current participant projection from the Federated Catalogue.
type GetCurrentParticipantHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const getCurrentParticipantStatement = `
MATCH (p:Participant)
WHERE p.uri = $participantId
OPTIONAL MATCH (p)-[:headquarterAddress]->(hq)
OPTIONAL MATCH (p)-[:legalAddress]->(la)
OPTIONAL MATCH (p)-[:TermsAndConditions]->(tc)
RETURN {
  legal_name: p.legalName,
  registration_number: p.registrationNumber,
  lei_code: p.leiCode,
  ethereum_address: p.ethereumAddress,
  headquarter_address: {
    country: hq.country,
    street_address: hq["street-address"],
    postal_code: hq["postal-code"],
    locality: hq.locality,
    legal_address: {
      country: la.country,
      street_address: la["street-address"],
      postal_code: la["postal-code"],
      locality: la.locality
    }
  },
  terms_and_conditions: tc.url
} AS n
LIMIT 1
`

func (h *GetCurrentParticipantHandler) Handle(qry GetCurrentParticipantQry) (*GetCurrentParticipantResult, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	reqBody := client.QueryRequest{
		Statement: getCurrentParticipantStatement,
		Parameters: map[string]string{
			"participantId": qry.ParticipantID,
		},
	}

	queryResp, err := h.FCClient.Query(h.Ctx, reqBody)
	if err != nil {
		return nil, err
	}

	if queryResp.TotalCount == 0 || len(queryResp.Items) == 0 {
		// Not found
		return nil, nil
	}

	var participant map[string]interface{}
	for _, v := range queryResp.Items[0] {
		if m, ok := v.(map[string]interface{}); ok {
			participant = m
			break
		}
	}
	if participant == nil {
		return nil, fmt.Errorf("query projection missing projected map for participantId=%s", qry.ParticipantID)
	}

	hq, ok := participant["headquarter_address"].(map[string]interface{})
	if !ok {
		hq = map[string]interface{}{}
	}

	la, ok := hq["legal_address"].(map[string]interface{})
	if !ok {
		la = map[string]interface{}{}
	}

	return &GetCurrentParticipantResult{
		LegalName:          ptr.StringFromMap(participant, "legal_name"),
		RegistrationNumber: ptr.StringFromMap(participant, "registration_number"),
		LeiCode:            ptr.StringFromMap(participant, "lei_code"),
		EthereumAddress:    ptr.StringFromMap(participant, "ethereum_address"),
		HeadquarterAddress: AddressResult{
			Country:       ptr.StringFromMap(hq, "country"),
			StreetAddress: ptr.StringFromMap(hq, "street_address"),
			PostalCode:    ptr.StringFromMap(hq, "postal_code"),
			Locality:      ptr.StringFromMap(hq, "locality"),
		},
		LegalAddress: AddressResult{
			Country:       ptr.StringFromMap(la, "country"),
			StreetAddress: ptr.StringFromMap(la, "street_address"),
			PostalCode:    ptr.StringFromMap(la, "postal_code"),
			Locality:      ptr.StringFromMap(la, "locality"),
		},
		TermsAndConditions: ptr.StringFromMap(participant, "terms_and_conditions"),
	}, nil
}
