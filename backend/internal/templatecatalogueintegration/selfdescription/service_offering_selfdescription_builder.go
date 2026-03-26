package selfdescription

import (
	"time"
)

// ServiceOfferingSdInput is the input required to build a Service Offering self-description JSON-LD.
type ServiceOfferingSdInput struct {
	ServiceOfferingID  string
	ParticipantID      string
	Description        string
	Keywords           []string
	EndPointURL        string
	TermsAndConditions string
}

// BuildServiceOfferingSelfDescription builds the Service Offering JSON-LD.
func BuildServiceOfferingSelfDescription(input ServiceOfferingSdInput) map[string]interface{} {
	now := time.Now().UTC()

	verifiableCredential := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"@id": input.ServiceOfferingID,
		"@type": []string{
			"VerifiableCredential",
		},
		"issuer":       input.ParticipantID,
		"issuanceDate": now.Format(time.RFC3339),
		"credentialSubject": map[string]interface{}{
			"gax-trust-framework:policy":       "",
			"gax-trust-framework:serviceTitle": "Digital Contracting Service",
			"gax-trust-framework:description":  input.Description,
			"dcat:keyword":                     input.Keywords,
			"@type":                            "gax-trust-framework:ServiceOffering",
			"gax-core:offeredBy": map[string]interface{}{
				"@id": input.ParticipantID,
			},
			"@id": input.ServiceOfferingID,
			"@context": map[string]interface{}{
				"gax-trust-framework": "https://w3id.org/gaia-x/gax-trust-framework#",
				"xsd":                 "http://www.w3.org/2001/XMLSchema#",
				"dcat":                "http://www.w3.org/ns/dcat#",
				"gax-core":            "https://w3id.org/gaia-x/core#",
			},
			"gax-trust-framework:dataAccountExport": map[string]interface{}{
				"gax-trust-framework:formatType":  "application/json",
				"@type":                           "gax-trust-framework:DataAccountExport",
				"gax-trust-framework:accessType":  "digital",
				"gax-trust-framework:requestType": "API",
			},
			"gax-trust-framework:termsAndConditions": map[string]interface{}{
				"gax-trust-framework:content": map[string]interface{}{
					"@value": input.TermsAndConditions,
					"@type":  "xsd:anyURI",
				},
				"@type": "gax-trust-framework:TermsAndConditions",
				// TODO: replace with the actual hash
				"gax-trust-framework:hash": "1234",
			},
			"gax-trust-framework:endPointURL":         input.EndPointURL,
			"gax-trust-framework:endpointDescription": "",
		},
	}

	verifiableCredential["proof"] = BuildProof(verifiableCredential, "assertionMethod")

	selfDescription := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"@id": input.ServiceOfferingID,
		"type": []string{
			"VerifiablePresentation",
		},
		"verifiableCredential": verifiableCredential,
	}
	selfDescription["proof"] = BuildProof(selfDescription, "assertionMethod")

	return selfDescription
}
