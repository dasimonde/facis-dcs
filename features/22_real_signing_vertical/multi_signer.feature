# Multi-signer signing workflow (DCS-FR-SM-07, DCS-FR-SM-17, UC-03-06):
# contracts that declare multiple dcs:SignatureField nodes require one PID
# presentation ceremony and one PAdES signature PER FIELD, applied
# sequentially (parallel signing conflicts with PDF/A-3 incremental updates,
# see the change request on DCS-FR-SM-17). Every ceremony must complete
# BEFORE the first signature so all signers' evidence is embedded ahead of
# the signature that freezes the document (a post-signature attachment trips
# standards-compliant PAdES diff analysis), and the deploy gate holds the
# contract back from activating until every declared field is signed
# (DCS-NFR-BR-03: contracts lacking the required signatures must not proceed
# to deployment/execution).

@UC-04 @DCS-FR-SM-07 @DCS-FR-SM-17 @UC-03-06
Feature: Multi-signer signing workflow

  @clean_db @DCS-FR-SM-07 @DCS-FR-SM-17 @DCS-FR-CWE-19 @DCS-FR-SM-24
  Scenario: Both declared signature fields must be signed before the contract activates
    Given contract "Dual Signer Contract" is a fresh draft declaring signature fields "SignerOne" and "SignerTwo"
    And contract "Dual Signer Contract" is submitted, reviewed, and approved via the standard workflow
    And contract "Dual Signer Contract" deploys to the configured target system
    And a completed signing ceremony exists for field "SignerOne" of contract "Dual Signer Contract"
    And a completed signing ceremony exists for field "SignerTwo" of contract "Dual Signer Contract"
    When the signer of field "SignerOne" applies their signature to contract "Dual Signer Contract"
    Then get http 200:Success code
    And the contract "Dual Signer Contract" is in state "SIGNED"
    And a manual deployment of contract "Dual Signer Contract" is rejected because signing is incomplete
    When the signer of field "SignerTwo" applies their signature to contract "Dual Signer Contract"
    Then get http 200:Success code
    And the contract target acknowledges the deployment of contract "Dual Signer Contract"
    And the signature view for contract "Dual Signer Contract" shows two "SIGNED" signatures covering fields "SignerOne" and "SignerTwo"

  # The all-ceremonies-before-first-signature gate: signer one cannot sign
  # while field two has no verified ceremony yet.
  @clean_db @DCS-FR-SM-07
  Scenario: The first signature is refused until every declared field has a completed ceremony
    Given contract "Incomplete Ceremonies Contract" is a fresh draft declaring signature fields "SignerOne" and "SignerTwo"
    And contract "Incomplete Ceremonies Contract" is submitted, reviewed, and approved via the standard workflow
    And a completed signing ceremony exists for field "SignerOne" of contract "Incomplete Ceremonies Contract"
    When the signer of field "SignerOne" applies their signature to contract "Incomplete Ceremonies Contract"
    Then the signature apply is rejected because not all declared fields have a completed ceremony

  @clean_db @DCS-FR-SM-17
  Scenario: A declared signature field cannot be signed twice
    Given contract "Double Field Contract" is a fresh draft declaring signature fields "SignerOne" and "SignerTwo"
    And contract "Double Field Contract" is submitted, reviewed, and approved via the standard workflow
    And a completed signing ceremony exists for field "SignerOne" of contract "Double Field Contract"
    And a completed signing ceremony exists for field "SignerTwo" of contract "Double Field Contract"
    When the signer of field "SignerOne" applies their signature to contract "Double Field Contract"
    Then get http 200:Success code
    When the signer of field "SignerOne" applies their signature to contract "Double Field Contract"
    Then the signature apply is rejected because the field is already signed
