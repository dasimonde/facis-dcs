package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubMatcher records what SubmitSignature's content gate compares and returns a
// canned verdict.
type stubMatcher struct {
	submitted []byte
	reference []byte
	match     bool
	mismatch  string
	err       error
}

func (s *stubMatcher) MatchContent(_ context.Context, submitted, reference []byte) (bool, string, error) {
	s.submitted = submitted
	s.reference = reference
	return s.match, s.mismatch, s.err
}

// The gate compares the submission against the PINNED prepared bytes — not a
// fresh render — so acceptance never depends on reproducing a render.
func TestAssertPreparedContentComparesAgainstThePinnedDocument(t *testing.T) {
	m := &stubMatcher{match: true}
	submitted := []byte("%PDF submitted")
	prepared := []byte("%PDF pinned at prepare")

	require.NoError(t, assertPreparedContent(context.Background(), m, submitted, prepared, "Signature1"))
	require.Equal(t, submitted, m.submitted)
	require.Equal(t, prepared, m.reference)
}

// A submission whose visible pages no longer render the prepared document is
// REFUSED with a typed error naming the field and what diverged — a caller
// mapping this to a client rejection must not have to string-match a 500.
func TestAssertPreparedContentRefusesADivergentSubmission(t *testing.T) {
	m := &stubMatcher{
		match:    false,
		mismatch: `page 1 content does not match compiled output (at byte 412: candidate="(Substituted clause"...)`,
	}

	err := assertPreparedContent(context.Background(), m, []byte("%PDF tampered"), []byte("%PDF pinned"), "Signature1")
	require.ErrorIs(t, err, ErrContentMismatch)
	require.Contains(t, err.Error(), "Signature1")
	require.Contains(t, err.Error(), "page 1 content does not match")
	require.Contains(t, err.Error(), "Substituted clause")
}

// An unreachable or failing comparison refuses the submission too: a signature
// is accepted only on a positive match, never on the absence of a negative.
func TestAssertPreparedContentRefusesWhenTheComparisonFails(t *testing.T) {
	m := &stubMatcher{err: errors.New("pdf-core unreachable")}

	err := assertPreparedContent(context.Background(), m, []byte("a"), []byte("b"), "Signature1")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrContentMismatch)
	require.Contains(t, err.Error(), "pdf-core unreachable")
}

// stubExtractor answers the payload gate with a canned attachment per PDF and
// records which documents it was asked about.
type stubExtractor struct {
	payloads map[string][]byte
	asked    []string
	err      error
}

func (s *stubExtractor) ExtractPayload(_ context.Context, pdf []byte) ([]byte, error) {
	s.asked = append(s.asked, string(pdf))
	if s.err != nil {
		return nil, s.err
	}
	return s.payloads[string(pdf)], nil
}

// A legitimate submission carries the same attachment it was handed — a PAdES
// revision supersedes the catalog, the page and the signature objects, never the
// embedded-file object — so the gate compares the submission's attachment against
// the PINNED document's own attachment and passes.
func TestAssertPreparedPayloadAcceptsAnUnchangedAttachment(t *testing.T) {
	submitted := []byte("%PDF prepared + PAdES revision")
	prepared := []byte("%PDF pinned at prepare")
	payload := []byte(`{"@id":"urn:contract:1","dcs:title":"As prepared"}`)
	e := &stubExtractor{payloads: map[string][]byte{
		string(submitted): payload,
		string(prepared):  payload,
	}}

	require.NoError(t, assertPreparedPayload(context.Background(), e, submitted, prepared, "Signature1"))
	require.Equal(t, []string{string(submitted), string(prepared)}, e.asked)
}

// The gap this closes: the visible pages are untouched — the content gate is
// blind to this — while the machine-readable contract every downstream decision
// reads has been substituted. It is REFUSED, under its own typed error so a
// caller can tell the two tampering shapes apart.
func TestAssertPreparedPayloadRefusesASubstitutedAttachment(t *testing.T) {
	submitted := []byte("%PDF prepared + revision superseding the attachment")
	prepared := []byte("%PDF pinned at prepare")
	e := &stubExtractor{payloads: map[string][]byte{
		string(submitted): []byte(`{"@id":"urn:contract:1","dcs:title":"Substituted"}`),
		string(prepared):  []byte(`{"@id":"urn:contract:1","dcs:title":"As prepared"}`),
	}}

	err := assertPreparedPayload(context.Background(), e, submitted, prepared, "Signature1")
	require.ErrorIs(t, err, ErrPayloadMismatch)
	require.NotErrorIs(t, err, ErrContentMismatch)
	require.Contains(t, err.Error(), "Signature1")
	// The refusal names both digests, and neither the prepared nor the
	// substituted document text.
	require.Contains(t, err.Error(), "submitted payload sha256")
	require.Contains(t, err.Error(), "prepared payload sha256")
	require.NotContains(t, err.Error(), "Substituted")
}

// The payload gate fails closed exactly as the content gate does: an extraction
// that cannot run is a refusal, not a pass.
func TestAssertPreparedPayloadRefusesWhenTheExtractionFails(t *testing.T) {
	e := &stubExtractor{err: errors.New("pdf-core unreachable")}

	err := assertPreparedPayload(context.Background(), e, []byte("a"), []byte("b"), "Signature1")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrPayloadMismatch)
	require.Contains(t, err.Error(), "pdf-core unreachable")
}

// A document with nothing extractable is refused rather than compared: two empty
// attachments are trivially equal, which would accept a submission that carries
// no machine-readable contract at all.
func TestAssertPreparedPayloadRefusesAnAbsentAttachment(t *testing.T) {
	e := &stubExtractor{payloads: map[string][]byte{}}

	err := assertPreparedPayload(context.Background(), e, []byte("a"), []byte("b"), "Signature1")
	require.ErrorIs(t, err, ErrPayloadMismatch)
	require.Contains(t, err.Error(), "no machine-readable contract")
}
