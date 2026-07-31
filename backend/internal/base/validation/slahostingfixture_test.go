package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// The Managed Kubernetes Hosting SLA is the SECOND domain carried on the
// mechanism the SRS Appendix C access grant already proves (see
// frontend/ClientApp/e2e/clause-editor.spec.ts): a clause pairs human prose
// with one ODRL rule whose boundaries are negotiated fields, and an imported
// hub vocabulary is what makes the domain addressable. The fixture is not
// designed here — it is what the authoring path emits (ClauseEditor.vue ->
// addClauseWithMeaning -> assembleCanonicalDocument, then the server's
// urn:uuid rebasing and shapes anchoring), so a green run here is evidence
// about real templates rather than about a document invented to pass.
//
// What the SLA adds beyond Appendix C: an imported shapes library supplying
// the asset classes, the deeper ODRL operands (percentage, count, payAmount,
// timeInterval, elapsedTime, delayPeriod, recipient), and a nested duty
// carrying a consequence.

const slaHostingFixture = "features/fixtures/sla_hosting_template.jsonld"

const slaHostingLibraryName = "facis-sla-hosting"

// slaHostingShapeSource serves the canonical shapes and the clause catalog the
// way production does, plus the two runtime-published SLA entries: the shapes
// library the document declares in sh:shapesGraph, and the domain ontology the
// builder's flat fields come from.
func slaHostingShapeSource(t *testing.T) func() {
	t.Helper()
	return swapShapeSource(t, fixtureShapeSource{
		shapesTTL:   mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-shapes.ttl"),
		catalogTTL:  mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-clause-catalog.ttl"),
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld"),
		ontologyTTL: mustReadRepoFile("backend/internal/semantichub/assets/facis-sla-ontology.ttl") +
			"\n\n" + mustReadRepoFile("features/fixtures/facis-sla-hosting-ontology.ttl"),
		libraries: map[string]string{
			slaHostingLibraryName: mustReadRepoFile("features/fixtures/facis-sla-hosting-shapes.ttl"),
		},
	})
}

func slaHostingTemplate(t *testing.T) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadRepoFile(slaHostingFixture)), &document))
	return document
}

// slaNegotiatedValues are the values the two parties agree, written the way
// the product writes them: typedFieldFill's {"@value", "@type"} literal
// carrying the field's declared datatype (models/dcs-jsonld.ts). A template
// carries none of these — applyInlineSemanticValues runs at contract
// instantiation only — so the audit assertions below run against the
// instantiated contract, not the template.
var slaNegotiatedValues = map[string]any{
	"field-governing-law":             map[string]any{"@value": "DE", "@type": "xsd:string"},
	"field-provisioned-nodes":         map[string]any{"@value": "12", "@type": "xsd:integer"},
	"field-node-class":                map[string]any{"@value": "GENERAL_PURPOSE", "@type": "xsd:string"},
	"field-service-region":            map[string]any{"@value": "EMEA", "@type": "xsd:string"},
	"field-contract-end-date":         map[string]any{"@value": "2028-12-31", "@type": "xsd:date"},
	"field-service-credit-rate":       map[string]any{"@value": "10", "@type": "xsd:decimal"},
	"field-committed-availability":    map[string]any{"@value": "99.9", "@type": "xsd:decimal"},
	"field-measurement-window":        map[string]any{"@value": "P1M", "@type": "xsd:string"},
	"field-monthly-fee":               map[string]any{"@value": "18500.00", "@type": "xsd:decimal"},
	"field-currency":                  map[string]any{"@value": "EUR", "@type": "xsd:string"},
	"field-billing-period":            map[string]any{"@value": "P1M", "@type": "xsd:string"},
	"field-incident-response-minutes": map[string]any{"@value": "30", "@type": "xsd:integer"},
	"field-required-signature-level":  map[string]any{"@value": "AES", "@type": "xsd:string"},
}

// slaHostingContract instantiates the template: retyped to dcs:Contract (which
// is what puts it under dcs:CanonicalContractShape) with every field filled.
func slaHostingContract(t *testing.T) map[string]any {
	t.Helper()
	contract := slaHostingTemplate(t)
	contract["@type"] = "dcs:Contract"
	metadata := contract["dcs:metadata"].(map[string]any)
	metadata["@type"] = "dcs:ContractMetadata"
	delete(metadata, "dcs:templateType")

	documentID := contract["@id"].(string)
	for fragment, value := range slaNegotiatedValues {
		require.True(t, setInlineFieldValue(contract, documentID+"#"+fragment, value),
			"no contract field %q to fill", fragment)
	}
	return contract
}

