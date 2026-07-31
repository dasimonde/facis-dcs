package provenance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureSigner struct {
	lastUnsigned json.RawMessage
}

func (s *captureSigner) CreateCredential(_ context.Context, unsignedVC json.RawMessage) (json.RawMessage, error) {
	s.lastUnsigned = append(json.RawMessage{}, unsignedVC...)
	return json.RawMessage(`{"proof":{"type":"Ed25519Signature2020"}}`), nil
}

func TestIssueLifecycleVC_IncludesInlineJSONLDContextAndSubjectID(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	assertion := NewLifecycleAssertion(
		"did:web:example.org:contracts:abc123",
		"f00dbabe",
		"active",
		"approved",
		"did:web:example.org:issuer",
		"",
		effectiveAt,
	)

	signer := &captureSigner{}
	_, _, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", testStatusRef, assertion)
	require.NoError(t, err)
	require.NotEmpty(t, signer.lastUnsigned)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))

	ctxList, ok := doc["@context"].([]interface{})
	require.True(t, ok)
	require.Len(t, ctxList, 3)
	// W3C VC Data Model 2.0: first context element MUST be the v2 URL.
	assert.Equal(t, "https://www.w3.org/ns/credentials/v2", ctxList[0], "@context[0] must be VC DM 2.0")
	assert.Equal(t, "https://w3id.org/security/data-integrity/v2", ctxList[1])

	inlineCtx, ok := ctxList[2].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://w3id.org/facis/dcs/ontology/v1#", inlineCtx["dcs"])
	assert.Equal(t, "dcs:contractId", inlineCtx["contract_id"])
	assert.Equal(t, "dcs:fileHash", inlineCtx["file_hash"])
	assert.Equal(t, "dcs:status", inlineCtx["status"])
	assert.Equal(t, "dcs:reason", inlineCtx["reason"])

	effectiveAtTerm, ok := inlineCtx["effective_at"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "dcs:effectiveAt", effectiveAtTerm["@id"])
	assert.Equal(t, "http://www.w3.org/2001/XMLSchema#dateTime", effectiveAtTerm["@type"])

	subject, ok := doc["credentialSubject"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, assertion.ContractID, subject["id"])
	assert.Equal(t, assertion.ContractID, subject["contract_id"])
}

// testStatusRef stands in for the entry a publisher allocated for the contract.
var testStatusRef = CredentialStatusRef{
	StatusListCredential: "http://statuslist/v1/tenants/default/status/1",
	Index:                4711,
}

func TestIssueLifecycleVC_CredentialStatusIncludedWhenStatusListURISet(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	contractID := "did:web:example.org:contracts:abc123"
	assertion := NewLifecycleAssertion(
		contractID, "f00dbabe", "active", "approved",
		"did:web:example.org:issuer", "", effectiveAt,
	)

	signer := &captureSigner{}
	_, _, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", testStatusRef, assertion)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))

	cs, ok := doc["credentialStatus"].(map[string]interface{})
	require.True(t, ok, "credentialStatus must be present in unsigned VC")
	// The advertised type must be the format the URI actually serves — the XFSC
	// token status list, not a W3C BitstringStatusListCredential (DCS-OR-C2PA-005).
	assert.Equal(t, "TokenStatusList", cs["type"])
	assert.Equal(t, "revocation", cs["statusPurpose"])

	// The credential advertises the entry the publisher allocated, verbatim:
	// nothing here re-derives it, so it cannot disagree with the entry a
	// revocation flips.
	assert.Equal(t, testStatusRef.StatusListCredential, cs["statusListCredential"])
	assert.Equal(t, fmt.Sprintf("%s#%d", testStatusRef.StatusListCredential, testStatusRef.Index), cs["id"])
	assert.Equal(t, fmt.Sprintf("%d", testStatusRef.Index), cs["statusListIndex"])
}

func TestIssueLifecycleVC_CredentialStatusOmittedWhenStatusListURIEmpty(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	assertion := NewLifecycleAssertion(
		"did:web:example.org:contracts:abc123", "f00dbabe", "active", "approved",
		"did:web:example.org:issuer", "", effectiveAt,
	)

	signer := &captureSigner{}
	_, _, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", CredentialStatusRef{}, assertion)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))
	_, hasCS := doc["credentialStatus"]
	assert.False(t, hasCS, "credentialStatus must be absent when no status list entry is given")
}

func TestIssueLifecycleVC_NormalizesNonURISubjectID(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC)
	assertion := NewLifecycleAssertion(
		"contract-123", // not an absolute URI
		"f00dbabe",
		"active",
		"approved",
		"did:web:example.org:issuer",
		"",
		effectiveAt,
	)

	signer := &captureSigner{}
	_, _, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", testStatusRef, assertion)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))
	subject, ok := doc["credentialSubject"].(map[string]interface{})
	require.True(t, ok)

	subjectID, ok := subject["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(subjectID, "urn:dcs:subject:"))
	assert.Equal(t, assertion.ContractID, subject["contract_id"])
}

// TestIssueLifecycleVC_VCDM2ValidFromNotIssuanceDate verifies the W3C VC DM 2.0
// field names: validFrom must be present, issuanceDate must be absent.
func TestIssueLifecycleVC_VCDM2ValidFromNotIssuanceDate(t *testing.T) {
	effectiveAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	assertion := NewLifecycleAssertion(
		"did:web:example.org:contracts:dm2",
		"f00dbabe",
		"active",
		"",
		"did:web:example.org:issuer",
		"",
		effectiveAt,
	)

	signer := &captureSigner{}
	_, _, err := IssueLifecycleVC(context.Background(), signer, "did:web:example.org:issuer", CredentialStatusRef{}, assertion)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(signer.lastUnsigned, &doc))

	// DM 2.0: validFrom replaces issuanceDate.
	assert.Contains(t, doc, "validFrom", "validFrom must be present (W3C VC DM 2.0)")
	assert.NotContains(t, doc, "issuanceDate", "issuanceDate must be absent in DM 2.0")
}
