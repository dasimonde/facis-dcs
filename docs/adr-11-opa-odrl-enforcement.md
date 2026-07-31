# ADR-11: OPA/Rego as the ODRL evaluation engine, replacing the hand-rolled operator switch

## Status (verified 2026-07-30) — read this before the sections below

This ADR is **implemented**. OPA is the production evaluator: `opaodrl.go`
embeds Rego and runs `rego.PreparedEvalQuery`; `odrlexpanded.go` and `kpi.go`
call it; `evaluateODRLConstraint` survives only in `opaodrl_test.go` as the
parity oracle. The sections below were written before the work landed and
still read as prospective. Four statements in them are now wrong and are
corrected here rather than by rewriting the record:

1. **"Own a minimal Go ODRL→Rego compiler"** overstates what shipped. There is
   no compiler: one *static* Rego module answers a single-constraint boolean,
   one constraint at a time. No contract policy is ever compiled to Rego.
   Every deontic decision — prohibition-violated-when-satisfied, `and`/`or`/
   `xone`, `andSequence`, duty recursion, context-operand deferral — is Go in
   `base/validation/odrlexpanded.go`.
2. **The pre-signatory call chain named in Context is not the chain that
   runs.** The blocking gates (`approve.go`, `signingmanagement/command/apply.go`)
   call `ValidateContractPolicySatisfaction`, which evaluates the embedded ODRL
   directly. It does not call `AuditContractContent`; that function's only
   non-test caller is the read-only audit-trail query. There is likewise no
   `/contract/verify` flow — the endpoint referenced in Context does not exist.
3. **The state of the world is `dcs:value`, not `dcs:parameterValue`.**
   Production reads `dcs:value` throughout (`odrlexpanded.go`,
   `contractclosedness.go`). `dcs:parameterValue` survives only as a mapping in
   the served JSON-LD context and has no production reader.
4. **The import path is `github.com/open-policy-agent/opa/v1/rego`**, not
   `opa/rego`.

## Context

ADR-6 established the *shape* of policy in this system — real ODRL emitted as
a well-formed `odrl:Set` under a DCS-published profile, enforced server-side
so a client cannot submit a constraint-violating value and have it accepted.
It deliberately left the *evaluator* hand-rolled:
`evaluateODRLConstraint` (`backend/internal/base/validation/contractcontentaudit.go`)
is a Go `switch` over nine operators (`eq/neq/gt/gteq/lt/lteq/isAnyOf/isNoneOf/hasPart`).

Two SRS-distinct enforcement moments share that one evaluator:

- **Pre-signatory policy checks** (DCS-FR-PACM-03 "Automated Regulatory and
  Policy Compliance Checks", DCS-FR-CSA-07 "Automated Compliance Checks",
  the `/template/verify` and `/contract/verify` flows). Evaluated against the
  contract's own submitted values at negotiate → approve → sign.
  Call sites: `ValidateContractPolicySatisfaction` → `AuditContractContent`
  (`approve.go`, `signingmanagement/command/apply.go`).
