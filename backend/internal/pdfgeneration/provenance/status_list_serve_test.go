package provenance_test

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/pdfgeneration/provenance/provenancetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheServedTokenHasTheShapeAVerifierRoutesOn: the media type is what
// SelectMechanismFromResponse routes on, and the header and claim shape is what
// the IETF handler then requires. A list that is correct but shaped differently
// is refused, and a refused list reads to a caller exactly like a revoked
// contract.
func TestTheServedTokenHasTheShapeAVerifierRoutesOn(t *testing.T) {
	list := provenancetest.NewSignedStatusList(t, 7)

	resp, err := http.Get(list.ListURI) //nolint:noctx // fetch of a local stub
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, provenance.StatusListMediaType, resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	parts := strings.Split(string(body), ".")
	require.Len(t, parts, 3, "a JWS compact serialization has three segments")

	header := decodeSegment(t, parts[0])
	assert.Equal(t, "ES256", header["alg"])
	assert.Equal(t, "statuslist+jwt", header["typ"])
	assert.Len(t, header["x5c"], 2)

	claims := decodeSegment(t, parts[1])
	assert.Equal(t, list.CredentialIssuer, claims["iss"])
	// `sub` must be the URI credentials name; the verifier refuses any difference.
	assert.Equal(t, list.ListURI, claims["sub"])
	assert.NotNil(t, claims["iat"])
	statusList, ok := claims["status_list"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 1, statusList["bits"])
	assert.NotEmpty(t, statusList["lst"])
}

// TestARevokedEntryReadsRevokedThroughTheOrdinaryVerifier is the point of the
// whole change: the served list goes through the same verifier, the same
// anchors and the same leaf binding as any other issuer's, and the bit the
// deployment set is the bit that comes back.
func TestARevokedEntryReadsRevokedThroughTheOrdinaryVerifier(t *testing.T) {
	list := provenancetest.NewSignedStatusList(t, 12)

	state, err := list.Verifier.State(context.Background(), list.Credential(12))
	require.NoError(t, err)
	assert.Equal(t, provenance.StatusRevoked, state)
	assert.True(t, list.Fetched())
}

func TestAClearEntryReadsActiveThroughTheOrdinaryVerifier(t *testing.T) {
	list := provenancetest.NewSignedStatusList(t, 12)

	state, err := list.Verifier.State(context.Background(), list.Credential(13))
	require.NoError(t, err)
	assert.Equal(t, provenance.StatusActive, state)
}

// A deployment issues its credentials under its did:web identity and serves
// their status list at its https origin. The list is bound to the credential by
// comparing the two identifiers as strings (ADR-34/-35), so the token has to
// name the identity that ISSUED them, not the origin it is served from —
// otherwise one deployment describes itself two ways and every revocation check
// of its own credentials reports "signed by an issuer other than the one it
// names", which a caller shows as an unknown state rather than as the
// misconfiguration it is.
func TestAListNamesTheIdentityThatIssuedTheCredentialsNotTheOriginItIsServedFrom(t *testing.T) {
	const issuerDID = "did:web:dcs.example.org"
	list := provenancetest.NewSignedStatusListIssuedBy(t, issuerDID, 12)

	require.NotEqual(t, list.IssuerURL, list.CredentialIssuer,
		"the point of this test is that the two identifiers differ, as they do in a deployment")

	revoked, err := list.Verifier.State(context.Background(), list.Credential(12))
	require.NoError(t, err)
	assert.Equal(t, provenance.StatusRevoked, revoked)

	active, err := list.Verifier.State(context.Background(), list.Credential(13))
	require.NoError(t, err)
	assert.Equal(t, provenance.StatusActive, active)
}

// TestAListSignedUnderAnUntrustedRootIsNotAReading is what replacing the
// unsigned fetch bought. Anyone able to answer the URL used to decide a
// contract's revocation state; now an answer nobody vouched for produces an
// error and the caller reports the state as unknown.
func TestAListSignedUnderAnUntrustedRootIsNotAReading(t *testing.T) {
	list := provenancetest.NewSignedStatusList(t, 12)
	stranger := provenance.NewCredentialStatusVerifier(
		provenancetest.VerifierAnchoring(x509.NewCertPool()))

	_, err := stranger.State(context.Background(), list.Credential(12))
	require.Error(t, err)
}

// TestAnUnconfiguredVerifierIsUnknownNotActive: a deployment that never
// configured status verification must not report every contract as in force.
func TestAnUnconfiguredVerifierIsUnknownNotActive(t *testing.T) {
	list := provenancetest.NewSignedStatusList(t)

	_, err := provenance.NewCredentialStatusVerifier(nil).State(context.Background(), list.Credential(1))
	require.Error(t, err)
}

// TestTheChainsRootIsWhatAVerifierHasToAnchor: a deployment reads its own anchor
// back out of the chain it signs with, because that root is minted per install
// and cannot be a committed fixture.
func TestTheChainsRootIsWhatAVerifierHasToAnchor(t *testing.T) {
	list := provenancetest.NewSignedStatusList(t)

	resp, err := http.Get(list.ListURI) //nolint:noctx // fetch of a local stub
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	header := decodeSegment(t, strings.Split(string(body), ".")[0])
	chain, ok := header["x5c"].([]any)
	require.True(t, ok)

	// RFC 7515 §4.7: leaf first, so the anchor is the last entry.
	last, err := base64.StdEncoding.DecodeString(chain[len(chain)-1].(string))
	require.NoError(t, err)
	assert.Equal(t, list.Root.Raw, last)
}

func decodeSegment(t *testing.T, segment string) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}
