# ADR-29: PoA organizational authority is issuer-bound, and Integration Manager scopes integrations, not access

Status: accepted
Date: 2026-07-28

## Context

Two role and credential questions surfaced while preparing the two-instance demo.

**1. The demo credential issuer let the holder choose the organization.** Its
`/offer` form carried a free-text "Organization (DID)" field, defaulting to
`did:web:demo-org.facis.example`, and the issuer signed whatever value was
submitted into the credential's `organization` claim. The credential's `iss` was
never holder-supplied — it is derived from the issuer's own base URL — but the
organization the holder claims to act for was.

The SRS defines a Power of Attorney as "a credential granting the holder
authority to act **on behalf of another party**" (Table 3, PoA). Section 2.3
requires "Signer Authorization & Power of Attorney (PoA) Credential Chain
Verification … ensuring they have the necessary authority to act on behalf of
their organizations. If a PoA exists, the system performs a credential chain
verification using a PoA Trust Service."

A self-asserted organization inverts that. The authority is supposed to flow
from the party to the holder; a form field makes the holder the source. There is
then no chain back to the organization for chain verification to verify, and
because DCS binds contract parties on the `organization` claim, a demo user
could mint themselves authority to act for any party — including the
counterparty instance.

**2. `Integration Manager` was scoped to machine identity management.** The role
is specified (UC-11-01, FR-CSA-25) but was declared in `userrole.go` without
being accepted by any endpoint, so a credential carrying it granted nothing.
Wiring it up raised the question of where its boundary lies. The first pass gave
it both the contract target registry and the machine identity registry — the
latter of which can mint an OAuth2 client carrying arbitrary system roles,
including `Sys. Contract Signer`.

SRS Table 4 separates the two concerns explicitly:

- **System Administrator** — "Maintains system configurations, permissions, and
  user access."
- **Integration Manager** — "Manages third-party system integrations (e.g., ERP,
  Trust Services)."

And Table 5 establishes that machine identities *are* users: the System Contract
Creator/Reviewer/Approver/Manager/Signer classes, plus "Contract Target System —
External system that receives and executes deployed contracts."

## Decision

**The organization in a PoA is the issuing party's own identity, never holder
input.** A credential issuer represents exactly one party and issues PoAs only
for that party. The demo issuer derives the `organization` claim from its own
host — the participant DID that the issuer DID is a sub-path of — and the form
field is removed rather than defaulted.

```
issuer base URL   https://dcs-ionos.facis.cloud/issuer
issuer DID        did:web:dcs-ionos.facis.cloud:issuer
organization      did:web:dcs-ionos.facis.cloud
```

Multiple parties are represented by multiple issuers, not by one issuer with a
selectable organization. Where a deployment genuinely needs one issuer to serve
several parties, the permitted set must be server-side configuration, never a
request parameter.

**`Integration Manager` scopes integrations; access management stays with
`System Administrator`.** Concretely:

| Endpoint group | Classification | Role |
|---|---|---|
| Contract target registry (list/create/update/delete/rotate secret) | third-party system integration | `Integration Manager` + `Sys. Administrator` |
| Machine identity registry (list/create/update/delete/rotate secret) | permissions and user access | `Sys. Administrator` only |

Creating a machine identity issues a credential carrying Table 5 system roles.
That it authenticates an API client rather than a person does not make it an
integration concern — it is granting user access, which Table 4 assigns to the
System Administrator.

**`Validator` and `Process Orchestrator` remain declared but unscoped.** For
`Validator` the capability already exists and is reachable: automatic gates that
run regardless of actor (contract closedness, SHACL hub conformance, ODRL policy
evaluation, enforced at submit/offer/approve/sign/deploy) plus human-triggered
surfaces under `Compliance Officer` (`/pac/monitor`, incident findings) and
`Auditor` (`/pac/audit`, `/pac/report`). Its only backing requirement,
FR-PACM-03, is descoped by client decision (ADR-24), which explicitly leaves
check initiation with Compliance Officer and Auditor. Adding a `Validator` scope
would be a third name for capabilities those roles already hold and would cut
against that decision. Both stay in the enum because they are SRS Table 4
vocabulary and role claims carrying them must remain valid.

## Consequences

- A demo participant can no longer obtain authority to act for a party that did
  not issue their credential. The two demo instances each issue for themselves,
  which is what makes cross-instance contracting a two-party exchange rather
  than two users of one fictional organization.
- `Integration Manager` cannot escalate to `Sys. Contract Signer` by minting a
  machine identity. The privilege boundary follows from the role definitions
  rather than from a special case.
- The issuer flow set is now chart content (`charts/orce/flows-issuer/`,
  selected by `flowsDir`) instead of existing only in a live cluster's
  ConfigMap, so an issuer can be rebuilt from source.
- A role that is specified but not yet implemented stays declared and unscoped.
  That is deliberate: it keeps SRS traceability while making "specified" and
  "enforced" distinguishable. The demo issuer must offer only roles that are
  actually enforced, or a participant receives a credential that logs in and
  grants nothing.
