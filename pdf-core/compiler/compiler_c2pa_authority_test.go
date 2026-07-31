package compiler

import (
	"testing"
	"time"
)

const testAuthorityDID = "did:web:dcs-ionos.facis.cloud"

// lifecycleFieldsOf reads the dcs.lifecycle assertion of manifest manifestIdx out
// of a compiled PDF.
func lifecycleFieldsOf(t *testing.T, pdf []byte, manifestIdx int) map[string]string {
	t.Helper()
	c2paBytes, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		t.Fatalf("extract C2PA: %v", err)
	}
	fields, err := extractLifecycleFields(c2paBytes, manifestIdx)
	if err != nil {
		t.Fatalf("extract lifecycle fields of manifest %d: %v", manifestIdx, err)
	}
	return fields
}

// TestGenesisLifecycleCarriesAuthority proves the embedded provenance names the
// party asserting it, not only the contract it is about. Without it the
// assertion identifies what was signed but never who says so, which is weaker
// than the deployment chart's own description of signing.issuerDID.
func TestGenesisLifecycleCarriesAuthority(t *testing.T) {
	ctx := WithLifecycleAuthority(testSigningContext(), testAuthorityDID)
	pdf, err := CompilePDF(ctx, []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if got := lifecycleFieldsOf(t, pdf, 0)["authority"]; got != testAuthorityDID {
		t.Fatalf("genesis lifecycle authority = %q, want %q", got, testAuthorityDID)
	}
}

// TestAmendmentLifecycleCarriesAuthority covers the second call site: an
// amendment is asserted by whichever instance applied it.
func TestAmendmentLifecycleCarriesAuthority(t *testing.T) {
	ctx := WithLifecycleAuthority(testSigningContext(), testAuthorityDID)
	compiled, err := CompilePDF(ctx, []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDF(ctx, compiled, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	if got := lifecycleFieldsOf(t, amended, 1)["authority"]; got != testAuthorityDID {
		t.Fatalf("amendment lifecycle authority = %q, want %q", got, testAuthorityDID)
	}
}

// TestVerifyReproducesAuthorityFromTheDocument is the constraint that makes the
// authority safe to embed at all. /verify recompiles a PDF from its embedded
// payload and demands the result reproduce the stored bytes, but the authority
// is not in the payload — so it has to be recovered from the stored manifest, as
// the lifecycle timestamp already is. A verifier that has never seen the
// originating instance's DID must still be able to re-derive the document.
func TestVerifyReproducesAuthorityFromTheDocument(t *testing.T) {
	authored := WithLifecycleAuthority(testSigningContext(), testAuthorityDID)
	compiled, err := CompilePDF(authored, []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDF(authored, compiled, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}

	// A verifier's context carries no authority of its own.
	if _, err := VerifyIncrementalUpdate(testSigningContext(), amended); err != nil {
		t.Fatalf("VerifyIncrementalUpdate on a PDF carrying an authority: %v", err)
	}
}

// TestAuthorityIsPerRenderNotGlobal guards the multi-tenant case: pdf-core is a
// stateless renderer several instances may share, so the authority travels with
// the request rather than being configured into the process.
func TestAuthorityIsPerRenderNotGlobal(t *testing.T) {
	const other = "did:web:dcs-osc.facis.cloud"

	first, err := CompilePDF(WithLifecycleAuthority(testSigningContext(), testAuthorityDID), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	second, err := CompilePDF(WithLifecycleAuthority(testSigningContext(), other), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if got := lifecycleFieldsOf(t, first, 0)["authority"]; got != testAuthorityDID {
		t.Fatalf("first authority = %q, want %q", got, testAuthorityDID)
	}
	if got := lifecycleFieldsOf(t, second, 0)["authority"]; got != other {
		t.Fatalf("second authority = %q, want %q", got, other)
	}
}
