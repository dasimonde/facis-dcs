package query

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/dss"
)

// PDFSignatureValidator validates every PAdES signature a PDF carries. The EU
// DSS client (signingmanagement/dss), which /signature/validate also uses, is
// the implementation.
type PDFSignatureValidator interface {
	ValidatePDFSignatures(ctx context.Context, pdf []byte) ([]*dss.Report, error)
}

// SignerAnchors anchors the signer of each C2PA manifest (DCS-OR-C2PA-007):
// the manifest's x5chain must verify against Roots and its leaf must identify
// one of Issuers, the instances party to the document, the way a status list's
// leaf is bound to its issuer (ADR-34 §3). Roots are the anchors this
// deployment holds for peers, which include the root of its own C2PA chain.
type SignerAnchors struct {
	Roots   *x509.CertPool
	Issuers []string
}

// anchorSigners returns the C2PA signer status and, per manifest, the party
// its leaf is anchored to or why it is not.
// Limit: the chain is checked at the current time; a leaf that expired since
// it signed reports invalid until manifests carry a trusted timestamp.
func anchorSigners(signers []pdfcore.ManifestSigner, anchors SignerAnchors) (status, detail string) {
	if len(signers) == 0 {
		return provenance.CheckNotAvailable, ""
	}
	if anchors.Roots == nil || len(anchors.Issuers) == 0 {
		return provenance.CheckIndeterminate, "no trust anchors or parties to anchor the C2PA signers to"
	}
	status = provenance.CheckValid
	details := make([]string, 0, len(signers))
	for _, signer := range signers {
		chain := make([]any, 0, len(signer.Chain))
		for _, cert := range signer.Chain {
			chain = append(chain, cert)
		}
		anchoredTo := ""
		var refusal error
		for _, issuer := range anchors.Issuers {
			if _, err := sdjwt.VerificationKeyFromX5C(chain, anchors.Roots, issuer); err != nil {
				refusal = err
				continue
			}
			anchoredTo = issuer
			break
		}
		if anchoredTo == "" {
			status = provenance.CheckInvalid
			details = append(details, fmt.Sprintf("%s signed by %q is not anchored to a party: %v", signer.Manifest, signer.Subject, refusal))
			continue
		}
		details = append(details, fmt.Sprintf("%s signed by %s", signer.Manifest, anchoredTo))
	}
	return status, strings.Join(details, "; ")
}

// pdfSignatureSourceDSS names the validator behind pdf_signature_status.
const pdfSignatureSourceDSS = "eu-dss"

// checkPDFSignatures is the PDF-signature check of the verifier
// (DCS-OR-C2PA-006): every PAdES signature is validated by the EU DSS and must
// be a sound advanced electronic signature, by the same criteria the signing
// path accepts a signature with (dss.Report.AssertValidAES). A contract that
// records qualified QES signatures must also have that many signatures that
// validate as QES (ADR-40), so a QES that lost its long-term validation shows
// as invalid. It returns the status, the source and the indication of each
// signature.
func checkPDFSignatures(ctx context.Context, pdf []byte, validator PDFSignatureValidator, qualified int) (status, source, detail string) {
	if !provenance.CarriesPAdESSignature(pdf) {
		return provenance.CheckNotAvailable, "", ""
	}
	if validator == nil {
		return provenance.CheckIndeterminate, pdfSignatureSourceDSS, "no EU DSS validation service is configured (DSS_URL)"
	}
	reports, err := validator.ValidatePDFSignatures(ctx, pdf)
	if err != nil {
		return provenance.CheckIndeterminate, pdfSignatureSourceDSS, err.Error()
	}
	status = provenance.CheckValid
	details := make([]string, 0, len(reports))
	for i, report := range reports {
		indication := report.Indication
		if report.SubIndication != "" {
			indication += "/" + report.SubIndication
		}
		details = append(details, fmt.Sprintf("signature %d by %q: %s", i+1, report.SignedBy, indication))
		if report.AssertValidAES() != nil {
			status = provenance.CheckInvalid
		}
	}
	if err := dss.AssertQualifiedSignatures(reports, qualified); err != nil {
		status = provenance.CheckInvalid
		details = append(details, err.Error())
	}
	return status, pdfSignatureSourceDSS, strings.Join(details, "; ")
}

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
		C2paSignerStatus:    provenance.CheckInvalid,
		VcProofStatus:       provenance.CheckInvalid,
		StatusListCheck:     "failed",
		StatusListError:     ptrToString("stored artifact is not authentic"),
		LifecycleStatus:     ptrToString(lifecycleStatus),
		PdfSignatureStatus:  provenance.CheckInvalid,
		Discrepancy:         ptrToString(discrepancyNotAuthentic),
	}
}

