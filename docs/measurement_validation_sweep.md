# Measurement and Validation Sweep

Status: Refreshed working draft -- traceability and evidence triage; no blanket implementation claim

Refresh snapshot:

- Repository state: branch `audit_check`, commit `a2686c2650432d5673f032c63e8029a3974c5668`
  (`2026-07-26`), plus the uncommitted documentation/configuration changes listed by `git status`.
- Refresh date: `2026-07-27`.
- Scope: current SRS, approved decisions, production-code candidates, exact requirement tags in
  `features/`, and locally available JUnit results.
- The full BDD run on `2026-07-26` reached feature 19 but was terminated by the outer 30-minute
  command timeout. Before the IPFS Document Manager liveness fix, later scenarios also encountered
  `connection refused` and timeout failures. Individual results from that run are observations, not
  a clean-suite verification.
- A clean focused rerun of `features/19_c2pa_conformance/c2pa_conformance.feature` after the
  infrastructure fix completed successfully: 9 scenarios passed and 2 decision/gap scenarios were
  skipped.
- `.reports/junit/` is append-only in the current harness and contains results from different dates.
  A report is evidence only when its timestamp and originating command are recorded; the directory
  count alone is not a test result.

Final Ticket-27 addendum (`2026-07-29`):

- Scope: external sequential audit-checkpoint publication and the shared Semantic Hub/ORCE gates for
  submission, offer, approval, signature and deployment. Exact requirement trace is
  DCS-FR-TR-03 (`docs/SRS_FACIS_DCS.txt:2301-2306`), DCS-FR-PACM-02
  (`docs/SRS_FACIS_DCS.txt:2874-2877`) and DCS-FR-PACM-03
  (`docs/SRS_FACIS_DCS.txt:2879-2882`), refined by ADR-6, ADR-8, ADR-9, ADR-11 and ADR-16.
- Independent final verification reported all 27 Ticket-27 scenarios and 188 steps green. The final
  semantic re-verification reported 22 scenarios and 162 steps green. The verifier also reported the
  complete backend suite, race checks, ORCE flow and Helm checks, JUnit output and the specification
  fingerprint guard green.
- This addendum verifies the concrete, configured semantic-rule and executor behavior. It does not
  create a generic legal-compliance oracle. Production PoA trust profile, QES/credential policy,
  domain-specific status mapping and further Federated Catalogue decisions remain deferred/out of
  scope.
- Recovery/lease handling for a manual-review continuation left in `DISPATCHING` by a hard process
  termination is a separate future ticket. The current implementation durably refuses parallel
  continuation (`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:581-599`).

Status vocabulary used in this refresh:

- `partial`: implementation/test candidates exist, but required coverage or clean execution is
  incomplete.
- `candidate_not_run`: current traceability exists, but no clean result was captured in this refresh.
- `observed_failure`: a current execution failed or errored; infrastructure-caused failures require
  a clean rerun and do not prove a product defect.
- `missing_trace`: no exact current implementation/test evidence was identified.
- `underspecified` / `blocked_external`: an objective result needs a decision or external resolution.
- `confirmation_required`: stakeholder input narrows the choice but is still internally ambiguous or
  lacks the exact normative profile/version.
- `architecture_candidate`: a proposed implementation/configuration direction exists, but its input,
  output, default behavior and approval are not yet specified.
- `integration_gap` / `compatibility_gap`: the target architecture or compatibility direction is
  known, but the integration or translation path is incomplete.
- `not_applicable_by_decision`: the original obligation was explicitly removed or replaced; the
  replacement is traced separately.
- `verified` is reserved for an atomic obligation with all evidence required by section D. This
  refresh does not assign `verified` to an aggregate validation path.

Scope decision recorded on `2026-07-27`:

- The unbounded requirement to check contracts against generic "eIDAS, GDPR, ISO and internal
  policies" is withdrawn. DCS does not contain an independent legal-standards oracle and must not
  claim generic compliance with those labels.
- The decided semantic-validation source is the set of concrete, versioned rules registered in the
  Semantic Hub. Contract artifacts pin the applicable Semantic Hub versions; validation resolves and
  evaluates those exact rules.
- ADR-8 defines the Semantic Hub as the only enforcement source and its version-pinning behavior;
  ADR-9 defines the SHACL engine and result mapping; ADR-23 defines arbitrary Semantic Hub SHACL
  libraries as the source of vocabulary semantics.
- Whether a concrete rule represents a legal, organizational or domain policy is metadata/governance
  of that registered rule. Runtime enforcement is based on its SHACL/validation-profile content and
  severity, not on DCS interpreting the name of a legal framework.
- Deployment-specific/custom runtime policies are delegated through the
  versioned `facis-pac-audit-executor/v1` HTTP boundary. The shipped ORCE
  `/audit/run` flow is the reference implementation for this environment; a
  deployment may configure another compatible executor. This proves an
  executable integration seam, but its findings remain the result of the
  configured rules and must not be presented as blanket legal or regulatory
  compliance.

DSS result-policy safety defaults accepted on `2026-07-27`:

- A configured but unreachable DSS is a hard failure; no policy evaluation may turn it into a
  successful validation.
- A missing or invalid per-DCS result policy is a hard failure.
- `TOTAL-PASSED` plus `QESIG` satisfies a QES requirement. A cryptographically valid AES/`ADESIG`
  does not satisfy a QES requirement: its integrity check may pass, but its signature-level check
  fails and blocks the workflow.
- `TOTAL-FAILED` and definitive cryptographic, hash, revocation or certificate failures map to
  `failed`. A recognized `INDETERMINATE` result for an incomplete or temporarily inconclusive check
  maps to `limited`. DSS transport/parse failure and unknown or unmapped results map to
  `unavailable`. Every category except `passed` blocks acceptance.
