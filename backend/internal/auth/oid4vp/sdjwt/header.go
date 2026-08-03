package sdjwt

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// CredentialTyp is the JWT typ RFC 9901 registers for SD-JWT VCs, and the one
// this deployment's own issuer stamps.
const CredentialTyp = "dc+sd-jwt"

// LegacyCredentialTyp is the typ SD-JWT VC carried before it was renamed. Most
// issuers and wallets in the field still emit it, and a credential is not worse
// for being stamped with the name its issuer's library was written against.
const LegacyCredentialTyp = "vc+sd-jwt"

// KBJWTTyp is the JWT typ for holder key-binding JWTs.
const KBJWTTyp = "kb+jwt"

// normalizeTyp drops the "application/" prefix RFC 7515 §4.1.9 explicitly
// allows a typ to be written with, so a header carrying the full media type
// compares equal to the bare subtype every implementation abbreviates it to.
func normalizeTyp(raw any) string {
	typ, _ := raw.(string)

	return strings.TrimPrefix(strings.TrimSpace(typ), "application/")
}

func validateCredentialHeader(token *jwt.Token) error {
	switch normalizeTyp(token.Header["typ"]) {
	case CredentialTyp, LegacyCredentialTyp:
		return nil
	}

	return fmt.Errorf("credential jwt typ must be %q or %q, got %q", CredentialTyp, LegacyCredentialTyp, token.Header["typ"])
}

func validateKBHeader(token *jwt.Token) error {
	if normalizeTyp(token.Header["typ"]) != KBJWTTyp {
		return fmt.Errorf("kb jwt typ must be %q", KBJWTTyp)
	}

	return nil
}
