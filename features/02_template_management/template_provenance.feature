# DCS-FR-TR-09 Template Provenance and Versioning: each registered template
# version carries provenance claims by its Creator, Reviewer, and Approver,
# sealed as a W3C Verifiable Credential in JSON-LD (issued at registration,
# backend/internal/templaterepository/command/provenance.go) and linked to
# the previous version's credential. Template users verify a template's
# provenance by retrieving and checking these credentials
# (GET /template/provenance/{did}).

@UC-02 @DCS-FR-TR-09
Feature: Per-version template provenance credentials

  @DCS-FR-TR-08
  Scenario: A registered template carries a signed provenance credential naming its actor trail
    Given I am authenticated with roles: "Template Manager"
    And template "Provenance Credential Template" is in "Registered" status
    When I retrieve the provenance credentials of template "Provenance Credential Template"
    Then get http 200:Success code
    And exactly one provenance credential is issued, sealing version 1 with no predecessor link
    And the provenance credential is a W3C VerifiableCredential in JSON-LD of type "TemplateProvenanceCredential"
    And the provenance credential names the template's creator, reviewer, approver, and registrar
    And the provenance credential binds the registered template content by its hash
    And the provenance credential carries a DataIntegrityProof issued by this instance

  Scenario: A role outside the template scope cannot retrieve provenance credentials
    Given template "Unauthorized Provenance Template" is in "Registered" status
    And I am authenticated with roles: "Contract Signer"
    When I attempt to retrieve the provenance credentials of template "Unauthorized Provenance Template"
    Then the request is denied with a client error
