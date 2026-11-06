# DCS-NFR-SF-01 Reset Possibility: the DCS is reset (its deployment restarted,
# so a new pod replaces the old one) in the middle of a contract workflow.
# The contract created before the reset is readable after it and continues
# its workflow on the new pod without any recovery step.

@NFR-SF-01 @DCS-NFR-SF-01
Feature: DCS restart

  @clean_db
  Scenario: A contract created before a DCS restart is read and continued after it
    Given contract "Restart Survivor Contract" has reached contract state "DRAFT"
    When the DCS deployment restarts
    Then the contract "Restart Survivor Contract" is in state "DRAFT"
    When the creator submits contract "Restart Survivor Contract" for review after the restart
    Then the contract "Restart Survivor Contract" is in state "SUBMITTED"

  # A hard kill (no grace period) right after the submit leaves its outbox
  # events unpublished and unanchored; the new pod delivers them.
  @clean_db
  Scenario: Work in flight when the DCS pod is killed continues on the new pod
    Given contract "Killed Pod Contract" has reached contract state "DRAFT"
    When the creator submits contract "Killed Pod Contract" for review and the DCS pod is killed at once
    Then the contract "Killed Pod Contract" is in state "SUBMITTED"
    And every outbox event of contract "Killed Pod Contract" is published and anchored
