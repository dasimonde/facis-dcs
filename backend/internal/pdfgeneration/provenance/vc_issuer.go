package provenance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/tsa"
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
	timestamper         Timestamper
}

// Timestamper obtains an RFC 3161 timestamp token over data. The TSA client
// (base/tsa) is the implementation.
type Timestamper interface {
	Enabled() bool
	TimestampBytes(ctx context.Context, data []byte) (*tsa.Receipt, error)
}

// timestampTimeout bounds one timestamp request of a lifecycle credential.
const timestampTimeout = 5 * time.Second

// WithTimestamper makes every lifecycle credential carry an RFC 3161 timestamp
// token over its assertion (DCS-OR-C2PA-009).
func (v *LocalVCIssuer) WithTimestamper(t Timestamper) *LocalVCIssuer {
	v.timestamper = t
	return v
}

// timestamp returns the RFC 3161 token over the assertion, or "" when no TSA
// is configured or it did not answer. The timestamp is a SHOULD of
// DCS-OR-C2PA-009, so an unreachable TSA does not stop a lifecycle event; the
// credential then carries no token and the audit checkpoint's timestamp
// stands for it.
func (v *LocalVCIssuer) timestamp(ctx context.Context, assertion LifecycleAssertion) string {
	if v.timestamper == nil || !v.timestamper.Enabled() {
		return ""
	}
	// Bounded so a slow TSA cannot hold a signing ceremony or a regeneration.
	ctx, cancel := context.WithTimeout(ctx, timestampTimeout)
	defer cancel()
	receipt, err := v.timestamper.TimestampBytes(ctx, TimestampedAssertionBytes(assertion))
	if err != nil {
		log.Printf("provenance: no RFC 3161 timestamp for the %s lifecycle credential of %s: %v", assertion.Status, assertion.ContractID, err)
		return ""
	}
	return receipt.Token
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
	assertion.Timestamp = v.timestamp(ctx, assertion)

	// Issue and sign the W3C VC via the Crypto Provider Service (DCS-IR-SI-10).
	// Pass the status list entry so credentialStatus is embedded in the VC (DCS-OR-C2PA-005).
	signedVC, vcID, err := IssueLifecycleVC(ctx, v.vcSigner, v.issuer, statusEntry, assertion)
	if err != nil {
		return "", nil, fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}
	return vcID, signedVC, nil
}
