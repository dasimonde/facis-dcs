// Package mapper builds interoperable JSON-LD envelopes from DCS database rows.
// Stored documents are already in canonical JSON-LD form (guaranteed by the
// normalization layer); the mapper returns them as-is.
package mapper

import (
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/base/datatype"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	templatedb "digital-contracting-service/internal/templaterepository/db"
)

// BuildTemplateJSONLD returns the stored canonical JSON-LD envelope for a
// contract template. The document must already be in canonical form
// (dcs:documentStructure present); non-canonical data returns an error.
func BuildTemplateJSONLD(template templatedb.ContractTemplate) (map[string]any, error) {
	inner, err := parseJSONB(template.TemplateData)
	if err != nil {
		return nil, fmt.Errorf("parse template_data: %w", err)
	}
	if !isCanonicalJSONLDEnvelope(inner) {
		return nil, fmt.Errorf("template data is not in canonical JSON-LD format")
	}
	return inner, nil
}

// BuildContractJSONLD returns the stored canonical JSON-LD envelope for a
// contract. The document must already be in canonical form; non-canonical data
// returns an error.
func BuildContractJSONLD(contract contractdb.Contract) (map[string]any, error) {
	inner, err := parseJSONB(contract.ContractData)
	if err != nil {
		return nil, fmt.Errorf("parse contract_data: %w", err)
	}
	if !isCanonicalJSONLDEnvelope(inner) {
		return nil, fmt.Errorf("contract data is not in canonical JSON-LD format")
	}
	return inner, nil
}

func isCanonicalJSONLDEnvelope(value map[string]any) bool {
	_, hasDocumentStructure := value["dcs:documentStructure"]
	return hasDocumentStructure
}

// parseJSONB decodes a *datatype.JSON JSONB value into a generic map.
// Returns an empty (non-nil) map when raw is nil or JSON null.
func parseJSONB(raw *datatype.JSON) (map[string]any, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(*raw, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}
