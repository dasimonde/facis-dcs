package auth

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	goa "goa.design/goa/v3/pkg"
	"goa.design/goa/v3/security"

	"digital-contracting-service/internal/auth/db"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
)

// nopConn is a database whose transactions do nothing; the repositories below
// are stubs that ignore the transaction handle.
type nopConn struct{}

func (nopConn) Connect(context.Context) (driver.Conn, error) { return nopConn{}, nil }
func (nopConn) Driver() driver.Driver                        { return nil }
func (nopConn) Prepare(string) (driver.Stmt, error)          { return nil, errors.New("no statements") }
func (nopConn) Close() error                                 { return nil }
func (nopConn) Begin() (driver.Tx, error)                    { return nopConn{}, nil }
func (nopConn) Commit() error                                { return nil }
func (nopConn) Rollback() error                              { return nil }

// stubAttempts keeps the attempts in memory and counts them like the
// Postgres repository.
type stubAttempts struct{ rows []db.AccessAttempt }

func (s *stubAttempts) Create(_ context.Context, _ *sqlx.Tx, a db.AccessAttempt) error {
	s.rows = append(s.rows, a)
	return nil
}
func (s *stubAttempts) ReadByUserID(context.Context, *sqlx.Tx, string) ([]db.AccessAttempt, error) {
	return nil, nil
}
func (s *stubAttempts) ReadByIP(context.Context, *sqlx.Tx, string) ([]db.AccessAttempt, error) {
	return nil, nil
}
func (s *stubAttempts) CountFailedAttempts(context.Context, *sqlx.Tx, string, time.Time) (int, error) {
	return 0, nil
}
func (s *stubAttempts) CountLockoutAttempts(_ context.Context, _ *sqlx.Tx, key string, since time.Time) (int, error) {
	n := 0
	for _, a := range s.rows {
		if a.LockoutKey != nil && *a.LockoutKey == key && a.AttemptedAt.After(since) {
			n++
		}
	}
	return n, nil
}
func (s *stubAttempts) ReadPage(context.Context, *sqlx.Tx, *db.AccessAttemptCursor, int) ([]db.AccessAttempt, error) {
	return nil, nil
}
func (s *stubAttempts) last() db.AccessAttempt { return s.rows[len(s.rows)-1] }

type stubLocks struct{ until map[string]time.Time }

func (s *stubLocks) LockedUntil(_ context.Context, _ *sqlx.Tx, key string) (time.Time, error) {
	return s.until[key], nil
}
func (s *stubLocks) SetLockout(_ context.Context, _ *sqlx.Tx, key string, until time.Time) error {
	s.until[key] = until
	return nil
}
func (s *stubLocks) ClearLockout(_ context.Context, _ *sqlx.Tx, key string) (bool, error) {
	active := s.until[key].After(time.Now())
	if active {
		s.until[key] = time.Now()
	}
	return active, nil
}
func (s *stubLocks) ListActive(context.Context, *sqlx.Tx) ([]db.Lockout, error) { return nil, nil }
func (s *stubLocks) locked(key string) bool                                     { return s.until[key].After(time.Now()) }

func unsignedToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]string{"alg": "none"}) + "." + enc(claims) + "."
}

type authFixture struct {
	authenticator JWTAuthenticator
	attempts      *stubAttempts
	locks         *stubLocks
	ctx           context.Context
	scheme        *security.JWTScheme
	token         func(sub string, exp time.Time) string
}

