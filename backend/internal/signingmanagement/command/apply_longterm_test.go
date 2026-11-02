package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/dss"
)

// stubExtender answers ExtendPDF and ValidatePDFSignatures with fixed values
// and records the calls.
type stubExtender struct {
	extended  []byte
	extendErr error
	reports   []*dss.Report
	levels    []string
}

func (s *stubExtender) ExtendPDF(_ context.Context, _ []byte, level string) ([]byte, error) {
	s.levels = append(s.levels, level)
	return s.extended, s.extendErr
}

func (s *stubExtender) ValidatePDFSignatures(context.Context, []byte) ([]*dss.Report, error) {
	return s.reports, nil
}

var (
	ltaSignedPDF   = []byte("%PDF prepared + signature")
	ltaExtendedPDF = []byte("%PDF prepared + signature + DSS + DocTimeStamp")
	ltaQES         = &dss.Report{Indication: "TOTAL-PASSED", SignedBy: "CN=A", Qualification: "QESig", SignatureFormat: "PAdES-BASELINE-LTA"}
	ltaAES         = &dss.Report{Indication: "INDETERMINATE", SubIndication: "NO_CERTIFICATE_CHAIN_FOUND", SignedBy: "CN=B", SignatureFormat: "PAdES-BASELINE-LTA"}
)

func signedAtLevel(field, level string) db.SignatureRecord {
	return db.SignatureRecord{Status: "SIGNED", CredentialType: level, FieldName: &field}
}

// DCS-FR-SM-02, DCS-FR-CSA-08 (ADR-40): the signature that completes the local
// signatures of a contract that carries a QES stores the PDF that DSS
// extended to PAdES-B-LTA.
func TestArchivePDFExtendsACompletedQESContractToLTA(t *testing.T) {
	for name, tc := range map[string]struct {
		level    string
		existing []db.SignatureRecord
		local    []string
		field    string
		reports  []*dss.Report
	}{
		"this signature is a QES": {level: "QES", local: []string{"Org A"}, field: "Org A", reports: []*dss.Report{ltaQES}},
		"an earlier signature is QES": {level: "AES", existing: []db.SignatureRecord{signedAtLevel("Org A", "QES")},
			local: []string{"Org A", "Org B"}, field: "Org B", reports: []*dss.Report{ltaQES, ltaAES}},
	} {
		t.Run(name, func(t *testing.T) {
			extender := &stubExtender{extended: ltaExtendedPDF, reports: tc.reports}

			stored, err := (&Applier{Extender: extender}).archivePDF(context.Background(), ltaSignedPDF, tc.level, tc.existing, tc.local, tc.field)

			require.NoError(t, err)
			require.Equal(t, ltaExtendedPDF, stored)
			require.Equal(t, []string{"PAdES_BASELINE_LTA"}, extender.levels)
		})
	}
}

// ADR-40 applies to QES only, and only to the signature that completes the
// local signatures: an AES contract and an intermediate signature keep the
// submitted PDF, and DSS is not asked.
func TestArchivePDFKeepsTheSubmittedPDFWithoutACompletedQES(t *testing.T) {
	for name, tc := range map[string]struct {
		level    string
		existing []db.SignatureRecord
		local    []string
		field    string
	}{
		"AES contract":                 {level: "AES", existing: []db.SignatureRecord{signedAtLevel("Org A", "AES")}, local: []string{"Org A", "Org B"}, field: "Org B"},
		"QES, another field unsigned":  {level: "QES", local: []string{"Org A", "Org B"}, field: "Org A"},
		"AES contract, no field":       {level: "AES", field: ""},
		"QES record revoked, AES here": {level: "AES", existing: []db.SignatureRecord{{Status: "REVOKED", CredentialType: "QES"}}, field: ""},
	} {
		t.Run(name, func(t *testing.T) {
			extender := &stubExtender{extended: ltaExtendedPDF, reports: []*dss.Report{ltaQES}}

			stored, err := (&Applier{Extender: extender}).archivePDF(context.Background(), ltaSignedPDF, tc.level, tc.existing, tc.local, tc.field)

			require.NoError(t, err)
			require.Equal(t, ltaSignedPDF, stored)
			require.Empty(t, extender.levels, "DSS was asked to extend the PDF")
		})
	}
}

// Every failure of the extension is the retryable
// ErrLongTermValidationUnavailable, never a stored PDF without long-term
// validation data.
func TestArchivePDFRefusesAnExtensionThatFailsTheChecks(t *testing.T) {
	notLTA := *ltaQES
	notLTA.SignatureFormat = "PAdES-BASELINE-T"
	notQES := *ltaQES
	notQES.Indication, notQES.SubIndication = "INDETERMINATE", "TRY_LATER"

	for name, extender := range map[string]*stubExtender{
		"DSS or TSA unavailable":          {extendErr: errors.New("dss: extension returned HTTP 500: no TSP source")},
		"result rewrites the signed PDF":  {extended: []byte("%PDF rewritten"), reports: []*dss.Report{ltaQES}},
		"result adds nothing":             {extended: ltaSignedPDF, reports: []*dss.Report{ltaQES}},
		"result is not LTA":               {extended: ltaExtendedPDF, reports: []*dss.Report{&notLTA}},
		"result does not validate as QES": {extended: ltaExtendedPDF, reports: []*dss.Report{&notQES}},
		"result names no signature":       {extended: ltaExtendedPDF},
	} {
		t.Run(name, func(t *testing.T) {
			stored, err := (&Applier{Extender: extender}).archivePDF(context.Background(), ltaSignedPDF, "QES", nil, nil, "")

			require.ErrorIs(t, err, ErrLongTermValidationUnavailable)
			require.Nil(t, stored)
		})
	}
}

// Without a DSS no QES can be accepted at all; a missing extender is a
// configuration fault, not a condition that clears on retry.
func TestArchivePDFRequiresAnExtenderForAQES(t *testing.T) {
	_, err := (&Applier{}).archivePDF(context.Background(), ltaSignedPDF, "QES", nil, nil, "")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrLongTermValidationUnavailable)
}
