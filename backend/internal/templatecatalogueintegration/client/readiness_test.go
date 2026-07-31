package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCheckReadinessCallsHealthThenVerificationExactlyOnce(t *testing.T) {
	t.Parallel()

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case HealthEndpointPath:
			w.Header().Set("Content-Type", JSONContentType)
			_, _ = w.Write([]byte(`{"status":"UP"}`))
		case "/realms/fc" + keycloakTokenPath:
			w.Header().Set("Content-Type", JSONContentType)
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
		case VerificationEndpointPath:
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("Authorization = %q, want Bearer test-token", got)
			}
			if r.URL.Query().Get("verifySchema") != "true" ||
				r.URL.Query().Get("verifySemantics") != "true" {
				t.Errorf("verification query = %q", r.URL.RawQuery)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read verification body: %v", err)
			}
			if !bytes.Contains(body, []byte(`"issuer": "did:example:issuer"`)) ||
				!bytes.Contains(body, []byte(`LegalPerson`)) {
				t.Errorf("verification body is not the embedded semantic fixture: %s", body)
			}
			w.Header().Set("Content-Type", JSONContentType)
			_, _ = w.Write([]byte(`{
				"verificationTimestamp":"2026-07-27T12:00:00Z",
				"lifecycleStatus":"ACTIVE",
				"issuer":"did:example:issuer",
				"issuedDateTime":"2010-01-01T19:23:24Z",
				"baseClass":"Participant",
				"claims":{"https://w3id.org/gaia-x/2511#hasLegallyBindingName":"Example"}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	fc, err := NewFederatedCatalogueClient(Config{
		APIURL:           server.URL,
		KeycloakRealmURL: server.URL + "/realms/fc",
		ClientID:         "dcs",
		ClientSecret:     "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := fc.CheckReadiness(context.Background(), true); err != nil {
		t.Fatalf("CheckReadiness() error = %v", err)
	}

	want := []string{
		http.MethodGet + " " + HealthEndpointPath,
		http.MethodPost + " /realms/fc" + keycloakTokenPath,
		http.MethodPost + " " + VerificationEndpointPath,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestCheckReadinessFailsFastBeforeVerificationWhenHealthIsDown(t *testing.T) {
	t.Parallel()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", JSONContentType)
		_, _ = w.Write([]byte(`{"status":"DOWN"}`))
	}))
	t.Cleanup(server.Close)

	fc, err := NewFederatedCatalogueClient(Config{
		APIURL:           server.URL,
		KeycloakRealmURL: server.URL + "/realms/fc",
		ClientID:         "dcs",
		ClientSecret:     "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := fc.CheckReadiness(context.Background(), true); err == nil {
		t.Fatal("CheckReadiness() succeeded for DOWN health")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want exactly one health call", calls)
	}
}

func TestCheckReadinessRejectsTerminalVerificationResponseWithoutRetry(t *testing.T) {
	t.Parallel()

	verificationCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case HealthEndpointPath:
			_, _ = w.Write([]byte(`{"status":"UP"}`))
		case "/realms/fc" + keycloakTokenPath:
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
		case VerificationEndpointPath:
			verificationCalls++
			http.Error(w, "terminal", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	fc, err := NewFederatedCatalogueClient(Config{
		APIURL:           server.URL,
		KeycloakRealmURL: server.URL + "/realms/fc",
		ClientID:         "dcs",
		ClientSecret:     "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := fc.CheckReadiness(context.Background(), true); err == nil {
		t.Fatal("CheckReadiness() succeeded for terminal verification response")
	}
	if verificationCalls != 1 {
		t.Fatalf("verification calls = %d, want exactly one", verificationCalls)
	}
}

func TestCheckReadinessSkipsNativeHealthForRemoteCatalogue(t *testing.T) {
	t.Parallel()

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/realms/fc" + keycloakTokenPath:
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
		case VerificationEndpointPath:
			_, _ = w.Write([]byte(`{
				"verificationTimestamp":"2026-07-27T12:00:00Z",
				"lifecycleStatus":"ACTIVE",
				"issuer":"did:example:issuer",
				"issuedDateTime":"2010-01-01T19:23:24Z",
				"baseClass":"Participant",
				"claims":{"https://w3id.org/gaia-x/2511#hasLegallyBindingName":"Example"}
			}`))
		default:
			t.Errorf("unexpected remote catalogue call %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	fc, err := NewFederatedCatalogueClient(Config{
		APIURL:           server.URL,
		KeycloakRealmURL: server.URL + "/realms/fc",
		ClientID:         "dcs",
		ClientSecret:     "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fc.CheckReadiness(context.Background(), false); err != nil {
		t.Fatalf("CheckReadiness() error = %v", err)
	}

	want := []string{
		http.MethodPost + " /realms/fc" + keycloakTokenPath,
		http.MethodPost + " " + VerificationEndpointPath,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestCheckReadinessRejectsIncompleteSemanticResult(t *testing.T) {
	t.Parallel()

	verificationCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/fc" + keycloakTokenPath:
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
		case VerificationEndpointPath:
			verificationCalls++
			_, _ = w.Write([]byte(`{"issuer":"did:example:issuer"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	fc, err := NewFederatedCatalogueClient(Config{
		APIURL:           server.URL,
		KeycloakRealmURL: server.URL + "/realms/fc",
		ClientID:         "dcs",
		ClientSecret:     "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fc.CheckReadiness(context.Background(), false); err == nil {
		t.Fatal("CheckReadiness() accepted an incomplete semantic result")
	}
	if verificationCalls != 1 {
		t.Fatalf("verification calls = %d, want exactly one", verificationCalls)
	}
}
