package provenance

import (
	"context"
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/auth/oid4vp/status"
)

// Revocation states a credential's status list resolves to. These are the words
// a verification report prints, so they say what was established and nothing
// more.
const (
	// StatusActive: the list verified and the credential's bit is clear.
	StatusActive = "active"
	// StatusRevoked: the list verified and the credential's bit is set.
	StatusRevoked = "revoked"
	// StatusSuspended: the list verified and the entry says suspended, which is
	// not revoked and not valid either.
	StatusSuspended = "suspended"
)

// CredentialStatusVerifier resolves a credential's own credentialStatus against
// the status list it names, and returns the state only once that list's
// signature has verified.
//
// It replaces a function whose name said what it was worth,
// ReadUnsignedStatusList: that one fetched a URL, believed the bitstring that
// came back, and left every caller to caveat the answer, because anyone who
// could answer the URL could decide whether a contract was revoked. This path
// is the ordinary one — the same verifier, handlers, trust anchors and
// leaf-identifies-issuer binding a wallet-presented credential's status list is
// held to (ADR-34 §3). A list that is unsigned, signed by an untrusted key, or
// signed by an issuer other than the one it names produces an error here, and
// the caller reports the state as unknown rather than as a reading.
type CredentialStatusVerifier struct {
	verifier *status.Verifier
}

// NewCredentialStatusVerifier wraps the configured status-list verifier. A nil
// verifier is not a disabled check: every call fails, so a deployment that
// forgot to configure status verification reports UNKNOWN rather than active.
func NewCredentialStatusVerifier(verifier *status.Verifier) *CredentialStatusVerifier {
	return &CredentialStatusVerifier{verifier: verifier}
}

// State returns the revocation state the credential's status list establishes.
//
// The credential must already have been verified: its credentialStatus points
// wherever its author chose, so following it before the credential's proof
// checks out is following an attacker's URL.
func (v *CredentialStatusVerifier) State(ctx context.Context, vcBytes []byte) (string, error) {
	if v == nil || v.verifier == nil {
		return "", fmt.Errorf("status list verification is not configured")
	}

	var claims map[string]any
	if err := json.Unmarshal(vcBytes, &claims); err != nil {
		return "", fmt.Errorf("credential is not readable JSON: %w", err)
	}

	result, err := v.verifier.VerifyStatus(ctx, status.VerifiedCredential{
		Format: "ldp_vc",
		Claims: claims,
	})
	if err != nil {
		return "", err
	}

	// One reference, one result: a lifecycle credential advertises exactly one
	// entry. The policy has already decided acceptance; what a verification
	// report needs is which state it saw.
	for _, r := range result.StatusResults {
		switch r.State {
		case status.StateValid:
			return StatusActive, nil
		case status.StateInvalid:
			return StatusRevoked, nil
		case status.StateSuspended:
			return StatusSuspended, nil
		default:
			return "", fmt.Errorf("status list entry %d of %s reads %q, which this build cannot interpret",
				r.Index, r.URI, r.State)
		}
	}
	return "", fmt.Errorf("credential advertises no status list entry")
}
