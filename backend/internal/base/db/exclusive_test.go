package db

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"digital-contracting-service/internal/base/pgtest"
)

// DCS-NFR-PER-02: a scheduled job runs on one replica at a time. While one
// replica runs it, the other replica skips; after it returns, the job is free.
func TestRunExclusiveRunsAJobOnOneReplicaAtATime(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()

	var otherRan, otherOK bool
	ran, err := RunExclusive(ctx, database, "test-job", func() error {
		ok, err := RunExclusive(ctx, database, "test-job", func() error { otherRan = true; return nil })
		otherOK = ok
		if err != nil {
			t.Errorf("second replica: %v", err)
		}
		// A different job is not blocked.
		if ok, err := RunExclusive(ctx, database, "other-job", func() error { return nil }); err != nil || !ok {
			t.Errorf("other job while test-job runs = %v, %v; want it to run", ok, err)
		}
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("first replica = %v, %v; want it to run", ran, err)
	}
	if otherOK || otherRan {
		t.Fatal("second replica ran the job while the first replica held it")
	}

	if ok, err := RunExclusive(ctx, database, "test-job", func() error { return nil }); err != nil || !ok {
		t.Fatalf("after the first replica finished = %v, %v; want the job free again", ok, err)
	}
}

// DCS-FR-SM-12: while one ship of a contract runs, the ships waiting for it
// hold no pooled connection, so the running ship still gets the connections
// its own transactions need.
func TestRunLockedWaitersLeaveThePoolToTheHolder(t *testing.T) {
	database := pgtest.Open(t)
	database.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	release := make(chan struct{})
	holderErr := make(chan error, 1)
	go func() {
		holderErr <- RunLocked(ctx, database, "contract-ship:x", func() error {
			<-release
			_, err := database.ExecContext(ctx, `SELECT 1`)
			return err
		})
	}()
	time.Sleep(200 * time.Millisecond)
	waiters := make(chan error, 3)
	for range 3 {
		go func() { waiters <- RunLocked(ctx, database, "contract-ship:x", func() error { return nil }) }()
	}
	time.Sleep(300 * time.Millisecond)
	close(release)

	if err := <-holderErr; err != nil {
		t.Fatalf("the running ship could not get a connection for its own work: %v", err)
	}
	for range 3 {
		if err := <-waiters; err != nil {
			t.Fatalf("a waiting ship: %v", err)
		}
	}
}

// RunLocked makes replicas that start a job together run it one after the
// other: each runs it, never two at once.
func TestRunLockedRunsAJobOnEveryReplicaOneAfterTheOther(t *testing.T) {
	database := pgtest.Open(t)
	const replicas = 3
	var inFlight, maxInFlight, ran atomic.Int32
	errs := make(chan error, replicas)
	for i := 0; i < replicas; i++ {
		go func() {
			errs <- RunLocked(context.Background(), database, "test-startup-job", func() error {
				n := inFlight.Add(1)
				if n > maxInFlight.Load() {
					maxInFlight.Store(n)
				}
				time.Sleep(50 * time.Millisecond)
				inFlight.Add(-1)
				ran.Add(1)
				return nil
			})
		}()
	}
	for i := 0; i < replicas; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("RunLocked: %v", err)
		}
	}
	if ran.Load() != replicas || maxInFlight.Load() != 1 {
		t.Fatalf("ran %d times, at most %d at once; want %d runs, one at a time", ran.Load(), maxInFlight.Load(), replicas)
	}
}
