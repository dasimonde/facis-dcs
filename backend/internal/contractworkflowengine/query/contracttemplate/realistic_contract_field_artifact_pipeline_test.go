package contracttemplate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

const (
	realisticTemplateDID = "did:web:example:template:realistic-service"
	realisticContractDID = "did:web:example:contract:realistic-service"
)

var realisticFieldSpecs = []struct {
	name     string
	label    string
	datatype string
	value    any
}{
	{"customer-name", "Auftraggeber", "xsd:string", "Beispiel Handel GmbH"},
	{"provider-name", "Auftragnehmer", "xsd:string", "FACIS Services GmbH"},
	{"service-description", "Leistungsbeschreibung", "xsd:string", "Betrieb und Support der digitalen Vertragsplattform"},
	{"monthly-fee", "Monatliche Vergütung", "xsd:decimal", 12500.00},
	{"currency", "Währung", "xsd:string", "EUR"},
	{"payment-term-days", "Zahlungsziel in Tagen", "xsd:integer", 30},
	{"term-months", "Vertragslaufzeit in Monaten", "xsd:integer", 24},
	{"availability", "Verfügbarkeit in Prozent", "xsd:decimal", 99.9},
	{"reaction-time-hours", "Reaktionszeit in Stunden", "xsd:integer", 4},
	{"venue", "Gerichtsstand", "xsd:string", "Berlin"},
}

var realisticSectionChildren = []struct {
	section string
	clauses []string
}{
	{"parties", []string{"clause-parties"}},
	{"service", []string{"clause-service", "clause-term"}},
	{"payment", []string{"clause-payment"}},
	{"sla", []string{"clause-sla"}},
	{"legal", []string{"clause-legal"}},
}

// TestRealisticContractFieldArtifactPipeline is the local, deterministic
// counterpart to the live lifecycle BDD. It exercises the same production
// normalization and template-to-contract conversion boundaries without
// depending on Federated Catalogue availability.
func TestRealisticContractFieldArtifactPipeline(t *testing.T) {
	rawTemplate := realisticJSON(t, realisticTemplateDocument())

	normalizedTemplate, err := validation.NormalizeTemplateData(rawTemplate)
	require.NoError(t, err)
	persistedTemplate, err := validation.NormalizeTemplateDataForPersistence(
		normalizedTemplate,
		realisticTemplateDID,
	)
	require.NoError(t, err)

	contractDraft, err := ConvertTemplateDataToContractData(
		persistedTemplate,
		realisticTemplateDID,
		1,
	)
	require.NoError(t, err)
	persistedContract, err := validation.NormalizeContractDataForPersistence(
		contractDraft,
		realisticContractDID,
		false,
	)
	require.NoError(t, err)

	templateDocument := realisticDocumentMap(t, persistedTemplate)
	contractDocument := realisticDocumentMap(t, persistedContract)
	for label, document := range map[string]map[string]any{
		"template": templateDocument,
		"contract": contractDocument,
	} {
		assertRealisticContractFields(t, label, document)
		assertRealisticDocumentStructure(t, label, document)
	}
	assertRealisticProvenanceAndRebase(t, templateDocument, contractDocument)

	artifactDir := t.TempDir()
	writeRealisticArtifact(t, filepath.Join(artifactDir, "realistic-contract-field-template.jsonld"), templateDocument)
	writeRealisticArtifact(t, filepath.Join(artifactDir, "realistic-contract-field-contract.jsonld"), contractDocument)
}