const clientIP = "203.0.113.7"

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	// The signature check is not under test here; TestValidateTokenRefusesForgedTokens covers it.
	t.Setenv("JWT_ALG_NONE_SUPPORTED", "true")
	t.Setenv("DCS_ALLOW_DEV_TRUST", "true")
	var issuer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/jwks"})
	}))
	t.Cleanup(srv.Close)
	issuer = srv.URL

	validator, err := middleware.NewHydraJWTValidator(context.Background(), middleware.HydraJWTConfig{
		PublicIssuerURL: issuer,
		ClientID:        "dcs-client",
	})
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	f := &authFixture{attempts: &stubAttempts{}, locks: &stubLocks{until: map[string]time.Time{}}}
	f.authenticator = JWTAuthenticator{
		Validator:    validator,
		DB:           sqlx.NewDb(sql.OpenDB(nopConn{}), "postgres"),
		AAttemptRepo: f.attempts,
		LockRepo:     f.locks,
	}
	ctx := context.WithValue(context.Background(), middleware.IPKey, clientIP)
	ctx = context.WithValue(ctx, goa.ServiceKey, "template_repository")
	f.ctx = context.WithValue(ctx, goa.MethodKey, "search")
	f.scheme = &security.JWTScheme{RequiredScopes: []string{"Template Creator"}}
	f.token = func(sub string, exp time.Time) string {
		return unsignedToken(t, map[string]any{
			"iss":       issuer,
			"sub":       sub,
			"client_id": "dcs-client",
			"exp":       exp.Unix(),
			"ext":       map[string]any{"iss": "did:web:participant.example", "roles": []string{"Template Creator"}},
		})
	}
	return f
}

func (f *authFixture) call(token string) error {
	_, err := f.authenticator.JWTAuth(f.ctx, token, f.scheme)
	return err
}

// FR-UC-01-4: exactly the threshold of invalid credentials locks the client
// address out, and a valid credential from it is then refused with the
// prescribed message until the lock ends. Expired tokens do not count.
func TestLockoutRefusesAValidCredential(t *testing.T) {
	f := newAuthFixture(t)
	valid := f.token("did:example:holder", time.Now().Add(time.Hour))

	// A burst of requests that all carry the same just-expired token is how
	// every client meets the end of a token's lifetime. It is refused, but it
	// does not count toward a lockout.
	expired := f.token("did:example:holder", time.Now().Add(-time.Minute))
	for range 6 {
		if err := f.call(expired); err == nil || err.Error() != CredentialRefused {
			t.Fatalf("an expired credential got %v, not %q", err, CredentialRefused)
		}
	}
	if f.locks.locked(clientIP) {
		t.Fatal("expired access tokens locked the client out")
	}
	// UC-IF-18: the refused expired token names its holder in the log.
	if by := f.attempts.last().AttemptBy; by == nil || *by != "did:example:holder" {
		t.Fatalf("expired token attempt logged by %v, want the token subject", by)
	}
	if err := f.call(valid); err != nil {
		t.Fatalf("a valid credential was refused before any lockout: %v", err)
	}

	threshold := conf.LoginAttemptsThresholdInDuration()
	for i := 1; i <= threshold; i++ {
		if err := f.call("not-a-token"); err == nil || err.Error() != CredentialRefused {
			t.Fatalf("a malformed credential got %v, not %q", err, CredentialRefused)
		}
		if locked := f.locks.locked(clientIP); locked != (i == threshold) {
			t.Fatalf("after %d malformed credentials locked = %t", i, locked)
		}
	}

	err := f.call(valid)
	if err == nil || err.Error() != CredentialRefused {
		t.Fatalf("a valid credential during the lockout got %v, not %q", err, CredentialRefused)
	}
	if !f.locks.locked(clientIP) {
		t.Fatal("a valid credential cleared the lockout")
	}
}

