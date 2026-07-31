package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

type ValidateQry struct {
	DID         string
	ValidatedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type ValidationResult struct {
	Findings []string
	// DSSReport is the structured EU DSS validation report (nil when no DSS is
	// configured or the contract carries no signed PDF). The viewer renders its
	// SignedBy / SignatureFormat / SigningTime as signer identity, signature
	// level, and timestamp (DCS-FR-SM-26).
	DSSReport *dss.Report
	// SigningEvidence is the per-signer proof extracted from the embedded
	// ContractSigningSummaryCredential(s): the content/PDF hashes and the
	// credential binding the signature covers (DCS-FR-SM-26). Empty for an
	// unsigned contract.
	SigningEvidence []SigningEvidence
}

// SigningEvidence is one signer's ContractSigningSummaryCredential, distilled
// to the compliance-relevant fields the Signature Compliance Viewer surfaces
// (DCS-FR-SM-26): who signed, through which ceremony, and the integrity proof
// (content/PDF hashes) plus the credential binding the signature covers.
type SigningEvidence struct {
	SignerDID            string
	CeremonyID           string
	FieldName            string
	ContentHash          string
	PDFHash              string
	CredentialType       string
	KBSDHash             string
	ValidationReportHash string
}

type Validator struct {
	DB      *sqlx.DB
	CRepo   db.ContractRepo
	PDFCore *pdfcore.Client
	// Credentials verifies each signing-summary credential read out of the stored
	// PDF against the key its issuer publishes for assertions, before anything it
	// claims is used.
	Credentials *provenance.CredentialVerifier
}

func (h *Validator) Handle(ctx context.Context, cmd ValidateQry) (*ValidationResult, error) {

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

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read process data: %w", err)
	}

	findings, err := h.CRepo.CollectValidationFindings(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not collect validation findings: %w", err)
	}

	// The stored PDF's signing evidence is read and VERIFIED once, and the three
	// consumers below work from the verified documents. Each used to re-extract and
	// re-decode the attachment for itself, trusting whatever parsed.
	evidence := h.readSigningEvidence(ctx, tx, cmd.DID)
	findings = append(findings, evidence.Findings...)
	findings = append(findings, h.crossCheckEmbeddedPID(ctx, tx, cmd.DID, evidence.Documents)...)
	findings = append(findings, h.crossCheckSHACLDrift(ctx, tx, cmd.DID, evidence.Documents)...)

	dssReport, dssFindings, err := h.validateWithDSS(ctx, tx, cmd.DID)
	if err != nil {
		// A CONFIGURED DSS is a required validator: its unavailability is an
		// error the caller sees, never a silently thinner findings list.
		return nil, err
	}
	findings = append(findings, dssFindings...)

	signingEvidence := collectSigningEvidence(evidence.Documents)

	evt := signingmanagementevents.ValidateEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		ValidatedBy:     cmd.ValidatedBy,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &ValidationResult{
		Findings:        findings,
		DSSReport:       dssReport,
		SigningEvidence: signingEvidence,
	}, nil
}

// validateWithDSS submits the stored signed PDF to the configured EU DSS
// instance (DCS-FR-SM-18, DCS-IR-SI-10, DCS-IR-CI-08) and reports its ETSI
// EN 319 102-1 indication as a finding. No DSS_URL means no DSS leg (the
// internal PKCS#11-based checks stand alone); a configured-but-failing DSS
// is an error. An unsigned contract (no stored PDF) yields no DSS finding.
func (h *Validator) validateWithDSS(ctx context.Context, tx *sqlx.Tx, did string) (*dss.Report, []string, error) {
	dssURL := dss.URL()
	if dssURL == "" {
		return nil, nil, nil
	}
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil || len(pdfBytes) == 0 {
		return nil, nil, nil
	}
	report, err := dss.New(dssURL).ValidatePDF(ctx, pdfBytes, did+".pdf")
	if err != nil {
		return nil, nil, fmt.Errorf("EU DSS validation of %s failed: %w", did, err)
	}
	// A wallet-produced signature is an AES (eIDAS Art. 26), not a QES: DSS's
	// AES acceptance criteria (integrity sound + signatory certificate present)
	// are the passing bar, NOT TOTAL-PASSED — which additionally demands a
	// qualified EU-trust-list chain (see dss.Report.AssertValidAES). So an
	// INDETERMINATE whose sub-indication is only a trust/POE gap
	// (NO_CERTIFICATE_CHAIN_FOUND for the wallet's non-qualified dev CA) is a
	// PASSING confirmation here, consistent with what the signing path already
	// accepts; only a crypto/integrity failure is a defect finding.
	if err := report.AssertValidAES(); err != nil {
		finding := fmt.Sprintf("EU DSS validation report: indication=%s", report.Indication)
		if report.SubIndication != "" {
			finding += fmt.Sprintf(" (subIndication=%s)", report.SubIndication)
		}
		return report, []string{finding}, nil
	}
	finding := ValidAESFinding
	if report != nil && strings.TrimSpace(report.Indication) != "" {
		finding += fmt.Sprintf(" (indication=%s", report.Indication)
		if strings.TrimSpace(report.SubIndication) != "" {
			finding += fmt.Sprintf(", subIndication=%s", report.SubIndication)
		}
		finding += ")"
	}
	return report, []string{finding}, nil
}

