# ADR-33: The target system reports verdicts, the DCS records them

Status: Accepted (2026-07-31). Supersedes the second of the two enforcement
moments [ADR-11](adr-11-opa-odrl-enforcement.md) established: the execution /
KPI-monitoring moment, whose call site was `EvaluateKPIViolation`. ADR-11's
first moment — server-side evaluation of a contract's own declared values
before a signature is admitted — stands unchanged, as does the OPA/Rego
evaluator it selected.

## Context

`DCS-FR-CWE-31` requires reported KPI values to be checked against the
contract's obligations, and `DCS-FR-CWE-09` requires deviations to be surfaced.
The DCS satisfies both today by re-deriving the verdict itself: the target
system posts a bare `(metric, value)` to the deployment callback, and
`validation.EvaluateKPIViolation` walks the contract's ODRL to decide whether
that value breaches it.

That places the classification in the wrong component, and the code says so.
`EvaluateKPIViolation`'s own contract records that it walks only each rule's
direct `odrl:constraint` list: a constraint nested in a `LogicalConstraint`
(`odrl:and` / `odrl:or` / `odrl:xone`), or under `odrl:duty` or
`odrl:consequence`, is never reached, and an unbound metric returns false. A
`false` therefore means *not violated OR not evaluated*, and the callback
persists both as `Violation: false`. The two are indistinguishable in
`contract_kpis`.

The gap is not an oversight to be closed by extending the walk. It is
structural:

- **The DCS cannot see the facts.** A duty to compensate is discharged by a
  payment the DCS never observes. A permission conditioned on `odrl:spatial`
  or `odrl:dateTime` is exercised against an access context only the enforcer
  is given. The DCS holds the terms; the target system holds the events.
- **The DCS is the weaker evaluator by construction.** The target system is an
  API gateway or ERP that must already decide, per request, whether to grant
  access — otherwise it could not enforce at all. Any verdict the DCS derives
  is a second, poorer opinion about a decision that was already made upstream
  with better information.
- **The SRS puts execution there.** §1.2 names the target system as the point
  of "automated runtime enforcement"; the glossary defines a Contract Target
  System as "an external system that receives and executes deployed contracts";
  §4.13 (UC-13 External System Contract Execution) states that it "ensures
  contract enforcement in target systems". `DCS-IR-SI-05` specifies the
  interface as carrying "status queries and event callbacks" — observations
  flowing back, not facts flowing in to be judged.

  `DCS-FR-CWE-09` is the requirement that reads against this, and it is quoted
  here in full because half of it is easy to miss: the system MUST monitor
  obligations and flag SLA violations, and **"Compliance rules MUST be
  enforced throughout the contract lifecycle."** Read alone that sentence puts
  enforcement on the DCS. It is satisfied by the enforcement moment ADR-11
  established and this ADR keeps: the DCS refuses to approve or admit a
  signature on a contract whose own declared values violate its own rules
  (`approve.go`, `apply.go`). What the DCS cannot enforce is a rule whose
  inputs arrive after execution begins, and §4.13 assigns exactly those to
  the target system. The lifecycle is covered by both components, not by one.

A system that reports "compliant" for an obligation it silently could not
evaluate is worse than one that reports nothing: the green row is read as
evidence.

How far that already goes is worth stating: `EvaluateKPIViolation` binds a
reported metric to a contract-data node `@id`, and no shipped producer sends
one — only the BDD harness and the Playwright spec do. The shipped target flow
reports its own activation latency, which binds to nothing. In production the
DCS-derived KPI verdict has therefore only ever returned `false`. Every KPI row
in a running deployment is a green row for a check that never ran.

## Decision

**The target system classifies; the DCS records, attributes, and audits.**

1. **The callback carries a verdict, not only a fact.** A KPI report states
   what was observed *and* what the enforcer concluded — satisfied, violated,
   or not evaluated — together with the rule it concluded about. The DCS
   persists the reported verdict rather than deriving one.

2. **"Not evaluated" is a distinct, first-class outcome.** It is neither a
   violation nor compliance, and it never renders as either. Where the DCS
   cannot say, it says that.

