package identity

import "testing"

func TestDIDWebPathSplitsAuthorityAndSegments(t *testing.T) {
	for _, tc := range []struct {
		did      string
		host     string
		segments []string
	}{
		{"did:web:example.com", "example.com", []string{}},
		{"did:web:example.com%3A8991", "example.com:8991", []string{}},
		{"did:web:example.com:tenant:b", "example.com", []string{"tenant", "b"}},
		{"did:web:localhost%3A18080:b", "localhost:18080", []string{"b"}},
	} {
		host, segments, err := DIDWebPath(tc.did)
		if err != nil {
			t.Fatalf("%s: %v", tc.did, err)
		}
		if host != tc.host {
			t.Errorf("%s: host = %q, want %q", tc.did, host, tc.host)
		}
		if len(segments) != len(tc.segments) {
			t.Fatalf("%s: segments = %v, want %v", tc.did, segments, tc.segments)
		}
		for i := range segments {
			if segments[i] != tc.segments[i] {
				t.Errorf("%s: segment %d = %q, want %q", tc.did, i, segments[i], tc.segments[i])
			}
		}
	}
}

// A bare authority resolves via /.well-known; an identifier with path segments
// resolves under those segments and must NOT use /.well-known, or every DID on
// one host collapses onto the same document.
func TestDIDWebDocumentPathFollowsSegments(t *testing.T) {
	if got := DIDWebDocumentPath(nil); got != "/.well-known/did.json" {
		t.Errorf("bare authority: got %q", got)
	}
	if got := DIDWebDocumentPath([]string{"b"}); got != "/b/did.json" {
		t.Errorf("single segment: got %q", got)
	}
	if got := DIDWebDocumentPath([]string{"tenant", "b"}); got != "/tenant/b/did.json" {
		t.Errorf("nested segments: got %q", got)
	}
}

// Two instances under one host must not be confusable: this is what the
// agreement-credential issuer check compares.
func TestDIDWebBaseURLDistinguishesInstancesOnOneHost(t *testing.T) {
	a := DIDWebBaseURL("https", "example.com", []string{"a"})
	b := DIDWebBaseURL("https", "example.com", []string{"b"})
	root := DIDWebBaseURL("https", "example.com", nil)
	if a == b || a == root || b == root {
		t.Fatalf("bases collided: a=%q b=%q root=%q", a, b, root)
	}
	if root != "https://example.com" {
		t.Errorf("root base = %q", root)
	}
}

func TestDIDWebPathRejectsMalformed(t *testing.T) {
	for _, did := range []string{"did:key:z6Mk", "did:web:", "did:web:example.com::b"} {
		if _, _, err := DIDWebPath(did); err == nil {
			t.Errorf("expected error for %q", did)
		}
	}
}

// The authority decides which server is asked for a peer's key material, and
// DIDWebBaseURL concatenates it into a URL unquoted. Decoding every escape
// there let the identifier write the path as well as the host — "%2F..%2F.."
// producing the authority "evil.example/../.." — so an identifier that passed
// a host check earlier in the flow could still aim the fetch elsewhere. %3A,
// the port separator, is the only escape did:web defines in an authority.
func TestDIDWebPathRefusesAnAuthorityThatIsNotOne(t *testing.T) {
	for _, did := range []string{
		"did:web:evil.example%2F..%2F..",          // path traversal, escaped
		"did:web:evil.example%2fadmin",            // same, lower-case escape
		"did:web:evil.example/../..",              // same, not even escaped
		"did:web:evil.example%3Fq=1",              // query
		"did:web:evil.example%23fragment",         // fragment
		"did:web:user%40evil.example",             // userinfo
		"did:web:evil.example%3A8080%3A9090",      // two ports
		"did:web:evil.example%20",                 // whitespace
		"did:web:evil.example%",                   // truncated escape
		"did:web:evil.example%3",                  // truncated escape
		"did:web:[::1]%3A8080",                    // bracketed literal
		"did:web:good.example%2f..%2fbad.example", // host swap
	} {
		host, segments, err := DIDWebPath(did)
		if err == nil {
			t.Errorf("%q resolved to authority %q (segments %v); an authority that is not a hostname must be refused", did, host, segments)
		}
	}

	// The one legitimate escape still works, in either case.
	for _, did := range []string{"did:web:example.com%3A8991", "did:web:example.com%3a8991"} {
		host, _, err := DIDWebPath(did)
		if err != nil || host != "example.com:8991" {
			t.Errorf("%q → %q, %v; the port separator must still decode", did, host, err)
		}
	}
}

// A path segment names a tenant; it does not get to rewrite the path it sits
// in, or two instances on one host stop being distinguishable.
func TestDIDWebPathRefusesSegmentsThatRewriteThePath(t *testing.T) {
	for _, did := range []string{
		"did:web:example.com:%2F..%2Fother",
		"did:web:example.com:..",
		"did:web:example.com:.",
		"did:web:example.com:a%2Fb",
		"did:web:example.com:a%23b",
	} {
		if host, segments, err := DIDWebPath(did); err == nil {
			t.Errorf("%q resolved to %q %v; a segment must not carry a separator", did, host, segments)
		}
	}
}

// DIDWebToHostname stays authority-only: certificate hostname verification
// checks the authority, not the path.
func TestDIDWebToHostnameIgnoresSegments(t *testing.T) {
	host, err := DIDWebToHostname("did:web:example.com%3A8991:tenant:b")
	if err != nil {
		t.Fatal(err)
	}
	if host != "example.com:8991" {
		t.Errorf("host = %q", host)
	}
}
