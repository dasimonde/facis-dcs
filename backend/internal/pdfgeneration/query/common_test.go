package query

import (
	"context"
	"errors"
	"testing"
)

func TestQueryStatusWithPublicationBarrierWaitsForTerminalPublication(t *testing.T) {
	calls := 0
	status, err := queryStatusWithPublicationBarrier(context.Background(), "terminated", func() (string, error) {
		calls++
		if calls == 1 {
			return "active", nil
		}
		return "revoked", nil
	})
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "revoked" {
		t.Fatalf("status = %q, want revoked", status)
	}
	if calls != 2 {
		t.Fatalf("query calls = %d, want 2", calls)
	}
}

func TestQueryStatusWithPublicationBarrierDoesNotWaitForActiveLifecycle(t *testing.T) {
	calls := 0
	status, err := queryStatusWithPublicationBarrier(context.Background(), "active", func() (string, error) {
		calls++
		return "active", nil
	})
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "active" {
		t.Fatalf("status = %q, want active", status)
	}
	if calls != 1 {
		t.Fatalf("query calls = %d, want 1", calls)
	}
}

func TestQueryStatusWithPublicationBarrierReturnsOutageError(t *testing.T) {
	outage := errors.New("status list service unavailable")
	status, err := queryStatusWithPublicationBarrier(context.Background(), "active", func() (string, error) {
		return "", outage
	})
	if !errors.Is(err, outage) {
		t.Fatalf("error = %v, want outage", err)
	}
	if status != "" {
		t.Fatalf("status = %q, want empty", status)
	}
}

func TestEvaluateLiveStatusCheckMarksOutageFailed(t *testing.T) {
	status, check, failure, passed := evaluateLiveStatusCheck(context.Background(), "active", func() (string, error) {
		return "", errors.New("connection refused")
	})
	if passed {
		t.Fatal("outage check passed, want failed")
	}
	if status != "unavailable" || check != "failed" {
		t.Fatalf("status/check = %q/%q, want unavailable/failed", status, check)
	}
	if failure == "" {
		t.Fatal("failure reason is empty")
	}
}

func TestEvaluateLiveStatusCheckKeepsActiveAndRevokedResults(t *testing.T) {
	for _, want := range []string{"active", "revoked"} {
		t.Run(want, func(t *testing.T) {
			status, check, failure, passed := evaluateLiveStatusCheck(context.Background(), "active", func() (string, error) {
				return want, nil
			})
			if !passed || status != want || check != "passed" || failure != "" {
				t.Fatalf("got status=%q check=%q failure=%q passed=%v", status, check, failure, passed)
			}
		})
	}
}
