@UC-02-01 @DCS-FR-UC-02-1
Feature: Create Contract Template
  Template Creators create reusable contract templates
  that serve as the basis for contract generation.

  Background:
    Given I am authenticated with roles: "Template Creator"

  @REQ-remove-free-template-state-selection-AC2 @DCS-FR-TR-10
  Scenario: Create a new contract template
    When I create a template "Standard NDA" in category "Legal"
    Then the template status is "Draft"

  @REQ-remove-document-number-AC1 @DCS-FR-TR-10 @DCS-FR-TR-11 @DCS-IR-SI-01
  Scenario: Legacy document numbers are not part of templates
    When I create a template with a legacy document number
    Then the template is identified by DID and version without a document number

  @DCS-FR-TR-06
  Scenario: Unauthorized role cannot create template
    Given I am authenticated with roles: "Template Reviewer"
    When I attempt to create a template
    Then the request is denied with an authorization error
