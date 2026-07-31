# Contract lifecycle termination and renewal (UC-06-02, POST /contract/
# terminate + POST /contract/renew, backend/design/contract_workflow_engine.go).
# KPI monitoring for ACTIVE contracts (UC-06-01) is covered by
# 05_contract_deployment/contract_deployment.feature — not duplicated here.
#
# Renewal (DCS-FR-CWE-11/22, DCS-FR-CSA-15) creates a NEW, independently
# versioned contract instance rather than mutating the original's expiry date
# in place: SRS DCS-FR-CWE-11 ("Renewals MUST generate a new contract
# instance with reference links") and DCS-FR-CSA-15 ("creation of renewal or
# extension contracts linked to archived originals... retain references to
# the prior contract's version, ID, and signatures") both describe a linked
# sibling document, not an edit of the original. The original contract is
# left completely intact; the new instance starts in DRAFT and carries a
# dcs:renewsContract JSON-LD back-reference to the original's DID and
# version (see backend/internal/contractworkflowengine/command/renew.go).
# The scenarios below assert that honest model, not an in-place expiry-date
# mutation.

@UC-06-02 @DCS-FR-CWE-11 @DCS-FR-CWE-12 @DCS-FR-UC-06-2
Feature: Contract termination

  @UC-06-02 @DCS-FR-CWE-23 @DCS-IR-CWE-12
  Scenario: Contract Manager terminates an approved contract
    Given contract "Termination Contract" has reached contract state "APPROVED"
    When the contract manager terminates contract "Termination Contract" with reason "BDD termination test"
    Then get http 200:Success code
    And the contract "Termination Contract" is in state "TERMINATED"
    And the contract "Termination Contract" has an audit event of type "TERMINATE_CONTRACT"

  @UC-06-02 @DCS-FR-CWE-23
  Scenario: A terminated contract cannot be terminated again
    Given contract "Double Termination Contract" has reached contract state "TERMINATED"
    When the contract manager terminates contract "Double Termination Contract" with reason "second attempt"
    Then the request is denied with a client error

  # DCS-FR-CSA-16: the termination record (reason, initiating identity and
  # role, effective timestamp) lives on the TERMINATE_CONTRACT audit event —
  # see TerminateEvent's reason/terminated_by/occurred_at/user_roles JSON tags
  # in backend/internal/contractworkflowengine/event/event.go. terminated_by
  # is the caller's participant DID (middleware.GetParticipantID).
  @UC-06-02 @DCS-FR-CSA-16
  Scenario: Termination records reason, initiating identity and role, and timestamp
    Given contract "Termination Evidence Contract" has reached contract state "APPROVED"
    When the contract manager terminates contract "Termination Evidence Contract" with reason "Budget for the engagement was withdrawn"
    Then get http 200:Success code
    And the contract "Termination Evidence Contract" is in state "TERMINATED"
    And the TERMINATE_CONTRACT audit event for contract "Termination Evidence Contract" records reason "Budget for the engagement was withdrawn", the terminating identity and role, and an effective timestamp

  # DCS-FR-CSA-16: TERMINATED has no outgoing update/negotiate edges in the
  # transition table (backend/internal/contractworkflowengine/datatype/
  # contractstate/transition.go), so mutations are rejected while retrieval
  # keeps working — the final state assertion below is itself a successful
  # authenticated retrieve of the terminated contract.
  @UC-06-02 @DCS-FR-CSA-16
  Scenario: A terminated contract is read-only: mutations are rejected but retrieval still works
    Given contract "Read-Only Terminated Contract" has reached contract state "TERMINATED"
    When the initiator attempts to update terminated contract "Read-Only Terminated Contract"
    Then the request is denied with a client error
    When the initiator attempts to negotiate a change on terminated contract "Read-Only Terminated Contract"
    Then the request is denied with a client error
    And the contract "Read-Only Terminated Contract" is in state "TERMINATED"

  @UC-06-02 @DCS-FR-CWE-11 @DCS-FR-CWE-22
  Scenario: Contract Manager renews a contract before its expiry notice period
    Given contract "Renewal Contract" with a contract term has reached contract state "SIGNED"
    And contract "Renewal Contract" is force-set to state "ACTIVE" directly in the database (pre-deploy test seam, bypassing the deployment chain)
    When the contract manager renews contract "Renewal Contract" for a new term
    Then get http 200:Success code
    And the renewal of "Renewal Contract" is a new contract in state "DRAFT"
    And the renewal of "Renewal Contract" has its own term dates
    And the contract "Renewal Contract" is in state "ACTIVE"

  @DCS-FR-CSA-15 @DCS-FR-CWE-22 @UC-06-02
  Scenario: Renewal contract references the original contract's DID and version
    Given contract "Renewal Source Contract" has reached contract state "SIGNED"
    And contract "Renewal Source Contract" is force-set to state "ACTIVE" directly in the database (pre-deploy test seam, bypassing the deployment chain)
    When the contract manager renews contract "Renewal Source Contract" for a new term
    Then get http 200:Success code
    And the renewal of "Renewal Source Contract" references the original contract's DID and version
