package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeFrontendSetsFramingAndCSPHeaders(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "<html></html>", "app.js": "1"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	noIndex := t.TempDir()

	for _, tc := range []struct{ dir, path string }{
		{dir, "/ui/"},                 // index.html
		{dir, "/ui/contracts/42"},     // client route, index.html fallback
		{dir, "/ui/app.js"},           // asset
		{noIndex, "/ui/contracts/42"}, // 404
	} {
		path := tc.path
		rec := httptest.NewRecorder()
		serveFrontend(rec, httptest.NewRequest(http.MethodGet, path, nil), tc.dir, "/ui", "/api")

		if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q, want DENY", path, got)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		for _, directive := range []string{"frame-ancestors 'none'", "script-src 'self'", "object-src 'none'"} {
			if !strings.Contains(csp, directive) {
				t.Errorf("%s: Content-Security-Policy %q lacks %q", path, csp, directive)
			}
		}
	}
}

// The deployment settings reach the UI without a write to the image: the
// filesystem of the backend container is read-only (DCS-NFR-SEC-05).
func TestServeFrontendWritesDeploymentSettingsAtServeTime(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"index.html": `<base href="__DCS_UI_BASE_PATH__" /><script src="config.js"></script>`,
		"config.js":  "window.DCS_CONFIG = undefined\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	for _, tc := range []struct{ path, want string }{
		{"/dcs/ui/config.js", `window.DCS_CONFIG = { API_BASE_URL: "/dcs/api" }`},
		{"/dcs/ui/", `<base href="/dcs/ui/" />`},
		{"/dcs/ui/index.html", `<base href="/dcs/ui/" />`},
		{"/dcs/ui/contracts/42", `<base href="/dcs/ui/" />`},
	} {
		rec := httptest.NewRecorder()
		serveFrontend(rec, httptest.NewRequest(http.MethodGet, tc.path, nil), dir, "/dcs/ui", "/dcs/api")
		if body := rec.Body.String(); rec.Code != http.StatusOK || !strings.Contains(body, tc.want) {
			t.Errorf("%s: %d %q, want %q", tc.path, rec.Code, body, tc.want)
		}
	}
	for name, body := range files {
		if got, _ := os.ReadFile(filepath.Join(dir, name)); string(got) != body {
			t.Errorf("%s changed on disk: %q", name, got)
		}
	}
}
