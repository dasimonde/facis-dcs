package design

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The design carries Meta("dcs:requirements", …) annotations naming the SRS
// requirements an endpoint implements. Goa drops unrecognised Meta keys, so
// nothing in the generated output or at runtime depends on them: this test is
// the only thing that keeps them true. Without it an annotation survives a
// requirement being renumbered, and reads as coverage evidence it is not.
//
// The SRS text is extracted from the PDF and splits identifiers across a
// space ("[DCS-NFR- COMP-03]"), so both sides are compared with whitespace
// removed.

var requirementsMeta = regexp.MustCompile(`Meta\("dcs:requirements",\s*([^)]*)\)`)

var requirementID = regexp.MustCompile(`"([^"]+)"`)

// citedRequirements returns every requirement ID the design annotates, mapped
// to the files citing it.
func citedRequirements(t *testing.T) map[string][]string {
	t.Helper()
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list design sources: %v", err)
	}
	cited := map[string][]string{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", source, err)
		}
		for _, call := range requirementsMeta.FindAllSubmatch(content, -1) {
			for _, id := range requirementID.FindAllSubmatch(call[1], -1) {
				requirement := strings.TrimSpace(string(id[1]))
				cited[requirement] = append(cited[requirement], source)
			}
		}
	}
	if len(cited) == 0 {
		t.Fatal("no dcs:requirements annotations found — the regex no longer matches the design")
	}
	return cited
}

func TestEveryCitedRequirementResolvesInTheSRS(t *testing.T) {
	srs, err := os.ReadFile(filepath.Join("..", "..", "docs", "SRS_FACIS_DCS.txt"))
	if err != nil {
		t.Fatalf("read SRS: %v", err)
	}
	flattened := strings.Join(strings.Fields(string(srs)), "")

	for requirement, sources := range citedRequirements(t) {
		if !strings.Contains(flattened, requirement) {
			t.Errorf("%s is cited by %s but appears nowhere in docs/SRS_FACIS_DCS.txt",
				requirement, strings.Join(sources, ", "))
		}
	}
}
