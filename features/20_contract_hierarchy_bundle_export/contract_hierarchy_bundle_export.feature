# Contract hierarchy (frame agreements) and ZIP bundle export.
#
# Hierarchy invariant (FR-TR-02, FR-CWE-02): a child contract links to its
# parent via a single dcs:parentContract reference — never the reverse.
# Documents enumerating children, multiple parent references, and parent
# cycles are rejected; the full-scope view is a parent_did search filter
# over children the instance legitimately holds.
#
# Bundle export (FR-TR-24, FR-CWE-30, FR-PACM-06): everything the instance
# holds for a contract/template is exported as one ZIP with a
# bundle-manifest.json integrity index; exports with missing referenced
# components are refused with a findings list. Beyond the parent chain
# (parents/), other locally-known, requester-readable members of the
# hierarchy family are packaged under related/ — see
# features/23_contract_bundle_export for the family-completeness scenarios.
#
# Scope notes:
#   - "parent must be a frame-capable type" is not asserted by any scenario.
#   - The 3-party sibling isolation is simulated with 2 physical instances:
#     a second child is linked to the frame ONLY on instance A and never
#     offered to instance B; the assertion is that B's parent_did-filtered
#     search never surfaces that child's DID.
#   - credentials/ in the contract bundle is extracted from the exported
#     PDF's C2PA manifest chain (the same source as manifest-store.c2pa);
#     signing-summary VCs are not part of the bundle's credentials/ folder
#     and are not asserted here.
#   - deployment/ in the contract bundle stays empty/optional (no
#     contract_deployments source exists).
#
# Hierarchy step definitions: steps/template_management/
# dcs_contract_hierarchy_steps.py. Bundle export: steps/pdf_generation/
# dcs_bundle_export_steps.py (that module imports the former's
# `_minimal_canonical_contract_data`/`_link_contract_to_parent` helpers to
# build parent/sibling fixtures instead of duplicating them). The related/
# family assertions live in steps/contract_bundle_export_steps.py.

