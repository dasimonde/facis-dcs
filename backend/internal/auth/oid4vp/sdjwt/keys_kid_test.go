package sdjwt

import "testing"

// A DID document names one key twice — the verification method's DID URL and
// the JWK's own kid inside it — and an issuer may sign under either. The
// resolver keeps one entry per verification method, so the lookup is what has
// to accept both names.
func TestKidNamesKey(t *testing.T) {
	const didURL = "did:web:localhost%3A8991#dev-key-1"

	const iss = "did:web:localhost%3A8991"

	for _, tc := range []struct {
		name          string
		jwkKid        string
		credentialKid string
		want          bool
	}{
		{"identical", "dev-key-1", "dev-key-1", true},
		{"credential cites the did url, jwk carries the fragment", "dev-key-1", didURL, true},
		{"jwk carries the did url, credential cites the fragment", didURL, "dev-key-1", true},
		{"different fragments", "dcs-vc", didURL, false},
		{"a did url from another document", "dev-key-1", "did:web:elsewhere.example#dev-key-1", false},
		{"two did urls differing only by document", "did:web:elsewhere.example#dev-key-1", didURL, false},
		{"empty fragment names nothing", "", "did:web:localhost%3A8991#", false},
		{"empty jwk kid", "", "dev-key-1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := kidNamesKey(tc.jwkKid, tc.credentialKid, iss); got != tc.want {
				t.Errorf("kidNamesKey(%q, %q) = %v, want %v", tc.jwkKid, tc.credentialKid, got, tc.want)
			}
		})
	}
}
