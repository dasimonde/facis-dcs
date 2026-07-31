# External publication of the public Merkle-checkpoint projection turns the
# local audit chain into evidence held outside the DCS operator's control.

@DCS-FR-PACM-02 @DCS-FR-PACM-03
Feature: Sequential external audit-checkpoint sink

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC1 @DCS-FR-PACM-02
  Scenario: Every new checkpoint is posted in sequence to the configured authenticated HTTPS sink
    Given the external checkpoint sink is reset and accepts checkpoints
    When two new audit checkpoints are created and the ORCE anchor worker runs
    Then the checkpoint sink has observed every sequence in order without a gap
    And each checkpoint attempt uses the configured sink path, timeout, and authentication

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC2 @DCS-FR-PACM-02
  Scenario: The external checkpoint payload is the public allowlist and nothing else
    Given the external checkpoint sink is reset and accepts checkpoints
    When a new audit checkpoint is created and the ORCE anchor worker runs
    Then the checkpoint sink receives only "seq,root,prev_root,leaf_count,created_at,tsa_timestamp,timestamped_at"
    And the checkpoint sink receives no contract data, CID, nonce, or identity

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC3 @DCS-FR-PACM-02
  Scenario: A lost response and worker restart cannot append a checkpoint twice
    Given the external checkpoint sink records the request and loses its response
    When a new audit checkpoint is created and the ORCE anchor worker runs
    And the ORCE anchor worker restarts with its persisted confirmation state
    And the external checkpoint sink accepts checkpoints
    And the ORCE anchor worker runs again
    Then both attempts carry the same checkpoint idempotency key
    And the external checkpoint sink contains one append for that sequence and root
    And the ORCE anchor worker confirms that sequence only after the successful response

  @REQ-external-checkpoint-and-semantic-workflow-gates-AC3 @DCS-FR-PACM-02
  Scenario Outline: A broken external chain stops later publication observably
    Given the external checkpoint sink is reset and rejects the next checkpoint with "<fault>"
    When a new audit checkpoint is created and the ORCE anchor worker runs
    Then the ORCE anchor worker reports a blocked external chain with "<fault>"
    When another audit checkpoint is created and the ORCE anchor worker runs
    Then no checkpoint after the blocked sequence is attempted

    Examples:
      | fault              |
      | sequence gap       |
      | previous-root mismatch |