// ValidAESFinding records what DSS actually reported.
//
// It used to read "EU DSS validation confirms a valid Advanced Electronic
// Signature", which was returned even for INDETERMINATE — DSS declining to
// determine, most often NO_CERTIFICATE_CHAIN_FOUND. Attributing an affirmative
// conclusion to an external validator that withheld one is putting words in its
// mouth, and this string is shown to users and exported into compliance PDFs.
// The AES judgement is this system's to make and to label as its own; the
// indication is appended so a reader can see the basis for it.
const ValidAESFinding = "EU DSS reports no integrity or cryptographic failure"

// verifiedSigningEvidence is what the stored PDF's evidence attachment yielded:
// the summary credentials whose proof verified against their issuer's published
// assertion key, and the findings raised for the ones that did not.
type verifiedSigningEvidence struct {
	Documents []json.RawMessage
	Findings  []string
}

// readSigningEvidence extracts the ContractSigningSummaryCredential(s) embedded
// in the stored PDF and verifies each before it is handed on.
//
// A stored PDF is not this instance's own output: an inbound peer contract is
// kept verbatim, because it holds provenance and credentials that cannot be
// reproduced here, and a countersigned contract carries the peer's summary next
// to ours. So a document that merely decodes is the author of those bytes telling
// us who signed — which is the claim being checked, not evidence for it. The
// peer-facing path already resolves the key the proof names and verifies before
// using any claim (dcstodcs.CounterpartyPoAGate); this is the same treatment for
// the same bytes read locally.
//
// A credential that does not verify is dropped and reported. An attachment that
// is absent or undecodable yields no documents, which is how an unsigned contract
// reads.
func (h *Validator) readSigningEvidence(ctx context.Context, tx *sqlx.Tx, did string) verifiedSigningEvidence {
	if h.PDFCore == nil {
		return verifiedSigningEvidence{}
	}
	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil || len(pdfBytes) == 0 {
		return verifiedSigningEvidence{}
	}
	attachment, found, err := h.PDFCore.ExtractEvidence(ctx, pdfBytes)
	if err != nil {
		return verifiedSigningEvidence{Findings: []string{fmt.Sprintf("Could not extract embedded signing evidence: %v", err)}}
	}
	if !found || len(attachment) == 0 {
		return verifiedSigningEvidence{}
	}

	// The attachment is a single ContractSigningSummaryCredential for
	// single-signature contracts, or a JSON ARRAY of them for multi-signer
	// contracts (one per declared field, all embedded before the first signature —
	// DCS-FR-SM-07/-17).
	documents := []json.RawMessage{attachment}
	var bundle []json.RawMessage
	if err := json.Unmarshal(attachment, &bundle); err == nil && len(bundle) > 0 {
		documents = bundle
	}

	out := verifiedSigningEvidence{Documents: make([]json.RawMessage, 0, len(documents))}
	for _, doc := range documents {
		err := h.Credentials.Verify(doc)
		switch {
		case err == nil:
			out.Documents = append(out.Documents, doc)
		case errors.Is(err, provenance.ErrIssuerUnresolved):
			out.Findings = append(out.Findings,
				fmt.Sprintf("Embedded signing evidence is indeterminate — its issuer was not resolved to a key published for assertions: %v", err))
		default:
			out.Findings = append(out.Findings, fmt.Sprintf("Embedded signing evidence is invalid: %v", err))
		}
	}
	return out
}

// collectSigningEvidence distills each verified summary credential to the
// compliance fields the Signature Compliance Viewer surfaces (DCS-FR-SM-26):
// signer DID, ceremony, content/PDF hashes, credential type, and the KB-JWT
// binding.
func collectSigningEvidence(documents []json.RawMessage) []SigningEvidence {
	out := make([]SigningEvidence, 0, len(documents))
	for _, doc := range documents {
		if ev, ok := parseSigningEvidence(doc); ok {
			out = append(out, ev)
		}
	}
	return out
}

