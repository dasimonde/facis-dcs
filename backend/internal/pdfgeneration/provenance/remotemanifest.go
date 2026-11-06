package provenance

import (
	"os"
	"strings"
)

// RemoteManifestURL builds the public C2PA remote-manifest URL for a contract
// DID (DCS-OR-C2PA-008): DCS_PUBLIC_URL + the C2PAService.GetManifest path
// (GET /c2pa/manifest/{contract_did}). The PDF carries this URL in the
// manifest's dcs.remote_manifests assertion and in the XMP dcterms:provenance
// property, so a verifier can get the manifest store when the embedded one is
// missing.
//
// When DCS_PUBLIC_URL is unset the path is returned on its own so the
// remote_manifests reference still points at the correct, host-relative
// endpoint. Returns "" for an empty DID (no manifest reference emitted).
func RemoteManifestURL(did string) string {
	if strings.TrimSpace(did) == "" {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("DCS_PUBLIC_URL")), "/")
	return base + "/c2pa/manifest/" + did
}