func realisticTemplateDocument() map[string]any {
	fields := make([]any, 0, len(realisticFieldSpecs))
	for _, spec := range realisticFieldSpecs {
		fields = append(fields, map[string]any{
			"@id":          "urn:uuid:field-" + spec.name,
			"@type":        "dcs:ContractField",
			"dcs:label":    spec.label,
			"dcs:datatype": spec.datatype,
			"dcs:required": true,
			"dcs:value":    spec.value,
		})
	}

	blocks := []any{
		realisticSection("parties", "1. Vertragsparteien"),
		realisticClause("clause-parties", "Vertragsparteien",
			"Der Vertrag wird geschlossen zwischen ", realisticFieldRef("customer-name"),
			" und ", realisticFieldRef("provider-name"), "."),
		realisticSection("service", "2. Leistung und Laufzeit"),
		realisticClause("clause-service", "Leistungsgegenstand",
			"Der Auftragnehmer erbringt folgende Leistung: ", realisticFieldRef("service-description"), "."),
		realisticClause("clause-term", "Laufzeit",
			"Die Vertragslaufzeit beträgt ", realisticFieldRef("term-months"), " Monate."),
		realisticSection("payment", "3. Vergütung"),
		realisticClause("clause-payment", "Vergütung und Zahlungsziel",
			"Die monatliche Vergütung beträgt ", realisticFieldRef("monthly-fee"), " ",
			realisticFieldRef("currency"), ". Rechnungen sind innerhalb von ",
			realisticFieldRef("payment-term-days"), " Tagen fällig."),
		realisticSection("sla", "4. Service Level"),
		realisticClause("clause-sla", "Verfügbarkeit und Reaktionszeit",
			"Die zugesicherte Verfügbarkeit beträgt ", realisticFieldRef("availability"),
			" Prozent; die Reaktionszeit beträgt höchstens ", realisticFieldRef("reaction-time-hours"), " Stunden."),
		realisticSection("legal", "5. Schlussbestimmungen"),
		realisticClause("clause-legal", "Gerichtsstand",
			"Ausschließlicher Gerichtsstand ist ", realisticFieldRef("venue"), "."),
	}

	layout := []any{realisticLayoutNode("root", true, "parties", "service", "payment", "sla", "legal")}
	for _, group := range realisticSectionChildren {
		layout = append(layout, realisticLayoutNode(group.section, false, group.clauses...))
		for _, clause := range group.clauses {
			layout = append(layout, realisticLayoutNode(clause, false))
		}
	}

	return map[string]any{
		"@context": map[string]any{
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#",
		},
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":            "dcs:TemplateMetadata",
			"dcs:title":        "Realistischer Plattform-Servicevertrag",
			"dcs:description":  "Servicevertrag mit Vergütung, Laufzeit, SLA und Gerichtsstand",
			"dcs:templateType": "dcs:Contract",
		},
		"dcs:contractFields": fields,
		"dcs:contractData": []any{
			map[string]any{
				"@id": "urn:uuid:data-parties", "@type": "dcs:ContractParties",
				"dcs:customer": realisticFieldRef("customer-name"),
				"dcs:provider": realisticFieldRef("provider-name"),
			},
			map[string]any{
				"@id": "urn:uuid:data-service", "@type": "dcs:ServiceDescription",
				"dcs:description": realisticFieldRef("service-description"),
				"dcs:termMonths":  realisticFieldRef("term-months"),
			},
			map[string]any{
				"@id": "urn:uuid:data-payment", "@type": "dcs:PaymentTerms",
				"dcs:monthlyFee":      realisticFieldRef("monthly-fee"),
				"dcs:currency":        realisticFieldRef("currency"),
				"dcs:paymentTermDays": realisticFieldRef("payment-term-days"),
			},
			map[string]any{
				"@id": "urn:uuid:data-service-level", "@type": "dcs:ServiceLevel",
				"dcs:availability":      realisticFieldRef("availability"),
				"dcs:reactionTimeHours": realisticFieldRef("reaction-time-hours"),
			},
			map[string]any{
				"@id": "urn:uuid:data-jurisdiction", "@type": "dcs:Jurisdiction",
				"dcs:venue": realisticFieldRef("venue"),
			},
		},
		"dcs:policies": []any{},
		"dcs:documentStructure": map[string]any{
			"@type":      "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": blocks},
			"dcs:layout": map[string]any{"@list": layout},
		},
	}
}

func realisticFieldRef(name string) map[string]any {
	return map[string]any{"@id": "urn:uuid:field-" + name}
}

func realisticSection(name, title string) map[string]any {
	return map[string]any{
		"@id": "urn:uuid:block-" + name, "@type": "dcs:Section", "dcs:title": title,
	}
}

func realisticClause(name, title string, content ...any) map[string]any {
	return map[string]any{
		"@id": "urn:uuid:block-" + name, "@type": "dcs:Clause", "dcs:title": title,
		"dcs:content": map[string]any{"@list": content},
	}
}

func realisticLayoutNode(name string, root bool, children ...string) map[string]any {
	childRefs := make([]any, 0, len(children))
	for _, child := range children {
		childRefs = append(childRefs, map[string]any{"@id": "urn:uuid:block-" + child})
	}
	node := map[string]any{
		"@id": "urn:uuid:block-" + name, "@type": "dcs:LayoutNode",
		"dcs:children": map[string]any{"@list": childRefs},
	}
	if root {
		node["dcs:isRoot"] = true
	}
	return node
}

