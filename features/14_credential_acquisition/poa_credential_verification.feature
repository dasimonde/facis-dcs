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
# this suite.
#
# Delegation chain (DCS-FR-SM-04): a counterparty's Power of Attorney may be
# delegated. The receiving instance walks the chain up to an issuer trusted
# for `peer` (at most 3 links) and verifies each link: the delegator's key
# signed it, the delegator permits delegation (may_delegate), the
# organization is the same, the roles are a subset, the link expires no later
# than its delegator, and no link is revoked. The Go tests in
# backend/internal/auth/oid4vp/poa_chain_test.go generate such chains. The
# testWallet issues no delegated credentials, and the local ceremony verifies
# the signer's PoA against the login issuer of this instance (ADR-35), so the
# delegated-authority scenario below stays @skip.

@UC-14 @DCS-FR-SM-03 @DCS-FR-SM-04
Feature: Power-of-Attorney credential verification at signing

  # The PoA presented at the ceremony has its status-list bit revoked on the
  # issuer first; the refusal must name that bit. A revoked link of a
  # counterparty's chain is refused in
  # TestVerifyCounterpartyPoA_RevokedLinkRefusesTheChain.
  @UC-14-01 @DCS-FR-SM-03 @DCS-FR-SM-04
  Scenario: A revoked PoA credential blocks signing
    Given contract "PoA Revoked Contract" has reached contract state "APPROVED"
    When a signing ceremony is started for the signing party of contract "PoA Revoked Contract"
    And the ceremony presentation is completed with a revoked Power of Attorney
    Then the ceremony presentation is refused for the revoked Power of Attorney
