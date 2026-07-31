package compiler

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// pdfWithObjects assembles a minimal document out of literal object bodies, so
// a test can spell a header or a stream keyword exactly as an attacker would.
func pdfWithObjects(objects ...string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	for _, object := range objects {
		b.WriteString(object)
	}
	b.WriteString("trailer\n<< /Root 1 0 R >>\n%%EOF\n")
	return b.Bytes()
}

// ISO 32000-1 7.2.3: any run of white space separates the tokens of an object
// header. A matcher spelled "%d 0 obj" misses this definition entirely and
// resolves an earlier one — or, with no earlier one, reports the object absent.
func TestObjectLookupAcceptsAnyWhitespaceBetweenHeaderTokens(t *testing.T) {
	pdf := pdfWithObjects(
		"19  0 obj\n<< /Length 8 >>\nstream\nORIGINAL\nendstream\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("a header separated by two spaces is still object 19's header: %v", err)
	}
	if string(content) != "ORIGINAL" {
		t.Fatalf("got %q, want the object's own stream", content)
	}
}

// An object freed and reused comes back with a raised generation number, and
// the xref sends the reader to that definition. Keying the lookup on "0"
// silently hands the checker the superseded bytes.
func TestObjectLookupResolvesANonZeroGeneration(t *testing.T) {
	pdf := pdfWithObjects(
		"19 0 obj\n<< /Length 10 >>\nstream\nSUPERSEDED\nendstream\nendobj\n",
		"19 1 obj\n<< /Length 7 >>\nstream\nCURRENT\nendstream\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != "CURRENT" {
		t.Fatalf("got %q, want the latest definition regardless of its generation", content)
	}
}

// ISO 32000-1 7.3.8.1 permits CRLF after the "stream" keyword and most
// producers emit it. An LF-only match skips the object's own stream and the
// unbounded forward scan then finds the NEXT object's — so the checker reads
// bytes from an object a reader never resolves for this reference.
func TestObjectStreamLookupReadsACRLFStreamAndNeverALaterObject(t *testing.T) {
	pdf := pdfWithObjects(
		"19 0 obj\r\n<< /Length 4 >>\r\nstream\r\nEVIL\r\nendstream\r\nendobj\r\n",
		"20 0 obj\n<< /Length 6 >>\nstream\nBENIGN\nendstream\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != "EVIL" {
		t.Fatalf("got %q, want object 19's own CRLF stream", content)
	}
}

// The same clipping stated as its own rule: an object with no stream at all
// must be reported as having none, not answered with the next object's bytes.
func TestObjectStreamLookupIsClippedAtTheObjectsOwnEndobj(t *testing.T) {
	pdf := pdfWithObjects(
		"19 0 obj\n<< /Type /Page >>\nendobj\n",
		"20 0 obj\n<< /Length 5 >>\nstream\nLATER\nendstream\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err == nil {
		t.Fatalf("object 19 has no stream, got %q from a later object", content)
	}
	if !strings.Contains(err.Error(), "no stream") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The C2PA hard binding excludes the manifest's own stream from the hash. A
// range taken from a later object excludes the wrong bytes, so the manifest
// covers content nobody signed and leaves its own bytes hashed.
func TestC2PAExclusionRangeIsTheObjectsOwnStream(t *testing.T) {
	pdf := pdfWithObjects(
		"9  0 obj\n<< /Length 8 >>\nstream\nMANIFEST\nendstream\nendobj\n",
		"20 0 obj\n<< /Length 5 >>\nstream\nLATER\nendstream\nendobj\n",
	)

	start, length, ok := findLastObjectStreamRange(pdf, 9)
	if !ok {
		t.Fatal("object 9's stream range must be found")
	}
	if got := string(pdf[start : start+length]); got != "MANIFEST" {
		t.Fatalf("got %q, want the manifest object's own stream", got)
	}
}

// A stream's data is arbitrary bytes and may say "endstream" or "endobj". Its
// extent comes from /Length, so the data's own words end nothing.
func TestStreamDataMaySayEndstreamAndEndobj(t *testing.T) {
	data := "line one endstream\nline two endobj\nline three"
	pdf := pdfWithObjects(
		fmt.Sprintf("19 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(data), data),
		"20 0 obj\n<< /Length 5 >>\nstream\nLATER\nendstream\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != data {
		t.Fatalf("got %q, want the whole stream", content)
	}
}

// A name object can never be a keyword: "/stream" is a dictionary key. The
// stream keyword is only the one standing where a stream may begin — directly
// after the dictionary.
func TestADictionaryKeyNamedStreamIsNotTheStreamKeyword(t *testing.T) {
	pdf := pdfWithObjects(
		"19 0 obj\n<< /stream\n(decoy) /Length 4 >>\nstream\nDATA\nendstream\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != "DATA" {
		t.Fatalf("got %q, want the object's own stream data", content)
	}
}

// 7.3.8.2 permits /Length to be an indirect reference, and the object it names
// is the only thing that says where the stream ends.
func TestStreamLengthMayBeAnIndirectReference(t *testing.T) {
	pdf := pdfWithObjects(
		"19 0 obj\n<< /Length 30 0 R >>\nstream\nDATA endstream\nendstream\nendobj\n",
		"30 0 obj\n14\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != "DATA endstream" {
		t.Fatalf("got %q, want the length the referenced object declares", content)
	}
}

// A /Length1 entry precedes /Length in the embedded font object's sibling
// spelling; a prefix match on the key reads the wrong number.
func TestStreamLengthIsNotTakenFromASimilarlyNamedKey(t *testing.T) {
	pdf := pdfWithObjects(
		"19 0 obj\n<< /Length1 99 /Length 4 /Params << /Length 77 >> >>\nstream\nDATA\nendstream\nendobj\n",
	)

	content, err := extractStreamContentByObjID(pdf, 19)
	if err != nil {
		t.Fatalf("extract object 19: %v", err)
	}
	if string(content) != "DATA" {
		t.Fatalf("got %q, want the top-level /Length", content)
	}
}

// A declared length that does not land on "endstream" describes no stream any
// reader can resolve. Saying so is the only honest answer: guessing hands a
// checker bytes the document does not contain.
func TestAStreamWhoseLengthDoesNotReachEndstreamIsRefused(t *testing.T) {
	pdf := pdfWithObjects(
		"19 0 obj\n<< /Length 2 >>\nstream\nDATA\nendstream\nendobj\n",
	)

	if content, err := extractStreamContentByObjID(pdf, 19); err == nil {
		t.Fatalf("a length inconsistent with the object must be refused, got %q", content)
	}
}

// The writer emits `data + "\nendstream"`, so a reader that trims an
// end-of-line off the DATA takes a byte that belongs to the stream whenever the
// data itself ends in one. For a binary C2PA manifest that is one document in
// 256, and the amendment path then fails to parse its own manifest.
func TestStreamDataEndingInAnEndOfLineKeepsItsLastByte(t *testing.T) {
	for name, data := range map[string]string{
		"carriage return": "MANIFEST\r",
		"line feed":       "MANIFEST\n",
	} {
		t.Run(name, func(t *testing.T) {
			pdf := pdfWithObjects(fmt.Sprintf("9 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(data), data))

			content, err := extractStreamContentByObjID(pdf, 9)
			if err != nil {
				t.Fatalf("extract object 9: %v", err)
			}
			if string(content) != data {
				t.Fatalf("got %q (%d bytes), want %q (%d bytes)", content, len(content), data, len(data))
			}
		})
	}
}