// UC-IF-17: refusals during a lock do not extend it, and when the lock ends
// the failures before it do not lock the address again at the next failure.
func TestFailuresDuringLockDoNotExtendIt(t *testing.T) {
	f := newAuthFixture(t)
	for range conf.LoginAttemptsThresholdInDuration() {
		_ = f.call("not-a-token")
	}
	end := f.locks.until[clientIP]
	if !end.After(time.Now()) {
		t.Fatal("no lock after the threshold")
	}
	for range 10 {
		_ = f.call("not-a-token")
	}
	if !f.locks.until[clientIP].Equal(end) {
		t.Fatal("refusals during the lock moved its end")
	}

	// The lock ends.
	f.locks.until[clientIP] = time.Now()
	if err := f.call("not-a-token"); err == nil {
		t.Fatal("a malformed credential got through")
	}
	if f.locks.locked(clientIP) {
		t.Fatal("the first failure after the lock ended locked the address again")
	}
	if err := f.call(f.token("did:example:holder", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("a valid credential after the lock ended was refused: %v", err)
	}
}

// NFR-18: an administrator unlock lets valid credentials through again, and
// the failures before the unlock do not count afterwards.
func TestUnlockEndsTheLockAndItsCount(t *testing.T) {
	f := newAuthFixture(t)
	for range conf.LoginAttemptsThresholdInDuration() {
		_ = f.call("not-a-token")
	}
	if ok, _ := f.locks.ClearLockout(context.Background(), nil, clientIP); !ok {
		t.Fatal("no active lock to lift")
	}
	if err := f.call(f.token("did:example:holder", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("a valid credential after the unlock was refused: %v", err)
	}
	_ = f.call("not-a-token")
	if f.locks.locked(clientIP) {
		t.Fatal("failures before the unlock still counted")
	}
}

// UC-IF-19: the lockout refusal tells the client when it may try again.
func TestLockoutRefusalCarriesRetryAfter(t *testing.T) {
	f := newAuthFixture(t)
	for range conf.LoginAttemptsThresholdInDuration() {
		_ = f.call("not-a-token")
	}
	rec := httptest.NewRecorder()
	middleware.ResponseHeaders(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), middleware.IPKey, clientIP)
		ctx = context.WithValue(ctx, goa.ServiceKey, "template_repository")
		ctx = context.WithValue(ctx, goa.MethodKey, "search")
		if _, err := f.authenticator.JWTAuth(ctx, f.token("did:example:holder", time.Now().Add(time.Hour)), f.scheme); err == nil {
			t.Error("a valid credential got through during the lockout")
		}
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Retry-After"); got == "" || got == "0" {
		t.Fatalf("Retry-After = %q, want the seconds until the lock ends", got)
	}
}

// NFR-9: the request budget belongs to the validated caller. Two tokens
// of one holder share one budget, and requests whose token does not validate
// are not counted and write no counter row.
func TestRateLimitIsPerValidatedCaller(t *testing.T) {
	database := pgtest.Open(t)
	f := newAuthFixture(t)
	f.authenticator.Limiter = middleware.NewRateLimiter(database, 1)

	for i := range 50 {
		_ = f.call("garbage-" + string(rune('a'+i%26)))
	}
	var rows int
	if err := database.Get(&rows, `SELECT count(*) FROM api_rate_limit_windows`); err != nil || rows != 0 {
		t.Fatalf("invalid tokens wrote %d counter rows (%v), want 0", rows, err)
	}
	clear(f.locks.until)

	if err := f.call(f.token("did:example:holder", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("first request refused: %v", err)
	}
	var named interface{ ErrorName() string }
	err := f.call(f.token("did:example:holder", time.Now().Add(2*time.Hour)))
	if !errors.As(err, &named) || named.ErrorName() != "too_many_requests" {
		t.Fatalf("a second token of the same caller got %v, want too_many_requests", err)
	}
	if err := f.call(f.token("did:example:other", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("another caller was limited: %v", err)
	}
}

// NFR-4: the switch that turns off the token signature check is refused
// unless the stack declares itself a development or test stack.
func TestAlgNoneRequiresDevTrust(t *testing.T) {
	var issuer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/jwks"})
	}))
	t.Cleanup(srv.Close)
	issuer = srv.URL
	cfg := middleware.HydraJWTConfig{PublicIssuerURL: issuer, ClientID: "dcs-client"}

	t.Setenv("JWT_ALG_NONE_SUPPORTED", "true")
	t.Setenv("DCS_ALLOW_DEV_TRUST", "")
	if _, err := middleware.NewHydraJWTValidator(context.Background(), cfg); err == nil {
		t.Fatal("the validator started with signature checks off outside a dev stack")
	}
	t.Setenv("DCS_ALLOW_DEV_TRUST", "true")
	if _, err := middleware.NewHydraJWTValidator(context.Background(), cfg); err != nil {
		t.Fatalf("a dev stack was refused: %v", err)
	}
}
