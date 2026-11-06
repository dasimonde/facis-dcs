package provenance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LifecycleAssertion is one contract lifecycle event (DCS-OR-C2PA-003): the
// contract's state at the moment it was recorded, so verifiers can reconstruct
// the full lifecycle history.
//
// It is the input to the lifecycle VC (IssueLifecycleVC) — the credential
// attached to the artifact as contract-lifecycle-vc.json. The matching
// dcs.lifecycle C2PA assertion is rendered by pdf-core from the same event and
// is NOT built from this struct; the two carry the same field names but see
// FileHash for the one place where the same name means two things.
type LifecycleAssertion struct {
	// Label identifies this assertion type.
	Label string `json:"label"`

	// ContractID is the contract's DID.
	ContractID string `json:"contract_id"`

	// FileHash is the SHA-256 (hex-encoded) of the protected PDF artifact bytes
	// as they stand immediately before the update that attaches this event's
	// credential. It reaches the artifact as VC credentialSubject.file_hash, and
	// a verifier reproduces it by truncating the artifact at that update and
	// hashing the prefix — the incremental update leaves the earlier revision
	// byte-for-byte intact.
	//
	// The C2PA dcs.lifecycle assertion's field of the same name is a DIFFERENT
	// value: the SHA-256 of the embedded contract.jsonld. It has to be. That
	// assertion sits inside the manifest that is embedded in the PDF, so it
	// cannot hash the bytes it is part of; binding the whole document is the job
	// of c2pa.hash.data, which hashes the file with the manifest range excluded.
	// See pdf-core TestLifecycleAssertionFileHashIsTheEmbeddedPayloadHash.
	FileHash string `json:"file_hash"`

	// Status is the contract lifecycle state at assertion time
	// (draft, active, amended, suspended, terminated, expired, replaced).
	Status string `json:"status"`

	// Reason is the human-readable reason for the state transition (may be empty).
	Reason string `json:"reason,omitempty"`

	// EffectiveAt is when this lifecycle state became effective.
	EffectiveAt time.Time `json:"effective_at"`

	// Authority is the DID of the entity asserting this lifecycle event.
	Authority string `json:"authority"`

	// VCId is the identifier of the W3C VC that records this lifecycle event
	// (DCS-OR-C2PA-004). Empty until the VC is issued.
	VCId string `json:"vc_id,omitempty"`

	// Timestamp is the RFC 3161 timestamp token (base64 DER) over
	// TimestampedAssertionBytes of this assertion (DCS-OR-C2PA-009). Empty when
	// no TSA is configured or the TSA did not answer.
	Timestamp string `json:"rfc3161_timestamp,omitempty"`
}

// TimestampedAssertionBytes is what the RFC 3161 timestamp of a lifecycle
// assertion covers: its contract_id, file_hash, status, reason and
// effective_at, as the JSON object the credential subject carries them in.
func TimestampedAssertionBytes(a LifecycleAssertion) []byte {
	raw, _ := json.Marshal(struct {
		ContractID  string `json:"contract_id"`
		FileHash    string `json:"file_hash"`
		Status      string `json:"status"`
		Reason      string `json:"reason"`
		EffectiveAt string `json:"effective_at"`
	}{a.ContractID, a.FileHash, a.Status, a.Reason, a.EffectiveAt.UTC().Format(time.RFC3339)})
	return raw
}

const lifecycleAssertionLabel = "org.facis.dcs.contract.lifecycle"

// NewLifecycleAssertion constructs a LifecycleAssertion with the required fields.
func NewLifecycleAssertion(contractID, fileHash, status, reason, authority, vcID string, effectiveAt time.Time) LifecycleAssertion {
	return LifecycleAssertion{
		Label:       lifecycleAssertionLabel,
		ContractID:  contractID,
		FileHash:    fileHash,
		Status:      status,
		Reason:      reason,
		EffectiveAt: effectiveAt,
		Authority:   authority,
		VCId:        vcID,
	}
}

