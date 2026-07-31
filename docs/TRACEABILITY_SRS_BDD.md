# SRS ⇄ BDD Traceability Matrix

**Sources.** Requirements: `docs/SRS_FACIS_DCS.pdf` (all 225 bracket-defined `[DCS-…]` IDs; the
`[DCS-FR-UC-XX-Y]` sections are the SRS's own use-case→requirement linkage and are treated as
mapping metadata, not requirements). Coverage: `features/**/*.feature` (behave suite run on every
CI push, kind-in-docker).

**2026-07-22 wave note.** Two DCS instances now negotiate a contract end to end
(`full-vertical-2dcs.spec.ts`: offer → negotiation ping-pong with a PDF exchanged per
adjustment → settle → dual signing → scoped audit, every workflow action driven through the
real UI). Federation follows the prefer-inbound rule — the peer's PDF is authoritative and
stored verbatim because it carries provenance we cannot reproduce, regeneration is
verify-only — and inbound counter-offers are authorised by counterparty rather than by local
RBAC. The audit trail's global tamper evidence moved from a per-event chain to Merkle
checkpoints (ADR-16), the contract format separates typed `dcs:ContractField` declarations
(ContractField model) in expanded JSON-LD (ADR-14), and machine callers became SRS System Users with one
OAuth2 client per class. Two dispositions in this wave are refusals rather than features:
`Sys. Contract Signer` holds no signing scope (ADR-17, eIDAS Art. 3(9)/26 — a signatory is a
natural person under sole control, and a legal person's instrument is a seal we do not
implement), and external anchoring of the checkpoint chain is wired but not in force, since
the ORCE flow polling the head ends at a debug node. New pack this wave:
12/system_user_classes.

**Claim correction (2026-07-22).** An assessment of `testWallet` against the ARF and the EUDI
reference libraries, fetching both verbatim, found the ARF citations used elsewhere in this
repo were wrong: Topic 16 is QES-scoped throughout and has no AES track, `QES_05` is
remote-QES enrolment rather than the flow list (that is `QES_06`), and CSC API 2.0 was
`QES_07`, now `Empty`. Only "QES_08 = PAdES" holds. Since the SRS descopes QES, **ARF Topic 16
conformance is not a claim this project may make**, and neither is sole control/SCAL2 —
testWallet's keys are shared files by design. The same assessment found four verified
divergences from the reference wallet that testWallet currently accommodates, so a green CI
run is not evidence of wallet interoperability.

**2026-07-17 wave note.** First fully green combined CI run: behave (312 scenarios) and the
Playwright suite (11 specs) pass in the same kind-stack job (run 29541989685 @ 27c0a1be).
Since the 2026-07-16 note the E2E layer grew the Semantic Hub management specs
(e2e/semantic-hub-publish.spec.ts) and surfaced a fifth UI defect (shacl-form fragments one
subject across several @id-sharing node objects; merged before emission) plus one product
concurrency bug: the async PDF/C2PA pipeline's cache write bumped contracts.updated_at and
tripped optimistic concurrency for racing editors — fixed so updated_at moves only on
non-cache column changes (migration 20260717, caught by the C2PA chain scenario).

**2026-07-16 wave note.** A Playwright E2E layer now covers the browser UI
(frontend/ClientApp/e2e, run in CI after behave against the same kind stack): real OID4VP
sessions per test, API-seeded fixtures, 9/9 specs green. It surfaced and fixed four real UI
defects (Pinia boot-order crash; the odrl:Set-era isOdrlSet predicate that blocked rendering
of every canonical contract; a crash on domainField-less ContractFields; shacl-form's
rdf:type-vs-@type serialization that kept ODRL typed clauses from ever becoming rules through
the browser). Rows below marked "e2e/" cite that suite. Same wave: contracts/templates carry
dereferenceable resource IRIs (@id = {DCS_PUBLIC_URL}/contract/{key}, resolve routes added),
policy sets follow the Offer→Agreement lifecycle sealed at first signature, and the CI run at
920cf253 reduced open BDD failures to two, both fixed (renewal IRI step; zero-finding
contracts now appear in /pac/audit as explicitly compliant).

**2026-07-14 wave note.** The rows citing packs 22/multi_signer and 23/semantic_hub, the
archive annotation/full-text scenarios (07), the JAdES provenance scenarios (17), the
target-acknowledgement/KPI scenarios (05), and the signature view/compliance scenarios (04)
were implemented in this wave: all scenarios bind (behave dry-run) and every backend unit
test is green, but the wave has not yet had its first full CI run — statuses here reflect
the executable evidence as written.

**Method.** Every requirement gets exactly one disposition. Scenario references name the
feature pack (by its `features/` directory number) plus a short scenario descriptor; most
scenarios also carry the requirement ID as a behave tag (`@DCS-…`), so `grep -r <ID> features/`
finds the executable evidence.

**Harness scope (applies to all 📋 rows).** The suite is a black-box HTTP harness against the
deployed service. Browser-UI behavior, TLS termination, platform hardening, and
process/documentation requirements are verified by review/ops, not BDD — the SRS itself lists
"Review of Documentation" as the verification method for most of them. This is a documented
decision (see the @skip Signature-Manager-UI scenario in `features/22_real_signing_vertical`),
not a coverage hole.

| Status | Meaning | Count |
|---|---|---|
| ✅ Covered | scenario(s) or Playwright specs assert the requirement end-to-end | 156 |
| 🔧 In progress | being implemented | 0 |
| 🟡 Partial | core behavior asserted; named residue not (yet) provable | 42 |
| 📋 Not BDD-verifiable | infrastructure/process requirement — verified outside the automated harnesses | 24 |
| ❌ Deviation | capability not implemented in the product; recorded deviation | 3 |
| | **Total** | **225** |

