package provenance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// VCIssuer handles W3C Verifiable Credential issuance for contract lifecycle events (DCS-OR-C2PA-004).
type VCIssuer interface {
	// IssueContractLifecycleVC creates and signs a W3C VC for a contract lifecycle event.
	// Returns the VC ID (a URI that identifies the issued credential) and the signed VC bytes.
	IssueContractLifecycleVC(
		ctx context.Context,
		contractID, fileHash, status, reason, authority string,
		effectiveAt time.Time,
	) (vcID string, vcBytes json.RawMessage, err error)
}

// LocalVCIssuer signs VCs via the Crypto Provider Service and publishes status to the status list.
// Status list binding is atomic with VC issuance: no VC can exist without status list binding (DCS-OR-C2PA-004 + DCS-OR-C2PA-005).
type LocalVCIssuer struct {
	vcSigner            VCSigner
	issuer              string // Issuer DID
	statusListPublisher StatusListPublisher
}

// NewLocalVCIssuer creates a local VC issuer with status list binding.
// statusListPublisher must not be nil; VC issuance is atomic with status list publication.
func NewLocalVCIssuer(vcSigner VCSigner, issuerDID string, statusListPublisher StatusListPublisher) *LocalVCIssuer {
	return &LocalVCIssuer{
		vcSigner:            vcSigner,
		issuer:              issuerDID,
		statusListPublisher: statusListPublisher,
	}
}

// IssueContractLifecycleVC publishes the contract status, then builds and signs a W3C VC
// for the lifecycle event (DCS-OR-C2PA-004, DCS-OR-C2PA-005).
func (v *LocalVCIssuer) IssueContractLifecycleVC(
	ctx context.Context,
	contractID, fileHash, status, reason, authority string,
	effectiveAt time.Time,
) (vcID string, vcBytes json.RawMessage, err error) {
	// Publish to status list FIRST, before VC ID generation: it returns the
	// contract's allocated entry, which the credential then advertises.
	// Hard fail if status list publication fails; required for compliance (DCS-OR-C2PA-005).
	statusEntry, err := v.statusListPublisher.PublishStatus(ctx, contractID, status, reason, effectiveAt)
	if err != nil {
		return "", nil, fmt.Errorf("publish contract status to status list (DCS-OR-C2PA-005): %w", err)
	}

	// Build a LifecycleAssertion for the VC binding fields.
	assertion := NewLifecycleAssertion(contractID, fileHash, status, reason, authority, "", effectiveAt)

	// Issue and sign the W3C VC via the Crypto Provider Service (DCS-IR-SI-10).
	// Pass the status list entry so credentialStatus is embedded in the VC (DCS-OR-C2PA-005).
	signedVC, vcID, err := IssueLifecycleVC(ctx, v.vcSigner, v.issuer, statusEntry, assertion)
	if err != nil {
		return "", nil, fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}
	return vcID, signedVC, nil
}