func assertRealisticContractFields(t *testing.T, label string, document map[string]any) {
	t.Helper()
	fields, ok := document["dcs:contractFields"].([]any)
	require.True(t, ok, "%s contract fields must be an array", label)
	require.Len(t, fields, 10, "%s must retain all contract fields", label)

	declared := make(map[string]struct{}, len(fields))
	for _, rawField := range fields {
		field := rawField.(map[string]any)
		require.Equal(t, "dcs:ContractField", field["@type"])
		id := field["@id"].(string)
		declared[id] = struct{}{}
		require.NotEmpty(t, field["dcs:label"])
		require.NotEmpty(t, field["dcs:datatype"])
		require.Equal(t, true, field["dcs:required"])
		require.Contains(t, field, "dcs:value")
	}

	contractData := document["dcs:contractData"].([]any)
	require.Len(t, contractData, 5, "%s must retain all typed business objects", label)
	referenced := map[string]struct{}{}
	collectRealisticFieldReferences(contractData, referenced)
	require.Equal(t, declared, referenced, "%s business data must reference every declared field", label)
}

func collectRealisticFieldReferences(value any, result map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		if id, ok := typed["@id"].(string); ok && strings.Contains(id, "#field-") {
			result[id] = struct{}{}
		}
		for _, nested := range typed {
			collectRealisticFieldReferences(nested, result)
		}
	case []any:
		for _, nested := range typed {
			collectRealisticFieldReferences(nested, result)
		}
	}
}

func assertRealisticDocumentStructure(t *testing.T, label string, document map[string]any) {
	t.Helper()
	structure := document["dcs:documentStructure"].(map[string]any)
	rawBlocks := structure["dcs:blocks"].(map[string]any)["@list"].([]any)
	require.Len(t, rawBlocks, 11, "%s must retain five sections and six clauses", label)

	blocks := map[string]map[string]any{}
	for _, rawBlock := range rawBlocks {
		block := rawBlock.(map[string]any)
		blocks[realisticIDSuffix(block["@id"].(string), "#block-")] = block
	}
	layout := map[string]map[string]any{}
	for _, rawNode := range structure["dcs:layout"].(map[string]any)["@list"].([]any) {
		node := rawNode.(map[string]any)
		layout[realisticIDSuffix(node["@id"].(string), "#block-")] = node
	}

	for _, group := range realisticSectionChildren {
		require.Equal(t, "dcs:Section", blocks[group.section]["@type"], "%s section %s", label, group.section)
		rawChildren := layout[group.section]["dcs:children"].(map[string]any)["@list"].([]any)
		children := make([]string, 0, len(rawChildren))
		for _, rawChild := range rawChildren {
			child := rawChild.(map[string]any)
			children = append(children, realisticIDSuffix(child["@id"].(string), "#block-"))
		}
		require.Equal(t, group.clauses, children, "%s clause indentation below %s", label, group.section)
		for _, clause := range group.clauses {
			require.Equal(t, "dcs:Clause", blocks[clause]["@type"], "%s clause %s", label, clause)
		}
	}
}

func assertRealisticProvenanceAndRebase(
	t *testing.T,
	templateDocument map[string]any,
	contractDocument map[string]any,
) {
	t.Helper()
	provenance := contractDocument["derivedFromTemplate"].(map[string]any)
	require.Equal(t, realisticTemplateDID, provenance["@id"])
	require.Equal(t, float64(1), provenance["version"])
	require.Equal(t, realisticTemplateDID, templateDocument["@id"])
	require.Equal(t, realisticContractDID, contractDocument["@id"])

	for _, document := range []struct {
		value  map[string]any
		prefix string
	}{
		{templateDocument, realisticTemplateDID + "#"},
		{contractDocument, realisticContractDID + "#"},
	} {
		ids := []string{}
		collectRealisticIDs(document.value, &ids)
		for _, id := range ids {
			if strings.Contains(id, "#field-") || strings.Contains(id, "#block-") || strings.Contains(id, "#data-") {
				require.True(t, strings.HasPrefix(id, document.prefix), "identifier %s must use prefix %s", id, document.prefix)
			}
		}
	}
}

func collectRealisticIDs(value any, result *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		if id, ok := typed["@id"].(string); ok {
			*result = append(*result, id)
		}
		for _, nested := range typed {
			collectRealisticIDs(nested, result)
		}
	case []any:
		for _, nested := range typed {
			collectRealisticIDs(nested, result)
		}
	}
}

func realisticIDSuffix(id, marker string) string {
	_, suffix, found := strings.Cut(id, marker)
	if !found {
		return id
	}
	return suffix
}

func realisticDocumentMap(t *testing.T, raw *datatype.JSON) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal(*raw, &document))
	return document
}

func realisticJSON(t *testing.T, value any) *datatype.JSON {
	t.Helper()
	raw, err := datatype.NewJSON(value)
	require.NoError(t, err)
	return &raw
}

func writeRealisticArtifact(t *testing.T, path string, document map[string]any) {
	t.Helper()
	content, err := json.MarshalIndent(document, "", "  ")
	require.NoError(t, err)
	content = append(content, '\n')
	require.NoError(t, os.WriteFile(path, content, 0o644))
	require.NoError(t, os.Chmod(path, 0o644))
}
