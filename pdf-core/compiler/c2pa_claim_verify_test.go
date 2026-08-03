package compiler

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
	"time"
)

func TestVerifyC2PAClaimSignaturesAcceptsCompiledPDF(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if err := VerifyC2PAClaimSignatures(pdf); err != nil {
		t.Fatalf("compiled PDF's claim signature must verify: %v", err)
	}
}

// TestVerifyC2PAClaimSignaturesRejectsForgedSignature is the check the old
// hardcoded C2PASignatureValid=true could never make: a manifest whose COSE
// signature bytes were replaced no longer verifies.
func TestVerifyC2PAClaimSignaturesRejectsForgedSignature(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	idx := bytes.Index(pdf, coseDetachedSig64Marker)
	if idx < 0 {
		t.Fatal("compiled PDF carries no COSE signature to forge")
	}
	forged := append([]byte(nil), pdf...)
	start := idx + len(coseDetachedSig64Marker)
	for i := start; i < start+64; i++ {
		forged[i] ^= 0xFF
	}
	err = VerifyC2PAClaimSignatures(forged)
	if err == nil {
		t.Fatal("a forged COSE signature must not verify")
	}
	if !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("expected a signature-verification failure, got: %v", err)
	}
}

// TestVerifyC2PAClaimSignaturesRejectsSwappedAssertion covers the attack a bare
// claim-signature check misses: the signature covers only the claim, so an
// assertion can be rewritten underneath a signature that still verifies. The
// claim's created_assertions hashes are what catch it.
func TestVerifyC2PAClaimSignaturesRejectsSwappedAssertion(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	store, err := ExtractManifestStore(pdf)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	// Rewrite the lifecycle assertion's recorded status in place, keeping the
	// byte length (and therefore every JUMBF box size) intact.
	marker := []byte("draft")
	idx := bytes.Index(store, marker)
	if idx < 0 {
		t.Fatal("lifecycle assertion does not carry the expected status text")
	}
	tamperedStore := append([]byte(nil), store...)
	copy(tamperedStore[idx:], []byte("DRAFT"))

	tampered := bytes.Replace(pdf, store, tamperedStore, 1)
	if bytes.Equal(tampered, pdf) {
		t.Fatal("manifest store was not replaced in the PDF")
	}
	err = VerifyC2PAClaimSignatures(tampered)
	if err == nil {
		t.Fatal("a rewritten assertion must not pass verification")
	}
	if !strings.Contains(err.Error(), "dcs.lifecycle") {
		t.Fatalf("expected the rewritten assertion to be named, got: %v", err)
	}
}

func TestVerifyC2PAClaimSignaturesRejectsPDFWithoutManifest(t *testing.T) {
	if err := VerifyC2PAClaimSignatures([]byte("%PDF-1.7\n%%EOF\n")); err == nil {
		t.Fatal("a PDF with no C2PA manifest store must not report a valid claim signature")
	}
}

// TestVerifyC2PAClaimSignaturesSurvivesAppendedRevision fixes the property the
// verdict depends on for signed contracts: a PAdES signature is an append-only
// revision, so the manifest it commits to is untouched and still verifies.
func TestVerifyC2PAClaimSignaturesSurvivesAppendedRevision(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	appended := append(append([]byte(nil), pdf...), []byte("\n% appended revision\n")...)
	if err := VerifyC2PAClaimSignatures(appended); err != nil {
		t.Fatalf("appending bytes after the base must not invalidate the claim signature: %v", err)
	}
}