// The submission/signing gate must pass the SLA both as an offered template
// and as the instantiated contract.
func TestSLAHostingFixtureConformsToHubShapes(t *testing.T) {
	defer slaHostingShapeSource(t)()

	require.NoError(t, RequireHubConformance(context.Background(), slaHostingTemplate(t)),
		"the offered SLA template must pass the gate it is published through")
	require.NoError(t, RequireHubConformance(context.Background(), slaHostingContract(t)),
		"the negotiated SLA contract must pass the gate it is signed through")
}

// Every rule the parties agreed survives persistence in its own deontic
// bucket: nine rules, split the way assemblePolicySet splits them.
func TestSLAHostingFixtureCarriesNineRules(t *testing.T) {
	defer slaHostingShapeSource(t)()

	root := slaHostingExpanded(t, slaHostingContract(t))
	policySet, ok := expandedFirst(root, dcsNamespace()+"policies")
	require.True(t, ok, "the contract declares an enclosing ODRL policy")

	buckets := map[string]int{
		odrlIRI + "obligation":  4,
		odrlIRI + "permission":  2,
		odrlIRI + "prohibition": 3,
	}
	for bucket, expected := range buckets {
		require.Len(t, expandedValues(policySet, bucket), expected, "rules in %s", bucket)
	}
	require.Len(t, expandedODRLPolicyRules(root), 9, "nine rules reach the policy engine")

	for _, rule := range expandedODRLPolicyRules(root) {
		require.NotEmpty(t, expandedValues(rule, odrlIRI+"action"), "every rule names an action")
		require.NotEmpty(t, expandedValues(rule, dcsNamespace()+"prose"),
			"every machine-readable rule is backed by the clause it operationalizes")
	}
}

// The workload permission's constraint tree is three levels deep and stays so
// through JSON-LD expansion: the builder's ALL root is a plain conjunction
// array (composeConstraintTree), one of whose members is an ANY group over two
// atomic constraints. Flattening any level would silently widen the grant.
func TestSLAHostingWorkloadPermissionKeepsADepthThreeConstraintTree(t *testing.T) {
	defer slaHostingShapeSource(t)()

	permission := slaHostingRule(t, "policy-r3-workload-execution")
	members := expandedValues(permission, odrlIRI+"constraint")
	require.Len(t, members, 3, "the ALL root is a conjunction of three nodes")

	var nested []map[string]any
	for _, raw := range members {
		node, ok := raw.(map[string]any)
		require.True(t, ok)
		if operator, children, isLogical := constraintLogical(node); isLogical {
			require.Equal(t, "or", operator, "the nested group is an ANY group")
			nested = children
		}
	}
	require.Len(t, nested, 2, "the nested ANY group holds both spatial constraints")
	for _, child := range nested {
		_, _, isLogical := constraintLogical(child)
		require.False(t, isLogical, "level three is atomic")
		require.NotEmpty(t, expandedValues(child, odrlIRI+"leftOperand"))
	}
	require.Equal(t, 3, slaConstraintDepth(members), "the tree is three levels deep")
}

// A duty's consequence — what the Provider owes when the duty is unmet — must
// survive as a duty in its own right, with its own action and constraints.
func TestSLAHostingServiceCreditConsequenceSurvives(t *testing.T) {
	defer slaHostingShapeSource(t)()

	permission := slaHostingRule(t, "policy-r3-workload-execution")
	duties := expandedNodes(permission, odrlIRI+"duty")
	require.Len(t, duties, 1, "the workload permission carries the availability duty")

	consequences := expandedNodes(duties[0], odrlIRI+"consequence")
	require.Len(t, consequences, 1, "the availability duty carries the service-credit consequence")
	require.Equal(t, odrlIRI+"compensate", expandedID(expandedValues(consequences[0], odrlIRI+"action")[0]),
		"an unmet availability duty compensates")
	require.NotEmpty(t, expandedValues(consequences[0], odrlIRI+"constraint"),
		"the consequence is bounded by the negotiated service credit rate")
}

// A boundary named as a field IRI is not decoration: it must dereference to
// the value the parties actually agreed, wherever it sits in the tree — on a
// rule, on a duty, or on a consequence.
func TestSLAHostingFieldBoundariesResolveToNegotiatedValues(t *testing.T) {
	defer slaHostingShapeSource(t)()

	root := slaHostingExpanded(t, slaHostingContract(t))
	index := expandedODRLFieldIndex(root)
	require.Len(t, index, 13, "thirteen negotiated fields")

	resolved := 0
	for _, constraint := range slaHostingAtomicConstraints(root) {
		operator := shaclLocalName(expandedID(constraint[odrlIRI+"operator"].([]any)[0]))
		for _, raw := range expandedValues(constraint, odrlIRI+"rightOperand") {
			field, isRef := raw.(map[string]any)
			if !isRef {
				continue
			}
			id, _ := field["@id"].(string)
			info, isField := index[id]
			if !isField {
				continue
			}
			require.True(t, info.hasValue, "boundary field %q carries no negotiated value", id)
			require.Equal(t, info.value, slaScalar(resolveRightOperand(constraint, operator, index)),
				"boundary %q must resolve to its negotiated value", id)
			resolved++
		}
	}
	require.Equal(t, 9, resolved, "every field-named boundary in the SLA resolves")
}

