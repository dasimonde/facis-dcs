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

const PIDCredentialQueryID = "eudi_pid_credential" // #nosec G101 -- identifier or vocabulary IRI, not a credential

// PIDClaimPaths are the PID claims the DCS requests and keeps (DCS-NFR-SEC-18,
// DCS-NFR-SQ-04): the given and family name, which the signature check
// compares with the signing certificate, and the claims the regulatory
// workflow gate looks for in the contract content
// (processauditandcompliance/workflowgate/regulatory.go). Every other claim of
// a PID is neither requested nor stored.
var PIDClaimPaths = [][]string{
	{"given_name"}, {"family_name"},
	{"birth_family_name"}, {"birth_date"}, {"birthdate"}, {"birth_place"},
	{"personal_administrative_number"}, {"email"}, {"phone_number"},
	{"street_address"}, {"address", "street_address"},
}

// pidClaimSets are the combinations of PIDClaimPaths a wallet may disclose,
// most complete first. DCQL has no optional claims, so a wallet returns the
// first set its PID holds; the last one is the names alone.
var pidClaimSets = [][]string{
	{"given_name", "family_name", "birth_family_name", "birth_date", "birth_place", "personal_administrative_number", "email", "phone_number", "address_street_address"},
	{"given_name", "family_name", "birth_date", "birth_place", "address_street_address"},
	{"given_name", "family_name", "birthdate", "street_address"},
	{"given_name", "family_name", "birth_date"},
	{"given_name", "family_name", "birthdate"},
	{"given_name", "family_name"},
}

func pidClaimID(path []string) string {
	return strings.Join(path, "_")
}

// PIDLegacyCredentialQueryID asks for the same PID under the SD-JWT VC format
// identifier that predates dc+sd-jwt.
const PIDLegacyCredentialQueryID = "eudi_pid_credential_vc_sd_jwt" // #nosec G101 -- identifier or vocabulary IRI, not a credential

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
		"claims":                               pidClaimsQuery(),
		"claim_sets":                           pidClaimSets,
	}
}

func pidClaimsQuery() []any {
	claims := make([]any, 0, len(PIDClaimPaths))
	for _, path := range PIDClaimPaths {
		claims = append(claims, map[string]any{"id": pidClaimID(path), "path": path})
	}
	return claims
}

// minimisePIDClaims keeps only the PIDClaimPaths of verified PID claims, so a
// wallet that discloses more than was requested has the rest dropped before
// anything is stored.
func minimisePIDClaims(raw json.RawMessage) (json.RawMessage, error) {
	var all map[string]any
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("pid claims: %w", err)
	}
	kept := map[string]any{}
	for _, path := range PIDClaimPaths {
		value, ok := claimAt(all, path)
		if !ok {
			continue
		}
		into := kept
		for _, name := range path[:len(path)-1] {
			next, ok := into[name].(map[string]any)
			if !ok {
				next = map[string]any{}
				into[name] = next
			}
			into = next
		}
		into[path[len(path)-1]] = value
	}
	return json.Marshal(kept)
}

func claimAt(claims map[string]any, path []string) (any, bool) {
	var value any = claims
	for _, name := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		if value, ok = object[name]; !ok {
			return nil, false
		}
	}
	return value, true
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
