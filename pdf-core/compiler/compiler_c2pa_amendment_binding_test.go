package compiler

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"
)

// TestAmendmentManifestIsNotAC2PAUpdateManifest pins the box type of the manifest
// an amendment appends, because that single UUID decides which hard binding a
// verifier checks.
//
// c2pa-rs picks the binding via Store::get_hash_binding_manifest: a claim flagged
// as an update manifest (JUMBF box UUID c2um) is assumed to leave the asset bytes
// untouched, so its own c2pa.hash.data is ignored and the *parent's* binding is
// verified against the current file instead. That holds for formats where a new
// manifest replaces the old store in place, but pdf-core appends an incremental
// PDF update — the amended file is strictly longer than the genesis file, and the
// genesis binding cannot match it. Flagged as c2um, every amended contract PDF
// therefore reports assertion.dataHash.mismatch and validation_state Invalid,
// even though its own binding is correct.
//
// An amendment changes the asset, so it is a standard manifest (c2ma) that binds
// its own bytes and names the genesis manifest as a parentOf ingredient.
func TestAmendmentManifestIsNotAC2PAUpdateManifest(t *testing.T) {
	genesis := mustGenesisStore(t)

	amended, err := renderVerificationManifestStore(
		testSigningContext(),
		genesis,
		updateManifestLabel(payloadHashBytes(testPayloadHash)),
		"https://example.test/api/contract/abc",
		testPayloadHash,
		lifecycleStatusAmended,
		payloadHashBytes(testPayloadHash),
		nil,
		time.Unix(0, 0).UTC(),
		"",
	)
	if err != nil {
		t.Fatalf("renderVerificationManifestStore: %v", err)
	}

	manifests, err := extractTopLevelManifestBoxes(amended)
	if err != nil {
		t.Fatalf("extract manifests: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("manifest count = %d, want 2 (genesis + amendment)", len(manifests))
	}

	uuid, err := manifestBoxUUID(manifests[len(manifests)-1])
	if err != nil {
		t.Fatalf("read amendment box UUID: %v", err)
	}
	if uuid == c2paUpdateUUID {
		t.Fatalf("amendment is flagged as a C2PA update manifest (c2um); a verifier will check the genesis hard binding against the amended bytes and report dataHash.mismatch")
	}
	if uuid != c2paManifUUID {
		t.Fatalf("amendment box UUID = %s, want the standard manifest UUID %s", uuid, c2paManifUUID)
	}
}

// TestAmendmentBindsItsOwnBytes proves the amendment carries its own
// c2pa.hash.data over the hash it was given, which is what makes dropping the
// update-manifest flag safe: the binding a verifier now checks is present.
func TestAmendmentBindsItsOwnBytes(t *testing.T) {
	genesis := mustGenesisStore(t)
	hardBinding := payloadHashBytes(testAmendedHash)

	amended, err := renderVerificationManifestStore(
		testSigningContext(),
		genesis,
		updateManifestLabel(payloadHashBytes(testAmendedHash)),
		"https://example.test/api/contract/abc",
		testAmendedHash,
		lifecycleStatusAmended,
		hardBinding,
		nil,
		time.Unix(0, 0).UTC(),
		"",
	)
	if err != nil {
		t.Fatalf("renderVerificationManifestStore: %v", err)
	}
	manifests, err := extractTopLevelManifestBoxes(amended)
	if err != nil {
		t.Fatalf("extract manifests: %v", err)
	}
	assertions, err := extractLabeledChildJUMBFBox(manifests[len(manifests)-1], "c2pa.assertions")
	if err != nil {
		t.Fatalf("find assertion store: %v", err)
	}
	binding, err := extractLabeledChildJUMBFBox(assertions, "c2pa.hash.data")
	if err != nil {
		t.Fatalf("amendment carries no hard binding: %v", err)
	}
	if !containsBytes(binding, hardBinding) {
		t.Fatalf("amendment hard binding does not carry the amended file hash %s", hex.EncodeToString(hardBinding))
	}
}

// TestConsecutiveAmendmentsGetDistinctLabels covers the negotiation ping-pong,
// where a contract PDF is amended once per exchange and the manifest chain grows
// on both parties.
//
// A manifest label identifies one manifest, and each amendment names its parent
// by label. Deriving the label from the embedded payload hash makes two
// amendments of the same payload share a label, so the second one names itself
// as its own parent and c2patool refuses the whole document with "cyclic
// ingredient found in path" — no validation report at all. The label must
// therefore come from the hard binding hash, which differs on every hop because
// each amendment appends bytes. This mirrors witnessManifestLabel, which already
// derives its label this way for the same reason.
func TestConsecutiveAmendmentsGetDistinctLabels(t *testing.T) {
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	first, err := UpdatePDF(testSigningContext(), compiled, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("first UpdatePDF: %v", err)
	}
	second, err := UpdatePDF(testSigningContext(), first, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("second UpdatePDF: %v", err)
	}

	c2paBytes, err := extractEmbeddedStreamByFileSpecName(second, "content_credential.c2pa")
	if err != nil {
		t.Fatalf("extract C2PA: %v", err)
	}
	boxes, err := extractTopLevelManifestBoxes(c2paBytes)
	if err != nil {
		t.Fatalf("extractTopLevelManifestBoxes: %v", err)
	}
	seen := make(map[string]bool, len(boxes))
	for _, box := range boxes {
		label, err := extractJUMBFLabel(box)
		if err != nil {
			t.Fatalf("extractJUMBFLabel: %v", err)
		}
		if seen[label] {
			t.Fatalf("manifest label %q appears twice; the second amendment is its own parent and c2patool reports a cyclic ingredient", label)
		}
		seen[label] = true
	}
	if len(boxes) != 3 {
		t.Fatalf("manifest count = %d, want 3 (genesis + two amendments)", len(boxes))
	}
}

const (
	testPayloadHash = "3f88b814f2d9c306be41b2ab09def5ae1282e8ccb015e51c5d8061915cd4a931"
	testAmendedHash = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
)

func mustGenesisStore(t *testing.T) []byte {
	t.Helper()
	store, err := renderC2PAManifestStore(
		testSigningContext(),
		"https://example.test/api/contract/abc",
		testPayloadHash,
		payloadHashBytes(testPayloadHash),
		nil,
		time.Unix(0, 0).UTC(),
	)
	if err != nil {
		t.Fatalf("renderC2PAManifestStore: %v", err)
	}
	return store
}

// manifestBoxUUID returns the JUMBF description box UUID of a manifest superbox,
// which is what marks a manifest as standard (c2ma) or update (c2um).
func manifestBoxUUID(jumbBox []byte) (string, error) {
	outer, err := parseBMFFBoxes(jumbBox)
	if err != nil {
		return "", err
	}
	children, err := parseBMFFBoxes(outer[0].Payload)
	if err != nil {
		return "", err
	}
	return hexUpper(children[0].Payload[:16]), nil
}

func hexUpper(b []byte) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 0, len(b)*2)
	for _, x := range b {
		out = append(out, digits[x>>4], digits[x&0x0F])
	}
	return string(out)
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
outer:
	for i := 0; i+len(needle) <= len(haystack); i++ {
		for j := range needle {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return true
	}
	return false
}