## 3.2.1 Template Repository (DCS-FR-TR-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-FR-TR-01 | Machine-Readable Format | ✅ Covered | Templates stored/retrieved as JSON-LD — 02/create_template, 02/template_identity; editor state is the JSON-LD doc. |
| DCS-FR-TR-02 | Multi-Tiered Contract Template Management | ✅ Covered | 20/hierarchy invariant scenarios (tagged @DCS-FR-TR-02): parent refs, child-enumeration rejection, cycle rejection. |
| DCS-FR-TR-03 | Semantic Hub for Schema Storage | ✅ Covered | Semantic Hub built (23/semantic_hub): versioned JSON-LD context + SHACL shape storage seeded with the FACIS v1 profile, public resolution, Template-Manager register/rollback (UC-02-08), every produced document anchored via resolvable standard-vocabulary anchors (@context hub URL, sh:shapesGraph, dcterms:conformsTo), and hub-prefix redefinition rejected at template creation. Hub management UI asserted: e2e/semantic-hub.spec.ts (dashboard lists every registered artifact from the live inventory; clause catalog served with labeled, shape-backed entries) and e2e/semantic-hub-publish.spec.ts (an operator publishes a brand-new shapes entry through the UI and it resolves immediately on the public route — the Gaia-X case; register + activate a new version of an existing entry). ADR-8: enforcement (`AuditContractContent`) reads its SHACL shapes/validation profile from the hub's active (or, for revalidation, pinned-per-document) version, hub-only (no disk fallback) — 23/semantic_hub "Activating a stricter SHACL shapes version..." proves activate/rollback actually changes what gets enforced, and that already-produced contracts stay pinned. ADR-9: the enforcement engine is goRDFlib, a conformant SHACL-core processor verified against the W3C SHACL/SHACL-1.2 suites (388/388, pinned commit recorded in the ADR) — real `sh:datatype`/`sh:minInclusive`/`sh:pattern`/`sh:node`/`sh:nodeKind` constraints, not a hand-rolled subset matcher; `internal/base/validation/contractcontentaudit_test.go` `TestAuditContractContentSHACLRejectsWrongDatatype` is the unit-level xsd:integer-rejection proof. |
| DCS-FR-TR-04 | Machine-Readable and Human-Readable Template Linking | ✅ Covered | MR→HR derivation proven via template PDF export + verify (02/template_integrity_audit); bidirectional *link* metadata not modeled beyond same-DID pairing. Phase 3 (ADR-10) partially addresses the machine-readable half for clauses specifically: typed clause instances (dcs:PaymentClause etc.) are generated from and validated against the same Semantic Hub SHACL shapes (GET /semantic/clauses, 23/semantic_hub "The clause catalog is seeded..."), so a clause's authored form and its enforcement share one source of truth — TestAuditContractContentValidatesTypedClauses proves server-side enforcement; the frontend palette (TypedClausePalette.vue) is manual/UI-review evidence, consistent with the existing DCS-IR-TR partial-row convention. Now fully covered: every machine rule must carry dcs:prose referencing its human-readable clause (Go gate + Odrl*ProseShape SHACL, 18 structure scenario), and e2e/template-typed-clause.spec.ts proves the UI end: a hub-palette typed clause becomes a prose-backed rule whose dcs:prose dereferences to a document block. |
| DCS-FR-TR-05 | Template Version Control | ✅ Covered | Template versions/approvals tracked; template audit-log scenario (02, @DCS-FR-TR-21/TR-05). retrieve_history_by_id exists. |
| DCS-FR-TR-06 | Role-Based Access Control for Template Repository | ✅ Covered | RBAC negative scenarios: 02/create, 02/update, 02/archive, 02/workflow 'Unauthorized role cannot …' + 01 pack 401 sweep. |
| DCS-FR-TR-07 | Compliance & Legal Validation | 🟡 Partial | Approval gate before usability proven (02/template_workflow + contract create requires REGISTERED). Domain-specific regulatory rule packs beyond ODRL/structural validation are not modeled. |
| DCS-FR-TR-08 | ) | 📋 Not BDD-verifiable | SRS formatting artifact: bracketed ID sits inside the §3.1.1 Template Builder UI narrative (review-task generation + unique ID). Substance covered by 02/template_workflow (submit→review task) and 02/template_identity (unique DID). |
| DCS-FR-TR-09 | Template Provenance and Versioning | ✅ Covered | Registration seals each version's provenance as a signed W3C VC (JSON-LD, ecdsa-rdfc-2019): creator/reviewer/approver/registrar claims, content hash, previous-credential linkage; served by GET /template/provenance/{did} (02/template_provenance). Provenance also travels in template bundle export (20). |
| DCS-FR-TR-10 | Searchable Metadata & Categorization | ✅ Covered | 02/search_templates: by name, description, details; RBAC negative. |
| DCS-FR-TR-11 | Template UUID / DID Assignment | ✅ Covered | 02/template_identity: UUID on creation, retrieve by DID. |
| DCS-FR-TR-12 | Template Customization | ✅ Covered | SHOULD — 02/generate_contract: contract generated from approved template with populated terms. |
| DCS-FR-TR-13 | Template Creation | ✅ Covered | 02/create_template + register step (/template/register), RBAC negative. |
| DCS-FR-TR-14 | Template Submission for Approval | ✅ Covered | 02/template_workflow 'Submit template for review'. |
| DCS-FR-TR-15 | Template Approval Process | ✅ Covered | 02/template_workflow approve/reject/resubmit set (7 scenarios). |
| DCS-FR-TR-16 | Template Update Management | ✅ Covered | 02/update_template (creator, reviewer variants) with version continuity. |
| DCS-FR-TR-17 | Template Retirement and Deprecation | ✅ Covered | 02/template_archive 'Deprecate an active template' + cannot-delete-deprecated guard. |
| DCS-FR-TR-18 | Template Deletion | ✅ Covered | 02/template_archive delete scenarios incl. RBAC negative. |
| DCS-FR-TR-19 | Template Retrieval | ✅ Covered | 02/generate_contract + template_identity retrieve-by-DID (/template/retrieve). |
| DCS-FR-TR-20 | Template Compliance and Integrity Verification | ✅ Covered | /template/verify scenario (02, tagged @DCS-FR-TR-20). |
| DCS-FR-TR-21 | Audit Logs for Template Changes | ✅ Covered | Template audit-log scenario (02, tagged @DCS-FR-TR-21). |
| DCS-FR-TR-22 | Notification System for Template Updates | ✅ Covered | Webhook platform (/orce): subscribable template.updated/template.registered events fan out to registered receivers with the template DID in the payload; delivery log with acknowledgement (GET /deliveries). Verified end-to-end against the ORCE monitoring flow (02/template_update_notifications). |
| DCS-FR-TR-23 | Structural Dependency Mapping The Template Repository MUST allow Te… | ✅ Covered | 20 hierarchy dependency enforcement + export refusal on missing component (tagged @DCS-FR-TR-26/@DCS-FR-PACM-06). |
| DCS-FR-TR-24 | Structural Export in Unified Format | ✅ Covered | 20 template bundle export (tagged @DCS-FR-TR-24). |
| DCS-FR-TR-25 | Multi-Contract Template Builder | ✅ Covered | Visual builder is a frontend concern (HTTP-only harness; see the 22 UI-gap precedent). Backing APIs covered via 20 hierarchy/bundle + 02 CRUD. UI now asserted: e2e/template-typed-clause.spec.ts drives the visual builder end-to-end (type selection, block palette, shacl-form, save) against the live stack. |
| DCS-FR-TR-26 | Logical Validation of Structural Dependencies | ✅ Covered | 20 'Export is refused with a findings list when a referenced component is missing' (tagged). |
| DCS-FR-TR-27 | Contract Type Classification | 🟡 Partial | Multi/single-party structure expressed via responsible-party DIDs and hierarchy; a dedicated contract-type classification facet for filtering is not modeled. |
| DCS-FR-TR-28 | Template Management Dashboard (see Section 3.1) | ✅ Covered | Dashboard UI; backing APIs (search/status/workflow) covered by 02 pack. UI now asserted: e2e/dashboards.spec.ts (template dashboard lists registered templates). |

## 3.2.2 Contract Workflow Engine (DCS-FR-CWE-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-FR-CWE-01 | Multi-Party Contract Management | ✅ Covered | 17/two_instance: offer with instance-B negotiator+approver, cross-instance OFFERED/APPROVED replication. |
| DCS-FR-CWE-02 | Hierarchical Contract Structures | ✅ Covered | 20 hierarchy scenarios: single-parent model, cycle rejection, frame-contract child listing. |
| DCS-FR-CWE-03 | Contract Assembling | ✅ Covered | 'Assemble contract from reusable clauses' (03/contract_creation). |
| DCS-FR-CWE-04 | Machine-Readable & Human-Readable Contract Synchronization | ✅ Covered | 03/format_review MR/HR hash scenarios + 08 verify endpoint + 22 dual-hash binding scenario (all tagged @DCS-FR-CWE-04). |
| DCS-FR-CWE-05 | Secure Human-Readable Contract Viewer | ✅ Covered | Tamper-evidence of the served HR view proven via verify + tamper seams (03/format_review). Viewer UI itself out of harness scope. Viewer UI now asserted: e2e/dashboards.spec.ts renders the human-readable document from the machine-readable JSON-LD (Contract Content tab). |
| DCS-FR-CWE-06 | Event-Driven Contract Execution | ✅ Covered | 05 auto-deployment on signing completion; 15 re-approval flow (tagged @DCS-FR-CWE-06); events logged (08). |
| DCS-FR-CWE-07 | Role-Based Access Control | ✅ Covered | Role-guard negatives across 03/05/07/08/22; credential-based roles via OIDC (01). |
| DCS-FR-CWE-08 | Version Control | ✅ Covered | Version history via /contract/retrieve_history_by_id — 'Track version history during negotiation' (03). |
| DCS-FR-CWE-09 | SLA & Compliance Monitoring | ✅ Covered | 05 KPI ingestion + SLA-violation flag scenarios (tagged @DCS-FR-CWE-09). |
| DCS-FR-CWE-10 | Contract Expiry | ✅ Covered | 19 expiry-cron scenario: 'expired' lifecycle banner after cron fires. |
| DCS-FR-CWE-11 | Contract Renewal | ✅ Covered | POST /contract/renew; renewal keeps refs to prior DID/version (06 pack). |
| DCS-FR-CWE-12 | Termination Handling | ✅ Covered | 06/contract_termination: terminate with reason, double-termination guard (tagged @DCS-FR-CWE-11/12). |
| DCS-FR-CWE-13 | Contract Creation | ✅ Covered | 03/contract_creation create-from-template, editable versioned draft. |
| DCS-FR-CWE-14 | Contract Submission for Review | ✅ Covered | 03 state-machine submit→review→approve chain; 'Submit contract for review after negotiation'. |
| DCS-FR-CWE-15 | Contract Review and Approval | ✅ Covered | 03/contract_approval approve/reject/initiate; partial-quorum enforcement proven with two DISTINCT approver peers in 17/two_instance's approval-quorum scenario: one approval leaves the contract REVIEWED, the second flips it APPROVED with both peer decisions recorded. |
| DCS-FR-CWE-16 | Contract Initiation | ✅ Covered | 'Contract transitions to signing phase upon approval' (03); sign-after-approve proven in 22. |
| DCS-FR-CWE-17 | Contract Review | 🟡 Partial | Redlining + version compare via history endpoint (03); automated missing-field checks via structural validation (20 hierarchy rejections). Side-by-side diff is a UI concern. |
| DCS-FR-CWE-18 | Contract Negotiation | ✅ Covered | 03/contract_negotiation: comments, redlines (green); decision rounds + negotiation log covered. |
| DCS-FR-CWE-19 | Contract Signing | ✅ Covered | 22 end-to-end AES signing with ceremony, status tracked (ceremony + e2e scenarios). |
| DCS-FR-CWE-20 | Store Contract in Archive | ✅ Covered | 05 archive-at-SIGNED scenario (tagged @DCS-FR-CWE-20). |
| DCS-FR-CWE-21 | Retrieve Contract from Archive | ✅ Covered | 07 archive retrieve/search with RBAC. |
| DCS-FR-CWE-22 | Contract Renewal Management | ✅ Covered | Renewal workflow endpoint (see CWE-11). |
| DCS-FR-CWE-23 | Contract Termination | ✅ Covered | 06 termination via API, removed from active flows (state TERMINATED). |
| DCS-FR-CWE-24 | Contract Management Dashboard | ✅ Covered | Dashboard UI; backing search/status APIs covered (03 state-filtered search, 07). UI now asserted: e2e/dashboards.spec.ts (contract dashboard lists contracts with lifecycle state). |
| DCS-FR-CWE-25 | Contract Review and Approval Interface | 🟡 Partial | Approval API surface covered (03/contract_approval); dedicated reviewer UI out of harness scope. |
| DCS-FR-CWE-26 | Contract Signing Interface | 🟡 Partial | Signing API + ceremony covered (22); browser signing UI documented out-of-scope (22 @skip UI scenario). |
| DCS-FR-CWE-27 | Contract Tracking and Status Overview | ✅ Covered | 03 state-machine state-filtered search; status history via retrieve_history_by_id (approval routing scenario). |
| DCS-FR-CWE-28 | Automated Contract Interaction via API | ✅ Covered | 12/contract_lifecycle_via_api: full lifecycle + queryable history via API. Machine callers now have real identities: one OAuth2 client_credentials client per SRS §2.4 Table 5 System User class, with roles fixed by deployment configuration because such a token carries no verifiable role claims (ADR-16) — 12/system_user_classes obtains real Hydra tokens and checks what each class may and may not reach. |
| DCS-FR-CWE-29 | Multi-Contract Visualization | ✅ Covered | 20 parent_did search filter + frame-contract detail (tagged @DCS-FR-CWE-29). |
| DCS-FR-CWE-30 | Contract Package Bundling | ✅ Covered | 20 bundle-export scenarios: ZIP members, parent-chain refs, manifest hashes (tagged @DCS-FR-CWE-30). |
| DCS-FR-CWE-31 | Contract Performance Tracking | ✅ Covered | 05 ACTIVE-in-live-list + KPI-on-detail scenarios (tagged @DCS-FR-CWE-31). |

