package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	goa "goa.design/goa/v3/pkg"

	genauth "digital-contracting-service/gen/auth"
	"digital-contracting-service/internal/auth"
	authdb "digital-contracting-service/internal/auth/db"
	pg "digital-contracting-service/internal/auth/db/pq"
	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
)

const lockoutIssuer = "did:web:issuer.example"

func newP256Key(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// publicP256JWK is the public JWK of key.
func publicP256JWK(t *testing.T, key *ecdsa.PrivateKey) map[string]string {
	t.Helper()
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString
	return map[string]string{"crv": "P-256", "kty": "EC", "x": b64(pub[1:33]), "y": b64(pub[33:])}
}

// loginTrust trusts issuerKey as lockoutIssuer for login.
func loginTrust(t *testing.T, issuerKey *ecdsa.PrivateKey) *oid4vp.TrustConfig {
	t.Helper()
	jwks, err := json.Marshal(map[string]any{"keys": []any{publicP256JWK(t, issuerKey)}})
	if err != nil {
		t.Fatal(err)
	}
	return &oid4vp.TrustConfig{
		VCTs: []string{oid4vp.PoAVCT},
		Issuers: map[string]oid4vp.TrustedIssuer{lockoutIssuer: {
			Purposes: []oid4vp.Purpose{oid4vp.PurposeLogin}, Organizations: []string{"did:web:org.example"},
			Mechanism: oid4vp.MechanismJWKS, JWKS: jwks,
		}},
	}
}

// holderPresentation returns a vp_token whose credential, signed by
// issuerKey as lockoutIssuer, binds a fresh holder key, and that key as
// did:jwk. Its key binding JWT is not signed, so the presentation is refused.
func holderPresentation(t *testing.T, issuerKey *ecdsa.PrivateKey) (string, string) {
	t.Helper()
	holder := publicP256JWK(t, newP256Key(t))
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss":          lockoutIssuer,
		"vct":          oid4vp.PoAVCT,
		"iat":          time.Now().Add(-time.Minute).Unix(),
		"exp":          time.Now().Add(time.Hour).Unix(),
		"organization": "did:web:org.example",
		"roles":        []any{"Template Creator"},
		"cnf":          map[string]any{"jwk": holder},
	})
	token.Header["typ"] = sdjwt.CredentialTyp
	issuerJWT, err := token.SignedString(issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	vp, err := json.Marshal(map[string][]string{"q": {issuerJWT + "~e30.e30.e30"}})
	if err != nil {
		t.Fatal(err)
	}
	did, _ := json.Marshal(holder)
	return string(vp), "did:jwk:" + base64.RawURLEncoding.EncodeToString(did)
}

