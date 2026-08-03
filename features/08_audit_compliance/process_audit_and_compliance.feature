# Process Audit & Compliance Management (UC-08,
# backend/design/process_audit_and_compliance.go, /pac/...). C2PA provenance
# export is covered separately by c2pa_provenance_export.feature. This file
# covers the four /pac/* endpoints themselves.

@UC-08 @DCS-IR-PACM-01
Feature: Process audit and compliance management

  @UC-08-02 @DCS-IR-PACM-01 @DCS-FR-UC-08-2 @DCS-IR-CI-10
  Scenario: Auditor triggers an audit on the CONTRACT_WORKFLOW_ENGINE scope and sees the create event
    Given contract "PAC Audit Contract" is in "Draft" status
    When the Auditor triggers a process audit with scope "CONTRACT_WORKFLOW_ENGINE"
    Then get http 200:Success code
    And the process audit response includes an audit trail entry for contract "PAC Audit Contract"

  @UC-08-02 @DCS-IR-PACM-01 @DCS-NFR-BR-05
  Scenario: A role outside Auditor/Compliance Officer cannot trigger a process audit
    Given I am authenticated with roles: "Template Creator"
    When I attempt to trigger a process audit with scope "CONTRACT_WORKFLOW_ENGINE"
    Then the request is denied with a client error

  @UC-08-01 @DCS-IR-PACM-02 @DCS-FR-UC-08-1 @DCS-IR-CI-10 @DCS-NFR-SEC-09
  Scenario: Auditor generates an audit report
    Given contract "PAC Report Contract" is in "Draft" status
    When the Auditor requests an audit report for scope "CONTRACT_WORKFLOW_ENGINE" in format "json"
    Then get http 200:Success code

  @UC-08-02 @DCS-IR-PACM-03
  Scenario: Compliance Officer runs continuous monitoring
    When the Compliance Officer requests continuous monitoring
    Then get http 200:Success code
    # The sweep event itself carries no resource DID and is anchored only to
    # the global chain, not the per-component PAC read path — the auditable
    # per-contract artifact (PAC_COMPLIANCE_RISK) is asserted by the
    # "Compliance monitoring detects risk during approval" scenario in
    # 03_contract_creation/contract_approval.feature.
    And the monitoring response reports a checked_at timestamp and a risks list

  # UNAUTHORIZED_ACCESS rule (querymonitor.go): the sweep reads back the
  # persisted CONTRACT_ACCESS_DENIED artifacts written by the party
  # read-scoping denial branch (query/contract/querybyid.go) and flags the
  # affected contract, attributing the risk to the denied actor (the
  # OID4VP-disclosed organization persisted as retrieved_by). The denial
  # event is committed before the 403 is returned, so the monitor sees it
  # without polling.
  @UC-08-02 @DCS-IR-PACM-03 @DCS-FR-PACM-02
  Scenario: Continuous monitoring flags a persisted access denial as an unauthorized-access risk
    Given I am authenticated with roles: "Contract Creator"
    And I have created contract "PAC Denied Access Contract" with parties "Acme Corp" and "TechVendor Inc"
    When a representative of unrelated party "UnrelatedCorp" attempts to access contract "PAC Denied Access Contract"
    Then the access is denied with a "not authorized to access this contract" error
    When the Compliance Officer requests continuous monitoring
    Then get http 200:Success code
    And the monitoring sweep flags contract "PAC Denied Access Contract" with an "UNAUTHORIZED_ACCESS" compliance risk attributed to actor "UnrelatedCorp"

  # DCS-FR-PACM-02 runs the sweep unattended on a schedule (the pac-monitor
  # CronJob, backed by cmd/pacmonitor), so the same violation is seen again and
  # again. Alerting therefore has to be per incident, not per sweep: the audit
  # trail records the risk once, while the sweep response keeps reporting it for
  # as long as it holds. Two consecutive sweeps are what distinguishes the two.
  @UC-08-02 @DCS-IR-PACM-03 @DCS-FR-PACM-02 @DCS-NFR-SEC-11
  Scenario: Continuous monitoring alerts once per violation, not on every sweep
    Given I am authenticated with roles: "Contract Creator"
    And I have created contract "PAC Repeat Sweep Contract" with parties "Acme Corp" and "TechVendor Inc"
    When a representative of unrelated party "UnrelatedCorp" attempts to access contract "PAC Repeat Sweep Contract"
    Then the access is denied with a "not authorized to access this contract" error
    When the Compliance Officer requests continuous monitoring
    And the Compliance Officer requests continuous monitoring
    Then get http 200:Success code
    And the monitoring sweep flags contract "PAC Repeat Sweep Contract" with an "UNAUTHORIZED_ACCESS" compliance risk attributed to actor "UnrelatedCorp"
    And the PAC audit trail records exactly one "UNAUTHORIZED_ACCESS" risk for contract "PAC Repeat Sweep Contract"

  @UC-08-02 @DCS-IR-PACM-04
  Scenario: Compliance Officer submits a non-compliance incident report
    When the Compliance Officer submits a non-compliance incident report
    Then get http 200:Success code

  # Backend half of the Non-Compliance Investigation UI (AC5, UI half is
  # frontend/ClientApp/e2e/non-compliance-investigation.spec.ts): the report
  # must not be a no-op — it links the finding, typed, to an affected contract
  # (or template) and the server must persist that link so it is auditable.
  @REQ-non-compliance-investigation-ui-AC5 @DCS-IR-PACM-04 @UC-08-02 @DCS-FR-PACM-05
  Scenario: A submitted incident report is persisted as a PAC audit event linked to the affected contract
    Given contract "PAC Incident Contract" is in "Draft" status
    When the Compliance Officer submits a non-compliance incident report linking contract "PAC Incident Contract" with risk type "UNAUTHORIZED_CLAUSE_CHANGE" and detail "Clause altered outside the approved negotiation window"
    Then get http 200:Success code
    And the incident report is recorded as a PAC audit event for contract "PAC Incident Contract" with risk type "UNAUTHORIZED_CLAUSE_CHANGE"

  # DCS-NFR-SEC-04: the deployment's file-based configuration (DID document,
  # OID4VP trust data, x5c trust anchors) is hashed at every startup and the
  # attestation recorded in the audit outbox
  # (processauditandcompliance/configattest, wired in cmd/dcs/main.go).
  # Operator hash pins (DCS_CONFIG_SHA256_PINS) turn the attestation into an
  # enforced gate — the abort path is covered by the package's unit tests;
  # this scenario asserts the running deployment produced the verification
  # log the requirement's measurement names.
  @DCS-NFR-SEC-04
  Scenario: Startup records a config integrity attestation with per-file hashes
    Then the audit outbox holds a CONFIG_INTEGRITY_ATTESTATION record hashing the DID document
