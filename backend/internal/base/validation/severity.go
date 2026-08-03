package validation

// PolicyFinding severities. A finding says what this audit concluded, so
// "the constraint holds" and "no verdict was reached here" are separate
// tokens: under ADR-33 the second one belongs to the target system that
// executes the contract, and a consumer that buckets it with the first
// reports a check that never ran as evidence that it passed.
const (
	// SeverityError blocks: the contract's own declared values breach a rule.
	SeverityError = "error"
	// SeverityWarning is a defect the author can close before signing.
	SeverityWarning = "warning"
	// SeveritySatisfied is a verdict: this audit evaluated the rule against
	// the document and it holds.
	SeveritySatisfied = "satisfied"
	// SeverityDeferred is the absence of a verdict: the facts arrive at
	// use-time (context operands, duties, consequences) or the operand does
	// not resolve, so the enforcer decides, not this audit.
	SeverityDeferred = "deferred"
	// SeverityInfo is the SHACL sh:Info translation (shaclengine.go) — an
	// advisory non-conformance a shape author marked informational. It is a
	// reported result, so it is not bucketed as passing.
	SeverityInfo = "info"
)

// PolicyFinding sources. AuditContractContent folds four audits into one list
// and they do not share an enforcement point, so a consumer acting on a
// finding needs to know which audit produced it.
const (
	// SourceHubShapes is the Semantic Hub SHACL bundle (ADR-8/ADR-9).
	SourceHubShapes = "hub-shapes"
	// SourceValidationProfile is the hub's SLA validation profile.
	SourceValidationProfile = "validation-profile"
	// SourceContractODRL is the contract's own embedded dcs:policies.
	SourceContractODRL = "contract-odrl"
	// SourcePolicySetODRL is the deployment's contract-content policy set.
	SourcePolicySetODRL = "policy-set-odrl"
)
