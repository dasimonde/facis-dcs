package reference

import (
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/status"
)

type W3C struct{}

func (W3C) Supports(credential status.VerifiedCredential) bool {
	_, ok := credential.Claims["credentialStatus"]
	return ok
}

func (W3C) Extract(credential status.VerifiedCredential) ([]status.Reference, error) {
	return parseW3CStatusReferences(credential.Claims["credentialStatus"], credential.Format)
}

func parseW3CStatusReferences(raw any, format string) ([]status.Reference, error) {
	items := []any{raw}
	if arr, ok := raw.([]any); ok {
		items = arr
	}

	refs := make([]status.Reference, 0, len(items))
	for _, item := range items {
		cs, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("credentialStatus must be an object or array of objects")
		}

		entryType, _ := cs["type"].(string)
		entryType = strings.TrimSpace(entryType)
		if entryType == "" {
			return nil, fmt.Errorf("credentialStatus.type is required")
		}

		// Two entry types name a list through credentialStatus, and they differ
		// in the document at the far end: BitstringStatusListEntry points at a
		// W3C status list credential, TokenStatusList at an IETF token status
		// list. The mechanism recorded here is only the starting guess — the
		// verifier re-routes on the media type the URI actually answers with
		// (status.SelectMechanismFromResponse) — but starting from the type the
		// credential declares is what makes the Accept header ask for the right
		// document in the first place.
		var mechanism status.Mechanism
		switch entryType {
		case entryTypeBitstringStatusList:
			mechanism = status.MechanismW3CBitstring
		case entryTypeTokenStatusList:
			mechanism = status.MechanismIETFToken
		default:
			return nil, fmt.Errorf("unsupported credentialStatus.type %q", entryType)
		}

		uri, _ := cs["statusListCredential"].(string)
		uri = strings.TrimSpace(uri)
		index, ok := parseStatusIndex(cs["statusListIndex"])
		if uri == "" || !ok {
			return nil, fmt.Errorf("credentialStatus has invalid statusListCredential or statusListIndex")
		}

		statusSize, ok := parseStatusSize(cs["statusSize"])
		if !ok {
			return nil, fmt.Errorf("credentialStatus has unsupported statusSize")
		}

		purpose, _ := cs["statusPurpose"].(string)
		refs = append(refs, status.Reference{
			Mechanism:        mechanism,
			URI:              uri,
			Index:            index,
			Purpose:          strings.TrimSpace(purpose),
			StatusSize:       statusSize,
			EntryType:        entryType,
			CredentialFormat: format,
		})
	}

	return refs, nil
}
