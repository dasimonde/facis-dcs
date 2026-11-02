package middleware

import (
	"context"
	"testing"
	"time"

	"digital-contracting-service/internal/base/pgtest"
)

// DCS-FR-CWE-28 with more than one replica: two limiter instances over the
// same database enforce one budget per caller, not one budget each.
func TestRateLimiterEnforcesOneBudgetAcrossReplicas(t *testing.T) {
	db := pgtest.Open(t)
	replicas := []*RateLimiter{NewRateLimiter(db, 3), NewRateLimiter(db, 3)}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, ok := replicas[i%2].Allow(ctx, "did:example:alice"); !ok {
			t.Fatalf("request %d within budget refused", i+1)
		}
	}
	for replica, l := range replicas {
		if wait, ok := l.Allow(ctx, "did:example:alice"); ok || wait <= 0 {
			t.Fatalf("request over budget on replica %d allowed (wait %v)", replica, wait)
		}
	}
	// A different caller has its own budget.
	if _, ok := replicas[1].Allow(ctx, "did:example:bob"); !ok {
		t.Fatal("other caller refused")
	}
}

func TestRateLimiterWindowResets(t *testing.T) {
	db := pgtest.Open(t)
	l := NewRateLimiter(db, 1)
	ctx := context.Background()
	now := time.Now()
	if _, ok, err := l.take(ctx, "tok", now); err != nil || !ok {
		t.Fatalf("first take = %v, %v; want within budget", ok, err)
	}
	if retry, ok, err := l.take(ctx, "tok", now.Add(30*time.Second)); err != nil || ok || retry <= 0 || retry > 31*time.Second {
		t.Fatalf("second take in the same window = %v, %v, retry %v; want over budget, retry about 30s", ok, err, retry)
	}
	if _, ok, err := l.take(ctx, "tok", now.Add(61*time.Second)); err != nil || !ok {
		t.Fatalf("take after the window elapsed = %v, %v; want the budget reset", ok, err)
	}
}

// Concurrent requests for one credential on two replicas are each counted
// exactly once.
func TestRateLimiterCountsConcurrentRequestsAtomically(t *testing.T) {
	db := pgtest.Open(t)
	replicas := []*RateLimiter{NewRateLimiter(db, 10), NewRateLimiter(db, 10)}
	now := time.Now()
	results := make(chan bool, 30)
	for i := 0; i < 30; i++ {
		go func(i int) {
			_, ok, err := replicas[i%2].take(context.Background(), "tok", now)
			results <- ok && err == nil
		}(i)
	}
	allowed := 0
	for i := 0; i < 30; i++ {
		if <-results {
			allowed++
		}
	}
	if allowed != 10 {
		t.Fatalf("allowed = %d of 30 concurrent requests, want exactly the budget of 10", allowed)
	}
}

// A new window deletes the rows of windows that ended, so the table does not
// grow with every credential ever seen.
func TestRateLimiterDeletesEndedWindows(t *testing.T) {
	db := pgtest.Open(t)
	l := NewRateLimiter(db, 10)
	ctx := context.Background()
	now := time.Now()
	if _, _, err := l.take(ctx, "old", now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.take(ctx, "new", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.Get(&n, `SELECT count(*) FROM api_rate_limit_windows`); err != nil || n != 1 {
		t.Fatalf("rows = %d, %v; want 1 (the ended window deleted)", n, err)
	}
}