func runVerify(ctx context.Context, pdfBytes []byte, pdfCore *pdfcore.Client,
	credentials *provenance.CredentialVerifier, statuses *provenance.CredentialStatusVerifier,
	signatures PDFSignatureValidator, anchors SignerAnchors, lifecycleStatus string, qualified int) (*pdfgen.PDFVerifyResult, error) {
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
	// The banner is the lifecycle state the verified credential asserts; the
	// contract's own state stands in when there is no verified credential.
	if vcProofStatus == provenance.CheckValid {
		if asserted := assertedLifecycleStatus(result.VCBytes); asserted != "" {
			lifecycleStatus = asserted
		}
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
		_, present, err := provenance.ExtractCredentialStatus(result.VCBytes)
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
			queryStatus := func() (string, error) {
				return statuses.State(ctx, result.VCBytes)
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

	pdfSignatureStatus, pdfSignatureSource, pdfSignatureDetail := checkPDFSignatures(ctx, pdfBytes, signatures, qualified)
	signerStatus, signerDetail := provenance.CheckNotAvailable, ""
	if verifyErr == nil {
		signerStatus, signerDetail = anchorSigners(result.Signers, anchors)
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
		C2paSignerStatus:    signerStatus,
		C2paSignerDetail:    ptrToString(signerDetail),
		VcProofStatus:       vcProofStatus,
		StatusListURI:       ptrToString(statusListURI),
		StatusListStatus:    ptrToString(statusListStatus),
		StatusListCheck:     statusListCheck,
		StatusListError:     ptrToString(statusListError),
		LifecycleStatus:     ptrToString(lifecycleStatus),
		PdfSignatureStatus:  pdfSignatureStatus,
		PdfSignatureSource:  ptrToString(pdfSignatureSource),
		PdfSignatureDetail:  ptrToString(pdfSignatureDetail),
		Discrepancy:         ptrToString(discrepancy),
	}, nil
}

// assertedLifecycleStatus is the credentialSubject.status of a lifecycle
// credential, or "".
func assertedLifecycleStatus(vc []byte) string {
	var credential struct {
		CredentialSubject struct {
			Status string `json:"status"`
		} `json:"credentialSubject"`
	}
	if json.Unmarshal(vc, &credential) != nil {
		return ""
	}
	return strings.TrimSpace(credential.CredentialSubject.Status)
}

func evaluateLiveStatusCheck(
	ctx context.Context,
	lifecycleStatus string,
	query func() (string, error),
) (status, check, failure string, passed bool) {
	state, err := queryStatusWithPublicationBarrier(ctx, lifecycleStatus, query)
	if err != nil {
		// The error itself, not a cause invented for it: it distinguishes a
		// service that never answered from a list that was served and could not
		// be used, and those are looked into in different places.
		return "unavailable", "failed", err.Error(), false
	}
	// "passed" says the lookup ran and the list it read established the state:
	// the list's own signature verified against the configured trust anchors,
	// and its leaf identified the issuer it names (ADR-34 §3). Before that it
	// said only that a URL had answered, and the value had to carry the
	// qualification with it.
	return state, "passed", "", true
}

func queryStatusWithPublicationBarrier(
	ctx context.Context,
	lifecycleStatus string,
	query func() (string, error),
) (string, error) {
	status, err := query()
	if err != nil || status != "active" ||
		(lifecycleStatus != "terminated" && lifecycleStatus != "suspended" && lifecycleStatus != "expired") {
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