// MapCWEStateToC2PA maps a CWE contract state to the canonical C2PA lifecycle
// vocabulary defined in DCS-OR-C2PA-003. Unsupported states return an error.
//
// OFFERED/NEGOTIATION/SUBMITTED/REVIEWED/APPROVED all map to "draft"
// (pre-signing contract-formation states — there is no executable/binding
// manifest yet), SIGNED/ACTIVE map to "active", REVOKED maps to
// "suspended", TERMINATED/EXPIRED map 1:1. APPROVED deliberately maps to
// "draft", not "active": approval alone does not make a contract binding.
//
// REJECTED and WITHDRAWN are not specified by DCS-OR-C2PA-003. Both are
// pre-signing terminal states reached before any manifest would be expected
// to exist, so both map to "draft" as the closest sensible SRS term.
func MapCWEStateToC2PA(cweState string) (string, error) {
	switch strings.ToUpper(cweState) {
	case "DRAFT":
		return "draft", nil
	case "OFFERED", "NEGOTIATION", "SUBMITTED", "REVIEWED", "APPROVED":
		// Pre-signing contract-formation states (C4): no binding manifest
		// exists yet, so all map to "draft".
		return "draft", nil
	case "REJECTED", "WITHDRAWN":
		// Not C2PA-003-specified; both are pre-signing terminal states, so
		// treated the same as the other pre-signing states (see doc comment).
		return "draft", nil
	case "SIGNED", "ACTIVE":
		return "active", nil
	case "REVOKED":
		return "suspended", nil
	case "REGISTERED":
		// Template-only catalogue state (not a CWE contract state): a
		// registered template is live/searchable, closest SRS equivalent is
		// "active".
		return "active", nil
	case "TERMINATED":
		return "terminated", nil
	case "EXPIRED":
		return "expired", nil
	case "SUSPENDED":
		return "suspended", nil
	case "REPLACED":
		return "replaced", nil
	default:
		// Pass-through if the caller already uses the SRS vocabulary.
		lower := strings.ToLower(cweState)
		switch lower {
		case "draft", "active", "amended", "suspended", "terminated", "expired", "replaced":
			return lower, nil
		}
		return "", fmt.Errorf("unsupported lifecycle state %q (allowed: DRAFT,OFFERED,NEGOTIATION,SUBMITTED,REVIEWED,APPROVED,REJECTED,WITHDRAWN,SIGNED,ACTIVE,REVOKED,REGISTERED,TERMINATED,EXPIRED,SUSPENDED,REPLACED,draft,active,amended,suspended,terminated,expired,replaced)", cweState)
	}
}

// DefaultLifecycleReason is the reason a lifecycle assertion records when the
// event that caused it names none: the expiry cron and the retry sweep carry no
// reason, and an assertion must carry one (DCS-OR-C2PA-003).
func DefaultLifecycleReason(state string) string {
	switch state {
	case "draft":
		return "Contract created as draft"
	case "active":
		return "Contract activated for execution"
	case "amended":
		return "Contract amended with new terms"
	case "suspended":
		return "Contract suspended pending review"
	case "terminated":
		return "Contract terminated by parties"
	case "expired":
		return "Contract reached expiration date"
	case "replaced":
		return "Contract replaced with newer version"
	default:
		return "Contract state changed to: " + state
	}
}

// LifecycleReason is the event's reason, or DefaultLifecycleReason when the
// event names none.
func LifecycleReason(eventReason, state string) string {
	if strings.TrimSpace(eventReason) != "" {
		return eventReason
	}
	return DefaultLifecycleReason(state)
}

// IsFrozenC2PAState reports whether a cached PDF's C2PA state means the artifact
// is immutable. Every pre-signing contract-formation state maps to "draft" (see
// MapCWEStateToC2PA), so anything past "draft" is a PAdES-signed (or
// post-signing) PDF whose /ByteRange a re-render would destroy. Such a PDF may
// only be served as-is or extended by an explicit incremental C2PA update — the
// export/verify read paths and the background regenerator all gate on this.
func IsFrozenC2PAState(c2paState string) bool {
	return c2paState != "" && c2paState != "draft"
}

// IsPostSigningLifecycleState reports whether c2paState is a lifecycle state a
// signed contract reaches after signing. Its lifecycle manifest and credential
// are appended to the signed PDF as an incremental update, as the re-anchor is
// (ADR-26), so every signature keeps verifying.
func IsPostSigningLifecycleState(c2paState string) bool {
	switch c2paState {
	case "suspended", "terminated", "expired":
		return true
	}
	return false
}

// SignedLifecycleAppendPending reports whether the signed PDF stored for a
// contract does not yet assert the post-signing lifecycle state target.
func SignedLifecycleAppendPending(signed bool, storedC2PAState, target string) bool {
	return signed && IsPostSigningLifecycleState(target) && storedC2PAState != target
}

