package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// lifecycleVC returns a lifecycle credential shaped like the one the DCS
// backend issues for a contract state transition (provenance.IssueLifecycleVC):
// what an external reader learns from it is credentialSubject.status.
func lifecycleVC(id, status string) []byte {
	return []byte(fmt.Sprintf(
		`{"@context":["https://www.w3.org/ns/credentials/v2"],"type":["VerifiableCredential","ContractLifecycleCredential"],"id":"urn:dcs:vc:%s","issuer":"did:web:dcs.example","validFrom":"2026-01-01T00:00:00Z","credentialSubject":{"id":"did:web:dcs.example:contract","contract_id":"did:web:dcs.example:contract","file_hash":"%s","status":%q,"reason":"","effective_at":"2026-01-01T00:00:00Z"}}`,
		id, id, status))
}

var (
	embeddedFilesNamesRE = regexp.MustCompile(`/EmbeddedFiles << /Names \[([^\]]*)\]`)
	filespecEFRefRE      = regexp.MustCompile(`/EF << /F (\d+) 0 R`)
)

// resolveEmbeddedFileByName returns the bytes an external reader gets when it
// asks the document for the attachment called name: it resolves the name
// through the current document catalog's /EmbeddedFiles name tree, which is the
// only route pypdf, Acrobat's attachment panel and every other by-name consumer
// take. It deliberately does NOT scan the file for the last filespec the way
// ExtractEmbeddedVC does.
func resolveEmbeddedFileByName(t *testing.T, pdf []byte, name string) []byte {
	t.Helper()
	rootID, ok := currentRootObjID(pdf)
	if !ok {
		t.Fatal("no /Root in the trailer")
	}
	start, end, ok := lastObjectBody(pdf, rootID)
	if !ok {
		t.Fatalf("catalog object %d not found", rootID)
	}
	names := embeddedFilesNamesRE.FindSubmatch(pdf[start:end])
	if names == nil {
		t.Fatal("catalog carries no /EmbeddedFiles name tree")
	}
	entry := regexp.MustCompile(regexp.QuoteMeta("("+name+")")+`\s*(\d+) 0 R`).FindSubmatch(names[1])
	if entry == nil {
		t.Fatalf("name tree has no entry for %q (names: %s)", name, names[1])
	}
	specID, err := strconv.Atoi(string(entry[1]))
	if err != nil {
		t.Fatalf("name tree entry for %q is not an object reference: %v", name, err)
	}
	specStart, specEnd, ok := lastObjectBody(pdf, specID)
	if !ok {
		t.Fatalf("filespec object %d not found", specID)
	}
	ef := filespecEFRefRE.FindSubmatch(pdf[specStart:specEnd])
	if ef == nil {
		t.Fatalf("filespec object %d carries no /EF reference", specID)
	}
	fileID, err := strconv.Atoi(string(ef[1]))
	if err != nil {
		t.Fatalf("filespec %d /EF reference invalid: %v", specID, err)
	}
	streamStart, streamEnd, ok := lastObjectStreamData(pdf, fileID)
	if !ok {
		t.Fatalf("embedded file stream not found in object %d", fileID)
	}
	return pdf[streamStart:streamEnd]
}

// signedContractWithLifecycleHistory builds the document a real ceremony
// produces: a compiled draft, an amendment carrying the "draft" lifecycle
// credential, the "active" credential stamped in immediately before signing
// (backend stampLifecycleForSigning), and then the PAdES revision itself.
func signedContractWithLifecycleHistory(t *testing.T) []byte {
	t.Helper()
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDFWithVC(testSigningContext(), compiled, []byte(minimalPayloadAmended), lifecycleVC("draftvc", "draft"), time.Now())
	if err != nil {
		t.Fatalf("amendment update: %v", err)
	}
	stamped, err := UpdatePDFWithVC(testSigningContext(), amended, []byte(minimalPayloadAmended), lifecycleVC("activevc", "active"), time.Now())
	if err != nil {
		t.Fatalf("pre-signing lifecycle stamp: %v", err)
	}
	return appendPAdESRevision(t, stamped, "Signature1")
}

