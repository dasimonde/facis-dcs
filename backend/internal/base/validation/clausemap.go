package validation

import (
	"strings"
)

// ClauseComponent is one dcs:Clause block from dcs:documentStructure.
type ClauseComponent struct {
	ID    string
	Title string
	// Types are the block's @type values (CURIE or IRI), e.g. dcs:Clause, dcs:PaymentClause.
	Types []string
	// Refs are IRIs referenced from the clause body (dcs:content chips, etc.).
	Refs []string
}

// MappedPolicyFinding is a PolicyFinding with conservative clause attribution.
// ClauseIDs is empty (contract-level / ambiguous), or exactly one clause when
// the mapping is reliable. Never invents compliance from an empty finding set.
type MappedPolicyFinding struct {
	Finding   PolicyFinding
	ClauseIDs []string
}

// ExtractClauseComponents lists dcs:Clause blocks and the IRIs they reference.
func ExtractClauseComponents(contractDocument any) ([]ClauseComponent, error) {
	doc, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, err
	}
	structure, _ := doc["dcs:documentStructure"].(map[string]any)
	if structure == nil {
		return nil, nil
	}
	blocksNode, _ := structure["dcs:blocks"].(map[string]any)
	if blocksNode == nil {
		return nil, nil
	}
	list, _ := blocksNode["@list"].([]any)
	out := make([]ClauseComponent, 0)
	for _, raw := range list {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if !isClauseType(block["@type"]) {
			continue
		}
		id := strings.TrimSpace(asString(block["@id"]))
		if id == "" {
			continue
		}
		title := strings.TrimSpace(asString(block["dcs:title"]))
		if title == "" {
			title = id
		}
		out = append(out, ClauseComponent{
			ID:    id,
			Title: title,
			Types: collectTypes(block["@type"]),
			Refs:  collectClauseRefs(block),
		})
	}
	return out, nil
}

func collectTypes(raw any) []string {
	switch t := raw.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		return []string{s}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, collectTypes(item)...)
		}
		return out
	default:
		return nil
	}
}

// MapFindingsToClauses attributes each finding to at most one clause when the
// link is unambiguous. Multiple matches or none → ClauseIDs empty.
func MapFindingsToClauses(findings []PolicyFinding, clauses []ClauseComponent, documentID string) []MappedPolicyFinding {
	docID := strings.TrimSpace(documentID)
	out := make([]MappedPolicyFinding, 0, len(findings))
	for _, finding := range findings {
		out = append(out, MappedPolicyFinding{
			Finding:   finding,
			ClauseIDs: attributeFindingToClauses(finding, clauses, docID),
		})
	}
	return out
}

func attributeFindingToClauses(finding PolicyFinding, clauses []ClauseComponent, documentID string) []string {
	focus := strings.TrimSpace(finding.FocusNode)
	field := strings.TrimSpace(finding.FieldIri)
	path := strings.TrimSpace(finding.Path)

	// Document-root focus is contract-level, not a clause.
	if focus != "" && documentID != "" && iriEqual(focus, documentID) {
		return nil
	}

	// Direct focus = clause @id.
	if focus != "" {
		matched := clausesMatching(clauses, func(c ClauseComponent) bool {
			return iriEqual(c.ID, focus)
		})
		if len(matched) == 1 {
			return []string{matched[0]}
		}
		if len(matched) > 1 {
			return nil
		}
	}

	// Focus / field IRI referenced by exactly one clause body.
	candidates := uniqueNonEmpty(focus, field)
	for _, cand := range candidates {
		matched := clausesMatching(clauses, func(c ClauseComponent) bool {
			for _, ref := range c.Refs {
				if iriEqual(ref, cand) {
					return true
				}
			}
			return false
		})
		if len(matched) == 1 {
			return []string{matched[0]}
		}
		if len(matched) > 1 {
			return nil
		}
	}

	// Path local-name matches the local name of exactly one referenced field.
	if path != "" {
		matched := clausesMatching(clauses, func(c ClauseComponent) bool {
			for _, ref := range c.Refs {
				if iriLocalName(ref) == path || iriEqual(ref, path) {
					return true
				}
			}
			return false
		})
		if len(matched) == 1 {
			return []string{matched[0]}
		}
	}

	return nil
}

func clausesMatching(clauses []ClauseComponent, pred func(ClauseComponent) bool) []string {
	ids := make([]string, 0)
	for _, c := range clauses {
		if pred(c) {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

func collectClauseRefs(block map[string]any) []string {
	seen := map[string]struct{}{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if id := strings.TrimSpace(asString(t["@id"])); id != "" {
				seen[id] = struct{}{}
			}
			for k, child := range t {
				if k == "@id" || k == "@type" {
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	// Prefer dcs:content, but also scan the block for nested @id chips.
	if content, ok := block["dcs:content"]; ok {
		walk(content)
	} else {
		walk(block)
	}
	// Do not treat the clause's own @id as a "ref" for field matching.
	self := strings.TrimSpace(asString(block["@id"]))
	delete(seen, self)
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func isClauseType(raw any) bool {
	switch t := raw.(type) {
	case string:
		t = strings.TrimSpace(t)
		if t == "Clause" || strings.HasSuffix(t, ":Clause") || strings.HasSuffix(t, "/Clause") {
			return true
		}
		// Typed catalog clauses (e.g. dcs:PaymentClause) end with "Clause".
		local := iriLocalName(t)
		return local == "Clause" || strings.HasSuffix(local, "Clause")
	case []any:
		for _, item := range t {
			if isClauseType(item) {
				return true
			}
		}
	}
	return false
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func iriEqual(a, b string) bool {
	return strings.TrimSpace(a) != "" && strings.TrimSpace(a) == strings.TrimSpace(b)
}

func iriLocalName(iri string) string {
	iri = strings.TrimSpace(iri)
	if i := strings.LastIndexAny(iri, "/#"); i >= 0 && i+1 < len(iri) {
		return iri[i+1:]
	}
	return iri
}

func uniqueNonEmpty(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// DocumentID returns the contract document @id when present.
func DocumentID(contractDocument any) string {
	doc, err := normalizeObject(contractDocument)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(asString(doc["@id"]))
}
