// Package dss integrates the EU Digital Signature Service (DSS, the
// eSignature building block's validation stack) as an ADDITIONAL, external
// AdES validator alongside the internal PKCS#11-based checks
// (DCS-FR-SM-18, DCS-IR-SI-10, DCS-IR-CI-08). The DSS demonstration webapp
// exposes REST validation under /services/rest/validation; when DSS_URL is
// configured the signature validator submits the signed PDF there and
// reports the returned indication. A configured-but-unreachable DSS is an
// ERROR, never silently skipped (required external dependencies hard-fail).
//
// Deployment note: the EU distributes the demo webapp as a ZIP/WAR, not as
// an official container image — deployment/helm/charts/dss wraps a pinned
// community image and stays DISABLED by default; enabling it is an operator
// decision (dss.enabled + DSS_URL).
package dss

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// URL returns the configured DSS endpoint ("" = DSS validation disabled).
func URL() string {
	return strings.TrimSpace(os.Getenv("DSS_URL"))
}

// Client submits signed documents to a DSS instance's REST validation API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New returns a Client for the given DSS base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Report is the distilled outcome of a DSS validation call.
type Report struct {
	// Indication is ETSI EN 319 102-1's main status indication
	// (e.g. TOTAL-PASSED, INDETERMINATE, TOTAL-FAILED).
	Indication string
	// SubIndication qualifies non-passed indications
	// (e.g. NO_CERTIFICATE_CHAIN_FOUND for a signer CA outside the EU LOTL).
	SubIndication string
	// SignedBy is the readable subject of the signing certificate the wallet
	// used (DSS simpleReport SignedBy). It is the sole-control evidence: the
	// DCS asserts this identifies the SIGNATORY, proving the signature was
	// produced with the signatory's own key — never a DCS key (eIDAS Art. 26c,
	// DCS-FR-SM-16 "secure key usage ... integrity validation upon signing").
	SignedBy string
	// SignatureFormat is the AdES format+level DSS recognized
	// (e.g. PAdES-BASELINE-B), the level evidence for DCS-FR-SM-01/-02.
	SignatureFormat string
	// SigningTime is the claimed/qualified signing time (DCS-FR-SM-18 timestamp
	// verification).
	SigningTime string
	// Qualification is DSS's qualification determination for the signature
	// (e.g. QESIG for a qualified electronic signature by a natural person,
	// ADESIG_QC/ADESIG for an advanced signature). It is the QES evidence
	// AssertValidQES requires TOTAL-PASSED to be paired with — AssertValidAES
	// never looks at it (ADR-20).
	Qualification string
	// SerialNumber is the signing certificate's serial number (sole control
	// audit trail, DCS-FR-SM-26).
	SerialNumber string
	// GivenName/Surname/CommonName are the signing certificate subject's name
	// attributes, when DSS reports them as structured fields. When DSS reports
	// only SignedBy as a DN string, ParseSubjectAttributes recovers the same
	// attributes from it (cert-subject to PID name matching, ADR-20).
	GivenName  string
	Surname    string
	CommonName string
}

// Passed reports whether the ETSI indication is TOTAL-PASSED.
func (r *Report) Passed() bool {
	return strings.EqualFold(r.Indication, "TOTAL-PASSED")
}

// cryptoFailureSubIndications are the ETSI EN 319 102-1 sub-indications that mean
// the signature itself is broken — bad crypto, a mismatched hash, a malformed
// container, or no signed data — as opposed to an incomplete trust chain or POE.
var cryptoFailureSubIndications = map[string]bool{
	"SIG_CRYPTO_FAILURE":    true,
	"HASH_FAILURE":          true,
	"FORMAT_FAILURE":        true,
	"SIGNED_DATA_NOT_FOUND": true,
}

