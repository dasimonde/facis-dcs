// Package federation holds the rules a DCS instance agrees to before it
// exchanges contracts with another instance (ADR-19): the rules document is
// compiled into the binary via go:embed, so acceptance of it is bound to the
// software version — two instances of the same build embed byte-identical
// rules and therefore compute the identical hash; a tampered or differently
// versioned rules document fails the hash comparison every peer's agreement
// credential is checked against.
package federation

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"strings"
)

//go:embed rules.md
var rules []byte

// rulesHash is computed once, deterministically, from the embedded bytes.
var rulesHash = computeHash(rules)

func computeHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Rules returns the embedded federation rules document.
func Rules() []byte {
	return rules
}

// Hash returns the SHA-256 hex digest of the embedded federation rules
// document — the value every agreement credential's termsOfUse.hash names.
func Hash() string {
	return rulesHash
}

// RulesPolicyURL derives the public, dereferenceable URL of this instance's
// own federation rules document (served by GetFederationRules) from its
// public API base URL (DCS_PUBLIC_URL): the well-known path is mounted at the
// origin root, outside the API path, mirroring did.json's own well-known
// convention.
func RulesPolicyURL(publicAPIURL string) string {
	origin := strings.TrimRight(strings.TrimSpace(publicAPIURL), "/")
	origin = strings.TrimSuffix(origin, "/api")
	return origin + "/.well-known/dcs-federation-rules.md"
}
