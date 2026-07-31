package provenance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// issuedCredentialStatus issues a lifecycle VC for one contract state and
// returns its id together with the credentialStatus it advertises.
func issuedCredentialStatus(t *testing.T, contractID string, status CredentialStatusRef, state string, at time.Time) (string, map[string]interface{}) {
	t.Helper()
	signer := &captureSigner{}
	assertion := NewLifecycleAssertion(contractID, "f00dbabe", state, "", "did:web:example.org:issuer", "", at)
	_, vcID, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", status, assertion)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))
	cs, ok := doc["credentialStatus"].(map[string]interface{})
	require.True(t, ok, "credentialStatus must be present")
	return vcID, cs
}

// TestLifecycleCredentialsOfOneContractShareTheContractsRevocationEntry pins the
// scope of the status list entry a lifecycle credential advertises. It is the
// CONTRACT's revocation bit, not a per-credential one, so every credential a
// contract accumulates — the draft, the active one the signature commits to, the
// terminated one — resolves the same index and revoking the contract invalidates
// all of them at once.
//
// That is the intent, not an oversight. The only revocation trigger in the
// system is a terminal contract state (PublishStatus below), which flips exactly
// one bit, the one allocated to the contract; there is no per-credential revoke
// anywhere. Giving each credential its own index would leave a terminated
// contract's superseded "draft" credential reading not-revoked forever, and a
// reader holding only that credential would conclude the contract is live.
func TestLifecycleCredentialsOfOneContractShareTheContractsRevocationEntry(t *testing.T) {
	const contractID = "did:web:example.org:contracts:abc123"

	var revokedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revokedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// One publisher across the contract's whole life: each lifecycle event asks
	// it where the contract's entry is, exactly as issuance does in production.
	p := newTestPublisher(srv.URL, "default")

	draftRef, err := p.PublishStatus(context.Background(), contractID, "draft", "", time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	activeRef, err := p.PublishStatus(context.Background(), contractID, "active", "", time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	draftID, draft := issuedCredentialStatus(t, contractID, draftRef, "draft", time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC))
	activeID, active := issuedCredentialStatus(t, contractID, activeRef, "active", time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))

	require.NotEqual(t, draftID, activeID, "each lifecycle event is its own credential")
	assert.Equal(t, draft["statusListIndex"], active["statusListIndex"],
		"both credentials must resolve the contract's single revocation entry")
	assert.Equal(t, draft["id"], active["id"])

	// The entry they name is the one a revocation actually flips.
	_, err = p.PublishStatus(context.Background(), contractID, "terminated", "", time.Now())
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(revokedPath, fmt.Sprintf("/revoke/%s", active["statusListIndex"])),
		"revocation POST %s must flip the entry the credentials advertise (%v)", revokedPath, active["statusListIndex"])
}

// TestDistinctContractsGetDistinctRevocationEntries is the other half of the
// scope: sharing is per contract and stops there, so one contract's termination
// must not read across to another's credentials.
func TestDistinctContractsGetDistinctRevocationEntries(t *testing.T) {
	p := newTestPublisher("http://statuslist", "default")

	aRef, err := p.PublishStatus(context.Background(), "did:web:example.org:contracts:a", "active", "", time.Now())
	require.NoError(t, err)
	bRef, err := p.PublishStatus(context.Background(), "did:web:example.org:contracts:b", "active", "", time.Now())
	require.NoError(t, err)

	_, a := issuedCredentialStatus(t, "did:web:example.org:contracts:a", aRef, "active", time.Now())
	_, b := issuedCredentialStatus(t, "did:web:example.org:contracts:b", bRef, "active", time.Now())

	assert.NotEqual(t, a["statusListIndex"], b["statusListIndex"])
}
