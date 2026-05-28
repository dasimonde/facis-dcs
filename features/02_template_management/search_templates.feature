@UC-02-02
Feature: Search and Retrieve Contract Templates
  Users search and access existing contract templates
  filtered by role-based access rights.

  @clean_db
  Scenario: Search templates by name
    Given I am authenticated with roles: "Template Manager"
    And template with name "Test template version 1A" and description "Test description 1" exists
    And template with name "Test template version 2A" and description "Test description 2" exists
    And template with name "Test template version 3A" and description "Test description 3" exists
    When I search for templates with name "2A"
    Then the result is one template where its name contains "2A"

  @clean_db
  Scenario: Search templates by description
    Given I am authenticated with roles: "Template Manager"
    And template with name "Test template version 1A" and description "Test description 1-1" exists
    And template with name "Test template version 2A" and description "Test description 2-2" exists
    And template with name "Test template version 3A" and description "Test description 3-3" exists
    When I search for templates with description "3-3"
    Then the result is one template where its description contains "3-3"

  @clean_db
  Scenario: Search in template details
    Given I am authenticated with roles: "Template Reviewer"
    And template with name "Test template version 1A" and template_data title "Test description 1-1" exists
    And template with name "Test template version 2A" and template_data title "Test description 2-2" exists
    And template with name "Test template version 3A" and template_data title "Test description 3-3" exists
    When I search for templates whats template_data contains keyword "2-2"
    Then the result is one template where its name contains "2A"
    And I see the template provenance

  Scenario: Retrieve unauthorized
    Given I am authenticated with roles: "Contract Creator"
    When I search for templates whats template_data contains keyword "2-2"
    Then the request is denied with an authorization error