- The unmodified DSS report is retained alongside the derived DCS category and workflow decision.
- Rego receives a normalized, versioned DCS input rather than the arbitrary raw DSS tree. Its input
  contains the validation purpose, contract-required signature level and normalized DSS
  `indication`, `sub_indication`, `qualification` and `signature_format`. Its output contains
  `category`, `decision`, `reason_code` and `message`.
- Transport and report-shape failures are rejected before Rego evaluation. Only
  `category=passed` together with `decision=allow` may continue the workflow.
- Every derived result records the normalized input, policy version, policy content hash and policy
  output for audit and replay. The initial policy uses a small allowlist of understood DSS results;
  unknown results fail closed as `unavailable`.

## Purpose and authority

This document inventories every measurement vector and every product-validation path that can be
derived from the current requirements sources. It deliberately does not treat code, feature tags,
tests, comments, READMEs, or planning/status metadata as proof that a requirement is met.

Source precedence:

1. `docs/SRS_FACIS_DCS.txt` as the machine-readable requirements source, including its explicit
   `Measurement` / `Verification Method` vectors and acceptance tables.
2. `docs/SRS_FACIS_DCS.pdf` as the original reference when extraction, formatting, or content in
   the text version is ambiguous.
3. Approved ADRs and final decision documents for the architecture or product questions they
   explicitly decide. Drafts and pending decisions are not binding.
4. Repository artifacts only as traceability candidates to be independently verified later.

The sweep separates three concepts that the SRS sometimes places close together:

- **Measurement vector**: an observation or metric to collect.
- **Validation path**: product behavior that accepts an input, performs checks, and returns or
  enforces an outcome.
- **Verification method**: the future evidence needed to demonstrate the requirement.

## Extraction rules

- Preserve one row per independently falsifiable vector, even when one requirement lists several
  metrics or checks.
- Prefer the effective requirement over SRS wording on conflict.
- Mark missing targets, fixtures, registered Semantic Hub rule artifacts, or external dependencies
  explicitly.
- Treat `Done` only as planning metadata. It is not evidence.
- Record positive, negative, boundary, stale-state, concurrency, and external-failure cases.
- Do not silently discard an SRS-only vector. Classify it as supplementary or conflicting.

## A. Explicit SRS measurement vectors

The SRS contains 32 NFR entries with an explicit `Measurement` field. None of those 32 fields
defines a numeric acceptance threshold. Some parent requirements do contain a binary normative
criterion (for example TLS 1.3), but the measurement field still lacks a sample size, test profile,
environment, pass threshold, or observation window.

| ID | Measurement vector(s) | SRS verification method | Target gap / boundary |
|---|---|---|---|
| DCS-NFR-PER-01 | Response time under load; throughput | Documentation review; testing | Load model, percentiles, concurrency and pass limits absent |
| DCS-NFR-PER-02 | Scalability results; supported users; supported connections | Documentation review; testing | Baseline, growth curve and acceptable degradation absent |
| DCS-NFR-PER-03 | Uptime percentage; MTTR | Documentation review; testing | Availability target and observation window absent |
| DCS-NFR-SF-01 | Recovery success rate; MTTR | Documentation review; testing | Failure classes, state-loss tolerance and targets absent |
| DCS-NFR-SF-02 | Remote-access security results; penetration findings | Documentation review; testing | Applicable remote-admin surface and acceptable findings absent |
| DCS-NFR-SF-03 | RTO; RPO | Documentation review; testing | Required RTO/RPO values and disaster scenarios absent |
| DCS-NFR-SEC-01 | TLS version compliance; security-audit results | Documentation review; testing | Normative protocol rule exists; scan scope and zero-tolerance rule not stated in measurement |
| DCS-NFR-SEC-02 | Standards compliance; cryptographic strength | Documentation review; testing | Authoritative standard/version and allowed algorithm catalogue absent |
| DCS-NFR-SEC-03 | Authentication success rate; unauthorized attempts | Documentation review; testing | Success-rate target is ambiguous; denial/logging targets absent |
| DCS-NFR-SEC-04 | Configuration-integrity verification logs | Documentation review; testing | Covered configuration set and expected log evidence absent |
| DCS-NFR-SEC-05 | Integrity-test results; detected tampering attempts | Documentation review; testing | Assets, attacks, detection target and remote-attestation scope absent |
| DCS-NFR-SEC-06 | Secure-storage compliance tests; key-access logs | Documentation review; testing | Required hardware class and acceptable access pattern absent |
| DCS-NFR-SEC-07 | Test coverage; security vulnerabilities found | Documentation review | Coverage type/threshold and acceptable vulnerability severity absent |
| DCS-NFR-SEC-08 | Number of data breaches | Documentation review; testing | Observation window and preventive test proxy absent |
| DCS-NFR-SEC-09 | Log availability; detected anomalies; uptime | Documentation review; testing | Retention, completeness, anomaly and uptime targets absent |
| DCS-NFR-SEC-10 | Integrity hash checks; detected modifications | Documentation review; testing | Protected objects, cadence and detection target absent |
| DCS-NFR-SEC-11 | Detected anomalies; MTTD | Documentation review; testing | Incident corpus and MTTD target absent |
| DCS-NFR-SEC-12 | Configuration-compliance score | Documentation review; source review | Benchmark/profile and minimum score absent |
| DCS-NFR-SEC-13 | Completed secure-deletion tasks | Documentation review; testing | Data classes, deletion semantics and completion target absent |
| DCS-NFR-SEC-14 | Encryption coverage percentage | Documentation review; testing | Data inventory, key handling and required percentage absent |
| DCS-NFR-SEC-15 | Security issues found in reviews | Documentation review; source review | Counting findings rewards detection, not remediation; severity gate absent |
| DCS-NFR-SEC-16 | Successful federated logins | Documentation review; testing | Provider matrix, negative cases and pass ratio absent |
| DCS-NFR-SEC-17 | Successful secure-boot validations | Documentation review; testing | Platform boundary and expected failure behavior absent |
| DCS-NFR-SEC-18 | Policy-aligned disclosures; consented M2M transactions; consent-audit results | Documentation review; testing; consent logs | Data-minimization oracle, consent validity and target absent |
| DCS-NFR-SQ-01 | Code-review/audit results; documentation completeness | Documentation review; source review | Style rules, dead-code policy and completeness metric absent |
| DCS-NFR-SQ-02 | Build success rate; deployment consistency | Documentation review; testing | Platforms, repetitions and target absent |
| DCS-NFR-SQ-03 | Container deployment/execution; orchestrator compatibility; CI/CD integration | Documentation review; deployment testing | Required environment matrix and pass criteria absent |
| DCS-NFR-SQ-04 | Privacy-impact assessment results | Documentation review | Required method, approver and acceptable residual risk absent |
| DCS-NFR-SQ-05 | Legally recognized signed transactions | Documentation review; testing | Legal profile, trust anchors and recognition oracle absent |
| DCS-NFR-SQ-06 | Successfully integrated third-party systems | Documentation review; testing | Required systems/protocols and semantic interoperability criteria absent |
| DCS-NFR-SQ-07 | WCAG compliance; usability results | Documentation review; testing | WCAG version/level and usability thresholds absent |
| DCS-NFR-SQ-08 | Working FACIS orchestration/XFSC integration | Documentation review; testing | Approved architecture decisions may permit adapters; deployed component/profile absent |