- **Execution / KPI monitoring** (DCS-FR-CWE-31 "Contract Performance
  Tracking", DCS-FR-PACM-02 "Compliance Monitoring", `/pac/monitor`).
  Evaluated against an externally-reported runtime KPI value.
  Call site: `EvaluateKPIViolation` (`kpi.go`, from `callback.go`).

The hand-rolled evaluator is correct for what it covers and uses the standard
ODRL operator IRIs and rule semantics (a Prohibition is violated when its
constraint is satisfied), but it is bespoke Go: no standard evaluation
semantics, no machine-readable evaluation report, and an operator/leftOperand
ceiling we maintain by hand.

### Why not "compile ODRL to SHACL and reuse goRDFlib (ADR-9)"

There is **no canonical ODRL→SHACL mapping**. The W3C ODRL Formal Semantics
does not define one, and the Gaia-X Tagus-era experimental ODRL SHACL profile
was dropped from the Danube line. SHACL validates the *structure* of a graph;
ODRL evaluation is a deontic computation over a policy, a request, and a state
of the world. Forcing one onto the other would be exactly the kind of bespoke
mapping this ADR is trying to retire — with none of the ecosystem support.

### Landscape reviewed (W3C ODRL landscape, 2026)

- **W3C ODRL Formal Semantics** (Community Group, not yet a Recommendation)
  now defines an *ODRL Evaluator*: inputs are Policy + State of the World +
  Evaluation Request + behaviour (open/closed); output is a structured
  **Compliance Report** (`PolicyReport → RuleReport → ConstraintReport →
  ConditionReport → ActionReport`).
- **SolidLab ODRL-Evaluator / FORCE** (Ghent University, MIT) — the reference
  implementation of that spec. Node.js, evaluates via the **EYE reasoner**
  (symbolic N3, WASM or native). Most standards-true, emits the Compliance
  Report — but a reasoner per evaluation is the heaviest option and the wrong
  latency profile for the approve/sign hot path.
- **ODRE** (OEG-UPM, Apache-2.0) — embeddable Java/Python library, but
  early-stage (v0.1.2, no releases) and supports only ~6 of ~30 operators and
  1 of 25+ left operands — *fewer than the hand-rolled code already has*.
  Adopting it would be a regression.
- **ODRL-PAP** (eclipse `wistefan/odrl-pap`, DOME/FIWARE, Apache-2.0) — not an
  evaluator but a **translator**: compiles ODRL to **Rego** and serves it as
  OPA bundles; runtime evaluation is **Open Policy Agent (OPA)**. Java/Quarkus,
  GraalVM-native, actively maintained (76 releases, v1.4.10 Jun 2026),
  supports the Gaia-X ODRL Verifiable-Credential profile. This is the
  FIWARE / DSBA / Gaia-X dataspace pattern (ODRL expressed, Rego enforced).

## Decision

**Evaluate ODRL by compiling it to Rego and running it on Open Policy Agent
(OPA), embedded in-process as a Go library
(`github.com/open-policy-agent/opa/rego`).**

- **License**: OPA is **Apache-2.0**, CNCF-graduated — same license as this
  project, on the Eclipse Foundation pre-approved list (Dash IP diligence is
  procedural). ODRL-PAP is Apache-2.0.
- **In-process, no sidecar for evaluation.** OPA is a Go library; the
  approve/sign/KPI hot paths call it directly. This is the lightest and most
  mature option that keeps DCS a self-contained Go service. Contrast: SolidLab
  would add a Node + EYE-reasoner sidecar (rejected on weight/latency), ODRE
  would regress coverage (rejected), raw hand-rolling forgoes a standard
  engine (the status quo we are replacing).
- **Ecosystem fit and reuse.** OPA/Rego is already part of the XFSC / FACIS
  ecosystem this service ships into (the FIWARE/DSBA dataspace stack uses
  ODRL-PAP + OPA). The same embedded OPA is reusable for **credential-based
  login authorization** (role/scope gating over OID4VP claims), so the
  dependency earns its place beyond contract policy.

### ODRL→Rego translation source (staged sub-decision)

The evaluation engine (OPA) is settled; the *translation* of ODRL to Rego has
two viable sources:

1. **Own a minimal Go ODRL→Rego compiler** for the DCS contract-constraint
   profile. Our ODRL is deliberately constrained (ADR-6: single-party
   `odrl:Set`, flat constraint lists, the nine operators above), so a
   deterministic, unit-tested compiler is proportionate, stays in Go, and adds
   no Java sidecar. **Chosen for the initial contract-policy path.**
2. **Adopt eclipse ODRL-PAP** when the credential-based-login / Verifiable-
   Credential authorization work lands — its Gaia-X ODRL-VC profile is exactly
   that domain, and reusing a maintained translator pays off once policies are
   VC-shaped rather than contract-constraint-shaped. **Deferred, revisit at
   that point.**

This keeps the near-term change small and Go-only while leaving a clean path
to the maintained translator where it is worth its weight.

### State of the world

The evaluator's "state of the world" is:

- Pre-signatory: the contract's **inline field values** — `dcs:parameterValue`
  carried on each `dcs:ContractField` (the value-inlining unification;
  the field an `odrl:leftOperand` names now carries its value directly).
- KPI: the externally-reported runtime KPI value passed to
  `EvaluateKPIViolation`, injected as a transient world fact keyed by the same
  field identity.

Both feed the same OPA query; only the source of the value differs — matching
the SRS's two enforcement moments over one policy set.

## Verification gate — met; OPA is wired in

*(Written prospectively. The gate was met and OPA is the production evaluator;
the parity oracle it describes lives in `opaodrl_test.go`. Retained as the
record of what was required, not as an open item.)*

Mirroring ADR-9's goRDFlib gate, OPA is not wired into enforcement until:

1. **License + transitive footprint** verified by import graph, not go.mod
   alone: `go list -deps github.com/open-policy-agent/opa/rego/...` reviewed;
   confirm no unexpected heavyweight or non-permissive transitive dep is
   linked into the binary (`go build` output inspected), and record the
   Apache-2.0 provenance for Dash IP.
2. **Parity gate**: every case in the existing ODRL audit corpus
   (`contractcontentaudit_test.go` — blacklisted country, exceeded maximum,
   invalid/valid jurisdiction, canonical-contract satisfied/violated/missing,
   typed right-operand, KPI violation) must produce an **identical
   pass/violate verdict** through the OPA path as through `evaluateODRLConstraint`.
   The hand-rolled evaluator stays in the tree as the parity oracle until this
   passes, then is removed from the production path and retained solely as the
   test parity oracle (below).
3. **Operator coverage**: the ODRL→Rego compiler covers at least the nine
   operators already supported; any gap fails the gate.

If the gate cannot be met, the hand-rolled evaluator remains and this ADR is
revised — the gate is the trigger to switch, not an assumption that it works.

## Integration — done

*(Written prospectively; the work below has shipped. See Status above for the
four points where this section's wording no longer matches the code.)*

- New dependency `github.com/open-policy-agent/opa/v1/rego`, pinned by version
  in `go.mod`; upgrades re-run the parity gate.
- `evaluateODRLConstraint` and its expanded/compact call sites
  (`odrlexpanded.go` `auditExpandedODRLRule`, `kpi.go` `EvaluateKPIViolation`)
  are replaced by an OPA query built from the compiled Rego for the contract's
  policies plus the state-of-the-world facts. The `PolicyFinding` mapping is
  retained (the PACM audit trail and SM-26 compliance viewer consume findings
  unchanged, exactly as ADR-9 did for SHACL results); OPA's decision, with the
  per-constraint reasons, populates the finding's operator/expected/actual
  detail and can be shaped toward the W3C Compliance Report Model at the
  finding boundary without changing consumers.
- ADR-6's decisions stand unchanged: real ODRL under the DCS profile, emitted
  as an `odrl:Set`, enforced server-side, not trimmable. Only the *evaluator
  mechanism* named in ADR-6 ("Constraint satisfaction is evaluated
  server-side via `evaluateODRLConstraint`") is superseded by this ADR.

## Consequences

- ODRL evaluation runs on a mature, CNCF-graduated, Apache-2.0 engine that is
  already part of the XFSC/dataspace ecosystem, rather than bespoke Go — and
  the same engine is available for credential-based login authorization.
- A translation layer (ODRL→Rego) is introduced and owned; it is deterministic
  and unit-tested, and its correctness is pinned by the parity gate against the
  evaluator it replaces.
- No dual-engine flag (greenfield, per ADR-9's precedent): `evaluateODRLConstraint`
  is removed from the production binary rather than kept behind a
  `POLICY_ENGINE=opa|builtin` switch — nothing in the shipped path evaluates
  ODRL except OPA. The switch is retained only in `opaodrl_test.go` as the
  parity oracle (a test fixture, not a runtime fallback), so any future change
  to `evaluateODRLConstraintOPA` is still checked against the reference
  semantics verdict-for-verdict.
- The value-inlining unification (`dcs:parameterValue` on the field) is a
  prerequisite that stands independently of the engine: it is the clean
  "state of the world" both this OPA path and the prior evaluator read.

## Extension (2026-07-29): issuer trust authorization

The same engine now decides issuer trust (ADR-31). `IssuerTrusted` and
`IssuerMayAttest` evaluate `internal/auth/oid4vp/policy/trust.rego` instead of
the `if` statements that used to hold those rules.

**The line: discovery and verification stay in Go, authorization is policy.**
Chain validation to a trust anchor, the leaf identifying its issuer, signature
and holder binding, revocation, and mechanism dispatch are typed, tested Go —
OPA has crypto builtins, but reimplementing those rules in a policy language
would trade tests for configurability nobody asked for. What moved is the part a
deployment actually changes: which issuers it trusts, for what, and on whose
behalf.

Three properties this buys:

- The rules are auditable as data. "Which issuers may grant a session here" is a
  file an operator reads, not control flow they infer.
- A denial explains itself. The policy emits `reasons`, surfaced in the error a
  caller reports; the previous booleans said only "no".
- Adding a distinction no longer means adding an enum value. The overloaded
  `peer` purpose — one grant meaning both "may authorize a signature here" and
  "may attest a counterparty's authority" — is separable by writing two rules
  rather than by changing the trust schema and every consumer of it.

The trust document travels as evaluation **input**, not as a bound data
document. Bound data is a snapshot taken at first evaluation, so a configuration
changed afterwards would go on being judged against what it said at startup,
and nothing would report the difference.

`OID4VP_TRUST_POLICY_PATH` replaces the embedded policy for a deployment whose
authorization rules differ. A policy that cannot be loaded or evaluated
**denies** — treating a broken policy as permissive would turn a configuration
mistake into silent trust.

The policy has tests of its own (`policy/trust_test.rego`). They run under
`opa test backend/internal/auth/oid4vp/policy` and, so they cannot rot quietly,
inside `go test` as well, which means CI runs them with no new tooling.