## 3.2.3 Signature Management (DCS-FR-SM-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-FR-SM-01 | Level of Assurance Flexibility for Simple Electronic Signature, Adv… | 🟡 Partial | AES level proven end-to-end (22 e2e scenario); credential_type honored (apply-fields scenario). QES requires a qualified TSP/QSCD — unavailable in hermetic env; deviation note for QES execution. |
| DCS-FR-SM-02 | Support for PAdES, JAdES, and CAdES Signatures | ✅ Covered | PAdES B-T proven (22) with ETSI.CAdES.detached CMS container (CAdES); JAdES baseline-B implemented for the machine-readable contract in the DCS-to-DCS flow (17 provenance + tamper-negative scenarios; internal/base/jades unit tests). |
| DCS-FR-SM-03 | Signing Identity and PoA Authorization Credentials | 🟡 Partial | Signer identity credential (PID SD-JWT VC) verified before signing (22 ceremony-gate, webhook/PID-embedding, and verify cross-check scenarios). PoA (dc+sd-jwt, vct urn:dcs:poa:v1) is presented at LOGIN and mapped into the Hydra session — every authenticated call is PoA-gated; issuer chain-walk to trust anchor stays open (recorded deviation; SRS TBD-B acknowledges XFSC PCM unavailability). A machine caller cannot sign at all: eIDAS Art. 3(9) makes a signatory a natural person and Art. 26 requires sole control, so the System Contract Signer class holds no signing scope (ADR-17) — 12/system_user_classes asserts the refusal at ceremony start, prepare and submit, and that the same client may still verify. |
| DCS-FR-SM-04 | Counterparty Authorization and PoA Credential Chain Verification | 🟡 Partial | Credential status/revocation is checked on every verification (status-list check in each verify path) — a revoked PoA blocks the login that gates signing. Chain-walk to a trust anchor remains roadmap (recorded deviation). |
| DCS-FR-SM-05 | Integration with Signing Identity and PoA Verifiable Credentials | ✅ Covered | W3C-compliant SD-JWT VC + KB-JWT presented, verified, embedded verbatim under the PAdES signature (22 verbatim-embedding + verify cross-check scenarios). |
| DCS-FR-SM-06 | Wallet for Identity, PoA Credential Management, and Signing | 🟡 Partial | Wallet protocol surface (OID4VP presentation, headless) proven (22 webhook + headless-ceremony scenarios); a real end-user wallet app is outside the harness. |
| DCS-FR-SM-07 | Multi-Signature and Role-Based Signing Flows | ✅ Covered | 22/multi_signer: one ceremony + one sequential PAdES signature per declared field, all-ceremonies-before-first-signature evidence embedding, ceremony-gate and double-signing negatives, deploy gate until every field is signed; role gating via 22 ceremony role-denial. |
| DCS-FR-SM-08 | Persisted Contract Signing Summary with Verifiable Credential and P… | ✅ Covered | 22 ContractSigningSummaryCredential issued + embedded; PDF/A-3 attachment under signature (tagged @DCS-FR-SM-08). Phase 4 (ADR-9): the credential now also carries schema_version/validation_report_hash — the Semantic Hub SHACL version the contract validated against at signing time and a stable hash of the findings (validation.SHACLEvidence) — and signature/validate re-runs pinned-version validation and cross-checks the hash for drift (crossCheckSHACLDrift, backend/internal/signingmanagement/query/validate.go), unit-tested via TestSHACLEvidenceIsStableAndDetectsDrift. |
| DCS-FR-SM-09 | Secure Human-Readable Contract Viewer | ✅ Covered | Same as CWE-05: tamper-evidence of served content proven; viewer UI out of harness. Viewer UI now asserted alongside CWE-05: e2e/dashboards.spec.ts. |
| DCS-FR-SM-10 | Proof of Contract Execution | ✅ Covered | 05 TSA-timestamped execution receipt appended to archive (tagged @DCS-FR-SM-10). |
| DCS-FR-SM-11 | Linked Machine-Readable and Human-Readable Signatures | ✅ Covered | 22 signature record binds PDF hash + JSON-LD content hash. |
| DCS-FR-SM-12 | Contract Deployment Trigger | ✅ Covered | 05 deploy-trigger scenarios incl. auto-trigger on signing (tagged @DCS-FR-SM-12). |
| DCS-FR-SM-13 | Signature Workflow Process | ✅ Covered | 22 ceremony orchestration and lifecycle statuses. |
| DCS-FR-SM-14 | Signature Request from Signer | ✅ Covered | 22 POST /signature/request + status polling (tagged @FR-SM-14). |
| DCS-FR-SM-15 | Contract Retrieval for Signing | ✅ Covered | Signed-PDF retrieval with validation exercised throughout 22 (IPFS-CID persisted artifact). |
| DCS-FR-SM-16 | Apply Digital Signature (via Cloud PCM or OCM Signer API Endpoint) | ✅ Covered | 22 real PAdES via HSM path (tagged @DCS-FR-SM-16). |
| DCS-FR-SM-17 | Multi-Signer Support | ✅ Covered | 22/multi_signer end-to-end: two DISTINCT signer identities recorded independently per field (signature view assertion), sequential application on signed bytes (mechanics also unit-proven by pdf-core TestPAdESSecondSignatureProbe); parallel signing stays a documented change request. |
| DCS-FR-SM-18 | Signature Validation | ✅ Covered | Signature validate endpoint scenario (04/signature_validation, tagged @DCS-FR-SM-18). |
| DCS-FR-SM-19 | Audit Log for Signatures | ✅ Covered | Signature audit-log scenario (04, tagged @DCS-FR-SM-19). |
| DCS-FR-SM-20 | Signature Revocation | ✅ Covered | 15 revocation → REVOKED + re-approval path (tagged @DCS-FR-SM-20). |
| DCS-FR-SM-21 | Signature Compliance Verification | ✅ Covered | Signature compliance endpoint scenario (04, tagged @DCS-FR-SM-21). |
| DCS-FR-SM-22 | Signature Dashboard for Contract Signers | ✅ Covered | Signer dashboard UI; backing status API covered (22 status polling). UI now asserted: e2e/dashboards.spec.ts (signing dashboard renders for the signer role). |
| DCS-FR-SM-23 | Signing Interface | 📋 Not BDD-verifiable | Browser signing UI + biometrics: documented out of harness (22 @skip UI scenario records the decision). |
| DCS-FR-SM-24 | Signature Status Tracking | ✅ Covered | 22 ceremony-status-progression scenario. |
| DCS-FR-SM-25 | Automated Signature Processing API | ✅ Covered | 22 fully headless API-driven ceremony (tagged @FR-SM-25). |
| DCS-FR-SM-26 | Signature Compliance Viewer | 🟡 Partial | GET /signature/view serves the viewer's full data set — per-signature signer identity, field, credential class, status, timestamps, container format + integrity findings (04 view scenarios incl. RBAC negative); the Vue viewer itself stays out of the HTTP harness. |
| DCS-FR-SM-27 | Support for PDF/A Format | ✅ Covered | 04/signature_validation asserts PDF/A-3 identification on the exported SIGNED PDF bytes (pdfaid:part=3, conformance=A, ISO 19005-3) plus the contract.jsonld associated file (AFRelationship /Source); full veraPDF-class validation remains an external check. |

