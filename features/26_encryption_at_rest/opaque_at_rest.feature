# Encryption at rest (ADR-28): every artifact the DCS writes to IPFS is
# encrypted per contract before storage (internal/base/artifactstore, AES-256-
# GCM under a per-contract CEK wrapped to the instance's HSM keyAgreement
# key). IPFS itself is content-addressed, unauthenticated storage — the only
# acceptable at-rest state there is ciphertext (DCS-NFR-SEC-14).
#
# The raw-bytes fetch goes through the suite's BDD_IPFS_EXEC seam
# (steps/support/tamper_seam.py, `ipfs cat`) — the same IPFS-level access the
# tamper scenarios use, deliberately bypassing every DCS API. The plaintext
# marker asserted absent is the document's own title ("BDD Contract Source
# Template", the template name embedded in the contract document by the
# suite's canonical fixture), not the BDD-side contract alias.
#
# The dedicated organization satisfies the identity model for state-mutating
# scenarios: this scenario drives a contract to SIGNED and exports through
# its own organization's identities.

@DCS-NFR-SEC-14
Feature: Contract artifacts are opaque at rest in IPFS

  Scenario: The stored contract PDF is ciphertext on the IPFS node while the authorized export returns the intact PDF
    Given the dedicated organization "BDD Encryption Opaque Org" is used for this scenario's identities
    And contract "Opaque At Rest Contract" has reached contract state "SIGNED"
    When the raw bytes behind contract "Opaque At Rest Contract"'s stored PDF CID are fetched directly from the IPFS node
    Then the raw stored bytes do not start with the PDF magic bytes
    And the raw stored bytes do not contain "BDD Contract Source Template"
    When the Contract Manager of the dedicated organization exports contract "Opaque At Rest Contract" as PDF
    Then the export response is an intact PDF document
