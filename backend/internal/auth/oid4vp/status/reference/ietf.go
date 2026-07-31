package reference

import (
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/status"
)

type IETF struct{}

func (IETF) Supports(credential status.VerifiedCredential) bool {
	statusClaim, ok := credential.Claims["status"].(map[string]any)
	if !ok {
		return false
	}
	sl, ok := statusClaim["status_list"].(map[string]any)
	return ok && sl["uri"] != nil && sl["idx"] != nil
}

func (IETF) Extract(credential status.VerifiedCredential) ([]status.Reference, error) {
	ref, err := parseIETFStatusReference(credential.Claims["status"], credential.Format)
	if err != nil {
		return nil, err
	}
	return []status.Reference{ref}, nil
}

func parseIETFStatusReference(raw any, format string) (status.Reference, error) {
	statusClaim, ok := raw.(map[string]any)
	if !ok {
		return status.Reference{}, fmt.Errorf("status claim must be an object")
	}
	sl, ok := statusClaim["status_list"].(map[string]any)
	if !ok {
		return status.Reference{}, fmt.Errorf("status.status_list is required")
	}

	uri, _ := sl["uri"].(string)
	uri = strings.TrimSpace(uri)
	index, ok := parseStatusIndex(sl["idx"])
	if uri == "" || !ok {
		return status.Reference{}, fmt.Errorf("status.status_list has invalid uri or idx")
	}

	return status.Reference{
		Mechanism:        status.MechanismIETFToken,
		URI:              uri,
		Index:            index,
		StatusSize:       defaultStatusSize,
		EntryType:        "TokenStatusList",
		CredentialFormat: format,
	}, nil
}