// UC-IF-2, UC-IF-3 (FR-UC-01-4): a refused login presentation gets the
// prescribed message, also as the status the login page shows, and repeated
// refused presentations of a credential a trusted issuer signed lock out the
// holder key it binds, not the address the wallet posts from.
func TestRepeatedInvalidPresentationsLockTheHolder(t *testing.T) {
	database := pgtest.Open(t)
	presentations := pg.NewPostgresPresentationAttemptRepo(database)
	attempts := &pg.PostgresAccessAttemptRepo{}
	issuerKey := newP256Key(t)
	svc := &authSvc{
		JWTAuthenticator: auth.NewJWTAuthenticator(nil, database, attempts, &pg.PostgresLockoutRepo{}),
		presentations:    presentations,
		dcqlQuery:        map[string]any{"credentials": []any{map[string]any{"id": "q"}}},
		trust:            loginTrust(t, issuerKey),
	}
	const walletIP = "198.51.100.20"
	ctx := context.WithValue(context.Background(), middleware.IPKey, walletIP)
	ctx = context.WithValue(ctx, goa.ServiceKey, genauth.ServiceName)
	ctx = context.WithValue(ctx, goa.MethodKey, "presentationCallback")

	present := func(vp string) (*authdb.PresentationAttempt, error) {
		t.Helper()
		state := rand.Text()
		if err := presentations.CreatePending(ctx, authdb.PresentationAttempt{PresentationState: state, Nonce: "n", ExpiresAt: time.Now().UTC().Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
		if err := presentations.SetHydraLoginChallenge(ctx, state, "challenge"); err != nil {
			t.Fatal(err)
		}
		_, err := svc.PresentationCallback(ctx, &genauth.PresentationCallbackPayload{State: state, VpToken: &vp})
		attempt, findErr := presentations.FindByPresentationState(ctx, state)
		if findErr != nil {
			t.Fatal(findErr)
		}
		return attempt, err
	}
	refused := func(attempt *authdb.PresentationAttempt, err error) {
		t.Helper()
		var named interface{ ErrorName() string }
		if !errors.As(err, &named) || named.ErrorName() != "unauthorized" || err.Error() != auth.CredentialRefused {
			t.Fatalf("refusal = %v, want unauthorized %q", err, auth.CredentialRefused)
		}
		if attempt.ErrorMessage == nil || *attempt.ErrorMessage != auth.CredentialRefused {
			t.Fatalf("status message = %v, want %q", attempt.ErrorMessage, auth.CredentialRefused)
		}
	}
	countFor := func(key string) int {
		t.Helper()
		var n int
		if err := database.Get(&n, `SELECT count(*) FROM access_attempts WHERE lockout_key = $1`, key); err != nil {
			t.Fatal(err)
		}
		return n
	}

	vp, did := holderPresentation(t, issuerKey)
	holderKey := "holder:" + did
	threshold := conf.LoginAttemptsThresholdInDuration()
	for i := 1; i <= threshold; i++ {
		refused(present(vp))
		if _, locked := svc.LockedUntil(ctx, holderKey); locked != (i == threshold) {
			t.Fatalf("after %d refused presentations the holder lock is %t", i, locked)
		}
	}
	if _, locked := svc.LockedUntil(ctx, walletIP); locked {
		t.Fatal("the wallet address was locked instead of the holder")
	}

	// A locked holder is refused before verification, and the refusal does
	// not count again.
	refused(present(vp))
	if n := countFor(holderKey); n != threshold {
		t.Fatalf("counted attempts for the locked holder = %d, want %d", n, threshold)
	}

	// Another holder posting from the same wallet address is not locked.
	other, otherDID := holderPresentation(t, issuerKey)
	refused(present(other))
	if n := countFor("holder:" + otherDID); n != 1 {
		t.Fatalf("the other holder's refusal counted %d times, want 1", n)
	}

	// UC-IF-3: a credential no trusted issuer signed can name any holder key.
	// Its refusals count for the posting address, never for the named holder.
	forged, victimDID := holderPresentation(t, newP256Key(t))
	refused(present(forged))
	if n := countFor("holder:" + victimDID); n != 0 {
		t.Fatalf("a forged credential counted %d refusals against the holder it names", n)
	}
	if n := countFor(walletIP); n != 1 {
		t.Fatalf("the forged credential counted %d refusals for the posting address, want 1", n)
	}
}

// NFR-18: the administrator unlock ends an active lock, is audited, and
// answers not_found when the key has no active lock.
func TestUnlockLiftsAnActiveLockout(t *testing.T) {
	database := pgtest.Open(t)
	locks := &pg.PostgresLockoutRepo{}
	svc := &authSvc{JWTAuthenticator: auth.NewJWTAuthenticator(nil, database, &pg.PostgresAccessAttemptRepo{}, locks)}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Sys. Administrator"}, "did:example:admin", "did:web:operator.example")
	if _, err := database.Exec(`INSERT INTO lockouts (lockout_key, locked_until) VALUES ('203.0.113.9', now() + interval '10 minutes')`); err != nil {
		t.Fatal(err)
	}

	listed, err := svc.Lockouts(ctx, &genauth.LockoutsPayload{})
	if err != nil || len(listed) != 1 || listed[0].Key != "203.0.113.9" {
		t.Fatalf("lockouts = %+v, %v", listed, err)
	}
	if err := svc.Unlock(ctx, &genauth.UnlockPayload{Key: "203.0.113.9"}); err != nil {
		t.Fatal(err)
	}
	if _, locked := svc.LockedUntil(ctx, "203.0.113.9"); locked {
		t.Fatal("the lock is still active after the unlock")
	}
	var events int
	if err := database.Get(&events, `SELECT count(*) FROM outbox_events WHERE event_type = 'LOCKOUT_LIFTED'`); err != nil || events != 1 {
		t.Fatalf("unlock audit events = %d, %v", events, err)
	}
	var named interface{ ErrorName() string }
	if err := svc.Unlock(ctx, &genauth.UnlockPayload{Key: "203.0.113.9"}); !errors.As(err, &named) || named.ErrorName() != "not_found" {
		t.Fatalf("second unlock = %v, want not_found", err)
	}
}
