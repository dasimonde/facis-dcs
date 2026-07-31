package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitAuthenticatedEnforcesPerCredentialBudget(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RateLimitAuthenticated(3, next)

	do := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/contract/search", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 3; i++ {
		if code := do("alice"); code != http.StatusOK {
			t.Fatalf("request %d within budget got %d", i+1, code)
		}
	}
	if code := do("alice"); code != http.StatusTooManyRequests {
		t.Fatalf("request over budget got %d, want 429", code)
	}
	// A different credential has its own budget.
	if code := do("bob"); code != http.StatusOK {
		t.Fatalf("other credential got %d, want 200", code)
	}
	// Unauthenticated requests are never counted or limited.
	for i := 0; i < 10; i++ {
		if code := do(""); code != http.StatusOK {
			t.Fatalf("unauthenticated request got %d, want 200", code)
		}
	}
}

func TestRateLimiterWindowResets(t *testing.T) {
	l := &rateLimiter{limit: 1, windows: map[[16]byte]*window{}}
	now := time.Now()
	if _, ok := l.take("tok", now); !ok {
		t.Fatal("first take should be within budget")
	}
	if _, ok := l.take("tok", now.Add(30*time.Second)); ok {
		t.Fatal("second take in the same window should be over budget")
	}
	if _, ok := l.take("tok", now.Add(61*time.Second)); !ok {
		t.Fatal("take after the window elapsed should reset the budget")
	}
}
