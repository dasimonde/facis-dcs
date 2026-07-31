package compiler

import (
	"bytes"
	"testing"
)

// TestCompilePDF_DeterministicWithCanonicalEpoch proves the same payload
// compiled twice with CanonicalCompiledAt yields byte-identical PDFs. Wall-clock
// time (the former /download behavior) breaks this; the fixed render epoch is
// what keeps the /download tamper-evidence invariant — same payload, same bytes.
func TestCompilePDF_DeterministicWithCanonicalEpoch(t *testing.T) {
	first, err := CompilePDF(testSigningContext(), []byte(claimBase), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}
	second, err := CompilePDF(testSigningContext(), []byte(claimBase), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("CompilePDF is non-deterministic: %d vs %d bytes and content differs", len(first), len(second))
	}
}
