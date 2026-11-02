package workflowgate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/templatestructure"
	"digital-contracting-service/internal/base/validation"
)

// SourceRegulatory marks the findings of the regulatory rule pack (ADR-37).
// The rules are technical checks of eIDAS and GDPR preconditions. They are
// not a legal assessment.
const SourceRegulatory = "regulatory-rule-pack"

// The rule pack runs before execution only: the deployment gate.
const regulatoryGate = "deployment"

const (
	RuleEIDASLevel        = "regulatory.eidas.signature-level"
	RuleQualifiedTSA      = "regulatory.eidas.qualified-timestamp"
	RuleGDPRRetention     = "regulatory.gdpr.retention-policy"
	RuleNoPIDInPDF        = "regulatory.gdpr.no-pid-in-pdf"
	RuleHierarchyComplete = "structure.hierarchy.parent-in-force"
	RuleHierarchyLevels   = "structure.hierarchy.template-levels"
)

// signedField is one signature this instance applied: the signature field and
// the level the validated signature achieved.
type signedField struct {
	Field    string `db:"field_name"`
	Signer   string `db:"signer_did"`
	Achieved string `db:"credential_type"`
}

// regulatoryFacts are the persisted facts the rule pack evaluates besides the
// contract document.
type regulatoryFacts struct {
	Signatures []signedField
	// PIDValues are the values of the signatories' PID claims.
	PIDValues []string
	// TSAQualified is the operator's declaration that the configured TSA is a
	// qualified trust service (DCS_TSA_QUALIFIED).
	TSAQualified bool
	// RetentionSet is true when the archived contract has a retention period.
	RetentionSet bool
	// ParentDID is the dcs:parentContract of the contract, ParentState the
	// state of that parent on this instance ("" when it is not known here).
	ParentDID, ParentState string
	// Template is the structure of the contract's template and ParentTemplate
	// that of the parent contract's template. Each is nil when it is not
	// known on this instance.
	Template, ParentTemplate *templatestructure.Template
}

// tsaQualified reads DCS_TSA_QUALIFIED. No QTSP is operated by default.
func tsaQualified() bool {
	enabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("DCS_TSA_QUALIFIED")))
	return enabled
}

func loadRegulatoryFacts(ctx context.Context, db *sqlx.DB, did string, content map[string]any) (regulatoryFacts, error) {
	facts := regulatoryFacts{TSAQualified: tsaQualified(), ParentDID: parentContractDID(content)}
	if err := db.SelectContext(ctx, &facts.Signatures, `
        SELECT COALESCE(field_name, '') AS field_name, signer_did, credential_type
        FROM contract_signatures
        WHERE contract_did = $1 AND status = 'SIGNED'
        ORDER BY id`, did); err != nil {
		return facts, fmt.Errorf("read signatures: %w", err)
	}
	var claims [][]byte
	if err := db.SelectContext(ctx, &claims, `
        SELECT pid_claims FROM signature_ceremonies
        WHERE contract_did = $1 AND pid_claims IS NOT NULL`, did); err != nil {
		return facts, fmt.Errorf("read PID claims: %w", err)
	}
	for _, raw := range claims {
		facts.PIDValues = append(facts.PIDValues, pidValues(raw)...)
	}
	if err := db.GetContext(ctx, &facts.RetentionSet, `
        SELECT EXISTS (SELECT 1 FROM contract_archive_entries
                       WHERE did = $1 AND deleted_at IS NULL AND retention_until IS NOT NULL)`, did); err != nil {
		return facts, fmt.Errorf("read archive retention: %w", err)
	}
	if facts.ParentDID != "" {
		err := db.GetContext(ctx, &facts.ParentState, `SELECT state FROM contracts_effective WHERE did = $1`, facts.ParentDID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return facts, fmt.Errorf("read parent contract: %w", err)
		}
		if facts.ParentTemplate, err = templatestructure.LoadForContract(ctx, db, facts.ParentDID); err != nil {
			return facts, err
		}
	}
	var err error
	if facts.Template, err = templatestructure.LoadForContract(ctx, db, did); err != nil {
		return facts, err
	}
	return facts, nil
}

func parentContractDID(content map[string]any) string {
	parent := content["dcs:parentContract"]
	if list, ok := parent.([]any); ok && len(list) > 0 {
		parent = list[0]
	}
	if node, ok := parent.(map[string]any); ok {
		id, _ := node["@id"].(string)
		return base.ResourceKey(id)
	}
	return ""
}

// pidClaimNames are the PID claims that identify a natural person. A given
// name alone is too common to identify anybody and is only matched together
// with the family name. A locality or country is not personal on its own.
var pidClaimNames = []string{
	"family_name", "birth_family_name", "birthdate", "birth_date", "birth_place",
	"personal_administrative_number", "email", "phone_number", "street_address",
}

func pidValues(raw []byte) []string {
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return nil
	}
	var values []string
	for _, name := range pidClaimNames {
		if value, ok := claims[name].(string); ok && strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	if address, ok := claims["address"].(map[string]any); ok {
		if street, ok := address["street_address"].(string); ok && strings.TrimSpace(street) != "" {
			values = append(values, street)
		}
	}
	given, _ := claims["given_name"].(string)
	family, _ := claims["family_name"].(string)
	if strings.TrimSpace(given) != "" && strings.TrimSpace(family) != "" {
		values = append(values, given+" "+family, family+", "+given)
	}
	return values
}

func regulatoryFinding(rule, title, severity, message string) validation.PolicyFinding {
	return validation.PolicyFinding{
		PolicySetID: SourceRegulatory, PolicyVersion: "1", RuleID: rule, Title: title,
		Severity: severity, Message: message, Source: SourceRegulatory,
	}
}