// CarriesPAdESSignature reports whether pdf already holds a PAdES signature: a
// signature value dictionary (/Type /Sig) carrying a /ByteRange. Both names are
// required, and both are matched on the name a conforming reader resolves, with
// #xx escapes decoded (ISO 32000-1 7.3.5).
//
// This is pdf-core's isPAdESSigned, and the two must agree: pdf-core refuses to
// re-render a PDF it calls signed, so a PDF only this side calls unsigned is one
// the regenerator keeps trying to amend, and a PDF only this side calls signed
// is one frozen as "active" forever — the received artifact then asserts an
// active contract while the workflow is merely OFFERED, and no local transition
// corrects it.
//
// Requiring both names is what keeps the contract's own text out of the answer.
// Clause text reaches the page content stream and the JSON-LD attachment
// verbatim, so a clause discussing /ByteRange writes that name into the file;
// decoding escapes is what keeps a peer's /Byte#52ange in it.
func CarriesPAdESSignature(pdf []byte) bool {
	names := pdfNameTokens(pdf)
	byteRange, sigType := false, false
	for i, name := range names {
		switch name.text {
		case "/ByteRange":
			byteRange = true
		case "/Type":
			// The value follows the key directly, white space apart; the two
			// spellings "/Type /Sig" and "/Type/Sig" are the same dictionary
			// entry.
			if i+1 < len(names) && names[i+1].text == "/Sig" && isPDFSpace(pdf[name.end:names[i+1].start]) {
				sigType = true
			}
		}
		if byteRange && sigType {
			return true
		}
	}
	return false
}

// ExtendsSignedRevision reports whether next can replace current without
// dropping a signature: current holds no PAdES signature, or next starts with
// current (an incremental update). Each PAdES signature covers all earlier
// revisions, so a document that does not start with a signed revision does
// not contain the signatures of that revision.
func ExtendsSignedRevision(current, next []byte) bool {
	return !CarriesPAdESSignature(current) || bytes.HasPrefix(next, current)
}

// pdfNameToken is one PDF name in a document, as a reader resolves it.
type pdfNameToken struct {
	text       string
	start, end int
}

// pdfNameTokens returns every name token in pdf in file order, with #xx escapes
// decoded.
func pdfNameTokens(pdf []byte) []pdfNameToken {
	var names []pdfNameToken
	for pos := 0; pos < len(pdf); pos++ {
		if pdf[pos] != '/' {
			continue
		}
		var decoded strings.Builder
		decoded.WriteByte('/')
		end := pos + 1
		for end < len(pdf) && !isPDFWhitespaceByte(pdf[end]) && !isPDFDelimiterByte(pdf[end]) {
			if pdf[end] == '#' && end+2 < len(pdf) {
				if b, err := strconv.ParseUint(string(pdf[end+1:end+3]), 16, 8); err == nil {
					decoded.WriteByte(byte(b))
					end += 3
					continue
				}
			}
			decoded.WriteByte(pdf[end])
			end++
		}
		names = append(names, pdfNameToken{text: decoded.String(), start: pos, end: end})
		pos = end - 1
	}
	return names
}

// isPDFSpace reports whether b is entirely PDF white space.
func isPDFSpace(b []byte) bool {
	for _, c := range b {
		if !isPDFWhitespaceByte(c) {
			return false
		}
	}
	return true
}

// isPDFWhitespaceByte reports whether b is one of the six PDF white-space
// characters (ISO 32000-1 table 1).
func isPDFWhitespaceByte(b byte) bool {
	switch b {
	case 0x00, '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// isPDFDelimiterByte reports whether b is one of the delimiter characters
// (ISO 32000-1 table 2), which end a name just as white space does.
func isPDFDelimiterByte(b byte) bool {
	switch b {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// ArtifactC2PAState is the lifecycle state a STORED artifact asserts, which is
// not always what the contract's own state maps to. A peer ships a PDF it has
// already signed while the receiving instance's workflow starts over at OFFERED,
// so recording the local state would file a PAdES-signed artifact as "draft" —
// re-renderable, and the next local terminate or expiry would append a manifest
// onto the counterparty's signed bytes. The artifact is the witness: it carries
// the /ByteRange a re-render would destroy, whatever the local workflow thinks.
// A state that is already frozen stands (a revoked contract's artifact is
// suspended, not active).
func ArtifactC2PAState(contractState string, pdf []byte) (string, error) {
	state, err := MapCWEStateToC2PA(contractState)
	if err != nil {
		return "", err
	}
	if IsFrozenC2PAState(state) || !CarriesPAdESSignature(pdf) {
		return state, nil
	}
	return MapCWEStateToC2PA("SIGNED")
}
