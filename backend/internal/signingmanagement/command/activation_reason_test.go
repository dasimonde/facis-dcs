package command

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

type reasonRecordingIssuer struct{ status, reason string }

func (r *reasonRecordingIssuer) IssueContractLifecycleVC(_ context.Context, _, _, status, reason, _ string, _ time.Time) (string, json.RawMessage, error) {
	r.status, r.reason = status, reason
	return "urn:vc:1", json.RawMessage(`{}`), nil
}

// DCS-OR-C2PA-003/-004: the "active" assertion stamped before signing records
// why the contract becomes active, the signature about to be applied, rather
// than a fixed text every contract shares.
func TestTheActiveStampRecordsTheSignatureAsItsReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/render/amendment":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pdf_base64":          base64.StdEncoding.EncodeToString([]byte("%PDF stamped")),
				"c2pa_sig_structures": []string{},
			})
		case "/c2pa/embed":
			_, _ = w.Write([]byte("%PDF stamped"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	issuer := &reasonRecordingIssuer{}

	_, _, err := stampLifecycleForSigning(context.Background(), "did:web:dcs.example:contract:1",
		activationReason("did:jwk:signer", "SignerOne"), []byte(`{}`), []byte("%PDF base"),
		pdfcore.New(srv.URL, func([]byte) ([]byte, error) { return make([]byte, 64), nil }), issuer, "did:web:dcs.example")
	if err != nil {
		t.Fatalf("stampLifecycleForSigning: %v", err)
	}
	if issuer.status != "active" || !strings.Contains(issuer.reason, "did:jwk:signer") || !strings.Contains(issuer.reason, "SignerOne") {
		t.Fatalf("active credential issued with status %q reason %q, want the signer and field as the reason", issuer.status, issuer.reason)
	}
}
