package command

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/signingmanagement/db/pg"
	"digital-contracting-service/internal/signingmanagement/dss"
)

// qesValidator reports every document as a valid QES by report.SignedBy.
type qesValidator struct{ report dss.Report }

func (v qesValidator) ValidatePDF(context.Context, []byte, string) (*dss.Report, error) {
	report := v.report
	return &report, nil
}

// DCS-FR-SM-02, DCS-FR-CSA-08 (ADR-40): when DSS cannot extend a QES contract
// to PAdES-B-LTA, the submission is refused as retryable and writes nothing.
// The ceremony stays unconsumed, so the wallet can send the same signature
// again, and no signature or archive entry exists without long-term
// validation data.
func TestSubmitWritesNothingWhenTheLTAExtensionFails(t *testing.T) {
	database, repo, ceremony := verifiedBindingCeremony(t)
	prepared := []byte("%PDF-1.7 prepared\n")
	signed := append(append([]byte{}, prepared...), signatureUpdate(t, "Jane", "Doe")...)
	_, err := database.Exec(`UPDATE signature_ceremonies SET prepared_pdf = $2, pinned_content_hash = 'h', pinned_renderer_version = 'r',
		pinned_signed_count = 0, pinned_contract_version = 1, required_credential_type = 'QES',
		pid_claims = '{"given_name":"Jane","family_name":"Doe"}' WHERE id = $1`, ceremony.ID, prepared)
	require.NoError(t, err)

	pdfCore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/verify/content-match":
			_, _ = w.Write([]byte(`{"match":true}`))
		case "/payload/extract":
			_, _ = w.Write([]byte(`{"@type":"dcs:Contract"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer pdfCore.Close()
	extender := &stubExtender{extendErr: errors.New("dss: extension returned HTTP 500: no TSP source")}

	err = (&Applier{
		DB: database, CRepo: &pg.PostgresContractRepo{}, CeremonyRepo: repo, PDFCore: pdfcore.New(pdfCore.URL, nil),
		Validator: qesValidator{dss.Report{Indication: "TOTAL-PASSED", Qualification: "QESig", SignedBy: "CN=Jane Doe"}},
		Extender:  extender,
	}).SubmitWalletSignature(context.Background(), SubmitSignatureCmd{SignedPDF: signed, ApplyCmd: ApplyCmd{
		DID: bindingContractDID, SignerDID: bindingSigner, AppliedBy: bindingOrg, HolderDID: bindingHolder, CeremonyID: ceremony.ID,
	}})

	require.ErrorIs(t, err, ErrLongTermValidationUnavailable)
	require.Equal(t, []string{"PAdES_BASELINE_LTA"}, extender.levels)
	var consumed bool
	require.NoError(t, database.Get(&consumed, `SELECT consumed_at IS NOT NULL FROM signature_ceremonies WHERE id = $1`, ceremony.ID))
	require.False(t, consumed, "the ceremony was consumed")
	var signatures, archived int
	require.NoError(t, database.Get(&signatures, `SELECT count(*) FROM contract_signatures WHERE contract_did = $1`, bindingContractDID))
	require.NoError(t, database.Get(&archived, `SELECT count(*) FROM contract_archive_entries WHERE did = $1`, bindingContractDID))
	require.Zero(t, signatures)
	require.Zero(t, archived)
	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did = $1`, bindingContractDID))
	require.Equal(t, "APPROVED", state)
}
