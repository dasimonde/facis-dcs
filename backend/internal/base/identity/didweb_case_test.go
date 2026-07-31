package identity

import "testing"

// DNS names are case-insensitive, so two did:web identifiers differing only in
// the case of their authority name the same host. Everything derived from the
// authority has to agree on that: the peer-trust scenarios build a synthetic
// peer by varying one letter, and if the derived URLs differ as strings the
// trust gate compares the agreement credential's issuer against a different
// spelling and refuses a peer that is in fact itself.
func TestDIDWebAuthorityIsCaseNormalised(t *testing.T) {
	lower, lowerSegments, err := DIDWebPath("did:web:dcs-a.localhost%3A18080:tenant:Blue")
	if err != nil {
		t.Fatalf("lowercase identifier: %v", err)
	}
	varied, variedSegments, err := DIDWebPath("did:web:Dcs-A.localhost%3A18080:tenant:Blue")
	if err != nil {
		t.Fatalf("case-varied identifier: %v", err)
	}
	if lower != varied {
		t.Errorf("case-varied authority resolved to %q, not %q", varied, lower)
	}

	// Path segments are case-SENSITIVE: two instances can share a host and be
	// told apart only by their path.
	if len(variedSegments) != 2 || variedSegments[1] != "Blue" {
		t.Errorf("path segments were normalised: %v", variedSegments)
	}
	if len(lowerSegments) != 2 || lowerSegments[1] != "Blue" {
		t.Errorf("path segments were normalised: %v", lowerSegments)
	}
}
