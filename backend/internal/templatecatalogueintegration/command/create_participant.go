package command

import (
	"context"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CreateParticipantCmd struct {
	Token               string
	ParticipantID       string
	LegalName           string
	RegistrationNumber  string
	LeiCode             string
	EthereumAddress     string
	HeadquarterCountry  string
	HeadquarterStreet   string
	HeadquarterPostal   string
	HeadquarterLocality string
	LegalStreet         string
	LegalPostal         string
	LegalLocality       string
	TermsAndConditions  string
}

type CreateParticipantResult struct {
	ID string
}

type CreateParticipant struct {
	Ctx      context.Context
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *CreateParticipant) Handle(cmd CreateParticipantCmd) (*CreateParticipantResult, error) {
	if h.FCClient == nil {
		return nil, fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.ParticipantID == "" {
		return nil, fmt.Errorf("participant id is empty")
	}

	jsonLD := buildCreateParticipantJSONLD(cmd, h.FCClient)

	body, err := json.Marshal(jsonLD)
	if err != nil {
		return nil, fmt.Errorf("marshal participant template payload failed: %w", err)
	}

	resp, err := h.FCClient.Post(h.Ctx, fcclient.ParticipantsEndpointPath, cmd.Token, nil, body)
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

	return &CreateParticipantResult{
		ID: fcResp.ID,
	}, nil
}

func buildCreateParticipantJSONLD(cmd CreateParticipantCmd, fc *fcclient.FederatedCatalogueClient) map[string]interface{} {
	now := time.Now().UTC()
	verifiableCredential := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"@id": cmd.ParticipantID,
		"type": []string{
			"VerifiableCredential",
		},
		"issuer":       cmd.ParticipantID,
		"issuanceDate": now.Format(time.RFC3339),
		"credentialSubject": map[string]interface{}{
			"@context": map[string]interface{}{
				"gx-participant":      "http://w3id.org/gaia-x/participant#",
				"gx-service-offering": "http://w3id.org/gaia-x/service#",
				"xsd":                 "http://www.w3.org/2001/XMLSchema#",
			},
			"id": cmd.ParticipantID,
			"@type": []string{
				"https://w3id.org/gaia-x/core#Participant",
			},
			"gx-participant:legalName": map[string]interface{}{
				"@value": cmd.LegalName,
				"@type":  "xsd:string",
			},
			"gx-participant:registrationNumber": map[string]interface{}{
				"@value": cmd.RegistrationNumber,
				"@type":  "xsd:string",
			},
			"gx-participant:leiCode": map[string]interface{}{
				"@value": cmd.LeiCode,
				"@type":  "xsd:string",
			},
			"gx-participant:ethereumAddress": map[string]interface{}{
				"@value": cmd.EthereumAddress,
				"@type":  "xsd:string",
			},
			"gx-participant:headquarterAddress": map[string]interface{}{
				"@type": "gx-participant:Address",
				"gx-participant:country": map[string]interface{}{
					"@value": cmd.HeadquarterCountry,
					"@type":  "xsd:string",
				},
				"gx-participant:street-address": map[string]interface{}{
					"@value": cmd.HeadquarterStreet,
					"@type":  "xsd:string",
				},
				"gx-participant:postal-code": map[string]interface{}{
					"@value": cmd.HeadquarterPostal,
					"@type":  "xsd:string",
				},
				"gx-participant:locality": map[string]interface{}{
					"@value": cmd.HeadquarterLocality,
					"@type":  "xsd:string",
				},
			},
			"gx-participant:legalAddress": map[string]interface{}{
				"@type": "gx-participant:Address",
				"gx-participant:country": map[string]interface{}{
					"@value": cmd.HeadquarterCountry,
					"@type":  "xsd:string",
				},
				"gx-participant:street-address": map[string]interface{}{
					"@value": cmd.LegalStreet,
					"@type":  "xsd:string",
				},
				"gx-participant:postal-code": map[string]interface{}{
					"@value": cmd.LegalPostal,
					"@type":  "xsd:string",
				},
				"gx-participant:locality": map[string]interface{}{
					"@value": cmd.LegalLocality,
					"@type":  "xsd:string",
				},
			},
			"gx-service-offering:TermsAndConditions": map[string]interface{}{
				"gx-service-offering:url": map[string]interface{}{
					"@value": cmd.TermsAndConditions,
					"@type":  "xsd:string",
				},
				"gx-service-offering:hash": map[string]interface{}{
					// TODO: replace with the actual hash
					"@value": "36ba819f30a3c4d4a7f16ee0a77259fc92f2e1ebf739713609f1c11eb41499e7aa2cd3a5d2011e073f9ba9c107493e3e8629cc15cd4fc07f67281d7ea9023db0",
					"@type":  "xsd:string",
				},
			},
		},
	}
	verifiableCredential["proof"] = fc.BuildProof(verifiableCredential, "assertionMethod")

	selfDescription := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"@id": cmd.ParticipantID,
		"type": []string{
			"VerifiablePresentation",
		},
		"verifiableCredential": []interface{}{
			verifiableCredential,
		},
	}
	selfDescription["proof"] = fc.BuildProof(selfDescription, "authentication")
	return selfDescription
}
