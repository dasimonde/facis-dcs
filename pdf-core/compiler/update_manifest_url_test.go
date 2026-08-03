package compiler

import (
	"bytes"
	"testing"
	"time"
)

// TestUpdatePDFWithManifestURLEmbedsXMPProvenance verifies that
// UpdatePDFWithOptions references the remote manifest URL (DCS-OR-C2PA-008
// AC3) via:
//  1. the C2PA-normative XMP dcterms:provenance link, AND
//  2. a normal "dcs.remote_manifests" C2PA assertion (mirroring
//     dcs.lifecycle) holding {"remote_manifests": [url]},
//
// but NOT via a standalone "remote_manifests" field on the claim
// (c2pa.claim.v2) itself. c2pa-rs 0.85.1 (c2patool 0.26.61) hard-rejects an
// unrecognized "remote_manifests" V2 claim field ("claim could not be
// converted from CBOR"), which broke c2patool/veraPDF validation for every
// DCS-produced PDF that ever had a manifest URL — i.e. almost all of them,
// since every lifecycle update passes one. Expressing the URL as a regular
// assertion (referenced from the claim only via an opaque hashed-URI, same
// as every other assertion) sidesteps that hard rejection.
func TestUpdatePDFWithManifestURLEmbedsXMPProvenance(t *testing.T) {
	const manifestURL = "http://localhost:8991/api/c2pa/manifest/did:example:contract-42"

	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF(base): %v", err)
	}

	updated, err := UpdatePDFWithOptions(testSigningContext(), original,
		[]byte(minimalPayloadAmended), nil, manifestURL, time.Now())
	if err != nil {
		t.Fatalf("UpdatePDFWithOptions: %v", err)
	}

	if !bytes.Contains(updated, []byte("dcterms:provenance")) {
		t.Error("updated PDF's XMP metadata does not contain a dcterms:provenance property")
	}
	if !bytes.Contains(updated, []byte(manifestURL)) {
		t.Errorf("updated PDF does not contain the remote manifest URL %q anywhere", manifestURL)
	}

	store, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}

	manifestBoxes, err := extractTopLevelManifestBoxes(store)
	if err != nil {
		t.Fatalf("extractTopLevelManifestBoxes: %v", err)
	}
	if len(manifestBoxes) == 0 {
		t.Fatalf("no manifests found in updated manifest store")
	}
	activeManifest := manifestBoxes[len(manifestBoxes)-1]

	claimBox, err := extractLabeledChildJUMBFBox(activeManifest, "c2pa.claim.v2")
	if err != nil {
		t.Fatalf("extract c2pa.claim.v2: %v", err)
	}
	// CBOR major type 3 (text string) header for a 16-byte string is
	// 0x60 | 16 = 0x70. "remote_manifests" is exactly 16 ASCII characters, so
	// this byte sequence is the standalone CBOR text item that would appear
	// as a literal claim field name — as opposed to occurring merely as a
	// substring of a longer string (e.g. the "dcs.remote_manifests"
	// assertion label or its "self#jumbf=..." reference URL, both of which
	// use different, longer CBOR text headers and are expected to appear).
	standaloneRemoteManifestsField := append([]byte{0x70}, []byte("remote_manifests")...)
	if bytes.Contains(claimBox, standaloneRemoteManifestsField) {
		t.Error("c2pa.claim.v2 must not contain a standalone 'remote_manifests' CBOR text field")
	}

	assertionStore, err := extractLabeledChildJUMBFBox(activeManifest, "c2pa.assertions")
	if err != nil {
		t.Fatalf("extract c2pa.assertions: %v", err)
	}
	remoteManifestsBox, err := extractLabeledChildJUMBFBox(assertionStore, "dcs.remote_manifests")
	if err != nil {
		t.Fatalf("dcs.remote_manifests assertion not found in assertion store: %v", err)
	}
	if !bytes.Contains(remoteManifestsBox, standaloneRemoteManifestsField) {
		t.Error("dcs.remote_manifests assertion is missing its 'remote_manifests' CBOR text field")
	}
	if !bytes.Contains(remoteManifestsBox, []byte(manifestURL)) {
		t.Errorf("dcs.remote_manifests assertion does not contain the manifest URL %q", manifestURL)
	}

	if _, err := VerifyIncrementalUpdate(testSigningContext(), updated); err != nil {
		t.Fatalf("VerifyIncrementalUpdate must still pass with the XMP provenance link embedded: %v", err)
	}
}

// TestUpdatePDFWithoutManifestURLHasNoProvenanceLink verifies the default path
// (no manifest URL) emits no dcterms:provenance XMP property and no
// remote_manifests claim field — matching pdf-core/features/manifest_url.feature's
// "absent" scenario.
func TestUpdatePDFWithoutManifestURLHasNoProvenanceLink(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF(base): %v", err)
	}
	updated, err := UpdatePDF(testSigningContext(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	if bytes.Contains(updated, []byte("dcterms:provenance")) {
		t.Error("updated PDF must not contain a dcterms:provenance property when no manifest URL is supplied")
	}
	store, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if bytes.Contains(store, []byte("remote_manifests")) {
		t.Error("manifest store must not contain remote_manifests when no manifest URL is supplied")
	}
	if _, err := VerifyIncrementalUpdate(testSigningContext(), updated); err != nil {
		t.Fatalf("VerifyIncrementalUpdate (no manifest url): %v", err)
	}
}