@DCS-OR-H
Feature: Contract hierarchy invariant and ZIP bundle export

  Background:
    Given I am authenticated with roles: "Contract Manager"

  # ---------------------------------------------------------------------
  # Hierarchy invariant
  # ---------------------------------------------------------------------

  @DCS-FR-TR-02 @DCS-FR-CWE-02
  Scenario: A contract document with more than one dcs:parentContract reference is rejected
    Given contract "Multi-Parent Contract" exists with no parent reference
    When contract "Multi-Parent Contract" is updated with two dcs:parentContract references
    Then the request is denied with a client error

  @DCS-FR-TR-02 @DCS-FR-CWE-02
  Scenario: A contract document with a child-enumerating property is rejected
    Given contract "Child Enumerating Contract" exists with no parent reference
    When contract "Child Enumerating Contract" is updated with a child-enumerating dcs:childContracts property
    Then the request is denied with a client error

  @DCS-FR-TR-02 @DCS-FR-CWE-02
  Scenario: A contract whose locally resolvable parent chain contains a cycle is rejected
    Given contracts "Cycle A" and "Cycle B" exist locally with no parent reference
    When contract "Cycle B" is updated to reference contract "Cycle A" as its parent
    Then get http 200:Success code
    When contract "Cycle A" is updated to reference contract "Cycle B" as its parent
    Then the request is denied with a client error

  @DCS-FR-CWE-29 @DCS-FR-CSA-05
  Scenario: contract search accepts a parent_did filter and returns only contracts referencing it
    Given contract "Filter Frame Contract" exists with no parent reference
    And contract "Filter Linked Child" references contract "Filter Frame Contract" as its parent
    And contract "Filter Unrelated Contract" exists with no parent reference
    When the contract search is queried with parent_did filter for contract "Filter Frame Contract"
    Then get http 200:Success code
    And the search results include contract "Filter Linked Child"
    And the search results do not include contract "Filter Unrelated Contract"

  @DCS-FR-CSA-26 @two-instance
  Scenario: Sibling isolation — instance B never sees a child kept local to instance A
    Given instance A and instance B are both running and trust each other
    When child contracts are created on instance A: one linked to a frame and offered to instance B, another linked to the same frame and kept local to instance A only
    Then instance B's parent_did-filtered search for the frame includes the offered child but never the sibling that stayed local to instance A

  @DCS-FR-CWE-29 @DCS-FR-CSA-05
  Scenario: Frame contract detail query shows linked child contracts with their states
    Given contract "Detail Frame Contract" exists with no parent reference
    And contract "Detail Linked Draft Child" references contract "Detail Frame Contract" as its parent, then reaches contract state "DRAFT"
    And contract "Detail Linked Negotiating Child" references contract "Detail Frame Contract" as its parent, then reaches contract state "NEGOTIATION"
    When the contract search is queried with parent_did filter for contract "Detail Frame Contract"
    Then get http 200:Success code
    And the search results for contract "Detail Frame Contract" show contract "Detail Linked Draft Child" with state "DRAFT"
    And the search results for contract "Detail Frame Contract" show contract "Detail Linked Negotiating Child" with state "NEGOTIATION"

  # ---------------------------------------------------------------------
  # ZIP bundle export
  # ---------------------------------------------------------------------

  @DCS-FR-CWE-30
  Scenario: Contract bundle export returns a ZIP with all required members
    Given contract "Bundle Export Contract" has reached contract state "SIGNED"
    And contract "Bundle Export Contract" has an exported PDF
    When I request the contract bundle export for "Bundle Export Contract"
    Then get http 200:Success code
    And the response has Content-Type "application/zip"
    And the contract bundle ZIP for "Bundle Export Contract" contains entries: contract.jsonld, contract.pdf, manifest-store.c2pa, credentials/, signatures.json, bundle-manifest.json

  @DCS-FR-CWE-30 @DCS-FR-CSA-05
  Scenario: Child contract bundle carries the parent chain upward and its locally-known sibling under related/
    # All three contracts are deliberately kept in DRAFT (not advanced to
    # SIGNED): contract/update — used to establish dcs:parentContract links
    # via steps/template_management/dcs_contract_hierarchy_steps.py's
    # _link_contract_to_parent — only accepts EventUpdate from Draft
    # (transition.go's Transitions[Draft][EventUpdate]), and PDF export/
    # verify does not require any particular contract state (see
    # features/19_c2pa_conformance's Draft banner scenario).
    # The sibling is created on THIS instance by the same organization, so
    # it is locally known and requester-readable — the FR-CWE-30 family rule
    # includes it under related/. Members held only by other instances (or
    # outside the requester's read authorization) stay absent; see ADR-7 and
    # features/23_contract_bundle_export.
    Given contract "Hierarchy Bundle Frame" exists with no parent reference
    And contract "Hierarchy Bundle Frame" has an exported PDF
    And contract "Hierarchy Bundle Child" and contract "Hierarchy Bundle Sibling" both reference contract "Hierarchy Bundle Frame" as their parent
    And contract "Hierarchy Bundle Child" has an exported PDF
    And contract "Hierarchy Bundle Sibling" has an exported PDF
    When I request the contract bundle export for "Hierarchy Bundle Child"
    Then get http 200:Success code
    And the contract bundle ZIP for "Hierarchy Bundle Child" contains the parent chain for "Hierarchy Bundle Frame"
    And the contract bundle ZIP for "Hierarchy Bundle Child" contains family member "Hierarchy Bundle Sibling" under related/

  @DCS-FR-CWE-30
  Scenario: Every bundle-manifest.json entry's SHA-256 matches the packaged bytes
    Given contract "Hash Integrity Bundle Contract" has reached contract state "SIGNED"
    And contract "Hash Integrity Bundle Contract" has an exported PDF
    When I request the contract bundle export for "Hash Integrity Bundle Contract"
    Then get http 200:Success code
    And every entry in the bundle-manifest.json for "Hash Integrity Bundle Contract" has a SHA-256 matching the packaged bytes

  @DCS-FR-TR-26 @DCS-FR-PACM-06
  Scenario: Export is refused with a findings list when a referenced component is missing
    Given contract "Incomplete Bundle Contract" exists with no exported PDF
    When I request the contract bundle export for "Incomplete Bundle Contract"
    Then the contract bundle export for "Incomplete Bundle Contract" is refused with a findings list

  @DCS-FR-TR-24 @DCS-FR-TR-09
  Scenario: Template bundle export returns a ZIP with flat template artifacts only
    Given an approved template "Bundle Export Template" is available for bundle export
    When I request the template bundle export for "Bundle Export Template"
    Then get http 200:Success code
    And the response has Content-Type "application/zip"
    And the template bundle ZIP for "Bundle Export Template" contains no frame/parent chain directory

  @DCS-FR-CSA-18
  Scenario: Contract bundle export honors RBAC like retrieval
    Given contract "RBAC Bundle Contract" has reached contract state "SIGNED"
    And contract "RBAC Bundle Contract" has an exported PDF
    When I request the contract bundle export for "RBAC Bundle Contract" with an unauthorized role
    Then the request is denied with an authorization error

  @DCS-FR-CSA-18
  Scenario: Contract bundle export creates an audit log entry
    Given contract "Audited Bundle Contract" has reached contract state "SIGNED"
    And contract "Audited Bundle Contract" has an exported PDF
    When I request the contract bundle export for "Audited Bundle Contract"
    Then get http 200:Success code
    And the contract "Audited Bundle Contract" has an audit event of type "EXPORT"
