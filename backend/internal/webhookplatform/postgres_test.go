package webhookplatform

import (
	"bytes"
	"context"
	"testing"

	"digital-contracting-service/internal/base/pgtest"
)

// Runs only against a real database: QA_DATABASE_URL must point to an empty
// Postgres that the migrations may write to.
func TestPostgresSubscriptionsScopeToOwner(t *testing.T) {
	db := pgtest.Open(t)

	ctx := context.Background()
	s := PostgresSubscriptions{DB: db, Secrets: testSecretBox(t)}
	secretA, _ := NewSecret()
	a, err := s.Add(ctx, Subscription{Event: "template.updated", CallbackURL: "https://a.example", Secret: secretA, OwnerDID: orgA, CreatedBy: "user-a"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	secretB, _ := NewSecret()
	if _, err := s.Add(ctx, Subscription{Event: "contract.created", CallbackURL: "https://b.example", Secret: secretB, OwnerDID: orgB, CreatedBy: "user-b"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	var stored []byte
	if err := db.Get(&stored, `SELECT secret_ciphertext FROM webhook_subscriptions WHERE id = $1`, a.ID); err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	if len(stored) == 0 || bytes.Contains(stored, []byte(secretA)) {
		t.Fatalf("stored secret_ciphertext = %x, want ciphertext without the secret", stored)
	}

	owned, err := s.ListByOwner(ctx, orgA)
	if err != nil || len(owned) != 1 || owned[0].ID != a.ID {
		t.Fatalf("ListByOwner(orgA) = %+v, %v; want only %s", owned, err, a.ID)
	}
	byEvent, err := s.ByEvent(ctx, "template.updated")
	if err != nil || len(byEvent) != 1 || byEvent[0].Secret != secretA {
		t.Fatalf("ByEvent = %+v, %v; want org A's subscription with its secret", byEvent, err)
	}

	// A subscription whose secret no longer opens is skipped, not fatal.
	secretC, _ := NewSecret()
	c, err := s.Add(ctx, Subscription{Event: "template.updated", CallbackURL: "https://c.example", Secret: secretC, OwnerDID: orgB, CreatedBy: "user-c"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := db.Exec(`UPDATE webhook_subscriptions SET secret_ciphertext = '\x00' WHERE id = $1`, c.ID); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	byEvent, err = s.ByEvent(ctx, "template.updated")
	if err != nil || len(byEvent) != 1 || byEvent[0].ID != a.ID {
		t.Fatalf("ByEvent with one unreadable row = %+v, %v; want only org A's subscription", byEvent, err)
	}

	if ok, err := s.DeleteOwned(ctx, a.ID, orgB); err != nil || ok {
		t.Fatalf("DeleteOwned by orgB = %v, %v; want false", ok, err)
	}
	if ok, err := s.DeleteOwned(ctx, "not-a-uuid", orgA); err != nil || ok {
		t.Fatalf("DeleteOwned(not-a-uuid) = %v, %v; want false, nil", ok, err)
	}
	if ok, err := s.DeleteOwned(ctx, a.ID, orgA); err != nil || !ok {
		t.Fatalf("DeleteOwned by owner = %v, %v; want true", ok, err)
	}
}