## 3.2.4 Contract Storage & Archive (DCS-FR-CSA-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-FR-CSA-01 | Tamper-Proof Contract Storage | ✅ Covered | Merkle-checkpointed, TSA-anchored audit trail (ADR-16) + tamper scenarios (03/format_review tampered-PDF) prove tamper-evidence. A submitted PDF may add a signature but never redefine the contract: the submit path compares the embedded JSON-LD against the prepared document, deliberately the opposite of the federation rule where an inbound PDF is authoritative. |
| DCS-FR-CSA-02 | Role-Based Access Control | ✅ Covered | 07 role-outside-archive-scope denied; access audited (20 export audit-log scenario). |
| DCS-FR-CSA-03 | Proof-of-Existence | ✅ Covered | TSA timestamp + IPFS anchoring per event (05 TSA receipt, 08 audit anchoring). |
| DCS-FR-CSA-04 | Contract Expiry & Renewal Tracking | 🟡 Partial | Expiry detection + banner proven (19). Configurable-threshold alert notifications not modeled — deviation note. |
| DCS-FR-CSA-05 | Hierarchical Contract Storage | ✅ Covered | 20 sibling-isolation + party-scoped-bundle scenarios: hierarchy preserved and scoped in archive/bundles. |
| DCS-FR-CSA-06 | Machine-Readable Contract Storage | ✅ Covered | JSON-LD stored + exported alongside PDF (20 bundle members; 05 deploy-payload shape); sync validated pre-archive via verify. |
| DCS-FR-CSA-07 | Automated Compliance Checks | 🟡 Partial | ODRL/structural gates block non-compliant contracts before they can reach SIGNED/archive (18 approve/sign gates, 20 export refusal); a distinct archive-time re-check is not separate from the workflow gate. |
| DCS-FR-CSA-08 | Store Signed Contract in Archive | ✅ Covered | 05 archive-at-SIGNED: archive entry exactly on SIGNED with evidence. |
| DCS-FR-CSA-09 | Generate and Assign Contract Identifier | ✅ Covered | Contract DIDs assigned at creation and used across workflows (03, 12, 17). |
| DCS-FR-CSA-10 | Index Contract Metadata | ✅ Covered | 07 state-filtered archive search; archive metadata view (contracts_archive_metadata). |
| DCS-FR-CSA-11 | Create Contract Summary and Tags | ✅ Covered | 07 annotation scenarios: manual + metadata-generated summaries, tag assignment, tag-filtered search (inclusion and exclusion), RBAC negative, ANNOTATE_ARCHIVED_CONTRACT audit event. |
| DCS-FR-CSA-12 | Retrieve Contract from Archive | ✅ Covered | 07 archive retrieval with RBAC + audit (20 export audit-log). |
| DCS-FR-CSA-13 | Search Contracts | ✅ Covered | Metadata/state search (07) plus full-text content search over the whole contract JSON-LD (stored tsvector, GIN-indexed) — 07 full-text scenario with positive and negative queries. |
| DCS-FR-CSA-14 | Contract Expiration Handling | ✅ Covered | 19 expired banner + expiry cron; expired contracts excluded from active workflows. |
| DCS-FR-CSA-15 | Contract Renewal and Extension | ✅ Covered | Renewal contract linked to archived original (06, tagged @DCS-FR-CSA-15). |
| DCS-FR-CSA-16 | Contract Termination | ✅ Covered | 06 termination with reason recorded; terminated contracts remain retrievable read-only (07 search by state). |
| DCS-FR-CSA-17 | Contract Deletion | ✅ Covered | Archive delete scenarios (07, tagged @DCS-FR-CSA-17) incl. audit logging. |
| DCS-FR-CSA-18 | Audit Log for Contract Storage and Retrieval | ✅ Covered | 20 export RBAC + audit-entry scenarios (tagged @DCS-FR-CSA-18); archive audit endpoint covered (07). |
| DCS-FR-CSA-19 | Compliance Verification for Archived Contracts | 🟡 Partial | Audit entries retrievable per component (07/08); automated compliance flagging of archived entries beyond workflow gates not modeled. |
| DCS-FR-CSA-20 | Automated Contract Monitoring and Alerts | 🟡 Partial | pac/monitor continuous monitoring (08); configurable UI/email alert delivery not modeled — deviation note. |
| DCS-FR-CSA-21 | Contract Archive Dashboard | 📋 Not BDD-verifiable | Dashboard UI; backing stats/search APIs covered (07). |
| DCS-FR-CSA-22 | Contract Search Interface | 📋 Not BDD-verifiable | Search UI; backing API covered (07 archive search). |
| DCS-FR-CSA-23 | Contract Expiration and Renewal Management UI | 📋 Not BDD-verifiable | Expiry/renewal UI; backing expiry + renewal APIs covered (19, 06). |
| DCS-FR-CSA-24 | Contract Compliance and Audit Viewer | ✅ Covered | Audit viewer UI; backing pac/report + archive audit APIs covered (08, 07). UI now asserted: e2e/dashboards.spec.ts (audit workstation renders scoped audits for the auditor). |
| DCS-FR-CSA-25 | Contract Processing API | ✅ Covered | Archive store/retrieve/search/delete APIs with authz + audit (07 pack, 20 export audit-log). |
| DCS-FR-CSA-26 | Archive Multi-Party Contract Component Assignments | ✅ Covered | 20 sibling isolation across instances + party-scoped bundle content (tagged @DCS-FR-CSA-26). |

## 3.2.5 Process Audit & Compliance (DCS-FR-PACM-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-FR-PACM-01 | Tamper-Proof Audit Trail for Contract Lifecycle | ✅ Covered | 08 process audit incl. create event; exportable via /pac/report. Global tamper evidence is now a Merkle checkpoint per anchoring batch, each root chained to its predecessor and timestamped once (ADR-16), replacing the per-event global chain that serialised every event behind one TSA round-trip and stalled the trail when a single event failed; per-resource hash chains are unchanged. Leaves are blinded so a published proof reveals nothing, and 12/system_user_classes asserts a checkpoint head carrying a root and leaking no entry data. An unanchorable event is dead-lettered rather than retried silently, i.e. a gap is visible. NOT yet in force: external anchoring — the ORCE flow polling the head ends at a debug node, so the log is tamper-evident to us but not provable against the operator. |
| DCS-FR-PACM-02 | Compliance Monitoring and Risk Detection | ✅ Covered | 08 continuous monitoring; risk-during-approval scenario (03/contract_approval). |
| DCS-FR-PACM-03 | Automated Regulatory and Policy Compliance Checks | ✅ Covered | 18 ODRL gates on approve+sign; /pac/monitor sweep flags MISSING_APPROVAL risks on approval-pending contracts and anchors each as PAC_COMPLIANCE_RISK per contract (03/contract_approval monitoring scenario, 08). |
| DCS-FR-PACM-04 | Role-Based Access Control for Audit Logs | 🟡 Partial | 08 non-auditor denied. Per-access justification recording not modeled — noted. |
| DCS-FR-PACM-05 | Contract Non-Compliance Investigation and Reporting | ✅ Covered | 08 incident-report scenario; monitor + report link findings. |
| DCS-FR-PACM-06 | Structural Integrity Validation for Multi-Contract Packages | ✅ Covered | 20 export-refusal structural-integrity scenario (tagged @DCS-FR-PACM-06). |
| DCS-FR-PACM-07 | Compliance Reporting by Contract Component and Party | 🟡 Partial | Scoped audit/report per component (08 scoped audit/report scenarios). Per-party/per-clause segmentation not modeled — noted. |

