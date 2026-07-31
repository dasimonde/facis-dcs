package compiler

import (
	"bytes"
	"context"
	"testing"
)

// The rendered backlink is the payload's CIDv1 content-address: it must be the CID
// of the EXACT verbatim embedded bytes, so A (render), B (recompile), and any
// verifier compute the same value and it resolves the payload from IPFS.
func TestRenderedBacklinkIsPayloadCID(t *testing.T) {
	pdf, err := CompilePDF(WithSigner(context.Background(), NewCapturingSigner()), []byte(richFilledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := ExtractEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatal(err)
	}
	want := payloadCID(embedded)
	// Normalize the content stream: the CID may be wrapped across Tj operators, so
	// strip everything but the base32/label alphabet before checking containment.
	text := renderedText(t, pdf)
	if !bytes.Contains(compactAlnum(text), compactAlnum([]byte(want))) {
		t.Fatalf("rendered backlink is not the payload CID %s", want)
	}
}

func compactAlnum(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		}
	}
	return out
}
