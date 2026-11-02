package command

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// UC-IF-8 (DCS-IR-CI-01): a target receives the signed contract and its
// signatures, so its URL is https, or plain http only toward loopback and the
// hosts the deployment names.
func TestValidateTargetURLAcceptsOnlyHTTPSOrANamedPlainHost(t *testing.T) {
	t.Setenv("DCS_DIDWEB_INSECURE_HOSTS", "dcs-orce:1880")
	for _, accepted := range []string{
		"https://erp.example/deploy",
		"http://dcs-orce:1880/contract-target/deploy",
		"http://localhost:1880/deploy",
		"http://127.0.0.1:1880/deploy",
	} {
		require.NoError(t, ValidateTargetURL(accepted), accepted)
	}
	for _, refused := range []string{
		"http://erp.example/deploy",
		"http://dcs-orce:1881/deploy",
		"ftp://erp.example/deploy",
		"erp.example/deploy",
		"",
	} {
		require.ErrorIs(t, ValidateTargetURL(refused), ErrInvalidTargetURL, refused)
	}
}

// The deployment POST goes to the registered URL only: a redirect is not
// followed, and a plain-http URL to an unnamed host is not dialled.
func TestDeployToFollowsNoRedirectAndRefusesPlainHTTP(t *testing.T) {
	var redirected atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer elsewhere.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	defer target.Close()
	client := NewHTTPContractTargetClient()

	require.Error(t, client.DeployTo(context.Background(), target.URL, map[string]any{"did": "c1"}))
	require.Zero(t, redirected.Load(), "the deployment payload followed a redirect")

	require.ErrorIs(t, client.DeployTo(context.Background(), "http://erp.example/deploy", map[string]any{"did": "c1"}), ErrInvalidTargetURL)
}
