package qry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// activeContract is one contract in force with its stored document.
type activeContract struct {
	DID          string `db:"did"`
	ContractData []byte `db:"contract_data"`
}

// missedDeadlineRisks reads the contracts in force and the rules the target
// system reported on, and flags each obligation whose deadline has passed
// without a report received by the deadline.
func (h *ComplianceMonitor) missedDeadlineRisks(ctx context.Context, tx *sqlx.Tx, checkedAt time.Time) ([]event2.ComplianceRisk, error) {
	var contracts []activeContract
	if err := tx.SelectContext(ctx, &contracts,
		`SELECT did, contract_data FROM contracts_effective WHERE state = $1 ORDER BY did`,
		contractstate.Active.String()); err != nil {
		return nil, fmt.Errorf("could not read contracts in force: %w", err)
	}
	var reports []struct {
		DID      string    `db:"did"`
		RuleID   string    `db:"rule_id"`
		Received time.Time `db:"received"`
	}
	if err := tx.SelectContext(ctx, &reports,
		`SELECT did, rule_id, MIN(observed_at) AS received FROM contract_kpis WHERE rule_id IS NOT NULL GROUP BY did, rule_id`); err != nil {
		return nil, fmt.Errorf("could not read reported rules: %w", err)
	}
	reported := make(map[string]time.Time, len(reports))
	for _, report := range reports {
		reported[report.DID+"|"+report.RuleID] = report.Received
	}
	return missedDeadlineRisksFrom(contracts, reported, checkedAt), nil
}

// missedDeadlineRisksFrom flags every obligation whose deadline lies before
// checkedAt and on which no report naming the rule arrived by the deadline
// (reported holds the first receipt time per contract and rule). A late report
// does not clear the miss. The detail carries the rule and the deadline only,
// so the same miss hashes the same on every sweep.
func missedDeadlineRisksFrom(contracts []activeContract, reported map[string]time.Time, checkedAt time.Time) []event2.ComplianceRisk {
	risks := make([]event2.ComplianceRisk, 0)
	for _, contract := range contracts {
		var document map[string]any
		if err := json.Unmarshal(contract.ContractData, &document); err != nil {
			continue
		}
		for _, deadline := range obligationDeadlines(document) {
			if !deadline.due.Before(checkedAt) {
				continue
			}
			if received, ok := reported[contract.DID+"|"+deadline.ruleID]; ok && !received.After(deadline.due) {
				continue
			}
			risks = append(risks, event2.ComplianceRisk{
				DID:      contract.DID,
				RiskType: RiskTypeDeadlineMissed,
				Detail: fmt.Sprintf("obligation %s was due by %s and the target system sent no report on it",
					deadline.ruleID, deadline.due.UTC().Format(time.RFC3339)),
				DetectedAt: checkedAt,
			})
		}
	}
	return risks
}

type obligationDeadline struct {
	ruleID string
	due    time.Time
}

// obligationDeadlines returns the deadline of each duty in the document's
// dcs:policies: an odrl:obligation entry or an odrl:duty nested in any rule,
// constrained by odrl:dateTime with lt or lteq.
func obligationDeadlines(document map[string]any) []obligationDeadline {
	policies, _ := document["dcs:policies"].(map[string]any)
	var deadlines []obligationDeadline
	var walk func(raw any, isDuty bool)
	walk = func(raw any, isDuty bool) {
		for _, rule := range odrlNodes(raw) {
			if id, _ := rule["@id"].(string); isDuty && strings.TrimSpace(id) != "" {
				for _, constraint := range odrlNodes(rule["odrl:constraint"]) {
					if due, ok := dateTimeDeadline(constraint); ok {
						deadlines = append(deadlines, obligationDeadline{ruleID: strings.TrimSpace(id), due: due})
					}
				}
			}
			walk(rule["odrl:duty"], true)
		}
	}
	walk(policies["odrl:obligation"], true)
	walk(policies["odrl:permission"], false)
	walk(policies["odrl:prohibition"], false)
	return deadlines
}

// dateTimeDeadline reads "odrl:dateTime lt|lteq <date>" as a deadline.
func dateTimeDeadline(constraint map[string]any) (time.Time, bool) {
	if odrlTerm(constraint["odrl:leftOperand"]) != "dateTime" {
		return time.Time{}, false
	}
	if operator := odrlTerm(constraint["odrl:operator"]); operator != "lt" && operator != "lteq" {
		return time.Time{}, false
	}
	value := constraint["odrl:rightOperand"]
	if node, ok := value.(map[string]any); ok {
		value = node["@value"]
	}
	text, _ := value.(string)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if due, err := time.Parse(layout, strings.TrimSpace(text)); err == nil {
			return due, true
		}
	}
	return time.Time{}, false
}

// odrlTerm returns the local name of an ODRL term given as a string or an
// {"@id": ...} node, in prefixed or full IRI form.
func odrlTerm(value any) string {
	if node, ok := value.(map[string]any); ok {
		value = node["@id"]
	}
	term, _ := value.(string)
	term = strings.TrimPrefix(term, "http://www.w3.org/ns/odrl/2/")
	return strings.TrimPrefix(term, "odrl:")
}

// odrlNodes reads a slot that JSON-LD lets hold one node or an array of them.
func odrlNodes(raw any) []map[string]any {
	switch value := raw.(type) {
	case map[string]any:
		return []map[string]any{value}
	case []any:
		nodes := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if node, ok := item.(map[string]any); ok {
				nodes = append(nodes, node)
			}
		}
		return nodes
	}
	return nil
}
