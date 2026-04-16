@UC-02-04
@skip
Feature: Update Contract Template
  Template Creators update existing templates
  with full version history preserved.

  Background:
    Given I am authenticated with role "Template Creator"

  Scenario: Update an existing template
    And template "Standard NDA" version "1.0" exists
    When I update template "Standard NDA"
    Then a new version "1.1" is created
    And the previous version remains accessible

  Scenario: Unauthorized role cannot update template
    Given I am authenticated with role "Template Reviewer"
    And template "Standard NDA" version "1.0" exists
    When I attempt to update template "Standard NDA"
    Then the request is denied with an authorization error
