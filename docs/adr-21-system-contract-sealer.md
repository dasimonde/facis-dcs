# ADR-21: System Contract Sealer — the seal ADR-17 deliberately deferred

## Status

Accepted (2026-07-24) as direction. **Implementation: pending — not scoped to
any current branch or session; future work.** Extends
[ADR-17](adr-17-machine-signing-is-not-an-aes-signature.md), which explicitly
left this option open rather than ruling it out:

> Electronic seals are not implemented. If sealing by a legal person is ever
> wanted — an organization attesting a document rather than a person signing
> it — it is a separate instrument needing its own key custody, its own
> certificate profile (seal certificates under Annex III), its own PAdES
> profile and its own SRS requirement. It must not be reached by relaxing
> this decision.

This ADR is that separate instrument, sketched out but not built.

## Context

ADR-17 stripped **System Contract Signer** (SRS §2.4 Table 5) of all signing
scope: a machine caller has no natural person behind it and no sole control,
so whatever it produced could at best be a seal, never an AES/QES signature —
and the class was kept, scopeless, so the refusal stays documented and
tested (`features/12_system_based_contract_management/system_user_classes.feature`,
and a live ORCE compliance probe,
`deployment/helm/charts/orce/flows/system-user-capabilities-flow.json`, that
errors `'COMPLIANCE DEFECT - a System Contract Signer was allowed to sign'`
if that ever stops being true).

Separately, the *human* signing path
(`backend/internal/signingmanagement/command/apply.go`) already supports
exactly the "download → sign externally with whatever tool → re-upload"
model an eIDAS seal would need: `PrepareSignature` returns a real, complete
PDF (PoA already embedded, not a hash-to-be-signed) and pins its exact bytes
on the ceremony; `SubmitSignature` accepts a whole finished PAdES-signed PDF
back and verifies the re-upload only *added* a signature — a
`bytes.HasPrefix` check against those pinned bytes (`docs/adr-20-signing-
acceptance-hardening.md` §2; this supersedes the `assertSubmittedPayloadIsOurs`
attachment-only comparison this ADR originally cited, deleted by ADR-20 as
insufficient) — rather than changing the document. DCS is, by design,
ignorant of how the signature was produced. Nothing about that machinery
requires a human or a wallet; it requires the resubmitted PDF to still say
what DCS thinks it says.

That's the seal ADR-17 deferred: reuse the same generic verify-on-reupload
path, without a PoA (a machine sealing for itself, not a person acting under
delegated organizational authority), gated to a correctly-named,
correctly-scoped role instead of AES's/QES's natural-person requirement.

## Decision (proposed)

- **Rename `SystemContractSigner` → `SystemContractSealer`** (SRS deviation
  record, same pattern ADR-17 already established: "SRS text should be
  revised"). Sibling enum values in
  `backend/internal/base/datatype/userrole/userrole.go` follow a
  `SystemContract<Role>` naming convention this fits directly.
- **Grant it exactly one new capability**: call the existing `Prepare` (no
  PoA — it seals as itself, not on an organization's delegated behalf) and
  `SubmitSignature` endpoints, the same generic content-verify gate the
  human external-signing path already uses. No new ceremony machinery.
- **Scope of DCS's guarantee is deliberately narrow**: DCS verifies the
  re-upload is cryptographically valid (DSS AdES/PAdES validation) and
  content-faithful (the `bytes.HasPrefix` pinned-byte check, ADR-20 §2), and
  *labels it correctly*
  in its own records (a new `instrument: seal` discriminator on the signature
  record / audit trail / provenance credential — the signature record
  schema carries no such field and would otherwise silently record this as
  an AES/QES signature, which is exactly the misrepresentation ADR-17
  exists to prevent).
- **DCS explicitly does NOT police the certificate profile** (that the key
  used is genuinely an Annex III seal certificate rather than an Annex I
  signature certificate misused as one). That determination — using the
  legally correct instrument for the integrator's context — is the
  integrating organization's responsibility, not DCS's. DCS's own
  responsibility stops at not lying about what it received.

## Open design question (unresolved — for whoever implements this)

Does the Sealer's path go through the existing signing-*ceremony* concept
(`signature_request.go`'s OID4VP/verifiable-presentation gate that must pass
before `prepare()` becomes reachable) at all, or does it bypass "ceremony" as
a concept entirely — a machine role has no wallet and no VP to present, so a
simpler, direct `Prepare`/`SubmitSignature` pair (no `startCeremony`) may be
the correct shape rather than reusing ceremony terminology built for a human
identity-verification step it doesn't need.

**Added consideration (ADR-20, 2026-07-25):** whichever shape wins, the
sole-control cert-identity check ADR-20 added compares the certificate's
`GIVEN_NAME`/`SURNAME` against the ceremony's verified **PID**
(`namesMatch` requires both non-empty, and fails closed — rejects — when
either side is empty; it never no-ops open). A Sealer ceremony has no PID at
all, so reusing `SubmitSignature` completely unmodified would fail-closed on
*every* Sealer submission, not silently accept any of them — safe, but
unusable. Implementing this ADR means giving the Sealer path its own
identity check in place of the PID-name comparison (the sealing
organization's identity, not a natural person's name — a different check
than "DCS explicitly does NOT police the certificate profile" below, which
is about *whether* the cert is a qualified Annex III seal cert, not *which*
identity it names), not bypassing ADR-20's gate outright.

## Consequences

- **Not a pure rename.**
  `features/12_system_based_contract_management/system_user_classes.feature`'s
  four scenarios currently assert *refusal* ("cannot start a signing
  ceremony," "cannot prepare," "cannot submit," "may still verify") — these
  need rewriting to assert the new *acceptance* behavior for sealing, while
  presumably still refusing anything wallet/AES-shaped.
- **The ORCE compliance probe must be updated in the same change.**
  `system-user-capabilities-flow.json` will start false-alarming
  (`COMPLIANCE DEFECT`) the moment this role can sign/seal anything, unless
  its assertion is inverted to match the new intended behavior.
- Other touch points already scoped: `querybyid.go`'s `privilegedReadRoles`
  (trivial, behavior-preserving), `values.bdd.yml`'s system-client role grant
  string. (`docs/TRACEABILITY_SRS_BDD.md` no longer exists, pruned as a stale
  doc in a prior sweep — its SRS §2.4 Table 5 citation has no surviving home
  to update; if this ADR is implemented, record the citation in this ADR's
  own SRS-coverage note instead of recreating that file.)
- Until implemented, **ADR-17's decision stands exactly as written** —
  System Contract Signer (not yet renamed) still holds no signing scope, and
  the existing BDD/ORCE refusal coverage still guards that.
