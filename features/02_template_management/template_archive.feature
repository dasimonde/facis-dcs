@UC-02-05 @DCS-FR-UC-02-1
Feature: Archive Contract Templates
  Template Managers deprecate outdated templates to prevent new contract generation
  and delete deprecated templates that are no longer needed.

  @clean_db @DCS-FR-TR-17
  Scenario: Deprecate an active template
    Given I am authenticated with roles: "Template Manager"
    And template "Old NDA" is in "Registered" status
    When I deprecate template "Old NDA"
    Then the template status is "Deprecated"
    And new contracts cannot be generated from this template

  @clean_db
  Scenario: Unauthorized role cannot deprecate template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Old NDA" is in "Approved" status
    When I deprecate template "Old NDA"
    Then the request is denied with an authorization error

  @clean_db @DCS-IR-TR-07
  Scenario: A template in review is not deleted
    Given I am authenticated with roles: "Template Manager"
    And template "Old NDA" is in "Reviewed" status
    When I delete template "Old NDA"
    Then the request is denied with a client error
    And template "Old NDA" is still in "Reviewed" status

  @clean_db @DCS-FR-TR-18
  Scenario: Delete a draft template
    Given I am authenticated with roles: "Template Manager"
    And template "Draft NDA" is in "Draft" status
    When I delete template "Draft NDA"
    Then the template status is "Deleted"

  @clean_db @DCS-FR-TR-18
  Scenario: Delete deprecated template
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" is in "Deprecated" status
    When I delete template "Standard NDA"
    Then the template status is "Deleted"

  @clean_db
  Scenario: Unauthorized role cannot delete template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Old NDA" is in "Submitted" status
    When I delete template "Old NDA"
    Then the request is denied with an authorization error
