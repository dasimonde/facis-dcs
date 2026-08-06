package compiler

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// signingEvidenceFileName is the attachment filename of the FIRST signing
// evidence embedded into a document — the SD-JWT VC PID presentation +
// ContractSigningSummaryCredential a party attaches before its PAdES signature
// is applied (embed-first-sign-second, DCS-FR-SM-08).
//
// Every signing party in a DCS-to-DCS federation attaches its own evidence, so
// a document accumulates one attachment per signer. They are named by 1-based
// ordinal — signing-evidence.json, signing-evidence-2.json, … — because an
// /EmbeddedFiles name tree holds exactly one entry per name: re-using one name
// would leave every earlier attachment unreachable by name.
const signingEvidenceFileName = "signing-evidence.json"

var signingEvidenceMarker = []byte("\n% dcs-pdf-core signing evidence\n")

// signingEvidenceSpecRE matches the /F entry of a signing-evidence filespec,
// capturing the filename and its ordinal suffix (absent on the first one).
var signingEvidenceSpecRE = regexp.MustCompile(`/F \(signing-evidence(?:-(\d+))?\.json\)`)

// signingEvidenceName returns the attachment filename for a 1-based ordinal.
func signingEvidenceName(ordinal int) string {
	if ordinal <= 1 {
		return signingEvidenceFileName
	}
	return fmt.Sprintf("signing-evidence-%d.json", ordinal)
}