// The whole audit — canonical shapes, the clause catalog, the declared SLA
// library and the embedded ODRL evaluation — reports nothing blocking about a
// properly negotiated SLA.
func TestSLAHostingContractAuditsClean(t *testing.T) {
	defer slaHostingShapeSource(t)()

	findings, err := AuditContractContent(context.Background(), slaHostingContract(t),
		mapPolicy(true, false), ContractContentAuditMetadata{})
	require.NoError(t, err)
	require.Empty(t, errorFindings(findings), fmt.Sprintf("expected a clean audit, got: %+v", errorFindings(findings)))
}

// The imported library is what makes the domain enforceable: a node count
// outside the published shape's bounds is refused, and the same shape is what
// the builder's asset picker offered. Without the library the value is
// unconstrained — which is the difference between "DCS does ODRL" and "DCS
// does hosting SLAs".
func TestSLAHostingLibraryEnforcesTheNegotiatedEnvironment(t *testing.T) {
	defer slaHostingShapeSource(t)()

	contract := slaHostingContract(t)
	require.True(t, setInlineFieldValue(contract, contract["@id"].(string)+"#field-provisioned-nodes",
		map[string]any{"@value": "900", "@type": "xsd:integer"}))

	err := RequireHubConformance(context.Background(), contract)
	require.Error(t, err, "900 nodes exceeds the published maximum of 500")
	require.Contains(t, err.Error(), "provisionedNodes")
}

// ---- helpers ----

func slaHostingExpanded(t *testing.T, document map[string]any) map[string]any {
	t.Helper()
	source, err := requireShapeSource()
	require.NoError(t, err)
	root, err := expandForAudit(context.Background(), document, source)
	require.NoError(t, err)
	return root
}

func slaHostingRule(t *testing.T, fragment string) map[string]any {
	t.Helper()
	contract := slaHostingContract(t)
	wanted := contract["@id"].(string) + "#" + fragment
	for _, rule := range expandedODRLPolicyRules(slaHostingExpanded(t, contract)) {
		if id, _ := rule["@id"].(string); id == wanted {
			return rule
		}
	}
	require.FailNow(t, "no rule "+wanted+" in the fixture")
	return nil
}

// slaConstraintDepth measures how deep a rule's constraint forest nests. The
// forest itself is level one — the builder writes an ALL root as a plain
// conjunction array rather than an explicit odrl:and node — so an atomic
// member sits at level two and a group's members at level three.
func slaConstraintDepth(members []any) int {
	deepest := 0
	for _, raw := range members {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		depth := 1
		if _, children, isLogical := constraintLogical(node); isLogical {
			nested := make([]any, 0, len(children))
			for _, child := range children {
				nested = append(nested, child)
			}
			depth = slaConstraintDepth(nested)
		}
		if depth > deepest {
			deepest = depth
		}
	}
	return deepest + 1
}

// slaHostingAtomicConstraints flattens every constraint the document carries —
// on a rule, on a duty, on a consequence, at any depth in a logical tree.
func slaHostingAtomicConstraints(root map[string]any) []map[string]any {
	out := []map[string]any{}
	var walkConstraint func(node map[string]any)
	walkConstraint = func(node map[string]any) {
		if _, children, isLogical := constraintLogical(node); isLogical {
			for _, child := range children {
				walkConstraint(child)
			}
			return
		}
		if _, ok := node[odrlIRI+"operator"]; ok {
			out = append(out, node)
		}
	}
	var walkNode func(node map[string]any)
	walkNode = func(node map[string]any) {
		for _, raw := range expandedValues(node, odrlIRI+"constraint") {
			if constraint, ok := raw.(map[string]any); ok {
				walkConstraint(constraint)
			}
		}
		for _, duty := range expandedNodes(node, odrlIRI+"duty") {
			walkNode(duty)
		}
		for _, consequence := range expandedNodes(node, odrlIRI+"consequence") {
			walkNode(consequence)
		}
	}
	for _, rule := range expandedODRLPolicyRules(root) {
		walkNode(rule)
	}
	return out
}

// slaScalar unwraps the one-element list resolveRightOperand returns for a set
// operator, so a single field boundary compares equal either way.
func slaScalar(value any) any {
	if list, ok := value.([]any); ok && len(list) == 1 {
		return list[0]
	}
	return value
}
