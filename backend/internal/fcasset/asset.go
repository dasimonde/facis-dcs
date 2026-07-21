package fcasset

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
)

const (
	CredentialsV2Context = "https://www.w3.org/ns/credentials/v2"
	DCSContextURL        = "https://w3id.org/facis/dcs/1#"
	SchemaContextURL     = "https://schema.org/"
	ProvContextURL       = "http://www.w3.org/ns/prov#"
)

// CatalogueSubject is the credentialSubject published to FC.
type CatalogueSubject struct {
	ID           string
	State        string
	Name         string
	Description  string
	Version      string
	TemplateType string
}

// BuildInput carries catalogue metadata required for an FC /assets JSON-LD payload.
type BuildInput struct {
	Issuer             string
	ValidFrom          time.Time
	Subject            CatalogueSubject
	TemplateDataString string
}

// BuildPayload assembles a thin catalogue VC for FC publish.
func BuildPayload(input BuildInput) (map[string]any, error) {
	if strings.TrimSpace(input.Subject.ID) == "" {
		return nil, fmt.Errorf("template did is empty")
	}

	if strings.TrimSpace(input.Issuer) == "" {
		return nil, fmt.Errorf("issuer is empty")
	}

	validFrom := input.ValidFrom.UTC()
	if validFrom.After(time.Now().UTC()) {
		validFrom = time.Now().UTC()
	}

	return map[string]any{
		"@context": []any{
			CredentialsV2Context,
			map[string]any{
				"dcs":    DCSContextURL,
				"schema": SchemaContextURL,
				"prov":   ProvContextURL,
			},
		},
		"id": input.Subject.ID,
		"type": []string{
			"VerifiableCredential",
			"dcs:ContractTemplate",
		},
		"issuer":    input.Issuer,
		"validFrom": validFrom.Format(time.RFC3339),
		"credentialSubject": map[string]any{
			"id":                     input.Subject.ID,
			"type":                   "dcs:ContractTemplate",
			"dcs:templateUuid":       input.Subject.ID,
			"dcs:state":              input.Subject.State,
			"dcs:templateType":       input.Subject.TemplateType,
			"schema:name":            input.Subject.Name,
			"schema:description":     input.Subject.Description,
			"schema:version":         input.Subject.Version,
			"dcs:templateDataString": input.TemplateDataString,
		},
	}, nil
}

// TemplateDataString serializes persisted template_data for FC graph storage.
func TemplateDataString(templateData *datatype.JSON) (string, error) {
	if templateData == nil || !templateData.IsNotNullValue() {
		return "", fmt.Errorf("template data is empty")
	}

	raw := []byte(*templateData)
	if !json.Valid(raw) {
		return "", fmt.Errorf("template data is not valid JSON")
	}

	return string(raw), nil
}

// CatalogueSubjectFromRepository builds FC catalogue metadata from a local template.
func CatalogueSubjectFromRepository(
	did string,
	version int,
	state string,
	name string,
	description string,
	templateType string,
) CatalogueSubject {
	return CatalogueSubject{
		ID:           did,
		State:        strings.ToLower(strings.TrimSpace(state)),
		Name:         name,
		Description:  description,
		Version:      strconv.Itoa(version),
		TemplateType: templateType,
	}
}