### A.1 C2PA measurement vectors

These ten vectors are explicit SRS requirements. Their applicability may be changed only by an
approved, traceable scope or deviation decision, never inferred from code, feature tags, or an
implementation-status claim.

| ID | Measurement and target | Required verification | Mandatory cases / failure cases |
|---|---|---|---|
| DCS-OR-C2PA-001 | Valid C2PA manifest on 100% of contract PDFs — scope confirmed by stakeholder input `2026-07-27` | Tool validation; documentation review | Missing, malformed, unsigned/untrusted and hash-mismatched manifests |
| DCS-OR-C2PA-002 | 100% pass both PDF-signature and C2PA verification after update | Sign -> append -> verify; PDF/C2PA tools | Embedded and remote manifest; multiple increments; corrupted increment |
| DCS-OR-C2PA-003 | SRS target: 100% coverage of seven states and all required fields | Schema validation; manifest unit tests | Stakeholder input tentatively narrows events to cross-DCS negotiation/renewal changes; this conflicts with the seven-state SRS target and needs a formal scope decision before draft, active, amended, suspended, terminated, expired or replaced can be removed |
| DCS-OR-C2PA-004 | 100% lifecycle events have matching VC | VC signature verification; VC/C2PA cross-check | Contract/file/status/reason/time mismatch; missing/revoked/untrusted VC |
| DCS-OR-C2PA-005 | Approved status appears in list within <= 5 minutes — target confirmed; status checks must be renewed | Integration test; timestamp comparison | Suspension, termination, stale/cache/outage, refresh and clock-boundary cases |
| DCS-OR-C2PA-006 | Correct banner for 100% of cases | Automated verifier; manual UX check | Independently check PDF signature, C2PA, VC and status list; no aggregate false-positive |
| DCS-OR-C2PA-007 | Key and PoA policy exists; two successful rotations/year | Policy/drill review; key-ID audit | Untrusted/rotated/revoked key; absent/expired/out-of-scope PoA |
| DCS-OR-C2PA-008 | 100% verification after embedded metadata stripping — requirement retained despite unclear provenance | Strip -> verify; remote-fetch logs | Missing remote manifest, unavailable endpoint, wrong contract binding and tampered remote data |
| DCS-OR-C2PA-009 | Trusted timestamp and audit entry for 100% of events | Log review; TSA verification | Trust candidates are EU Trusted List-qualified services or explicitly configured certificate chains; exact trust-validation policy remains to be confirmed |
| DCS-OR-C2PA-010 | Zero PDFs lose legal-signature validity after C2PA updates | Signature validation before/after | PAdES Baseline B-T is the current candidate; exact ETSI profile/version, lifecycle update order and repeated updates require confirmation/evidence |

## B. Authoritative product-validation paths

The following paths are derived from SRS requirements and approved decisions. A row is a
behavioral obligation, not a claim that a single endpoint or existing test covers it.