## 3.1.4 Communications Interfaces (DCS-IR-CI-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-CI-01 | HTTPS/TLS 1.3 Transport | 📋 Not BDD-verifiable | TLS termination is deployment config (prod ingress); BDD kind env intentionally runs plaintext behind Traefik. Verify via deployment values review. |
| DCS-IR-CI-02 | REST/JSON API Conventions | ✅ Covered | All suite traffic is REST/JSON; PDFs served as application/pdf (03 format review HR export). |
| DCS-IR-CI-03 | Browser Access over HTTPS | 📋 Not BDD-verifiable | HTTPS for UI = same deployment concern as CI-01. |
| DCS-IR-CI-04 | OAuth2/OIDC Flows | ✅ Covered | 01 pack: OIDC login/refresh/logout/introspection paths incl. expired-credential rejection. |
| DCS-IR-CI-05 | OpenID Discovery & JWKS | ✅ Covered | Token validation against Hydra discovery/JWKS exercised by every authenticated scenario; expired-JWT scenario pins issuer handling (01). |
| DCS-IR-CI-06 | OpenID4VC/VP Bindings | 🟡 Partial | OID4VP presentation flow proven headlessly (22 webhook/PID-embedding scenarios). OID4VCI issuance is the wallet/issuer side, outside DCS runtime — noted. |
| DCS-IR-CI-07 | Orchestration Webhooks | ✅ Covered | 05 ORCE Node-RED flow round-trip incl. hash verification + ack. |
| DCS-IR-CI-08 | DSS Remote Signing over HTTPS | 🟡 Partial | Internal signing endpoints (c2paSign/padesSign) fill the DSS role in-cluster (21 internal-signing scenario); external DSS/TSP over HTTPS not reachable hermetically. |
| DCS-IR-CI-09 | Revocation List Synchronization | 🟡 Partial | CRL revocation flip proven (21); the ≤5-minute propagation bound is not timed in-suite. |
| DCS-IR-CI-10 | PACM Audit Event Transport | ✅ Covered | 08 pack uses /pac/audit + /pac/report over HTTPS JSON (transport per CI-01 in prod). |

## 3.1.3 Software Interfaces (DCS-IR-SI-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-SI-01 | Template Catalogue Integration | ✅ Covered | Template catalogue endpoints scenarios (02/template_catalogue, tagged @DCS-IR-SI-01). |
| DCS-IR-SI-02 | Workflow Orchestration (Node-RED) Integration | ✅ Covered | 05 shipped ORCE contract-target flow round-trip (tagged @DCS-IR-SI-02). |
| DCS-IR-SI-03 | Platform Authentication & Authorization Integration | ✅ Covered | 01 pack — all components enforce OAuth2/OIDC. |
| DCS-IR-SI-04 | Wallet & TSP Signing Integration | 🟡 Partial | OID4VP + remote-signing seam proven via headless ceremony + HSM signing (22); real TSP integration out of hermetic scope. |
| DCS-IR-SI-05 | External Target System API Integration | ✅ Covered | 05 external target deploy API scenarios incl. shared-secret callback (tagged @DCS-IR-SI-05). |
| DCS-IR-SI-06 | Counterparty DCS Information Endpoint | ✅ Covered | 17 get_sync/post_sync + GetServiceDID: policy-gated peer information exchange (untrusted peer rejected). |
| DCS-IR-SI-07 | OpenID Provider Discovery & JWKS Consumption | ✅ Covered | Hydra discovery/JWKS consumption (see CI-05). |
| DCS-IR-SI-08 | OpenID4VP Login & Access Control | ✅ Covered | OID4VP login is the only authentication path: Hydra login+consent is accepted solely after trust-anchored presentation verification (`auth_login.go` PresentationCallback → `oid4vp.Verify` → `AcceptLoginAndConsent`), and every authenticated scenario performs the full headless VP login. JAR ES256-signed (21); PID presentation re-verified at signing (22). |
| DCS-IR-SI-09 | Credential Status & Revocation Service | ✅ Covered | 21 CRL/status-list revocation flip. |
| DCS-IR-SI-10 | Digital Signature Service (DSS) Authorization & Signing | 🟡 Partial | DSS-shaped authorize+sign+timestamp path via internal signing + TSA (22 timestamp scenario); external DSS not hermetic. |
| DCS-IR-SI-11 | Relational Database Access | ✅ Covered | PostgreSQL with versioned migrations exercised by the entire suite (backend/migrations/sql). |
| DCS-IR-SI-12 | Crypto Provider & DID/VC Operations | ✅ Covered | 21 HSM-backed DID/VC/C2PA operations (Crypto-Provider role). |

## 3.1.1 UI — Template Repository (DCS-IR-TR-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-TR-01 | Template Builder MUST allow Template Creator to create new contract… | ✅ Covered | API: 02 create/update template. Builder UI now asserted: e2e/template-typed-clause.spec.ts creates a brand-new template through the visual builder (/ui/templates/new → hub typed-clause palette → shacl-form → save) and verifies the emitted /template/create envelope. |
| DCS-IR-TR-02 | Template Builder MUST allow searching and retrieving existing templ… | 🟡 Partial | API: 02 search/retrieve. Template listing UI asserted (e2e/dashboards.spec.ts); in-UI search interaction unasserted. |
| DCS-IR-TR-03 | Template Review MUST allow Reviewers to retrieve, verify, update, a… | 🟡 Partial | API: 02 workflow review steps. UI out of harness. |
| DCS-IR-TR-04 | Template Review MUST support forwarding a verified template to appr… | 🟡 Partial | API: 02 approve/reject/resubmit transitions. UI out of harness. |
| DCS-IR-TR-05 | Template Approval MUST allow Approvers to retrieve, approve, reject… | 🟡 Partial | API: 02 approval set. UI out of harness. |
| DCS-IR-TR-06 | Template Approval MUST ensure that only validated templates enter t… | 🟡 Partial | API: only REGISTERED templates usable in contract create (03 steps). UI out of harness. |
| DCS-IR-TR-07 | Template Management Dashboard MUST allow Managers to register, arch… | 🟡 Partial | API: 02 register/archive/update/search + audit. UI out of harness. |
| DCS-IR-TR-08 | Template Management Dashboard MUST provide lifecycle oversight of a… | 🟡 Partial | API: lifecycle oversight via search/status/history. UI out of harness. |

