package event

import (
	"context"
	"testing"
	"time"

	"digital-contracting-service/internal/base/pgtest"
	tplrepo "digital-contracting-service/internal/templaterepository/db/pg"
)

// DCS-NFR-PER-02: a template's regeneration waits while another replica
// regenerates the same template, before it reads the stored state it renders
// from.
func TestTemplateRegenerationWaitsForTheTemplateLock(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:me:template:locked"

	other, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Rollback() }()
	if _, err := other.Exec("SELECT pg_advisory_xact_lock(hashtext($1))", did); err != nil {
		t.Fatal(err)
	}

	s := &Subscriber{DB: db, TRepo: &tplrepo.PostgresContractTemplateRepo{}}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = s.appendTemplateC2PA(ctx, minimalCWEEvent{DID: did, OccurredAt: time.Now().UTC()})
	if err == nil || ctx.Err() == nil {
		t.Fatalf("regeneration while another replica holds the template returned before the deadline with %v; want it to wait for the lock", err)
	}
}
