# The CEK wrap key in the DID document (ADR-28): gendid publishes a third
# verification method — the HSM's P-256 key-agreement PUBLIC key (label
# dcs-ecdh, env DCS_HSM_KEY_ECDH) — referenced by a top-level `keyAgreement`
# relation. Peers wrap content-encryption keys to this key when shipping
# contracts (ECDH-ES+A256KW); unwrapping requires the instance's HSM.
#
# What a consumer may rely on is the RELATIONSHIP, never the position: DID Core
# gives the order of verificationMethod no meaning, so this scenario asserts
# which relationship each key is published in and deliberately asserts nothing
# about which entry comes first. That the key-agreement key appears in no other
# relationship is the fail-closed half — a key published for encryption must not
# be able to verify a signature.
#
# Read-only, unauthenticated scenario (GET /.well-known/did.json): it mutates
# no per-identity state, so the dedicated-organization rule for state-mutating
# scenarios does not apply here.

@DCS-NFR-SEC-14
Feature: The served DID document publishes the key-agreement method for CEK wrapping

  Scenario: did.json carries the dcs-ecdh verification method under keyAgreement, and under keyAgreement alone
    When I fetch this instance's served DID document
    Then the DID document's keyAgreement relation names exactly one verification method with fragment "dcs-ecdh"
    And that key-agreement verification method is a P-256 JsonWebKey2020 published under its own id
    And the key-agreement method appears in no other verification relationship
    And the DID document publishes its identity key for authentication
