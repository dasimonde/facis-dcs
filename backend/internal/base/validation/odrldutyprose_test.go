package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A service-credit clause — the element that makes an SLA an SLA rather than a
// price list — is a Permission carrying a Duty (pay the credit) whose
// consequence (escalate) fires when the duty is unmet. Every one of those nodes
// is an odrl:Duty, so dcs:OdrlDutyProseShape demands prose of each: the
// authoring path must bind the clause block to the whole rule tree, not only to
// its top-level rule.

// serviceCreditPermission mirrors the document the clause editor's store now
// assembles: one clause block, one Permission, a nested Duty with a
// consequence, every rule node backed by that block.
func serviceCreditPermission() map[string]any {
	return map[string]any{
		"@id":           "urn:uuid:policy-service-credit",
		"@type":         "odrl:Permission",
		"odrl:action":   map[string]any{"@id": "odrl:use"},
		"odrl:assigner": map[string]any{"@id": "urn:uuid:party-provider"},
		"odrl:assignee": map[string]any{"@id": "urn:uuid:party-customer"},
		"odrl:target":   map[string]any{"@id": "did:example:contract"},
		"dcs:prose":     map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:duty": []any{map[string]any{
			"@type":       "odrl:Duty",
			"odrl:action": map[string]any{"@id": "odrl:compensate"},
			"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
			"odrl:constraint": []any{
				map[string]any{
					"@type":             "odrl:Constraint",
					"odrl:leftOperand":  map[string]any{"@id": "urn:uuid:field-company-country"},
					"odrl:operator":     map[string]any{"@id": "odrl:eq"},
					"odrl:rightOperand": map[string]any{"@type": "xsd:string", "@value": "DEU"},
				},
			},
			"odrl:consequence": []any{map[string]any{
				"@type":       "odrl:Duty",
				"odrl:action": map[string]any{"@id": "odrl:inform"},
				"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
			}},
		}},
	}
}

func TestNestedDutyAndConsequenceConformWhenBoundToProse(t *testing.T) {
	defer odrlVocabularyShapeSource(t)()

	contract := canonicalAuditContract()
	contract["dcs:policies"].(map[string]any)["odrl:permission"] = []any{serviceCreditPermission()}

	require.NoError(t, RequireHubConformance(context.Background(), contract),
		"a service-credit permission whose duty and consequence cite the clause they are backed by must submit")
}

// The structural validator ran ahead of the shapes: it declared a nested duty
// party-less and prose-less, so a document it accepted was refused by
// dcs:OdrlDutyProseShape at submit, hours later and on a different screen. The
// two now say the same thing.
func TestNestedDutyWithoutProseIsRefusedBeforeSubmit(t *testing.T) {
	for _, unbound := range []string{"duty", "consequence"} {
		t.Run(unbound, func(t *testing.T) {
			permission := serviceCreditPermission()
			duty := permission["odrl:duty"].([]any)[0].(map[string]any)
			if unbound == "duty" {
				delete(duty, "dcs:prose")
			} else {
				delete(duty["odrl:consequence"].([]any)[0].(map[string]any), "dcs:prose")
			}

			require.ErrorContains(t, validateODRLRuleShape(permission), "duty is missing dcs:prose")
		})
	}
}

// The prose backing is the point of dcs:prose, so it stays enforced on nested
// nodes: the fix binds prose, it does not narrow the shape to policy-level
// duties.
func TestNestedDutyWithoutProseIsStillRefused(t *testing.T) {
	defer odrlVocabularyShapeSource(t)()

	for _, unbound := range []string{"duty", "consequence"} {
		t.Run(unbound, func(t *testing.T) {
			contract := canonicalAuditContract()
			permission := serviceCreditPermission()
			duty := permission["odrl:duty"].([]any)[0].(map[string]any)
			if unbound == "duty" {
				delete(duty, "dcs:prose")
			} else {
				delete(duty["odrl:consequence"].([]any)[0].(map[string]any), "dcs:prose")
			}
			contract["dcs:policies"].(map[string]any)["odrl:permission"] = []any{permission}

			require.Error(t, RequireHubConformance(context.Background(), contract))
		})
	}
}
