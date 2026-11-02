package command

import (
	"context"
	"errors"
	"testing"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/db/pg"
	"digital-contracting-service/internal/signingmanagement/query"
)

// FR-SM-13 retries: a ceremony whose presentation fails is presented again; a
// ceremony whose deadline passes reports expired, and the signer starts a new
// ceremony for the same field, which then completes.
func TestFailedOrExpiredCeremonyCanBeRetried(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	const contractDID, field = "did:example:retry-contract", "did:example:party-a"
	if _, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version) VALUES ($1,'','Org A','APPROVED','tpl',1)`, contractDID); err != nil {
		t.Fatalf("insert contract: %v", err)
	}
	repo := &pg.PostgresCeremonyRepo{}
	start := &StartCeremonyHandler{DB: database, CeremonyRepo: repo}
	present := &PresentationHandler{DB: database, CeremonyRepo: repo}
	status := &query.CeremonyStatusHandler{DB: database, CeremonyRepo: repo}
	startCmd := StartCeremonyCmd{ContractDID: contractDID, FieldName: field, RequestedBy: "did:example:signer", BaseURL: "https://dcs.example", ClientID: "x509_san_dns:dcs.example"}
	presentation := func(ceremonyID, organization string) PresentationCmd {
		return PresentationCmd{CeremonyID: ceremonyID, SignerDID: "did:example:signer", SDHash: "sd", VpToken: "vp", PoAOrganization: organization, PoASubjectDID: "did:example:signer", PoARoles: []string{"Contract Signer"}}
	}
	statusOf := func(id string) string {
		t.Helper()
		c, err := status.Handle(ctx, query.CeremonyStatusQry{CeremonyID: id})
		if err != nil || c == nil {
			t.Fatalf("ceremony status %s: %v", id, err)
		}
		return c.Status
	}

	first, err := start.Handle(ctx, startCmd)
	if err != nil {
		t.Fatalf("start ceremony: %v", err)
	}
	if _, err := present.CompletePresentation(ctx, presentation(first.ID, "did:example:other-party")); !errors.Is(err, ErrPoAUnauthorized) {
		t.Fatalf("a PoA for another party was accepted: %v", err)
	}
	if got := statusOf(first.ID); got != db.CeremonyPending {
		t.Fatalf("after a failed presentation the ceremony is %q, want %q", got, db.CeremonyPending)
	}

	if _, err := database.Exec(`UPDATE signature_ceremonies SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = $1`, first.ID); err != nil {
		t.Fatalf("pass the deadline: %v", err)
	}
	if got := statusOf(first.ID); got != db.CeremonyExpired {
		t.Fatalf("a ceremony after its deadline reports %q, want %q", got, db.CeremonyExpired)
	}
	if _, err := present.CompletePresentation(ctx, presentation(first.ID, field)); !errors.Is(err, ErrCeremonyExpired) {
		t.Fatalf("a presentation after the deadline was accepted: %v", err)
	}

	retry, err := start.Handle(ctx, startCmd)
	if err != nil {
		t.Fatalf("start a new ceremony after expiry: %v", err)
	}
	if _, err := present.CompletePresentation(ctx, presentation(retry.ID, "did:example:other-party")); !errors.Is(err, ErrPoAUnauthorized) {
		t.Fatalf("a PoA for another party was accepted: %v", err)
	}
	if _, err := present.CompletePresentation(ctx, presentation(retry.ID, field)); err != nil {
		t.Fatalf("the presentation on the retried ceremony failed: %v", err)
	}
	if got := statusOf(retry.ID); got != db.CeremonyVerified {
		t.Fatalf("the retried ceremony is %q, want %q", got, db.CeremonyVerified)
	}

	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	verified, err := repo.FindVerifiedCeremonyByField(ctx, tx, contractDID, field)
	if err != nil || verified == nil || verified.ID != retry.ID {
		t.Fatalf("the verified ceremony for the field is %+v (%v), want the retried ceremony %s", verified, err, retry.ID)
	}
}
