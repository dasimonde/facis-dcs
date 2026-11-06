package event

import (
	"testing"
	"time"
)

// DCS-NFR-SF-01: a run of an outbox job that lasts longer than its limit is
// reported stuck; an idle job or a run within its limit is not.
func TestStuckJobsReportsARunPastItsLimit(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = trackRun("outbox-publishing", func() error { <-release; return nil })
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for jobRuns["outbox-publishing"].started.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the run did not start")
		}
		time.Sleep(time.Millisecond)
	}

	if stuck := StuckJobs(time.Now()); len(stuck) != 0 {
		t.Fatalf("a run within its limit is reported stuck: %v", stuck)
	}
	if stuck := StuckJobs(time.Now().Add(6 * time.Minute)); len(stuck) != 1 {
		t.Fatalf("a publishing run of 6 minutes is not reported stuck: %v", stuck)
	}

	close(release)
	<-done
	if stuck := StuckJobs(time.Now().Add(6 * time.Minute)); len(stuck) != 0 {
		t.Fatalf("a finished run is reported stuck: %v", stuck)
	}
}