| Path | Authoritative source(s) | Input and operation | Required outcome | Required negative/boundary coverage | Status / gap |
|---|---|---|---|---|---|
| VAL-01 Template Semantic Hub content validation | DCS-FR-TR-07; ADR-8/-9/-23 | Persisted root plus immediate component snapshots; applicable versioned Semantic Hub shapes/profile | Findings cover composition-wide content while root-only rules stay root-scoped and identify the applied Hub version | Malformed root/component remains a finding; stale repository component must not replace snapshot; no recursive snapshot walk; missing Hub version hard-fails | **partial** — exact active BDD candidate passed in the interrupted run; the Hub source/version decision is resolved, while full negative/boundary evidence remains open |
| VAL-02 Template compliance and integrity verification | DCS-FR-TR-20 | Read-only verify request for retrieved template and persisted snapshots | Reproducible integrity/compliance report with component-origin paths | Reusable component changes after composition; standalone component; nested snapshot boundary | **partial** — exact active BDD candidate passed; snapshot and nested-boundary coverage is not established |
| VAL-03 Direct template dependency validation | DCS-FR-TR-26 | Direct DIDs at selection and create/update, inside persistence transaction | Every reference exists, is COMPONENT, reusable, non-self and acyclic; valid write commits | Missing/malformed/wrong type/wrong state/self/cycle; state changes after selection; failure leaves persistence unchanged | **candidate_not_run** — exact tag now exists in the hierarchy/bundle feature; no clean result captured |
| VAL-04 Contract assembly validation | DCS-FR-CWE-03 | Reusable clauses/templates plus contract metadata and content | Structure, required metadata and content logic pass before assembled result proceeds | Missing/duplicate/incompatible components; malformed metadata; contradictory logic | **partial + underspecified** — exact active happy-path candidate passed; the content-logic oracle and required negative matrix remain open |
| VAL-05 Human contract review validation | DCS-FR-CWE-14, DCS-FR-CWE-25, DCS-IR-CWE-05 | Submitted negotiated contract, reviewer identity/comments | Assigned reviewer can inspect, validate, comment and produce routed status change | Unauthorized/unassigned reviewer; stale version; rejection/comments; concurrent update | **partial + underspecified** — interface/state-machine candidate exists, but no exact trace for both functional sources and no complete reviewer/concurrency oracle |
| VAL-06 External API action validation | DCS-FR-CWE-28 | Authenticated create/update/query action | Action, role, state transition and rate limit are validated | Malformed body, forbidden action, invalid transition, replay/rate limit | **partial + underspecified** — active system-API candidates exist and the lifecycle API feature passed; action/rate-limit catalogue and boundary coverage remain absent |
| VAL-07 Signer identity/PoA credential validity | DCS-FR-SM-03; stakeholder input 2026-07-27 | Identity VC and mandatory PoA VC whenever a natural person signs for a company | PoA is present, currently valid, issued under the applicable trust profile and authorizes the person, action and represented company | Missing/expired/revoked/malformed/untrusted/out-of-scope/wrong-company PoA | **partial** — the mandatory-PoA rule is supplied and active happy/missing/wrong-party scenarios exist; issuer-chain scenarios remain skipped, dev/demo uses the project CA, and the production trust profile still needs confirmation |
| VAL-08 Counterparty delegation-chain verification | DCS-FR-SM-04 | Counterparty PoA chain and trust anchors | Complete, valid, traceable delegation to signer | Broken/cyclic/expired/revoked/out-of-scope delegation; untrusted anchor | **candidate_not_run** — active counterparty PoA audit scenario exists, but the trusted-root chain walk remains explicitly skipped |
| VAL-09 W3C/eIDAS credential verification | DCS-FR-SM-05 | Presented identity/PoA credentials | Format, signature, issuer/trust and semantic profile validate | Unsupported proof/data model, signature failure, issuer/status outage | **missing_trace + underspecified** — no exact feature trace; exact credential/proof profiles remain absent |
| VAL-10 Signature workflow completion validation | DCS-FR-SM-13 | Workflow definition and signer-step states | Completion only after order, deadlines and dependencies are satisfied | Missing/failed/retried/out-of-order/late steps; parallel signer boundary | **candidate_not_run** — exact real-signing trace and ceremony-deadline implementation now exist; the captured suite did not reach this feature |
| VAL-11 Cryptographically validated retrieval for signing | DCS-FR-SM-15 | Retrieved contract, expected identity/hash, signer authorization | Correct immutable contract delivered and retrieval logged | Hash/identity mismatch, altered/stale document, unauthorized signer, logging failure | **partial** — the exact signature-validation feature executed 8 active scenarios successfully with 1 skipped DSS scenario; the full immutable-retrieval matrix is not established |
| VAL-12 Applied-signature validation | DCS-FR-SM-18; stakeholder input and accepted policy contract 2026-07-27 | Signed contract, credential status, raw DSS report, normalized versioned policy input and timestamp | Preserve the raw report; evaluate the versioned per-DCS Rego policy; record normalized input, output, version and hash; only `passed` plus `allow` continues | Tampered document/signature, revoked/unknown credential, invalid/untrusted/expired timestamp, DSS/TSA outage, missing/invalid policy, malformed report and unmapped DSS result | **partial** — policy contract, safe mapping boundaries and fail-closed behavior are decided; implementation, the initial explicit DSS allowlist and complete mapping tests remain |
| VAL-13 Signature revocation and re-signing | DCS-FR-SM-20; stakeholder input 2026-07-27 | Credential/organizational revocation event or compliance alert followed by an optional new signature | Original contract and signature history remain immutable; revocation/compliance alert is visible in the UI; re-signing appends another signature to the existing contract and synchronizes it to peer instances | Duplicate/out-of-order event, alert propagation, append failure, concurrent signature, partial peer synchronization | **observed_failure** — intended outcome is supplied; one transition scenario passed and one failed during the pre-fix IPFS outage, while append-and-peer-sync is not yet cleanly evidenced |
| VAL-14 Signature-policy compliance | DCS-FR-SM-21; stakeholder input and accepted policy contract 2026-07-27 | Required signature type from the contract, presented signature type, credential status, signer role and PoA | QES is the default; only an explicit contract requirement may select another supported level; AES remains visible, but fails and blocks when QES is required; QSeal is out of scope | AES where QES is required, missing/unsupported contract override, role/PoA mismatch, stale status, unsupported QSeal | **partial** — the rule is decided and AES-vs-QES hardening exists; the normalized policy integration and end-to-end mapping evidence remain |
| VAL-15 Deterministic MR-to-HR rendering | DCS-FR-CSA-06, UC-03-05; stakeholder input 2026-07-27 | Authoritative machine-readable contract and `dcs:documentStructure` | Human-readable PDF is deterministically recompiled from the machine-readable contract; no independent HR-to-MR ingestion path exists because accepted documents pass through the template authoring lifecycle | Non-deterministic output, missing/reordered rendered section, wrong source/version, modified PDF not matching a deterministic rebuild | **observed_failure** — direction and authority are resolved; the current feature recorded 9 passing cases and 2 errors, so clean deterministic-rebuild evidence is still required |
| VAL-16 Generic pre-archive legal-framework compliance | DCS-FR-CSA-07; scope decision 2026-07-27 | Original generic eIDAS/GDPR/ISO/internal-policy check | No independent generic legal-framework oracle is implemented or claimed | Reports must not present a Semantic Hub rule result as blanket legal compliance | **not_applicable_by_decision** — the unbounded generic requirement is withdrawn; concrete Semantic Hub rule enforcement is traced by VAL-18 |
| VAL-17 Archived-contract compliance | DCS-FR-CSA-19 | Archived document, retention/signature/metadata policy | Non-compliant archive entry is flagged | Expired retention, invalid signature, incomplete metadata, immutable evidence unavailable | **candidate_not_run + underspecified** — exact archive/audit feature exists but all 20 scenarios were excluded by the normal suite tag expression; policy profiles remain absent |
| VAL-18 Workflow Semantic Hub/PDP rule enforcement | DCS-FR-TR-03, DCS-FR-PACM-02/-03; ADR-6/-8/-9/-11 | Contract at submission, offer, approval, signature and deployment plus its immutable, artifact-pinned Semantic Hub shapes/libraries/profile and one configured executor decision | Local blocking findings prevent dispatch; combined precedence is `BLOCKED > REVIEW > PASSED`; REVIEW persists one immutable Compliance Officer decision and resumes without executor redispatch; only PASSED continues automatically | Activation/rollback and old pins; missing/unknown/unavailable Hub assets; timeout/non-2xx/malformed/mismatched/invalid executor result; empty valid result; same-snapshot concurrency and exactly-one dispatch | **verified for the concrete Ticket-27 gate scope** — `features/27_external_checkpoint_and_workflow_gates/semantic_workflow_gates.feature:4-118`; independent final Ticket-27 run 27/27 scenarios and 188/188 steps green, final semantic re-verification 22/22 scenarios and 162/162 steps green. Generic legal-framework interpretation and the separately listed deferred policy choices are not claimed |
| VAL-19 Multi-contract structural integrity | DCS-FR-PACM-06; stakeholder input 2026-07-27 | Locally IRI-reachable parent-linked contracts and annexes | Bundle export walks the local graph and emits each reachable contract once; cycles are permitted and flattened by a visited set; a deleted/missing linked artifact is a hard failure | Missing/deleted/orphan/wrong-version component; cycle termination and de-duplication; link to a contract not imported into the local DCS | **candidate_not_run + integration_gap** — hierarchy/bundle feature exists; cross-DCS linkage is not a remote dereference contract and requires prior import/transfer, which is not yet a complete workflow |
| VAL-20 Template correctness/semantics/authenticity | UC-02-07 | Template, JSON-LD context, SHACL/schema, signature/VC provenance | Report lists schema and authenticity checks; failures block generation | Invalid JSON-LD/SHACL, missing context, signature/VC invalid, mixed pass/fail | **missing_trace** — related template integrity/SHACL candidates exist, but no exact UC trace or combined schema-plus-authenticity evidence was identified |
| VAL-21 Counterparty signature use case | UC-04-03; stakeholder input 2026-07-27 | PDF, expected document hash, VC/status response in W3C or supported XFSC compatibility format | Report shows PDF integrity, hash match and a freshly renewed status check; official W3C support is the target while the XFSC format remains a compatibility path | Encoding mismatch, stale response, unknown entry, outage, incompatible bit order | **candidate_not_run + compatibility_gap** — exact OID4VP/real-signing traces exist; W3C plus XFSC dual-format behavior and the no-false-positive boundary need explicit tests |
| VAL-22 Pre-execution contract validation | UC-10-02; DCS-FR-PACM-03; ADR-8/-9/-11 | Deployable contract plus its immutable Semantic Hub bundle and configured executor | The shared deployment gate persists the correlated run; blocked results prevent deployment and review results require an immutable Compliance Officer decision | Missing/unavailable pinned rules, executor failure, warning/block boundary, concurrent same-snapshot requests and review continuation | **verified for the Ticket-27 semantic workflow-gate scope** — deployment appears in the success and manual-review gate matrices at `features/27_external_checkpoint_and_workflow_gates/semantic_workflow_gates.feature:16-22` and `features/27_external_checkpoint_and_workflow_gates/semantic_workflow_gates.feature:47-53`; aggregate legal-framework claims remain excluded |
| VAL-23 System-driven API review | UC-12-02 | Contract review API request and rule set | Issues returned; failing contract cannot proceed to approval | Empty/malformed request, unauthorized caller, evaluator failure, concurrent transition | **partial** — exact lifecycle-via-API feature passed 3 scenarios; evaluator failure, concurrency and persisted review report remain unproven |
| VAL-24 OIDC token validation | DCS-IR-SI-07, DCS-IR-CI-05; stakeholder input 2026-07-27 | Hydra discovery metadata, JWKS and token/client assertions | Issuer, signature, audience/client, expiry and required claims validate against the deployed Hydra provider | Key rotation, unknown `kid`, stale cache, bad issuer/audience/clock, discovery outage | **partial** — provider choice is resolved and the authentication feature passed 7 scenarios; rotation, cache and discovery-outage boundaries remain |
| VAL-25 Credential/status validation interface | DCS-IR-SI-09, DCS-IR-CI-09; stakeholder input 2026-07-27 | Credential/contract identifier and official W3C status list or supported XFSC compatibility format | Current state is refreshed for enforcement and publication becomes visible within the binding <= 5-minute window | Encoding incompatibility, stale response, unknown entry, outage, suspended/revoked transition, refresh after cache expiry | **candidate_not_run + compatibility_gap** — five-minute target and refresh requirement are supplied; exact W3C profile, XFSC mapping and clean evidence remain |
| VAL-26 DID/VC cryptographic verification interface | DCS-IR-SI-12 | DID document or VC/VP plus trust/resolution inputs | DID/VC/VP proof validates for wallet integration | Resolution failure, unsupported proof, revoked/rotated key, domain/challenge mismatch | **observed_failure** — peer-trust and PKI traces exist; the current peer feature recorded 9 passes, 5 failures and 1 error, while PKI was not reached |
| VAL-27 C2PA compound verifier | DCS-OR-C2PA-006 | PDF plus embedded/remote C2PA, VC and live status | Four independently named checks and correct lifecycle banner | One failed/unavailable check must not be masked by others | **partial** — focused clean run passed all 9 active scenarios; `replaced` remains skipped and the four checks are not yet proven across every failure/unavailable combination |
| VAL-28 C2PA/PDF update compatibility | DCS-OR-C2PA-002/-010; stakeholder input 2026-07-27 | PAdES Baseline B-T candidate profile and one or more C2PA/signature increments | Existing PDF signatures remain valid after every append; a re-signing adds a new incremental signature rather than replacing history | Update/sign ordering, repeat update, corrupted xref/increment, peer synchronization, every formally supported signature profile | **observed_failure + confirmation_required** — “Baseline B-T” is plausible but must be confirmed by exact ETSI profile/version; provenance feature had 5 pre-fix timeout errors and needs a clean rerun |
| VAL-29 C2PA remote fallback | DCS-OR-C2PA-008 | PDF with stripped/missing embedded credential plus contract-bound remote link | Verifier obtains and validates remote manifest | Missing/wrong/tampered/unavailable remote artifact; link substitution | **partial** — public manifest/history/link scenarios passed in the focused run, but the actual strip-then-remote-verify scenario remains skipped |
| VAL-30 C2PA VC/status binding | DCS-OR-C2PA-004/-005 | Lifecycle assertion, status VC and published list | All binding fields agree and status publication meets <= 5 minutes | Field mismatch, revoked/untrusted VC, stale list, time boundary | **missing_trace** — no exact C2PA-004 or C2PA-005 feature trace was found |
| VAL-31 External audit-checkpoint publication | DCS-FR-PACM-02; ADR-16 | Every unconfirmed public Merkle checkpoint, in sequence, to the configured authenticated HTTPS sink | Only the public allowlist is sent; stable idempotency survives lost responses/restart; confirmation advances only after 2xx; a chain gap durably blocks later publication | Sequence gap, previous-root mismatch, lost response, restart, authentication/path/timeout and payload leakage | **verified for Ticket 27** — `features/27_external_checkpoint_and_workflow_gates/external_checkpoint_sink.feature:4-43`; included in the independent 27/27-scenario, 188/188-step final run. The separate external and internal timeout controls are defined at `deployment/helm/charts/orce/values.yaml:30-43` |

