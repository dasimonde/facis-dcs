# OID4VP "Document Retrieval" signing ceremony (ADR-12, DCS-FR-SM-16,
# DCS-IR-SI-04): the QR-driven wallet automation layer on top of the working
# prepare/submit signing. After a ceremony is PID-verified, the DCS publishes a
# STANDARD OID4VP Document-Retrieval request object (a signed JAR carrying
# documentDigests, documentLocations, response_uri, nonce) as a QR/deep link.
# The wallet scans it, fetches the request object, fetches the to-be-signed
# document, signs it with the signatory's own key (sole control), and posts the
# signed document back to the response_uri callback — where the DCS reuses the
# exact validate + finalize path /signature/submit uses. This is what makes a
# real EUDI wallet a configuration swap: nothing DCS-specific crosses the wallet
# boundary but the standard request object.
#
# The harness plays the wallet+QTSP stand-in: steps drive the published QR end to
# end via testWallet/dcs_wallet/oid4vp_signing.py (fetch request object -> fetch
# document -> sign via the external EU DSS SCA -> post to callback), exactly as a
# real wallet would.

@UC-04 @DCS-FR-SM-16 @DCS-IR-SI-04 @ADR-12
Feature: OID4VP Document-Retrieval signing ceremony

  @DCS-FR-SM-16 @DCS-IR-SI-04
  Scenario: Publishing a signing request emits a standard OID4VP Document-Retrieval request object
    Given contract "RSV DocRetrieval Publish Contract" is APPROVED and has completed a signing ceremony for signatory "SignerDrPublish"
    When the signer publishes the OID4VP signing request for contract "RSV DocRetrieval Publish Contract"
    Then get http 200:Success code
    And the publish response carries a client_id, request_uri, and expires_at
    And the published request object is a signed JAR carrying documentDigests, documentLocations, response_uri, and a nonce

  @DCS-FR-SM-16 @DCS-FR-SM-18 @UC-04-02 @UC-04-03
  Scenario: A wallet scanning the QR signs the fetched document and the contract reaches SIGNED via the callback
    Given contract "RSV DocRetrieval E2E Contract" is APPROVED and has completed a signing ceremony for signatory "SignerDrE2E"
    When the signer publishes the OID4VP signing request for contract "RSV DocRetrieval E2E Contract"
    Then get http 200:Success code
    When the wallet signs contract "RSV DocRetrieval E2E Contract" by consuming the OID4VP signing request as "SignerDrE2E"
    Then the wallet callback reports the contract "RSV DocRetrieval E2E Contract" as SIGNED
    And the contract "RSV DocRetrieval E2E Contract" is in state "SIGNED"
    And the signature view for contract "RSV DocRetrieval E2E Contract" shows a "SIGNED" signature for the signing party