3. **The DCS does not overrule an upstream verdict.** It may refuse a
   malformed report, and it still checks that the reporter is the registered
   target the contract deployed to (ADR-25/ADR-27), but it does not
   re-adjudicate the terms. Attribution, tamper evidence, alerting and the
   audit trail stay with the DCS — those are its competences, and the reported
   verdict is evidence like any other.

4. **Enforcement of ODRL rules is a target-system responsibility.** The DCS's
   obligation is to deliver the rules the parties signed, in the documented
   `odrl:policy` slot of the deployment envelope, so the enforcer can act on
   them. The contract-time audit keeps validating that a policy is
   *well-formed and closed* — that is a document property, checkable without
   the world.

## Consequences

The DCS stops needing an ODRL evaluation engine that matches a real enforcer's
coverage. That is a substantial reduction in scope: logical constraints,
duties, consequences, context operands and profile-defined operands no longer
have to be independently implemented and kept in step with whatever the target
system actually does. This is the triage benefit, and it is the main reason to
take the decision now rather than after the engine has been half-built.

`EvaluateKPIViolation` is superseded. Removing it must not silently turn
existing violations into compliance: the callback's payload gains the verdict
fields first, and a report that carries no verdict is recorded as *not
evaluated* — never as compliant. Per the project's greenfield rule the old rows
are not migrated.

The compliance monitor's KPI-breach class (ADR-32) now reads a reported verdict
instead of re-deriving one. Its other three classes — awaiting approval, denied
access, deployment never received — are derivations over the DCS's own state
and are unaffected.

The audit view must stop rendering a deferred finding in the passing bucket.
`odrlexpanded.go` already records that "info" means DEFERRED, not passed; under
this ADR that distinction becomes load-bearing rather than advisory, because a
deferred constraint is precisely one whose verdict belongs upstream.

That is not a re-colouring. `info` is currently emitted for a *satisfied*
constraint as well as a deferred one, so the severity has to be split into two
tokens before any view changes — otherwise every satisfied constraint turns
amber across the audit view, the CSV and the exported PDF, which count `info`
as passed. The split reaches the external executor vocabulary too, which offers
only PASSED / FAILED / REVIEW and has no value for "not evaluated". This is the
largest piece of work the ADR implies, not an afterthought to it.

The verdict field, the deletion of `EvaluateKPIViolation` and the scenarios
that exercise it land as ONE change. Accepting a verdict while still deriving
one is the dual path the project's greenfield rule forbids, and the alternative
— recording every verdict-less report as not evaluated while the old tests still
assert a derived flag — breaks those tests on the same commit either way.

Until the target system reports verdicts, nothing produces a KPI breach at all:
the shipped ORCE flow evaluates nothing, so `CONTRACT_UNDERPERFORMANCE` stops
being raised on the day this lands. That is a true statement of what the system
knows, replacing a false one, but it is visible and should not surprise anyone.

A target system that reports nothing leaves the contract unobserved, and the
DCS says exactly that. It does not infer compliance from silence — which is
what an unbound metric returning `false` amounts to today.

## Alternatives considered

**Extend `EvaluateKPIViolation` to walk the full constraint tree.** Closes the
nesting hole and needs no interface change. Rejected: it deepens the
duplication. The DCS would still be deciding, from a single reported value,
questions whose inputs — payment received, region of access, elapsed time —
it does not hold, and every future ODRL feature would have to be implemented
twice.

**Have the DCS pull observations and evaluate them.** Keeps classification in
one place and needs no target-side change. Rejected for the same reason plus a
worse one: it makes the DCS's verdict authoritative over the component that
actually granted or denied the access, so the record could contradict what
happened.

**Accept both verdicts and flag disagreement.** Attractive as tamper evidence —
a target claiming compliance where the terms say otherwise is worth seeing.
Rejected for now because it requires the DCS to have the coverage this ADR
removes; the disagreement signal is only meaningful once both evaluators are
complete. Worth revisiting if a deployment ever needs to distrust its own
target system, which is a different problem from enforcing a contract.
