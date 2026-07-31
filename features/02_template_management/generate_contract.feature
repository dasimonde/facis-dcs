@UC-02-03
Feature: Generate Contract from Template
  Contract Creators generate contract instances
  by populating approved templates with data.
  
  # contract/create only accepts templates in state REGISTERED or PUBLISHED
  # (backend ReadContractTemplateDataByID) — APPROVED alone is not enough.
  @DCS-IR-TR-06
  Scenario: Generate contract from approved template
    Given I am authenticated with roles: "Contract Creator"
    And template "Standard NDA" is in "Registered" status
    When I generate a contract from template "Standard NDA"
    Then a contract is created

  @REQ-remove-document-number-AC2 @DCS-FR-CWE-13 @DCS-FR-CSA-09
  Scenario: Generated contracts contain no legacy document number
    Given a registered template with a legacy document number exists
    When I generate a contract from that legacy template
    Then the contract keeps its template reference without a document number

  Scenario: Unauthorized role cannot generate contract
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Registered" status
    When I generate a contract from template "Standard NDA"
    Then the request is denied with an authorization error

  # DCS-NFR-BR-04: every contract MUST originate from an approved, versioned
  # template — contract/create resolves the template DID against REGISTERED/
  # PUBLISHED rows only (backend ReadContractTemplateDataByID), so a template
  # still in Draft is not usable for contract creation.
  @DCS-FR-TR-07 @DCS-NFR-BR-04
  Scenario: Contract creation from an unapproved draft template is denied
    Given I am authenticated with roles: "Contract Creator"
    And template "Unapproved Draft NDA" is in "Draft" status
    When I generate a contract from template "Unapproved Draft NDA"
    Then contract creation from the unapproved template is denied
