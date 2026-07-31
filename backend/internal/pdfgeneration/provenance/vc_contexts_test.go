package provenance

import (
	"strings"
	"testing"
)

// The document canonicalized here can arrive from a peer, so an unknown context
// must be an error rather than a fetch: fetching let a stranger name any URL and
// have this service request it, and made verification depend on what a remote
// server happened to return.
func TestUnknownContextIsRefusedNotFetched(t *testing.T) {
	loader := pinnedContextLoader{allowed: map[string]bool{}}

	for _, url := range []string{
		"https://attacker.example/context.jsonld",
		"http://169.254.169.254/latest/meta-data/",
		"https://www.w3.org/ns/credentials/v3",
	} {
		if _, err := loader.LoadDocument(url); err == nil {
			t.Errorf("context %q was loaded rather than refused", url)
		} else if !strings.Contains(err.Error(), "not one this deployment verifies against") {
			t.Errorf("context %q refused for the wrong reason: %v", url, err)
		}
	}
}

// The embedded contexts every VC this system issues depends on are preloaded,
// so pinning costs nothing on the ordinary path.
func TestEmbeddedContextsResolveWithoutTheNetwork(t *testing.T) {
	loader := vcDocumentLoader()
	for url := range embeddedVCContexts {
		if _, err := loader.LoadDocument(url); err != nil {
			t.Errorf("embedded context %q did not resolve offline: %v", url, err)
		}
	}
}

func TestAllowListNamesAdditionalContexts(t *testing.T) {
	t.Setenv("DCS_VC_CONTEXT_ALLOWED", " https://issuer.example/ctx.jsonld , ")
	allowed := allowedRemoteContexts()
	if !allowed["https://issuer.example/ctx.jsonld"] {
		t.Error("a named context was not allowed")
	}
	if len(allowed) != 1 {
		t.Errorf("blank entries became allowed contexts: %v", allowed)
	}
}
