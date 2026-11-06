package compiler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// pdfBoxDict writes a dictionary the way PDFBox does: each "<<" and ">>" on a
// line of its own.
func pdfBoxDict(dict string) string {
	return strings.NewReplacer("<< ", "<<\n", " >>", "\n>>").Replace(dict)
}

// appendXRefStreamRevision appends a revision the way PDFBox writes it over a
// document whose last section is a cross-reference stream (PDF 1.5): the
// objects in PDFBox dictionary layout, then a /Type /XRef stream that holds the
// trailer entries. The revision has no "trailer" keyword.
func appendXRefStreamRevision(t *testing.T, pdf []byte, objs map[int]string) []byte {
	t.Helper()
	prevStartXref, err := previousStartXref(pdf)
	if err != nil {
		t.Fatalf("previousStartXref: %v", err)
	}
	maxObjID, err := findTrailerMaxObjID(pdf)
	if err != nil {
		t.Fatalf("findTrailerMaxObjID: %v", err)
	}
	root, ok := currentRootObjID(pdf)
	if !ok {
		t.Fatal("no /Root in the trailer")
	}
	xrefID := maxObjID + 1
	for id := range objs {
		if id >= xrefID {
			xrefID = id + 1
		}
	}
	ids := make([]int, 0, len(objs)+1)
	for id := range objs {
		ids = append(ids, id)
	}
	ids = append(ids, xrefID)
	sort.Ints(ids)

	base := len(pdf)
	var buf bytes.Buffer
	buf.WriteString("\n")
	offsets := map[int]int{}
	for _, id := range ids[:len(ids)-1] {
		offsets[id] = base + buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, pdfBoxDict(objs[id]))
	}
	offsets[xrefID] = base + buf.Len()
	var rows bytes.Buffer
	index := make([]string, 0, len(ids))
	for _, id := range ids {
		rows.WriteByte(1)
		_ = binary.Write(&rows, binary.BigEndian, uint32(offsets[id]))
		_ = binary.Write(&rows, binary.BigEndian, uint16(0))
		index = append(index, fmt.Sprintf("%d 1", id))
	}
	idEntry := ""
	if fileID := extractTrailerID(pdf); fileID != "" {
		idEntry = "\n/ID " + fileID
	}
	fmt.Fprintf(&buf, "%d 0 obj\n<<\n/Type /XRef\n/Size %d\n/Root %d 0 R\n/Prev %d%s\n/W [1 4 2]\n/Index [%s]\n/Length %d\n>>\nstream\n",
		xrefID, xrefID+1, root, prevStartXref, idEntry, strings.Join(index, " "), rows.Len())
	buf.Write(rows.Bytes())
	fmt.Fprintf(&buf, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", offsets[xrefID])
	return append(append([]byte(nil), pdf...), buf.Bytes()...)
}

// appendXRefStreamDSSRevision is appendDSSRevision written by PDFBox over a
// document with cross-reference streams.
func appendXRefStreamDSSRevision(t *testing.T, pdf []byte) []byte {
	t.Helper()
	maxObjID, err := findTrailerMaxObjID(pdf)
	if err != nil {
		t.Fatalf("findTrailerMaxObjID: %v", err)
	}
	root, _ := currentRootObjID(pdf)
	crlID, dssID := maxObjID+1, maxObjID+2
	crl := "revocation-material"
	catalog := strings.TrimSpace(objectBody(t, pdf, root))
	newCatalog := strings.TrimSuffix(catalog, ">>") + fmt.Sprintf("/DSS %d 0 R >>", dssID)
	return appendXRefStreamRevision(t, pdf, map[int]string{
		root:  newCatalog,
		crlID: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(crl), crl),
		dssID: fmt.Sprintf("<< /Type /DSS /CRLs [%d 0 R] >>", crlID),
	})
}

// signedContractPDF is a prepared contract with a PAdES signature appended.
func signedContractPDF(t *testing.T) []byte {
	t.Helper()
	return appendPAdESRevision(t, preparedContractPDF(t), "Signature1")
}

// definedObjectIDs returns the id of every "N 0 obj" header in pdf with the
// number of definitions.
func definedObjectIDs(pdf []byte) map[string]int {
	ids := map[string]int{}
	for _, m := range regexp.MustCompile(`(?m)^(\d+) 0 obj`).FindAllSubmatch(pdf, -1) {
		ids[string(m[1])]++
	}
	return ids
}

// ADR-40: DSS appends the long-term validation data (a /DSS dictionary and a
// document timestamp) before finalize re-anchors the provenance. Appended in
// that order the provenance chain verifies. Appended after the re-anchor, the
// last manifest's hard binding no longer covers the file.
func TestLongTermValidationRevisionMustPrecedeTheReanchor(t *testing.T) {
	for name, appendDSS := range map[string]func(*testing.T, []byte) []byte{
		"classic cross-reference": appendDSSRevision,
		"cross-reference stream":  appendXRefStreamDSSRevision,
	} {
		t.Run(name, func(t *testing.T) {
			extended := appendDSS(t, signedContractPDF(t))
			reanchored, err := ReanchorProvenance(testSigningContext(), extended, "", time.Now())
			if err != nil {
				t.Fatalf("ReanchorProvenance: %v", err)
			}
			if _, err := VerifyIncrementalUpdate(testSigningContext(), reanchored); err != nil {
				t.Fatalf("DSS revision before the re-anchor: %v", err)
			}

			late, err := ReanchorProvenance(testSigningContext(), signedContractPDF(t), "", time.Now())
			if err != nil {
				t.Fatalf("ReanchorProvenance: %v", err)
			}
			if _, err := VerifyIncrementalUpdate(testSigningContext(), appendDSS(t, late)); err == nil {
				t.Fatal("a DSS revision after the re-anchor verified, but the hard binding does not cover it")
			}
		})
	}
}

