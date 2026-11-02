package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/db/pg"
	"digital-contracting-service/internal/signingmanagement/query"
)

const (
	bindingContractDID = "did:example:binding"
	bindingField       = "Org Y"
	bindingSigner      = "did:jwk:victim"
	bindingOrg         = "Org Y"
	bindingHolder      = "did:jwk:victim-session"
)

// verifiedBindingCeremony starts a ceremony in the victim's session and
// completes its presentation with the victim's PID and PoA.
func verifiedBindingCeremony(t *testing.T) (*sqlx.DB, *pg.PostgresCeremonyRepo, *db.SignatureCeremony) {
	t.Helper()
	database := pgtest.Open(t)
	ctx := context.Background()
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data)
		VALUES ($1,'',$2,'APPROVED','tpl',1,'{}')`, bindingContractDID, bindingOrg)
	require.NoError(t, err)
	repo := &pg.PostgresCeremonyRepo{}
	started, err := (&StartCeremonyHandler{DB: database, CeremonyRepo: repo}).Handle(ctx, StartCeremonyCmd{
		ContractDID: bindingContractDID, FieldName: bindingField, RequestedBy: bindingOrg, RequestedHolderDID: bindingHolder,
		BaseURL: "https://dcs.example", ClientID: "x509_san_dns:dcs.example",
	})
	require.NoError(t, err)
	_, err = (&PresentationHandler{DB: database, CeremonyRepo: repo}).CompletePresentation(ctx, PresentationCmd{
		CeremonyID: started.ID, SignerDID: bindingSigner, SDHash: "sd", VpToken: "vp",
		PoAOrganization: bindingField, PoASubjectDID: bindingSigner, PoARoles: []string{"Contract Signer"},
	})
	require.NoError(t, err)
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	ceremony, err := repo.GetCeremonyByID(ctx, tx, started.ID)
	require.NoError(t, err)
	return database, repo, ceremony
}

func resolveInTx(t *testing.T, database *sqlx.DB, repo db.CeremonyRepo, cmd ApplyCmd) (*db.SignatureCeremony, error) {
	t.Helper()
	tx, err := database.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	return resolveCeremony(context.Background(), tx, repo, cmd)
}

// DCS-FR-SM-14, DCS-FR-SM-15: prepare and submit resolve only a ceremony whose
// verified signatory is the named signer, that the calling session started,
// and that is not consumed. Another signer cannot take the victim's ceremony
// by naming its field, and another session of the same organization cannot
// take it by naming the victim.
func TestResolveCeremonyReturnsOnlyTheCallersOwnCeremony(t *testing.T) {
	database, repo, ceremony := verifiedBindingCeremony(t)
	victim := ApplyCmd{DID: bindingContractDID, SignerDID: bindingSigner, AppliedBy: bindingOrg, HolderDID: bindingHolder}

	attacker := victim
	attacker.SignerDID, attacker.FieldName = "did:jwk:attacker", bindingField
	_, err := resolveInTx(t, database, repo, attacker)
	require.ErrorIs(t, err, ErrCeremonyRequired, "the field path returned another signer's ceremony")

	otherSession := victim
	otherSession.FieldName, otherSession.HolderDID = bindingField, "did:jwk:other-session"
	_, err = resolveInTx(t, database, repo, otherSession)
	require.ErrorIs(t, err, ErrCeremonyNotCallers)
	otherSession.CeremonyID, otherSession.FieldName = ceremony.ID, ""
	_, err = resolveInTx(t, database, repo, otherSession)
	require.ErrorIs(t, err, ErrCeremonyNotCallers)

	for _, cmd := range []ApplyCmd{
		{DID: victim.DID, SignerDID: victim.SignerDID, AppliedBy: victim.AppliedBy, HolderDID: victim.HolderDID, FieldName: bindingField},
		{DID: victim.DID, SignerDID: victim.SignerDID, AppliedBy: victim.AppliedBy, HolderDID: victim.HolderDID, CeremonyID: ceremony.ID},
	} {
		got, err := resolveInTx(t, database, repo, cmd)
		require.NoError(t, err)
		require.Equal(t, ceremony.ID, got.ID)
	}

	_, err = database.Exec(`UPDATE signature_ceremonies SET consumed_at = now() WHERE id = $1`, ceremony.ID)
	require.NoError(t, err)
	victim.CeremonyID = ceremony.ID
	_, err = resolveInTx(t, database, repo, victim)
	require.ErrorIs(t, err, ErrCeremonyConsumed)
	victim.CeremonyID, victim.FieldName = "", bindingField
	_, err = resolveInTx(t, database, repo, victim)
	require.ErrorIs(t, err, ErrCeremonyRequired, "a consumed ceremony resolved through the field")
}

// DCS-FR-SM-03, DCS-FR-SM-20: the document to be signed is prepared only
// against a current presentation, and a consumed ceremony is never published
// again, so a re-signature after a revocation needs a new presentation.
func TestAConsumedOrStaleCeremonyDoesNotSignAgain(t *testing.T) {
	database, repo, ceremony := verifiedBindingCeremony(t)
	ctx := context.Background()

	require.NoError(t, assertCeremonyFresh(ceremony, time.Now().UTC()))
	stale := time.Now().UTC().Add(-verifiedCeremonyTTL - time.Minute)
	ceremony.VerifiedAt = &stale
	require.ErrorIs(t, assertCeremonyFresh(ceremony, time.Now().UTC()), ErrCeremonyRequired)

	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.MarkCeremonyConsumed(ctx, tx, ceremony.ID))
	err = repo.StorePreparedRequest(ctx, tx, db.PreparedRequest{CeremonyID: ceremony.ID, RequestNonce: "n2", RequestExpiresAt: time.Now().Add(time.Hour), CredentialType: "AES", Roles: []byte("[]")})
	require.Error(t, err, "a consumed ceremony was published again")
	consumed, err := repo.GetCeremonyByID(ctx, tx, ceremony.ID)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
}

// DCS-FR-SM-03, DCS-FR-UC-14-1: the Power of Attorney presented at the
// ceremony is issued to the person the PID identifies and grants the Contract
// Signer role.
func TestPresentationRequiresAPoAOfTheSignatoryWithTheSignerRole(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version) VALUES ($1,'',$2,'APPROVED','tpl',1)`, bindingContractDID, bindingOrg)
	require.NoError(t, err)
	repo := &pg.PostgresCeremonyRepo{}
	present := &PresentationHandler{DB: database, CeremonyRepo: repo}
	start := func() string {
		c, err := (&StartCeremonyHandler{DB: database, CeremonyRepo: repo}).Handle(ctx, StartCeremonyCmd{
			ContractDID: bindingContractDID, FieldName: bindingField, RequestedBy: bindingOrg, BaseURL: "https://dcs.example", ClientID: "x509_san_dns:dcs.example"})
		require.NoError(t, err)
		return c.ID
	}
	cmd := func(id, subject string, roles ...string) PresentationCmd {
		return PresentationCmd{CeremonyID: id, SignerDID: bindingSigner, SDHash: "sd", VpToken: "vp", PoAOrganization: bindingField, PoASubjectDID: subject, PoARoles: roles}
	}

	id := start()
	_, err = present.CompletePresentation(ctx, cmd(id, bindingSigner, "Contract Reviewer"))
	require.ErrorIs(t, err, ErrPoAUnauthorized, "a PoA without the Contract Signer role verified the ceremony")
	_, err = present.CompletePresentation(ctx, cmd(id, "did:jwk:someone-else", "Contract Signer"))
	require.ErrorIs(t, err, ErrPoAUnauthorized, "a PoA of another person verified the ceremony")
	verified, err := present.CompletePresentation(ctx, cmd(id, bindingSigner, "Contract Reviewer", "Contract Signer"))
	require.NoError(t, err)
	require.Equal(t, db.CeremonyVerified, verified.Status)
}

