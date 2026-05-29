package template

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

type GetByIDQry struct {
	DID     string
	Version int
}

type GetByIDHandler struct {
	Ctx      context.Context
	FCClient *client.FederatedCatalogueClient
}

const retrieveTemplateByIDStatement = `
MATCH (ct:ContractTemplate)
WHERE ct.did = $did AND ct.version = $version
OPTIONAL MATCH (ct)-[:operatedBy]->(p:Participant)
OPTIONAL MATCH (p)-[:headquarterAddress]->(hq)
OPTIONAL MATCH (p)-[:TermsAndConditions]->(tc)
RETURN {
  did: ct.did,
  document_number: ct.documentNumber,
  version: ct.version,
  schema_version: ct.schemaVersion,
  template_data_json: ct.templateDataJSON,
  name: ct.name,
  description: ct.description,
  template_type: ct.templateType,
  participant_id: p.uri,
  participant: {
    legal_name: p.legalName,
    registration_number: p.registrationNumber,
    lei_code: p.leiCode,
    headquarter_address: {
      country: hq.country,
      locality: hq.locality
    },
    terms_and_conditions: tc.url
  },
  created_at: ct.createdAt,
  updated_at: ct.updatedAt
} AS n
LIMIT 1
`

func (h *GetByIDHandler) Handle(qry GetByIDQry) (*templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, error) {
	if h.FCClient == nil {
		return nil, client.ErrFederatedCatalogueNotConfigured
	}
	if qry.DID == "" {
		return nil, fmt.Errorf("did is empty")
	}
	if qry.Version < 1 {
		return nil, fmt.Errorf("version must be greater than 0")
	}

	resp, err := h.FCClient.Query(h.Ctx, client.QueryRequest{
		Statement: retrieveTemplateByIDStatement,
		Parameters: map[string]string{
			"did":     qry.DID,
			"version": strconv.Itoa(qry.Version),
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.TotalCount == 0 || len(resp.Items) == 0 {
		return nil, nil
	}

	var n map[string]interface{}
	for _, v := range resp.Items[0] {
		if m, ok := v.(map[string]interface{}); ok {
			n = m
			break
		}
	}
	if n == nil {
		return nil, fmt.Errorf("query projection missing projected map for did=%s version=%d", qry.DID, qry.Version)
	}

	templateData, err := parseTemplateDataJSON(ptr.StringFromMap(n, "template_data_json"))
	if err != nil {
		return nil, err
	}

	return &templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse{
		Did:            ptr.StringFromMap(n, "did"),
		DocumentNumber: ptr.Ref(ptr.StringFromMap(n, "document_number")),
		Version:        ptr.Ref(ptr.IntFromMap(n, "version")),
		SchemaVersion:  ptr.Ref(ptr.IntFromMap(n, "schema_version")),
		TemplateData:   templateData,
		Name:           ptr.Ref(ptr.StringFromMap(n, "name")),
		Description:    ptr.Ref(ptr.StringFromMap(n, "description")),
		TemplateType:   ptr.Ref(ptr.StringFromMap(n, "template_type")),
		Participant:    mapTemplateParticipantSummary(n),
		CreatedAt:      ptr.Ref(ptr.StringFromMap(n, "created_at")),
		UpdatedAt:      ptr.Ref(ptr.StringFromMap(n, "updated_at")),
	}, nil
}

func parseTemplateDataJSON(templateDataJSON string) (any, error) {
	if strings.TrimSpace(templateDataJSON) == "" {
		return nil, nil
	}

	var templateData map[string]interface{}
	if err := json.Unmarshal([]byte(templateDataJSON), &templateData); err != nil {
		return nil, fmt.Errorf("unmarshal templateDataJSON failed: %w", err)
	}

	return templateData, nil
}

func mapTemplateParticipantSummary(n map[string]interface{}) *templatecatalogueintegration.TemplateCatalogueParticipantSummary {
	participantRaw, ok := n["participant"].(map[string]interface{})
	if !ok || participantRaw == nil {
		// Optional participant summary
		return nil
	}

	headquarterRaw, _ := participantRaw["headquarter_address"].(map[string]interface{})

	return &templatecatalogueintegration.TemplateCatalogueParticipantSummary{
		LegalName:          ptr.Ref(ptr.StringFromMap(participantRaw, "legal_name")),
		RegistrationNumber: ptr.Ref(ptr.StringFromMap(participantRaw, "registration_number")),
		LeiCode:            ptr.Ref(ptr.StringFromMap(participantRaw, "lei_code")),
		HeadquarterAddress: &templatecatalogueintegration.TemplateCatalogueParticipantHeadquarterSummary{
			Country:  ptr.Ref(ptr.StringFromMap(headquarterRaw, "country")),
			Locality: ptr.Ref(ptr.StringFromMap(headquarterRaw, "locality")),
		},
		TermsAndConditions: ptr.Ref(ptr.StringFromMap(participantRaw, "terms_and_conditions")),
	}
}
