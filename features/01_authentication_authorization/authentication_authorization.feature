@FR-UC-01-1 @FR-UC-01-3 @FR-UC-01-4
Feature: User Authentication & Authorization
  Users authenticate securely and are authorized based on roles and credentials.

  @clean_db
  Scenario: Authorization denied for expired credential
    Given I hold an expired credential with roles: "Template Creator"
    When I try to create a template "Standard NDA" in category "Legal"
    Then the request is denied because of credential expiration

  @clean_db
  Scenario: Role enforcement prevents unauthorized actions
    Given I am authenticated with roles: "Contract Creator"
    When I try to create a template "Standard NDA" in category "Legal"
    Then the request is denied with an authorization error

  @clean_db
  Scenario: Multiple failed credential attempts trigger account lockout
    Given I hold an expired credential with roles: "Template Creator"
    When I try to search for templates with name "Standard NDA" "20"
    Then the request is denied because of too many failed attempts

  @clean_db
  Scenario: Locked account cannot authenticate even with valid credential
    Given I hold an expired credential with roles: "Template Creator"
    And I try to search for templates with name "Standard NDA" "20"
    And the request is denied because of too many failed attempts
    When I am authenticated with roles: "Contract Creator"
    Then the request is denied because of too many failed attempts