// AssertValidAES enforces the DCS's acceptance criteria for a wallet-produced
// Advanced Electronic Signature (eIDAS Art. 26, DCS-FR-SM-16/-18): the signature
// is cryptographically sound, a signing certificate is present, and — the
// sole-control proof — that certificate identifies the ceremony's signatory.
//
// It deliberately does NOT require DSS's TOTAL-PASSED. TOTAL-PASSED additionally
// asserts the signing certificate chains to a QUALIFIED EU trust-list CA, which
// is a QES property; AES needs only integrity and unique linkage to the
// signatory (Art. 26 a/b/d). So an INDETERMINATE result whose sub-indication is
// a trust/POE gap (e.g. NO_CERTIFICATE_CHAIN_FOUND for a non-qualified CA) is
// accepted, while a TOTAL-FAILED or any crypto/integrity failure is rejected.
//
// AES (eIDAS Art. 26) requires a cryptographically sound signature over the
// document by a signatory's certificate; it does NOT require the certificate to
// carry any wallet-PID identifier — no such binding is standardised (the EUDI
// reference QTSP only copies PID name attributes into the subject at enrolment).
// The signatory's identity is established by the ceremony's verified PID and
// recorded there; here we assert only that the signature is a valid AES.
func (r *Report) AssertValidAES() error {
	if strings.EqualFold(r.Indication, "TOTAL-FAILED") || cryptoFailureSubIndications[strings.ToUpper(strings.TrimSpace(r.SubIndication))] {
		return fmt.Errorf("dss: signature failed validation: indication %s / %s", r.Indication, r.SubIndication)
	}
	if strings.TrimSpace(r.SignedBy) == "" {
		return fmt.Errorf("dss: signature carries no signing certificate")
	}
	return nil
}

// qualifiedSignatureQualifications are DSS's SignatureQualification values that
// mean "qualified electronic signature by a natural person" (ETSI TS 119
// 172-4 QESig) — a qualified certificate on a QSCD, chained to an EU
// trusted-list CA.
var qualifiedSignatureQualifications = map[string]bool{
	"QESIG": true,
}

// AssertValidQES enforces the DCS's acceptance criteria for a Qualified
// Electronic Signature (eIDAS Art. 3(12), Annex I): everything AssertValidAES
// requires, PLUS DSS's TOTAL-PASSED indication (a qualified cert chaining to a
// trusted-list CA IS required for QES, unlike AES) and a QESig qualification
// determination (qualified certificate + QSCD, ETSI TS 119 172-4). Unlike
// AssertValidAES, an INDETERMINATE/NO_CERTIFICATE_CHAIN_FOUND result is
// REJECTED here: a trust-chain gap disqualifies a QES claim even though it is
// tolerated for AES (ADR-20).
func (r *Report) AssertValidQES() error {
	if err := r.AssertValidAES(); err != nil {
		return err
	}
	if !r.Passed() {
		return fmt.Errorf("dss: QES requires indication TOTAL-PASSED (a qualified cert chaining to a trusted-list CA), got %s/%s", r.Indication, r.SubIndication)
	}
	if !qualifiedSignatureQualifications[strings.ToUpper(strings.TrimSpace(r.Qualification))] {
		return fmt.Errorf("dss: QES requires a QESig qualification determination (qualified certificate + QSCD), got %q", r.Qualification)
	}
	return nil
}

// AssertMeetsLevel enforces the acceptance gate for the required signature
// level (SM-01 per-contract level enforcement, ADR-20): AssertValidQES for a
// contract that requires QES, AssertValidAES (the permissive gate, unchanged)
// for everything else. Applying the wrong gate to a level the contract does
// not require would either reject a legitimate AES signature against QES
// criteria it was never meant to satisfy, or accept a QES-required signature
// on AES's relaxed trust-chain tolerance — this is the single dispatch point
// that keeps the two gates from being applied to the wrong ceremony.
func (r *Report) AssertMeetsLevel(required string) error {
	if strings.EqualFold(strings.TrimSpace(required), "QES") {
		return r.AssertValidQES()
	}
	return r.AssertValidAES()
}

// ParseSubjectAttributes extracts GIVENNAME/SURNAME/CN/SERIALNUMBER RDN
// attributes from a certificate subject distinguished name string (DSS's
// SignedBy, e.g. "CN=Jane Doe, SURNAME=Doe, GIVENNAME=Jane"). Used as a
// fallback when DSS does not report GivenName/Surname as structured fields.
func ParseSubjectAttributes(dn string) map[string]string {
	attrs := map[string]string{}
	for _, part := range strings.FieldsFunc(dn, func(r rune) bool { return r == ',' || r == '/' }) {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		attrs[key] = value
	}
	return attrs
}

// SubjectGivenName returns the signing certificate's given name, preferring
// DSS's structured GivenName field and falling back to parsing SignedBy.
func (r *Report) SubjectGivenName() string {
	if strings.TrimSpace(r.GivenName) != "" {
		return r.GivenName
	}
	return ParseSubjectAttributes(r.SignedBy)["GIVENNAME"]
}

// SubjectSurname returns the signing certificate's surname, preferring DSS's
// structured Surname field and falling back to parsing SignedBy.
func (r *Report) SubjectSurname() string {
	if strings.TrimSpace(r.Surname) != "" {
		return r.Surname
	}
	return ParseSubjectAttributes(r.SignedBy)["SURNAME"]
}