## 3.1.1 UI — Contract Workflow (DCS-IR-CWE-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-CWE-01 | Contract Creation UI MUST allow Contract Creators to create and sub… | 🟡 Partial | API: 03 create from approved template. UI out of harness. |
| DCS-IR-CWE-02 | Contract Creation UI MUST enable population of contract data, inclu… | ✅ Covered | API: parties/policies/evidence populated at create (03, 18, 05 evidence). UI now asserted: e2e/contract-fill.spec.ts fills a placeholder through the edit UI and the emitted document carries the forField-bound typed value in an odrl:Offer, with no editor-internal keys leaking. |
| DCS-IR-CWE-03 | Contract Negotiation UI MUST allow parties to exchange responses, r… | 🟡 Partial | API: negotiation responses/redlines/comments (03). UI out of harness. |
| DCS-IR-CWE-04 | Contract Negotiation UI MUST support comparison of contract version… | 🟡 Partial | API: version history compare (03). UI out of harness. |
| DCS-IR-CWE-05 | Contract Review UI MUST allow Reviewers to retrieve, inspect, and v… | ✅ Covered | 03 state-machine invalid-transition + approval-chain scenarios (tagged @DCS-IR-CWE-05): review path enforced. |
| DCS-IR-CWE-06 | Contract Review UI MUST allow Reviewers to respond with findings, r… | ✅ Covered | Review responses with findings/comments (tagged @DCS-IR-CWE-06 on the state-machine scenarios; approval comments in 03/contract_approval). |
| DCS-IR-CWE-07 | Contract Review UI MUST provide search capabilities to locate contr… | 🟡 Partial | API: contract search by state/metadata/parent (03 state-filtered search, 20 parent_did filter). UI out of harness. |
| DCS-IR-CWE-08 | Contract Approval UI MUST allow Approvers to retrieve contracts in … | 🟡 Partial | API: approvers retrieve reviewed contracts (03/contract_approval). UI out of harness. |
| DCS-IR-CWE-09 | Contract Approval UI MUST allow Approvers to approve, reject (with … | ✅ Covered | Approve / reject-with-reason / resubmit proven (03/contract_approval + state machine). |
| DCS-IR-CWE-10 | Contract Approval UI MUST ensure approved contracts are forwarded i… | ✅ Covered | Approved contracts proceed to signing (03 approval-transition + 22, tagged @DCS-IR-CWE-10). Catalogue forwarding is a deliberate MANUAL user action by architectural decision: catalogue registration can fail, be re-run, or be unconfigured — an explicit action models that honestly (same rationale as template publication, 02/template_catalogue). |
| DCS-IR-CWE-11 | Contract Management Dashboard UI MUST allow Managers to retrieve an… | ✅ Covered | API: lifecycle-wide search (03 state-filtered search). Dashboard UI now asserted: e2e/dashboards.spec.ts (contract dashboard lists contracts with their lifecycle state for the Contract Manager role). |
| DCS-IR-CWE-12 | Contract Management Dashboard UI MUST allow Managers to store evide… | 🟡 Partial | API: evidence store (05 TSA receipt), terminate (06), audits (08). UI out of harness. |
| DCS-IR-CWE-13 | Contract Management Dashboard UI MUST provide lifecycle monitoring … | 🟡 Partial | API: lifecycle monitoring via states/history/KPIs (05). UI out of harness. |

## 3.1.1 UI — Storage & Archive (DCS-IR-CSA-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-CSA-01 | Archive Manager Dashboard UI MUST allow Archive Managers to retriev… | ✅ Covered | 07 retrieve+search archive scenarios (tagged @DCS-IR-CSA-01). |
| DCS-IR-CSA-02 | Archive Manager Dashboard UI MUST allow storing new contracts and e… | ✅ Covered | Evidence store into archive (05 TSA receipt); signed contracts auto-stored (05 archive-at-SIGNED). |
| DCS-IR-CSA-03 | Archive Manager Dashboard UI MUST allow terminating or deleting arc… | ✅ Covered | Terminate covered (06); archive delete scenarios (07, tagged @DCS-FR-CSA-17). |
| DCS-IR-CSA-04 | Archive Manager Dashboard UI MUST allow running audits on archive o… | ✅ Covered | Archive audit endpoint covered (07, tagged @DCS-IR-CSA-04). |
| DCS-IR-CSA-05 | Archive Access UI MUST allow Observers to retrieve and search archi… | ✅ Covered | 07 least-privilege access enforcement (tagged @DCS-IR-CSA-05). |
| DCS-IR-CSA-06 | Archive Access UI MUST ensure that read-only users cannot modify, t… | ✅ Covered | 07 read-only Observer scenario: Contract Observer retrieves the archive (200) yet delete is denied — matches the design scoping (retrieve/search: Archive Manager+Observer; store/delete: Archive Manager only). |

## 3.1.1 UI — Signature Management (DCS-IR-SM-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-SM-01 | Secure Contract Viewer UI MUST allow Signers and Managers to retrie… | ✅ Covered | API: approved-contract retrieval for signing (22). Viewer UI now asserted: e2e/dashboards.spec.ts renders the human-readable document from the machine-readable JSON-LD in the contract view (Contract Content tab). |
| DCS-IR-SM-02 | Secure Contract Viewer UI MUST allow verification of contract integ… | ✅ Covered | Integrity/envelope verification via verify endpoints (08, 19, 22 verify cross-check). |
| DCS-IR-SM-03 | Secure Contract Viewer UI MUST allow applying signatures with appro… | ✅ Covered | Signature application with verified credentials (22 ceremony-gate + webhook/PID scenarios). |
| DCS-IR-SM-04 | Secure Contract Viewer UI MUST allow validation of applied signatur… | ✅ Covered | Applied-signature validation endpoint scenario (04, tagged @DCS-FR-SM-18). |
| DCS-IR-SM-05 | Signature Compliance Viewer UI MUST allow compliance users to valid… | 🟡 Partial | Compliance users (Compliance Officer/Auditor scopes) read GET /signature/view (04) with cryptographic integrity findings from the shared validation machinery; trust anchors/proofs/timestamps validated in verify paths (21, 22). UI out of harness. |
| DCS-IR-SM-06 | Signature Compliance Viewer UI MUST allow revocation of signatures … | ✅ Covered | 15 signature revocation (tagged @DCS-FR-SM-20). |
| DCS-IR-SM-07 | Signature Compliance Viewer UI MUST allow running compliance checks… | ✅ Covered | Compliance-check endpoint scenario (04, tagged @DCS-FR-SM-21). |
| DCS-IR-SM-08 | Signature Compliance Viewer UI MUST allow generating audit reports … | ✅ Covered | Signature audit-report scenario (04, tagged @DCS-FR-SM-19). |

## 3.1.1 UI — Process Audit & Compliance (DCS-IR-PACM-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-PACM-01 | Auditing Tool UI MUST allow Auditors to initiate audits across cont… | ✅ Covered | 08 scoped-audit scenario (tagged @DCS-IR-PACM-01). |
| DCS-IR-PACM-02 | Auditing Tool UI MUST provide reporting capabilities with exportabl… | ✅ Covered | 08 report-generation scenario (tagged @DCS-IR-PACM-02). |
| DCS-IR-PACM-03 | Non-Compliance Investigation UI MUST allow Compliance Officers to c… | ✅ Covered | 08 continuous monitoring with structured checked_at+risks response; risk detection during approval incl. PAC-trail anchoring in 03/contract_approval. |
| DCS-IR-PACM-04 | Non-Compliance Investigation UI MUST allow incident reporting and l… | ✅ Covered | 08 incident-reporting scenario (tagged @DCS-IR-PACM-04). |

## 3.1.2 Hardware Interfaces (DCS-IR-HI-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-IR-HI-01 | Interface for Use of Signing Secrets (HSM/QSCD/TPM) | ✅ Covered | 21 pack: PKCS#11/SoftHSM-backed keys, ES256 everywhere, rotation + CRL (tagged @DCS-IR-HI-01). |
| DCS-IR-HI-02 | FIDO2 Security Key Interface | ❌ Deviation | FIDO2/WebAuthn login not implemented (no WebAuthn endpoints). Hardware-authenticator flows also not automatable headlessly. Recorded deviation. |
| DCS-IR-HI-03 | Platform TPM 2.0 / Secure Enclave Interface | ❌ Deviation | TPM sealing/remote attestation not implemented; platform-infra concern. Recorded deviation. |

## 3.4 Business Rules (DCS-NFR-BR-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-NFR-BR-01 | Strong Authentication & Role Binding | ✅ Covered | Wallet-VC login is the only auth path — there is no password or non-VP fallback (see SI-08); roles bind from the verified presentation and RBAC is enforced everywhere (01). The second factor (wallet unlock / holder authentication) lives in the wallet, outside the DCS boundary. |
| DCS-NFR-BR-02 | Participant Eligibility | ✅ Covered | 17: unverified/untrusted peers rejected on every DCS-to-DCS surface. |
| DCS-NFR-BR-03 | Legally Valid Signatures | ✅ Covered | 05 non-SIGNED deploy refusal (tagged @DCS-NFR-BR-03); AES default (22). |
| DCS-NFR-BR-04 | Template Governance | ✅ Covered | Contract create only from REGISTERED templates (03 steps + 02 approval chain). |
| DCS-NFR-BR-05 | Immutable Auditability | ✅ Covered | Hash-chained TSA/IPFS audit for all lifecycle actions (08) + RBAC on logs (08 non-auditor denial). |
| DCS-NFR-BR-06 | Revocation & Termination Propagation | ✅ Covered | Signature revocation → REVOKED immediately (15); cross-instance propagation via the synchronizer's SignatureManagement broadcast (17 revocation-propagation scenario: revoke on A, REVOKED replicated on B through the JAdES-verified post_sync path). |
| DCS-NFR-BR-07 | Token & API Control | 🟡 Partial | Role-scoped tokens enforced (01); explicit minimal-scope token issuance policy is IdP config — noted. |
| DCS-NFR-BR-08 | DCS-to-DCS Interoperability Safeguards | ✅ Covered | 17 pack (tagged @NFR-BR-08): authenticated, trusted-peer-only exchanges with audit. Phase 4: post_sync (backend/internal/service/dcs_to_dcs.go) calls validation.RemoteShapeSource/VerifyAgainstOriginatorHub after the four existing trust layers accept a synced contract — it resolves the document's sh:shapesGraph anchor back to the ORIGINATOR's public Semantic Hub and re-validates against those exact shapes, not the receiver's own local hub (best-effort/non-blocking: a peer hub outage never fails an otherwise-trusted sync). 17/two_instance_peer_trust "A contract synced from instance A carries a sh:shapesGraph anchor resolvable against instance A's own Semantic Hub" (@two-instance) proves the reachability precondition end to end; the validation logic itself is proven by TestVerifyAgainstOriginatorHub (httptest-simulated peer hub). |
| DCS-NFR-BR-09 | Catalogue-Aligned Publishing | ✅ Covered | Catalogue publish/consume scenario (02/template_catalogue). |

## 3.3.3 Security (DCS-NFR-SEC-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-NFR-SEC-01 | Transport Layer Security | 📋 Not BDD-verifiable | TLS 1.3-only is deployment/ingress config; not asserted from the plaintext BDD env. |
| DCS-NFR-SEC-02 | State-of-the-art Cryptography | ✅ Covered | 21: P-256/ES256 across DID/JAR/VC/C2PA/PAdES, no legacy RSA (tagged @DCS-NFR-SEC-02). |
| DCS-NFR-SEC-03 | Authentication and Authorization | ✅ Covered | 01 pack + role negatives suite-wide; party read-scoping on retrieve_by_id (03: dcs:parties gate, 403 forbidden, CONTRACT_ACCESS_DENIED audit event; Sys.*/Auditor org-independent; peer-adopted contracts readable by the adopting instance). |
| DCS-NFR-SEC-04 | Integrity Protection for Configuration | 📋 Not BDD-verifiable | Config integrity (signed/authenticated config) is platform concern; Helm-managed config — review-verified. |
| DCS-NFR-SEC-05 | Integrity Protection for Service | 📋 Not BDD-verifiable | Service integrity/attestation — platform concern (image digests, admission control). |
| DCS-NFR-SEC-06 | Storage of Secrets | ✅ Covered | Private keys live in PKCS#11 token only (21 DID-key scenarios; provisioning scripts). |
| DCS-NFR-SEC-07 | Testing | 📋 Not BDD-verifiable | Process requirement — this BDD suite + Go tests + linters + CI are the evidence; pentest is external. |
| DCS-NFR-SEC-08 | Confidentiality | 🟡 Partial | RBAC + party read-scoping proven at API level (03 party-access scenarios, 403 + audit trail); storage-level encryption is infra (SEC-14). |
| DCS-NFR-SEC-09 | Monitoring, Logging & Auditability | ✅ Covered | Immutable audit logs retrievable for audits (08); /metrics exposed (16/prometheus). |
| DCS-NFR-SEC-10 | Data Integrity | ✅ Covered | Per-resource hash chains + Merkle checkpoint roots (ADR-16) + tamper-detection scenarios (03/format_review) + C2PA/PAdES integrity (19/22). |
| DCS-NFR-SEC-11 | Monitoring & Incident Response | 🟡 Partial | Prometheus /metrics (16); automated incident response not modeled — noted. |
| DCS-NFR-SEC-12 | Secure Configuration Management | 📋 Not BDD-verifiable | Secure config management — GitOps/platform concern. |
| DCS-NFR-SEC-13 | Secure Data Disposal | 🟡 Partial | Archive delete with audit (07); cryptographic erasure policy is infra — noted. |
| DCS-NFR-SEC-14 | Data Encryption at Rest & In Transit | 📋 Not BDD-verifiable | Encryption at rest — storage/platform config; in transit see SEC-01. |
| DCS-NFR-SEC-15 | Secure Software Development Lifecycle (SDLC) | 📋 Not BDD-verifiable | SDLC process — lint/hooks/CI in repo; review-verified. |
| DCS-NFR-SEC-16 | Identity Federation | 🟡 Partial | OIDC federation via Hydra proven; third-party IdP interop is config — noted. |
| DCS-NFR-SEC-17 | Secure Boot & Hardware Security | ❌ Deviation | Secure boot — out of software scope for this service; platform deviation. |
| DCS-NFR-SEC-18 | Selective Disclosure for Privacy | ✅ Covered | SD-JWT selective disclosure in ceremony webhook (22 webhook + verbatim-embedding scenarios, tagged @NFR-SEC-18). |

## 3.3.2 Safety (DCS-NFR-SF-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-NFR-SF-01 | Reset Possibility | 📋 Not BDD-verifiable | Stateless pods restarted throughout suite runs (rollouts during deploys) — k8s-level property; review-verified. |
| DCS-NFR-SF-02 | Remote Administration | 📋 Not BDD-verifiable | Remote administration channel — operations concern. |
| DCS-NFR-SF-03 | Business Continuity & Disaster Recovery | 📋 Not BDD-verifiable | BC/DR — operations concern (RTO/RPO). |

## 3.3.1 Performance (DCS-NFR-PER-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-NFR-PER-01 | Performance by Design | 📋 Not BDD-verifiable | Performance-by-design — evidenced anecdotally by suite runtime; formal load testing out of BDD scope. |
| DCS-NFR-PER-02 | Scalability | 📋 Not BDD-verifiable | Scalability — load testing out of BDD scope. |
| DCS-NFR-PER-03 | Availability & Resilience | 📋 Not BDD-verifiable | Availability/resilience — ops concern; k8s restarts exercised incidentally. |

## 3.3.4 Software Quality (DCS-NFR-SQ-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-NFR-SQ-01 | Programming Style | 📋 Not BDD-verifiable | Code style — golangci-lint/ESLint/Prettier + hooks; review-verified. |
| DCS-NFR-SQ-02 | Build Scripts | ✅ Covered | Build scripts exist and are exercised by CI + dev-stack (repo Makefiles/scripts) — process evidence. |
| DCS-NFR-SQ-03 | Containerized Deployment | ✅ Covered | Docker multi-stage + Helm on kind is exactly how this suite runs — continuously proven in CI. |
| DCS-NFR-SQ-04 | Privacy by Design | 📋 Not BDD-verifiable | Privacy by design — process; selective disclosure covered (SEC-18). |
| DCS-NFR-SQ-05 | Non-Repudiation | ✅ Covered | Non-repudiation: PAdES + RFC3161 TSA + signer identity + immutable audit (22, 05 TSA receipt). |
| DCS-NFR-SQ-06 | System Interoperability | 🟡 Partial | Interoperability: DCS-to-DCS (17), ORCE (05 round-trip), OIDC; broader enterprise-system matrix untested. |
| DCS-NFR-SQ-07 | Usability & Accessibility | 📋 Not BDD-verifiable | WCAG/usability — UI concern out of harness. |
| DCS-NFR-SQ-08 | Orchestration Layer | ✅ Covered | FACIS/XFSC ORCE integration is load-bearing in the suite (TSA + contract-target flows). |

## 3.3.5 Compliance (DCS-NFR-COMP-…)

| ID | Requirement | Status | Evidence / disposition |
|---|---|---|---|
| DCS-NFR-COMP-01 | Legal Compliance | 📋 Not BDD-verifiable | Legal/GDPR/eIDAS compliance — documentation/process requirement (includes COMP-02/03 embedded in SRS text). |

## Use-case map (SRS §4 / `[DCS-FR-UC-XX-Y]` linkage sections)

| UC | Title | Feature pack(s) |
|---|---|---|
| UC-01 | User Authentication & Authorization | 01_authentication_authorization |
| UC-02 | Contract Template Management | 02_template_management (incl. catalogue) |
| UC-03 | Contract Creation | 03_contract_creation (creation, negotiation, approval, format review, state machine) |
| UC-04 | Contract Signing | 22_real_signing_vertical, 04_contract_signing, 21_pki_consolidation |
| UC-05 | Contract Deployment | 05_contract_deployment |
| UC-06 | Contract Lifecycle Management | 06_contract_lifecycle (termination + renewal), 19 (expiry) |
| UC-07 | Contract Storage & Security | 07_contract_storage_security, 20 (bundles/audit) |
| UC-08 | Contract Compliance & Auditing | 08_audit_compliance, 18_odrl_soundness |
| UC-09 | DCS Administration | Roles are derived from credential presentation and then embedded in the ORY/Hydra session (01_authentication_authorization: federated login, role enforcement); negatives suite-wide |
| UC-10 | Contract Automation & Integration | 05 (ORCE), 12 (API automation), 18 (integrity gates) |
| UC-11 | API & System Integrations | 05, 12, 17; catalogue (02) |
| UC-12 | System-Based Contract Management | 12_system_based_contract_management (contract_lifecycle_via_api, system_user_classes) |
| UC-13 | External System Contract Execution | 05_contract_deployment (target-system deploy/callback/evidence) |
| UC-14 | Identity & PoA Credential Acquisition | 22 (PID identity); PoA = deviation (14_credential_acquisition documents it) |
| UC-15 | Access Rights Revocation | 15_access_revocation, 21 CRL revocation |

## Deviation register (capabilities the product does not implement — honest ❌, no fake scenarios)

| Item | Requirement(s) | Note |
|---|---|---|
| QES execution | DCS-FR-SM-01 | Needs qualified TSP/QSCD; AES delivered |
| PoA credential acquisition + chain-walk | DCS-FR-SM-03/04, UC-14 | PoA presented at login with status checking; issuer chain-walk is deferred roadmap work — 14 pack keeps tagged @skip placeholders |
| Configurable expiry/alert notifications | DCS-FR-CSA-04/20 | Detection covered; delivery channels absent |
| FIDO2/WebAuthn | DCS-IR-HI-02 | No WebAuthn endpoints |
| TPM sealing / remote attestation | DCS-IR-HI-03, DCS-NFR-SEC-17 | Platform concern, not implemented |
| "Replaced" C2PA lifecycle banner | (19 lifecycle-banner subset) | Explicit scope decision, tracked in 19 pack header |

OID4VP-as-login (DCS-IR-SI-08, DCS-NFR-BR-01) is exercised by every scenario's
authentication: the harness performs the full headless VP login (login → challenge binding →
JAR → vp_token → verification → token exchange) and the backend accepts a Hydra login only
through that path. A dedicated rejected-presentation-at-login negative remains follow-up
work, not a deviation.
Multi-signer flows (DCS-FR-SM-07/17) graduated from this list: 22/multi_signer asserts them
end-to-end, including two distinct signer identities and the deploy gate.


## Semantic canonicity evidence (interoperability assertion)

Claim: every artifact the DCS produces is standard, canonical semantics — consumable by
off-the-shelf JSON-LD/RDF/ODRL/SHACL tooling with no DCS-specific conventions beyond a served,
declared ODRL profile.

| Assertion | Evidence |
|---|---|
| Documents are valid JSON-LD 1.1; every term resolves | `@context` is the Semantic Hub's versioned, dereferenceable URL (23/semantic_hub resolve scenarios); json-gold expansion round-trips in every audit (`expandForAudit`, backend/internal/base/validation/odrlexpanded.go); external contexts resolve or normalization fails (`validateExternalContextsResolvable`). |
| Policy sets are conformant ODRL 2.2 | One enclosing `odrl:Offer` (unsigned) / `odrl:Agreement` (sealed at first signature — 18 "first signature seals" scenario); rules under permission/prohibition/obligation only (odrl:duty bucket rejected); policy identity is `@id` (separate `uid` rejected); exactly one action + assigner/assignee/target per rule (18 structure scenarios; `validateODRLPolicySet`). |
| Custom semantics are declared, not implied | The DCS ODRL profile is a served document (`/semantic/ontology/dcs-odrl-profile`; docs/semantic-ontology/odrl/dcs-odrl-profile.ttl) declaring `dcs:provideCompliantValue odrl:includedIn odrl:use` and `dcs:ContractField ⊑ odrl:LeftOperand`; every policy set declares `odrl:profile`. |
| Constraint↔value binding is plain graph traversal | A submitted value references its field by IRI (`dcs:forField` = the constraint's `odrl:leftOperand`); enforced by SHACL (`dcs:SemanticConditionValueShape`) and exercised by every 18/05 enforcement scenario plus the Playwright fill spec (e2e/contract-fill.spec.ts). |
| Machine rules are prose-backed | `dcs:prose` required on every rule (Go gate + `Odrl*ProseShape` SHACL shapes); the Playwright builder spec asserts the emitted rule's prose dereferences to a document block. |
| SHACL validation is real SHACL | goRDFlib engine (ADR-9) against hub-served shapes pinned by `sh:shapesGraph` (ADR-8); version-pinning proven by 23 "stricter shapes version" scenario; shacl-form renders forms from the same raw Turtle (Gaia-X shapes render unmodified). |
| Provenance is PROV-aligned | `dcs:derivedFromTemplate ⊑ prov:wasDerivedFrom`, `dcs:renewsContract ⊑ prov:wasRevisionOf` (generated OWL, docs/semantic-ontology/linkml); derivation carries the template `@id` + version as a node, not an opaque blob. |
| Canonical hashing is a standard | RFC 8785 JCS for the JAdES payload and deployment content hash — byte-identical across Go (`gowebpki/jcs`) and Python (`jcs`), proven by the cross-language fixture test and 17/05 hash-verification scenarios. |
| Vocabulary identity is shared, not per-instance | w3id.org namespaces are the vocabulary identity (hub serves the content); two federated instances speak the same terms (17 peer scenarios exchange and verify full documents). Public w3id dereferencing is the one external residual (registration PR). |

## eIDAS 2.0 signature soundness evidence

Claim: the signature chain is architecturally sound under eIDAS 2.0 for advanced electronic
signatures (AdES), with qualified-level (QES) execution being the single recorded deviation
(no qualified TSP/QSCD reachable from the hermetic environment; `credential_type` carries the
level end-to-end for when one is wired).

| Assertion | Evidence |
|---|---|
| Signer identification via the EUDI-wallet mechanism | Signing requires a completed OID4VP PID presentation ceremony (SD-JWT VC + KB-JWT), verified before any signature (`ErrCeremonyRequired`, 22 ceremony-gate scenarios); the presentation is embedded verbatim into the signed PDF inside a signing-summary VC (embed-first-sign-second, 22 PID-embedding + verify cross-check). |
| Signature format is a recognized AdES baseline | PAdES signatures over PDF/A-3 via the HSM-backed P-256 key (PKCS#11/SoftHSM2, ADR-1; 21 internal-signing + CRL scenarios); DCS-to-DCS transport signatures are JAdES baseline-B (ETSI TS 119 182-1: sigT marked crit, x5c chain — backend/internal/base/jades, 17 peer verification scenarios). |
| Trust anchoring against the EU list | `EUTrustPool` (LOTL/TSL) gates every contract mutation and both peer-sync legs (`VerifyEIDASCertificate` in contract_workflow_engine.go and dcs_to_dcs.go); dev CA is a swappable TrustAnchor, not a bypass. |
| Revocation is enforced | Credential status lists checked on every verification via the mechanism-detecting status verifier (W3C BitstringStatusList/StatusList2021, IETF Token Status List over JOSE/COSE-CWT, XFSC — backend/internal/auth/oid4vp/status + statuslist_verify.go, docs/status-list-verification.md); CRL revocation flips signing to rejection (21 CRL scenario); signature revocation is first-class (04 revoke scenarios). |
| Time evidence is RFC 3161 | Archive entries and deployment receipts carry TSA timestamps verified against the provisioned TSA certificate (05 TSA-evidence scenario, 07 archive entries); JAdES carries claimed signing time (sigT). |
| The signed bytes are the semantic document | The content hash (RFC 8785) and the PAdES signature are computed over the sealed odrl:Agreement document inside the signing transaction (apply.go seal-before-hash); export/verify recompute and compare (03 format_review tamper seams, 20 export refusal). |
| Integrity of the archived artifact | C2PA manifest with lifecycle assertions stamped before signing (update-then-sign, ADR-4; 19 conformance scenarios), archive chain hash-linked and TSA-timestamped with signing evidence embedded (08 audit_ui_archive_orce feature). |
| What would make it QES | A qualified TSP signing certificate on a QSCD replacing the dev CA in the same PKCS#11 slot, plus a qualified TSA endpoint in `TSA_URL` — both configuration-level swaps by design (ADR-1, ADR-3); recorded as the DCS-FR-SM-01 partial. |