// EmbedSigningEvidence appends a PDF incremental update that attaches evidence
// as an embedded file. The original bytes are preserved as a prefix so a
// subsequently applied PAdES signature's ByteRange covers the evidence. When
// evidence is empty the PDF is returned unchanged.
//
// The call is repeatable: each call attaches one more evidence file, under the
// next ordinal name, over whatever revisions the document has accumulated in
// the meantime — the counterparty's evidence goes into a PDF that already
// carries the first party's signature, validation data and C2PA manifests.
func EmbedSigningEvidence(pdfBytes, evidence []byte) ([]byte, error) {
	if len(evidence) == 0 {
		return pdfBytes, nil
	}

	maxObjID, err := findTrailerMaxObjID(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("embed evidence: find max object ID: %w", err)
	}
	prevStartXref, err := previousStartXref(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("embed evidence: find startxref: %w", err)
	}
	fileID := extractTrailerID(pdfBytes)

	baseLen := len(pdfBytes)
	fileObjID := maxObjID + 1
	specObjID := maxObjID + 2
	fileName := signingEvidenceName(len(signingEvidenceSpecs(pdfBytes)) + 1)

	// The attachment carries /AFRelationship, so ISO 19005-3 clause 6.8 also
	// requires it to be a LISTED associated file: without membership of the
	// document catalog's /AF array veraPDF rejects the PDF ("file specification
	// dictionary for an embedded file is not associated with the PDF document").
	// Supersede the catalog in this same incremental update, exactly as the
	// lifecycle-VC attachment does. The catalog is the one the trailer names,
	// not object 1: a signer may publish a fresh catalog object, and patching a
	// definition no reader resolves would list the attachment nowhere and drop
	// whatever that catalog carries (the /AcroForm holding the signed field).
	catalogObjID, ok := currentRootObjID(pdfBytes)
	if !ok {
		return nil, fmt.Errorf("embed evidence: no /Root in the PDF trailer")
	}
	patchedCatalog, err := catalogWithAssociatedFile(pdfBytes, catalogObjID, specObjID, fileName)
	if err != nil {
		return nil, fmt.Errorf("embed evidence: associate attachment with the document: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(signingEvidenceMarker)

	fileOffset := baseLen + buf.Len()
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", fileObjID))
	buf.Write(streamObject(evidence, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fjson /Length %d >>", len(evidence))))
	buf.WriteString("\nendobj\n")

	specOffset := baseLen + buf.Len()
	buf.WriteString(fmt.Sprintf(
		"%d 0 obj\n<< /Type /Filespec /F (%s) /UF (%s) /AFRelationship /Supplement /EF << /F %d 0 R >> >>\nendobj\n",
		specObjID, fileName, fileName, fileObjID))

	catalogOffset := baseLen + buf.Len()
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", catalogObjID))
	buf.Write(patchedCatalog)
	buf.WriteString("\nendobj\n")

	xrefStart := baseLen + buf.Len()
	buf.WriteString("xref\n")
	// The superseded catalog is a separate subsection from the two appended
	// objects, which always follow it: their ids start past the current maximum.
	buf.WriteString(fmt.Sprintf("%d 1\n", catalogObjID))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", catalogOffset))
	buf.WriteString(fmt.Sprintf("%d 2\n", fileObjID))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", fileOffset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", specOffset))

	idEntry := ""
	if fileID != "" {
		idEntry = " /ID " + fileID
	}
	buf.WriteString(fmt.Sprintf(
		"trailer\n<< /Size %d /Root %d 0 R /Prev %d%s >>\nstartxref\n%d\n%%%%EOF\n",
		specObjID+1, catalogObjID, prevStartXref, idEntry, xrefStart))

	return append(append([]byte(nil), pdfBytes...), buf.Bytes()...), nil
}

// evidenceSpec locates one signing-evidence filespec: its ordinal and the
// offset of its /F entry.
type evidenceSpec struct {
	ordinal int
	offset  int
}

// signingEvidenceSpecs returns the current filespec of every signing-evidence
// attachment in the document, ordered by ordinal — the order they were
// embedded, oldest first. A name defined more than once resolves to its last
// definition, as a reader resolves a superseded object.
func signingEvidenceSpecs(pdf []byte) []evidenceSpec {
	byOrdinal := map[int]int{}
	for _, m := range signingEvidenceSpecRE.FindAllSubmatchIndex(pdf, -1) {
		ordinal := 1
		if m[2] >= 0 {
			n, err := strconv.Atoi(string(pdf[m[2]:m[3]]))
			if err != nil {
				continue
			}
			ordinal = n
		}
		byOrdinal[ordinal] = m[0]
	}
	specs := make([]evidenceSpec, 0, len(byOrdinal))
	for ordinal, offset := range byOrdinal {
		specs = append(specs, evidenceSpec{ordinal: ordinal, offset: offset})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ordinal < specs[j].ordinal })
	return specs
}

// ExtractSigningEvidence returns the raw bytes of every evidence attachment
// EmbedSigningEvidence put into the document, oldest first — one per signing
// party. Returns an empty slice when the document carries none, and an error on
// a malformed reference.
func ExtractSigningEvidence(pdfBytes []byte) ([][]byte, error) {
	specs := signingEvidenceSpecs(pdfBytes)
	attachments := make([][]byte, 0, len(specs))
	for _, spec := range specs {
		fileName := signingEvidenceName(spec.ordinal)
		efPos := bytes.Index(pdfBytes[spec.offset:], []byte("/EF << /F "))
		if efPos < 0 {
			return nil, fmt.Errorf("%s filespec missing /EF reference", fileName)
		}
		efPos += spec.offset + len("/EF << /F ")
		refEnd := bytes.Index(pdfBytes[efPos:], []byte(" 0 R"))
		if refEnd < 0 {
			return nil, fmt.Errorf("%s object reference malformed", fileName)
		}
		objID, err := strconv.Atoi(strings.TrimSpace(string(pdfBytes[efPos : efPos+refEnd])))
		if err != nil {
			return nil, fmt.Errorf("%s object id invalid: %w", fileName, err)
		}
		streamStart, streamEnd, ok := lastObjectStreamData(pdfBytes, objID)
		if !ok {
			return nil, fmt.Errorf("%s stream not found in object %d", fileName, objID)
		}
		attachments = append(attachments, append([]byte(nil), pdfBytes[streamStart:streamEnd]...))
	}
	return attachments, nil
}
