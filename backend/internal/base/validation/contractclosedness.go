package validation

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrContractNotClosed is a client-input error (map to 4xx): a contract still
// carries unresolved contractFields and so is not yet a contract. Templates
// (odrl:Offer) may stay open; a contract must be closed before it leaves draft
// and is signed.
var ErrContractNotClosed = errors.New("contract is not closed")

// ValidateContractClosed enforces the SRS contract-completeness invariant. A
// template's ODRL is open — parties, negotiated boundaries and values are
// contractFields resolved during generation and negotiation. A contract must be
// closed: the SRS requires Contract Generation to "fill in the necessary
// contractFields" so the "filled-out contract MUST be ready to be sent to the
// Responder", and Contract Approval to verify "schema completeness". This gate
// runs at approval and again at the signing seal.
//
// A contract is closed when every field the policy relies on is
// materialized: each negotiated-boundary right operand references a filled
// field, each required data field a policy enforces carries a value, and each
// prose field binds to a filled field.
//
// Note the narrowness, against the SRS wording above: the `dcs:required` check
// happens inside the per-rule constraint loop below, so it only covers fields
// an ODRL constraint's leftOperand names. A field marked `dcs:required: true`
// that no constraint references and no prose block binds is not checked by
// this function at all. "No unresolved placeholders" at the call sites means
// no placeholder the policy or the prose depends on — not every required field.
func ValidateContractClosed(contractDocument any) error {
	data, err := normalizeObject(contractDocument)
	if err != nil {
		return fmt.Errorf("decode contract document: %w", err)
	}
	fields := contractFieldValues(data)

	seen := map[string]bool{}
	unresolved := []string{}
	add := func(message string) {
		if !seen[message] {
			seen[message] = true
			unresolved = append(unresolved, message)
		}
	}

	for _, rule := range collectODRLPolicyRules(topLevelValue(data, "policies")) {
		constraints := policyConstraints(rule["odrl:constraint"])
		constraints = append(constraints, dutyConstraints(rule["odrl:duty"])...)
		for _, constraint := range constraints {
			for _, leaf := range compactConstraintLeaves(constraint) {
				// A required data field a policy enforces must carry a value; a
				// context operand (spatial, dateTime, …) is use-time context, not
				// a document field.
				if left := nodeReferenceID(leaf["odrl:leftOperand"]); left != "" && !isODRLContextOperandTerm(left) {
					if info, ok := fields[left]; ok && info.required && !info.hasValue {
						add(fmt.Sprintf("required data field %q has no value", left))
					}
				}
				// A negotiated boundary (a right operand referencing a field)
				// must have its agreed value.
				if right := nodeReferenceID(leaf["odrl:rightOperand"]); right != "" {
					if info, ok := fields[right]; ok && !info.hasValue {
						add(fmt.Sprintf("negotiated boundary %q has no agreed value", right))
					}
				}
				// So must a negotiated unit (an odrl:unit referencing a field):
				// a boundary whose unit was never agreed states no bound.
				if unit := nodeReferenceID(leaf["odrl:unit"]); unit != "" {
					if info, ok := fields[unit]; ok && !info.hasValue {
						add(fmt.Sprintf("negotiated unit %q has no agreed value", unit))
					}
				}
			}
		}
	}

	for _, message := range unresolvedProseFieldReferences(data, fields) {
		add(message)
	}

	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		return fmt.Errorf("%w: %s", ErrContractNotClosed, strings.Join(unresolved, "; "))
	}
	return nil
}

type contractFieldInfo struct {
	required bool
	hasValue bool
}

// contractFieldValues indexes the document's top-level ContractField nodes by
// @id, noting whether each is required and carries an inline value (dcs:value).
func contractFieldValues(data documentData) map[string]contractFieldInfo {
	out := map[string]contractFieldInfo{}
	fields, _ := topLevelValue(data, "contractFields").([]any)
	for _, rawField := range fields {
		field, ok := rawField.(map[string]any)
		if !ok {
			continue
		}
		id, _ := field["@id"].(string)
		if id == "" {
			continue
		}
		required, _ := field["dcs:required"].(bool)
		out[id] = contractFieldInfo{required: required, hasValue: hasInlineValue(field)}
	}
	return out
}

// hasInlineValue reports whether a field carries a non-empty filled value
// (dcs:value); an absent key or empty string is unset.
func hasInlineValue(field map[string]any) bool {
	value, present := field["dcs:value"]
	if !present || value == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(value)) != ""
}

// nodeReferenceID returns the @id of a JSON-LD reference node ({"@id": …}); a
// value node ({"@value": …}) or a list is not a reference and returns "".
func nodeReferenceID(value any) string {
	node, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if _, isValue := node["@value"]; isValue {
		return ""
	}
	id, _ := node["@id"].(string)
	return id
}

// unresolvedProseFieldReferences reports clause field references (bare
// {"@id"} nodes) whose top-level field carries no value — a field
// that never got materialized.
func unresolvedProseFieldReferences(data documentData, fields map[string]contractFieldInfo) []string {
	messages := []string{}
	structure, ok := topLevelValue(data, "documentStructure").(map[string]any)
	if !ok {
		return messages
	}
	blocks, ok := jsonLDList(structure["dcs:blocks"])
	if !ok {
		blocks, _ = structure["dcs:blocks"].([]any)
	}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		content, ok := jsonLDList(block["dcs:content"])
		if !ok {
			continue
		}
		for _, rawSegment := range content {
			segment, ok := rawSegment.(map[string]any)
			if !ok {
				continue
			}
			fieldID, _ := segment["@id"].(string)
			if fieldID == "" {
				continue
			}
			if info, ok := fields[fieldID]; !ok || !info.hasValue {
				messages = append(messages, fmt.Sprintf("prose field binds to unfilled field %q", fieldID))
			}
		}
	}
	return messages
}
