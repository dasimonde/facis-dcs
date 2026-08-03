package compiler

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// deepNestPDF is one object whose dictionary holds `depth` nested arrays. It is
// the shape a peer can put on the wire: dcs_to_dcs hands a peer's raw PDF to
// VerifyContent before any of our own validation runs.
func deepNestPDF(depth int) []byte {
	var b bytes.Buffer
	b.WriteString("1 0 obj\n<< /K ")
	b.Write(bytes.Repeat([]byte("["), depth))
	b.Write(bytes.Repeat([]byte("]"), depth))
	b.WriteString(" /Length 3 >>\nstream\nabc\nendstream\nendobj\n")
	return b.Bytes()
}

// A /Length of math.MaxInt64 made the bounds check `pos+length > len(pdf)` wrap
// negative, so the guard passed and the slice index below it panicked with
// "index out of range [-9223372036854775751]". Reachable on every peer PDF via
// /verify/content and /evidence/extract, and inside SubmitSignature's content
// gate, where net/http turns the panic into a connection reset instead of a 422.
func TestStreamLengthNearMaxIntIsRejectedNotOverflowed(t *testing.T) {
	for _, length := range []string{
		"9223372036854775807",
		"9223372036854775806",
		"4611686018427387904",
	} {
		pdf := []byte("1 0 obj\n<< /Length " + length + " >>\nstream\nx\nendstream\nendobj\n")
		header, ok := lastObjectHeader(pdf, 1)
		if !ok {
			t.Fatalf("/Length %s: the object itself must still resolve", length)
		}
		if header.hasStream {
			t.Errorf("/Length %s: reported a stream of [%d,%d) in a %d-byte file",
				length, header.streamStart, header.streamEnd, len(pdf))
		}
	}
}

// The same overflow through the gate it actually fires in: an appended
// incremental update superseding the page content object keeps
// bytes.HasPrefix(submitted, prepared) true, so the byte-prefix pin passes it
// through to MatchPageContent.
func TestMatchPageContentSurvivesAnOverflowingLengthInAnAppendedRevision(t *testing.T) {
	prepared, err := CompilePDF(testSigningContext(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	contentID, err := currentPageContentObjID(prepared)
	if err != nil {
		t.Fatalf("locating the page content object: %v", err)
	}
	crafted := append(append([]byte(nil), prepared...),
		[]byte(fmt.Sprintf("\n%d 0 obj\n<< /Length 9223372036854775807 >>\nstream\nx\nendstream\nendobj\n", contentID))...)

	if !bytes.HasPrefix(crafted, prepared) {
		t.Fatal("the append must preserve the prepared prefix, or the gate never reaches MatchPageContent")
	}
	if err := MatchPageContent(crafted, prepared); err == nil {
		t.Error("a superseding object with an unsatisfiable /Length must be rejected, not matched")
	}
}

// currentPageContentObjID returns the object id of the first page's content
// stream, the object an attacker supersedes to reach the content gate.
func currentPageContentObjID(pdf []byte) (int, error) {
	ranges, err := pageContentStreamRanges(pdf)
	if err != nil {
		return 0, err
	}
	if len(ranges) == 0 {
		return 0, fmt.Errorf("no page content streams")
	}
	return ranges[0].objID, nil
}

// Past maxObjectNesting the walk fails closed: the definition is reported
// absent, so no caller is handed an object extent spanning the rest of the file.
func TestObjectNestedPastTheLimitFailsClosed(t *testing.T) {
	if _, ok := lastObjectHeader(deepNestPDF(maxObjectNesting+1), 1); ok {
		t.Errorf("an object nested %d deep must not resolve", maxObjectNesting+1)
	}
	header, ok := lastObjectHeader(deepNestPDF(maxObjectNesting-1), 1)
	if !ok || !header.hasStream {
		t.Errorf("an object nested %d deep is within the limit and must resolve with its stream", maxObjectNesting-1)
	}
}

// The limit's justification: what this compiler actually emits. If a future
// change nests objects anywhere near maxObjectNesting, this fails first.
func TestObjectNestingOfARealPDFIsFarBelowTheLimit(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	deepest := 0
	for objID := 1; objID < 200; objID++ {
		for _, definition := range objectDefinitions(pdf, objID) {
			if d := nestingDepth(pdf, skipPDFSpace(pdf, definition.body)); d > deepest {
				deepest = d
			}
		}
	}
	if deepest == 0 {
		t.Fatal("measured no nesting at all — the walk found no objects")
	}
	if deepest > maxObjectNesting/8 {
		t.Errorf("a compiled PDF nests %d deep, too close to the limit of %d", deepest, maxObjectNesting)
	}
	t.Logf("deepest object nesting in a compiled PDF: %d (limit %d)", deepest, maxObjectNesting)
}

// nestingDepth returns how deeply the object at pos nests composites.
func nestingDepth(pdf []byte, pos int) int {
	if pos >= len(pdf) {
		return 0
	}
	var closer []byte
	switch {
	case pdf[pos] == '(':
		return 0
	case bytes.HasPrefix(pdf[pos:], dictOpen):
		pos, closer = pos+len(dictOpen), dictClose
	case pdf[pos] == '<':
		return 0
	case pdf[pos] == '[':
		pos, closer = pos+1, arrayClose
	default:
		return 0
	}
	deepest := 0
	for pos < len(pdf) {
		pos = skipPDFSpace(pdf, pos)
		if pos >= len(pdf) || bytes.HasPrefix(pdf[pos:], closer) {
			break
		}
		if d := nestingDepth(pdf, pos); d > deepest {
			deepest = d
		}
		next, ok := skipPDFObject(pdf, pos, 0)
		if !ok || next <= pos {
			break
		}
		pos = next
	}
	return deepest + 1
}

const deepNestChildEnv = "PDFCORE_DEEP_NEST_CHILD_DEPTH"

// A stack overflow is a runtime throw, not a panic: net/http's per-connection
// recover cannot catch it, so an unbounded walk takes the whole process down
// and every in-flight request with it. Proving that requires observing a
// process, so the walk runs in a re-executed child with a lowered stack limit —
// at which the overflow reproduces from a 200 KB input instead of an 8 MB one.
func TestDeeplyNestedObjectDoesNotKillTheProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestDeepNestChild$", "-test.v")
	cmd.Env = append(os.Environ(), deepNestChildEnv+"=100000")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("walking a deeply nested object killed the process (%v):\n%s", err, firstLines(string(out), 6))
	}
}

// TestDeepNestChild is the child half of the test above: it lowers the stack
// limit and walks the nested object in-process.
func TestDeepNestChild(t *testing.T) {
	depth := os.Getenv(deepNestChildEnv)
	if depth == "" {
		t.Skip("child of TestDeeplyNestedObjectDoesNotKillTheProcess")
	}
	debug.SetMaxStack(1 << 22)
	var n int
	if _, err := fmt.Sscanf(depth, "%d", &n); err != nil {
		t.Fatalf("%s=%q: %v", deepNestChildEnv, depth, err)
	}
	if _, ok := lastObjectHeader(deepNestPDF(n), 1); ok {
		t.Errorf("an object nested %d deep must not resolve", n)
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
