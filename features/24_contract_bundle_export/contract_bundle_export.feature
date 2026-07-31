# Hierarchy-family completeness of the contract bundle export
# (DCS-FR-CWE-30 "Contract Package Bundling", DCS-FR-TR-24 "Structural
# Export in Unified Format"): GET /contract/export/{did} bundles the
# requested contract, its locally-known parent chain (parents/<did>/, as
# asserted by features/20_contract_hierarchy_bundle_export), and EVERY other
# locally-known, requester-readable member of the hierarchy family — the
# topmost locally-known ancestor's descendants, e.g. siblings — under
# related/<did>/ with the same per-contract entry structure. Members held
# only by other instances, or outside the requester's party read
# authorization, are simply absent; nothing is fetched remotely and nothing
# fails because a family member is missing (see ADR-7).
#
# Fixtures reuse steps/template_management/dcs_contract_hierarchy_steps.py
# and steps/pdf_generation/ (contract creation, dcs:parentContract linking,
# PDF export, bundle request); the related/ assertions live in
# steps/contract_bundle_export_steps.py.

@DCS-OR-H
Feature: Contract bundle export covers the locally-known hierarchy family

  Background:
    Given I am authenticated with roles: "Contract Manager"

  @DCS-FR-CWE-30 @DCS-FR-TR-24
  Scenario: initiator exports a bundle containing the locally-known hierarchy
    Given contract "Family Export Frame" exists with no parent reference
    And contract "Family Export Frame" has an exported PDF
    And contract "Family Export Child" and contract "Family Export Sibling" both reference contract "Family Export Frame" as their parent
    And contract "Family Export Child" has an exported PDF
    And contract "Family Export Sibling" has an exported PDF
    When I request the contract bundle export for "Family Export Child"
    Then get http 200:Success code
    And the response has Content-Type "application/zip"
    And the contract bundle ZIP for "Family Export Child" contains the parent chain for "Family Export Frame"
    And the contract bundle ZIP for "Family Export Child" contains family member "Family Export Sibling" under related/
    And every entry in the bundle-manifest.json for "Family Export Child" has a SHA-256 matching the packaged bytes

  @DCS-FR-CWE-30 @DCS-FR-TR-24
  Scenario: exporting the root frame yields the same family members
    Given contract "Family Root Frame" exists with no parent reference
    And contract "Family Root Frame" has an exported PDF
    And contract "Family Root Child" and contract "Family Root Sibling" both reference contract "Family Root Frame" as their parent
    And contract "Family Root Child" has an exported PDF
    And contract "Family Root Sibling" has an exported PDF
    When I request the contract bundle export for "Family Root Frame"
    Then get http 200:Success code
    And the response has Content-Type "application/zip"
    And the contract bundle ZIP for "Family Root Frame" contains family member "Family Root Child" under related/
    And the contract bundle ZIP for "Family Root Frame" contains family member "Family Root Sibling" under related/
