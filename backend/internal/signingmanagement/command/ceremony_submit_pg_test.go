package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

// SM-2 (DCS-FR-SM-03, DCS-FR-SM-20): submit accepts the signature only within
// signedCeremonyTTL of the presentation, and only while the PoA and the PID
// the signatory presented are neither revoked nor suspended and their status
// can be read.
func TestSubmitRefusesAStalePresentationOrARevokedCredential(t *testing.T) {
	database, repo, ceremony := verifiedBindingCeremony(t)
	_, err := database.Exec(`UPDATE signature_ceremonies SET prepared_pdf = 'pdf', pinned_content_hash = 'h', pinned_renderer_version = 'r',
		pinned_signed_count = 0, pinned_contract_version = 1, required_credential_type = 'AES', poa_vp_token = 'poa' WHERE id = $1`, ceremony.ID)
	require.NoError(t, err)
	cmd := SubmitSignatureCmd{SignedPDF: []byte("signed"), ApplyCmd: ApplyCmd{
		DID: bindingContractDID, SignerDID: bindingSigner, AppliedBy: bindingOrg, HolderDID: bindingHolder, CeremonyID: ceremony.ID,
	}}
	reader := func(state status.State, err error) CredentialStateReader {
		return func(context.Context, string) (status.State, error) { return state, err }
	}
	submit := func(poa, pid CredentialStateReader) error {
		return (&Applier{
			DB: database, CeremonyRepo: repo, Validator: unusedValidator{}, PDFCore: &pdfcore.Client{}, CredentialState: poa, PIDState: pid,
		}).SubmitWalletSignature(context.Background(), cmd)
	}

	err = submit(reader(status.StateInvalid, nil), reader(status.StateValid, nil))
	require.ErrorIs(t, err, ErrCeremonyRequired)
	require.ErrorContains(t, err, "Power of Attorney")
	err = submit(reader(status.StateValid, nil), reader(status.StateSuspended, nil))
	require.ErrorIs(t, err, ErrCeremonyRequired)
	require.ErrorContains(t, err, "PID")
	err = submit(reader("", errors.New("status list unreachable")), reader(status.StateValid, nil))
	require.ErrorIs(t, err, ErrCeremonyRequired)

	_, err = database.Exec(`UPDATE signature_ceremonies SET verified_at = now() - $2::interval WHERE id = $1`,
		ceremony.ID, (signedCeremonyTTL + time.Minute).String())
	require.NoError(t, err)
	err = submit(nil, nil)
	require.ErrorIs(t, err, ErrCeremonyRequired)
	require.ErrorContains(t, err, "older than")
}
