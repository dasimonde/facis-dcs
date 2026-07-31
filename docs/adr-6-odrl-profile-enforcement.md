# ADR-6: Real ODRL under a DCS profile, enforced server-side

## Context

The template builder's earlier machine-readable output was not ODRL by any
reasonable reading: bare, actionless `odrl:Duty` nodes with no enclosing
`odrl:Set`/`odrl:Policy`, no declared profile, and no parties or target.
No standard policy engine or Contract Target System could consume it, and
FR-PACM-03's constraint-satisfaction requirement was enforced (if at all)
only in the frontend — trivially bypassable by anyone calling the API
directly, which makes it a security gap, not a UX nicety.

## Decision

- Policies are emitted as a well-formed `odrl:Set` under a DCS-published
  profile IRI (`https://w3id.org/facis/dcs/ontology/v1/odrl-profile`,
  `frontend/ClientApp/src/modules/template-repository/utils/sla-ontology-catalog.ts`).
- Rule types are concrete (`OdrlDuty`, `OdrlPermission`, `OdrlProhibition`
  — `frontend/ClientApp/src/models/semantic/facis-dcs-semantic.ts`), never
  the abstract `odrl:Rule`.
- Constraint satisfaction is evaluated **server-side**
  (`evaluateODRLConstraint`, `backend/internal/base/validation/contractcontentaudit.go`)
  at acceptance and signing time — a client cannot submit a
  constraint-violating value and have it silently accepted, because the
  frontend is not the enforcement point.
  **(Superseded by ADR-11 on the evaluator *mechanism* only: the hand-rolled
  `evaluateODRLConstraint` is replaced by ODRL→Rego on embedded OPA. The
  server-side, not-trimmable enforcement decision here stands unchanged.)**
- ~~The `odrl:Offer` → `odrl:Agreement` two-party upgrade and compound
  (AND/OR-nested) constraint expressions are not implemented — every
  shipped policy is a single-party `odrl:Set` with a flat constraint
  list.~~
  **Superseded — both have since shipped.** The `odrl:Offer` →
  `odrl:Agreement` upgrade is sealed on the first signature
  (`signingmanagement/command/apply.go`, `sealAgreementForSigning`; asserted by
  `apply_seal_test.go`) and guarded on renewal
  (`contractworkflowengine/command/renew.go`). Compound constraints are
  authorable in the frontend rule builder
  (`clauses-editor/constraint-draft.ts`), audited recursively over
  `odrl:and` / `odrl:or` / `odrl:xone` / `odrl:andSequence`
  (`base/validation/odrlexpanded.go`), and SHACL-tested
  (`odrlconstrainttree_test.go`). Do not read this bullet as current scope.

## Consequences

- A Contract Target System (DCS's example ORCE flow) can
  consume the emitted `odrl:Set` as real ODRL, not a bespoke shape it has
  to special-case.
- Server-side enforcement is **not trimmable** — it is a security property,
  not a feature.
  **(Amended by ADR-24: `DCS-FR-PACM-03` — autonomous system-side *legal*-
  conformity assessment — is descoped by client decision, so it is no longer
  the authority for this bullet. ADR-24 explicitly retains the technical
  conformance gates, SHACL hub conformance and the policy audits, which is
  what this ADR is actually about; the decision stands on that basis.)**
- The emitted `odrl:Set` is validated against ODRL SHACL shapes in CI and
  can express the SRS Appendix C example policy.
