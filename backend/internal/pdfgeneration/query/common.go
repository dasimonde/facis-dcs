package query

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

type PDFStateData struct {
	IPFSCID         string
	RendererVersion string
	C2PAState       string
	PayloadHash     string
}

type pdfStateUpdater func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error

// pdfSignatureNotAvailable is the honest PDF-signature check status reported
// by the verify endpoint (DCS-OR-C2PA-006): the verify path does not
// cryptographically re-verify the PAdES CMS signature over its /ByteRange
// (pdf-core's /verify treats the signed span as an opaque suffix), and the
// verifier must never falsely report a passed PDF signature check it did not
// actually perform.
const pdfSignatureNotAvailable = provenance.CheckNotAvailable

const (
	statusPublicationConsistencyWait = 6 * time.Second
	statusPublicationRetryInterval   = 100 * time.Millisecond
)

// Failure classes reported in PDFVerifyResult.Discrepancy.
const (
	discrepancyNone         = ""
	discrepancyHashMismatch = "content_hash_mismatch"
	discrepancyNotAuthentic = "artifact_not_authentic"
	discrepancyFailed       = "verification_failed"
)

// stampLifecycle embeds a C2PA lifecycle assertion (DCS-OR-C2PA-003) for the
// given contract state into pdfBytes and returns the updated PDF plus the
// renderer version pdf-core reports. It performs no IPFS storage or DB
// bookkeeping — callers decide what to do with the result. This is the
// building block shared by:
//   - apply.go, which stamps the "active" lifecycle state into the base PDF
//     BEFORE PAdES-signing it (update-then-sign), so the signed artefact
//     already carries its final pre-signature-freeze manifest and never
//     needs a post-signature revision for that transition; and
//   - appendAndCache below, for lifecycle transitions that happen entirely
//     before any PAdES signature exists (draft-state edits).
//
// A PDF that already carries a PAdES signature must never be passed to this
// function again: rewriting a referenced embedded-file object (the C2PA
// manifest attachment) after signing, however carefully byte-range-preserving,
// is treated as an unexplained modification by standards-compliant PAdES
// validators (Adobe Reader, pyHanko's diff-analysis) — even though the CMS
// signature itself stays cryptographically valid. See
// exportcontract.go/verifycontract.go, which freeze the PDF once its C2PA
// state is no longer "draft".
//
// This restriction is specific to re-stamping lifecycle state. It is not a
// general "never append to a signed PDF" rule: DCS-OR-C2PA-002 mandates
// appending via PDF incremental updates, and signingmanagement's finalize()
// appends a provenance-only manifest after signing (ADR-26). (There is no
// DCS-FR-SM-16/B sub-clause; the earlier citation here was wrong.)
func stampLifecycle(
	ctx context.Context,
	did, state string,
	jsonldBytes, pdfBytes []byte,
	pdfCore *pdfcore.Client,
	vcIssuer provenance.VCIssuer,
	issuerDID string,
) ([]byte, string, error) {
	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return pdfBytes, "", fmt.Errorf("map lifecycle state %q: %w", state, err)
	}

	log.Printf("pdfgeneration: stampLifecycle %s state=%s c2paState=%s pdfLen=%d",
		did, state, c2paState, len(pdfBytes))

	reason := stateToReason(c2paState)

	h := sha256.Sum256(pdfBytes)
	assetHash := hex.EncodeToString(h[:])

	_, vcBytes, err := vcIssuer.IssueContractLifecycleVC(
		ctx, did, assetHash, c2paState, reason, issuerDID, time.Now().UTC(),
	)
	if err != nil {
		return pdfBytes, "", fmt.Errorf("issue lifecycle VC (DCS-OR-C2PA-004): %w", err)
	}

	updatedPDF, rendererVersion, err := pdfCore.Update(ctx, pdfBytes, jsonldBytes, vcBytes, provenance.RemoteManifestURL(did))
	if err != nil {
		return pdfBytes, "", fmt.Errorf("pdf-core update for %s: %w", did, err)
	}
	return updatedPDF, rendererVersion, nil
}

