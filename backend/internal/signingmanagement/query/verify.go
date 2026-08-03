package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/db"
	event2 "digital-contracting-service/internal/signingmanagement/event"

	"github.com/jmoiron/sqlx"
)

// SignatureVerifyQry carries the inputs for verifying a contract's signatures.
type SignatureVerifyQry struct {
	DID        string
	VerifiedBy string
	HolderDID  string
	UserRoles  userrole.UserRoles
}

// SignatureVerifyResult holds the signature verification summary.
type SignatureVerifyResult struct {
	Match    bool
	Findings []string
	// SigCount is the number of non-revoked signatures on the contract.
	SigCount    int
	JsonldHash  *string
	BasePdfHash *string
}

// SignatureVerifier handles the SignatureVerifyQry command.
type SignatureVerifier struct {
	DB      *sqlx.DB
	CRepo   db.ContractRepo
	PDFCore *pdfcore.Client
	// Credentials verifies the lifecycle credential embedded in the stored PDF
	// against the key its issuer publishes for assertions.
	Credentials *provenance.CredentialVerifier
	// CredentialStatus resolves that credential's revocation entry against the
	// status list it names, and only once that list's own signature verified.
	CredentialStatus *provenance.CredentialStatusVerifier
}

// Handle verifies that the contract is APPROVED and returns the count of
// active (non-revoked) signatures.
func (h *SignatureVerifier) Handle(ctx context.Context, cmd SignatureVerifyQry) (*SignatureVerifyResult, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	// Validates APPROVED state via repo filter.
	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("contract %s not available for verification: %w", cmd.DID, err)
	}

	sigCount, err := h.CRepo.CountSignatureForContractDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not count signature for contract %s: %w", cmd.DID, err)
	}

	// Fetch PDF bytes and run MR/HR hash check (DCS-FR-CWE-04).
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, cmd.DID)
	if err != nil || len(pdfBytes) == 0 {
		// Without stored bytes no check ran at all, which is a different result
		// from a check that ran and failed. match=false alone reads as the
		// latter, so the reason is named.
		finding := "No contract PDF stored yet — no integrity check was performed"
		if err != nil {
			finding = fmt.Sprintf("Contract PDF could not be read, so no integrity check was performed: %v", err)
		}
		return &SignatureVerifyResult{
			Match:    false,
			SigCount: sigCount,
			Findings: []string{finding},
		}, nil
	}

	// pdf-core /verify: re-renders JSON-LD and compares, validates C2PA chain.
	// 200 → intact; 409 → content mismatch; other → C2PA invalid.
	verifyResult, verifyErr := h.PDFCore.Verify(ctx, pdfBytes)
	match := verifyErr == nil
	c2paManifestFound := verifyErr == nil || (verifyErr != nil && strings.Contains(verifyErr.Error(), "status 409"))

	// The claim-signature verdict is pdf-core's COSE check (every manifest's claim
	// signature against its own x5chain leaf, plus the assertion hashes the signed
	// claim commits to). A /verify that never returned a body carries no verdict at
	// all, which is a different finding from a failed one.
	c2paSignatureStatus := provenance.CheckNotAvailable
	switch {
	case verifyErr != nil:
	case verifyResult.C2PASignatureValid:
		c2paSignatureStatus = provenance.CheckValid
	default:
		c2paSignatureStatus = provenance.CheckInvalid
	}

	// The embedded lifecycle credential is verified against the key its ISSUER
	// publishes for assertions, not merely parsed: a contract received from a peer
	// is stored verbatim, so the credential in it is the peer's, and a credential
	// that decodes is not a credential that was issued.
	vcProofStatus := provenance.CheckNotAvailable
	var vcProofErr error
	if verifyResult.VCPresent && len(verifyResult.VCBytes) > 0 {
		vcProofErr = h.Credentials.Verify(verifyResult.VCBytes)
		vcProofStatus = provenance.CredentialCheck(vcProofErr)
	}

	// Query live revocation state from the status list the credential names
	// (DCS-OR-C2PA-006). VC bytes are returned directly by pdf-core — no PDF byte
	// scanning required. The lookup follows the credential's own
	// credentialStatus, so it runs only for a credential whose proof verified;
	// otherwise the pointer is the credential author's choice. A list that could
	// not be fetched, or whose signature did not verify, is an UNKNOWN revocation
	// state — not an absent one. Swallowing the error left statusListStatus
	// empty, the finding was never appended, and a revoked contract came back
	// clean for as long as the outage lasted.
	statusListStatus := ""
	switch {
	case vcProofStatus == provenance.CheckValid:
		_, present, refErr := provenance.ExtractCredentialStatus(verifyResult.VCBytes)
		switch {
		// An entry this build cannot read leaves the revocation state unknown, the
		// same as an outage does. Skipping it silently made it read as "nothing to
		// check", which is what an absent entry means and not what this is.
		case refErr != nil:
			statusListStatus = fmt.Sprintf("UNKNOWN (%v)", refErr)
		case present:
			state, statusErr := h.CredentialStatus.State(ctx, verifyResult.VCBytes)
			if statusErr != nil {
				statusListStatus = fmt.Sprintf("UNKNOWN (%v)", statusErr)
			} else {
				statusListStatus = state
			}
		}
	case vcProofStatus != provenance.CheckNotAvailable:
		statusListStatus = "UNKNOWN (lifecycle credential proof not verified)"
	}

	evt := event2.VerifyEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		VerifiedBy:      cmd.VerifiedBy,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	if err = event.Create(ctx, tx, evt, componenttype.SignatureManagement); err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	findings := verifyFindings(verifyFindingInputs{
		VerifyErr:           verifyErr,
		C2PAManifestFound:   c2paManifestFound,
		C2PASignatureStatus: c2paSignatureStatus,
		C2PASignatureError:  verifyResult.C2PASignatureError,
		VCProofStatus:       vcProofStatus,
		VCProofErr:          vcProofErr,
		StatusListStatus:    statusListStatus,
	})

	jsonldHash, basePDFHash := verifyDigests(verifyResult)

	return &SignatureVerifyResult{
		Match:       match,
		Findings:    findings,
		SigCount:    sigCount,
		JsonldHash:  jsonldHash,
		BasePdfHash: basePDFHash,
	}, nil
}