// DCS-FR-SM-19, DCS-FR-SM-23: a refused wallet response is kept on the
// ceremony, where the status poll reports it, and audited; refusals that
// follow within walletRefusalAuditInterval update the ceremony but add no
// audit entry, so the unauthenticated wallet endpoints cannot flood the trail.
func TestWalletRefusalsAreReportedAndAuditedAtMostOncePerInterval(t *testing.T) {
	database, repo, ceremony := verifiedBindingCeremony(t)
	ctx := context.Background()
	cmd := ApplyCmd{DID: bindingContractDID, SignerDID: bindingSigner, FieldName: bindingField, CeremonyID: ceremony.ID}
	audited := func() int {
		var n int
		require.NoError(t, database.Get(&n, `SELECT count(*) FROM outbox_events WHERE event_type = 'APPLY_SIGNATURE_FAILED'`))
		return n
	}

	RecordWalletRefusal(ctx, database, repo, cmd, "WALLET_CALLBACK", "document_mismatch", errors.New("submitted document does not match\x00\n"))
	RecordWalletRefusal(ctx, database, repo, cmd, "WALLET_CALLBACK", "nonce_mismatch", errors.New("not bound to the nonce"))
	require.Equal(t, 1, audited())

	status, err := (&query.CeremonyStatusHandler{DB: database, CeremonyRepo: repo}).Handle(ctx, query.CeremonyStatusQry{CeremonyID: ceremony.ID})
	require.NoError(t, err)
	require.Equal(t, "nonce_mismatch", *status.LastRefusalCode)
	require.Equal(t, "not bound to the nonce", *status.LastRefusal)

	_, err = database.Exec(`UPDATE signature_ceremonies SET last_refusal_at = last_refusal_at - interval '1 minute' WHERE id = $1`, ceremony.ID)
	require.NoError(t, err)
	RecordWalletRefusal(ctx, database, repo, cmd, "WALLET_CALLBACK", "nonce_mismatch", errors.New("not bound to the nonce"))
	require.Equal(t, 2, audited())

	require.Equal(t, "a b ", SanitizeRefusal("a\x00b\n"))
	require.Len(t, []rune(SanitizeRefusal(string(make([]rune, 2*maxRefusalLength)))), maxRefusalLength+1)
}

// DCS-FR-SM-13, DCS-FR-SM-23: a published signing request that passes its
// expiry without a signature reports expired.
func TestAnExpiredSigningRequestReportsExpired(t *testing.T) {
	database, repo, ceremony := verifiedBindingCeremony(t)
	ctx := context.Background()
	_, err := database.Exec(`UPDATE signature_ceremonies SET request_expires_at = now() - interval '1 minute' WHERE id = $1`, ceremony.ID)
	require.NoError(t, err)
	status, err := (&query.CeremonyStatusHandler{DB: database, CeremonyRepo: repo}).Handle(ctx, query.CeremonyStatusQry{CeremonyID: ceremony.ID})
	require.NoError(t, err)
	require.Equal(t, db.CeremonyExpired, status.Status)
}
