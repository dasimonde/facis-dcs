package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const dcsOntologyIRI = "https://w3id.org/facis/dcs/ontology/v1#"

// canonicalNamespaceMap returns the display prefix→namespace map used to compact
// IRIs in the rendered PDF (glossary terms, ontology links).
func canonicalNamespaceMap() map[string]string {
	return map[string]string{
		"dcterms": "http://purl.org/dc/terms/",
		"schema":  "https://schema.org/",
		"prov":    "http://www.w3.org/ns/prov#",
	}
}

// expandCanonicalIRI resolves a compact IRI from the canonical compact form back
// to its full absolute IRI. Handles the prefixes declared in the canonical
// context (prov:, schema:, dcterms:, xsd:) plus bare names expanded via @vocab.
// Full IRIs (http/https/urn) are returned unchanged.
func expandCanonicalIRI(compact string) string {
	switch {
	case strings.HasPrefix(compact, "prov:"):
		return "http://www.w3.org/ns/prov#" + compact[5:]
	case strings.HasPrefix(compact, "schema:"):
		return "https://schema.org/" + compact[7:]
	case strings.HasPrefix(compact, "dcterms:"):
		return "http://purl.org/dc/terms/" + compact[8:]
	case strings.HasPrefix(compact, "xsd:"):
		return "http://www.w3.org/2001/XMLSchema#" + compact[4:]
	case strings.HasPrefix(compact, "http://"), strings.HasPrefix(compact, "https://"), strings.HasPrefix(compact, "urn:"):
		return compact
	}
	if !strings.Contains(compact, ":") {
		return dcsOntologyIRI + compact
	}
	return compact
}

// contractFieldValue extracts a field's filled value,
// accepting a bare scalar, a JSON number, or a typed {"@value":…} literal.
func contractFieldValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		if v, ok := obj["@value"]; ok {
			return fmt.Sprint(v)
		}
	}
	return ""
}

// scalarText renders a JSON scalar to its deterministic display text.
func scalarText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func parseCanonicalSegment(item ContentItem, fields map[string]string) clauseSegment {
	// Value objects: typed literal or plain string.
	if item.Value != "" {
		if item.Datatype != "" {
			return clauseSegment{Type: "typed-value", Value: item.Value, Datatype: expandCanonicalIRI(item.Datatype)}
		}
		return clauseSegment{Type: "prose", Text: item.Value}
	}

	// Decode raw JSON for schema: properties (schema:url, schema:name).

	var raw map[string]any
	if len(item.Raw) > 0 && item.Raw[0] == '{' {
		json.Unmarshal(item.Raw, &raw) //nolint:errcheck // best-effort parse for segment properties
	}

	// External link: object carries schema:url.
	if urlVal, ok := raw["schema:url"]; ok {
		href := ""
		switch v := urlVal.(type) {
		case string:
			href = v
		case map[string]any:
			if s, ok := v["@value"].(string); ok {
				href = s
			} else if s, ok := v["@id"].(string); ok {
				href = s
			}
		}
		text := href
		if name, ok := raw["schema:name"].(string); ok && name != "" {
			text = name
		}
		return clauseSegment{Type: "external-link", Text: text, Href: href}
	}

	// IRI reference: object carries @id.
	if item.ID != "" {
		if value, fieldReference := fields[item.ID]; fieldReference {
			if value == "" {
				value = "_____"
			}
			return clauseSegment{Type: "prose", Text: value}
		}
		fullIRI := expandCanonicalIRI(item.ID)
		text := fullIRI
		if raw != nil {
			if name, ok := raw["schema:name"].(string); ok && name != "" {
				text = name
			}
		}
		return clauseSegment{Type: "ontology-link", Text: text, Ref: fullIRI}
	}

	return clauseSegment{Type: "prose"}
}

func parseCanonicalClause(block *Block, fields map[string]string) clauseData {
	if block.Type == "TextBlock" {
		return clauseData{Segments: []clauseSegment{{Type: "prose", Text: block.Text}}}
	}

	clause := clauseData{Segments: []clauseSegment{}}
	for _, rawItem := range block.Content {
		var item ContentItem
		if err := json.Unmarshal(rawItem, &item); err == nil {
			clause.Segments = append(clause.Segments, parseCanonicalSegment(item, fields))
		}
	}
	return clause
}