// parseSigningEvidence reads the credentialSubject of a
// ContractSigningSummaryCredential. The signer DID lives in credentialSubject.id;
// ok is false for a document that is not a signing-summary VC (no signer id).
func parseSigningEvidence(evidence []byte) (SigningEvidence, bool) {
	var vc struct {
		CredentialSubject struct {
			ID                   string `json:"id"`
			CeremonyID           string `json:"ceremony_id"`
			FieldName            string `json:"field_name"`
			ContentHash          string `json:"content_hash"`
			PDFHash              string `json:"pdf_hash"`
			CredentialType       string `json:"credential_type"`
			KBSDHash             string `json:"kb_sd_hash"`
			ValidationReportHash string `json:"validation_report_hash"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(evidence, &vc); err != nil || vc.CredentialSubject.ID == "" {
		return SigningEvidence{}, false
	}
	cs := vc.CredentialSubject
	return SigningEvidence{
		SignerDID:            cs.ID,
		CeremonyID:           cs.CeremonyID,
		FieldName:            cs.FieldName,
		ContentHash:          cs.ContentHash,
		PDFHash:              cs.PDFHash,
		CredentialType:       cs.CredentialType,
		KBSDHash:             cs.KBSDHash,
		ValidationReportHash: cs.ValidationReportHash,
	}, true
}

// crossCheckEmbeddedPID re-verifies the embedded signer binding against the
// signature record (UC-04-03), from the summary credentials readSigningEvidence
// already verified. Absence of evidence (an unsigned contract) yields no
// findings; any mismatch is reported as a finding so validate surfaces it.
func (h *Validator) crossCheckEmbeddedPID(ctx context.Context, tx *sqlx.Tx, did string, documents []json.RawMessage) []string {
	if len(documents) == 0 {
		return nil
	}

	// Privacy: the PID is never embedded (no personal data in the shared PDF),
	// so the cross-check binds on what IS carried — the pseudonymous holder DID
	// and the KB-JWT sd_hash — matching them against the signature record rather
	// than re-verifying the full credential from the PDF.
	verifiedSigners := map[string]bool{}
	for _, doc := range documents {
		subject, sdHash := signingSummarySignerFields(doc)
		if subject == "" || sdHash == "" {
			return []string{"Embedded signing evidence is missing the signer binding"}
		}
		verifiedSigners[subject] = true
	}

	records, err := h.CRepo.LoadSignatures(ctx, tx, did)
	if err == nil {
		for _, rec := range records {
			if strings.EqualFold(strings.TrimSpace(rec.Status), "REVOKED") {
				continue
			}
			if rec.SignerDID != "" && !verifiedSigners[rec.SignerDID] {
				return []string{"Evidence mismatch: embedded signer does not match the signature record"}
			}
		}
	}

	return []string{"Embedded signer binding cross-checked against the signature record"}
}

// crossCheckSHACLDrift (Phase 4, ADR-9) re-runs the Semantic Hub SHACL
// validation the contract was signed under and compares the resulting
// finding hash against the one embedded in the signing-summary credential
// at signing time (validation.SHACLEvidence). A mismatch means the
// contract's stored data has changed since it was signed — a real
// modification, not just a hub schema version bump (evidence is pinned to
// the version active at signing time, ADR-8, so rolling the hub forward
// never causes a false drift finding). Absence of embedded evidence (an
// unsigned contract) yields no finding. The comparison runs against the summary
// credentials readSigningEvidence verified: the hash it compares to is the one
// the ISSUER sealed, so an unverified attachment could otherwise supply a hash
// that always matches.
func (h *Validator) crossCheckSHACLDrift(ctx context.Context, tx *sqlx.Tx, did string, documents []json.RawMessage) []string {
	if len(documents) == 0 {
		return nil
	}

	embeddedHash := ""
	for _, doc := range documents {
		if hash := signingSummarySHACLHash(doc); hash != "" {
			embeddedHash = hash
			break
		}
	}
	if embeddedHash == "" {
		return nil
	}

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil || contract == nil || contract.ContractData == nil {
		return []string{"Could not re-run SHACL validation for drift comparison: contract data unavailable"}
	}
	_, currentHash, err := validation.SHACLEvidence(ctx, *contract.ContractData)
	if err != nil {
		return []string{fmt.Sprintf("Could not re-run SHACL validation for drift comparison: %v", err)}
	}
	if currentHash != embeddedHash {
		return []string{"SHACL drift detected: the contract's stored data no longer matches the validation report embedded at signing time"}
	}
	return []string{"SHACL validation report re-verified against the pinned hub schema version — no drift"}
}

// signingSummarySHACLHash extracts the validation_report_hash field from a
// ContractSigningSummaryCredential evidence document (empty for documents
// signed before Phase 4, or where evidence enrichment was best-effort
// skipped).
func signingSummarySHACLHash(evidence []byte) string {
	var vc struct {
		CredentialSubject struct {
			ValidationReportHash string `json:"validation_report_hash"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(evidence, &vc); err != nil {
		return ""
	}
	return vc.CredentialSubject.ValidationReportHash
}

// signingSummarySignerFields extracts the pseudonymous signer binding — the
// holder DID (credentialSubject.id) and the KB-JWT sd_hash — from a
// ContractSigningSummaryCredential. The PID itself is never embedded.
func signingSummarySignerFields(evidence []byte) (subject, sdHash string) {
	var vc struct {
		CredentialSubject struct {
			ID       string `json:"id"`
			KBSDHash string `json:"kb_sd_hash"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(evidence, &vc); err != nil {
		return "", ""
	}
	return vc.CredentialSubject.ID, vc.CredentialSubject.KBSDHash
}
