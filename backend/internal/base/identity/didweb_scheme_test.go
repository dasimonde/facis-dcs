package identity

import (
	"slices"
	"testing"
)

// did:web is https. Loopback is the one exception the resolver makes on its own;
// an in-cluster Service publishing over plain http under a non-loopback name is
// an exception a deployment has to state, so that stating it cannot be mistaken
// for a fallback that applies everywhere.
func TestDIDWebSchemesAllowsHTTPOnlyWhereNamed(t *testing.T) {
	if got := DIDWebSchemes("dcs-orce:1880"); slices.Contains(got, "http") {
		t.Errorf("an unnamed non-loopback host resolved over http: %v", got)
	}

	t.Setenv("DCS_DIDWEB_INSECURE_HOSTS", "dcs-orce:1880, dcs-orce-mismatch:1880")

	for _, host := range []string{"dcs-orce:1880", "DCS-ORCE:1880", "dcs-orce-mismatch:1880"} {
		got := DIDWebSchemes(host)
		if !slices.Contains(got, "http") {
			t.Errorf("named host %q was not permitted http: %v", host, got)
		}
		if got[0] != "https" {
			t.Errorf("host %q tried %q before https: %v", host, got[0], got)
		}
	}

	// Naming one host must not carry any other along with it.
	for _, host := range []string{"dcs-orce", "dcs-orce:1881", "evil.example", "dcs-orce.evil.example:1880"} {
		if got := DIDWebSchemes(host); slices.Contains(got, "http") {
			t.Errorf("host %q was permitted http although it was never named: %v", host, got)
		}
	}
}
