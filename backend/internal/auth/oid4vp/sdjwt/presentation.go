package sdjwt

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Presentation is a dc+sd-jwt presentation (issuer JWT, disclosures, holder KB-JWT).
type Presentation struct {
	IssuerJWT   string
	Disclosures []string
	KBJWT       string
	SDHash      string
}

// ParsePresentation splits a compact SD-JWT+KB presentation string.
//
// RFC 9901 wire format: <issuer-jwt>~<disclosure 1>~…~<disclosure N>~<kb-jwt>.
// A presentation without key binding ends with "~" (empty last segment); DCS login
// always requires holder binding, so those are rejected. Segment contents are
// validated cryptographically downstream, not pattern-matched here.
func ParsePresentation(vpToken string) (*Presentation, error) {
	parts := strings.Split(strings.TrimSpace(vpToken), "~")
	if len(parts) < 2 {
		return nil, fmt.Errorf("vp_token is not an sd-jwt presentation")
	}

	issuerJWT := parts[0]
	if issuerJWT == "" {
		return nil, fmt.Errorf("sd-jwt presentation is missing the issuer jwt")
	}

	kbJWT := parts[len(parts)-1]
	if kbJWT == "" {
		return nil, fmt.Errorf("sd-jwt presentation is missing kb-jwt")
	}

	disclosures := parts[1 : len(parts)-1]
	for i, disclosure := range disclosures {
		if disclosure == "" {
			return nil, fmt.Errorf("sd-jwt presentation disclosure %d is empty", i+1)
		}
	}

	return &Presentation{
		IssuerJWT:   issuerJWT,
		Disclosures: disclosures,
		KBJWT:       kbJWT,
		SDHash:      SDHash(issuerJWT, disclosures),
	}, nil
}

func presentationBodyForSDHash(issuerJWT string, disclosures []string) string {
	body := issuerJWT + "~"
	for _, disclosure := range disclosures {
		body += disclosure + "~"
	}

	return body
}

// SDHash returns the RFC 9901 sd_hash for the given issuer JWT and disclosures.
func SDHash(issuerJWT string, disclosures []string) string {
	return sha256Base64URL(presentationBodyForSDHash(issuerJWT, disclosures))
}

func sha256Base64URL(value string) string {
	sum := sha256.Sum256([]byte(value))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}
