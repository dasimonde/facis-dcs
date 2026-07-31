package provenance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"digital-contracting-service/internal/base/identity"
)

// Outcomes a named verification check reports. A check that was not performed
// says so instead of borrowing the vocabulary of one that passed: the frontend's
// verdict classifier (frontend/ClientApp/src/utils/signature-verdict.ts) reads
// CheckIndeterminate and CheckNotAvailable as withheld verdicts and only
// CheckValid as a pass.
const (
	// CheckValid: the check ran and the artifact satisfied it.
	CheckValid = "valid"
	// CheckInvalid: the check ran and the artifact failed it.
	CheckInvalid = "invalid"
	// CheckIndeterminate: the artifact is there to be checked but this verifier
	// could not reach a verdict on it — an unreachable issuer, no trust
	// configuration.
	CheckIndeterminate = "indeterminate"
	// CheckNotAvailable: there is nothing to check, or this code path performs no
	// such check.
	CheckNotAvailable = "not_available"
)

// ErrIssuerUnresolved marks a credential this instance could not reach a verdict
// on, as opposed to one whose proof was checked and failed. A caller that
// reported the two the same way would turn an unreachable peer into a forged
// credential, and — the direction that actually matters — the absence of any
// means to verify into a pass.
var ErrIssuerUnresolved = errors.New("the credential's issuer could not be resolved to a key it publishes for assertions")

// CredentialVerifier verifies the Data Integrity proof of a credential carried
// inside a PDF against the key its ISSUER publishes for assertions.
//
// It exists because inbound peer PDFs are stored verbatim (they hold provenance
// and credentials this instance cannot reproduce), so a credential read back out
// of stored bytes is not necessarily one this instance issued. Parsing it proves
// nothing about who wrote it.
type CredentialVerifier struct {
	// Own is this instance's own published DID document, used for the credentials
	// it issued itself: the document is already in memory and is the one actually
	// published, rather than whatever an HTTP round trip back to ourselves returns.
	Own *identity.DIDDocument
	// Resolve fetches another issuer's DID document. Defaults to
	// identity.FetchDIDDocument.
	Resolve func(did string) (*identity.DIDDocument, error)
}

// Verify reports whether the credential's proof was made by a key its issuer
// publishes for assertions. A nil verifier is not a pass: with no way to resolve
// an issuer nothing can be verified, and that is ErrIssuerUnresolved.
func (v *CredentialVerifier) Verify(vc json.RawMessage) error {
	if v == nil {
		return fmt.Errorf("%w: no issuer resolution is configured", ErrIssuerUnresolved)
	}
	if len(strings.TrimSpace(string(vc))) == 0 {
		return errors.New("credential is empty")
	}

	var envelope struct {
		Issuer json.RawMessage `json:"issuer"`
		Proof  struct {
			VerificationMethod string `json:"verificationMethod"`
			ProofPurpose       string `json:"proofPurpose"`
		} `json:"proof"`
	}
	if err := json.Unmarshal(vc, &envelope); err != nil {
		return fmt.Errorf("decode credential: %w", err)
	}

	issuer, err := credentialIssuerID(envelope.Issuer)
	if err != nil {
		return err
	}
	// A credential is an assertion; a proof made for any other purpose does not
	// establish one, and proofPurpose is mandatory (W3C VC Data Integrity §2.1),
	// so an omitted one is a malformed proof rather than a permissive default —
	// which is what it was, and it let a proof made for any other purpose pass by
	// simply leaving the field out. It also decides which relationship the key is
	// then required to be published under, so it cannot be absent.
	if purpose := strings.TrimSpace(envelope.Proof.ProofPurpose); purpose != string(identity.PurposeAssertion) {
		return fmt.Errorf("credential proof was made for %q, not %s", purpose, identity.PurposeAssertion)
	}

	doc, err := v.issuerDocument(issuer)
	if err != nil {
		return err
	}
	// The proof says which key made it; the issuer's document says whether that
	// key may make assertions. Verifying against a key resolved by our own label
	// instead lets an issuer's proof be checked against a different one of its
	// keys than the proof claims.
	key, err := doc.AssertionKey(envelope.Proof.VerificationMethod)
	if err != nil {
		return fmt.Errorf("credential issuer %q: %w", issuer, err)
	}
	if err := VerifyDataIntegrityProof(vc, key); err != nil {
		return fmt.Errorf("credential proof does not verify against issuer %q: %w", issuer, err)
	}
	return nil
}

// issuerDocument resolves an issuer identifier to its DID document, preferring
// this instance's own in-memory document for its own credentials.
func (v *CredentialVerifier) issuerDocument(issuer string) (*identity.DIDDocument, error) {
	if v.Own != nil {
		if ownID, err := v.Own.GetID(); err == nil && identity.SameDIDWeb(issuer, ownID) {
			return v.Own, nil
		}
	}
	resolve := v.Resolve
	if resolve == nil {
		resolve = identity.FetchDIDDocument
	}
	doc, err := resolve(issuer)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrIssuerUnresolved, issuer, err)
	}
	return doc, nil
}

// credentialIssuerID reads the issuer identifier, which the VC data model writes
// either as a bare string or as an object carrying an id.
func credentialIssuerID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("credential names no issuer")
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if strings.TrimSpace(asString) == "" {
			return "", errors.New("credential names no issuer")
		}
		return strings.TrimSpace(asString), nil
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err != nil {
		return "", fmt.Errorf("decode credential issuer: %w", err)
	}
	if strings.TrimSpace(asObject.ID) == "" {
		return "", errors.New("credential names no issuer")
	}
	return strings.TrimSpace(asObject.ID), nil
}

// CredentialCheck turns a verification outcome into the status a verify result
// reports for it.
func CredentialCheck(err error) string {
	switch {
	case err == nil:
		return CheckValid
	case errors.Is(err, ErrIssuerUnresolved):
		return CheckIndeterminate
	default:
		return CheckInvalid
	}
}