func appendAndCache(
	ctx context.Context,
	tx *sqlx.Tx,
	did, state string,
	jsonldBytes, pdfBytes []byte,
	artifacts *artifactstore.Store,
	scope artifactstore.Scope,
	pdfCore *pdfcore.Client,
	vcIssuer provenance.VCIssuer,
	issuerDID string,
	updateState pdfStateUpdater,
) ([]byte, error) {
	c2paState, err := provenance.MapCWEStateToC2PA(state)
	if err != nil {
		return pdfBytes, fmt.Errorf("map lifecycle state %q: %w", state, err)
	}

	updatedPDF, rendererVersion, err := stampLifecycle(ctx, did, state, jsonldBytes, pdfBytes, pdfCore, vcIssuer, issuerDID)
	if err != nil {
		return pdfBytes, err
	}

	pdfCID, err := artifacts.Put(ctx, scope, updatedPDF)
	if err != nil {
		return updatedPDF, fmt.Errorf("store PDF in IPFS for %s: %w", did, err)
	}

	if err := updateState(ctx, tx, did, PDFStateData{
		IPFSCID:         pdfCID,
		RendererVersion: rendererVersion,
		C2PAState:       c2paState,
		PayloadHash:     payloadHash(jsonldBytes),
	}); err != nil {
		return nil, fmt.Errorf("persist PDF state for %s: %w", did, err)
	}

	log.Printf("pdfgeneration: appendAndCache %s done → CID=%s pdfLen=%d", did, pdfCID, len(updatedPDF))
	return updatedPDF, nil
}

// tamperedVerifyResult is the verdict for an artifact that failed
// authenticated decryption. Every check reports negative rather than absent:
// the stored bytes are not the ones this instance sealed, so nothing about
// them verifies, and the hashes stay empty because there is no trustworthy
// content to hash.
func tamperedVerifyResult(lifecycleStatus string) *pdfgen.PDFVerifyResult {
	return &pdfgen.PDFVerifyResult{
		Match:               false,
		C2paManifestFound:   false,
		C2paSignatureStatus: provenance.CheckInvalid,
		VcProofStatus:       provenance.CheckInvalid,
		StatusListCheck:     "failed",
		StatusListError:     ptrToString("stored artifact is not authentic"),
		LifecycleStatus:     ptrToString(lifecycleStatus),
		PdfSignatureStatus:  pdfSignatureNotAvailable,
		Discrepancy:         ptrToString(discrepancyNotAuthentic),
	}
}

