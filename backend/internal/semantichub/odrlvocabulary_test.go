package semantichub

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tggo/goRDFlib/shacl"
)

// The authoring UI offers ODRL terms from a hand-maintained TypeScript list
// while the clause catalog enforces them with sh:in. When the two disagree the
// picker offers a term no derived contract can ever satisfy, and the author
// only finds out days later — on another instance — as a SHACL constraint
// component name. These tests pin the two lists equal, per rule type, so the
// disagreement cannot be reintroduced by editing one side.

const (
	odrlNS = "http://www.w3.org/ns/odrl/2/"
	dcsNS  = "https://w3id.org/facis/dcs/ontology/v1#"
)

func TestODRLActionVocabularyMatchesAuthoringUI(t *testing.T) {
	graph := loadClauseCatalog(t)
	offered := parseFrontendTermIDs(t, "ODRL_ACTIONS")

	// Every rule type draws from the same vocabulary: ODRL 2.2 attaches no
	// action restriction to a rule's deontic type.
	for _, shape := range []string{"OdrlObligationShape", "OdrlPermissionShape", "OdrlProhibitionShape"} {
		permitted := permittedValues(t, graph, dcsNS+shape, odrlNS+"action")
		require.Equal(t, offered, permitted,
			"%s permits a different odrl:action set than the authoring UI offers; a term the picker "+
				"offers but the shapes reject produces a template no contract can satisfy", shape)
	}
}

func TestODRLOperatorVocabularyMatchesAuthoringUI(t *testing.T) {
	graph := loadClauseCatalog(t)
	offered := parseFrontendTermIDs(t, "ODRL_OPERATORS")
	permitted := permittedValues(t, graph, dcsNS+"OdrlConstraintShape", odrlNS+"operator")
	require.Equal(t, offered, permitted,
		"the constraint shape permits a different odrl:operator set than the authoring UI offers")
}

// TestODRLActionVocabularyIsClosed keeps sh:in meaningful: an IRI that is
// neither ODRL core nor profile-declared is still refused, so widening the
// vocabulary did not turn the constraint into a rubber stamp.
func TestODRLActionVocabularyIsClosed(t *testing.T) {
	permitted := permittedValues(t, loadClauseCatalog(t), dcsNS+"OdrlObligationShape", odrlNS+"action")
	require.NotContains(t, permitted, "https://evil.example/never-declared-action")
	require.Contains(t, permitted, "odrl:compensate", "the ODRL specification's archetypal Duty action")
	require.Contains(t, permitted, "dcs:provideCompliantValue", "the DCS profile's own action")
}

// permittedValues expands the sh:in list constraining path on shape, as
// compacted odrl:/dcs: identifiers sorted for comparison.
func permittedValues(t *testing.T, graph *shacl.Graph, shapeIRI, pathIRI string) []string {
	t.Helper()
	var values []string
	for _, property := range graph.Objects(shacl.IRI(shapeIRI), shacl.IRI(shacl.SH+"property")) {
		paths := graph.Objects(property, shacl.IRI(shacl.SH+"path"))
		if len(paths) != 1 || paths[0].Value() != pathIRI {
			continue
		}
		heads := graph.Objects(property, shacl.IRI(shacl.SH+"in"))
		require.Len(t, heads, 1, "expected exactly one sh:in on %s %s", shapeIRI, pathIRI)
		for _, member := range expandRDFList(t, graph, heads[0]) {
			values = append(values, compactTerm(member.Value()))
		}
	}
	require.NotEmpty(t, values, "no sh:in found for %s on %s", pathIRI, shapeIRI)
	sort.Strings(values)
	return values
}

// expandRDFList walks rdf:first/rdf:rest from a list head, which may be a
// blank node or (as with the shared dcs:odrlActionVocabulary) a named one.
func expandRDFList(t *testing.T, graph *shacl.Graph, head shacl.Term) []shacl.Term {
	t.Helper()
	var members []shacl.Term
	for node := head; node.Value() != shacl.RDFNil; {
		first := graph.Objects(node, shacl.IRI(shacl.RDFFirst))
		require.Len(t, first, 1, "malformed RDF list at %s", node.Value())
		members = append(members, first[0])

		rest := graph.Objects(node, shacl.IRI(shacl.RDFRest))
		require.Len(t, rest, 1, "malformed RDF list at %s", node.Value())
		node = rest[0]
		require.Less(t, len(members), 500, "RDF list does not terminate")
	}
	return members
}

func compactTerm(iri string) string {
	switch {
	case strings.HasPrefix(iri, odrlNS):
		return "odrl:" + strings.TrimPrefix(iri, odrlNS)
	case strings.HasPrefix(iri, dcsNS):
		return "dcs:" + strings.TrimPrefix(iri, dcsNS)
	default:
		return iri
	}
}

func loadClauseCatalog(t *testing.T) *shacl.Graph {
	t.Helper()
	// The embedded asset is the authoring source Seed installs, so testing it
	// tests exactly what a deployment will serve.
	graph, err := shacl.LoadTurtleString(string(genesisClauseCatalog), "urn:dcs:hub:clause-catalog")
	require.NoError(t, err)
	return graph
}

var frontendTermIDPattern = regexp.MustCompile(`id:\s*'([^']+)'`)

// parseFrontendTermIDs reads the ids of one exported OdrlTerm[] out of the
// authoring UI's vocabulary module. Deliberately textual: the point is to fail
// when a developer edits that TypeScript file without editing the shapes.
func parseFrontendTermIDs(t *testing.T, constName string) []string {
	t.Helper()
	source := readRepoFile(t, filepath.Join(
		"frontend", "ClientApp", "src", "modules", "template-repository", "utils", "odrl-vocabulary.ts"))

	start := strings.Index(source, "export const "+constName+": OdrlTerm[] = [")
	require.GreaterOrEqual(t, start, 0, "%s not found in odrl-vocabulary.ts", constName)
	end := strings.Index(source[start:], "\n]")
	require.Greater(t, end, 0, "unterminated %s literal", constName)

	var ids []string
	for _, match := range frontendTermIDPattern.FindAllStringSubmatch(source[start:start+end], -1) {
		ids = append(ids, match[1])
	}
	require.NotEmpty(t, ids, "no term ids parsed from %s", constName)
	sort.Strings(ids)
	return ids
}

// readRepoFile resolves a repository-relative path from wherever `go test`
// happens to be running.
func readRepoFile(t *testing.T, relPath string) string {
	t.Helper()
	for _, prefix := range []string{".", "..", "../..", "../../..", "../../../.."} {
		if data, err := os.ReadFile(filepath.Join(prefix, relPath)); err == nil {
			return string(data)
		}
	}
	t.Fatalf("could not locate %s from the test working directory", relPath)
	return ""
}
