package compiler

import (
	"bytes"
	"testing"
	"time"
)

// coseHeaderES256 starts every protected header the compiler builds: a CBOR
// map of two entries (0xA2) whose first entry is alg (1) -> ES256 (-7, 0x26).
// The x5chain follows, so the alg is checked at this fixed position: a search
// for the EdDSA bytes (0x01 0x27) anywhere in the header also hits random
// certificate bytes.
var coseHeaderES256 = []byte{0xA2, 0x01, 0x26}

func TestCOSEProtectedHeaderDeclaresES256(t *testing.T) {
	header, err := buildCoseProtectedHeadersWithX5Chain(testSigningContext())
	if err != nil {
		t.Fatalf("buildCoseProtectedHeadersWithX5Chain: %v", err)
	}
	if !bytes.HasPrefix(header, coseHeaderES256) {
		t.Fatalf("protected header does not start with alg ES256(-7): % x", header[:min(len(header), 8)])
	}
}

// TestCompiledManifestDeclaresES256 checks the alg survives into a full compiled
// PDF's embedded C2PA manifest (the AC6 second scenario's observable behaviour):
// the manifest carries the protected header built for the same signing chain.
func TestCompiledManifestDeclaresES256(t *testing.T) {
	ctx := testSigningContext()
	header, err := buildCoseProtectedHeadersWithX5Chain(ctx)
	if err != nil {
		t.Fatalf("buildCoseProtectedHeadersWithX5Chain: %v", err)
	}
	pdf, err := CompilePDF(ctx, []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	c2pa, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		t.Fatalf("extract C2PA: %v", err)
	}
	if !bytes.HasPrefix(header, coseHeaderES256) || !bytes.Contains(c2pa, header) {
		t.Fatalf("embedded C2PA manifest does not carry the ES256 protected header")
	}
}

// TestZeroCOSESignaturesMasksDifferingSignatures proves that two PDFs which
// differ only in their (non-deterministic, HSM-produced) ES256 signature bytes
// compare equal after masking — the property the /verify recompile relies on.
func TestZeroCOSESignaturesMasksDifferingSignatures(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, coseDetachedSig64Marker) {
		t.Fatalf("compiled PDF has no COSE signature marker to mask")
	}

	tampered := append([]byte(nil), pdf...)
	idx := bytes.Index(tampered, coseDetachedSig64Marker) + len(coseDetachedSig64Marker)
	for i := idx; i < idx+64; i++ {
		tampered[i] ^= 0xFF
	}
	if bytes.Equal(pdf, tampered) {
		t.Fatalf("tampering did not change the signature bytes")
	}
	if !bytes.Equal(ZeroCOSESignatures(pdf), ZeroCOSESignatures(tampered)) {
		t.Fatalf("masked PDFs differ despite only signature bytes changing")
	}
}