## C. Validation surfaces that must not become separate truth sources

These UI/API requirements expose validation paths but do not define independent validation semantics.
They must delegate to the same authoritative rule implementations and return their real findings.

| Surface requirement | Must expose / delegate to |
|---|---|
| DCS-IR-TR-03/-04/-06 | VAL-01, VAL-02, VAL-03 and VAL-20; verified state gates approval |
| DCS-IR-CWE-05 | VAL-05 and, where applicable, VAL-15/VAL-18 |
| DCS-IR-SM-02/-04 | VAL-11 and VAL-12 |
| DCS-IR-SM-05/-07 | VAL-14 plus trust-anchor, cryptographic-proof and timestamp detail from VAL-12 |
| DCS-IR-SM-08 | Export the actual VAL-12/VAL-14 findings, including unavailable/limited checks |
| DCS-IR-CSA-04 | Archive-operation and integrity evidence from VAL-15 through VAL-17 |
| DCS-IR-PACM-01 through -04 | Initiate, monitor, report and link VAL-01/VAL-18/VAL-22 findings without redefining rules |

## D. Cross-cutting evidence rules for the later verification sweep

Each validation path needs all of the following before it can be classified as satisfied:

1. A stable trigger or API/UI entry point and an identified authorization rule.
2. A named, versioned source for schemas, policies, trust anchors, algorithms, status formats and
   lifecycle mappings; no component-local hard-coded substitute.
