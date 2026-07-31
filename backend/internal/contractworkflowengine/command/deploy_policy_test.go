package command

import "testing"

// The target system is where runtime enforcement happens (SRS §1.2, glossary
// "Contract Target System: receives and executes deployed contracts"), so the
// policy it is handed has to carry the rules the parties signed.
func TestDeploymentPolicyCarriesTheSignedRules(t *testing.T) {
	contract := map[string]any{
		"dcs:policies": map[string]any{
			"@type":        "odrl:Offer",
			"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
			"odrl:permission": []any{map[string]any{
				"@type":       "odrl:Permission",
				"odrl:action": map[string]any{"@id": "odrl:use"},
				"odrl:duty": []any{map[string]any{
					"odrl:action": map[string]any{"@id": "odrl:compensate"},
				}},
			}},
		},
	}

	policy := deploymentPolicy(contract, "corr-1")

	if policy["@type"] != "odrl:Set" {
		t.Fatalf("the deployed policy must be an odrl:Set, got %v", policy["@type"])
	}
	if policy["odrl:profile"] == nil {
		t.Fatal("the deployed policy must name the profile its rules are read under")
	}
	permissions, ok := policy["odrl:permission"].([]any)
	if !ok || len(permissions) != 1 {
		t.Fatalf("the deployed policy must carry the contract's permissions, got %#v", policy["odrl:permission"])
	}
	rule, _ := permissions[0].(map[string]any)
	if rule["odrl:duty"] == nil {
		t.Fatal("a permission's duty is what the enforcer gates on; it must survive the handover")
	}
}

func TestDeploymentPolicyOfAContractWithoutRulesIsAnEmptySet(t *testing.T) {
	policy := deploymentPolicy(map[string]any{}, "corr-2")
	if policy["@type"] != "odrl:Set" {
		t.Fatalf("expected an odrl:Set, got %v", policy["@type"])
	}
	for _, property := range odrlRuleProperties {
		if _, present := policy[property]; present {
			t.Fatalf("a contract that states no rules must not gain %s", property)
		}
	}
}
