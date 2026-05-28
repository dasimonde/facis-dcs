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

  # FR-UC-01-4: Multiple invalid credential attempts trigger lockout
  @clean_db
  Scenario: Multiple failed credential attempts trigger account lockout
    Given I hold an expired credential with roles: "Template Creator"
    When I try to search for templates with name "Standard NDA" "5"
    Then the request is denied because of too many failed attempts

  @skip
  Scenario: Locked account cannot authenticate even with valid credential
    Given my account has been locked due to failed attempts
    When I hold a valid verifiable credential
    And I attempt to access the DCS system
    Then the request is denied with error "Account locked"
    And I am instructed to contact an administrator