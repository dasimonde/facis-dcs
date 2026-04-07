package selfdescription

import (
	"digital-contracting-service/internal/base/datatype"
	tcselfdescription "digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
	"encoding/json"
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
	TemplateData   *datatype.JSON `db:"template_data"`
}

func BuildTemplateResourceSelfDescription(input TemplateResourceInput) map[string]interface{} {
	now := time.Now().UTC()
	templateID := buildTemplateResourceID(input.ParticipantID, input.DID, input.DocumentNumber, input.Version)
	templateDataJSONString := "{}"
	if input.TemplateData != nil {
		templateDataJSONString = string(*input.TemplateData)
	}

	createdAt := input.CreatedAt.UTC().Format(time.RFC3339)
	updatedAt := input.UpdatedAt.UTC().Format(time.RFC3339)
	schemaVersion := 1
	templateDataVersion := resolveTemplateDataVersion(input.TemplateData, 1)
	schemaContextURL := fmt.Sprintf("https://argo.asd-stack.eu/facis/dcs-semantic/template/v%d#", schemaVersion)
	templateDataContextURL := fmt.Sprintf("https://argo.asd-stack.eu/facis/dcs-semantic/template-data/v%d#", templateDataVersion)
	templateContext := map[string]interface{}{
		"gax-core":            "https://w3id.org/gaia-x/core#",
		"gax-trust-framework": "https://w3id.org/gaia-x/gax-trust-framework#",
		"dct":                 "http://purl.org/dc/terms/",
		"xsd":                 "http://www.w3.org/2001/XMLSchema#",
		"dcs-template":        schemaContextURL,
	}
	for k, v := range buildTemplateDataContext(templateDataVersion, templateDataContextURL) {
		templateContext[k] = v
	}

	verifiableCredential := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
			"https://www.w3.org/2018/credentials/examples/v1",
			"https://w3id.org/security/suites/jws-2020/v1",
		},
		"credentialSubject": map[string]interface{}{
			"@context": templateContext,
			"@id":      templateID,
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
			"dcs-template:templateData":     input.TemplateData,
			"dcs-template:templateDataJSON": templateDataJSONString,
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

func buildTemplateDataContext(version int, templateDataContextURL string) map[string]interface{} {
	switch version {
	case 1:
		return map[string]interface{}{
			"dcs-template-data":         templateDataContextURL,
			"dcs-template:templateData": "dcs-template:templateData",
			"type":                      "@type",
			"customMetaData":            "dcs-template-data:customMetaData",
			"name":                      "dcs-template-data:name",
			"value":                     "dcs-template-data:value",
			"documentBlocks":            "dcs-template-data:documentBlocks",
			"text":                      "dcs-template-data:text",
			"title":                     "dcs-template-data:title",
			"blockId":                   "dcs-template-data:blockId",
			"conditionIds":              "dcs-template-data:conditionIds",
			"documentOutline":           "dcs-template-data:documentOutline",
			"isRoot":                    "dcs-template-data:isRoot",
			"children":                  "dcs-template-data:children",
			"semanticConditions":        "dcs-template-data:semanticConditions",
			"parameters":                "dcs-template-data:parameters",
			"operators":                 "dcs-template-data:operators",
			"operate":                   "dcs-template-data:operate",
			"targets":                   "dcs-template-data:targets",
			"isRequired":                "dcs-template-data:isRequired",
			"parameterName":             "dcs-template-data:parameterName",
			"conditionId":               "dcs-template-data:conditionId",
			"conditionName":             "dcs-template-data:conditionName",
			"schemaVersion":             "dcs-template-data:schemaVersion",
			"subTemplateSnapshots":      "dcs-template-data:subTemplateSnapshots",
			"templateDataVersion":       "dcs-template-data:templateDataVersion",
			"did":                       "dcs-template-data:did",
			"templateId":                "dcs-template-data:templateId",
			"document_number":           "dcs-template-data:document_number",
			"version":                   "dcs-template-data:version",
			"template_data":             "dcs-template-data:template_data",
			"CLAUSE":                    "dcs-template-data:CLAUSE",
			"SECTION":                   "dcs-template-data:SECTION",
			"TEXT":                      "dcs-template-data:TEXT",
			"APPROVED_TEMPLATE":         "dcs-template-data:APPROVED_TEMPLATE",
			"date":                      "dcs-template-data:date",
			"string":                    "dcs-template-data:string",
			"integer":                   "dcs-template-data:integer",
			"decimal":                   "dcs-template-data:decimal",
		}
	default:
		return map[string]interface{}{
			"dcs-template-data": templateDataContextURL,
		}
	}
}

func resolveTemplateDataVersion(templateData *datatype.JSON, fallback int) int {
	if templateData == nil || !templateData.IsNotNullValue() {
		return fallback
	}

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal(*templateData, &templateDataMap); err != nil {
		return fallback
	}

	rawVersion, exists := templateDataMap["templateDataVersion"]
	if !exists || rawVersion == nil {
		return fallback
	}

	switch v := rawVersion.(type) {
	case float64:
		return int(v)
	default:
		return fallback
	}
}
