package identity

import "testing"

// Normalising must never merge two identifiers that SameDIDWeb keeps apart:
// it is used to key the trust-policy lookup, so a collision there would let one
// issuer inherit another's entry.
func TestNormalizeAgreesWithSameDIDWeb(t *testing.T) {
	dids := []string{
		"did:web:h.example%3A1:x%3Ay",
		"did:web:h.example%3A1:x:y",
		"did:web:H.example%3A1:X",
		"did:web:h.example",
		"did:web:a.example:tenant:Blue",
	}
	for _, a := range dids {
		if !SameDIDWeb(a, NormalizeDIDWeb(a)) {
			t.Errorf("%s does not match its own normalisation %s", a, NormalizeDIDWeb(a))
		}
		for _, b := range dids {
			if a == b {
				continue
			}
			if NormalizeDIDWeb(a) == NormalizeDIDWeb(b) && !SameDIDWeb(a, b) {
				t.Errorf("%s and %s are different peers but normalise alike", a, b)
			}
		}
	}
}