3. Independent results for every check. `unavailable`, `limited`, `blocked` and `not_applicable`
   must not be collapsed into `passed`.
4. A deterministic positive fixture and at least one negative fixture per listed failure class.
5. Boundary cases for time, lifecycle state, concurrency, stale caches/snapshots and external
   dependency failure where applicable.
6. Enforcement evidence: a finding is insufficient where the requirement says block, reject,
   invalidate, preserve persistence, or prevent archival/deployment/approval.
7. Durable, correlated evidence containing requirement/path ID, input identity/version, rule-set
   version, timestamp, actor, findings and outcome, without leaking secrets or excess personal data.
8. An independently repeatable command or scenario. Comments, tags and status fields are only
   traceability hints.

## E. Specification gaps and unresolved decisions

- The SRS text for `DCS-FR-SM-03` is complete; verification still needs recognized-authority and
  trust-anchor profiles before the path can be considered fully specified.
- All 32 NFR measurement vectors lack complete targets/test profiles. They cannot currently yield
  an objective pass/fail result without an approved interpretation.
- C2PA-001 through -010 have measurable SRS targets and remain in scope unless an approved,
  traceable scope or deviation decision says otherwise.
- The former generic eIDAS/GDPR/ISO/internal-policy validation obligation is removed by the
  `2026-07-27` scope decision. Concrete versioned Semantic Hub SHACL shapes and validation profiles
  are the only external rule source; DCS makes no blanket legal-compliance claim.
