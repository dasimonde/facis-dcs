package compiler

import (
	"bytes"
	"fmt"
	"testing"
)

// An unanchored search for "19 0 obj" also matches inside "100019 0 obj". Both
// signing gates resolved objects that way, so an appended revision could
// supersede a page stream or the JSON-LD attachment with tampered content and
// carry a decoy whose id merely ENDS in the target's digits holding the
// original — the gate read the decoy and reported a match on a document whose
// visible page had been replaced.
func TestObjectHeaderLookupIsNotFooledByAnIDSuffix(t *testing.T) {
	pdf := []byte("%PDF-1.7\n" +
		"19 0 obj\n<< /Length 8 >>\nstream\nORIGINAL\nendstream\nendobj\n" +
		"100019 0 obj\n<< /Length 5 >>\nstream\nDECOY\nendstream\nendobj\n")

	last, ok := lastObjectHeader(pdf, 19)
	if !ok || last.start != 9 {
		t.Errorf("last header for 19 resolved to %d, not the real definition at 9", last.start)
	}
	first, ok := firstObjectHeader(pdf, 19)
	if !ok || first.start != 9 {
		t.Errorf("first header for 19 resolved to %d, not 9", first.start)
	}
	// The decoy is still findable by its own id.
	if decoy, ok := lastObjectHeader(pdf, 100019); !ok || decoy.start <= 9 {
		t.Errorf("object 100019 resolved to %d, which is not its own definition", decoy.start)
	}

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != "ORIGINAL" {
		t.Errorf("extracted %q — a suffix-colliding decoy was read instead of object 19", content)
	}
}

// A superseding revision must win: the gate compares what a reader renders.
func TestObjectHeaderLookupPrefersTheLatestDefinition(t *testing.T) {
	pdf := []byte("%PDF-1.7\n" +
		"19 0 obj\n<< /Length 5 >>\nstream\nFIRST\nendstream\nendobj\n" +
		"19 0 obj\n<< /Length 10 >>\nstream\nSUPERSEDED\nendstream\nendobj\n")

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != "SUPERSEDED" {
		t.Errorf("extracted %q, want the latest definition", content)
	}
}

// decoyPDF is a minimal PDF whose embedded-file object (objID) is defined
// twice: once really and once as a decoy whose id merely ENDS in the same
// digits ("100019" for "19"). An unanchored substring search reads the decoy
// while a conforming reader follows the xref to the real object. crlfHeader
// spells the real object's header with the CRLF end-of-line every reader
// accepts.
func decoyPDF(fileName string, objID int, real, decoy string, decoyFirst, crlfHeader bool) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	buf.WriteString(fmt.Sprintf(
		"1 0 obj\n<< /Type /Filespec /F (%s) /UF (%s) /EF << /F %d 0 R >> >>\nendobj\n",
		fileName, fileName, objID))

	writeStreamObject := func(id int, eol, content string) {
		buf.WriteString(eol)
		buf.WriteString(fmt.Sprintf("%d 0 obj", id))
		buf.WriteString(eol)
		buf.WriteString(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))
	}

	realEOL := "\n"
	if crlfHeader {
		realEOL = "\r\n"
	}
	if decoyFirst {
		writeStreamObject(objID*10000+objID, "\n", decoy)
		writeStreamObject(objID, realEOL, real)
	} else {
		writeStreamObject(objID, realEOL, real)
		writeStreamObject(objID*10000+objID, "\n", decoy)
	}
	buf.WriteString("startxref\n0\n%%EOF\n")
	return buf.Bytes()
}

// A CRLF header is valid PDF and every reader accepts it, so a lookup that
// matches only "\n%d 0 obj\n" silently returns the EARLIER definition of the
// object — the suffix-decoy evasion in a different spelling.
func TestObjectHeaderLookupAcceptsACRLFHeader(t *testing.T) {
	pdf := []byte("%PDF-1.7\n19 0 obj\nFIRST\nendobj\r\n19 0 obj\r\nCURRENT\nendobj\n")

	headers := objectHeaders(pdf, 19)
	if len(headers) != 2 {
		t.Fatalf("object 19 is defined twice (LF then CRLF), got %d headers", len(headers))
	}
	last, ok := lastObjectHeader(pdf, 19)
	if !ok || last.start != headers[1].start {
		t.Fatalf("the CRLF definition at %d must win, got %d", headers[1].start, last.start)
	}
	if !bytes.HasPrefix(pdf[last.body:], []byte("CURRENT")) {
		t.Fatalf("the body must start past the CRLF closing the header, got %q", pdf[last.body:])
	}
}

