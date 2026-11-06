package validation

import "context"

// ClauseSectionProjection is one clause row for PACM-07 By Clauses reporting.
type ClauseSectionProjection struct {
	ClauseID               string
	Title                  string
	Status                 string
	Findings               []PolicyFinding // defects only (excludes satisfied)
	ApplicableChecks       string
	ApplicableChecksStatus string
	VerdictNote            string
}

// MergeClauseDomainFieldFindings indexes ContractFields, evaluates ontology
// ValueConstraints (and presence/type for unconstrained DomainFields) for
// uniquely referenced fields, and appends those executed results to findings.
// Does not mutate the contract document. Safe to call when the ontology is
// unavailable — returns findings unchanged with a non-nil error.
func MergeClauseDomainFieldFindings(
	ctx context.Context,
	contractDocument any,
	clauses []ClauseComponent,
	findings []PolicyFinding,
) ([]PolicyFinding, error) {
	fields, err := IndexContractFields(contractDocument)
	if err != nil {
		return findings, err
	}
	domainFindings, err := AssessClauseDomainFields(ctx, clauses, fields)
	if err != nil {
		return findings, err
	}
	if len(domainFindings) == 0 {
		return findings, nil
	}
	out := make([]PolicyFinding, 0, len(findings)+len(domainFindings))
	out = append(out, findings...)
	out = append(out, domainFindings...)
	return out, nil
}

// ProjectClauseSections maps audit findings onto clause components and derives
// coverage-aware statuses. Used by the clause-report query and integration tests.
func ProjectClauseSections(
	clauses []ClauseComponent,
	findings []PolicyFinding,
	documentID string,
	index ShapeTargetIndex,
	indexOK bool,
	auditRan bool,
) (sections []ClauseSectionProjection, unassigned []PolicyFinding) {
	mapped := MapFindingsToClauses(findings, clauses, documentID)

	byClauseDefects := map[string][]PolicyFinding{}
	byClauseAll := map[string][]PolicyFinding{}
	unassigned = make([]PolicyFinding, 0)
	for _, m := range mapped {
		if len(m.ClauseIDs) == 1 {
			id := m.ClauseIDs[0]
			byClauseAll[id] = append(byClauseAll[id], m.Finding)
			if m.Finding.Severity != SeveritySatisfied {
				byClauseDefects[id] = append(byClauseDefects[id], m.Finding)
			}
			continue
		}
		if m.Finding.Severity == SeveritySatisfied {
			continue
		}
		unassigned = append(unassigned, m.Finding)
	}

	sections = make([]ClauseSectionProjection, 0, len(clauses))
	for _, c := range clauses {
		defects := byClauseDefects[c.ID]
		if defects == nil {
			defects = []PolicyFinding{}
		}
		attributed := byClauseAll[c.ID]
		if attributed == nil {
			attributed = []PolicyFinding{}
		}
		cov := DeriveClauseCoverage(ClauseCoverageInput{
			Clause:      c,
			Attributed:  attributed,
			AllFindings: findings,
			AllClauses:  clauses,
			AuditRan:    auditRan,
			IndexOK:     indexOK,
			Index:       index,
		})
		sections = append(sections, ClauseSectionProjection{
			ClauseID:               c.ID,
			Title:                  c.Title,
			Status:                 cov.Status,
			Findings:               defects,
			ApplicableChecks:       FormatApplicableChecksLabel(cov),
			ApplicableChecksStatus: cov.ApplicableChecks,
			VerdictNote:            cov.VerdictNote,
		})
	}
	return sections, unassigned
}
