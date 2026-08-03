package command

import (
	"encoding/json"
	"strings"
	"testing"

	"digital-contracting-service/internal/base/datatype"
)

func TestResolvePendingTargetsPointsRulesAtTheContract(t *testing.T) {
	doc := map[string]any{
		"@id": "https://dcs.example/api/contract/c-1",
		"dcs:policies": map[string]any{
			"odrl:permission": []any{map[string]any{
				"odrl:target": map[string]any{
					"@id": "https://dcs.example/api/template/t-1#pending-target",
				},
			}},
			"odrl:obligation": []any{map[string]any{
				"odrl:target": map[string]any{"@id": "urn:uuid:pending-target"},
			}},
		},
	}
	raw, err := datatype.NewJSON(doc)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := resolvePendingTargets(&raw)
	if err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	if err := json.Unmarshal(*resolved, &out); err != nil {
		t.Fatal(err)
	}
	policies := out["dcs:policies"].(map[string]any)
	for _, bucket := range []string{"odrl:permission", "odrl:obligation"} {
		rule := policies[bucket].([]any)[0].(map[string]any)
		got := rule["odrl:target"].(map[string]any)["@id"].(string)
		if got != "https://dcs.example/api/contract/c-1" {
			t.Fatalf("%s target must name the contract, got %q", bucket, got)
		}
	}
}

func TestResolvePendingTargetsLeavesARealAssetAlone(t *testing.T) {
	const asset = "https://example.org/api/orders"
	doc := map[string]any{
		"@id": "https://dcs.example/api/contract/c-1",
		"dcs:policies": map[string]any{
			"odrl:permission": []any{map[string]any{
				"odrl:target": map[string]any{"@id": asset},
			}},
		},
	}
	raw, err := datatype.NewJSON(doc)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := resolvePendingTargets(&raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(*resolved), asset) {
		t.Fatalf("a rule naming a real resource must keep it: %s", string(*resolved))
	}
}
