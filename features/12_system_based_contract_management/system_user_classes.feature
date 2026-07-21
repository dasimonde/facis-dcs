# SRS §2.4 Table 5 System User classes: machine callers that reach DCS over its
# API rather than through a browser. Unlike a Human User they hold no wallet and
# present no verifiable credential, so they authenticate against this
# deployment's Hydra with their own client_id/client_secret (client_credentials)
# and DCS decides what each may do from deployment configuration — see ADR-16.
#
# One OAuth2 client per class, never one shared machine identity: a client
# reaches only what its class may reach, and the audit trail attributes actions
# to the capability rather than to "the integration".
#
# The System Contract Signer scenarios assert a REFUSAL. eIDAS Art. 3(9) makes a
# signatory a natural person and Art. 26 requires their sole control, so an
# unattended client cannot produce an advanced electronic signature; a legal
# person's instrument is a seal (Art. 3(25)), which DCS does not implement. The
# class is kept and demonstrably powerless rather than silently absent — ADR-17.

@UC-12 @DCS-FR-CWE-13 @ADR-16
Feature: SRS System User classes authenticate as machine clients

  @UC-12-01 @DCS-FR-CWE-13
  Scenario: A System Contract Creator authenticates with its own client credentials
    When the system client "dcs-orce-creator" obtains an access token
    Then the system client holds a machine access token

  @UC-12-01 @DCS-FR-CWE-28
  Scenario: A System Contract Manager authenticates with its own client credentials
    When the system client "dcs-orce-manager" obtains an access token
    Then the system client holds a machine access token

  @DCS-IR-PACM-01 @ADR-16
  Scenario: The System Auditor reads the audit checkpoint head it anchors externally
    Given contract "Anchored Audit Contract" has reached contract state "APPROVED"
    When the system client "dcs-orce-notary" obtains an access token
    And the system client requests GET "/pac/audit/checkpoint/head"
    Then the audit checkpoint head carries a Merkle root

  @DCS-IR-PACM-01 @ADR-16
  Scenario: The System Auditor reaches the tamper-evidence surface and nothing else
    When the system client "dcs-orce-notary" obtains an access token
    And the system client requests GET "/contract/retrieve"
    Then the system client request is refused as forbidden

  # The bodies below are well-formed on purpose: goa rejects a malformed
  # payload at the transport layer before the security scheme runs, so an empty
  # body would return 400 and prove nothing about authorization. The values are
  # arbitrary — the refusal happens before the handler looks anything up.

  @UC-04 @DCS-FR-SM-03 @ADR-17
  Scenario: A System Contract Signer cannot start a signing ceremony
    When the system client "dcs-orce-signer" obtains an access token
    And the system client requests POST "/signature/request" with body
      """
      {"contract_did": "urn:uuid:refused", "field_name": "signature-field-refused"}
      """
    Then the system client request is refused as forbidden

  @UC-04 @DCS-FR-SM-03 @ADR-17
  Scenario: A System Contract Signer cannot prepare a document for signature
    When the system client "dcs-orce-signer" obtains an access token
    And the system client requests POST "/signature/prepare" with body
      """
      {"did": "urn:uuid:refused", "signer_did": "did:web:refused.example"}
      """
    Then the system client request is refused as forbidden

  @UC-04 @DCS-FR-SM-03 @ADR-17
  Scenario: A System Contract Signer cannot submit a signature
    When the system client "dcs-orce-signer" obtains an access token
    And the system client requests POST "/signature/submit" with body
      """
      {"did": "urn:uuid:refused", "signer_did": "did:web:refused.example", "signed_pdf": "JVBERi0="}
      """
    Then the system client request is refused as forbidden

  @UC-04 @DCS-FR-SM-19 @ADR-17
  Scenario: A System Contract Signer may still verify signatures it cannot create
    When the system client "dcs-orce-signer" obtains an access token
    And the system client requests GET "/signature/retrieve"
    Then get http 200:Success code
