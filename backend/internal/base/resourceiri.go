package base

import (
	"os"
	"strings"
)

// ResourceIRI builds the dereferenceable identity of a DCS resource: the
// instance's public origin (the same DCS_PUBLIC_URL the Semantic Hub
// anchors use) plus the resource's resolve route, e.g.
// {DCS_PUBLIC_URL}/contract/{key}. An already-absolute identifier (a full
// IRI, a did:, a urn:) passes through unchanged. Without a configured
// public URL the reference stays host-relative, mirroring
// semantichub.AnchorURL's convention.
func ResourceIRI(kind, key string) string {
	if strings.Contains(key, "://") || strings.HasPrefix(key, "did:") || strings.HasPrefix(key, "urn:") {
		return key
	}
	origin := strings.TrimRight(strings.TrimSpace(os.Getenv("DCS_PUBLIC_URL")), "/")
	return origin + "/" + kind + "/" + key
}

// ResourceKey extracts the system key from a resource IRI — the last path
// segment, by definition of the ResourceIRI scheme. Non-IRI identifiers
// (bare keys, did:, urn:) pass through unchanged.
func ResourceKey(iri string) string {
	if !strings.Contains(iri, "://") && !strings.HasPrefix(iri, "/") {
		return iri
	}
	trimmed := strings.TrimRight(iri, "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}
