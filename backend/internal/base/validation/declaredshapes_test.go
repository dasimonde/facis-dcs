package validation

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

const legalPersonClass = "https://example.org/legal#LegalPerson"

// legalLibraryShapes is a registered hub shapes library (the shape of what
// the semantic-data-objects flow registers): it targets its own class and
// says nothing about the DCS envelope.
const legalLibraryShapes = `@prefix sh:  <http://www.w3.org/ns/shacl#> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix ex:  <https://example.org/legal#> .

ex:LegalPersonShape a sh:NodeShape ;
    sh:targetClass ex:LegalPerson ;
    sh:property [ sh:path ex:registrationNumber ; sh:datatype xsd:string ; sh:minCount 1 ] .
`

func hubShapesAnchor(name string, version int) string {
	if version <= 0 {
		return "https://dcs.example/api/semantic/shapes/" + name
	}
	return "https://dcs.example/api/semantic/shapes/" + name + "?version=" + strconv.Itoa(version)
}

func legalPersonNode() map[string]any {
	return map[string]any{
		"@id":   "urn:uuid:legal-person-1",
		"@type": legalPersonClass,
	}
}

func libraryShapeSource(t *testing.T) func() {
	t.Helper()
	return swapShapeSource(t, fixtureShapeSource{
		shapesTTL:   mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-shapes.ttl"),
		catalogTTL:  mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-clause-catalog.ttl"),
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld"),
		libraries:   map[string]string{"legal-shapes": legalLibraryShapes},
	})
}

// A submitter does not choose its own gate. sh:shapesGraph is an ordinary
// top-level key of client-submitted contract JSON-LD, so a document naming
// only a registered library — or only the clause catalog — must not be
// validated against that graph ALONE: the canonical DCS envelope shapes and
// the catalog are resolved for every document whatever it declares, and a
// declaration can only ADD.
func TestDeclaringOtherGraphsCannotEscapeTheCanonicalShapes(t *testing.T) {
	restore := libraryShapeSource(t)
	defer restore()

	escapes := map[string]any{
		"the clause catalog alone":     map[string]any{"@id": hubShapesAnchor(clauseCatalogEntryName, 0)},
		"a registered library alone":   map[string]any{"@id": hubShapesAnchor("legal-shapes", 3)},
		"a library beside the catalog": []any{hubShapesAnchor("legal-shapes", 3), hubShapesAnchor(clauseCatalogEntryName, 0)},
	}
	for name, anchor := range escapes {
		t.Run(name, func(t *testing.T) {
			contract := canonicalAuditContract()
			contract["sh:shapesGraph"] = anchor
			// A canonical-shapes violation: dcs:metadata without a title.
			contract["dcs:metadata"] = map[string]any{"@type": "dcs:ContractMetadata"}
			// A clause-catalog violation: a negative payment amount.
			contract["dcs:typedClause"] = map[string]any{
				"@id":          "urn:uuid:payment-escape",
				"@type":        "dcs:PaymentClause",
				"dcs:amount":   map[string]any{"@value": -5, "@type": "xsd:integer"},
				"dcs:currency": "EUR",
			}

			findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
			require.NoError(t, err)
			requirePolicyFinding(t, findings, "title-MinCountConstraintComponent")
			requirePolicyFinding(t, findings, "amount-MinInclusiveConstraintComponent")

			require.ErrorContains(t, RequireHubConformance(context.Background(), contract), "title")
		})
	}
}

// The pin still holds where it is a pin and not an escape: the version a
// document names for the CANONICAL graph is the version it is validated
// against, and naming it twice at two versions is a contradiction, not a
// choice the resolver makes silently.
func TestCanonicalShapesVersionIsPinnedByTheDocument(t *testing.T) {
	restore := libraryShapeSource(t)
	defer restore()

	contract := canonicalAuditContract()
	contract["sh:shapesGraph"] = map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 7)}
	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, shaclOnlyFindings(findings))

	contract["sh:shapesGraph"] = []any{
		hubShapesAnchor(hubShapesEntryName, 7),
		hubShapesAnchor(hubShapesEntryName, 2),
	}
	_, err = AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.ErrorContains(t, err, "two versions")
}

