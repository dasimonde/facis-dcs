package fcasset

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/templaterepository/datatype/partytype"
)

const (
	CredentialsV2Context = "https://www.w3.org/ns/credentials/v2" // #nosec G101 -- identifier or vocabulary IRI, not a credential
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
	PartyType    string
	// The searchable metadata an importing instance takes over (DCS-FR-TR-10, DCS-FR-TR-02).
	Jurisdiction  string
	Industry      string
	TemplateLevel string
}

// BuildInput carries catalogue metadata required for an FC /assets JSON-LD payload.
type BuildInput struct {
	Issuer             string
	SubjectIRI         string
	ValidFrom          time.Time
	Subject            CatalogueSubject
	TemplateDataString string
}

// BuildPayload assembles a thin catalogue VC for FC publish.
func BuildPayload(input BuildInput) (map[string]any, error) {
	if _, err := partytype.New(input.Subject.PartyType); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.SubjectIRI) == "" {
		return nil, fmt.Errorf("template subject iri is empty")
	}

	if strings.TrimSpace(input.Subject.ID) == "" {
		return nil, fmt.Errorf("template did is empty")
	}

	if strings.TrimSpace(input.Issuer) == "" {
		return nil, fmt.Errorf("issuer is empty")
	}

	claims := map[string]any{
		"dcs:templateUuid":       input.Subject.ID,
		"dcs:state":              input.Subject.State,
		"dcs:templateType":       input.Subject.TemplateType,
		"dcs:partyType":          input.Subject.PartyType,
		"schema:name":            input.Subject.Name,
		"schema:description":     input.Subject.Description,
		"schema:version":         input.Subject.Version,
		"dcs:templateDataString": input.TemplateDataString,
	}
	for key, value := range map[string]string{
		"dcs:jurisdiction":  input.Subject.Jurisdiction,
		"dcs:industry":      input.Subject.Industry,
		"dcs:templateLevel": input.Subject.TemplateLevel,
	} {
		if value != "" {
			claims[key] = value
		}
	}
	return credential(input.Issuer, input.SubjectIRI, input.ValidFrom, "dcs:ContractTemplate", claims), nil
}

// ContractSubject is the contract metadata published to FC (DCS-IR-CWE-10). It
// holds no contract content and no party.
type ContractSubject struct {
	ID         string
	State      string
	Name       string
	Version    int
	TemplateID string
	StartDate  *time.Time
	EndDate    *time.Time
}

// BuildContractPayload assembles a thin catalogue VC for a contract.
func BuildContractPayload(issuer, subjectIRI string, validFrom time.Time, subject ContractSubject) (map[string]any, error) {
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(subjectIRI) == "" || strings.TrimSpace(subject.ID) == "" {
		return nil, fmt.Errorf("issuer, contract subject iri and contract did are required")
	}
	claims := map[string]any{
		"dcs:contractUuid":        subject.ID,
		"dcs:state":               strings.ToLower(subject.State),
		"schema:name":             subject.Name,
		"schema:version":          strconv.Itoa(subject.Version),
		"dcs:derivedFromTemplate": subject.TemplateID,
	}
	if subject.StartDate != nil {
		claims["schema:startDate"] = subject.StartDate.UTC().Format(time.RFC3339)
	}
	if subject.EndDate != nil {
		claims["schema:endDate"] = subject.EndDate.UTC().Format(time.RFC3339)
	}
	return credential(issuer, subjectIRI, validFrom, "dcs:Contract", claims), nil
}

// credential wraps claims about subjectIRI in the unsecured VC envelope FC
// ingests.
func credential(issuer, subjectIRI string, validFrom time.Time, subjectType string, claims map[string]any) map[string]any {
	validFrom = validFrom.UTC()
	if validFrom.After(time.Now().UTC()) {
		validFrom = time.Now().UTC()
	}

	// credentialSubject's own "id" becomes the RDF subject for every
	// property below once FC ingests this as JSON-LD — it must be a
	// well-formed absolute IRI (base.ResourceIRI), not the bare UUID: FC's
	// JsonLdToRdf silently drops ("Non well-formed subject... has been
	// skipped") every triple whose subject isn't one, confirmed live. The
	// bare UUID travels separately as a plain literal property.
	subject := map[string]any{"id": subjectIRI, "type": subjectType}
	for key, value := range claims {
		subject[key] = value
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
		"id": subjectIRI,
		// The VerifiableCredential type is load-bearing twice over, so it stays
		// even though no proof is attached. In the VC 2.0 context `issuer`,
		// `validFrom` and `credentialSubject` are defined only inside that
		// type's scoped @context — drop the type and JSON-LD expansion discards
		// the whole credentialSubject, so FC ingests an empty node and publish
		// still answers 200. FC then reifies each ingested triple with
		// credentialSubject (see templatecatalogueintegration's sparql.go), which
		// the read path joins on, so retrieval depends on FC having parsed this
		// as a credential too.
		//
		// What is genuinely missing is the proof, not the type: this is an
		// unsecured self-description and nothing downstream should treat its
		// contents as attested. Signing it is the fix; untyping it only breaks
		// the catalogue.
		"type":              []string{"VerifiableCredential", subjectType},
		"issuer":            issuer,
		"validFrom":         validFrom.Format(time.RFC3339),
		"credentialSubject": subject,
	}
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
	partyType string,
) CatalogueSubject {
	return CatalogueSubject{
		ID:           did,
		State:        strings.ToLower(strings.TrimSpace(state)),
		Name:         name,
		Description:  description,
		Version:      strconv.Itoa(version),
		TemplateType: templateType,
		PartyType:    partyType,
	}
}
