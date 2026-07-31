package template

import (
	"fmt"
	"strings"

	"digital-contracting-service/internal/fcasset"
	"digital-contracting-service/internal/templatecatalogueintegration/internal/ptr"
)

// credentialSubjectIRI is the VC 2.0 term FC annotates every ingested triple
// with, pointing back at the credentialSubject that asserted it. FC's Fuseki
// backend stores claims as RDF-star reified triples
// (<<(?s ?p ?o)>> credentialSubjectIRI ?s), not plain triples — see
// eu.xfsc.fc.graphdb.service.SparqlGraphStore in the upstream FC source.
const credentialSubjectIRI = "https://www.w3.org/2018/credentials#credentialSubject"

var (
	dcsTemplateUUIDIRI       = fcasset.DCSContextURL + "templateUuid"
	dcsStateIRI              = fcasset.DCSContextURL + "state"
	dcsTemplateTypeIRI       = fcasset.DCSContextURL + "templateType"
	dcsTemplateDataStringIRI = fcasset.DCSContextURL + "templateDataString"
	schemaNameIRI            = fcasset.SchemaContextURL + "name"
	schemaDescriptionIRI     = fcasset.SchemaContextURL + "description"
	schemaVersionIRI         = fcasset.SchemaContextURL + "version"
)

// fieldTriple builds one RDF-star pattern binding a template's field (stored
// as a credentialSubject property in Fuseki) to a SPARQL variable. All of a
// template's own fields share the same subject (?s), which is itself the
// template's DID — DCS's own asset payload (fcasset.BuildPayload) sets the
// credentialSubject's own "id" to the template DID, so subject ==
// credentialSubject IRI == DID for every triple emitted here.
func fieldTriple(predicateIRI, varName string) string {
	return fmt.Sprintf("  <<(?s <%s> ?%s)>> <%s> ?s .\n", predicateIRI, varName, credentialSubjectIRI)
}

// coreFieldTriples is the shared basic graph pattern for a template's
// searchable/listable fields, joined on the one subject variable ?s (== DID).
func coreFieldTriples() string {
	var b strings.Builder
	b.WriteString(fieldTriple(dcsTemplateUUIDIRI, "template_uuid"))
	b.WriteString(fieldTriple(schemaNameIRI, "name"))
	b.WriteString(fieldTriple(schemaDescriptionIRI, "description"))
	b.WriteString(fieldTriple(schemaVersionIRI, "version"))
	b.WriteString(fieldTriple(dcsStateIRI, "state"))
	return b.String()
}

// sparqlEscapeString escapes a string for safe embedding as a SPARQL string
// literal (query text is built by string interpolation — FC's SPARQL backend
// does not support parameterized queries, see GraphQuery/SparqlGraphStore
// upstream, which ignores any "parameters" the request carries).
func sparqlEscapeString(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return replacer.Replace(s)
}

// countFromResults reads a single-row aggregate (e.g. `SELECT (COUNT(?s) AS
// ?total) WHERE {...}`) out of query results. Fuseki's PaginatedResults
// always reports TotalCount as len(items) (row count of the query itself,
// not the aggregate value) — see eu.xfsc.fc.core.pojo.PaginatedResults
// upstream — so the actual count must be read out of the bound variable.
func countFromResults(items []map[string]interface{}, varName string) int {
	if len(items) == 0 {
		return 0
	}
	return ptr.IntFromMap(items[0], varName)
}
