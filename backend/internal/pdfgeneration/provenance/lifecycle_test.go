package provenance

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleAssertion_AllFieldsPresent(t *testing.T) {
	effectiveAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	a := NewLifecycleAssertion(
		"did:example:contract123",
		"abc123hash",
		"draft",
		"initial creation",
		"did:example:authority",
		"urn:dcs:vc:vcid",
		effectiveAt,
	)

	raw, err := json.Marshal(a)
	require.NoError(t, err)

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))

	// DCS-OR-C2PA-003: all fields required.
	for _, field := range []string{
		"label", "contract_id", "file_hash",
		"status", "reason", "effective_at", "authority", "vc_id",
	} {
		assert.Contains(t, m, field, "field %q missing from lifecycle assertion JSON", field)
	}

	assert.Equal(t, lifecycleAssertionLabel, a.Label)
}

// TestMapCWEStateToC2PA_CWEUppercaseMappings verifies that every uppercase CWE state
// (as emitted by the CWE state machine) maps to the correct SRS C2PA state.
// This is the fix for Gap 4 (DCS-OR-C2PA-003 lifecycle vocabulary coverage).
//
// OFFERED/NEGOTIATION/SUBMITTED/REVIEWED/APPROVED all map to "draft"
// (APPROVED deliberately does NOT map to "active": approval alone does not
// make a contract binding), SIGNED/ACTIVE map to "active", REVOKED maps to
// "suspended", and the REJECTED/WITHDRAWN pre-signing terminal states map
// to "draft".
func TestMapCWEStateToC2PA_CWEUppercaseMappings(t *testing.T) {
	cases := []struct {
		cwe  string
		want string
	}{
		{"DRAFT", "draft"},
		{"OFFERED", "draft"},
		{"NEGOTIATION", "draft"},
		{"SUBMITTED", "draft"},
		{"REVIEWED", "draft"},
		{"APPROVED", "draft"},
		{"REJECTED", "draft"},
		{"WITHDRAWN", "draft"},
		{"SIGNED", "active"},
		{"ACTIVE", "active"},
		{"REVOKED", "suspended"},
		{"TERMINATED", "terminated"},
		{"EXPIRED", "expired"},
		{"SUSPENDED", "suspended"},
		{"REPLACED", "replaced"},
	}
	for _, tc := range cases {
		t.Run(tc.cwe, func(t *testing.T) {
			got, err := MapCWEStateToC2PA(tc.cwe)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got, "CWE state %q must map to SRS state %q", tc.cwe, tc.want)
		})
	}
}

func TestMapCWEStateToC2PA_UnknownStateFails(t *testing.T) {
	_, err := MapCWEStateToC2PA("UNKNOWN_FUTURE_STATE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported lifecycle state")
}

// TestMapCWEStateToC2PA_AllSRSStatesCovered verifies that the SRS-mandated states
// (DCS-OR-C2PA-003) are reachable from at least one input. "amended" is not
// produced by any CWE contract state (NEGOTIATION/REJECTED map to "draft");
// it remains reachable only via the lowercase SRS-vocabulary pass-through,
// which is exercised here too.
func TestMapCWEStateToC2PA_AllSRSStatesCovered(t *testing.T) {
	required := map[string]bool{
		"draft": false, "active": false, "amended": false,
		"suspended": false, "terminated": false, "expired": false, "replaced": false,
	}
	// Map a representative CWE input for each SRS state ("amended" via the
	// lowercase pass-through, since no CWE state maps to it anymore).
	inputs := []string{"DRAFT", "SIGNED", "amended", "SUSPENDED", "TERMINATED", "EXPIRED", "REPLACED"}
	for _, in := range inputs {
		got, err := MapCWEStateToC2PA(in)
		require.NoError(t, err)
		required[got] = true
	}
	for state, covered := range required {
		assert.True(t, covered, "SRS state %q must be reachable from at least one CWE input", state)
	}
}

func TestLifecycleAssertion_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	a := NewLifecycleAssertion(
		"did:example:c1", "hash1",
		"active", "", "did:example:auth", "",
		time.Now().UTC(),
	)

	raw, err := json.Marshal(a)
	require.NoError(t, err)

	// reason and vc_id are omitempty — absent when empty.
	assert.NotContains(t, string(raw), `"reason"`)
	assert.NotContains(t, string(raw), `"vc_id"`)
}

