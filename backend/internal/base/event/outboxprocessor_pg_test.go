package event

import (
	"context"
	"testing"

	"digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/base/pgtest"
)

// DCS-NFR-PER-02: only one replica anchors outbox events at a time. While
// another replica holds the anchoring job, this replica's pass leaves the
// outbox alone; once the job is free, the pass processes the events.
func TestAnchorPendingSkipsWhileAnotherReplicaAnchors(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	if _, err := database.Exec(`INSERT INTO outbox_events (component, event_type, event_data, did, processed)
		VALUES ('CONTRACT_WORKFLOW_ENGINE', 'RETRIEVE_CONTRACT', '{}', 'did:web:me:contract:1', FALSE)`); err != nil {
		t.Fatal(err)
	}
	unprocessed := func() int {
		var n int
		if err := database.Get(&n, `SELECT count(*) FROM outbox_events WHERE processed = FALSE`); err != nil {
			t.Fatal(err)
		}
		return n
	}
	j := OutboxProcessor{DB: database}

	held, err := db.RunExclusive(ctx, database, "outbox-anchoring", func() error {
		if err := j.anchorPending(ctx); err != nil {
			t.Errorf("anchorPending while another replica anchors: %v", err)
		}
		if n := unprocessed(); n != 1 {
			t.Errorf("unprocessed events = %d while another replica anchors, want 1 (left alone)", n)
		}
		return nil
	})
	if err != nil || !held {
		t.Fatalf("hold the anchoring job = %v, %v", held, err)
	}

	if err := j.anchorPending(ctx); err != nil {
		t.Fatalf("anchorPending: %v", err)
	}
	if n := unprocessed(); n != 0 {
		t.Fatalf("unprocessed events = %d after the pass, want 0", n)
	}
}
