@FR-UC-01-1 @FR-UC-01-3 @FR-UC-01-4
Feature: User Authentication & Authorization
  Users authenticate securely and are authorized based on roles and credentials.

  @clean_db @DCS-IR-CI-04 @DCS-IR-CI-05 @DCS-IR-SI-07 @DCS-NFR-BR-02
  Scenario: Authorization denied for expired credential
    Given I hold an expired credential with roles: "Template Creator"
    When I try to create a template "Standard NDA" in category "Legal"
    Then the request is denied because of credential expiration

  @clean_db @DCS-IR-SI-08
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

  @clean_db @DCS-NFR-SEC-16
  Scenario: Federated login through Hydra and the wallet issues a usable access token
    When I initiate a federated login
    And I bind the Hydra login challenge to the pending presentation
    And I present a wallet credential with roles: "Template Creator"
    And I complete the federated session and obtain an access token
    Then the access token authorizes an authenticated API call

  @clean_db @DCS-FR-UC-09-2
  Scenario: Successful authentication is logged with actor and timestamp
    When I initiate a federated login
    And I bind the Hydra login challenge to the pending presentation
    And I present a wallet credential with roles: "Template Creator"
    And I complete the federated session and obtain an access token
    Then the login presentation audit event records the actor and timestamp

  @clean_db @DCS-NFR-BR-01 @DCS-IR-SI-09
  Scenario: A login presentation with a revoked credential status is rejected
    # Client Auflage (2026-07-26): credential status and revocation are
    # checked at every login. The presentation is otherwise perfectly valid
    # — only its status-list index is revoked — isolating the status check.
    When I initiate a federated login
    And I bind the Hydra login challenge to the pending presentation
    And I present a wallet credential with roles "Template Creator" whose status-list index is revoked
    Then the login presentation is rejected for a revoked credential