func walkCanonicalSectionNode(sec sectionData, ln *LayoutNode, layoutByID map[string]*LayoutNode, blockByID map[string]*Block, fields map[string]string) sectionData {
	for _, childID := range ln.Children {
		block := blockByID[childID]
		if block == nil {
			continue
		}
		switch block.Type {
		case "Clause", "TextBlock":
			sec.Clauses = append(sec.Clauses, parseCanonicalClause(block, fields))
		case "Section":
			sub := sectionData{
				Heading: strings.TrimSpace(block.Title),
				Clauses: []clauseData{},
			}
			if subLayout, ok := layoutByID[childID]; ok {
				sub = walkCanonicalSectionNode(sub, subLayout, layoutByID, blockByID, fields)
			}
			sec.Subsections = append(sec.Subsections, sub)
		}
	}
	return sec
}

func walkCanonicalSections(ds *DocumentStructure, fields map[string]string) []sectionData {
	blockByID := make(map[string]*Block, len(ds.Blocks))
	for i := range ds.Blocks {
		b := &ds.Blocks[i]
		if b.ID != "" {
			blockByID[b.ID] = b
		}
	}

	layoutByID := make(map[string]*LayoutNode, len(ds.Layout))
	var rootLayout *LayoutNode
	for i := range ds.Layout {
		ln := &ds.Layout[i]
		if ln.IsRoot {
			rootLayout = ln
		} else if ln.ID != "" {
			layoutByID[ln.ID] = ln
		}
	}

	if rootLayout == nil {
		return nil
	}

	var sections []sectionData
	flushAnonymousSection := func(sec *sectionData) {
		if sec == nil {
			return
		}
		if strings.TrimSpace(sec.Heading) == "" && len(sec.Clauses) == 0 && len(sec.Subsections) == 0 {
			return
		}
		sections = append(sections, *sec)
	}

	var anonymous *sectionData
	for _, childID := range rootLayout.Children {
		block := blockByID[childID]
		if block == nil {
			continue
		}
		switch block.Type {
		case "Section":
			flushAnonymousSection(anonymous)
			anonymous = nil
			sec := sectionData{
				Heading: strings.TrimSpace(block.Title),
				Clauses: []clauseData{},
			}
			if secLayout, ok := layoutByID[childID]; ok {
				sec = walkCanonicalSectionNode(sec, secLayout, layoutByID, blockByID, fields)
			}
			sections = append(sections, sec)
		case "Clause", "TextBlock":
			if anonymous == nil {
				anonymous = &sectionData{Clauses: []clauseData{}}
			}
			anonymous.Clauses = append(anonymous.Clauses, parseCanonicalClause(block, fields))
		}
	}
	flushAnonymousSection(anonymous)
	return sections
}

// extractDocumentModelFromCanonical builds a documentModel from a canonical
// compact JSON-LD payload produced by CanonicalizePayload. It unmarshals the
// JSON into typed Go structs and traverses the layout tree to assemble sections,
// clauses, and signature fields without accessing the JSON-LD expanded form.
func extractDocumentModelFromCanonical(canonical []byte, hashHex string) (documentModel, error) {
	var tmpl ContractTemplate
	if err := json.Unmarshal(canonical, &tmpl); err != nil {
		return documentModel{}, fmt.Errorf("unmarshal canonical payload: %w", err)
	}

	model := documentModel{
		Sections:        []sectionData{},
		SignatureFields: []sigFieldDef{},
		Glossary:        []glossaryTerm{},
		NamespaceMap:    canonicalNamespaceMap(),
		// EmbeddedPayload is set by the caller (CompilePDF/updatePDF) to the
		// VERBATIM submitted bytes — not the canonical form used here only to
		// build the render model and the graph hash.
		PayloadHash: hashHex,
		FileID:      hashHex[:32],
		ContractID:  tmpl.ID,
	}

	if tmpl.Metadata == nil || strings.TrimSpace(tmpl.Metadata.Title) == "" {
		return model, fmt.Errorf("metadata.title is required but was not found in the payload")
	}
	model.Title = strings.TrimSpace(tmpl.Metadata.Title)

	fields := map[string]string{}
	for _, field := range tmpl.ContractFields {
		fields[field.ID] = contractFieldValue(field.Value)
	}

	if tmpl.DocumentStructure != nil {
		model.Sections = walkCanonicalSections(tmpl.DocumentStructure, fields)
	}

	for _, sf := range tmpl.SignatureFields {
		name := strings.TrimSpace(sf.SignatoryName)
		if name == "" {
			continue
		}
		label := strings.TrimSpace(sf.Title)
		if label == "" {
			label = name
		}
		model.SignatureFields = append(model.SignatureFields, sigFieldDef{Name: name, Label: label})
	}

	// compact ontology-link text when no name was provided
	var compactLinks func([]sectionData)
	compactLinks = func(sections []sectionData) {
		for si := range sections {
			for ci := range sections[si].Clauses {
				for segi := range sections[si].Clauses[ci].Segments {
					seg := &sections[si].Clauses[ci].Segments[segi]
					if seg.Type == "ontology-link" && seg.Text == seg.Ref && seg.Ref != "" {
						seg.Text = compactIRI(seg.Ref, model.NamespaceMap)
					}
				}
			}
			compactLinks(sections[si].Subsections)
		}
	}
	compactLinks(model.Sections)

	seen := make(map[string]struct{})
	var collectRefs func([]sectionData)
	collectRefs = func(sections []sectionData) {
		for _, section := range sections {
			for _, clause := range section.Clauses {
				for _, seg := range clause.Segments {
					if seg.Type == "ontology-link" && seg.Ref != "" {
						if _, ok := seen[seg.Ref]; !ok {
							seen[seg.Ref] = struct{}{}
							model.Glossary = append(model.Glossary, glossaryTerm{
								Term:    compactIRI(seg.Ref, model.NamespaceMap),
								TermURI: seg.Ref,
							})
						}
					}
				}
			}
			collectRefs(section.Subsections)
		}
	}
	collectRefs(model.Sections)

	return model, nil
}

