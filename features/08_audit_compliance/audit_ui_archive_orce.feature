# Minimal completion slice for the existing audit workstation and the archive
# integrity evidence created by the SIGNED transition. Continuous monitoring,
# incident management, retention and general archive CRUD are intentionally
# outside this feature.

# @isolated_stack: every scenario assumes a clean database (@clean_db wipes
# contracts but not the append-only audit/archive chains) and one scenario
# restarts ORCE — both incompatible with the 340-scenario shared-stack run.
# Runs in its dedicated stack: make -C tests/bdd run_bdd_audit_kind_ci.
@isolated_stack @UC-07-03 @UC-08-01 @UC-08-02 @DCS-IR-PACM-01 @DCS-IR-PACM-02 @DCS-IR-CSA-04
Feature: Minimal auditing workstation with archive integrity and ORCE evidence

  @REQ-audit-ui-archive-orce-AC1 @DCS-IR-PACM-01 @DCS-FR-CSA-24 @DCS-FR-PACM-01
  Scenario: Auditor can filter every audit scope to a real resource DID
    Given contract "Scoped Audit Contract" is submitted, reviewed, approved, and signed via the standard workflow
    When the Auditor runs scope "templates" for the source template of that contract with justification "external audit BDD-4711"
    Then the process audit request is accepted
    And the filtered audit contains a non-empty "templates" group for the source template of that contract
    When the Auditor runs scope "contracts" for that contract with justification "external audit BDD-4711"
    Then the process audit request is accepted
    And the filtered audit contains a non-empty "contracts" group for that contract
    When the Auditor runs scope "signatures" for that contract with justification "external audit BDD-4711"
    Then the process audit request is accepted
    And the filtered audit contains a non-empty "signatures" group for that contract
    When the Auditor runs scope "archive" for that contract with justification "external audit BDD-4711"
    Then the process audit request is accepted
    And the filtered audit contains a non-empty "archive" group for that contract

  @REQ-audit-ui-archive-orce-AC1 @DCS-IR-CSA-04 @DCS-FR-CSA-24 @DCS-FR-PACM-04
  Scenario: Archive Manager is restricted to the archive audit scope
    When the Archive Manager runs scope "archive" with justification "archive control BDD-4712"
    Then the process audit request is accepted
    When the Archive Manager runs scope "contracts" with justification "scope escalation BDD-4713"
    Then the request is denied with an authorization error

  @REQ-audit-ui-archive-orce-AC1 @DCS-IR-PACM-01 @DCS-FR-PACM-04
  Scenario: Audit scope and justification are validated
    When the Auditor runs scope "unknown" with justification "scope validation BDD-4714"
    Then get http 400:Bad Request
    When the Auditor runs scope "contracts" without a justification
    Then get http 400:Bad Request

  @REQ-audit-ui-archive-orce-AC2 @DCS-IR-CSA-04 @DCS-FR-CSA-24
  Scenario: Archive integrity results carry unambiguous workstation semantics
    Given contract "Audit Semantics Contract" is submitted, reviewed, approved, and signed via the standard workflow
    When the Auditor runs scope "archive" for that contract with justification "semantic classification BDD-4715"
    Then the audit response distinguishes timeline events from integrity checks
    And every integrity check has a result, rule reference, and reason
    And the archive integrity result is passed

  @REQ-audit-ui-archive-orce-AC2 @DCS-IR-PACM-01
  @clean_db
  Scenario: An empty audit is an explicit successful state
    Given the audit executor returns a successful empty result
    When the Auditor runs scope "archive" with justification "empty state BDD-4716"
    Then the audit response is a successful empty result

  @REQ-audit-ui-archive-orce-AC2 @DCS-IR-PACM-01
  Scenario: Audit API reports an infrastructure failure separately from an empty result
    When the Auditor runs an archive audit while audit trail persistence is unavailable with justification "infrastructure state BDD-4717"
    Then the audit request fails with an infrastructure error

  @REQ-audit-ui-archive-orce-AC1 @DCS-IR-PACM-01
  Scenario: A pre-effective contract remains visible as lifecycle evidence
    Given contract "Pre-effective Audit Contract" exists in a pre-effective lifecycle state
    When the Auditor runs scope "contracts" for that contract with justification "pre-effective lifecycle BDD-4717"
    Then the process audit request is accepted
    And the contract audit contains lifecycle evidence for that contract
    And no failed finding is caused solely by the contract being pre-effective

  @REQ-audit-ui-archive-orce-AC3 @DCS-FR-CSA-19 @UC-08-02 @DCS-FR-CSA-01 @DCS-FR-CSA-03 @DCS-NFR-BR-05 @DCS-NFR-SEC-10
  Scenario: A valid archive entry passes each independent integrity rule
    Given contract "Valid Integrity Contract" is submitted, reviewed, approved, and signed via the standard workflow
    When the Auditor runs scope "archive" for that contract with justification "integrity proof BDD-4718"
    Then passed findings exist for DB snapshot, content hash, IPFS snapshot, ORCE receipt, ORCE chain, and RFC-3161 TSA

  @REQ-audit-ui-archive-orce-AC3 @DCS-FR-CSA-19 @UC-08-02 @DCS-FR-CSA-01
  Scenario Outline: Archive evidence defects are returned as individual failed findings
    Given contract "Defective Integrity Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And its archived evidence is corrupted as "<defect>"
    When the Auditor runs scope "archive" for that contract with justification "negative integrity BDD-4719"
    Then get http 200:Success code
    And a failed archive finding with rule "<rule>" and a non-empty reason is returned

    Examples:
      | defect          | rule                 |
      | content hash    | ARCHIVE_CONTENT_HASH |
      | missing receipt | ARCHIVE_ORCE_RECEIPT |
      | invalid TSA     | ARCHIVE_TSA_RFC3161  |

  @REQ-audit-ui-archive-orce-AC4 @DCS-FR-CWE-20 @DCS-FR-CSA-18 @DCS-FR-CSA-08 @DCS-NFR-SQ-04
  Scenario: SIGNED archival records real signing evidence without credential disclosure
    Given contract "Signing Evidence Contract" is submitted, reviewed, approved, and signed via the standard workflow
    Then its archive entry records signer, credential type, ceremony, field, signing time, PDF CID, and PDF hash
    And its archive entry stores credential hashes but no credential payload

  @REQ-audit-ui-archive-orce-AC5 @DCS-FR-CWE-20 @DCS-FR-CSA-18
  Scenario: ORCE continues the persisted chain after a restart
    Given the configured ORCE archive notary is reachable with its bearer token
    When archive event "bdd-restart-chain-first" is notarized
    And the ORCE archive flow is restarted
    And archive event "bdd-restart-chain-second" is notarized
    Then the second ORCE receipt references the first receipt hash

  @REQ-audit-ui-archive-orce-AC5 @DCS-FR-CSA-18 @DCS-NFR-SEC-09
  Scenario: ORCE protects both append and retrieval with the configured bearer token
    Given the configured ORCE archive notary is reachable with its bearer token
    When the current ORCE audit log is remembered
    And archive event "bdd-unauthorized-append" is posted without a bearer token
    Then the ORCE request is unauthorized
    And the ORCE audit log is unchanged
    When the ORCE audit log is requested without a bearer token
    Then the ORCE request is unauthorized

  @REQ-audit-ui-archive-orce-AC5 @DCS-FR-CSA-18
  Scenario: ORCE append is idempotent and rejects conflicting reuse of an archive ID
    Given the configured ORCE archive notary is reachable with its bearer token
    When archive event "bdd-idempotent-entry" is notarized twice with identical content
    Then both ORCE responses contain the same receipt
    When archive event "bdd-idempotent-entry" is notarized with different content
    Then the ORCE request is rejected as a conflict

  @REQ-audit-ui-archive-orce-AC6 @DCS-IR-PACM-02 @UC-08-01 @UC-08-02 @DCS-FR-PACM-01 @DCS-FR-PACM-05 @DCS-FR-UC-08-1
  Scenario Outline: Every report format contains the complete filtered audit result
    Given contract "Complete Report Contract" is submitted, reviewed, approved, and signed via the standard workflow
    When the Auditor exports scope "archive" for that contract as "<format>" with justification "external report BDD-4720"
    Then the "<format>" report contains timestamped audit results
    And the "<format>" report contains archive findings with rule references and results
    And the exact delivered report bytes have a recorded SHA-256 hash and IPFS CID

    Examples:
      | format |
      | json   |
      | csv    |
      | pdf    |

  @REQ-audit-ui-archive-orce-AC7 @DCS-FR-CSA-18 @DCS-FR-CSA-24 @DCS-FR-PACM-04 @DCS-FR-UC-07-2
  Scenario: Audit and export access records the actor and mandatory purpose
    When a Contract Manager runs scope "contracts" with justification "unauthorized audit BDD-4721"
    Then the request is denied with an authorization error
    And the denied audit access is logged with actor, roles, time, scope, and justification
    When the Auditor runs scope "contracts" with justification "authorized audit BDD-4722"
    Then the process audit request is accepted
    And the audit action is logged with actor, roles, time, scope, and justification
    When a Contract Manager exports scope "contracts" as "json" with justification "unauthorized report BDD-4722"
    Then the request is denied with an authorization error
    And the denied report access is logged with actor, roles, time, scope, and justification
    When the Auditor exports scope "contracts" as "json" with justification "authorized report BDD-4722"
    Then the report request is accepted
    And the report action is logged with actor, roles, time, scope, and justification

  @REQ-audit-ui-archive-orce-AC8 @DCS-IR-CSA-04 @DCS-FR-CSA-19 @DCS-FR-CSA-24
  Scenario: One damaged archive entry does not hide a valid entry and both audit routes agree
    Given contract "Healthy Archive Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And contract "Damaged Archive Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And its archived evidence is corrupted as "content hash"
    When the Auditor runs scope "archive" with justification "mixed archive BDD-4723"
    Then the archive audit contains a passed summary for "Healthy Archive Contract"
    And the archive audit contains a failed finding for "Damaged Archive Contract"
    And the PAC archive audit and archive audit endpoint return the same integrity findings
