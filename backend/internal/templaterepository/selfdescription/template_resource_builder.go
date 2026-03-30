package selfdescription

import (
	tcselfdescription "digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
	"fmt"
	"strings"
	"time"
)

type TemplateResourceInput struct {
	ParticipantID  string
	DID            string
	DocumentNumber string
	Version        int
	TemplateType   string
	Name           string
	Description    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func BuildTemplateResourceSelfDescription(input TemplateResourceInput) map[string]interface{} {
	now := time.Now().UTC()
	templateID := buildTemplateResourceID(input.ParticipantID, input.DID, input.DocumentNumber, input.Version)

	createdAt := input.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := input.UpdatedAt.UTC().Format(time.RFC3339)
	schemaVersion := 1
	schemaContextURL := fmt.Sprintf("https://argo.asd-stack.eu/facis/dcs-semantic/template/v%d#", schemaVersion)

	verifiableCredential := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
			"https://www.w3.org/2018/credentials/examples/v1",
			"https://w3id.org/security/suites/jws-2020/v1",
		},
		"credentialSubject": map[string]interface{}{
			"@context": map[string]interface{}{
				"gax-core":            "https://w3id.org/gaia-x/core#",
				"gax-trust-framework": "https://w3id.org/gaia-x/gax-trust-framework#",
				"dct":                 "http://purl.org/dc/terms/",
				"xsd":                 "http://www.w3.org/2001/XMLSchema#",
				"dcs-template":        schemaContextURL,
			},
			"@id": templateID,
			"@type": []string{
				"gax-trust-framework:Resource",
				"dcs-template:ContractTemplate",
			},
			"gax-core:operatedBy": map[string]interface{}{
				"@id": input.ParticipantID,
			},
			"gax-trust-framework:name": map[string]interface{}{
				"@type":  "xsd:string",
				"@value": input.Name,
			},
			"dct:description": map[string]interface{}{
				"@type":  "xsd:string",
				"@value": input.Description,
			},
			"dcs-template:name":           input.Name,
			"dcs-template:did":            input.DID,
			"dcs-template:documentNumber": input.DocumentNumber,
			"dcs-template:version":        input.Version,
			"dcs-template:schemaVersion":  schemaVersion,
			"dcs-template:templateType":   input.TemplateType,
			"dcs-template:description":    input.Description,
			"dcs-template:participantId":  input.ParticipantID,
			"dcs-template:createdAt": map[string]interface{}{
				"@type":  "xsd:dateTime",
				"@value": createdAt,
			},
			"dcs-template:updatedAt": map[string]interface{}{
				"@type":  "xsd:dateTime",
				"@value": updatedAt,
			},
		},
		"expirationDate": "2034-01-23T11:29:40Z",
		"issuanceDate":   now.Format(time.RFC3339),
		"issuer":         input.ParticipantID,
		"type": []string{
			"VerifiableCredential",
		},
	}
	verifiableCredential["proof"] = tcselfdescription.BuildProof(verifiableCredential, "assertionMethod")

	selfDescription := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
			"https://w3id.org/security/suites/jws-2020/v1",
		},
		"holder": input.ParticipantID,
		"id":     templateID,
		"type": []string{
			"VerifiablePresentation",
		},
		"verifiableCredential": []interface{}{
			verifiableCredential,
		},
	}
	selfDescription["proof"] = tcselfdescription.BuildProof(selfDescription, "assertionMethod")
	return selfDescription
}

// buildTemplateResourceID builds the id for the template resource.
// example participantID:  did:web:argo.asd-stack.eu:participant:1a3ab67b-237b-4375-95a3-ad06165bb528
// example did: 					 2eeb2d07-8492-4bab-868a-00bfbaf038c2
// example documentNumber: 35625b2b-b5e0-46fc-8d69-4fe38d9f036d
// example version: 			 1
// example result: 				 did:web:argo.asd-stack.eu:contract-template:2eeb2d07-8492-4bab-868a-00bfbaf038c2:35625b2b-b5e0-46fc-8d69-4fe38d9f036d:1
func buildTemplateResourceID(participantID, did, documentNumber string, version int) string {
	// participantId could be either a DID or a URL.
	base := strings.ReplaceAll(participantID, "/participant/", "/template/")
	base = strings.ReplaceAll(base, ":participant:", ":template:")

	if strings.Contains(base, "://") {
		trimmed := strings.TrimRight(base, "/")
		lastSlash := strings.LastIndex(trimmed, "/")
		if lastSlash == -1 {
			return fmt.Sprintf("%s/%s:%s:%d", trimmed, did, documentNumber, version)
		}
		return fmt.Sprintf("%s/%s:%s:%d", trimmed[:lastSlash], did, documentNumber, version)
	}

	parts := strings.Split(base, ":")
	if len(parts) == 0 {
		return fmt.Sprintf("%s:%s:%s:%d", base, did, documentNumber, version)
	}
	parts = parts[:len(parts)-1]
	parts = append(parts, did, documentNumber, fmt.Sprintf("%d", version))
	return strings.Join(parts, ":")
}
