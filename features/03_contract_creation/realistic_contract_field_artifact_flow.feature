@UC-02 @UC-02-01 @UC-02-03 @UC-03-01
@DCS-FR-TR-01 @DCS-FR-TR-04 @DCS-FR-TR-12 @DCS-FR-TR-13 @DCS-FR-TR-14 @DCS-FR-TR-15
@DCS-FR-CWE-03 @DCS-FR-CWE-13 @DCS-PC-06 @DCS-NFR-BR-04 @DCS-NFR-SEC-07
Feature: Realistic contract field artifact flow
  A realistic service agreement remains structurally and semantically intact
  from its reviewed template through the contract derived from that template.

  @clean_db
  @REQ-realistic-contract-field-artifact-flow-AC1
  @REQ-realistic-contract-field-artifact-flow-AC2
  @REQ-realistic-contract-field-artifact-flow-AC3
  @REQ-realistic-contract-field-artifact-flow-AC4
  @REQ-realistic-contract-field-artifact-flow-AC5
  Scenario: Registered realistic template produces traceable JSON-LD artifacts
    Given a realistic service agreement template with contract fields is prepared
    When the Template Creator creates the realistic service agreement template
    And the Template Creator submits the realistic template for review
    And the Template Reviewer verifies the realistic template
    And the Template Reviewer forwards the realistic template for approval
    And the Template Approver approves the realistic template
    And the Template Manager registers the realistic template
    And the Template Manager retrieves the registered realistic template
    And the Contract Creator creates a contract from that registered template DID
    And the Contract Creator retrieves the realistic contract
    Then the retrieved realistic template is registered
    And both retrieved documents contain the complete ContractField model
    And both retrieved documents retain the section and indented clause structure
    And the contract records its template provenance and rebases internal identifiers
    And neither retrieved document contains a dcs Placeholder node
    And the retrieved template and contract are saved as pretty JSON-LD artifacts
