package provenance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStatusEntry is the allocated entry the stub publishers hand back.
var stubStatusEntry = CredentialStatusRef{
	StatusListCredential: "http://statuslist/v1/tenants/default/status/1",
	Index:                4711,
}

// stubStatusListPublisher is a test double for StatusListPublisher.
type stubStatusListPublisher struct {
	publishCalled bool
	revokeCalled  bool
	failPublish   bool
}

func (s *stubStatusListPublisher) PublishStatus(_ context.Context, _, _, _ string, _ time.Time) (CredentialStatusRef, error) {
	s.publishCalled = true
	if s.failPublish {
		return CredentialStatusRef{}, errors.New("status list service unavailable")
	}
	return stubStatusEntry, nil
}

func (s *stubStatusListPublisher) RevokeStatus(_ context.Context, _ string) (CredentialStatusRef, error) {
	s.revokeCalled = true
	return stubStatusEntry, nil
}

// TestLocalVCIssuer_IssuesVCAndPublishesStatus verifies the happy path:
// status list is updated BEFORE the VC is signed, and a valid VC ID + bytes are returned.
func TestLocalVCIssuer_IssuesVCAndPublishesStatus(t *testing.T) {
	vcSigner := &captureSigner{}
	statusList := &stubStatusListPublisher{}
	issuer := NewLocalVCIssuer(vcSigner, "did:example:issuer", statusList)

	vcID, vcBytes, err := issuer.IssueContractLifecycleVC(
		context.Background(),
		"did:example:contract1",
		"aaabbbccc",
		"active",
		"approved",
		"did:example:authority",
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.True(t, statusList.publishCalled, "status list must be updated as part of VC issuance (DCS-OR-C2PA-005)")
	assert.NotEmpty(t, vcID, "vcID must be a non-empty URI")
	assert.Greater(t, len(vcBytes), 0, "vcBytes must be non-empty")
}

// TestLocalVCIssuer_VCIDIsURN verifies that the returned VC ID has the
// expected "urn:dcs:vc:" prefix so it can be stored as VCId in LifecycleAssertion.
func TestLocalVCIssuer_VCIDIsURN(t *testing.T) {
	issuer := NewLocalVCIssuer(&captureSigner{}, "did:example:issuer", &stubStatusListPublisher{})

	vcID, _, err := issuer.IssueContractLifecycleVC(
		context.Background(),
		"did:example:c1", "hash123", "draft", "", "did:example:auth",
		time.Now().UTC(),
	)
	require.NoError(t, err)
	assert.True(t, len(vcID) > len("urn:dcs:vc:"), "vcID must start with urn:dcs:vc: prefix")
	assert.Equal(t, "urn:dcs:vc:", vcID[:len("urn:dcs:vc:")])
}

// TestLocalVCIssuer_FailsHardWhenStatusListFails verifies the hard-fail policy
// (DCS-OR-C2PA-005, feedback_hard_fail_dependencies): if the status list service
// is unreachable, VC issuance must return an error — never silently skip.
func TestLocalVCIssuer_FailsHardWhenStatusListFails(t *testing.T) {
	vcSigner := &captureSigner{}
	statusList := &stubStatusListPublisher{failPublish: true}
	issuer := NewLocalVCIssuer(vcSigner, "did:example:issuer", statusList)

	_, _, err := issuer.IssueContractLifecycleVC(
		context.Background(),
		"did:example:contract1", "hash", "terminated", "", "did:example:auth",
		time.Now().UTC(),
	)
	require.Error(t, err, "VC issuance must fail when status list is unreachable")
	assert.Contains(t, err.Error(), "publish contract status to status list",
		"error must reference the status list publication step")
	assert.True(t, statusList.publishCalled, "PublishStatus must have been called before the failure")
}

// TestLocalVCIssuer_StatusListCalledBeforeVCSigning verifies ordering:
// the status list is always updated before VC signing to ensure the status
// list entry exists before any external system can verify the VC.
func TestLocalVCIssuer_StatusListCalledBeforeVCSigning(t *testing.T) {
	callOrder := []string{}

	trackingSigner := &trackingCaptureSigner{onSign: func() {
		callOrder = append(callOrder, "vc_sign")
	}}
	trackingStatus := &trackingStatusListPublisher{onPublish: func() {
		callOrder = append(callOrder, "status_publish")
	}}

	issuer := NewLocalVCIssuer(trackingSigner, "did:example:issuer", trackingStatus)
	_, _, err := issuer.IssueContractLifecycleVC(
		context.Background(),
		"did:example:c1", "hash", "active", "", "did:example:auth",
		time.Now().UTC(),
	)
	require.NoError(t, err)
	require.Len(t, callOrder, 2)
	assert.Equal(t, "status_publish", callOrder[0], "status list must be updated before VC signing")
	assert.Equal(t, "vc_sign", callOrder[1])
}

// trackingCaptureSigner records when CreateCredential is called.
type trackingCaptureSigner struct {
	onSign func()
}

func (s *trackingCaptureSigner) CreateCredential(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	if s.onSign != nil {
		s.onSign()
	}
	return json.RawMessage(`{"proof":{"type":"Ed25519Signature2020"}}`), nil
}

// trackingStatusListPublisher records when PublishStatus is called.
type trackingStatusListPublisher struct {
	onPublish func()
}

func (s *trackingStatusListPublisher) PublishStatus(_ context.Context, _, _, _ string, _ time.Time) (CredentialStatusRef, error) {
	if s.onPublish != nil {
		s.onPublish()
	}
	return stubStatusEntry, nil
}

func (s *trackingStatusListPublisher) RevokeStatus(_ context.Context, _ string) (CredentialStatusRef, error) {
	return stubStatusEntry, nil
}
