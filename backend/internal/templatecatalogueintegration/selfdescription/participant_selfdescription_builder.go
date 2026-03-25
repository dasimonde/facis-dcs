package selfdescription

import (
	"time"
)

// ParticipantSdInput is the shared input needed to build a Participant self-description JSON-LD.
// It is intended to be reused by both create and update flows.
type ParticipantSdInput struct {
	ParticipantID      string
	LegalName          string
	RegistrationNumber string
	LeiCode            string
	EthereumAddress    string
	// headquarterAddress
	HeadquarterCountry       string
	HeadquarterStreetAddress string
	HeadquarterPostalCode    string
	HeadquarterLocality      string
	// legalAddress
	LegalAddressCountry       string
	LegalAddressStreetAddress string
	LegalAddressPostalCode    string
	LegalAddressLocality      string
	// TermsAndConditions
	TermsAndConditions string
}

// BuildParticipantSelfDescription builds the Participant JSON-LD.
func BuildParticipantSelfDescription(input ParticipantSdInput) map[string]interface{} {
	now := time.Now().UTC()
	verifiableCredential := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"@id": input.ParticipantID,
		"type": []string{
			"VerifiableCredential",
		},
		"issuer":       input.ParticipantID,
		"issuanceDate": now.Format(time.RFC3339),
		"credentialSubject": map[string]interface{}{
			"@context": map[string]interface{}{
				"gx-participant":      "http://w3id.org/gaia-x/participant#",
				"gx-service-offering": "http://w3id.org/gaia-x/service#",
				"xsd":                 "http://www.w3.org/2001/XMLSchema#",
			},
			"id": input.ParticipantID,
			"@type": []string{
				"https://w3id.org/gaia-x/core#Participant",
			},
			"gx-participant:legalName": map[string]interface{}{
				"@value": input.LegalName,
				"@type":  "xsd:string",
			},
			"gx-participant:registrationNumber": map[string]interface{}{
				"@value": input.RegistrationNumber,
				"@type":  "xsd:string",
			},
			"gx-participant:leiCode": map[string]interface{}{
				"@value": input.LeiCode,
				"@type":  "xsd:string",
			},
			"gx-participant:ethereumAddress": map[string]interface{}{
				"@value": input.EthereumAddress,
				"@type":  "xsd:string",
			},
			"gx-participant:headquarterAddress": map[string]interface{}{
				"@type": "gx-participant:Address",
				"gx-participant:country": map[string]interface{}{
					"@value": input.HeadquarterCountry,
					"@type":  "xsd:string",
				},
				"gx-participant:street-address": map[string]interface{}{
					"@value": input.HeadquarterStreetAddress,
					"@type":  "xsd:string",
				},
				"gx-participant:postal-code": map[string]interface{}{
					"@value": input.HeadquarterPostalCode,
					"@type":  "xsd:string",
				},
				"gx-participant:locality": map[string]interface{}{
					"@value": input.HeadquarterLocality,
					"@type":  "xsd:string",
				},
			},
			"gx-participant:legalAddress": map[string]interface{}{
				"@type": "gx-participant:Address",
				"gx-participant:country": map[string]interface{}{
					"@value": input.LegalAddressCountry,
					"@type":  "xsd:string",
				},
				"gx-participant:street-address": map[string]interface{}{
					"@value": input.LegalAddressStreetAddress,
					"@type":  "xsd:string",
				},
				"gx-participant:postal-code": map[string]interface{}{
					"@value": input.LegalAddressPostalCode,
					"@type":  "xsd:string",
				},
				"gx-participant:locality": map[string]interface{}{
					"@value": input.LegalAddressLocality,
					"@type":  "xsd:string",
				},
			},
			"gx-service-offering:TermsAndConditions": map[string]interface{}{
				"gx-service-offering:url": map[string]interface{}{
					"@value": input.TermsAndConditions,
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

	verifiableCredential["proof"] = BuildProof(verifiableCredential, "assertionMethod")

	selfDescription := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"@id": input.ParticipantID,
		"type": []string{
			"VerifiablePresentation",
		},
		"verifiableCredential": []interface{}{
			verifiableCredential,
		},
	}
	selfDescription["proof"] = BuildProof(selfDescription, "authentication")

	return selfDescription
}

// TODO: replace with the actual verification method
// BuildProof returns a proof template.
func BuildProof(_ map[string]interface{}, proofPurpose string) map[string]interface{} {
	now := time.Now().UTC()

	proof := map[string]interface{}{
		"type":               "JsonWebSignature2020",
		"created":            now.Format(time.RFC3339),
		"proofPurpose":       proofPurpose,
		"verificationMethod": "did:web:argo.asd-stack.eu#key-1",
		"jws":                "eyJhbGciOiJSUzI1NiIsImI2NCI6ZmFsc2UsImNyaXQiOlsiYjY0Il19..kTCYt5XsITJX1CxPCT8yAV-TVIw5WEuts01mqpQy7UJiN5mgREEMGlv50aqzpqh4Qq_PbChOMqsLfRoPsnsgxD-WUcX16dUOqV0G_zS245-kronKb78cPktb3rk-BuQy72IFLN25DYuNzVBAh4vGHSrQyHUGlcTwLtjPAnKb78",
	}
	if proofPurpose == "authentication" {
		proof["challenge"] = "1f44d55f-f161-4938-a659-f8026467f126"
		proof["domain"] = "4jt78h47fh47"
	}
	return proof
}
