# Access Rights Revocation (UC-15-01). The revoke action itself
# (POST /signature/revoke) and its ContractSigner-level effects are exercised
# extensively by 22_real_signing_vertical/real_signing_vertical.feature; this
# file covers the piece that pack does not assert: that revoking a signature
# transitions the CONTRACT's own lifecycle state to REVOKED
# (backend/internal/signingmanagement/command/revoke.go,
# contractstate.EventRevoke) and that re-signing is required to restore it —
# the observable "rights withdrawn until re-signing" behavior Table 7 names.

@UC-15-01 @DCS-FR-SM-20
Feature: Signature revocation transitions the contract to REVOKED

  @UC-15-01
  Scenario: Revoking a contract's signature moves the contract to REVOKED
    Given contract "Revocation State Contract" has reached contract state "REVOKED"
    Then the contract "Revocation State Contract" is in state "REVOKED"

  @UC-15-01 @DCS-FR-CWE-06
  Scenario: A revoked contract can be re-approved to allow re-signing
    Given contract "Revocation Restore Contract" has reached contract state "REVOKED"
    And I am authenticated with roles: "Contract Approver"
    When I approve contract "Revocation Restore Contract"
    Then get http 200:Success code
    And the contract "Revocation Restore Contract" is in state "APPROVED"

  @clean_db @UC-04 @DCS-IR-SM-06 @REQ-workflow-ui-error-safety-and-audit-states-AC4
  Scenario: A successful signature revocation records the exact reason in the audit trail
    Given contract "Reasoned Signature Revocation Contract" has reached contract state "SIGNED"
    When the Contract Manager revokes the applied signature of contract "Reasoned Signature Revocation Contract" with reason "Signer credential revoked by issuer"
    Then get http 200:Success code
    And the "REVOKE_SIGNATURE" signature audit entry for contract "Reasoned Signature Revocation Contract" records exact reason "Signer credential revoked by issuer"

  @clean_db @UC-04 @DCS-IR-SM-06 @REQ-workflow-ui-error-safety-and-audit-states-AC4
  Scenario: A missing or blank revocation reason cannot revoke a signature
    Given contract "Unreasoned Signature Revocation Contract" has reached contract state "SIGNED"
    When the Contract Manager attempts to revoke the applied signature of contract "Unreasoned Signature Revocation Contract" without a reason
    Then the signature revocation is rejected because a nonblank reason is required
    And the applied signature of contract "Unreasoned Signature Revocation Contract" remains "SIGNED"
    When the Contract Manager attempts to revoke the applied signature of contract "Unreasoned Signature Revocation Contract" with a whitespace-only reason
    Then the signature revocation is rejected because a nonblank reason is required
    And the applied signature of contract "Unreasoned Signature Revocation Contract" remains "SIGNED"
