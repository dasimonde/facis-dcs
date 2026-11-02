package query

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// DCS-OR-C2PA-006: the banner is the lifecycle state the verified credential
// in the PDF asserts. The contract's own state stands in only when there is no
// verified credential to read it from.
func TestRunVerifyTakesTheBannerFromTheVerifiedCredential(t *testing.T) {
	key := testKey(t)
	statusList := newStatusListServer(t)
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": true,
		"vc_present":           true,
		// The credential asserts "active".
		"vc_bytes": base64.StdEncoding.EncodeToString(testLifecycleCredential(t, key, statusList.ListURI)),
	})

	for name, tc := range map[string]struct {
		resolve func(string) (*identity.DIDDocument, error)
		banner  string
	}{
		"verified credential": {func(string) (*identity.DIDDocument, error) { return testIssuerDocument(t, &key.PublicKey), nil }, "active"},
		"unverified credential": {func(string) (*identity.DIDDocument, error) {
			return nil, errors.New("dial tcp: connection refused")
		}, "draft"},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore,
				&provenance.CredentialVerifier{Resolve: tc.resolve}, statusList.Verifier, nil, SignerAnchors{}, "draft")
			if err != nil {
				t.Fatalf("runVerify: %v", err)
			}
			if result.LifecycleStatus == nil || *result.LifecycleStatus != tc.banner {
				t.Fatalf("lifecycle_status = %v, want %q", result.LifecycleStatus, tc.banner)
			}
		})
	}
}
