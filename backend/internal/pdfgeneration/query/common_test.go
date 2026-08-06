package query

import (
	"context"
	"errors"
	"strings"
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

// The failure the report shows is the one that happened. Every failure here
// used to be described as the status service being unreachable, so a list that
// was served in full and could not be parsed sent the reader to the network
// (ADR-34, and the same misattribution in signingmanagement/query/verify.go).
func TestEvaluateLiveStatusCheckReportsTheFailureItHad(t *testing.T) {
	_, _, failure, _ := evaluateLiveStatusCheck(context.Background(), "active", func() (string, error) {
		return "", errors.New("parse status list response: unexpected end of JSON input")
	})
	if !strings.Contains(failure, "parse status list response") {
		t.Fatalf("failure = %q, want the parse error it actually had", failure)
	}
	if strings.Contains(strings.ToLower(failure), "unreachable") {
		t.Fatalf("failure = %q, but the list was served — nothing was unreachable", failure)
	}
}

// The state is reported plainly, because the list it came off was verified: its
// signature checked against the configured anchors and its leaf identified the
// issuer it names (ADR-34 §3). It used to be qualified as UNVERIFIED — correctly,
// while the list was fetched unsigned and whoever answered the URL chose the
// answer. That qualification must not survive the list becoming signed, or every
// report would keep disclaiming a state that IS established.
func TestEvaluateLiveStatusCheckReportsTheVerifiedStateAsItIs(t *testing.T) {
	for _, want := range []string{"active", "revoked"} {
		t.Run(want, func(t *testing.T) {
			status, check, failure, passed := evaluateLiveStatusCheck(context.Background(), "active", func() (string, error) {
				return want, nil
			})
			if !passed || check != "passed" || failure != "" {
				t.Fatalf("got check=%q failure=%q passed=%v", check, failure, passed)
			}
			if status != want {
				t.Fatalf("status = %q, want %q", status, want)
			}
		})
	}
}
