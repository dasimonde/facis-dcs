@UC-08 @DCS-OR-C2PA
Feature: C2PA Lifecycle Assertions on Exported Contract PDFs
  When contracts and templates are exported as PDF, the system appends a
  C2PA lifecycle manifest as an incremental update.  Each manifest carries
  a lifecycle assertion that binds the contract identity, current state, and
  a hash of the JSON-LD source so that auditors can verify provenance without
  access to the original JSON-LD.

  Background:
    Given I am authenticated with roles: "Contract Manager"

  # DCS-OR-C2PA-001, DCS-OR-C2PA-003
  @DCS-OR-C2PA-001 @DCS-OR-C2PA-003
  Scenario: Exported PDF contains C2PA lifecycle assertion
    Given contract "Service Agreement" is in "Draft" status
    When I export contract "Service Agreement" as PDF
    Then the response is a valid PDF document
    And the PDF contains a C2PA manifest
    And the manifest lifecycle assertion includes field "contract_id"
    And the manifest lifecycle assertion includes field "file_hash"
    And the manifest contains a lifecycle assertion with field "status" equal to "draft"

  # DCS-OR-C2PA-003 — all required assertion fields present
  @DCS-OR-C2PA-003
  Scenario: C2PA lifecycle assertion carries all required fields
    Given contract "Service Agreement" is in "Draft" status
    When I export contract "Service Agreement" as PDF
    Then the PDF contains a C2PA manifest
    And the manifest lifecycle assertion includes field "contract_id"
    And the manifest lifecycle assertion includes field "file_hash"
    And the manifest lifecycle assertion includes field "status"
    And the manifest lifecycle assertion includes field "effective_at"
    And the manifest lifecycle assertion includes field "authority"

  # DCS-OR-C2PA-002, DCS-OR-C2PA-010 — incremental update does not disturb base layer
  @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: C2PA manifest is appended as incremental update preserving base PDF
    Given contract "Service Agreement" is in "Draft" status
    And contract "Service Agreement" has an exported PDF
    When I export contract "Service Agreement" as PDF
    Then the response is a valid PDF document
    And the PDF contains a C2PA manifest

  # DCS-OR-C2PA-003 — chain linkage between lifecycle events
  # The "transitions to state" step (steps/pdf_generation/pdf_steps.py)
  # drives a real Draft -> Under Review transition through the workflow API
  # (ContractService._prepare_contract_under_review) and polls the export
  # endpoint until the second C2PA assertion lands. At the pdf-core level,
  # pdf-core/features/lifecycle_assertion.feature:61 covers the same
  # prev_manifest_hash invariant.
  @DCS-OR-C2PA-003
  Scenario: C2PA manifest chain links successive lifecycle events
    Given contract "Service Agreement" has been exported in "Draft" state
    When contract "Service Agreement" transitions to "Under Review" state
    And I export contract "Service Agreement" as PDF
    Then the PDF contains two C2PA assertions
    And the second assertion's prev_manifest_hash matches the first assertion's hash

  # DCS-FR-CWE-04 — verify endpoint confirms MR/HR hash match after export
  @DCS-FR-CWE-04 @DCS-NFR-SEC-10
  Scenario: Verify endpoint confirms MR/HR hash match on freshly exported PDF
    Given contract "Service Agreement" is in "Draft" status
    And contract "Service Agreement" has an exported PDF
    When I verify the MR/HR hash consistency for contract "Service Agreement"
    Then the verification result shows match is true
    And the response includes jsonld_hash and base_pdf_hash
