@UC-02-05
Feature: Archive Contract Templates
  Template Managers deprecate outdated templates to prevent new contract generation
  and delete deprecated templates that are no longer needed.

  @clean_db
  Scenario: Deprecate an active template
    Given I am authenticated with roles: "Template Manager"
    And template "Old NDA" is in "Approved" status
    When I deprecate template "Old NDA"
    Then the template status is "Deprecated"
    And new contracts cannot be generated from this template

  @clean_db
  Scenario: Unauthorized role cannot deprecate template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Old NDA" is in "Approved" status
    When I deprecate template "Old NDA"
    Then the request is denied with an authorization error

  @clean_db
  Scenario: Delete reviewed template
    Given I am authenticated with roles: "Template Manager"
    And template "Old NDA" is in "Reviewed" status
    When I delete template "Old NDA"
    Then the template status is "Deleted"

  @clean_db
  Scenario: Cannot delete deprecated template
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" is in "Deprecated" status
    When I delete template "Standard NDA"
    Then I receive error "invalid contract template state"

  @clean_db
  Scenario: Unauthorized role cannot delete template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Old NDA" is in "Submitted" status
    When I delete template "Old NDA"
    Then the request is denied with an authorization error