// A decoy appended AFTER the real object defeats a trailing-substring search:
// the evidence attachment the signing-summary credentials are read from would
// come from another contract's validly signed PDF, so the drift and PID
// cross-checks in signing verification run against the wrong sealed hashes.
func TestExtractSigningEvidenceIgnoresADecoyObjectID(t *testing.T) {
	pdf := decoyPDF(signingEvidenceFileName, 42, `{"evidence":"real"}`, `{"evidence":"decoy"}`, false, false)

	evidence, err := ExtractSigningEvidence(pdf)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("the evidence attachment is present once, got %d", len(evidence))
	}
	if string(evidence[0]) != `{"evidence":"real"}` {
		t.Fatalf("read the decoy object's stream: %s", evidence[0])
	}
}

func TestExtractSigningEvidenceReadsTheCurrentCRLFDefinition(t *testing.T) {
	pdf := decoyPDF(signingEvidenceFileName, 42, `{"evidence":"current"}`, `{"evidence":"superseded"}`, true, true)

	evidence, err := ExtractSigningEvidence(pdf)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("the evidence attachment is present once, got %d", len(evidence))
	}
	if string(evidence[0]) != `{"evidence":"current"}` {
		t.Fatalf("read a superseded definition: %s", evidence[0])
	}
}

// StripEmbeddedJSONLD zeroes the attachment before the stripped bytes are page-
// matched and hashed. A decoy defined EARLIER defeats a leading-substring
// search, so the strip blanks the wrong range and the real payload survives.
func TestStripEmbeddedJSONLDIgnoresADecoyObjectID(t *testing.T) {
	pdf := decoyPDF("contract.jsonld", 19, `{"payload":"real"}`, `{"payload":"decoy"}`, true, false)

	stripped, err := StripEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("StripEmbeddedJSONLD: %v", err)
	}
	if len(stripped) != len(pdf) {
		t.Fatalf("stripping must preserve every object offset, got %d bytes for %d", len(stripped), len(pdf))
	}
	if bytes.Contains(stripped, []byte(`{"payload":"real"}`)) {
		t.Fatal("the attachment a reader resolves survived; a decoy object's stream was zeroed instead")
	}
	if !bytes.Contains(stripped, []byte(`{"payload":"decoy"}`)) {
		t.Fatal("only the resolved object's stream may be zeroed")
	}
}

func TestStripEmbeddedJSONLDStripsTheCurrentCRLFDefinition(t *testing.T) {
	pdf := decoyPDF("contract.jsonld", 19, `{"payload":"current"}`, `{"payload":"superseded"}`, true, true)

	stripped, err := StripEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("StripEmbeddedJSONLD: %v", err)
	}
	if bytes.Contains(stripped, []byte(`{"payload":"current"}`)) {
		t.Fatal("the current (CRLF-headered) definition was not the one stripped")
	}
}

// The lifecycle VC attachment is resolved the same way and needs the same
// anchoring.
func TestExtractEmbeddedVCIgnoresADecoyObjectID(t *testing.T) {
	pdf := decoyPDF("contract-lifecycle-vc.json", 42, `{"vc":"real"}`, `{"vc":"decoy"}`, false, false)

	vc, found, err := ExtractEmbeddedVC(pdf)
	if err != nil {
		t.Fatalf("ExtractEmbeddedVC: %v", err)
	}
	if !found {
		t.Fatal("the lifecycle VC attachment is present")
	}
	if string(vc) != `{"vc":"real"}` {
		t.Fatalf("read the decoy object's stream: %s", vc)
	}
}
