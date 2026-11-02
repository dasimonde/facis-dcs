package query

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

const copyTestDID = "did:example:contract-copy"

// token returns the value of "key=value" in a fake PDF, or "".
func token(pdf []byte, key string) string {
	for _, field := range strings.Fields(string(pdf)) {
		if v, ok := strings.CutPrefix(field, key+"="); ok {
			return v
		}
	}
	return ""
}

// newCopyPDFCore fakes the pdf-core endpoints a copy verification calls. A fake
// PDF is a line of fields: "manifest" when it embeds a C2PA manifest,
// "ref=<url>" for its XMP manifest reference, "pages=<x>" for its page content
// and "payload=<x>" for its machine-readable payload.
func newCopyPDFCore(t *testing.T) *pdfcore.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest/extract":
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte(" manifest ")) {
				http.Error(w, `{"name":"bad_request"}`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("jumb"))
		case "/manifest/reference":
			body, _ := io.ReadAll(r.Body)
			if ref := token(body, "ref"); ref != "" {
				_, _ = w.Write([]byte(ref))
				return
			}
			http.Error(w, `{"name":"not_found"}`, http.StatusNotFound)
		case "/payload/extract":
			body, _ := io.ReadAll(r.Body)
			_, _ = w.Write([]byte(token(body, "payload")))
		case "/verify/content-match":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			pdf, ref := []byte(r.FormValue("pdf")), []byte(r.FormValue("reference"))
			// A copy is held to the exact comparison, signature widgets included.
			exact := r.FormValue("exact") == "true"
			_ = json.NewEncoder(w).Encode(map[string]any{"match": exact && token(pdf, "pages") == token(ref, "pages")})
		case "/verify":
			_ = json.NewEncoder(w).Encode(map[string]any{"match": true, "c2pa_signature_valid": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return pdfcore.New(srv.URL, func([]byte) ([]byte, error) { return make([]byte, 64), nil })
}

// storedVerification is the verification of the contract PDF that holds the
// remote manifest.
func storedVerification() *pdfgen.PDFVerifyResult {
	return &pdfgen.PDFVerifyResult{
		Match:               true,
		C2paManifestFound:   true,
		C2paSignatureStatus: provenance.CheckValid,
		VcProofStatus:       provenance.CheckNotAvailable,
		StatusListCheck:     "not_available",
		LifecycleStatus:     ptrToString("active"),
		PdfSignatureStatus:  provenance.CheckNotAvailable,
	}
}

// storedTestPDF is the contract PDF this instance stores for copyTestDID.
var storedTestPDF = "%PDF manifest ref=" + provenance.RemoteManifestURL(copyTestDID) + " pages=A payload=P "

// copyTestParties are the instances party to copyTestDID.
var copyTestParties = []string{"did:web:dcs-a.example", "did:web:dcs-b.example%3A8443"}

func verifyTestCopy(t *testing.T, copyPDF string) (*pdfgen.PDFVerifyResult, bool) {
	t.Helper()
	fetched := false
	original := func(context.Context) ([]byte, error) {
		fetched = true
		return []byte(storedTestPDF), nil
	}
	result, err := verifyCopy(context.Background(), newCopyPDFCore(t), nil, nil, nil, SignerAnchors{},
		VerifyContractPdfCopyQry{DID: copyTestDID, PDF: []byte(copyPDF)}, copyTestParties, storedVerification(), original)
	if err != nil {
		t.Fatalf("verifyCopy: %v", err)
	}
	return result, fetched
}

// A copy whose embedded manifest was removed is verified from the remote
// manifest it references (DCS-OR-C2PA-008).
func TestStrippedCopyIsVerifiedFromTheRemoteManifest(t *testing.T) {
	result, fetched := verifyTestCopy(t, "%PDF ref="+provenance.RemoteManifestURL(copyTestDID)+" pages=A payload=P")
	if !fetched {
		t.Fatal("the contract PDF that holds the remote manifest was not fetched")
	}
	if !result.Match || !result.C2paManifestFound || result.C2paSignatureStatus != provenance.CheckValid {
		t.Fatalf("the stripped copy did not verify from the remote manifest: %+v", result)
	}
	if result.C2paManifestSource == nil || *result.C2paManifestSource != manifestSourceRemote {
		t.Fatalf("c2pa_manifest_source = %v, want %q", result.C2paManifestSource, manifestSourceRemote)
	}
}

// The remote manifest describes the contract PDF; a copy with other pages or
// another payload does not verify from it.
func TestStrippedCopyWithOtherContentDoesNotVerify(t *testing.T) {
	ref := " ref=" + provenance.RemoteManifestURL(copyTestDID)
	for name, copyPDF := range map[string]string{
		"other pages":   "%PDF" + ref + " pages=B payload=P",
		"other payload": "%PDF" + ref + " pages=A payload=Q",
	} {
		t.Run(name, func(t *testing.T) {
			result, _ := verifyTestCopy(t, copyPDF)
			if result.Match {
				t.Fatalf("a copy with %s verified: %+v", name, result)
			}
			if result.Discrepancy == nil || *result.Discrepancy != discrepancyHashMismatch {
				t.Fatalf("discrepancy = %v, want %q", result.Discrepancy, discrepancyHashMismatch)
			}
		})
	}
}

// A stripped copy that references no manifest, or the manifest of another
// contract, has no manifest to verify against.
func TestStrippedCopyWithoutThisContractsReferenceHasNoManifest(t *testing.T) {
	for name, copyPDF := range map[string]string{
		"no reference":       "%PDF pages=A payload=P",
		"another contract's": "%PDF ref=" + provenance.RemoteManifestURL("did:example:other") + " pages=A payload=P",
		"a non-party host":   "%PDF ref=https://dcs-x.example/c2pa/manifest/" + copyTestDID + " pages=A payload=P",
	} {
		t.Run(name, func(t *testing.T) {
			result, _ := verifyTestCopy(t, copyPDF)
			if result.Match || result.C2paManifestFound {
				t.Fatalf("a copy without this contract's manifest verified: %+v", result)
			}
		})
	}
}

// The last manifest of a contract can be written by the counterparty's
// instance, which writes its own remote manifest URL into the PDF. A stripped
// copy of it references this contract's manifest at that party's host.
func TestStrippedCopyReferencingAPartysManifestIsVerified(t *testing.T) {
	for _, ref := range []string{
		"https://dcs-a.example/c2pa/manifest/" + copyTestDID,
		"https://dcs-b.example:8443/c2pa/manifest/" + copyTestDID,
	} {
		result, _ := verifyTestCopy(t, "%PDF ref="+ref+" pages=A payload=P")
		if !result.Match || !result.C2paManifestFound {
			t.Fatalf("a stripped copy referencing %s did not verify: %+v", ref, result)
		}
	}
}

// A copy that carries its manifest and is the stored PDF, or an earlier
// revision of it, is verified against its own manifest.
func TestCopyWithAnEmbeddedManifestIsVerifiedAgainstIt(t *testing.T) {
	for _, copyPDF := range []string{storedTestPDF, storedTestPDF[:len(storedTestPDF)-1]} {
		result, _ := verifyTestCopy(t, copyPDF)
		if !result.Match || !result.C2paManifestFound {
			t.Fatalf("the copy did not verify against its embedded manifest: %+v", result)
		}
		if result.C2paManifestSource == nil || *result.C2paManifestSource != manifestSourceEmbedded {
			t.Fatalf("c2pa_manifest_source = %v, want %q", result.C2paManifestSource, manifestSourceEmbedded)
		}
	}
}

// A self-consistent PDF that carries its own manifest but is not the stored PDF
// is never verified against itself: a genuine PDF of another contract, or one
// built under another issuer, does not verify for this contract.
func TestCopyOfAnotherContractWithEmbeddedManifestDoesNotVerify(t *testing.T) {
	for name, copyPDF := range map[string]string{
		"another contract":       "%PDF manifest ref=" + provenance.RemoteManifestURL("did:example:other") + " pages=B payload=Q",
		"forged pages":           "%PDF manifest ref=" + provenance.RemoteManifestURL(copyTestDID) + " pages=FORGED payload=P",
		"forged payload":         "%PDF manifest ref=" + provenance.RemoteManifestURL(copyTestDID) + " pages=A payload=FORGED",
		"another issuer's build": "%PDF manifest pages=A payload=P",
	} {
		t.Run(name, func(t *testing.T) {
			result, _ := verifyTestCopy(t, copyPDF)
			if result.Match {
				t.Fatalf("a copy that is not the stored PDF verified against itself: %+v", result)
			}
			if result.C2paManifestSource != nil && *result.C2paManifestSource == manifestSourceEmbedded {
				t.Fatal("a copy that is not the stored PDF was verified against its own manifest")
			}
		})
	}
}

// A copy with an embedded manifest that the stored PDF does not start with, but
// that shows the same pages and payload and references this contract's
// manifest (a party's later revision), is verified from the remote manifest.
func TestCopyThatDivergesFromTheStoredPDFIsVerifiedFromTheRemoteManifest(t *testing.T) {
	result, _ := verifyTestCopy(t, "%PDF manifest ref=https://dcs-b.example:8443/c2pa/manifest/"+copyTestDID+" pages=A payload=P peer-revision")
	if !result.Match || result.C2paManifestSource == nil || *result.C2paManifestSource != manifestSourceRemote {
		t.Fatalf("the party's copy did not verify from the remote manifest: %+v", result)
	}
}