// ValidatePDF submits pdf to POST {base}/services/rest/validation/validateSignature
// and returns the simple report's indication. Any transport or protocol
// failure is an error — the caller treats a configured DSS as required.
func (c *Client) ValidatePDF(ctx context.Context, pdf []byte, name string) (*Report, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("dss: no base URL configured")
	}
	body, err := json.Marshal(map[string]any{
		"signedDocument": map[string]any{
			"bytes": base64.StdEncoding.EncodeToString(pdf),
			"name":  name,
		},
		"originalDocuments": []any{},
		"policy":            nil,
		"signatureId":       nil,
	})
	if err != nil {
		return nil, err
	}
	// The DSS demo container restarts under CI resource pressure; a request
	// issued during a restart hits connection-refused/EOF. Retry the transport
	// across a restart window before failing — a configured DSS stays required,
	// this just waits for it to come back rather than dropping the check.
	var resp *http.Response
	deadline := time.Now().Add(90 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/services/rest/validation/validateSignature", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err = c.httpClient.Do(req)
		if err == nil {
			break
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return nil, fmt.Errorf("dss: validation request failed: %w", err)
		}
		time.Sleep(3 * time.Second)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dss: read validation response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dss: validation returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	report, err := parseReport(respBody)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// parseReport extracts the Indication/SubIndication pair — and everything
// else the Report carries — from a DSS WSReportsDTO.
func parseReport(raw []byte) (*Report, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("dss: parse validation response: %w", err)
	}
	report := &Report{}
	if sig, sigID := latestSignatureEntry(doc); sig != nil {
		walkReport(sig, report)
		// GivenName/Surname/CommonName never appear under simpleReport (per
		// its XSD, a Signature entry's only certificate-shaped child is
		// CertificateChain.Certificate.QualifiedName - a display string, not
		// structured RDN fields) - they live on diagnosticData's own
		// per-certificate entries, resolved below.
		if cert := resolveSigningCertificate(doc, sigID); cert != nil {
			walkReport(cert, report)
		}
	} else {
		// Fallback for DSS response shapes that don't nest under
		// simpleReport.signatureOrTimestampOrEvidenceRecord the way the JSON
		// REST shape does (the DTO layout differs across DSS versions -
		// simpleReport signature entries vs. XML-derived attribute casing) -
		// walk the whole document generically as before.
		walkReport(doc, report)
	}
	if report.Indication == "" {
		return nil, fmt.Errorf("dss: validation response carries no Indication")
	}
	return report, nil
}

// latestSignatureEntry finds simpleReport.signatureOrTimestampOrEvidenceRecord
// and returns the "Signature" object of its LAST Signature-typed entry
// (Timestamp/EvidenceRecord entries in the same list are skipped). PAdES
// incremental updates append a new signature after any already on the
// document, and DSS reports them in that same order, so the last Signature
// entry is the one this submission is actually being validated for.
//
// A document can carry more than one signature by the time it's submitted
// (a multi-signer contract's second-and-later signatories always incrementally
// sign on top of earlier ones), and simpleReport then carries one entry per
// signature. Walking the whole response tree instead of scoping to this one
// (the previous implementation) picked up whichever entry's Indication/
// SubIndication/SignedBy/GivenName/Surname fields the walk order happened to
// visit first - for an ordered array that is deterministically the OLDEST
// signature, not the one just submitted, silently validating and attributing
// the wrong signatory's certificate (ADR-20 cert↔PID sole-control check).
// latestSignatureEntry also returns that entry's own Id (a DSS WSReportsDTO
// token id, e.g. "S-<hash>") — shared verbatim between simpleReport and
// diagnosticData for the same signature (see resolveSigningCertificate) — so
// callers can cross-reference the rest of the response for this exact
// signature. "" if the entry carries no (string) Id.
func latestSignatureEntry(doc any) (any, string) {
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, ""
	}
	simpleReportAny, ok := lookupCI(root, "simpleReport")
	if !ok {
		return nil, ""
	}
	simpleReport, ok := simpleReportAny.(map[string]any)
	if !ok {
		return nil, ""
	}
	entriesAny, ok := lookupCI(simpleReport, "signatureOrTimestampOrEvidenceRecord")
	if !ok {
		return nil, ""
	}
	entries, ok := entriesAny.([]any)
	if !ok {
		return nil, ""
	}
	var last any
	var lastID string
	for _, entry := range entries {
		obj, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		sigAny, ok := lookupCI(obj, "Signature")
		if !ok {
			continue
		}
		last = sigAny
		lastID = ""
		if sig, ok := sigAny.(map[string]any); ok {
			lastID = idString(sig)
		}
	}
	return last, lastID
}

