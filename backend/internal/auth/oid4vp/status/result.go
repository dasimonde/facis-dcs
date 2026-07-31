// Package status verifies credential status via remote status lists (W3C, IETF, XFSC).
//
// It fetches the list URI from credential claims, detects the mechanism from the
// response, checks the entry bit at the given index, and maps the result to a
// normalized State. Wired into OID4VP via oid4vp.ConfigureStatusListVerification.
package status

import (
	"context"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
)

type Mechanism string

const (
	MechanismW3CBitstring Mechanism = "w3c-bitstring-v1"
	MechanismIETFToken    Mechanism = "ietf-token-status-list"
	MechanismXFSC         Mechanism = "xfsc-status-list"
)

type State string

const (
	StateValid               State = "valid"
	StateInvalid             State = "invalid"
	StateSuspended           State = "suspended"
	StateRefreshRequired     State = "refresh_required"
	StateApplicationSpecific State = "application_specific"
	StateUnknown             State = "unknown"
)

type Reference struct {
	Mechanism        Mechanism
	URI              string
	Index            uint64
	Purpose          string
	StatusSize       uint
	EntryType        string
	CredentialFormat string
	// Prefetched is set by the verifier after a single Accept-based GET for IETF
	// references so handlers do not repeat the same request.
	Prefetched *fetch.Response
}

type Result struct {
	Mechanism Mechanism
	State     State
	RawValue  uint64
	Purpose   string
	URI       string
	Index     uint64
	CheckedAt time.Time
}

type VerifiedCredential struct {
	Format string
	Claims map[string]any
}

// Handler checks one status reference against its remote status list.
type Handler interface {
	Mechanism() Mechanism
	Check(ctx context.Context, credential VerifiedCredential, ref Reference) (Result, error)
}

// ReferenceExtractor pulls status references from decoded credential claims.
type ReferenceExtractor func(VerifiedCredential) ([]Reference, error)
