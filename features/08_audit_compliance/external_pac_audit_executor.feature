# Contract boundary for externally executed PAC checks. The bundled ORCE flow
# is the reference implementation; deployments may point the same versioned
# HTTP contract at another compatible executor.

@UC-08-02 @DCS-IR-PACM-01 @DCS-FR-UC-08-2 @DCS-FR-PACM-03
Feature: Configurable external PAC audit executor

  @REQ-external-pac-audit-executor-AC1 @DCS-FR-PACM-04 @DCS-NFR-SEC-03
  Scenario: Authentication and request validation happen before executor dispatch
    Given the reference ORCE audit executor is reachable and its observations are reset
    When the "unauthenticated contracts scope" PAC audit request is submitted
    Then the PAC audit request is rejected before execution
    And the ORCE audit executor has observed no request
    When the "authenticated unsupported scope" PAC audit request is submitted
    Then the PAC audit request is rejected before execution
    And the ORCE audit executor has observed no request

  @REQ-external-pac-audit-executor-AC2 @DCS-IR-SI-02 @DCS-IR-CI-07 @DCS-NFR-SEC-12
  Scenario: The bundled ORCE flow and a configuration-only replacement implement the same contract
    Given the reference ORCE audit executor is reachable and its observations are reset
    When a valid versioned audit request is posted directly to the bundled ORCE reference flow
    Then the ORCE response satisfies the versioned audit executor contract
    And the ORCE response identifies executor "facis-orce-reference"
    Given the ORCE audit executor observations are reset
    When the Auditor requests an executor-backed "contracts" audit without a resource DID
    Then the PAC audit request is accepted with executor "customer-compatible-audit"
    And the ORCE audit executor has observed exactly one request
    And the observed request was handled by the compatible endpoint "/customer-audit/run"

  @REQ-external-pac-audit-executor-AC3 @DCS-IR-PACM-01 @DCS-FR-CSA-19
  Scenario: DCS dispatches one correlated evidence-bearing job for every audit scope
    Given contract "External Executor Evidence Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And the reference ORCE audit executor is reachable and its observations are reset
    When the Auditor requests an executor-backed "templates" audit for contract "External Executor Evidence Contract" with justification "external executor BDD"
    Then the ORCE audit executor has observed exactly one request
    And the observed executor request carries version, audit correlation, requester, scope, resource, and justification
    And the observed executor request contains DCS-procured "templates" evidence
    Given the reference ORCE audit executor is reachable and its observations are reset
    When the Auditor requests an executor-backed "contracts" audit for contract "External Executor Evidence Contract" with justification "external executor BDD"
    Then the ORCE audit executor has observed exactly one request
    And the observed executor request carries version, audit correlation, requester, scope, resource, and justification
    And the observed executor request contains DCS-procured "contracts" evidence
    Given the reference ORCE audit executor is reachable and its observations are reset
    When the Auditor requests an executor-backed "signatures" audit for contract "External Executor Evidence Contract" with justification "external executor BDD"
    Then the ORCE audit executor has observed exactly one request
    And the observed executor request carries version, audit correlation, requester, scope, resource, and justification
    And the observed executor request contains DCS-procured "signatures" evidence
    Given the reference ORCE audit executor is reachable and its observations are reset
    When the Auditor requests an executor-backed "archive" audit for contract "External Executor Evidence Contract" with justification "external executor BDD"
    Then the ORCE audit executor has observed exactly one request
    And the observed executor request carries version, audit correlation, requester, scope, resource, and justification
    And the observed executor request contains DCS-procured "archive" evidence

  @REQ-external-pac-audit-executor-AC4 @DCS-FR-PACM-01 @DCS-FR-PACM-07 @DCS-NFR-SEC-10
  Scenario: Executor identity, version, receipt, and findings survive into the persisted result
    Given contract "Persisted Executor Result Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And the reference ORCE audit executor is reachable and returns a valid result with a receipt
    When the Auditor requests an executor-backed "archive" audit for contract "Persisted Executor Result Contract" with justification "persist executor evidence"
    Then the PAC audit request is accepted with executor "customer-compatible-audit"
    And the audit result contains executor version, execution time, correlated findings, and receipt
    And the PAC audit run stores audit and correlation IDs, executor provenance, receipt, and request and response hashes
    And a matching PAC_AUDIT_EXECUTED outbox event records the persisted run hashes
    And the persisted PAC audit run rejects an update as append-only
    When the Auditor reads the persisted "archive" report for contract "Persisted Executor Result Contract"
    Then the report contains the same audit ID, executor metadata, findings, and receipt

  @REQ-external-pac-audit-executor-AC4 @REQ-external-pac-audit-executor-AC5
  @DCS-IR-CI-10 @DCS-NFR-PER-03 @DCS-NFR-SEC-10
  Scenario: Invalid or unavailable executor results fail closed without local findings
    Given contract "Fail Closed Audit Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And the reference ORCE audit executor is reachable and returns "mismatched version"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned
    Given the reference ORCE audit executor is reachable and returns "mismatched correlation"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned
    Given the reference ORCE audit executor is reachable and returns "mismatched scope"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned
    Given the reference ORCE audit executor is reachable and returns "mismatched resource"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned
    Given the reference ORCE audit executor is reachable and returns "invalid finding"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned
    Given the reference ORCE audit executor is reachable and returns "non-2xx response"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned
    Given the reference ORCE audit executor is reachable and returns "malformed JSON"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned
    Given the reference ORCE audit executor is reachable and returns "timeout"
    When the Auditor requests an executor-backed "contracts" audit for contract "Fail Closed Audit Contract" with justification "fail closed BDD"
    Then the audit fails with an executor infrastructure error
    And no local fallback findings are returned

  @REQ-external-pac-audit-executor-AC5 @DCS-IR-CI-10 @DCS-NFR-PER-03
  Scenario: A valid executor result with no findings is a successful empty audit
    Given the reference ORCE audit executor is reachable and returns a valid empty result
    When the Auditor requests an executor-backed "archive" audit without a resource DID
    Then the PAC audit request is accepted with an empty findings list
    And the ORCE audit executor has observed exactly one request

  @REQ-external-pac-audit-executor-AC6 @DCS-IR-PACM-02 @DCS-FR-UC-08-1 @DCS-FR-PACM-05 @DCS-FR-PACM-07
  Scenario Outline: Every report format uses the persisted run and never executes the checks again
    Given contract "Persisted Report Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And the reference ORCE audit executor is reachable and returns a valid result with a receipt
    When the Auditor requests an executor-backed "archive" audit for contract "Persisted Report Contract" with justification "report replay BDD"
    Then the PAC audit request is accepted with executor "customer-compatible-audit"
    And the ORCE audit executor has observed exactly one request
    When the Auditor exports format "<format>" from the persisted "archive" run for contract "Persisted Report Contract"
    Then the "<format>" report represents the same persisted executor run
    And the exact report bytes have a PAC_REPORT_GENERATED SHA-256 hash and IPFS CID
    And the ORCE audit executor has still observed exactly one request

    Examples:
      | format |
      | json   |
      | csv    |
      | pdf    |
