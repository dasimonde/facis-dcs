package validation

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The authoring UI's action and operator pickers must offer only terms the
// hub shapes accept. Offering more produced templates from which no valid
// contract could ever be derived — a user picking odrl:compensate for a
// payment obligation, the semantically correct ODRL term, authored a template
// that failed SHACL validation days later on a peer instance. These tests run
// every offered term through the real SHACL pass against the shipped catalog.

func odrlVocabularyShapeSource(t *testing.T) func() {
	t.Helper()
	return swapShapeSource(t, fixtureShapeSource{
		shapesTTL: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-shapes.ttl") +
			"\n\n" + mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-clause-catalog.ttl"),
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld"),
	})
}

func TestEveryOfferedODRLActionValidatesAsADuty(t *testing.T) {
	defer odrlVocabularyShapeSource(t)()

	actions := offeredODRLTerms(t, "ODRL_ACTIONS")
	require.Greater(t, len(actions), 40, "expected the full ODRL 2.2 core action vocabulary")

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			contract := canonicalAuditContract()
			rules := contract["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)
			rules[0].(map[string]any)["odrl:action"] = map[string]any{"@id": action}

			findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
			require.NoError(t, err)
			requireNoPolicyFinding(t, findings, "action-InConstraintComponent",
				"the authoring UI offers %s, so the clause catalog must permit it", action)
		})
	}
}

func TestEveryOfferedODRLOperatorValidatesInAConstraint(t *testing.T) {
	defer odrlVocabularyShapeSource(t)()

	for _, operator := range offeredODRLTerms(t, "ODRL_OPERATORS") {
		t.Run(operator, func(t *testing.T) {
			contract := canonicalAuditContract()
			rules := contract["dcs:policies"].(map[string]any)["odrl:obligation"].([]any)
			constraint := rules[0].(map[string]any)["odrl:constraint"].(map[string]any)
			constraint["odrl:operator"] = map[string]any{"@id": operator}

			findings, err := AuditContractContent(context.Background(), contract, mapPolicy(true, false), ContractContentAuditMetadata{})
			require.NoError(t, err)
			// Whether the constraint is satisfied is a separate verdict; the
			// operator itself must never be rejected as unknown vocabulary.
			requireNoPolicyFinding(t, findings, "operator-InConstraintComponent",
				"the authoring UI offers %s and the policy engine evaluates it", operator)
		})
	}
}

func requireNoPolicyFinding(t *testing.T, findings []PolicyFinding, ruleID, format string, args ...any) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			require.Failf(t, "unexpected finding "+ruleID, format+"\ngot: %s", append(args, finding.Message)...)
		}
	}
}

var offeredTermPattern = regexp.MustCompile(`id:\s*'([^']+)'`)

// offeredODRLTerms reads the ids of one exported OdrlTerm[] out of the
// authoring UI's vocabulary module — the list the picker actually renders.
func offeredODRLTerms(t *testing.T, constName string) []string {
	t.Helper()
	source := mustReadRepoFile("frontend/ClientApp/src/modules/template-repository/utils/odrl-vocabulary.ts")
	start := strings.Index(source, "export const "+constName+": OdrlTerm[] = [")
	require.GreaterOrEqual(t, start, 0, "%s not found in odrl-vocabulary.ts", constName)
	end := strings.Index(source[start:], "\n]")
	require.Greater(t, end, 0, "unterminated %s literal", constName)

	var ids []string
	for _, match := range offeredTermPattern.FindAllStringSubmatch(source[start:start+end], -1) {
		ids = append(ids, match[1])
	}
	require.NotEmpty(t, ids, "no term ids parsed from %s", constName)
	return ids
}