- Semantic Hub and configured executor findings guard submission, offer, approval, signature and
  deployment through the shared snapshot coordinator. Blocking results dominate review results;
  review dominates pass. This concrete gate set is verified by Ticket 27 and does not reinstate the
  removed generic legal-compliance obligation
  (`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:304-333`).
- UC-04-03 explicitly limits full status-list validation because of an incompatible external
  encoding. Any report that calls retrieval alone a successful status validation is a false positive.
- Linked paths such as MR/HR consistency must be assessed per atomic obligation; an aggregate
  planning or implementation-status label is not evidence.

### E.1 Stakeholder answers received on 2026-07-27

These answers narrow the open questions but are not all equally final. Statements containing an
explicit uncertainty or an internal conflict are marked `confirmation_required` rather than silently
promoted to an approved requirement.

| Topic | Current answer | Classification / remaining question |
|---|---|---|
| PoA | Every natural person acting for a company must present a valid PoA | Decision supplied; production issuer/chain/revocation trust profile remains |
| Dev/demo trust | Project-owned certificate authority | Decision supplied for dev/demo only |
| Signature type | QES is the default; only an explicit contract requirement may select another supported level; AES fails and blocks when QES is required while its cryptographic integrity result remains separately visible; QSeal is not planned | Decision supplied; implementation/evidence remains |
| PDF signature profile | PAdES Baseline B-T | `confirmation_required`: exact ETSI standard/profile/version |
| DSS outage | Hard failure | Decision supplied |
| DSS result mapping | Per-DCS Rego policy supplied through a ConfigMap, with a basic fail-closed policy deployed by default; normalized versioned input and structured `category`/`decision`/`reason_code`/`message` output | Decision supplied; implement the policy runner, persist raw and derived evidence, define the initial understood-result allowlist and test every mapping/fallback |
| TSA trust | EU Trusted List-qualified services or explicitly configured certificate chains | Direction supplied; exact LOTL/Trusted List path validation and configured-anchor policy remain |
| Credential revocation | Preserve contract/history; surface compliance state in UI, optionally alert | Decision supplied; exact alert severity and downstream blocking behavior remain |
| Re-signing | Append another signature to the existing contract and synchronize to peers | Decision supplied; concurrency, ordering and partial-sync recovery remain |
| Extensible policies | Semantic Hub remains the DCS technical semantic-rule source; deployment-specific/custom PAC findings are produced through `facis-pac-audit-executor/v1`, with shipped ORCE `/audit/run` as this environment's reference and a configurable compatible replacement | Implemented and independently verified for the executor boundary; the configured rule set determines what is actually checked, so this is not a blanket compliance claim |
| MR/HR | MR is authoritative; HR PDF is deterministically compiled from MR using `dcs:documentStructure`; there is no independent HR-to-MR path | Decision supplied |
| Contract hierarchy | Parent linkage; cycles allowed and flattened with a visited set during bundle export | Decision supplied |
| Cross-DCS bundle references | Bundle traversal is local IRI reachability; a needed foreign contract must first be transferred/imported into the local DCS | Integration gap: a general existing-contract import/transfer workflow is not complete |
| Deleted linked artifact | Hard failure | Decision supplied |
| C2PA coverage | Every contract PDF carries C2PA | Decision supplied |
| C2PA lifecycle | Tentative answer: only negotiation and renewal/cross-DCS changes, not all internal states | `confirmation_required`: conflicts with the explicit seven-state target in DCS-OR-C2PA-003 |
| C2PA stripped-metadata fallback | Requirement should remain and be covered | Direction supplied; implementation/test remains skipped |
| Status lists | Official W3C format is the target; XFSC service format is supported as a compatibility path; <= 5 minutes is binding and checks must refresh | Decision supplied; exact W3C profile and lossless XFSC mapping remain |
| OIDC provider | Hydra | Decision supplied |
| Wallets | Paradym wallet for custom credentials; EUDI Wallet reference implementation for document signing | Decision supplied; exact supported versions/conformance profiles remain |
| DCS-to-DCS compatibility | Current development baseline, kept aligned with the latest development state | Provisional development policy; no stable protocol-version or backward-compatibility promise is defined |

## F. Refreshed trace and execution observations

### F.1 Trace changes since the previous draft

- Active PoA-at-signing scenarios now cover successful authorization, missing PoA, wrong-party PoA
  and a tampered counterparty PoA in
  `features/22_real_signing_vertical/poa_at_signing.feature`. The separate issuer-chain walk to a
  trusted root in `features/14_credential_acquisition/poa_credential_verification.feature` remains
  skipped. PoA is therefore no longer wholly unimplemented or wholly skipped, but it is not complete.
- Active signing-hardening scenarios now cover revoked PID, trusted and untrusted x5c issuers,
  invalid JAdES, document-byte mismatch, AES where QES is required, replay and ceremony deadlines in
  `features/22_real_signing_vertical/signing_acceptance_hardening.feature` and related real-signing
  features. These scenarios were added after the previous committed sweep and were not reached by
  the interrupted full run.