// extractDocumentModel builds the render model directly from the verbatim
// payload's documentStructure, honoring @list order. It strips the dcs: prefix
// and unwraps @list wrappers without a json-gold expand/compact round trip, so
// the ordered structures the payload carries drive the render deterministically
// (that round trip reorders @set/multi-value nodes non-deterministically).
func extractDocumentModel(payload []byte, hashHex string) (documentModel, error) {
	shaped, err := shapeForModel(payload)
	if err != nil {
		return documentModel{}, err
	}
	return extractDocumentModelFromCanonical(shaped, hashHex)
}

// shapeForModel rewrites a JSON-LD payload into the bare-term, plain-array shape
// the render structs read: it strips the dcs: prefix from keys and @type values
// and unwraps {"@list":[...]} to [...], preserving array order. It never
// reorders, so the same payload always yields identical model input.
func shapeForModel(raw []byte) ([]byte, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON-LD payload: %w", err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return nil, fmt.Errorf("JSON-LD payload must be a JSON object at the root")
	}
	return json.Marshal(stripDCSTerms(doc))
}

// listValuedTerms are the documentStructure properties the render model reads as
// Go slices. JSON-LD serializes a single-cardinality value as a bare object (or
// scalar) rather than a 1-element array, so shapeForModel coerces them to arrays.
var listValuedTerms = map[string]bool{
	"layout":          true,
	"blocks":          true,
	"children":        true,
	"content":         true,
	"signatureFields": true,
	"contractFields":  true,
	"contractData":    true,
}

// idRefListTerms are list properties the model reads as []string of bare IRIs,
// but DCS serializes each element as an @id-reference object ({"@id":"c1"}).
// flattenIDRefs reduces each such element to its @id string.
var idRefListTerms = map[string]bool{"children": true}

func flattenIDRefs(v any) any {
	arr, ok := v.([]any)
	if !ok {
		return v
	}
	for i, e := range arr {
		if m, ok := e.(map[string]any); ok {
			if id, ok := m["@id"].(string); ok {
				arr[i] = id
			}
		}
	}
	return arr
}

func stripDCSTerms(v any) any {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 1 {
			if list, ok := t["@list"]; ok {
				return stripDCSTerms(list)
			}
		}
		out := make(map[string]any, len(t))
		for k, val := range t {
			key := stripDCSPrefix(k)
			if key == "@type" {
				out[key] = stripDCSType(val)
				continue
			}
			shaped := stripDCSTerms(val)
			if listValuedTerms[key] {
				if _, isArray := shaped.([]any); !isArray {
					shaped = []any{shaped}
				}
			}
			if idRefListTerms[key] {
				shaped = flattenIDRefs(shaped)
			}
			out[key] = shaped
		}
		return out
	case []any:
		for i := range t {
			t[i] = stripDCSTerms(t[i])
		}
		return t
	default:
		return v
	}
}

func stripDCSType(v any) any {
	switch t := v.(type) {
	case string:
		return stripDCSPrefix(t)
	case []any:
		for i := range t {
			if s, ok := t[i].(string); ok {
				t[i] = stripDCSPrefix(s)
			}
		}
		return t
	default:
		return v
	}
}

func stripDCSPrefix(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "dcs:"), dcsOntologyIRI)
}

func compactIRI(iri string, nsMap map[string]string) string {
	prefixes := make([]string, 0, len(nsMap))
	for p := range nsMap {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		if strings.HasPrefix(iri, nsMap[prefix]) {
			return prefix + ":" + strings.TrimPrefix(iri, nsMap[prefix])
		}
	}
	return iri
}
