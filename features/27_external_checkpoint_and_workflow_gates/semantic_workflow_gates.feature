# Every consequential contract transition is guarded by the local Semantic
# Hub evaluation and one correlated dispatch to the configured PAC executor.

@DCS-FR-TR-03 @DCS-FR-PACM-02 @DCS-FR-PACM-03
Feature: Semantic workflow gates

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC4
  Scenario Outline: Each consequential workflow transition evaluates locally and dispatches once
    Given contract "Gate <gate> Success" is ready for the "<gate>" gate
    And the workflow-gate executor is reset and returns a valid empty success
    When the "<gate>" gate is requested for contract "Gate <gate> Success"
    Then the "<gate>" transition succeeds for contract "Gate <gate> Success"
    And one correlated "<gate>" workflow-gate request was observed
    And that request carries the local Semantic Hub evaluation and immutable snapshot

    Examples:
      | gate       |
      | submission |
      | offer      |
      | approval   |
      | signature  |
      | deployment |

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC5
  Scenario: An ERROR or FAILED gate result blocks the transition
    Given contract "Failed Workflow Gate" is ready for the "submission" gate
    And the workflow-gate executor is reset and returns a valid FAILED result
    When the "submission" gate is requested for contract "Failed Workflow Gate"
    Then the workflow transition is blocked by the failed gate
    And contract "Failed Workflow Gate" remains in its pre-gate state
    And one correlated "submission" workflow-gate request was observed

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC5
  Scenario Outline: A WARNING or REVIEW result persists a PACM manual review before transition
    Given contract "Manual Review <gate> Gate" is ready for the "<gate>" gate
    And the workflow-gate executor is reset and returns a valid REVIEW result
    When the "<gate>" gate is requested for contract "Manual Review <gate> Gate"
    Then contract "Manual Review <gate> Gate" remains in its pre-gate state
    And one correlated "<gate>" workflow-gate request was observed
    And a pending PACM manual review is persisted for the correlated gate run
    When the Compliance Officer approves that gate run with justification "reviewed semantic warning"
    Then the PACM manual review records the decision and justification
    And the approved PACM review is append-only and records one successful continuation attempt
    And the "<gate>" transition succeeds for contract "Manual Review <gate> Gate"
    And the workflow-gate executor still has exactly one correlated "<gate>" request

    Examples:
      | gate       |
      | submission |
      | offer      |
      | approval   |
      | signature  |
      | deployment |

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC6 @DCS-FR-TR-03
  Scenario: New snapshots use the activated bundle while older snapshots remain pinned across rollback
    Given an immutable workflow snapshot "old" is created from the active shapes, libraries, and profile
    When the Template Manager activates new effective shapes, libraries, and profile versions
    And an immutable workflow snapshot "new" is created from the active shapes, libraries, and profile
    And the Template Manager rolls the effective Semantic Hub bundle back
    Then workflow snapshot "old" still resolves its original effective bundle
    And workflow snapshot "new" still resolves the newly activated effective bundle
    And a workflow snapshot created after rollback deterministically resolves the restored bundle

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC7 @DCS-FR-TR-03
  Scenario Outline: Unresolvable Semantic Hub assets fail closed without executor fallback
    Given contract "Hub Asset <fault>" is ready for the "submission" gate
    And its immutable workflow snapshot has a "<fault>" Semantic Hub asset
    And the workflow-gate executor observations are reset
    When the "submission" gate is requested for contract "Hub Asset <fault>"
    Then the workflow transition is blocked by Semantic Hub resolution
    And the workflow-gate executor has observed no request
    And no fallback shapes, library, or profile version is recorded

    Examples:
      | fault       |
      | missing     |
      | unknown     |
      | unavailable |

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC8
  Scenario Outline: Invalid or unavailable workflow-gate execution fails closed after one dispatch
    Given contract "Executor <fault>" is ready for the "submission" gate
    And the workflow-gate executor is reset and returns "<fault>"
    When the "submission" gate is requested for contract "Executor <fault>"
    Then the workflow transition is blocked by the workflow-gate executor
    And one correlated "submission" workflow-gate request was observed
    And no successful workflow-gate result is persisted

    Examples:
      | fault                 |
      | timeout               |
      | non-2xx response      |
      | malformed JSON        |
      | mismatched correlation |
      | invalid result        |

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC8
  Scenario: Concurrent claims for one immutable snapshot create one PostgreSQL run and one dispatch
    Given contract "Concurrent Workflow Gate Claim" is ready for the "submission" gate
    And the workflow-gate executor is reset and returns "timeout"
    When two concurrent clients request the "submission" gate with the same snapshot token for contract "Concurrent Workflow Gate Claim"
    Then both concurrent requests fail closed with the same workflow-gate run ID
    And contract "Concurrent Workflow Gate Claim" remains in its pre-gate state
    And one correlated "submission" workflow-gate request was observed
    And exactly one PostgreSQL workflow-gate run exists for that snapshot, gate, correlation, and run ID

  # A sequential second transition is a different state snapshot and therefore
  # not an at-most-once retry. Concurrent same-snapshot claim arbitration is a
  # unit-level persistence regression; this scenario proves the live boundary.
  @REQ-external-checkpoint-and-semantic-workflow-gates-AC8
  Scenario: A valid empty result is successful and persisted once for the snapshot and gate
    Given contract "Empty Successful Gate" is ready for the "submission" gate
    And the workflow-gate executor is reset and returns a valid empty success
    When the "submission" gate is requested for contract "Empty Successful Gate"
    Then the "submission" transition succeeds for contract "Empty Successful Gate"
    And one correlated "submission" workflow-gate request was observed
    And exactly one successful empty workflow-gate run is persisted for that snapshot, gate, and correlation
