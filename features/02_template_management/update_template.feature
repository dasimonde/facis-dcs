@UC-02-04 @DCS-FR-UC-02-1
Feature: Update Contract Template
  Template Creators update existing templates
  with full version history preserved.

  @clean_db
  Scenario: Update an existing template as creator
    Given I am authenticated with roles: "Template Creator"
    And template with name "Standard NDA" and description "Template Description" exists
    When I update template "Standard NDA" name to "Test Name"
    Then the result is a template with name "Test Name"

  @clean_db
  Scenario: Update an existing template as creator
    Given I am authenticated with roles: "Template Creator"
    And template with name "Standard NDA" and description "Template Description" exists
    When I update template "Standard NDA" description to "Test Description"
    Then the result is a template with description "Test Description"

  @clean_db @DCS-IR-TR-03
  Scenario: Update an existing template as reviewer
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status with name "Standard NDA" and description "Template Description"
    When I update template "Standard NDA" name to "Standard NDA"
    Then the result is a template with name "Standard NDA"

  @clean_db
  Scenario: Update an existing template as creator
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status with name "Standard NDA" and description "Template Description"
    When I update template "Standard NDA" description to "Test Description"
    Then the result is a template with description "Test Description"

  @clean_db
  Scenario: Unauthorized role cannot update template
    Given I am authenticated with roles: "Template Approver"
    And template with name "Standard NDA" and description "Template Description" exists
    When I update template "Standard NDA" description to "Test Description"
    Then the request is denied with an authorization error

  @clean_db @REQ-remove-free-template-state-selection-AC3 @DCS-IR-TR-07
  Scenario: Legacy manager state input cannot bypass the template workflow
    Given I am authenticated with roles: "Template Manager"
    And template "Managed Draft" is in "Draft" status
    When I update manager metadata for template "Managed Draft" with name "Managed Draft Updated" and legacy state "Approved"
    Then the result is a template with name "Managed Draft Updated"
    And template "Managed Draft" status remains "Draft"

  @clean_db @REQ-remove-free-template-state-selection-AC4 @DCS-IR-TR-07
  Scenario: Manager metadata updates preserve the template status
    Given I am authenticated with roles: "Template Manager"
    And template "Managed Submitted" is in "Submitted" status
    When I update manager metadata for template "Managed Submitted" with description "Manager maintained description"
    Then the result is a template with description "Manager maintained description"
    And template "Managed Submitted" status remains "Submitted"
