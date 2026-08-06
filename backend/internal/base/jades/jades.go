// Package jades implements the JAdES baseline-B signature profile
// (ETSI TS 119 182-1) for the machine-readable contract representation
// (DCS-FR-SM-02): a compact JWS whose protected header carries the signer's
// X.509 chain (x5c) and the ETSI claimed-signing-time header (sigT, marked
// critical). The DCS-to-DCS synchronizer signs every contract it broadcasts
// with the instance's HSM-backed P-256 DID key, and the receiving peer
// verifies the signature — and its binding to the sender's did:web document
// key — before accepting the synced contract.
package jades

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/gowebpki/jcs"

	"digital-contracting-service/internal/base/identity"
)

// protectedHeader is the JAdES B-B protected header. sigT (claimed signing
// time, ETSI TS 119 182-1 §5.1.1) is REQUIRED for the baseline profile and
// must be listed in crit since it is not a plain RFC 7515 header.
type protectedHeader struct {
	Alg  string   `json:"alg"`
	Typ  string   `json:"typ"`
	Cty  string   `json:"cty"`
	X5C  []string `json:"x5c"`
	SigT string   `json:"sigT"`
	Crit []string `json:"crit"`
}

// BuildContractPayload canonicalizes the signed contract representation —
// DID, version, and the full JSON-LD contract document — with the JSON
// Canonicalization Scheme (RFC 8785), reproducible bit-for-bit from any
// conforming JCS implementation in any language.
func BuildContractPayload(did string, contractVersion int, contractData []byte) ([]byte, error) {
	var document any
	if len(contractData) == 0 {
		contractData = []byte(`{}`)
	}
	if err := json.Unmarshal(contractData, &document); err != nil {
		return nil, fmt.Errorf("decode contract document: %w", err)
	}
	payload := map[string]any{
		"dcs:contractDid":      did,
		"dcs:contractVersion":  contractVersion,
		"dcs:contractDocument": document,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(encoded)
}

// SettlementType is the @type of the settlement artifact a peer signs and
// ships on reaching its own settled state.
const SettlementType = "dcs:ContractSettlement"

// Settlement is the peer's statement that it settled a contract: which
// contract, which document (by digest — the load-bearing binding, since
// contract_version is a per-instance counter), who settled, toward whom, and
// when.
type Settlement struct {
	ContractDID     string
	ContractVersion int
	DocumentDigest  string
	SettledBy       string
	SettledWith     string
	SettledAt       time.Time
}

// BuildSettlementPayload canonicalizes the settlement artifact with JCS
// (RFC 8785), exactly as BuildContractPayload canonicalizes a signed
// contract, so the receiver re-derives the bytes from the claimed fields
// instead of trusting the sender's serialization.
func BuildSettlementPayload(s Settlement) ([]byte, error) {
	payload := map[string]any{
		"@context":                   map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@type":                      SettlementType,
		"dcs:contractDid":            s.ContractDID,
		"dcs:contractVersion":        s.ContractVersion,
		"dcs:contractDocumentDigest": s.DocumentDigest,
		"dcs:settledBy":              s.SettledBy,
		"dcs:settledWith":            s.SettledWith,
		"dcs:settledAt":              s.SettledAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(encoded)
}

// SettlementWithdrawalType is the @type of the artifact a peer signs and ships
// when it takes back a settlement it previously shipped.
const SettlementWithdrawalType = "dcs:ContractSettlementWithdrawal"

// SettlementWithdrawal is the peer's statement that the settlement it made of
// one named document version no longer stands. It names that document by the
// same digest the settlement carried, which is what tells the receiver which
// settlement is being taken back — a withdrawal that names a version the
// receiver no longer holds a settlement for removes nothing.
type SettlementWithdrawal struct {
	ContractDID    string
	DocumentDigest string
	WithdrawnBy    string
	WithdrawnFrom  string
	WithdrawnAt    time.Time
}

// BuildSettlementWithdrawalPayload canonicalizes the withdrawal artifact with
// JCS (RFC 8785), as BuildSettlementPayload does for the settlement it revokes.
func BuildSettlementWithdrawalPayload(w SettlementWithdrawal) ([]byte, error) {
	payload := map[string]any{
		"@context":                   map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@type":                      SettlementWithdrawalType,
		"dcs:contractDid":            w.ContractDID,
		"dcs:contractDocumentDigest": w.DocumentDigest,
		"dcs:withdrawnBy":            w.WithdrawnBy,
		"dcs:withdrawnFrom":          w.WithdrawnFrom,
		"dcs:withdrawnAt":            w.WithdrawnAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(encoded)
}

// ContractDocumentDigest is the version identity of a contract document that
// both instances can compute from their own copy: the SHA-256 of its JCS
// canonicalization. Canonicalizing first is what makes it survive the
// serialization each side's jsonb column applies to the same document.
func ContractDocumentDigest(contractData []byte) (string, error) {
	if len(contractData) == 0 {
		contractData = []byte(`{}`)
	}
	var document any
	if err := json.Unmarshal(contractData, &document); err != nil {
		return "", fmt.Errorf("decode contract document: %w", err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// Sign produces a JAdES baseline-B compact JWS over payload using the DID
// document's HSM-backed P-256 key and its x5c certificate chain.
func Sign(d *identity.DIDDocument, payload []byte) (string, error) {
	// The chain that travels is the one belonging to the key that signs, which
	// the document publishes for this instance's signer — not whichever method
	// happens to come first.
	method := d.SigningMethod()
	if method == nil {
		return "", errors.New("jades: DID document is not bound to a signer")
	}
	x5c := method.PublicKeyJWK.X5C
	if len(x5c) == 0 {
		return "", fmt.Errorf("jades: verification method %q carries no x5c certificate chain", method.ID)
	}

	header := protectedHeader{
		Alg:  "ES256",
		Typ:  "jose",
		Cty:  "application/json",
		X5C:  x5c,
		SigT: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Crit: []string{"sigT"},
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) +
		"." + base64.RawURLEncoding.EncodeToString(payload)

	derSig, err := d.Sign([]byte(signingInput))
	if err != nil {
		return "", fmt.Errorf("jades: signing failed: %w", err)
	}
	joseSig, err := derToJOSE(derSig)
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(joseSig), nil
}

// Verify parses and verifies a JAdES compact JWS: ES256 algorithm, the
// critical sigT header, and the signature itself against the x5c leaf
// certificate's P-256 public key. It returns the payload and the leaf
// public key; binding that key to a trusted identity (e.g. the sending
// peer's did:web document) is the caller's responsibility.
func Verify(jws string) (payload []byte, leaf *ecdsa.PublicKey, err error) {
	parts := strings.Split(jws, ".")
	if len(parts) != 3 {
		return nil, nil, errors.New("jades: expected a compact JWS with three segments")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("jades: decode protected header: %w", err)
	}
	var header protectedHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, nil, fmt.Errorf("jades: parse protected header: %w", err)
	}
	if header.Alg != "ES256" {
		return nil, nil, fmt.Errorf("jades: unsupported alg %q (only ES256)", header.Alg)
	}
	if header.SigT == "" {
		return nil, nil, errors.New("jades: missing sigT (claimed signing time) header")
	}
	critSeen := false
	for _, c := range header.Crit {
		switch c {
		case "sigT":
			critSeen = true
		default:
			return nil, nil, fmt.Errorf("jades: unsupported critical header %q", c)
		}
	}
	if !critSeen {
		return nil, nil, errors.New("jades: sigT must be marked critical (crit)")
	}
	if len(header.X5C) == 0 {
		return nil, nil, errors.New("jades: missing x5c certificate chain")
	}

	leafDER, err := base64.StdEncoding.DecodeString(header.X5C[0])
	if err != nil {
		return nil, nil, fmt.Errorf("jades: decode x5c leaf: %w", err)
	}
	leafCert, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, nil, fmt.Errorf("jades: parse x5c leaf: %w", err)
	}
	leafKey, ok := leafCert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, nil, errors.New("jades: x5c leaf key is not ECDSA")
	}

	payload, err = base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("jades: decode payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, fmt.Errorf("jades: decode signature: %w", err)
	}
	if len(sig) != 64 {
		return nil, nil, fmt.Errorf("jades: expected a 64-byte ES256 signature, got %d bytes", len(sig))
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(leafKey, digest[:], r, s) {
		return nil, nil, errors.New("jades: ES256 signature verification failed")
	}

	return payload, leafKey, nil
}

// derToJOSE converts an ASN.1 DER-encoded ECDSA signature (what crypto.Signer
// implementations, including PKCS#11-backed ones, return) into the fixed
// 64-byte r||s form JWS requires for ES256.
func derToJOSE(der []byte) ([]byte, error) {
	var sig struct {
		R, S *big.Int
	}
	if _, err := asn1.Unmarshal(der, &sig); err != nil {
		return nil, fmt.Errorf("jades: parse DER signature: %w", err)
	}
	out := make([]byte, 64)
	sig.R.FillBytes(out[:32])
	sig.S.FillBytes(out[32:])
	return out, nil
}
