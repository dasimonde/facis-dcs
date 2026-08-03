package oid4vp

import (
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

// PIDVCT is the German EUDI PID credential type — the real one, asserting an
// identity that was actually proofed.
//
// A deployment served by a demo issuer must NOT request this: no identity
// proofing happens there, so accepting it under this type would claim an
// assurance level nobody established. Point OID4VP_PID_DCQL_QUERY at the demo
// type instead (urn:dcs:pid:demo:v1), which is what the demo issuer mints.
const PIDVCT = "urn:eudi:pid:de:1"

// DemoPIDVCT is the type the bundled demo PID issuer mints. It is deliberately
// not an EUDI type: it describes a person nobody verified.
const DemoPIDVCT = "urn:dcs:pid:demo:v1"

const PIDCredentialQueryID = "eudi_pid_credential"

// PIDLegacyCredentialQueryID asks for the same PID under the SD-JWT VC format
// identifier that predates dc+sd-jwt.
const PIDLegacyCredentialQueryID = "eudi_pid_credential_vc_sd_jwt"

// DefaultPIDDCQLQuery requests an SD-JWT VC PID credential for identity
// presentation, under either format identifier — a wallet matches a credential
// against the format string, so asking only for the current one hides every PID
// an issuer stamped with the older one.
// Override the full query via OID4VP_PID_DCQL_QUERY when needed.
func DefaultPIDDCQLQuery() map[string]any {
	return map[string]any{
		"credentials": []any{
			pidCredentialQuery(PIDCredentialQueryID, sdjwt.CredentialTyp),
			pidCredentialQuery(PIDLegacyCredentialQueryID, sdjwt.LegacyCredentialTyp),
		},
		// Either query satisfies the request. Without credential_sets a wallet
		// must satisfy ALL of them, which would mean holding the same PID twice.
		"credential_sets": []any{
			map[string]any{"options": []any{
				[]any{PIDCredentialQueryID},
				[]any{PIDLegacyCredentialQueryID},
			}},
		},
	}
}

func pidCredentialQuery(id, format string) map[string]any {
	return map[string]any{
		"id":     id,
		"format": format,
		"meta": map[string]any{
			// The demo type by default: a deployment served by the demo
			// issuer must not ask for the EUDI type, and asking for what
			// nobody can honestly issue only fails later and less clearly.
			"vct_values": []string{DemoPIDVCT},
		},
		"require_cryptographic_holder_binding": true,
	}
}

func LoadPIDDCQLQuery(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultPIDDCQLQuery(), nil
	}

	var q any
	err := json.Unmarshal([]byte(raw), &q)
	if err != nil {
		return nil, fmt.Errorf("invalid OID4VP_PID_DCQL_QUERY JSON: %w", err)
	}

	return q, nil
}
