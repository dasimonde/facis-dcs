package service

import (
	"context"
	"net/http"
	"os"
	"strings"

	"digital-contracting-service/internal/pathutil"
)

const apiPathPrefixEnv = "DCS_API_PATH"
const defaultAPIPathPrefix = ""

const (
	oauthStateCookieName = "oidc_state"
	idTokenCookieName    = "id_token"
)

// contextKey is a private type for context keys in this package.
type contextKey int

const (
	httpRequestKey    contextKey = iota
	refreshTokenKey   contextKey = iota
	responseWriterKey contextKey = iota
)

// RequestContextMiddleware injects the *http.Request and http.ResponseWriter
// into the context so that service implementations can access them.
// This is used by the Auth service to read cookies and set cookie headers.
func RequestContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), httpRequestKey, r)
		ctx = context.WithValue(ctx, responseWriterKey, w)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// HTTPRequestFromContext extracts the *http.Request from context.
func HTTPRequestFromContext(ctx context.Context) (*http.Request, bool) {
	r, ok := ctx.Value(httpRequestKey).(*http.Request)
	return r, ok
}

// ResponseWriterFromContext extracts the http.ResponseWriter from context.
func ResponseWriterFromContext(ctx context.Context) (http.ResponseWriter, bool) {
	w, ok := ctx.Value(responseWriterKey).(http.ResponseWriter)
	return w, ok
}

// SetRefreshTokenInContext stores the refresh token in context for the
// response encoder to pick up and set as a cookie.
// Since context is immutable, this uses the ResponseWriter to set the cookie
// header directly.
func SetRefreshTokenInContext(ctx context.Context, refreshToken string) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok || refreshToken == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Path:     authCookiePath(),
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})
}

// SetOAuthStateCookie stores OAuth state for the Hydra callback.
func SetOAuthStateCookie(ctx context.Context, state string) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok || state == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Path:     oauthStateCookiePath(),
		MaxAge:   10 * 60,
	})
}

func ReadOAuthStateCookie(ctx context.Context) (string, error) {
	r, ok := HTTPRequestFromContext(ctx)
	if !ok {
		return "", http.ErrNoCookie
	}
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func ClearOAuthStateCookie(ctx context.Context) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Path:     oauthStateCookiePath(),
		MaxAge:   -1,
	})
}

func oauthStateCookiePath() string {
	return pathutil.JoinPaths(apiPathPrefixEnv, defaultAPIPathPrefix, "/auth/callback")
}

func authCookiePath() string {
	return pathutil.JoinPaths(apiPathPrefixEnv, defaultAPIPathPrefix, "/auth")
}

// SetIDTokenCookie stores the Hydra id_token for RP-initiated logout (id_token_hint).
func SetIDTokenCookie(ctx context.Context, idToken string) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok || idToken == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     idTokenCookieName,
		Value:    idToken,
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Path:     authCookiePath(),
		MaxAge:   7 * 24 * 60 * 60,
	})
}

func readIDTokenCookie(ctx context.Context) string {
	r, ok := HTTPRequestFromContext(ctx)
	if !ok {
		return ""
	}
	cookie, err := r.Cookie(idTokenCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func ClearIDTokenCookie(ctx context.Context) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     idTokenCookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Path:     authCookiePath(),
		MaxAge:   -1,
	})
}

// ClearRefreshTokenCookie clears the refresh token cookie by setting MaxAge to -1.
func ClearRefreshTokenCookie(ctx context.Context) {
	w, ok := ResponseWriterFromContext(ctx)
	if !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HttpOnly: true,
		Secure:   authCookieSecure(),
		SameSite: http.SameSiteLaxMode,
		Path:     authCookiePath(),
		MaxAge:   -1, // Delete the cookie
	})
}

func authCookieSecure() bool {
	allowInsecureCookies := strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_INSECURE_COOKIES")), "true")

	return !allowInsecureCookies
}
