package statuspublication

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

type failingPublisher struct{ provenance.StatusListPublisher }

func (failingPublisher) PublishStatus(context.Context, string, string, string, time.Time) (provenance.CredentialStatusRef, error) {
	return provenance.CredentialStatusRef{}, errors.New("status list store unavailable")
}

// A terminal contract state enqueued with the transition reaches the status
// list this deployment serves (DCS-IR-SI-09): a failed delivery stays pending
// with its error, and the next delivery sets the contract's revocation bit.
func TestQueuedTerminationSetsTheServedRevocationBit(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	const contract = "did:web:dcs.example:contract:terminated"

	for range 2 {
		tx := db.MustBeginTx(ctx, nil)
		if err := EnqueueTx(ctx, tx, contract, "TERMINATED", "end", time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	failing := &Worker{DB: db, Publisher: failingPublisher{}}
	if processed, err := failing.ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("failed delivery: processed=%v err=%v", processed, err)
	}
	var row struct {
		Pending   bool    `db:"pending"`
		LastError *string `db:"last_error"`
	}
	if err := db.Get(&row, `SELECT published_at IS NULL AS pending, last_error FROM status_publication_queue WHERE contract_did = $1`, contract); err != nil {
		t.Fatal(err)
	}
	if !row.Pending || row.LastError == nil {
		t.Fatalf("a failed delivery must stay pending with its error, got %+v", row)
	}
	if _, err := db.Exec(`UPDATE status_publication_queue SET next_attempt_at = CURRENT_TIMESTAMP - INTERVAL '1 second'`); err != nil {
		t.Fatal(err)
	}

	revocations := &provenance.PostgresStatusListRevocations{DB: db}
	allocator := provenance.NewPostgresStatusListAllocator(db, provenance.DefaultListID)
	publisher := provenance.NewDCSStatusListPublisher(
		func(listID int) string { return provenance.StatusListURI("https://dcs.example", listID) }, allocator, revocations)
	worker := &Worker{DB: db, Publisher: publisher}
	if processed, err := worker.ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("delivery: processed=%v err=%v", processed, err)
	}
	if processed, _ := worker.ProcessOnce(ctx); processed {
		t.Fatal("a delivered entry must not be delivered again")
	}

	entry, err := allocator.Allocate(ctx, contract)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := revocations.RevokedIndices(ctx, entry.ListID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(revoked, entry.Index) {
		t.Fatalf("revoked indices %v of list %d do not contain the terminated contract's index %d", revoked, entry.ListID, entry.Index)
	}
}

// DCS-OR-C2PA-005: a revoked signature suspends the contract's entry, which is
// not a revocation, and signing again ends the suspension. A terminated
// contract's entry stays revoked whatever is published after.
func TestQueuedSuspensionIsDistinctAndEndsWhenTheContractIsActiveAgain(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	const contract = "did:web:dcs.example:contract:suspended"

	tx := db.MustBeginTx(ctx, nil)
	if err := EnqueueTx(ctx, tx, contract, "suspended", "signature revoked", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	revocations := &provenance.PostgresStatusListRevocations{DB: db}
	allocator := provenance.NewPostgresStatusListAllocator(db, provenance.DefaultListID)
	publisher := provenance.NewDCSStatusListPublisher(
		func(listID int) string { return provenance.StatusListURI("https://dcs.example", listID) }, allocator, revocations)
	if processed, err := (&Worker{DB: db, Publisher: publisher}).ProcessOnce(ctx); err != nil || !processed {
		t.Fatalf("delivery: processed=%v err=%v", processed, err)
	}

	entry, err := allocator.Allocate(ctx, contract)
	if err != nil {
		t.Fatal(err)
	}
	states := func() (revoked, suspended bool) {
		r, err := revocations.RevokedIndices(ctx, entry.ListID)
		if err != nil {
			t.Fatal(err)
		}
		s, err := revocations.SuspendedIndices(ctx, entry.ListID)
		if err != nil {
			t.Fatal(err)
		}
		return slices.Contains(r, entry.Index), slices.Contains(s, entry.Index)
	}
	if revoked, suspended := states(); revoked || !suspended {
		t.Fatalf("after suspension: revoked=%v suspended=%v, want suspended only", revoked, suspended)
	}

	if _, err := publisher.PublishStatus(ctx, contract, "active", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if revoked, suspended := states(); revoked || suspended {
		t.Fatalf("after signing again: revoked=%v suspended=%v, want neither", revoked, suspended)
	}

	for _, status := range []string{"terminated", "suspended", "active"} {
		if _, err := publisher.PublishStatus(ctx, contract, status, "", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if revoked, suspended := states(); !revoked || suspended {
		t.Fatalf("after termination: revoked=%v suspended=%v, want revoked only", revoked, suspended)
	}
}
