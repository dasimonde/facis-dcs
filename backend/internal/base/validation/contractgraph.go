package validation

import (
	"fmt"
	"strings"
)

// validateContractMetadataType fails fast on a dcs:Contract document whose
// metadata node is not typed dcs:ContractMetadata — the render gate's SHACL
// would otherwise reject the document asynchronously, long after the
// submitting API call succeeded.
func validateContractMetadataType(data documentData) error {
	documentType, _ := data["@type"].(string)
	if compactTerm(documentType) != "Contract" {
		return nil
	}
	metadata, ok := topLevelValue(data, "metadata").(map[string]any)
	if !ok {
		return nil // cardinality is the canonical shapes' concern
	}
	metadataType, _ := metadata["@type"].(string)
	if term := compactTerm(metadataType); term != "" && term != "ContractMetadata" {
		return fmt.Errorf("a dcs:Contract document's dcs:metadata must be typed dcs:ContractMetadata, got %q", metadataType)
	}
	return nil
}

// materializeContractDataFields returns a deep copy of a contract document
// whose contract-data field references are dereferenced for shape
// validation: a property holding {"@id": <declared field>} is replaced by
// the field's filled dcs:value, so an external SHACL library written
// against plain instance data (literals on the property) validates the
// live document without knowing the field indirection. An unfilled field's
// reference is dropped — the property is simply absent until negotiation
// fills it, and a library's own cardinality constraints report exactly
// that. References to other domain objects pass through untouched.
func materializeContractDataFields(contract map[string]any) map[string]any {
	fields := contractFieldFills(contract)
	if len(fields) == 0 {
		return contract
	}
	copied := deepCopyValue(contract).(map[string]any)
	contractData, _ := topLevelValue(copied, "contractData").([]any)
	for _, rawObject := range contractData {
		object, ok := rawObject.(map[string]any)
		if !ok {
			continue
		}
		for property, rawValue := range object {
			if strings.HasPrefix(property, "@") {
				continue
			}
			if members, isArray := rawValue.([]any); isArray {
				materialized := make([]any, 0, len(members))
				for _, member := range members {
					if value, drop := materializeFieldRef(member, fields); !drop {
						materialized = append(materialized, value)
					}
				}
				if len(materialized) == 0 {
					delete(object, property)
				} else {
					object[property] = materialized
				}
				continue
			}
			if value, drop := materializeFieldRef(rawValue, fields); drop {
				delete(object, property)
			} else {
				object[property] = value
			}
		}
	}
	return copied
}

// contractFieldFills indexes the document's declared fields:
// @id -> (filled dcs:value, present). An unfilled field maps to present=false.
func contractFieldFills(contract map[string]any) map[string]fieldFill {
	declarations, _ := topLevelValue(contract, "contractFields").([]any)
	fills := make(map[string]fieldFill, len(declarations))
	for _, rawField := range declarations {
		field, ok := rawField.(map[string]any)
		if !ok {
			continue
		}
		id, _ := field["@id"].(string)
		if id == "" {
			continue
		}
		value, filled := fieldFillValue(field)
		fills[id] = fieldFill{value: value, filled: filled}
	}
	return fills
}

type fieldFill struct {
	value  any
	filled bool
}

// fieldFillValue reads a field's dcs:value, treating an absent value and an
// empty string as unfilled.
func fieldFillValue(field map[string]any) (any, bool) {
	value, exists := field["dcs:value"]
	if !exists {
		return nil, false
	}
	if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
		return nil, false
	}
	return value, true
}

// materializeFieldRef resolves one property value: a reference to a filled
// field becomes the fill, a reference to an unfilled field is dropped, and
// everything else (literals, domain-object references) passes through.
func materializeFieldRef(value any, fields map[string]fieldFill) (materialized any, drop bool) {
	ref, ok := value.(map[string]any)
	if !ok || len(ref) != 1 {
		return value, false
	}
	id, _ := ref["@id"].(string)
	fill, isField := fields[id]
	if !isField {
		return value, false
	}
	if !fill.filled {
		return nil, true
	}
	return deepCopyValue(fill.value), false
}

func deepCopyValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(v))
		for key, member := range v {
			copied[key] = deepCopyValue(member)
		}
		return copied
	case []any:
		copied := make([]any, len(v))
		for index, member := range v {
			copied[index] = deepCopyValue(member)
		}
		return copied
	default:
		return v
	}
}
