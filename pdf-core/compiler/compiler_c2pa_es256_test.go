package compiler

import (
	"bytes"
	"testing"
	"time"
)

// coseAlgES256 is the CBOR encoding of the protected-header entry {1: -7}:
// key 1 (alg) -> -7 (ES256). {1: -8} (EdDSA) would encode as 0x01 0x27.
var coseAlgES256 = []byte{0x01, 0x26}
var coseAlgEdDSA = []byte{0x01, 0x27}

func TestCOSEProtectedHeaderDeclaresES256(t *testing.T) {
	header := buildCoseProtectedHeadersWithX5Chain()
	if !bytes.Contains(header, coseAlgES256) {
		t.Fatalf("protected header does not declare alg ES256(-7): % x", header)
	}
	if bytes.Contains(header, coseAlgEdDSA) {
		t.Fatalf("protected header still declares alg EdDSA(-8): % x", header)
	}
}

// TestCompiledManifestDeclaresES256 checks the alg survives into a full compiled
// PDF's embedded C2PA manifest (the AC6 second scenario's observable behaviour).
func TestCompiledManifestDeclaresES256(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	c2pa, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		t.Fatalf("extract C2PA: %v", err)
	}
	if !bytes.Contains(c2pa, coseAlgES256) {
		t.Fatalf("embedded C2PA manifest does not declare alg ES256(-7)")
	}
	if bytes.Contains(c2pa, coseAlgEdDSA) {
		t.Fatalf("embedded C2PA manifest still declares alg EdDSA(-8)")
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