// resolveSigningCertificate returns the diagnosticData.UsedCertificates entry
// for the certificate that produced signatureID, per DSS's WSReportsDTO
// schema (dss-diagnostic-jaxb DiagnosticData.xsd, DSS 6.2):
// diagnosticData.Signatures.Signature[] carries one entry per signature
// (Id-matched against simpleReport's own entries) whose SigningCertificate
// attribute "Certificate" is an IDREF into
// diagnosticData.UsedCertificates.Certificate[] (matched by that entry's own
// Id) — the certificate entry carrying the GivenName/Surname/CommonName RDN
// attributes simpleReport never exposes. UsedCertificates is a FLAT list of
// every certificate in the document (every signature's signing cert, CA
// certs, TSA certs) — resolving through this Id chain, rather than scanning
// that flat list for the first GivenName/Surname found, is what keeps a
// multi-signature document's certificates from being mixed up the same way
// latestSignatureEntry's scoping fixes simpleReport's own ambiguity.
func resolveSigningCertificate(doc any, signatureID string) map[string]any {
	if signatureID == "" {
		return nil
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	diagnosticDataAny, ok := lookupCI(root, "diagnosticData")
	if !ok {
		return nil
	}
	diagnosticData, ok := diagnosticDataAny.(map[string]any)
	if !ok {
		return nil
	}

	certID := ""
	if signaturesAny, ok := lookupCI(diagnosticData, "Signatures"); ok {
		for _, sig := range wrappedList(signaturesAny, "Signature") {
			if idString(sig) != signatureID {
				continue
			}
			scAny, ok := lookupCI(sig, "SigningCertificate")
			if !ok {
				break
			}
			sc, ok := scAny.(map[string]any)
			if !ok {
				break
			}
			if v, ok := lookupCI(sc, "Certificate"); ok {
				if s, ok := v.(string); ok {
					certID = s
				}
			}
			break
		}
	}
	if certID == "" {
		return nil
	}

	usedCertsAny, ok := lookupCI(diagnosticData, "UsedCertificates")
	if !ok {
		return nil
	}
	for _, cert := range wrappedList(usedCertsAny, "Certificate") {
		if idString(cert) == certID {
			return cert
		}
	}
	return nil
}

// wrappedList normalizes a JAXB list-wrapper element - e.g.
// {"Signature": [...]} or, for a single item, sometimes {"Signature": {...}}
// depending on the JSON mapper - to a slice of objects.
func wrappedList(wrapperAny any, itemKey string) []map[string]any {
	wrapper, ok := wrapperAny.(map[string]any)
	if !ok {
		return nil
	}
	itemAny, ok := lookupCI(wrapper, itemKey)
	if !ok {
		return nil
	}
	switch v := itemAny.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{v}
	default:
		return nil
	}
}

// idString reads a token's "Id" field (an XML attribute in the source XSD,
// flattened to a plain JSON key like every other attribute in this DTO).
func idString(m map[string]any) string {
	if v, ok := lookupCI(m, "Id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// lookupCI is a case-insensitive map lookup, matching walkReport's existing
// case-insensitive key handling (DSS's JSON key casing varies by version).
func lookupCI(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	lower := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return nil, false
}

// walkReport pulls the first occurrence of each distilled field from a DSS
// WSReportsDTO. The DTO layout differs across DSS versions (simpleReport
// entries vs. XML-derived attribute casing), so the search walks the JSON
// generically instead of pinning one version's schema.
func walkReport(node any, report *Report) {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			s, ok := val.(string)
			if !ok {
				continue
			}
			switch strings.ToLower(key) {
			case "indication":
				setFirst(&report.Indication, s)
			case "subindication":
				setFirst(&report.SubIndication, s)
			case "signedby":
				setFirst(&report.SignedBy, s)
			case "signatureformat":
				setFirst(&report.SignatureFormat, s)
			case "signingtime":
				setFirst(&report.SigningTime, s)
			case "signaturequalification", "qualification":
				setFirst(&report.Qualification, s)
			case "serialnumber", "certificateserialnumber":
				setFirst(&report.SerialNumber, s)
			case "givenname":
				setFirst(&report.GivenName, s)
			case "surname":
				setFirst(&report.Surname, s)
			case "commonname":
				setFirst(&report.CommonName, s)
			}
		}
		for _, val := range v {
			walkReport(val, report)
		}
	case []any:
		for _, item := range v {
			walkReport(item, report)
		}
	}
}

func setFirst(dst *string, s string) {
	if *dst == "" && s != "" {
		*dst = s
	}
}
