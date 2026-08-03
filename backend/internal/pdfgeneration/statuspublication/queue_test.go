package statuspublication

import (
	"testing"
	"time"
)

func TestRetryDelayIsPromptAndBounded(t *testing.T) {
	if got := retryDelay(1); got != time.Second {
		t.Fatalf("first retry = %s, want 1s", got)
	}
	if got := retryDelay(20); got != time.Minute {
		t.Fatalf("retry cap = %s, want 1m", got)
	}
}
