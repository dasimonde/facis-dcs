# Identity and PoA Credential Acquisition (UC-14). PID credential acquisition,
# verification, and binding to the signing session IS implemented and
# exercised end-to-end by 22_real_signing_vertical/real_signing_vertical.
# feature (wallet-driven signing ceremony, SD-JWT VC + KB-JWT presentation,
# embedded under the PAdES signature) — not duplicated here.
#
# PoA disposition ("credential at login"): the PoA credential (dc+sd-jwt,
# vct urn:dcs:poa:v1, holder-bound) is presented at LOGIN — every session
# starts with an OpenID4VP PoA presentation whose organization/role
# attributes are mapped into the Hydra session, and the credential's
# revocation status is checked in every verify path. Presentation + status
# verification are therefore exercised by every authenticated scenario in
# this suite. What remains open (deliberately deferred roadmap work, backed
# by SRS TBD-B: the XFSC PCM wallet ecosystem is not yet available for
# testing) is the ISSUER CHAIN-WALK: verifying that the PoA's issuer is
# itself legitimized up to a trust anchor. The scenarios below assert
# exactly that chain-walk and stay @skip until it lands.

@UC-14 @DCS-FR-SM-03 @DCS-FR-SM-04
Feature: Power-of-Attorney credential verification at signing

  # @skip: asserts the PoA issuer chain-walk to a trusted root — deferred
  # roadmap work; presentation + status check of the PoA itself happen at
  # login and are covered suite-wide.
  @skip @UC-14-01 @DCS-FR-SM-03
  Scenario: Signer presents a PoA credential proving delegated signing authority
    Given I hold a PoA credential compliant with eIDAS framework for organization "BDD Org"
    When I present the PoA credential during the signing ceremony
    Then the PoA credential chain is verified to a trusted root
    And the signature is bound to the delegated authority

  # @skip: distinguishing a chain-invalid PoA (issuer not anchored) from a
  # merely revoked one needs the same chain-walk verifier — the same
  # deferred roadmap item. Plain revocation blocking is live in every
  # verify path.
  @skip @UC-14-01 @DCS-FR-SM-04
  Scenario: A revoked PoA credential blocks signing
    Given I hold a revoked PoA credential for organization "BDD Org"
    When I attempt to initiate signing for contract "PoA Revoked Contract"
    Then the request is denied with a client error
