package dcstodcs

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"digital-contracting-service/internal/base/federation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validityNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

func credentialWithWindow(fields string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"type":["VerifiableCredential"]%s}`, fields))
}

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func TestRequireCredentialInForce_AcceptsACurrentWindow(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q,"validUntil":%q`,
		rfc3339(validityNow.Add(-time.Hour)), rfc3339(validityNow.Add(23*time.Hour))))
	require.NoError(t, requireCredentialInForce(credential, validityNow))
}

// The 1.1 spelling carries the same statement and is covered by the same proof,
// so a peer on that model version is not a peer with no window.
func TestRequireCredentialInForce_AcceptsTheVC11Spelling(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"issuanceDate":%q,"expirationDate":%q`,
		rfc3339(validityNow.Add(-time.Hour)), rfc3339(validityNow.Add(time.Hour))))
	require.NoError(t, requireCredentialInForce(credential, validityNow))
}

// The finding this closes: without an expiry the credential of a peer removed
// from the federation keeps verifying forever.
func TestRequireCredentialInForce_RefusesACredentialWithNoExpiry(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q`, rfc3339(validityNow)))
	err := requireCredentialInForce(credential, validityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no validUntil")
}

func TestRequireCredentialInForce_RefusesAnExpiredCredential(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q,"validUntil":%q`,
		rfc3339(validityNow.Add(-48*time.Hour)), rfc3339(validityNow.Add(-24*time.Hour))))
	err := requireCredentialInForce(credential, validityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired at")
}

func TestRequireCredentialInForce_RefusesACredentialNotYetInForce(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q,"validUntil":%q`,
		rfc3339(validityNow.Add(time.Hour)), rfc3339(validityNow.Add(2*time.Hour))))
	err := requireCredentialInForce(credential, validityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in force before")
}

// An expiry far enough out is no expiry, so requiring one would otherwise be
// satisfied by naming the next century.
func TestRequireCredentialInForce_RefusesAWindowLongerThanTheFederationPermits(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q,"validUntil":%q`,
		rfc3339(validityNow), rfc3339(validityNow.Add(federation.MaxAgreementCredentialLifetime+time.Hour))))
	err := requireCredentialInForce(credential, validityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "longer than")
}

// Omitting validFrom must not buy the peer an unbounded window: the credential
// is then in force since always, so its remaining lifetime is measured from now
// and the same federation bound applies.
func TestRequireCredentialInForce_RefusesAnUnboundedWindowWithNoValidFrom(t *testing.T) {
	credential := credentialWithWindow(`,"validUntil":"3000-01-01T00:00:00Z"`)
	err := requireCredentialInForce(credential, validityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "longer than")
}

// A short window with no start is still a short window.
func TestRequireCredentialInForce_AcceptsABoundedWindowWithNoValidFrom(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"validUntil":%q`, rfc3339(validityNow.Add(time.Hour))))
	require.NoError(t, requireCredentialInForce(credential, validityNow))
}

// Two independently operated instances have independently drifting clocks, so a
// credential minted a moment ago must not read as not yet in force.
func TestRequireCredentialInForce_ToleratesClockSkewAtBothEnds(t *testing.T) {
	justAhead := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q,"validUntil":%q`,
		rfc3339(validityNow.Add(time.Minute)), rfc3339(validityNow.Add(time.Hour))))
	require.NoError(t, requireCredentialInForce(justAhead, validityNow))

	justLapsed := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q,"validUntil":%q`,
		rfc3339(validityNow.Add(-time.Hour)), rfc3339(validityNow.Add(-time.Minute))))
	require.NoError(t, requireCredentialInForce(justLapsed, validityNow))
}

func TestRequireCredentialInForce_RefusesAWindowThatRunsBackwards(t *testing.T) {
	credential := credentialWithWindow(fmt.Sprintf(`,"validFrom":%q,"validUntil":%q`,
		rfc3339(validityNow), rfc3339(validityNow.Add(-time.Hour))))
	err := requireCredentialInForce(credential, validityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "precedes")
}

func TestRequireCredentialInForce_RefusesATimestampItCannotRead(t *testing.T) {
	credential := credentialWithWindow(`,"validUntil":"tomorrow"`)
	err := requireCredentialInForce(credential, validityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RFC3339")
}
