@UC-02-05
@skip
Feature: Deprecate Contract Template
  Template Managers mark outdated templates as deprecated
  to prevent new contract generation.

  Background:
    Given I am authenticated with role "Template Manager"

  Scenario: Deprecate an active template
    And template "Old NDA" is in "Approved" status
    When I deprecate template "Old NDA"
    Then the template status is "Deprecated"
    And new contracts cannot be generated from this template

  Scenario: Unauthorized role cannot deprecate template
    Given I am authenticated with role "Template Reviewer"
    And template "Old NDA" is in "Approved" status
    When I attempt to deprecate template "Old NDA"
    Then the request is denied with an authorization error