// The pin is a pin: a document is validated against the shapes graphs it
// declares, so a shapes library registered and left ACTIVE in the hub cannot
// change the verdict on a document that never named it. Without this, the
// same contract validates differently on two deployments and re-validating
// an old contract fails against shapes that did not exist when it was
// signed — the drift pinned_content_hash exists to detect.
func TestUndeclaredShapeLibraryDoesNotAffectValidation(t *testing.T) {
	restore := libraryShapeSource(t)
	defer restore()

	// A LegalPerson without ex:registrationNumber: a violation of the
	// registered library, and of nothing in the canonical DCS shapes.
	contract := canonicalAuditContract()
	contract["sh:shapesGraph"] = map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)}
	contract["dcs:typedClause"] = legalPersonNode()

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, shaclOnlyFindings(findings))
	require.NoError(t, RequireHubConformance(context.Background(), contract))
}

// The opt-in half: declaring the library in sh:shapesGraph — SHACL's own
// multi-valued data-graph→shapes-graph link — puts the document's data
// objects under that library's constraints (ADR-23).
func TestDeclaredShapeLibraryIsEnforced(t *testing.T) {
	restore := libraryShapeSource(t)
	defer restore()

	contract := canonicalAuditContract()
	contract["sh:shapesGraph"] = []any{
		map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)},
		map[string]any{"@id": hubShapesAnchor("legal-shapes", 3)},
	}
	contract["dcs:typedClause"] = legalPersonNode()

	findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	finding := requirePolicyFinding(t, findings, "registrationNumber-MinCountConstraintComponent")
	require.Equal(t, "error", finding.Severity)
	// The reported version stays the canonical graph's — the pin drift
	// detection compares against, not a library's own numbering.
	require.Equal(t, 1, finding.ShapesVersion)
}

// A declared graph the hub cannot serve fails the document closed: never a
// silent fallback to whatever the hub does hold.
func TestUnresolvableDeclaredShapesGraphFailsClosed(t *testing.T) {
	restore := libraryShapeSource(t)
	defer restore()

	contract := canonicalAuditContract()
	contract["sh:shapesGraph"] = []any{
		map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)},
		map[string]any{"@id": hubShapesAnchor("never-registered", 2)},
	}

	_, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
	require.ErrorContains(t, err, "never-registered")
}

func TestDeclaredShapesGraphsReadsEveryAnchorForm(t *testing.T) {
	anchors := declaredShapesGraphs(map[string]any{
		"sh:shapesGraph": []any{
			map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 2)},
			hubShapesAnchor("legal-shapes", 0),
			// Duplicates collapse; a non-hub IRI declares nothing.
			hubShapesAnchor("legal-shapes", 0),
			SchemaSHACLShapesV1,
		},
	})
	require.Equal(t, []shapesGraphAnchor{
		{Name: hubShapesEntryName, Version: 2},
		{Name: "legal-shapes", Version: 0},
	}, anchors)

	// The single-anchor form every canonical document carries.
	require.Equal(t,
		[]shapesGraphAnchor{{Name: hubShapesEntryName, Version: 1}},
		declaredShapesGraphs(map[string]any{
			"sh:shapesGraph": map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)},
		}))

	// An unanchored document declares nothing and falls back to the
	// canonical shapes, never to a registered library.
	require.Empty(t, declaredShapesGraphs(map[string]any{}))
	require.Empty(t, declaredShapesGraphs(map[string]any{"sh:shapesGraph": map[string]any{"@id": SchemaSHACLShapesV1}}))
}

