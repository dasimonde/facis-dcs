package hydra

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The status reference put in the login context at login has to reach the
// token session claims, because refresh reads it from the access token.
func TestConsentCarriesTheCredentialStatusIntoTheSession(t *testing.T) {
	statusRef := map[string]any{
		"iss":    "https://issuer.example",
		"status": map[string]any{"status_list": map[string]any{"idx": 7, "uri": "https://issuer.example/status/1"}},
	}
	var accepted consentAcceptReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"requested_scope": []string{"openid", "offline_access"},
				"context": map[string]any{
					"roles":               []string{"Template Creator"},
					"iss":                 "did:web:participant.example",
					CredentialStatusClaim: statusRef,
				},
			})
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&accepted); err != nil {
				t.Errorf("consent accept body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"redirect_to": "https://dcs.example/auth/callback?code=abc"})
		}
	}))
	t.Cleanup(srv.Close)

	client := New(Config{PublicIssuerURL: srv.URL, AdminURL: srv.URL})
	if _, err := client.AcceptConsent(context.Background(), "challenge"); err != nil {
		t.Fatalf("consent failed: %v", err)
	}

	for name, claims := range map[string]map[string]any{
		"access token": accepted.Session.AccessToken,
		"id token":     accepted.Session.IDToken,
	} {
		got, _ := json.Marshal(claims[CredentialStatusClaim])
		want, _ := json.Marshal(statusRef)
		if string(got) != string(want) {
			t.Fatalf("%s session claims carry %s as the credential status, want %s", name, got, want)
		}
	}
}
