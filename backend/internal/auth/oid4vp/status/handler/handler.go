// Package handler implements W3C, IETF, and XFSC status-list mechanism handlers.
// Each handler verifies the list security envelope, decodes the bitstring, and maps
// the entry to a normalized status result.
package handler

import (
	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/reference"
)

// NewVerifier wires the default status-list handlers and reference extractor.
func NewVerifier(trust *status.TrustConfig, opts Options) *status.Verifier {
	client := fetch.NewClient()
	return &status.Verifier{
		Fetcher:            client,
		ReferenceExtractor: reference.Extract,
		Handlers: map[status.Mechanism]status.Handler{
			status.MechanismW3CBitstring: &W3CBitstring{Fetcher: client, Trust: trust},
			status.MechanismIETFToken:    &IETFToken{Fetcher: client, Trust: trust},
			status.MechanismXFSC: &XFSC{
				Fetcher: client,
				Trust:   trust,
			},
		},
		Policy: status.StrictPolicy{},
	}
}