// Production side of the opt-in: a document whose data uses a class governed
// by a registered library declares that library itself, so the library is
// enforced without any document being exposed to libraries it does not use.
func TestNormalizationDeclaresTheLibraryGoverningADataObject(t *testing.T) {
	SetShapeLibraryAnchors(map[string]ShapeLibraryAnchor{
		legalPersonClass: {Name: "legal-shapes", URL: hubShapesAnchor("legal-shapes", 3)},
	})
	t.Cleanup(func() { SetShapeLibraryAnchors(nil) })

	withLegalPerson := normalizedTemplateWithDataObject(t, legalPersonNode())
	require.Equal(t, []any{
		map[string]any{"@id": SchemaSHACLShapesV1},
		map[string]any{"@id": hubShapesAnchor("legal-shapes", 3)},
	}, withLegalPerson["sh:shapesGraph"])

	// A document using no library class keeps the canonical anchor alone.
	plain, err := NormalizeTemplateData(canonicalTemplateData(t))
	require.NoError(t, err)
	var plainDoc map[string]any
	require.NoError(t, json.Unmarshal(*plain, &plainDoc))
	require.Equal(t, map[string]any{"@id": SchemaSHACLShapesV1}, plainDoc["sh:shapesGraph"])
}

// Anchors are added, never rewritten (ADR-8) — but "already declared" is name
// AND version. A document arriving with an OLD version of the library
// pre-declared does not thereby suppress the active anchor: it is validated
// against both, so pre-declaring a laxer version cannot buy a laxer verdict.
func TestNormalizationAddsTheActiveLibraryBesideAStalePreDeclaredVersion(t *testing.T) {
	SetShapeLibraryAnchors(map[string]ShapeLibraryAnchor{
		legalPersonClass: {Name: "legal-shapes", URL: hubShapesAnchor("legal-shapes", 5)},
	})
	t.Cleanup(func() { SetShapeLibraryAnchors(nil) })

	document := templateWithDataObject(t, legalPersonNode())
	document["sh:shapesGraph"] = []any{
		map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)},
		map[string]any{"@id": hubShapesAnchor("legal-shapes", 1)},
	}
	raw, err := datatype.NewJSON(document)
	require.NoError(t, err)
	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &doc))
	require.Equal(t, []any{
		map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)},
		map[string]any{"@id": hubShapesAnchor("legal-shapes", 1)},
		map[string]any{"@id": hubShapesAnchor("legal-shapes", 5)},
	}, doc["sh:shapesGraph"])
}

// The same library at the SAME version is declared once: re-normalizing a
// document does not grow its anchor list.
func TestNormalizationKeepsAnAlreadyDeclaredLibraryVersion(t *testing.T) {
	SetShapeLibraryAnchors(map[string]ShapeLibraryAnchor{
		legalPersonClass: {Name: "legal-shapes", URL: hubShapesAnchor("legal-shapes", 1)},
	})
	t.Cleanup(func() { SetShapeLibraryAnchors(nil) })

	document := templateWithDataObject(t, legalPersonNode())
	document["sh:shapesGraph"] = []any{
		map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)},
		map[string]any{"@id": hubShapesAnchor("legal-shapes", 1)},
	}
	raw, err := datatype.NewJSON(document)
	require.NoError(t, err)
	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &doc))
	require.Equal(t, []any{
		map[string]any{"@id": hubShapesAnchor(hubShapesEntryName, 1)},
		map[string]any{"@id": hubShapesAnchor("legal-shapes", 1)},
	}, doc["sh:shapesGraph"])
}

func templateWithDataObject(t *testing.T, object map[string]any) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal(*canonicalTemplateData(t), &document))
	document["dcs:contractData"] = []any{object}
	return document
}

func normalizedTemplateWithDataObject(t *testing.T, object map[string]any) map[string]any {
	t.Helper()
	raw, err := datatype.NewJSON(templateWithDataObject(t, object))
	require.NoError(t, err)
	normalized, err := NormalizeTemplateData(&raw)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &document))
	return document
}
