# ADR-25: Contract target systems are a configured registry, and each contract designates its own

Status: Accepted (2026-07-27). The registry and the per-contract designation
stand. How a registered target authenticates the acknowledgement it sends back
has since changed: the `DEPLOYMENT_CALLBACK_SECRET` shared by every target when
this was written is replaced, in
[ADR-27](adr-27-machine-credentials-issued-not-configured.md), by an OAuth2
client each target holds as its own registered machine identity. A target
therefore carries an `oauth_client_id` alongside the endpoint described below.

## Context

Deployment addressed exactly one target system, held in the `CONTRACT_TARGET_URL`
environment variable and read at dispatch time (`command.ContractTargetURL`).
Three consequences followed from that single global value.

An operator could not say *where* a contract should go. Every contract in the
deployment went to the same endpoint, so a DCS serving several execution
environments — an ERP for one counterparty, an API gateway for another — could
not be expressed at all. DCS-IR-SI-05 specifies the interface against external
target system**s**, plural.

A deployment record could not say where it went in any durable sense. The
`target_url` column recorded a string copied from process configuration, so the
same contract redeployed after a configuration change produced rows that were
indistinguishable but meant different things.

An alert could not name the target usefully. When a dispatch failed there was
nothing to identify beyond a URL that might since have changed, which is part of
why the failure was only written to the process log.

## Decision

**Target systems are a first-class, persisted registry**, administered through
the UI (UC-09-01, system configuration): a name, an endpoint, a description and
an enabled flag. `CONTRACT_TARGET_URL` is removed rather than kept as a
fallback — a deployment whose destination depends on which of two mechanisms
happens to be configured is worse than one that fails loudly.

**Each contract designates the target it deploys to**, referencing a registry
entry. The designation is part of the contract's own record, not a property of
the deployment run, so:

- the automatic trigger on signing completion (DCS-FR-SM-12) has a destination
  without a human present to choose one;
- the manager-initiated deployment (UC-05 stimulus) sends it to the same place
  by default, and may direct a re-dispatch elsewhere;
- the audit trail and any failure alert can name *which system* was missed.

**A contract with no target designated simply does not deploy, and that is a
normal outcome, not a failure.** Not every party deploys what it signs — a
negotiating peer may hold the agreement without executing it anywhere — so
deployment is opt-in and its absence is neither an error nor an alert. A
*manager who asks for a deployment* on such a contract is refused with a message
saying so, because they asked; the automatic trigger just does nothing.

## Consequences

The deployment record references the registry entry it was dispatched to, so a
later edit to that entry's URL does not rewrite history: the row keeps both the
reference and the endpoint as it stood at dispatch.

Deployment gains a failure state. The row was previously written `DISPATCHED`
*before* the outbound call was attempted and a failure was only logged, so a
deployment the target never received was indistinguishable from one it
acknowledged. Dispatch outcome is now recorded, and the compliance monitor
raises a risk for each failure — reading back the persisted outcome rather than
re-deriving one, as the underperformance alert already does.

Choosing a target becomes a step someone must take. That is the intended cost:
it makes the destination explicit and reviewable instead of implicit in
deployment configuration. The registry is deployment state, so an instance with
one target still configures it once rather than never.

Existing deployments do not migrate. Per the project's greenfield rule the
database may be wiped; no compatibility path reads the old environment variable.

The registry can be seeded from deployment configuration (`contractTargets` in
the chart), so a fresh install — or a test cluster recreated on every run — has
its targets without anyone opening the admin UI. Seeding is **create-only,
matched on name**: an entry an administrator later repoints in the UI is not
reverted by the next restart, because configuration winning every restart would
send deployments somewhere they had been deliberately moved away from.

## Alternatives considered

**A default target in the registry.** Auto-deploy uses the default, manual
deploy picks. Smaller change and no contract can fail for lack of a choice, but
a contract's destination would remain a global setting at signing time —
re-introducing the problem this ADR addresses, one indirection further away.

**Dispatch to every enabled target.** Fits fan-out (an ERP *and* a gateway both
need the contract), but makes "did this contract deploy" a per-target question
and multiplies acknowledgement and failure bookkeeping. Not required by any
current use case; the registry does not preclude adding it.

**Manager-initiated deployment only.** Matches UC-05's stimulus exactly and is
the most predictable, but contradicts DCS-FR-SM-12's MUST for an automatic
trigger on signing completion, so it would have to be recorded as a deviation.
Both mechanisms are specified, and both are kept.
