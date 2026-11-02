package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	goa "goa.design/goa/v3/pkg"

	pac "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth"
	authdb "digital-contracting-service/internal/auth/db"
	pg "digital-contracting-service/internal/auth/db/pq"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/migrations"
)

func TestAccessAttemptsPostgres(t *testing.T) {
	database := pgtest.Open(t)
	if err := migrations.Run(database); err != nil {
		t.Fatal(err)
	}
	for _, column := range []struct{ table, column string }{{"access_attempts", "outcome"}, {"pac_audit_runs", "snapshot_bytes"}} {
		var nullable string
		if err := database.Get(&nullable, `SELECT is_nullable FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND column_name=$2`, column.table, column.column); err != nil || nullable != "NO" {
			t.Fatalf("required column %+v: %s, %v", column, nullable, err)
		}
	}
	var plaintext int
	if err := database.Get(&plaintext, `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='pac_audit_runs' AND column_name IN ('response_bytes','response_json','receipt')`); err != nil || plaintext != 0 {
		t.Fatalf("plaintext columns=%d: %v", plaintext, err)
	}
	validator, token := signedTokens(t)
	attempts := &pg.PostgresAccessAttemptRepo{}
	locks := &pg.PostgresLockoutRepo{}
	service := &processAuditAndCompliancesrvc{DB: database, JWTAuthenticator: auth.NewJWTAuthenticator(validator, database, attempts, locks)}
	endpoints := pac.NewEndpoints(service)
	endpoints.Use(auth.AccessMetadataMiddleware)
	ctx := context.WithValue(context.Background(), goa.ServiceKey, pac.ServiceName)
	ctx = context.WithValue(ctx, middleware.IPKey, "192.0.2.60")
	call := func(method string, endpoint goa.Endpoint, payload any, want string) any {
		t.Helper()
		result, err := endpoint(context.WithValue(ctx, goa.MethodKey, method), payload)
		var named interface{ ErrorName() string }
		if want == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.As(err, &named) || named.ErrorName() != want {
			t.Fatalf("%s: want %s, got %v", method, want, err)
		}
		return result
	}
	assertLast := func(outcome string, success bool, total int) authdb.AccessAttempt {
		t.Helper()
		var rows []authdb.AccessAttempt
		if err := database.Select(&rows, "SELECT * FROM access_attempts ORDER BY id DESC"); err != nil {
			t.Fatal(err)
		}
		if len(rows) != total || rows[0].Outcome != outcome || rows[0].Success != success {
			t.Fatalf("want %d rows ending %s/%t, got %+v", total, outcome, success, rows)
		}
		return rows[0]
	}
	did, scope := "did:example:contract", "contracts"
	total := 0
	for _, role := range []userrole.UserRole{userrole.ContractCreator, userrole.ArchiveManager} {
		jwt := token(role, time.Now().Add(time.Hour))
		call("audit", endpoints.Audit, &pac.PACAuditRequest{Token: &jwt, Scope: scope, Did: &did, Justification: "Denial"}, "forbidden")
		total++
		row := assertLast(authdb.AccessOutcomeDenied, true, total)
		if row.Scope != scope || row.DID == nil || *row.DID != did || row.Justification != "Denial" || row.AttemptBy == nil || *row.AttemptBy != "did:example:holder" {
			t.Fatalf("lost metadata: %+v", row)
		}
	}
	archiveToken := token(userrole.ArchiveManager, time.Now().Add(time.Hour))
	call("audit_report", endpoints.AuditReport, &pac.AuditReportPayload{Token: &archiveToken, Scope: &scope, Justification: "Report denial"}, "forbidden")
	total++
	assertLast(authdb.AccessOutcomeDenied, true, total)
	for _, role := range []userrole.UserRole{userrole.ArchiveManager, userrole.SystemAuditor} {
		jwt := token(role, time.Now().Add(time.Hour))
		call("access_attempts", endpoints.AccessAttempts, &pac.AccessAttemptsPayload{Token: &jwt, Limit: 2, Justification: "Reader denial"}, "forbidden")
		total++
		assertLast(authdb.AccessOutcomeDenied, true, total)
	}
	tx, err := database.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	failed, err := attempts.CountLockoutAttempts(ctx, tx, "192.0.2.60", time.Now().Add(-conf.LoginLockoutDuration()))
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err != nil || failed != 0 {
		t.Fatalf("denials counted as credential failures: %d %v", failed, err)
	}
	// Only a token that is not genuine counts toward the lockout; an expired
	// one is the normal end of its lifetime (FR-UC-01-4).
	forged := token(userrole.Auditor, time.Now().Add(time.Hour)) + "x"
	for range conf.LoginAttemptsThresholdInDuration() {
		call("access_attempts", endpoints.AccessAttempts, &pac.AccessAttemptsPayload{Token: &forged, Limit: 2, Justification: "Forged credentials"}, "unauthorized")
		total++
		assertLast(authdb.AccessOutcomeFailed, false, total)
	}
	tx, err = database.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	localCount, countErr := attempts.CountLockoutAttempts(ctx, tx, "192.0.2.60", time.Now().In(time.FixedZone("UTC+2", 2*60*60)).Add(-conf.LoginLockoutDuration()))
	utcCount, utcErr := attempts.CountLockoutAttempts(ctx, tx, "192.0.2.60", time.Now().UTC().Add(-conf.LoginLockoutDuration()))
	if countErr != nil || utcErr != nil || localCount != conf.LoginAttemptsThresholdInDuration() || utcCount != localCount {
		t.Fatalf("failed credential counts differ: UTC+2=%d (%v), UTC=%d (%v)", localCount, countErr, utcCount, utcErr)
	}
	until, err := locks.LockedUntil(ctx, tx, "192.0.2.60")
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if locked := until.After(time.Now()); err != nil || !locked {
		t.Fatalf("credential failures did not lock IP: %t %v", locked, err)
	}
	jwt := token(userrole.Auditor, time.Now().Add(time.Hour))
	call("access_attempts", endpoints.AccessAttempts, &pac.AccessAttemptsPayload{Token: &jwt, Limit: 2, Justification: "Locked read"}, "unauthorized")
	total++
	assertLast(authdb.AccessOutcomeFailed, false, total)
	tx, err = database.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	if lifted, err := locks.ClearLockout(ctx, tx, "192.0.2.60"); err != nil || !lifted {
		t.Fatalf("unlock: %t %v", lifted, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	first := call("access_attempts", endpoints.AccessAttempts, &pac.AccessAttemptsPayload{Token: &jwt, Limit: 2, Justification: "Read"}, "").(*pac.PACAccessAttempts)
	total++
	assertLast(authdb.AccessOutcomeSuccess, true, total)
	if len(first.Attempts) != 2 || first.NextCursor == nil {
		t.Fatalf("missing first page: %+v", first)
	}
	next := call("access_attempts", endpoints.AccessAttempts, &pac.AccessAttemptsPayload{Token: &jwt, Limit: 2, Cursor: first.NextCursor, Justification: "Read next"}, "").(*pac.PACAccessAttempts)
	if len(next.Attempts) != 2 || next.Attempts[0].ID >= first.Attempts[1].ID {
		t.Fatalf("cursor repeated/skipped ordering: %+v then %+v", first.Attempts, next.Attempts)
	}
	call("access_attempts", endpoints.AccessAttempts, &pac.AccessAttemptsPayload{Token: &jwt, Limit: 2, Justification: " "}, "bad_request")
}

func signedTokens(t *testing.T) (*middleware.HydraJWTValidator, func(userrole.UserRole, time.Time) string) {
	t.Helper()
	t.Setenv("JWT_ALG_NONE_SUPPORTED", "false")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer *httptest.Server
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body any
		if r.URL.Path == "/.well-known/openid-configuration" {
			body = map[string]any{"issuer": issuer.URL, "jwks_uri": issuer.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}}
		} else {
			body = map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}}
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(issuer.Close)
	validator, err := middleware.NewHydraJWTValidator(context.Background(), middleware.HydraJWTConfig{PublicIssuerURL: issuer.URL, ClientID: "test-client"})
	if err != nil {
		t.Fatal(err)
	}
	return validator, func(role userrole.UserRole, expiry time.Time) string {
		t.Helper()
		claims, err := json.Marshal(map[string]any{"iss": issuer.URL, "sub": "did:example:holder", "aud": "test-client", "client_id": "test-client", "exp": expiry.Unix(), "iat": time.Now().Add(-time.Minute).Unix(), "ext": map[string]any{"iss": "did:example:participant", "roles": []string{role.String()}}})
		if err != nil {
			t.Fatal(err)
		}
		unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test-key"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
		digest := sha256.Sum256([]byte(unsigned))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
}