- `features/23_semantic_hub/semantic_hub.feature` traces version registration, activation, rollback,
  artifact anchoring, prefix-conflict rejection and the rule that newly produced contracts use the
  active shapes while existing contracts remain pinned. The interrupted full run did not reach this
  feature, so it is a current candidate rather than executed evidence in this refresh.
- Exact C2PA feature tags still exist for C2PA-001, -002, -003, -006, -007, -008 and -010. No exact
  feature trace was found for C2PA-004, C2PA-005 or C2PA-009.
- The focused C2PA conformance run proves that its nine active scenarios currently pass. It does not
  prove C2PA-008's strip-and-remote-fallback obligation because that scenario remains skipped, and it
  does not prove the `replaced` banner because that scenario also remains skipped.
- The signature-validation feature executed eight active scenarios successfully, but its EU DSS
  scenario remains tagged `@skip`. A configured DSS pod being present in the BDD deployment does not
  turn a skipped scenario into evidence.
- `features/15_access_revocation/signature_revocation_state.feature` is active. The JUnit directory
  also contains an older fully skipped report for `signature_revocation.feature`, but that feature no
  longer exists in the repository and the report is stale. The current state feature's restore
  scenario failed during the now-fixed IPFS Document Manager outage and needs a clean rerun before
  product behavior can be classified.
- Direct NFR feature tags remain sparse relative to the 32 vectors, and none supplies the missing
  numeric target, sample profile or observation window.

### F.2 Execution evidence captured during the refresh

| Command / artifact | Result | Evidence use |
|---|---|---|
| `make -C tests/bdd run_bdd_kind_once` | Terminated by the outer command timeout after 30 minutes while running feature 19 | Useful for per-feature observations only; not a clean-suite result |
| `features/02_template_management/generate_contract.feature` JUnit (`2026-07-26`) | 4 passed | Candidate evidence for VAL-01 |
| `features/02_template_management/template_integrity_audit.feature` JUnit (`2026-07-26`) | 3 passed | Candidate evidence for VAL-02 |
| `features/04_contract_signing/signature_validation.feature` JUnit (`2026-07-26`) | 8 passed, 1 skipped | Candidate evidence for VAL-11/12/14; incomplete |
| `features/12_system_based_contract_management/contract_lifecycle_via_api.feature` JUnit (`2026-07-26`) | 3 passed | Candidate evidence for VAL-06/23 |
| `features/18_odrl_soundness/odrl_soundness.feature` JUnit (`2026-07-26`) | 22 passed | Candidate evidence for the ODRL subset of VAL-18 |
| `features/17_peer_trust/two_instance_peer_trust.feature` JUnit (`2026-07-26`) | 9 passed, 5 failed, 1 error | Current failing evidence for VAL-26; requires diagnosis/rerun |
| `features/08_audit_compliance/c2pa_provenance_export.feature` JUnit (`2026-07-26`) | 5 timeout errors before the infrastructure fix | Inconclusive for VAL-28; clean rerun required |
| `make -C tests/bdd run_bdd_kind_once F=features/19_c2pa_conformance/c2pa_conformance.feature` | 9 passed, 2 skipped after the infrastructure fix | Clean partial evidence for VAL-27/29 |
| Ticket-27 independent final run (`2026-07-29`) | 27 scenarios passed, 188 steps passed; backend full/race, ORCE flow/Helm, JUnit and fingerprint checks reported green | Final evidence for VAL-18, VAL-22 and VAL-31 |
| Ticket-27 final Semantic re-verification (`2026-07-29`) | 22 scenarios passed, 162 steps passed | Focused final evidence for deterministic bundles, all five gates, fail-closed behavior, review and concurrency |

The JUnit directory must be cleaned or made run-scoped before the next evidence run. Its current
append-only behavior mixes timestamps and can make stale XML files appear to belong to the latest
command.

## G. Remaining work in execution order

1. **Confirm the remaining normative details.** Define the 32 NFR targets, production PoA/credential
   trust profile, exact PAdES Baseline profile/version, TSA trust-validation policy, C2PA
   lifecycle-event scope and exact W3C status-list profile. The QES/default-contract rule and DSS
   policy contract are decided; their implementation and exhaustive mapping tests remain.
   The generic regulatory-rule question is closed: semantic rules are versioned Hub artifacts, while
   custom deployment policy runs through the verified external audit-executor
   boundary rather than a DCS legal oracle.
2. **Repair evidence hygiene.** Produce a unique report directory per command, record commit,
   configuration and cluster identity, and avoid mixing old XML with current results.
3. **Rerun current inconclusive paths on the fixed cluster.** At minimum rerun VAL-13, VAL-15,
   VAL-26 and VAL-28 candidates, then retain their logs and reports.
4. **Execute newly added but uncaptured features.** Run hierarchy/bundle, PoA-at-signing,
   signing-hardening, OID4VP retrieval, real-signing and PKI consolidation independently.
5. **Implement or trace the missing paths.** VAL-09, VAL-20 and VAL-30 currently lack exact,
   complete traceability. VAL-16 is intentionally retained only as a record of the removed generic
   requirement and requires no implementation.
6. **Unskip only by implementation or approved decision.** Resolve the PoA trusted-root chain walk,
   EU DSS validation, C2PA strip-and-remote fallback and `replaced` lifecycle banner, or record an
   approved scope/deviation decision.
7. **Perform the atomic verification pass.** For each VAL row, link production code, configuration,
   positive/negative/boundary fixtures, enforcement evidence and one independently repeatable clean
   command before assigning `verified`.
