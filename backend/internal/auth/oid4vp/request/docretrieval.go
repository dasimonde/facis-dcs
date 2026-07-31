package request

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SHA256OID is the ETSI/NIST object identifier for SHA-256, the hashAlgorithmOID
// the EUDI walletdriven-signer reference advertises for document digests.
const SHA256OID = "2.16.840.1.101.3.4.2.1"

// DocumentDigest names one document the wallet is asked to sign: the base64
// (standard, not URL) hash of the to-be-signed bytes and a human label. The
// field order and encoding mirror the EUDI reference (get_document_digest).
type DocumentDigest struct {
	Hash  string `json:"hash"`
	Label string `json:"label"`
}

// DocumentLocationMethod is how the wallet retrieves the document. "public" is
// an unauthenticated GET (the EUDI reference's only method).
type DocumentLocationMethod struct {
	Type string `json:"type"`
}

// DocumentLocation is where the wallet fetches one to-be-signed document from,
// matching the EUDI reference (get_document_location): {uri, method:{type}}.
type DocumentLocation struct {
	URI    string                 `json:"uri"`
	Method DocumentLocationMethod `json:"method"`
}

// DocRetrievalParams are the parameters of an OID4VP "Document Retrieval"
// request object (EUDI walletdriven-signer). The wallet fetches the documents
// from DocumentLocations, drives its own SCA+QTSP, and posts the signed
// documents back to ResponseURI. The DCS never signs.
type DocRetrievalParams struct {
	// ClientID is the full OpenID4VP client identifier, prefix included
	// (X509SANDNSClientID) — the same one the ceremony's deep link and its
	// identity request object carry.
	ClientID    string
	ResponseURI string
	Nonce       string
	ExpiresAt   time.Time
	// SignatureQualifier is the eIDAS level requested (CSC vocabulary,
	// e.g. "eu_eidas_aes" for an AES, "eu_eidas_qes" for a QES).
	SignatureQualifier string
	DocumentDigests    []DocumentDigest
	DocumentLocations  []DocumentLocation
	// WalletNonce is the nonce a wallet supplied when it fetched this request
	// object by POST, echoed back so the wallet can tell the response apart
	// from a replayed one. The ceremony's deep link asks for
	// request_uri_method=post, so a wallet that takes that replay protection
	// refuses a request object that drops its nonce. Empty when the wallet
	// fetched by GET or sent none.
	WalletNonce string
}

// BuildDocumentRetrievalJWT creates the signed request object (JAR) a wallet
// consumes to sign the DCS's prepared documents. Its claim set follows the EUDI
// walletdriven-signer reference's generate_request_object: response_type
// "sign_response", response_mode "direct_post", and the camelCase
// documentDigests/documentLocations/hashAlgorithmOID members.
//
// The client identifier carries its scheme as a prefix and no separate
// client_id_scheme claim accompanies it: that is the OpenID4VP 1.0 / HAIP
// encoding, where the prefix is part of the identifier, and the bare value
// plus client_id_scheme is the superseded pre-1.0 draft encoding an
// ARF-compliant wallet may reject. This choice is taken from those
// specifications directly — the SRS asks only that the chosen natural-person
// wallet demonstrate ARF compliance and says nothing about the encoding. The
// identifier is the same value the ceremony's deep link and its identity
// request object name, so the one request_uri cannot present two different
// verifiers.
//
// signer is an X5CSigner carrying the DCS's own DID/hostname x5c chain in the
// JAR header (not a bare jwk), so the wallet resolves the DNS name the
// identifier claims from the leaf certificate's SAN.
// Structurally x509_san_dns-conformant; still never exercised against an
// actual EUDI wallet implementation, only the project's own testWallet stand-in.
func BuildDocumentRetrievalJWT(signer Signer, params DocRetrievalParams) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("request signer is not configured")
	}
	clientID := strings.TrimSpace(params.ClientID)
	if clientID == "" {
		return "", fmt.Errorf("client_id is required")
	}
	responseURI := strings.TrimSpace(params.ResponseURI)
	if responseURI == "" {
		return "", fmt.Errorf("response_uri is required")
	}
	nonce := strings.TrimSpace(params.Nonce)
	if nonce == "" {
		return "", fmt.Errorf("nonce is required")
	}
	qualifier := strings.TrimSpace(params.SignatureQualifier)
	if qualifier == "" {
		return "", fmt.Errorf("signatureQualifier is required")
	}
	if len(params.DocumentDigests) == 0 || len(params.DocumentDigests) != len(params.DocumentLocations) {
		return "", fmt.Errorf("documentDigests and documentLocations must be non-empty and parallel")
	}
	now := time.Now().UTC()
	exp := params.ExpiresAt.UTC()
	if !exp.After(now) {
		return "", fmt.Errorf("request expiry must be in the future")
	}

	digests := make([]any, 0, len(params.DocumentDigests))
	for _, d := range params.DocumentDigests {
		digests = append(digests, map[string]any{"hash": d.Hash, "label": d.Label})
	}
	locations := make([]any, 0, len(params.DocumentLocations))
	for _, l := range params.DocumentLocations {
		locations = append(locations, map[string]any{"uri": l.URI, "method": map[string]any{"type": l.Method.Type}})
	}

	claims := jwt.MapClaims{
		// iss names the verifier that signed this request object (RFC 9101); a
		// wallet checks it against client_id before trusting the request, the same
		// as for the identity request object BuildJWT produces.
		"iss":                clientID,
		"client_id":          clientID,
		"response_type":      "sign_response",
		"response_mode":      "direct_post",
		"response_uri":       responseURI,
		"nonce":              nonce,
		"signatureQualifier": qualifier,
		"documentDigests":    digests,
		"documentLocations":  locations,
		"hashAlgorithmOID":   SHA256OID,
		"iat":                now.Unix(),
		"exp":                exp.Unix(),
	}

	if walletNonce := strings.TrimSpace(params.WalletNonce); walletNonce != "" {
		claims["wallet_nonce"] = walletNonce
	}

	return signer.SignAuthorizationRequestJWT(claims)
}