// TestNameTreeResolvesTheCurrentLifecycleVC pins what an external reader is told
// about a SIGNED contract. Each lifecycle stamp attaches a fresh
// contract-lifecycle-vc.json filespec, so the /EmbeddedFiles name tree entry has
// to move with it: while it stayed pinned to the first filespec, every reader
// that resolves the attachment by name — pypdf, Acrobat's attachment panel, a
// wallet — read "draft" off a signed contract, and only the backend was spared
// because ExtractEmbeddedVC scans for the LAST filespec instead.
func TestNameTreeResolvesTheCurrentLifecycleVC(t *testing.T) {
	signed := signedContractWithLifecycleHistory(t)

	resolved := resolveEmbeddedFileByName(t, signed, "contract-lifecycle-vc.json")

	var vc struct {
		ID                string `json:"id"`
		CredentialSubject struct {
			Status string `json:"status"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(resolved, &vc); err != nil {
		t.Fatalf("attachment resolved by name is not a credential: %v", err)
	}
	if vc.CredentialSubject.Status != "active" {
		t.Fatalf("attachment resolved by name reports status %q (credential %s), want the current %q",
			vc.CredentialSubject.Status, vc.ID, "active")
	}
}

// TestNameTreeAndLastFilespecAgree keeps the two routes into the attachment from
// diverging again: whatever ExtractEmbeddedVC hands the backend must be the same
// bytes the name tree hands everyone else.
func TestNameTreeAndLastFilespecAgree(t *testing.T) {
	signed := signedContractWithLifecycleHistory(t)

	backendSees, ok, err := ExtractEmbeddedVC(signed)
	if err != nil || !ok {
		t.Fatalf("ExtractEmbeddedVC: ok=%v err=%v", ok, err)
	}
	if readerSees := resolveEmbeddedFileByName(t, signed, "contract-lifecycle-vc.json"); string(readerSees) != string(backendSees) {
		t.Fatalf("name tree resolves %s, ExtractEmbeddedVC returns %s", readerSees, backendSees)
	}
}

// TestUpdateManifestRecordsTheAttachedCredentialsState pins the C2PA lifecycle
// chain to the states the document actually passed through. The amendment
// manifest used to hardcode "amended" on every hop, so the signed contract's
// provenance read draft -> amended -> amended while the credential the signature
// commits to said "active": the artifact contradicted the API's lifecycle_status
// with nothing able to detect it, and ADR-13's "federation state is derivable
// from the artifact alone" did not hold.
func TestUpdateManifestRecordsTheAttachedCredentialsState(t *testing.T) {
	signed := signedContractWithLifecycleHistory(t)

	want := []string{"draft", "draft", "active"}
	for idx, wantStatus := range want {
		if got := lifecycleFieldsOf(t, signed, idx)["status"]; got != wantStatus {
			t.Errorf("manifest %d lifecycle status = %q, want %q", idx, got, wantStatus)
		}
	}
}

// TestUpdateManifestRecordsAmendedWithoutACredential keeps "amended" as the
// honest reading of a hop that records no lifecycle event: a content amendment
// with no credential attached is not a state transition.
func TestUpdateManifestRecordsAmendedWithoutACredential(t *testing.T) {
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDF(testSigningContext(), compiled, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}
	if got := lifecycleFieldsOf(t, amended, 1)["status"]; got != "amended" {
		t.Fatalf("amendment without a credential recorded status %q, want %q", got, "amended")
	}
}

// TestLifecycleAssertionFileHashIsTheEmbeddedPayloadHash pins what the C2PA
// dcs.lifecycle assertion's file_hash names: the embedded machine-readable
// contract, not the PDF. It cannot be the PDF: the assertion is inside the
// manifest that is inside those very bytes, so hashing them from here is
// impossible — the whole-file binding is the job of c2pa.hash.data, which hashes
// the document with the manifest range excluded. The lifecycle VC's
// credentialSubject.file_hash names the artifact bytes instead (the revision
// this credential is attached to, hashed before the attachment); the two fields
// share a name and do not share a meaning.
func TestLifecycleAssertionFileHashIsTheEmbeddedPayloadHash(t *testing.T) {
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDFWithVC(testSigningContext(), compiled, []byte(minimalPayloadAmended), lifecycleVC("draftvc", "draft"), time.Now())
	if err != nil {
		t.Fatalf("amendment update: %v", err)
	}

	for idx, pdf := range [][]byte{compiled, amended} {
		payload, err := ExtractLatestEmbeddedJSONLD(pdf)
		if err != nil {
			t.Fatalf("extract embedded payload of manifest %d: %v", idx, err)
		}
		want := sha256.Sum256(payload)
		if got := lifecycleFieldsOf(t, pdf, idx)["file_hash"]; got != hex.EncodeToString(want[:]) {
			t.Errorf("manifest %d file_hash = %q, want sha256 of the embedded contract.jsonld %x", idx, got, want)
		}
	}
}

// TestLifecycleChainSurvivesReplay proves the lifecycle states are reproducible
// from the document's own bytes: VerifyIncrementalUpdate replays every hop from
// the payload and credential it embedded, so a status read off the attached
// credential must rebuild byte-for-byte.
func TestLifecycleChainSurvivesReplay(t *testing.T) {
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	amended, err := UpdatePDFWithVC(testSigningContext(), compiled, []byte(minimalPayloadAmended), lifecycleVC("draftvc", "draft"), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("amendment update: %v", err)
	}
	stamped, err := UpdatePDFWithVC(testSigningContext(), amended, []byte(minimalPayloadAmended), lifecycleVC("activevc", "active"), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("pre-signing lifecycle stamp: %v", err)
	}
	if _, err := VerifyIncrementalUpdate(testSigningContext(), stamped); err != nil {
		t.Fatalf("VerifyIncrementalUpdate: %v", err)
	}
}
