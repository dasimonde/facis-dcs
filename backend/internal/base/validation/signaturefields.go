package validation

import (
	"encoding/json"
	"strings"
)

// RequiredSignatureFields returns the contract's declared signature-field
// names (dcs:SignatureField nodes' signatoryName, the AcroForm field name
// pdf-core renders and /sign targets — see pdf-core/compiler/dcs_schema.go).
// An empty result means the contract declares no explicit signature fields
// and follows the single-signature flow (DCS-FR-SM-07/-17: contracts that
// require multiple signatures declare one field per signatory).
func RequiredSignatureFields(contractData []byte) []string {
	var doc map[string]any
	if err := json.Unmarshal(contractData, &doc); err != nil {
		return nil
	}
	// The declaration may carry the prefixed or the JSON-LD-compacted term,
	// depending on the contract-data form in hand.
	raw, ok := doc["dcs:signatureFields"].([]any)
	if !ok {
		raw, _ = doc["signatureFields"].([]any)
	}
	fields := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := node["dcs:signatoryName"].(string)
		if name == "" {
			name, _ = node["signatoryName"].(string)
		}
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		fields = append(fields, name)
	}
	return fields
}

// RequiredCredentialType returns the contract's OWN declared signature-level
// requirement (dcs:requiredCredentialType) for the dcs:SignatureField node
// named field — "AES" or "QES" — defaulting to "AES" when the field carries no
// explicit requirement or is not declared at all (SM-01 per-contract level
// enforcement, ADR-20). The caller-supplied credential_type request parameter
// is what the DCS ASKS the wallet to produce; this is what it ENFORCES at
// submit, and the two are deliberately independent.
func RequiredCredentialType(contractData []byte, field string) string {
	const defaultLevel = "AES"
	var doc map[string]any
	if err := json.Unmarshal(contractData, &doc); err != nil {
		return defaultLevel
	}
	raw, ok := doc["dcs:signatureFields"].([]any)
	if !ok {
		raw, _ = doc["signatureFields"].([]any)
	}
	for _, item := range raw {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := node["dcs:signatoryName"].(string)
		if name == "" {
			name, _ = node["signatoryName"].(string)
		}
		if strings.TrimSpace(name) != field {
			continue
		}
		level, _ := node["dcs:requiredCredentialType"].(string)
		if level == "" {
			level, _ = node["requiredCredentialType"].(string)
		}
		level = strings.ToUpper(strings.TrimSpace(level))
		if level == "" {
			return defaultLevel
		}
		return level
	}
	return defaultLevel
}