// verifyFindingInputs is what the verify report is assembled from: pdf-core's
// outcome plus the checks this instance ran on what pdf-core returned.
type verifyFindingInputs struct {
	VerifyErr           error
	C2PAManifestFound   bool
	C2PASignatureStatus string
	C2PASignatureError  string
	VCProofStatus       string
	VCProofErr          error
	StatusListStatus    string
}

// verifyFindings renders the human-readable reasons the viewer shows beside the
// verdict. A finding must be something a check established. When pdf-core
// refused the document, the checks that would have run on its response did not
// run, and their inputs are absent for that reason alone — reporting that
// absence as a property of the document turns one refusal into a list of
// fabricated defects.
func verifyFindings(in verifyFindingInputs) []string {
	findings := make([]string, 0, 5)
	// pdf-core's own reason for refusing. Without it the caller got match=false
	// and a set of secondary findings that were all consequences of that refusal,
	// with nothing naming what the refusal was.
	if in.VerifyErr != nil {
		findings = append(findings, fmt.Sprintf("Integrity check did not complete: %v", in.VerifyErr))
	}
	switch {
	case !in.C2PAManifestFound:
		findings = append(findings, "C2PA manifest not found")
	case in.C2PASignatureStatus == provenance.CheckInvalid:
		findings = append(findings, fmt.Sprintf("C2PA signature invalid: %s", in.C2PASignatureError))
	case in.C2PASignatureStatus == provenance.CheckNotAvailable:
		findings = append(findings, "C2PA signature check not available")
	}
	switch in.VCProofStatus {
	case provenance.CheckNotAvailable:
		if in.VerifyErr == nil {
			findings = append(findings, "Contract lifecycle credential missing from the PDF")
		}
	case provenance.CheckIndeterminate:
		findings = append(findings, fmt.Sprintf("Contract lifecycle credential proof is indeterminate: %v", in.VCProofErr))
	case provenance.CheckInvalid:
		findings = append(findings, fmt.Sprintf("Contract lifecycle credential proof invalid: %v", in.VCProofErr))
	}
	if status := strings.TrimSpace(in.StatusListStatus); status != "" {
		findings = append(findings, fmt.Sprintf("Status list state: %s", status))
	}
	return findings
}

// verifyDigests carries the digests pdf-core reached its verdict on onto the two
// optional response fields: the embedded machine-readable payload and the
// deterministic re-render produced from it. A digest pdf-core could not compute
// stays absent rather than becoming a pointer to "" — an empty hash states
// something about the artifact, where an absent one states that no digest was
// taken. They are populated on a content mismatch too, where the re-render
// digest is precisely what the stored bytes failed to match.
func verifyDigests(result pdfcore.VerifyResult) (jsonldHash, basePDFHash *string) {
	optional := func(digest string) *string {
		if digest == "" {
			return nil
		}
		return &digest
	}
	return optional(result.JSONLDHash), optional(result.BasePDFHash)
}
