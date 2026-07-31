# Template catalogue integration (DCS-IR-SI-01, UC-02): POST /template/publish
# (backend/design/template_repository.go) pushes a REGISTERED template to the
# XFSC Federated Catalogue; GET /catalogue/template/retrieve and
# /catalogue/template/search (backend/design/template_catalogue_integration.go)
# read it back. The Federated Catalogue integration itself is already
# exercised indirectly (register-only path) by template_workflow.feature's
# "Register approved template" scenario; this file adds the publish/retrieve/
# search round-trip and its own RBAC scope, which that file does not cover.

@DCS-IR-SI-01 @DCS-NFR-BR-09 @UC-02
Feature: Template catalogue integration

  @clean_db @DCS-NFR-SQ-06
  Scenario: Template Manager publishes a registered template to the catalogue
    Given I am authenticated with roles: "Template Manager"
    And template "Catalogue Publish Template" is in "Registered" status
    When I publish template "Catalogue Publish Template"
    Then get http 200:Success code
    And the template status is "Published"

  @clean_db
  Scenario: A published template can be retrieved via the catalogue
    Given I am authenticated with roles: "Template Manager"
    And template "Catalogue Retrieve Template" is in "Registered" status
    And I publish template "Catalogue Retrieve Template"
    And I am authenticated with roles: "Contract Creator"
    When I retrieve the template catalogue
    Then get http 200:Success code
    And the catalogue result includes template "Catalogue Retrieve Template"

  @clean_db
  Scenario: A published template can be found via catalogue search
    Given I am authenticated with roles: "Template Manager"
    And template "Catalogue Search Template" is in "Registered" status
    And I publish template "Catalogue Search Template"
    And I am authenticated with roles: "Contract Creator"
    When I search the template catalogue by name "Catalogue Search Template"
    Then get http 200:Success code
    And the catalogue search result includes template "Catalogue Search Template"

  @clean_db
  Scenario: A role outside the catalogue scope cannot publish a template
    Given I am authenticated with roles: "Template Manager"
    And template "Unauthorized Catalogue Publish Template" is in "Registered" status
    And I am authenticated with roles: "Template Creator"
    When I attempt to publish template "Unauthorized Catalogue Publish Template" with my current role
    Then the request is denied with a client error