// A peer ships a PDF it has already signed, and the receiving instance's own
// workflow starts over at OFFERED. Recording the local state would file that
// signed artifact as "draft" — re-renderable — so the next local terminate or
// expiry would append a C2PA manifest onto the counterparty's signed bytes.
func TestArtifactC2PAStateFreezesAPeerSignedPDFTheLocalStateCallsADraft(t *testing.T) {
	signed := signedPDF

	for _, localState := range []string{"OFFERED", "NEGOTIATION", "APPROVED"} {
		state, err := ArtifactC2PAState(localState, signed)
		require.NoError(t, err)
		assert.True(t, IsFrozenC2PAState(state),
			"a PAdES-signed artifact received in %s must be frozen, got %q", localState, state)
		assert.Equal(t, "active", state)
	}
}

// The same receipt without a signature is an ordinary draft artifact: it must
// stay renderable, or this instance could never render its own first artifact
// for the contract.
func TestArtifactC2PAStateLeavesAnUnsignedReceiptRenderable(t *testing.T) {
	state, err := ArtifactC2PAState("OFFERED", []byte("%PDF-1.7\nno signature here\n"))

	require.NoError(t, err)
	assert.Equal(t, "draft", state)
	assert.False(t, IsFrozenC2PAState(state))
}

// A state that is already frozen says something the signature does not: a
// revocation ship lands in REVOKED, and its artifact is suspended, not active.
func TestArtifactC2PAStateKeepsAnAlreadyFrozenState(t *testing.T) {
	state, err := ArtifactC2PAState("REVOKED", signedPDF)

	require.NoError(t, err)
	assert.Equal(t, "suspended", state)
}

// signedPDF is the shape a PAdES signature value dictionary takes: /Type /Sig
// with the /ByteRange covering the file around its /Contents.
var signedPDF = []byte("%PDF-1.7\n12 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite" +
	" /SubFilter /ETSI.CAdES.detached /ByteRange [0 840 960 1200] /Contents <30820> >>\nendobj\n")

// clauseTextMentioningByteRange is an UNSIGNED PDF whose page content stream and
// JSON-LD attachment carry a clause discussing /ByteRange. Contract text reaches
// both verbatim, so it is author- and peer-controlled.
var clauseTextMentioningByteRange = []byte("%PDF-1.7\n4 0 obj\n<< /Length 62 >>\nstream\n" +
	"BT (This clause mentions /ByteRange for illustrative purposes) Tj ET\nendstream\nendobj\n")

func TestCarriesPAdESSignatureNeedsTheSignatureDictionaryNotJustAByteRange(t *testing.T) {
	assert.True(t, CarriesPAdESSignature(signedPDF))
	assert.False(t, CarriesPAdESSignature(clauseTextMentioningByteRange),
		"a clause discussing /ByteRange is not a signature")
	assert.False(t, CarriesPAdESSignature([]byte("%PDF-1.7\n1 0 obj\n<< >>\nendobj\n")))
	assert.True(t, CarriesPAdESSignature([]byte("<< /Type/Sig /ByteRange [0 1 2 3] >>")),
		"/Type/Sig without the separating space is the same dictionary entry")
	assert.False(t, CarriesPAdESSignature([]byte("<< /Type /SigFlags /ByteRange [0 1 2 3] >>")),
		"/SigFlags is not /Sig")
}

// A conforming reader resolves /Byte#52ange as /ByteRange, so a peer spelling it
// that way ships a genuinely signed PDF. Missing it files the signed artifact as
// a re-renderable draft, and the next local terminate amends the counterparty's
// signed bytes — the very failure the artifact-is-the-witness rule exists to
// prevent.
func TestCarriesPAdESSignatureResolvesNameEscapes(t *testing.T) {
	escaped := []byte("%PDF-1.7\n12 0 obj\n<< /#54ype /S#69g /Byte#52ange [0 840 960 1200] >>\nendobj\n")

	assert.True(t, CarriesPAdESSignature(escaped))
}

// The consequence at the receiving end: receivepdf stores pdf_c2pa_state from
// ArtifactC2PAState, IsFrozenC2PAState("active") is true, and a frozen artifact
// is served as-is forever. An unsigned peer offer whose text merely discusses
// /ByteRange must stay a draft, or its C2PA banner asserts an ACTIVE contract
// that is only OFFERED and no local transition ever corrects it.
func TestArtifactC2PAStateLeavesAnOfferMentioningByteRangeADraft(t *testing.T) {
	state, err := ArtifactC2PAState("OFFERED", clauseTextMentioningByteRange)

	require.NoError(t, err)
	assert.Equal(t, "draft", state)
	assert.False(t, IsFrozenC2PAState(state))
}
