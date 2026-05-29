# DCS-FR-TR-11
@UC-02-13
Feature: Template Identity and Traceability
  Templates are assigned unique identifiers
  for traceability across contract workflows.

  Scenario: Template receives UUID on creation
    Given I am authenticated with roles: "Template Creator"
    When I create a template "Standard NDA" in category "Legal"
    Then the template is assigned a UUID
    And the UUID is unique across the system

  Scenario: Retrieve template by DID
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" has a DID assigned
    When I retrieve template by DID
    Then I receive the correct template

