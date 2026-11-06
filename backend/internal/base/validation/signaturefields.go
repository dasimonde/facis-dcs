package validation

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
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

// SignatureLevelSatisfies reports whether an achieved signature level (SES,
// AES, QES) meets a required level.
func SignatureLevelSatisfies(achieved, required string) bool {
	return signatureLevelSatisfies(achieved, required)
}

// SigningOrder returns the position (dcs:signingOrder, a positive integer) of
// each declared signature field that states one (DCS-FR-SM-13). A field with
// a lower position signs first. A field without a position has no place in
// the order: it neither waits for nor blocks another field.
func SigningOrder(contractData []byte) map[string]int {
	var doc map[string]any
	if err := json.Unmarshal(contractData, &doc); err != nil {
		return nil
	}
	raw, ok := doc["dcs:signatureFields"].([]any)
	if !ok {
		raw, _ = doc["signatureFields"].([]any)
	}
	order := map[string]int{}
	for _, item := range raw {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := node["dcs:signatoryName"].(string)
		if name == "" {
			name, _ = node["signatoryName"].(string)
		}
		position, ok := node["dcs:signingOrder"]
		if !ok {
			position = node["signingOrder"]
		}
		if typed, ok := position.(map[string]any); ok {
			position = typed["@value"]
		}
		var n float64
		switch v := position.(type) {
		case float64:
			n = v
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				continue
			}
			n = float64(parsed)
		default:
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || n < 1 || n != math.Trunc(n) {
			continue
		}
		order[name] = int(n)
	}
	return order
}

// SigningDeadline returns the contract's signing deadline
// (dcs:signingDeadline, an xsd:dateTime or xsd:date at the top level of the
// contract document), after which no signature is accepted (DCS-FR-SM-13).
// A date without a time includes the whole day in UTC.
func SigningDeadline(contractData []byte) (time.Time, bool) {
	var doc map[string]any
	if err := json.Unmarshal(contractData, &doc); err != nil {
		return time.Time{}, false
	}
	value, ok := doc["dcs:signingDeadline"]
	if !ok {
		value = doc["signingDeadline"]
	}
	if typed, ok := value.(map[string]any); ok {
		value = typed["@value"]
	}
	text, _ := value.(string)
	text = strings.TrimSpace(text)
	if deadline, err := time.Parse(time.RFC3339, text); err == nil {
		return deadline.UTC(), true
	}
	if day, err := time.Parse("2006-01-02", text); err == nil {
		return day.Add(24*time.Hour - time.Nanosecond), true
	}
	return time.Time{}, false
}
