package provenance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/tsa"
)

// fakeTimestamper records what it was asked to timestamp.
type fakeTimestamper struct {
	data []byte
	err  error
}

func (f *fakeTimestamper) Enabled() bool { return true }

func (f *fakeTimestamper) TimestampBytes(_ context.Context, data []byte) (*tsa.Receipt, error) {
	f.data = data
	if f.err != nil {
		return nil, f.err
	}
	return &tsa.Receipt{Token: "MIIBtoken", TokenEncoding: "base64"}, nil
}

func issuedSubject(t *testing.T, timestamper Timestamper) (map[string]any, []string) {
	t.Helper()
	signer := &captureSigner{}
	issuer := NewLocalVCIssuer(signer, "did:example:issuer", &stubStatusListPublisher{}).WithTimestamper(timestamper)
	_, _, err := issuer.IssueContractLifecycleVC(context.Background(), "did:example:contract1", "aaabbbccc",
		"terminated", "ended by the parties", "did:example:authority", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	var vc struct {
		Context           []any          `json:"@context"`
		CredentialSubject map[string]any `json:"credentialSubject"`
	}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &vc))
	terms, _ := json.Marshal(vc.Context)
	return vc.CredentialSubject, []string{string(terms)}
}

// DCS-OR-C2PA-009: a lifecycle credential carries an RFC 3161 timestamp token
// over its assertion — contract_id, file_hash, status, reason, effective_at —
// inside the signed subject, as a term its context defines.
func TestALifecycleCredentialCarriesATimestampOverItsAssertion(t *testing.T) {
	timestamper := &fakeTimestamper{}
	subject, context := issuedSubject(t, timestamper)

	assert.Equal(t, "MIIBtoken", subject["rfc3161_timestamp"])
	assert.Contains(t, context[0], `"rfc3161_timestamp":"dcs:rfc3161Timestamp"`)
	stamped := string(timestamper.data)
	for _, field := range []string{"did:example:contract1", "aaabbbccc", "terminated", "ended by the parties", "2026-05-01T00:00:00Z"} {
		assert.True(t, strings.Contains(stamped, field), "the timestamp must cover %q, covered %s", field, stamped)
	}
}

// The timestamp is a SHOULD: a TSA that does not answer leaves the credential
// without a token instead of stopping the lifecycle event.
func TestALifecycleCredentialIsIssuedWhenTheTSADoesNotAnswer(t *testing.T) {
	subject, _ := issuedSubject(t, &fakeTimestamper{err: errors.New("tsa unreachable")})
	_, present := subject["rfc3161_timestamp"]
	assert.False(t, present)
}
