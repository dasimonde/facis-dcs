# Power of Attorney at signing (UC-14, DCS-FR-SM-03/-04/-26): a natural person
# signs on behalf of an organization, presenting a Power of Attorney at the
# signing ceremony that authorizes them to act for the party — the participating
# DCS instance DID — they sign as. A missing or wrong-party PoA blocks signing
# (UC-14). The Signature Compliance Viewer re-checks every party node in the
# (possibly peer-synced) contract and raises an audit finding for any party, own
# or a counterparty's, that signed without a Power of Attorney for that party.

@UC-04 @DCS-FR-SM-03 @DCS-FR-SM-04 @DCS-FR-SM-26 @ADR-12
Feature: Power of Attorney at signing

  @DCS-FR-SM-03 @UC-14 @DCS-NFR-SEC-18
  Scenario: A signatory with a Power of Attorney for their party signs and is compliant
    Given contract "PoA Happy Contract" is APPROVED and has completed a signing ceremony for signatory "SignerPoaHappy"
    When the signer publishes the OID4VP signing request for contract "PoA Happy Contract"
    Then get http 200:Success code
    When the wallet signs contract "PoA Happy Contract" by consuming the OID4VP signing request as "SignerPoaHappy"
    Then the contract "PoA Happy Contract" has completed signing
    And the signature compliance for contract "PoA Happy Contract" raises no Power of Attorney finding

  @UC-14
  Scenario: Signing is blocked when no Power of Attorney is presented at the ceremony
    Given contract "PoA Missing Contract" has reached contract state "APPROVED"
    When a signing ceremony is started for the signing party of contract "PoA Missing Contract"
    And the ceremony presentation is completed with no Power of Attorney
    Then the signing request is rejected because the Power of Attorney does not authorize this signature

  @UC-14
  Scenario: Signing is blocked when the Power of Attorney authorizes a different party
    Given contract "PoA Wrong Contract" has reached contract state "APPROVED"
    When a signing ceremony is started for the signing party of contract "PoA Wrong Contract"
    And the ceremony presentation is completed with a Power of Attorney for a different party
    Then the signing request is rejected because the Power of Attorney does not authorize this signature

  @DCS-FR-SM-04 @DCS-FR-SM-26
  Scenario: A counterparty that signed without a valid Power of Attorney is raised in audit
    Given contract "PoA Counterparty Contract" is APPROVED and has completed a signing ceremony for signatory "SignerPoaCp"
    When the signer publishes the OID4VP signing request for contract "PoA Counterparty Contract"
    Then get http 200:Success code
    When the wallet signs contract "PoA Counterparty Contract" by consuming the OID4VP signing request as "SignerPoaCp"
    Then the contract "PoA Counterparty Contract" has completed signing
    When the counterparty Power of Attorney on contract "PoA Counterparty Contract" is tampered to authorize a different organization
    Then the signature compliance for contract "PoA Counterparty Contract" raises a Power of Attorney finding
    And an audit event records the Power of Attorney finding for contract "PoA Counterparty Contract"

  # ---------------------------------------------------------------------
  # Mutual binding across instances (ADR-31). The scenarios above check what
  # a contract SAYS about a counterparty's authority; these two check the
  # evidence behind it. The shipping instance carries the Power of Attorney
  # its signatory presented at the ceremony, and the receiver verifies it:
  # issuer trusted for `peer` and entitled to that organization, credential
  # not revoked, held by the signatory the contract records.
  #
  # Failing closed applies to evidence that is present and does not verify.
  # Evidence that is ABSENT is accepted — a peer that retains none must still
  # federate — and a party that signed without a Power of Attorney keeps being
  # raised by the compliance viewer, as the scenario above requires.
  # ---------------------------------------------------------------------

  @DCS-FR-SM-04 @UC-14 @ADR-31 @two-instance
  Scenario: A counterparty's Power of Attorney travels with its signature and is verified on the receiving instance
    Given instance A and instance B are both running and trust each other
    When the initiator on instance A creates and offers a contract with instance B as counterparty
    Then the contract appears on instance B in state OFFERED within a few seconds
    When instance A drives the contract to APPROVED through its own local workflow
    And instance B drives its own copy of the contract to APPROVED through its own local workflow
    And instance A applies a ceremony-backed signature to the contract
    Then instance B holds instance A's signature with its Power of Attorney verified

  # The refusal is exercised between the two REAL instances, not from a
  # synthetic peer: the gates in front of the Power of Attorney — challenge-
  # response, agreement credential, policy endpoint — and the signing summary
  # the receiver reads the attribution from all have to pass for the credential
  # to be what refuses the ship, and only the instance that actually signed
  # holds evidence that does. The scenario ships A's own signed PDF, A's own
  # summary and A's own identity, substituting nothing but the credential.
  @DCS-FR-SM-04 @UC-14 @ADR-31 @two-instance
  Scenario: A ship whose Power of Attorney does not verify is refused and raises an incident
    Given instance A and instance B are both running and trust each other
    And the local policy endpoint (PDP) is running and allows every request
    And contract "PoA Evidence Contract" is APPROVED and has completed a signing ceremony for signatory "SignerPoaEvidence"
    When the signer publishes the OID4VP signing request for contract "PoA Evidence Contract"
    Then get http 200:Success code
    When the wallet signs contract "PoA Evidence Contract" by consuming the OID4VP signing request as "SignerPoaEvidence"
    Then the contract "PoA Evidence Contract" has completed signing
    When instance A ships contract "PoA Evidence Contract"'s PDF to instance B with a Power of Attorney that does not verify
    Then instance B refuses the ship because the counterparty's Power of Attorney does not verify
    And exactly one incident is recorded in instance B's audit trail for contract "PoA Evidence Contract"