// The shapes a real contract PDF reaches by the time a DCS verifies it: an
// amendment carrying a remote-manifest reference, and the provenance-only
// re-anchor appended over a signed document (ADR-26). Every manifest in both must
// verify, or the backend reports a signed contract as C2PA-invalid.
func TestVerifyC2PAClaimSignaturesAcceptsTheProductionManifestShapes(t *testing.T) {
	const manifestURL = "http://localhost:8991/api/c2pa/manifest/did:example:contract-42"

	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	amended, err := UpdatePDFWithOptions(testSigningContext(), compiled,
		[]byte(minimalPayloadAmended), nil, manifestURL, time.Now())
	if err != nil {
		t.Fatalf("UpdatePDFWithOptions: %v", err)
	}
	if err := VerifyC2PAClaimSignatures(amended); err != nil {
		t.Fatalf("an amendment carrying a remote-manifest reference must verify: %v", err)
	}

	reanchored, err := ReanchorProvenance(testSigningContext(), appendSignatureRevision(amended), manifestURL, time.Now())
	if err != nil {
		t.Fatalf("ReanchorProvenance: %v", err)
	}
	if err := VerifyC2PAClaimSignatures(reanchored); err != nil {
		t.Fatalf("a re-anchored signed document must verify: %v", err)
	}
}

func TestDecodeCOSESign1RejectsInlinePayload(t *testing.T) {
	// D2 84 <bstr protected> A0 <bstr payload> <bstr sig>: a COSE_Sign1 whose
	// payload is carried inline rather than detached.
	inline := []byte{0xD2, 0x84}
	inline = append(inline, cborBytes([]byte{0xA0})...)
	inline = append(inline, 0xA0)
	inline = append(inline, cborBytes([]byte("claim"))...)
	inline = append(inline, cborBytes(make([]byte, 64))...)
	if _, _, err := decodeCOSESign1(inline); err == nil {
		t.Fatal("an inline COSE_Sign1 payload must be refused")
	}
}

func TestCOSEX5ChainLeafKeyRequiresES256(t *testing.T) {
	headers := cborMap(cborUint(1), cborNegInt(-8), cborUint(33), cborArray(cborBytes([]byte("not-a-cert"))))
	if _, err := coseX5ChainLeafKey(headers); err == nil {
		t.Fatal("protected headers declaring EdDSA must be refused")
	}
}

func TestCOSEX5ChainLeafKeyReadsCompilerHeaders(t *testing.T) {
	protected, err := buildCoseProtectedHeadersWithX5Chain(testSigningContext())
	if err != nil {
		t.Fatalf("buildCoseProtectedHeadersWithX5Chain: %v", err)
	}
	key, err := coseX5ChainLeafKey(protected)
	if err != nil {
		t.Fatalf("coseX5ChainLeafKey: %v", err)
	}
	if !key.Equal(testC2PASigner.Public()) {
		t.Fatal("the x5chain leaf key must be the key the compiler's signer holds")
	}
}

func TestAssertionLabelFromURI(t *testing.T) {
	cases := map[string]string{
		"self#jumbf=c2pa.assertions/dcs.lifecycle":                          "dcs.lifecycle",
		"self#jumbf=/c2pa/urn:c2pa:abc/c2pa.assertions/c2pa.hash.data":      "c2pa.hash.data",
		"self#jumbf=/c2pa/urn:c2pa:abc/c2pa.assertions/dcs.remote_manifest": "dcs.remote_manifest",
		"self#jumbf=c2pa.signature":                                         "",
	}
	for uri, want := range cases {
		if got := assertionLabelFromURI(uri); got != want {
			t.Errorf("assertionLabelFromURI(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestDecodeCBORReadsHashedURIMap(t *testing.T) {
	hash := sha256.Sum256([]byte("assertion"))
	encoded := cborMap(
		cborText("url"), cborText("self#jumbf=c2pa.assertions/dcs.lifecycle"),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(hash[:]),
	)
	item, size, err := decodeCBOR(encoded)
	if err != nil {
		t.Fatalf("decodeCBOR: %v", err)
	}
	if size != len(encoded) {
		t.Fatalf("decodeCBOR consumed %d of %d bytes", size, len(encoded))
	}
	url, ok := item.entry("url")
	if !ok || url.text != "self#jumbf=c2pa.assertions/dcs.lifecycle" {
		t.Fatalf("url entry not decoded: %+v", url)
	}
	decodedHash, ok := item.entry("hash")
	if !ok || !bytes.Equal(decodedHash.bytes, hash[:]) {
		t.Fatal("hash entry not decoded")
	}
}