// regulatoryFindings evaluates the rule pack. Every rule yields one finding at
// least, so the gate evidence shows what was checked and not only what failed.
func regulatoryFindings(content map[string]any, facts regulatoryFacts) []validation.PolicyFinding {
	raw, _ := json.Marshal(content)
	var findings []validation.PolicyFinding

	// eIDAS: the level each signature achieved meets the level its field requires.
	levelFindings := 0
	for _, sig := range facts.Signatures {
		required := validation.RequiredCredentialType(raw, sig.Field)
		if validation.SignatureLevelSatisfies(sig.Achieved, required) {
			continue
		}
		levelFindings++
		findings = append(findings, regulatoryFinding(RuleEIDASLevel, "eIDAS signature level", validation.SeverityError,
			fmt.Sprintf("signature of %s for field %q achieved %s, the contract requires %s", sig.Signer, sig.Field, sig.Achieved, required)))
	}
	if levelFindings == 0 {
		findings = append(findings, regulatoryFinding(RuleEIDASLevel, "eIDAS signature level", validation.SeveritySatisfied,
			fmt.Sprintf("%d applied signature(s) meet the level their field requires", len(facts.Signatures))))
	}

	// eIDAS: a qualified signature needs a qualified time stamp.
	requiresQES := false
	for _, field := range validation.RequiredSignatureFields(raw) {
		requiresQES = requiresQES || validation.RequiredCredentialType(raw, field) == "QES"
	}
	switch {
	case requiresQES && !facts.TSAQualified:
		findings = append(findings, regulatoryFinding(RuleQualifiedTSA, "Qualified time stamp", validation.SeverityError,
			"the contract requires QES, but the configured TSA is not declared qualified (DCS_TSA_QUALIFIED)"))
	case requiresQES:
		findings = append(findings, regulatoryFinding(RuleQualifiedTSA, "Qualified time stamp", validation.SeveritySatisfied,
			"the contract requires QES and the configured TSA is declared qualified"))
	default:
		findings = append(findings, regulatoryFinding(RuleQualifiedTSA, "Qualified time stamp", validation.SeveritySatisfied,
			"no signature field requires QES, so no qualified time stamp is required"))
	}

	// GDPR: erasure is the archive deletion with key shredding (ADR-28); the
	// retention period is what an archived contract must declare.
	if facts.RetentionSet {
		findings = append(findings, regulatoryFinding(RuleGDPRRetention, "GDPR retention policy", validation.SeveritySatisfied,
			"the archived contract has a retention period; erasure is the archive deletion with key shredding"))
	} else {
		findings = append(findings, regulatoryFinding(RuleGDPRRetention, "GDPR retention policy", validation.SeverityInfo,
			"the archived contract has no retention period; erasure is the archive deletion with key shredding"))
	}

	// GDPR: the contract document is the payload and the source of the
	// rendered PDF, so a signatory's PID must not occur in it.
	// Limit: exact match of whole string values; a PID value inside a longer text is not found.
	pid := make(map[string]bool, len(facts.PIDValues))
	for _, value := range facts.PIDValues {
		pid[strings.ToLower(strings.TrimSpace(value))] = true
	}
	var leaked []string
	walkStrings(content, func(value string) {
		if pid[strings.ToLower(strings.TrimSpace(value))] {
			leaked = append(leaked, value)
		}
	})
	if len(leaked) > 0 {
		findings = append(findings, regulatoryFinding(RuleNoPIDInPDF, "No PID in the contract PDF", validation.SeverityWarning,
			fmt.Sprintf("the contract document contains %d value(s) of a signatory's PID claims", len(leaked))))
	} else {
		findings = append(findings, regulatoryFinding(RuleNoPIDInPDF, "No PID in the contract PDF", validation.SeveritySatisfied,
			"the contract document contains no value of a signatory's PID claims"))
	}

	// Structure (DCS-FR-PACM-06): a sub-agreement executes only under a parent in force.
	switch {
	case facts.ParentDID == "":
	case facts.ParentState == "":
		findings = append(findings, regulatoryFinding(RuleHierarchyComplete, "Parent contract in force", validation.SeverityWarning,
			fmt.Sprintf("parent contract %s is not known on this instance", facts.ParentDID)))
	case facts.ParentState != "SIGNED" && facts.ParentState != "ACTIVE":
		findings = append(findings, regulatoryFinding(RuleHierarchyComplete, "Parent contract in force", validation.SeverityError,
			fmt.Sprintf("parent contract %s is %s, not SIGNED or ACTIVE", facts.ParentDID, facts.ParentState)))
	default:
		findings = append(findings, regulatoryFinding(RuleHierarchyComplete, "Parent contract in force", validation.SeveritySatisfied,
			fmt.Sprintf("parent contract %s is %s", facts.ParentDID, facts.ParentState)))
	}

	// Structure (DCS-FR-PACM-06, DCS-FR-TR-26): a contract has the parent its
	// template level and required templates call for. An unknown parent is
	// flagged by the rule above.
	if facts.Template != nil && (facts.ParentDID == "" || facts.ParentTemplate != nil) {
		if err := templatestructure.CheckParent(*facts.Template, facts.ParentTemplate); err != nil {
			findings = append(findings, regulatoryFinding(RuleHierarchyLevels, "Template levels and dependencies", validation.SeverityError, err.Error()))
		} else {
			findings = append(findings, regulatoryFinding(RuleHierarchyLevels, "Template levels and dependencies", validation.SeveritySatisfied,
				fmt.Sprintf("the %s contract has the parent its template requires", facts.Template.Level)))
		}
	}
	return findings
}

func walkStrings(value any, visit func(string)) {
	switch typed := value.(type) {
	case string:
		visit(typed)
	case map[string]any:
		for _, child := range typed {
			walkStrings(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkStrings(child, visit)
		}
	}
}
