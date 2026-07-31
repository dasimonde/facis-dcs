package identity

import "testing"

// Two spellings of one host are one peer; two paths under it are two peers.
// Callsites used to decide this individually and disagreed — the trust gate
// resolved a case-varied self-DID to this instance while the same-peer guard in
// front of it did not.
func TestSameDIDWeb(t *testing.T) {
	same := [][2]string{
		{"did:web:dcs-a.localhost%3A18080", "did:web:Dcs-A.localhost%3A18080"},
		{"did:web:a.example:tenant:blue", "did:web:A.EXAMPLE:tenant:blue"},
		{"did:web:a.example", "did:web:a.example"},
	}
	for _, pair := range same {
		if !SameDIDWeb(pair[0], pair[1]) {
			t.Errorf("%s and %s name the same host but were read as different peers", pair[0], pair[1])
		}
	}

	different := [][2]string{
		// Path segments are case-sensitive: folding them would merge two
		// instances that share a host and differ only by path.
		{"did:web:a.example:tenant:blue", "did:web:a.example:tenant:Blue"},
		{"did:web:a.example:tenant:blue", "did:web:a.example:tenant:green"},
		{"did:web:a.example:tenant", "did:web:a.example"},
		{"did:web:a.example", "did:web:b.example"},
		{"did:web:a.example", "https://a.example"},
		{"did:web:a.example%3A8080", "did:web:a.example%3A9090"},
	}
	for _, pair := range different {
		if SameDIDWeb(pair[0], pair[1]) {
			t.Errorf("%s and %s are different peers but were read as one", pair[0], pair[1])
		}
	}
}

func TestNormalizeDIDWeb(t *testing.T) {
	// The port keeps its %3A: a bare colon in the authority is a path segment
	// separator, so decoding it would turn one identifier into a different one.
	const want = "did:web:dcs-a.localhost%3A18080:tenant:Blue"
	if got := NormalizeDIDWeb("did:web:Dcs-A.localhost%3A18080:tenant:Blue"); got != want {
		t.Errorf("normalised to %q, want %q", got, want)
	}
	// Normalising is idempotent, and a normalised identifier still parses.
	if got := NormalizeDIDWeb(want); got != want {
		t.Errorf("re-normalising changed %q to %q", want, got)
	}
	if !SameDIDWeb(want, "did:web:Dcs-A.localhost%3A18080:tenant:Blue") {
		t.Error("a normalised identifier no longer matches the one it came from")
	}
	// Anything this resolver cannot parse is returned untouched rather than
	// mangled into something that looks canonical.
	if got := NormalizeDIDWeb("https://a.example"); got != "https://a.example" {
		t.Errorf("a non-did:web identifier was rewritten to %q", got)
	}
}
