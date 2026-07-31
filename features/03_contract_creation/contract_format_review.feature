@UC-03-05 @FR-CWE-04
Feature: Machine-Readable and Human-Readable Contract Review
  Contract Creators, Contract Reviewers, and Contract Managers review
  contracts in both machine-readable and human-readable formats. The system
  ensures synchronization and highlights any inconsistencies.

  @clean_db
  Scenario: View contract in machine-readable format
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" exists
    When I view contract "Service Agreement" in machine-readable format
    Then the JSON-LD or XML representation is displayed
    And the structure is valid

  @clean_db
  Scenario: View contract in human-readable format
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" exists
    When I view contract "Service Agreement" in human-readable format
    Then the PDF or document view is displayed
    And the content is readable

  @clean_db @DCS-FR-CSA-06
  Scenario: Synchronized view of both formats
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" exists
    When I request synchronized view of contract "Service Agreement"
    Then both machine-readable and human-readable views are rendered
    And both formats are derived from the same source
    And both formats have matching content hashes

  # Real MR/HR inconsistency induced via the IPFS CID-swap seam
  # (steps/support/tamper_seam.py, steps/pdf_generation/
  # contract_format_review_tamper_steps.py) — the same seam used by
  # "Tampered PDF fails hash verification" below. The verify endpoint only
  # checks the STORED PDF's internal self-consistency (embedded JSON-LD vs
  # its own recompiled base layer), so the discrepancy is genuinely induced
  # server-side, not merely asserted client-side.
  Scenario: System highlights inconsistencies between formats
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" has a formatting error
    When I review both formats of contract "Service Agreement"
    Then the system highlights inconsistencies
    And the specific discrepancies are identified

  # "version 2.0" is adapted to this system's real versioning primitive:
  # contracts carry an integer contract_version (bumped by the negotiation
  # merge in submit.go), not a semver-style "X.Y" string — the Given below
  # drives one full negotiate -> accept -> submit/merge round to reach
  # contract_version 2, and "version tag \"2.0\"" is checked against that
  # integer.
  @clean_db
  Scenario: Export both formats with same version tag
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" with version "2.0" exists
    When I export contract "Service Agreement" in both formats
    Then the machine-readable export has version tag "2.0"
    And the human-readable export has version tag "2.0"
    And both exports are consistent

  @clean_db
  Scenario: Validate machine-readable structure
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" has machine-readable representation
    When I validate the machine-readable structure
    Then the schema validation passes
    And required fields are present
    And data types are correct

  # "Fixing" a content-addressed corruption means re-pointing the stored
  # artifact back at known-good content — IPFS is content-addressed, so the
  # bad bytes at the tampered CID can never be edited in place (see
  # tamper_seam.py). "I fix the inconsistency" therefore restores the
  # original, self-consistent CID captured when the inconsistency was
  # induced (real remediation for this failure class, not a re-export —
  # see contract_format_review_tamper_steps.py for why a plain re-export
  # would just re-serve the cached/tampered bytes here).
  Scenario: Fix inconsistency and re-validate
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a detected inconsistency
    When I fix the inconsistency
    And I re-validate contract "Service Agreement"
    Then no inconsistencies are highlighted
    And both formats are synchronized

  # "Contract Observer" is (deliberately, per design/pdf_generation.go
  # export_contract_pdf/verify_contract_pdf Security scopes) an AUTHORIZED
  # role for format-review endpoints, so it cannot stand in for "unauthorized"
  # here — "Contract Negotiator" is the role actually excluded from both
  # endpoints' scope lists and is used instead.
  @clean_db
  Scenario: Unauthorized role cannot access format review
    Given I am authenticated with roles: "Contract Negotiator"
    And contract "Service Agreement" exists
    When I attempt to access the synchronized view of contract "Service Agreement"
    Then the request is denied with an authorization error

  @DCS-FR-CWE-04 @DCS-FR-CSA-06 @DCS-FR-TR-04 @DCS-IR-CI-02
  Scenario: Export contract as PDF and verify MR/HR hash match
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" exists in "Under Review" state
    When I export contract "Service Agreement" as PDF
    Then the response is a valid PDF document
    And the PDF contains an embedded JSON-LD attachment named "contract.jsonld"
    And the embedded JSON-LD matches the contract source

  @DCS-FR-CWE-04 @DCS-FR-CWE-05 @DCS-FR-CSA-06 @DCS-FR-TR-04
  Scenario: Verify MR/HR content hash consistency
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in "Draft" status
    And contract "Service Agreement" has an exported PDF
    When I verify the MR/HR hash consistency for contract "Service Agreement"
    Then the verification result shows match is true
    And the response includes jsonld_hash and base_pdf_hash

  @DCS-FR-CWE-04 @DCS-FR-CSA-01
  Scenario: Tampered PDF fails hash verification
    # Real integration-level tampering detection: the tampered PDF is
    # injected into the shared in-cluster IPFS node and the contract's
    # pdf_ipfs_cid row is repointed at it (steps/support/tamper_seam.py,
    # `contract "..." has an exported PDF with a tampered base layer` in
    # steps/pdf_generation/pdf_steps.py) — a genuine black-box exercise of
    # GET /pdf/verify/contract/{did} against server-stored tampered bytes,
    # not just the Go unit tests in verify/verifier_test.go.
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in "Draft" status
    And contract "Service Agreement" has an exported PDF with a tampered base layer
    When I verify the MR/HR hash consistency for contract "Service Agreement"
    Then the verification result shows match is false
