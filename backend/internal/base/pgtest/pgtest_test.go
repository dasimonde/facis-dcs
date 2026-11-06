package pgtest

import (
	"runtime"
	"testing"
)

// outcome records whether URL failed or skipped the test it was given.
type outcome struct {
	testing.TB
	failed, skipped bool
}

func (o *outcome) Helper()      {}
func (o *outcome) Fatal(...any) { o.failed = true; runtime.Goexit() }
func (o *outcome) Skip(...any)  { o.skipped = true; runtime.Goexit() }

func urlOutcome(t *testing.T) *outcome {
	o := &outcome{TB: t}
	done := make(chan struct{})
	go func() { defer close(done); URL(o) }()
	<-done
	return o
}

func TestURLFailsInCIWithoutDatabase(t *testing.T) {
	t.Setenv("QA_DATABASE_URL", "")

	t.Setenv("CI", "true")
	if o := urlOutcome(t); !o.failed || o.skipped {
		t.Fatalf("in CI without QA_DATABASE_URL: failed=%v skipped=%v, want failed", o.failed, o.skipped)
	}

	t.Setenv("CI", "")
	if o := urlOutcome(t); o.failed || !o.skipped {
		t.Fatalf("outside CI without QA_DATABASE_URL: failed=%v skipped=%v, want skipped", o.failed, o.skipped)
	}
}
