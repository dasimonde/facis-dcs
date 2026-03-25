package query

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
)

// GetCurrentParticipantQry represents the input required to fetch the current participant projection.
type GetCurrentParticipantQry struct {
	ParticipantID string
	Token         string
}

type AddressResponse struct {
	Country       string
	StreetAddress string
	PostalCode    string
	Locality      string
}

// GetParticipantResponse is the FC /query projection result consumed by the service layer.
type GetParticipantResponse struct {
	LegalName          string
	RegistrationNumber string
	LeiCode            string
	EthereumAddress    string
	HeadquarterAddress AddressResponse
	LegalAddress       AddressResponse
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

func (h *GetCurrentParticipantHandler) Handle(qry GetCurrentParticipantQry) (*GetParticipantResponse, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
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

	queryResp, err := h.FCClient.Query(h.Ctx, qry.Token, reqBody)
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

	return &GetParticipantResponse{
		LegalName:          derefString(participant, "legal_name"),
		RegistrationNumber: derefString(participant, "registration_number"),
		LeiCode:            derefString(participant, "lei_code"),
		EthereumAddress:    derefString(participant, "ethereum_address"),
		HeadquarterAddress: AddressResponse{
			Country:       derefString(hq, "country"),
			StreetAddress: derefString(hq, "street_address"),
			PostalCode:    derefString(hq, "postal_code"),
			Locality:      derefString(hq, "locality"),
		},
		LegalAddress: AddressResponse{
			Country:       derefString(la, "country"),
			StreetAddress: derefString(la, "street_address"),
			PostalCode:    derefString(la, "postal_code"),
			Locality:      derefString(la, "locality"),
		},
		TermsAndConditions: derefString(participant, "terms_and_conditions"),
	}, nil
}

// derefString extracts a string field from an FC query projection.
// If the field is missing or not a string, it returns an empty string.
func derefString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