// A lifecycle update after the long-term validation data (e.g. the revoked
// credential) keeps the /DSS the catalog references, numbers its objects past
// every object DSS added, and lists its credential in /EmbeddedFiles. It holds
// for a classic section and for a PDFBox cross-reference stream with
// multi-line dictionaries.
func TestLifecycleUpdateAfterTheLongTermValidationRevision(t *testing.T) {
	for name, appendDSS := range map[string]func(*testing.T, []byte) []byte{
		"classic cross-reference": appendDSSRevision,
		"cross-reference stream":  appendXRefStreamDSSRevision,
	} {
		t.Run(name, func(t *testing.T) {
			extended := appendDSS(t, signedContractPDF(t))
			reanchored, err := ReanchorProvenance(testSigningContext(), extended, "", time.Now())
			if err != nil {
				t.Fatalf("ReanchorProvenance: %v", err)
			}
			payload, err := ExtractLatestEmbeddedJSONLD(reanchored)
			if err != nil {
				t.Fatalf("ExtractLatestEmbeddedJSONLD: %v", err)
			}
			vc := []byte(`{"credentialSubject":{"status":"revoked"}}`)

			updated, err := UpdatePDFWithVC(testSigningContext(), reanchored, payload, vc, time.Now())
			if err != nil {
				t.Fatalf("UpdatePDFWithVC: %v", err)
			}

			root, ok := currentRootObjID(updated)
			if !ok {
				t.Fatal("no /Root in the last trailer")
			}
			catalog := objectBody(t, updated, root)
			if !regexp.MustCompile(`/DSS\s+\d+ 0 R`).MatchString(catalog) {
				t.Fatalf("the catalog lost /DSS: %s", catalog)
			}
			if !strings.Contains(catalog, "(contract-lifecycle-vc.json)") {
				t.Fatalf("/EmbeddedFiles does not list the credential: %s", catalog)
			}
			for id, count := range definedObjectIDs(updated[len(reanchored):]) {
				if count > 1 || (definedObjectIDs(reanchored)[id] > 0 && id != "9" && id != "11" && id != fmt.Sprint(root)) {
					t.Errorf("the lifecycle update redefines object %s that it does not supersede", id)
				}
			}
			got, present, err := ExtractEmbeddedVC(updated)
			if err != nil || !present || !bytes.Equal(got, vc) {
				t.Fatalf("ExtractEmbeddedVC = %q, %v, %v", got, present, err)
			}
			if _, err := VerifyIncrementalUpdate(testSigningContext(), updated); err != nil {
				t.Fatalf("VerifyIncrementalUpdate: %v", err)
			}
		})
	}
}

// The trailer readers take the trailer of the last section the last startxref
// names. dss-pades-b-t.pdf is a document DSS 6.2.1 signed at PAdES-B-T: its
// signature revision is PDFBox output, a classic section with a multi-line
// trailer dictionary.
func TestTrailerReadersReadADSSSignedRevision(t *testing.T) {
	pdf, err := os.ReadFile("testdata/dss-pades-b-t.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if maxID, err := findTrailerMaxObjID(pdf); err != nil || maxID != 7 {
		t.Fatalf("findTrailerMaxObjID = %d, %v, want 7", maxID, err)
	}
	if root, ok := currentRootObjID(pdf); !ok || root != 1 {
		t.Fatalf("currentRootObjID = %d, %v, want 1", root, ok)
	}
	if id := extractTrailerID(pdf); id != "[<67C514BD8C6D141D13A6E32C93169BAD> <67C514BD8C6D141D13A6E32C93169BAD>]" {
		t.Fatalf("extractTrailerID = %q", id)
	}
}

// catalogWithAssociatedFile lists the credential in a catalog that PDFBox
// wrote with multi-line dictionaries.
func TestCatalogWithAssociatedFileReadsAPDFBoxCatalog(t *testing.T) {
	catalog := pdfBoxDict("<< /Type /Catalog /Pages 2 0 R /AF [10 0 R] " +
		"/Names << /EmbeddedFiles << /Names [(content_credential.c2pa) 10 0 R] >> >> /DSS 30 0 R >>")
	pdf := []byte("%PDF-1.7\n1 0 obj\n" + catalog + "\nendobj\n")

	dict, err := catalogWithAssociatedFile(pdf, 1, 42, lifecycleVCFileName)
	if err != nil {
		t.Fatalf("catalogWithAssociatedFile: %v", err)
	}
	if !regexp.MustCompile(`/AF \[10 0 R 42 0 R\]`).Match(dict) {
		t.Errorf("/AF does not list the credential: %s", dict)
	}
	if !bytes.Contains(dict, []byte("(contract-lifecycle-vc.json) 42 0 R")) {
		t.Errorf("/EmbeddedFiles does not list the credential: %s", dict)
	}
	if !bytes.Contains(dict, []byte("/DSS 30 0 R")) {
		t.Errorf("the catalog lost /DSS: %s", dict)
	}
}
