package dcstodcs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/federation"
	"digital-contracting-service/internal/base/identity"
)

// credentialClockSkew is the tolerance applied to a peer's own time claims — the
// same five minutes the wallet credential path allows (auth/oid4vp/sdjwt), since
// two independently operated instances have independently drifting clocks and a
// credential minted a second ago must not be rejected as not yet in force.
const credentialClockSkew = 5 * time.Minute

// requireCredentialInForce checks a credential's own validity window.
//
// An expiry is required, not merely honoured when present: the window is the
// only thing that ends a peer's layer-3a standing, so a credential without one
// is a credential that outlives the peer's membership in the federation. The
// window is also refused when it is longer than the federation permits, which is
// the same statement — a peer naming the year 3000 has published a credential
// that never expires.
//
// Both spellings are read: validFrom/validUntil (VC Data Model 2.0) and
// issuanceDate/expirationDate (1.1), because a peer's model version is its own
// choice and both are covered by the proof either way.
func requireCredentialInForce(raw json.RawMessage, now time.Time) error {
	var window struct {
		ValidFrom      string `json:"validFrom"`
		ValidUntil     string `json:"validUntil"`
		IssuanceDate   string `json:"issuanceDate"`
		ExpirationDate string `json:"expirationDate"`
	}
	if err := json.Unmarshal(raw, &window); err != nil {
		return fmt.Errorf("decode validity window: %w", err)
	}

	from, err := parseCredentialTime("validFrom", firstNonEmpty(window.ValidFrom, window.IssuanceDate))
	if err != nil {
		return err
	}
	until, err := parseCredentialTime("validUntil", firstNonEmpty(window.ValidUntil, window.ExpirationDate))
	if err != nil {
		return err
	}
	if until.IsZero() {
		return fmt.Errorf("carries no validUntil, so it would never stop being accepted")
	}
	// The window's length is measured from its start, or — when the peer omitted
	// one, leaving the credential in force since always — from now. Skipping the
	// bound for a missing validFrom would make omitting it the way to publish the
	// year 3000, which is the very window the bound exists to refuse.
	lifetimeFrom := from
	if from.IsZero() {
		lifetimeFrom = now
	} else {
		if until.Before(from) {
			return fmt.Errorf("validUntil %s precedes validFrom %s", until.Format(time.RFC3339), from.Format(time.RFC3339))
		}
		if now.Add(credentialClockSkew).Before(from) {
			return fmt.Errorf("is not in force before %s", from.Format(time.RFC3339))
		}
	}
	if until.Sub(lifetimeFrom) > federation.MaxAgreementCredentialLifetime {
		return fmt.Errorf("claims a validity window of %s, longer than the %s the federation permits",
			until.Sub(lifetimeFrom), federation.MaxAgreementCredentialLifetime)
	}
	if now.Add(-credentialClockSkew).After(until) {
		return fmt.Errorf("expired at %s", until.Format(time.RFC3339))
	}
	return nil
}

func parseCredentialTime(field, value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s %q is not an RFC3339 timestamp", field, value)
	}
	return parsed.UTC(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// requireDocumentIsFor binds a resolved DID document to the identifier it was
// resolved for. did:web resolution is an HTTP GET whose response is trusted as
// key material, and the response gets to say who it is about.
//
// Compared with the same-peer rule PostPdf's own guard uses rather than as
// strings: two spellings of one did:web identifier (the %3A port escape, the case
// of the authority) name the same document, while two identifiers under one host
// do not name each other. An id that is not a did:web identifier at all compares
// unequal, which is the fail-closed answer.
func requireDocumentIsFor(doc *identity.DIDDocument, requestedDID string) error {
	if doc == nil {
		return fmt.Errorf("no peer did document")
	}
	docID, err := doc.GetID()
	if err != nil {
		return fmt.Errorf("peer did.json: %w", err)
	}
	if !identity.SameDIDWeb(docID, requestedDID) {
		return fmt.Errorf("did.json fetched for %q identifies itself as %q", requestedDID, docID)
	}
	return nil
}
