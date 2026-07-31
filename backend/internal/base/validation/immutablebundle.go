package validation

import (
	"errors"
	"fmt"
	"strings"

	"digital-contracting-service/internal/base/datatype"
)

var immutableSemanticBundleFields = [...]string{
	"@context",
	"sh:shapesGraph",
	"dcs:effectiveShapes",
	"dcterms:conformsTo",
}

// NormalizeContractMutationForPersistence normalizes a client-authored
// replacement document while preserving the semantic bundle selected by the
// server when the contract was created. Client values for every bundle field
// are deliberately discarded as a unit.
func NormalizeContractMutationForPersistence(candidate, stored *datatype.JSON, did string, requireSemanticValues bool) (*datatype.JSON, error) {
	bundle, err := immutableSemanticBundle(stored)
	if err != nil {
		return nil, err
	}
	data, err := decodeDocumentData(candidate)
	if err != nil {
		return nil, err
	}
	for _, field := range immutableSemanticBundleFields {
		delete(data, field)
	}
	withoutClientPins, err := encodeDocumentData(data)
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizeContractDataForPersistence(withoutClientPins, did, requireSemanticValues)
	if err != nil {
		return nil, err
	}
	normalizedData, err := decodeDocumentData(normalized)
	if err != nil {
		return nil, err
	}
	for field, value := range bundle {
		normalizedData[field] = value
	}
	return encodeDocumentData(normalizedData)
}

// ValidateImmutableSemanticBundle checks that a stored contract still carries
// the complete, version-pinned server bundle required before any mutation.
func ValidateImmutableSemanticBundle(stored *datatype.JSON) error {
	_, err := immutableSemanticBundle(stored)
	return err
}

func immutableSemanticBundle(stored *datatype.JSON) (map[string]any, error) {
	data, err := decodeDocumentData(stored)
	if err != nil {
		return nil, fmt.Errorf("stored immutable semantic bundle is malformed: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("stored immutable semantic bundle is missing")
	}
	for _, field := range immutableSemanticBundleFields {
		if _, ok := data[field]; !ok {
			return nil, fmt.Errorf("stored immutable semantic bundle is missing %s", field)
		}
	}
	if pinnedHubContextVersion(data) <= 0 {
		return nil, errors.New("stored immutable semantic bundle has an invalid @context pin")
	}
	shapes := declaredShapesGraphs(data)
	if len(shapes) == 0 {
		return nil, errors.New("stored immutable semantic bundle has an invalid sh:shapesGraph pin")
	}
	for _, shape := range shapes {
		if strings.TrimSpace(shape.Name) == "" || shape.Version <= 0 {
			return nil, errors.New("stored immutable semantic bundle has an invalid sh:shapesGraph pin")
		}
	}
	if _, err := effectiveShapeRefs(data); err != nil {
		return nil, fmt.Errorf("stored immutable semantic bundle has invalid dcs:effectiveShapes: %w", err)
	}
	profile := anchorIRI(data["dcterms:conformsTo"])
	if !strings.Contains(profile, "/semantic/profile/") || anchorVersion(profile) <= 0 {
		return nil, errors.New("stored immutable semantic bundle has an invalid dcterms:conformsTo pin")
	}

	bundle := make(map[string]any, len(immutableSemanticBundleFields))
	for _, field := range immutableSemanticBundleFields {
		bundle[field] = data[field]
	}
	return bundle, nil
}
