package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The policy engine evaluates arbitrary and/or/xone constraint trees
// (odrlexpanded.go) and the authoring UI can build them, but the clause
// catalog required leftOperand/operator/rightOperand of every odrl:constraint
// value — which an odrl:LogicalConstraint has none of. Every logical tree the
// builder produced was therefore refused by hub conformance. These tests hold
// the shapes level with what the engine and the builder support.

func TestLogicalConstraintTreesConformToHubShapes(t *testing.T) {
	defer odrlVocabularyShapeSource(t)()

	for _, combinator := range []string{"odrl:and", "odrl:or", "odrl:xone", "odrl:andSequence"} {
		t.Run(combinator, func(t *testing.T) {
			contract := canonicalAuditContract()
			duty := contract["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)[0].(map[string]any)
			duty["odrl:constraint"] = map[string]any{
				"@type": "odrl:LogicalConstraint",
				combinator: map[string]any{"@list": []any{
					atomicConstraint("odrl:eq", "DEU"),
					atomicConstraint("odrl:neq", "FRA"),
				}},
			}

			require.NoError(t, RequireHubConformance(context.Background(), contract),
				"a %s constraint tree is evaluable by the policy engine, so the shapes must accept it", combinator)
		})
	}
}

// A direct atomic constraint is still validated: accepting logical
// constraints alongside must not turn the atomic shape into a rubber stamp.
func TestAtomicConstraintIsStillValidated(t *testing.T) {
	defer odrlVocabularyShapeSource(t)()

	contract := canonicalAuditContract()
	duty := contract["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)[0].(map[string]any)
	broken := atomicConstraint("odrl:eq", "DEU")
	broken["odrl:operator"] = map[string]any{"@id": "odrl:notAnOperator"}
	duty["odrl:constraint"] = broken

	require.Error(t, RequireHubConformance(context.Background(), contract),
		"an unknown operator must still be refused")
}

// A nested duty is an odrl:Duty like any other, so the prose-backing rule
// applies to it — the authoring UI does not attach prose to nested duties
// today, which is a separate defect this change does not touch.
func TestPermissionWithNestedDutyConformsWhenDutyCarriesProse(t *testing.T) {
	defer odrlVocabularyShapeSource(t)()

	contract := canonicalAuditContract()
	contract["dcs:policies"].(map[string]any)["odrl:permission"] = []any{map[string]any{
		"@id":           "urn:uuid:policy-permission-1",
		"@type":         "odrl:Permission",
		"odrl:action":   map[string]any{"@id": "odrl:use"},
		"odrl:assigner": map[string]any{"@id": "urn:uuid:party-provider"},
		"odrl:assignee": map[string]any{"@id": "urn:uuid:party-customer"},
		"odrl:target":   map[string]any{"@id": "urn:uuid:policy-target"},
		"dcs:prose":     map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:duty": []any{map[string]any{
			"@type":       "odrl:Duty",
			"odrl:action": map[string]any{"@id": "odrl:compensate"},
			"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
		}},
	}}

	require.NoError(t, RequireHubConformance(context.Background(), contract),
		"a permission whose duty compensates, backed by prose, is the archetypal ODRL payment obligation")
}

func atomicConstraint(operator, right string) map[string]any {
	return map[string]any{
		"@type":             "odrl:Constraint",
		"odrl:leftOperand":  map[string]any{"@id": "urn:uuid:field-company-country"},
		"odrl:operator":     map[string]any{"@id": operator},
		"odrl:rightOperand": map[string]any{"@type": "xsd:string", "@value": right},
	}
}
