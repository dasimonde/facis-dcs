package identity

import "strings"

// SameDIDWeb reports whether two did:web identifiers name the same peer.
//
// The authority is case-insensitive because DNS is, and because DIDWebPath
// normalises it — so a peer DID stored before that normalisation, or spelled
// differently by a counterparty, denotes the same host. Path segments are
// compared exactly: two instances can share a host and be told apart only by
// their path, so folding those would merge distinct peers.
//
// Comparing the raw strings instead leaves each callsite deciding, and the
// callsites disagreed: the trust gate resolved a case-varied self-DID to this
// instance while the same-peer guard in front of it did not.
func SameDIDWeb(a, b string) bool {
	if a == b {
		return true
	}
	aHost, aSegments, err := DIDWebPath(a)
	if err != nil {
		return false
	}
	bHost, bSegments, err := DIDWebPath(b)
	if err != nil {
		return false
	}
	if aHost != bHost || len(aSegments) != len(bSegments) {
		return false
	}
	for i := range aSegments {
		if aSegments[i] != bSegments[i] {
			return false
		}
	}
	return true
}

// NormalizeDIDWeb returns the canonical spelling of a did:web identifier, or the
// input unchanged when it is not one this resolver can parse.
func NormalizeDIDWeb(did string) string {
	if _, _, err := DIDWebPath(did); err != nil {
		return did
	}
	// Only the authority is rewritten, and the path segments are carried across
	// EXACTLY as written. DIDWebPath decodes segments, so re-emitting the decoded
	// forms would merge two distinct identifiers whenever a segment contained an
	// escape — did:web:h%3A1:x%3Ay and did:web:h%3A1:x:y are different peers and
	// must stay different strings.
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(did), "did:web:"), ":")
	// The port keeps its %3A: a bare colon in the authority is a segment
	// separator, so emitting it raw turns one identifier into another.
	// Lowercasing the authority also lowercases the port escape; %3A is the
	// canonical spelling, and both are accepted on the way in.
	parts[0] = strings.ReplaceAll(strings.ToLower(parts[0]), "%3a", "%3A")
	return "did:web:" + strings.Join(parts, ":")
}