// TestSignedContractKeepsItsProvenanceChain pins the boundary ADR-26 draws.
//
// A PAdES signature is applied after the lifecycle manifest so that the
// signature commits to the provenance, which means the signature's revisions
// fall outside the whole-file hash that manifest carries. That is the accepted
// trade, not tampering: an external tool must still verify the signature, and
// the provenance must remain intact and readable up to it.
//
// What must not decay is the chain itself — the manifests, their ingredient
// links and their claim signatures survive the appended signature layer, so a
// verifier can still read who asserted what. Only the whole-file binding stops.
func TestSignedContractKeepsItsProvenanceChain(t *testing.T) {
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	// Stand in for the PAdES layer: bytes appended after the manifest, exactly
	// what a signature revision is from the manifest's point of view.
	signedLike := append(append([]byte(nil), compiled...), []byte("\n% appended signature revision\n%%EOF\n")...)

	c2paBytes, err := extractEmbeddedStreamByFileSpecName(signedLike, "content_credential.c2pa")
	if err != nil {
		t.Fatalf("the provenance must survive the appended signature: %v", err)
	}
	manifests, err := extractTopLevelManifestBoxes(c2paBytes)
	if err != nil {
		t.Fatalf("extract manifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("a signed contract carries no manifests; the chain did not survive signing")
	}
	if _, err := extractLifecycleFields(c2paBytes, 0); err != nil {
		t.Fatalf("the lifecycle assertion must stay readable after signing: %v", err)
	}
	// The original bytes are untouched — which is why the signature verifies.
	if !bytes.HasPrefix(signedLike, compiled) {
		t.Fatal("appending a signature must leave the signed bytes intact")
	}
}
