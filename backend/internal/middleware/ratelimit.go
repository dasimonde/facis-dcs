package middleware

import (
	"context"
	"crypto/sha256"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
)

// RateLimiter enforces the per-caller request budget for API interactions
// (DCS-FR-CWE-28). The authenticator calls it after the token is validated,
// with the validated caller identity as key. So a request that does not
// authenticate is not counted and writes no counter row, and one caller
// cannot get more budget from other tokens or other spellings of the
// Authorization header.
//
// The counters are in Postgres, so all replicas of the instance share one
// budget per caller. When the counter cannot be read, the request is let
// through: the limiter does not make a database fault a 429.
type RateLimiter struct {
	db    *sqlx.DB
	limit int
}

func NewRateLimiter(db *sqlx.DB, limitPerMinute int) *RateLimiter {
	return &RateLimiter{db: db, limit: limitPerMinute}
}

// Limit is the budget per caller and minute.
func (l *RateLimiter) Limit() int { return l.limit }

// Allow counts one request of caller. When the caller is over budget it
// returns false and how long until the window resets.
func (l *RateLimiter) Allow(ctx context.Context, caller string) (time.Duration, bool) {
	retryAfter, ok, err := l.take(ctx, caller, time.Now())
	if err != nil {
		log.Printf("rate limit: count request: %v", err)
		return 0, true
	}
	return retryAfter, ok
}

// take counts one request for the caller in one atomic upsert and reports
// whether it is within budget; when over budget it returns how long until the
// window resets. A window older than a minute restarts at now with count 1.
// When a window restarts, rows of windows that ended are deleted.
func (l *RateLimiter) take(ctx context.Context, caller string, now time.Time) (time.Duration, bool, error) {
	sum := sha256.Sum256([]byte(caller))
	var row struct {
		Start time.Time `db:"window_start"`
		Count int       `db:"request_count"`
	}
	err := l.db.QueryRowxContext(ctx, `
		INSERT INTO api_rate_limit_windows AS w (credential_hash, window_start, request_count)
		VALUES ($1, $2::timestamptz, 1)
		ON CONFLICT (credential_hash) DO UPDATE SET
			window_start  = CASE WHEN w.window_start <= $2 - INTERVAL '1 minute' THEN $2 ELSE w.window_start END,
			request_count = CASE WHEN w.window_start <= $2 - INTERVAL '1 minute' THEN 1 ELSE w.request_count + 1 END
		RETURNING window_start, request_count`, sum[:16], now).StructScan(&row)
	if err != nil {
		return 0, false, err
	}
	if row.Count == 1 {
		if _, err := l.db.ExecContext(ctx, `DELETE FROM api_rate_limit_windows WHERE window_start <= $1::timestamptz - INTERVAL '1 minute'`, now); err != nil {
			log.Printf("rate limit: delete ended windows: %v", err)
		}
	}
	if row.Count > l.limit {
		return time.Minute - now.Sub(row.Start), false, nil
	}
	return 0, true, nil
}

type responseHeaderKey struct{}

// ResponseHeaders lets code that sees only the request context, such as the
// authenticator, set response headers (Retry-After).
func ResponseHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), responseHeaderKey{}, w.Header())))
	})
}

// SetRetryAfter sets the Retry-After header of the response to wait, rounded
// up to whole seconds.
func SetRetryAfter(ctx context.Context, wait time.Duration) {
	if h, ok := ctx.Value(responseHeaderKey{}).(http.Header); ok {
		h.Set("Retry-After", strconv.Itoa(int((wait+time.Second-1)/time.Second)))
	}
}