func runVerify(ctx context.Context, pdfBytes []byte, pdfCore *pdfcore.Client,
	credentials *provenance.CredentialVerifier, lifecycleStatus string) (*pdfgen.PDFVerifyResult, error) {
	result, verifyErr := pdfCore.Verify(ctx, pdfBytes)
	match := verifyErr == nil
	c2paManifestFound := verifyErr == nil
	if verifyErr != nil {
		c2paManifestFound = strings.Contains(verifyErr.Error(), "status 409")
	}

	// pdf-core answers 409 specifically for "manifest present, content hash
	// comparison failed" — the genuine MR/HR discrepancy — which is a different
	// finding from a manifest that is missing or a call that never landed.
	discrepancy := discrepancyNone
	switch {
	case verifyErr == nil:
	case c2paManifestFound:
		discrepancy = discrepancyHashMismatch
	default:
		discrepancy = discrepancyFailed
	}

	// A /verify that never returned a body carries no claim-signature verdict:
	// pdf-core reports one only for a PDF it accepted, so there is nothing to
	// report rather than a failure to report.
	c2paSignatureStatus := provenance.CheckNotAvailable
	switch {
	case verifyErr != nil:
	case result.C2PASignatureValid:
		c2paSignatureStatus = provenance.CheckValid
	default:
		c2paSignatureStatus = provenance.CheckInvalid
	}

	vcProofStatus := provenance.CheckNotAvailable
	if result.VCPresent && len(result.VCBytes) > 0 {
		vcProofStatus = provenance.CredentialCheck(credentials.Verify(result.VCBytes))
	}

	// The revocation lookup follows the credential's OWN credentialStatus, so it
	// runs only once the credential is known to be the issuer's: an unverified
	// credential points the check wherever its author chose. Without a verdict on
	// the credential the revocation state is unknown, which is said rather than
	// left empty — empty reads as "nothing to report", and that is how a revoked
	// contract came back clean.
	statusListURI := ""
	statusListStatus := ""
	statusListCheck := "not_available"
	statusListError := ""
	switch {
	case vcProofStatus == provenance.CheckValid:
		statusListURI = provenance.ExtractStatusListURI(result.VCBytes)
		ref, present, err := provenance.ExtractCredentialStatus(result.VCBytes)
		switch {
		case err != nil:
			match = false
			statusListStatus = "unavailable"
			statusListCheck = "failed"
			statusListError = fmt.Sprintf("invalid embedded status-list reference: %v", err)
		case !present:
			match = false
			statusListStatus = "unavailable"
			statusListCheck = "failed"
			statusListError = "embedded lifecycle credential has no usable status-list reference"
		default:
			httpClient := &http.Client{Timeout: 10 * time.Second}
			queryStatus := func() (string, error) {
				return provenance.QueryStatusListStatus(ctx, httpClient, ref.StatusListCredential, ref.Index)
			}
			var statusPassed bool
			statusListStatus, statusListCheck, statusListError, statusPassed =
				evaluateLiveStatusCheck(ctx, lifecycleStatus, queryStatus)
			if !statusPassed {
				match = false
			}
		}
	case vcProofStatus != provenance.CheckNotAvailable:
		statusListStatus = "UNKNOWN (lifecycle credential proof not verified)"
	}

	return &pdfgen.PDFVerifyResult{
		Match:             match,
		C2paManifestFound: c2paManifestFound,
		// The digests pdf-core reached its verdict on, carried through rather than
		// recomputed here. They ride along with a 409 content mismatch too, where
		// base and stored diverge and name which side moved; all three are empty
		// only when pdf-core could not compute them at all.
		JsonldHash:          result.JSONLDHash,
		BasePdfHash:         result.BasePDFHash,
		StoredBasePdfHash:   result.StoredBasePDFHash,
		C2paSignatureStatus: c2paSignatureStatus,
		VcProofStatus:       vcProofStatus,
		StatusListURI:       ptrToString(statusListURI),
		StatusListStatus:    ptrToString(statusListStatus),
		StatusListCheck:     statusListCheck,
		StatusListError:     ptrToString(statusListError),
		LifecycleStatus:     ptrToString(lifecycleStatus),
		// DCS-OR-C2PA-006: the PDF-signature check is an independently named
		// check, distinct from the C2PA COSE signature check. This path performs
		// no cryptographic PAdES re-verification, so it honestly reports
		// "not_available" rather than faking a passed PDF-signature verification.
		PdfSignatureStatus: pdfSignatureNotAvailable,
		Discrepancy:        ptrToString(discrepancy),
	}, nil
}

func evaluateLiveStatusCheck(
	ctx context.Context,
	lifecycleStatus string,
	query func() (string, error),
) (status, check, failure string, passed bool) {
	status, err := queryStatusWithPublicationBarrier(ctx, lifecycleStatus, query)
	if err != nil {
		return "unavailable", "failed", err.Error(), false
	}
	return status, "passed", "", true
}

func queryStatusWithPublicationBarrier(
	ctx context.Context,
	lifecycleStatus string,
	query func() (string, error),
) (string, error) {
	status, err := query()
	if err != nil || status != "active" ||
		(lifecycleStatus != "terminated" && lifecycleStatus != "suspended") {
		return status, err
	}

	timer := time.NewTimer(statusPublicationConsistencyWait)
	defer timer.Stop()
	ticker := time.NewTicker(statusPublicationRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return status, nil
		case <-ticker.C:
			status, err = query()
			if err != nil || status != "active" {
				return status, err
			}
		}
	}
}

func stateToReason(state string) string {
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

func ptrToString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func payloadHash(jsonld []byte) string {
	h := sha256.Sum256(jsonld)
	return hex.EncodeToString(h[:])
}
