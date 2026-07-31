package oid4vp

import (
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

// PoAVCT is the DCS Proof of Authority credential type (version 1).
const PoAVCT = "urn:dcs:poa:v1"

const PoACredentialQueryID = "dcs_poa_credential"

// PoALegacyCredentialQueryID asks for the same Power of Attorney under the
// SD-JWT VC format identifier that predates dc+sd-jwt. A PoA may be issued by
// any issuer this deployment trusts for the purpose, not only its own (ADR-31).
const PoALegacyCredentialQueryID = "dcs_poa_credential_vc_sd_jwt"

// DefaultDCQLQuery requests an SD-JWT VC PoA credential for OpenID4VP
// presentation, under either format identifier.
// Override the full query via OID4VP_DCQL_QUERY when needed.
func DefaultDCQLQuery() map[string]any {
	return map[string]any{
		"credentials": []any{
			poaCredentialQuery(PoACredentialQueryID, sdjwt.CredentialTyp),
			poaCredentialQuery(PoALegacyCredentialQueryID, sdjwt.LegacyCredentialTyp),
		},
		"credential_sets": []any{
			map[string]any{"options": []any{
				[]any{PoACredentialQueryID},
				[]any{PoALegacyCredentialQueryID},
			}},
		},
	}
}

func poaCredentialQuery(id, format string) map[string]any {
	return map[string]any{
		"id":     id,
		"format": format,
		"meta": map[string]any{
			"vct_values": []string{PoAVCT},
		},
		"require_cryptographic_holder_binding": true,
		"claims": []any{
			map[string]any{"path": []string{"organization"}},
			map[string]any{"path": []string{"roles"}},
		},
	}
}

func LoadDCQLQuery(raw string) (any, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return DefaultDCQLQuery(), nil
	}

	var q any

	err := json.Unmarshal([]byte(raw), &q)
	if err != nil {
		return nil, fmt.Errorf("invalid OID4VP_DCQL_QUERY JSON: %w", err)
	}

	return q, nil
}
