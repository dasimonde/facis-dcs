package dcstodcs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp"
)

// The summary carries the contract it attests. Without checking it, a genuine
// summary and Power of Attorney from one contract could be attached to a ship
// of another and recorded as a signature on it — the presentation itself has no
// audience or nonce this instance can check, so this is the only thing binding
// the evidence to the exchange it arrived in.
func TestSummaryMustAttestTheContractItIsShippedWith(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	err := gate.Check(testSignedParty, "urn:contract:a-different-one", verifier(),
		[]SignatoryPoA{poaFor(testSignedParty)})

	require.Error(t, err)
	assert.Contains(t, gateError(t, err).Error(), "attests a signature on contract")
	assert.Empty(t, seen, "evidence for another contract must be refused before the credential is verified")
}

// The matching case still passes, so the check cannot be satisfied vacuously.
func TestSummaryForThisContractIsAccepted(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	require.NoError(t, gate.Check(testSignedParty, testContract, verifier(),
		[]SignatoryPoA{poaFor(testSignedParty)}))
	require.Len(t, seen, 1)
}
