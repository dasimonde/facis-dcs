// JSON-LD document loader for the two W3C context URLs used in Data Integrity proofs.
// Fetch at runtime; on timeout or network error fall back to embedded files in contexts/.
package envelope

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/piprate/json-gold/ld"
)

//go:embed contexts/credentials-v2.jsonld contexts/data-integrity-v2.json contexts/ed25519-signature-2020-v1.jsonld
var embeddedContexts embed.FS

const (
	// VC Data Model v2 base @context (https://www.w3.org/ns/credentials/v2).
	// MUST be the first @context entry: https://www.w3.org/TR/vc-data-model-2.0/#dfn-context
	// Base context definition: https://www.w3.org/TR/vc-data-model-2.0/#base-context
	credentialsV2ContextURL = "https://www.w3.org/ns/credentials/v2"

	// Data Integrity v2 security @context (https://w3id.org/security/data-integrity/v2).
	// SHOULD be injected when securing a document: https://www.w3.org/TR/vc-data-integrity/#context-injection
	// Registered context hash: https://www.w3.org/TR/vc-data-integrity/#contexts-and-vocabularies
	// Proof RDFC hashing (credential + proof configuration):
	// https://www.w3.org/TR/vc-di-eddsa/#hashing-eddsa-rdfc-2022
	dataIntegrityV2ContextURL = "https://w3id.org/security/data-integrity/v2"

	// Ed25519Signature2020 JSON-LD @context.
	// Specification: https://w3.org/TR/vc-di-eddsa/#ed25519signature2020
	ed25519Signature2020ContextURL = "https://w3id.org/security/suites/ed25519-2020/v1"

	contextFetchTimeout = 10 * time.Second
)

var supportedContextFiles = map[string]string{
	credentialsV2ContextURL:        "contexts/credentials-v2.jsonld",
	dataIntegrityV2ContextURL:      "contexts/data-integrity-v2.json",
	ed25519Signature2020ContextURL: "contexts/ed25519-signature-2020-v1.jsonld",
}

// DefaultDocumentLoader resolves the context URLs above for RDF Dataset Canonicalization.
func DefaultDocumentLoader() ld.DocumentLoader {
	return &fetchingContextLoader{
		httpClient: &http.Client{Timeout: contextFetchTimeout},
		remote:     make(map[string]map[string]any),
	}
}

// EmbeddedDocumentLoader is an alias kept for callers that historically used only embeds.
func EmbeddedDocumentLoader() ld.DocumentLoader {
	return DefaultDocumentLoader()
}

type fetchingContextLoader struct {
	httpClient *http.Client
	mu         sync.Mutex
	remote     map[string]map[string]any
}

func (l *fetchingContextLoader) LoadDocument(url string) (*ld.RemoteDocument, error) {
	embedPath, ok := supportedContextFiles[url]
	if !ok {
		return nil, fmt.Errorf("unsupported context url %q", url)
	}

	if doc := l.cachedRemote(url); doc != nil {
		return &ld.RemoteDocument{DocumentURL: url, Document: doc}, nil
	}

	doc, err := fetchContextDocument(l.httpClient, url)
	if err == nil {
		l.storeRemote(url, doc)
		return &ld.RemoteDocument{DocumentURL: url, Document: doc}, nil
	}

	// On timeout or network error, fall back to embedded contexts/credentials-v2.jsonld
	// or contexts/data-integrity-v2.json (byte-identical to the online response).
	// See https://www.w3.org/TR/vc-data-integrity/#validating-contexts
	fallback, fbErr := loadEmbeddedContext(embedPath)
	if fbErr != nil {
		return nil, fmt.Errorf("load context %q: remote: %w; embedded: %v", url, err, fbErr)
	}
	return &ld.RemoteDocument{DocumentURL: url, Document: fallback}, nil
}

func (l *fetchingContextLoader) cachedRemote(url string) map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.remote[url]
}

func (l *fetchingContextLoader) storeRemote(url string, doc map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remote[url] = doc
}

// fetchContextDocument GETs a context URL (Accept: application/ld+json, 10s timeout).
func fetchContextDocument(client *http.Client, url string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/ld+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	body = bytesTrimSpace(body)
	if len(body) == 0 {
		return nil, fmt.Errorf("GET %s: empty body", url)
	}

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", url, err)
	}
	return doc, nil
}

func loadEmbeddedContext(path string) (map[string]any, error) {
	raw, err := embeddedContexts.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
