package provenance

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/piprate/json-gold/ld"

	"digital-contracting-service/internal/base/safehttp"
)

// The JSON-LD contexts every lifecycle/summary VC proof canonicalizes against
// are embedded at compile time and preloaded into the document loader:
// RDFC-1.0 normalization runs on every PDF export and signature apply, and a
// default (remote-fetching, uncached) loader turns each of those into live
// w3id.org/w3.org HTTP round-trips — a runtime internet dependency that
// collapses under BDD-suite load and is unavailable in hermetic CI. The
// embedded copies are the published, versioned W3C documents.
//
// A context outside this set is REFUSED rather than fetched. The document being
// canonicalized can arrive from a peer — the signing summary embedded in a
// shipped PDF — so a remote fallback let a stranger name any URL and have this
// service fetch it, on an unauthenticated path, with no address policy. It also
// made verification non-deterministic: canonicalization would depend on what a
// remote server returned that day, so the same credential could verify now and
// fail later. A verifier that cannot expand a document deterministically has
// not verified it.
//
// DCS_VC_CONTEXT_ALLOWED names additional contexts a deployment genuinely needs
// (a real EUDI issuer, an XFSC schema). Those are fetched through safehttp, so
// even an allowed context cannot become a hop to an address that answers only
// because the request comes from here.
//
//go:embed contexts/credentials-v2.json contexts/data-integrity-v2.json
var vcContextFS embed.FS

var embeddedVCContexts = map[string]string{
	"https://www.w3.org/ns/credentials/v2":        "contexts/credentials-v2.json",
	"https://w3id.org/security/data-integrity/v2": "contexts/data-integrity-v2.json",
}

var (
	vcLoaderOnce sync.Once
	vcLoader     *ld.CachingDocumentLoader
)

// pinnedContextLoader answers only for contexts this deployment has agreed to.
// Anything else is an error, not a fetch.
type pinnedContextLoader struct {
	allowed map[string]bool
	remote  ld.DocumentLoader
}

func (l pinnedContextLoader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	if l.allowed[u] {
		return l.remote.LoadDocument(u)
	}
	return nil, ld.NewJsonLdError(ld.LoadingDocumentFailed, fmt.Sprintf(
		"JSON-LD context %q is not one this deployment verifies against; add it to DCS_VC_CONTEXT_ALLOWED if it is expected", u))
}

func allowedRemoteContexts() map[string]bool {
	allowed := map[string]bool{}
	for _, url := range strings.Split(os.Getenv("DCS_VC_CONTEXT_ALLOWED"), ",") {
		if url = strings.TrimSpace(url); url != "" {
			allowed[url] = true
		}
	}
	return allowed
}

// vcDocumentLoader returns the process-wide caching JSON-LD document loader,
// preloaded with the embedded VC contexts. Preloading is fatal on error: a
// missing or unparsable embedded context would otherwise silently degrade to
// remote fetching, which is exactly the failure mode this exists to remove.
func vcDocumentLoader() ld.DocumentLoader {
	vcLoaderOnce.Do(func() {
		vcLoader = ld.NewCachingDocumentLoader(pinnedContextLoader{
			allowed: allowedRemoteContexts(),
			remote:  ld.NewDefaultDocumentLoader(safehttp.Client(10*time.Second, safehttp.Policy{})),
		})
		for url, path := range embeddedVCContexts {
			raw, err := vcContextFS.ReadFile(path)
			if err != nil {
				panic(fmt.Sprintf("provenance: embedded VC context %s missing: %v", path, err))
			}
			doc, err := ld.DocumentFromReader(bytes.NewReader(raw))
			if err != nil {
				panic(fmt.Sprintf("provenance: embedded VC context %s unparsable: %v", path, err))
			}
			vcLoader.AddDocument(url, doc)
		}
	})
	return vcLoader
}
