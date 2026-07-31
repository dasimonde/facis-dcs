# Key-shredding erasure (ADR-28): IPFS cannot delete ([DCS-NFR-SF-03]), so
# GDPR erasure ([DCS-NFR-COMP-03] Art. 17, [DCS-NFR-SEC-13] secure disposal,
# [DCS-IR-CSA-03] archive deletion under defined policies) is implemented as
# logged destruction of the wrapped content-encryption keys on BOTH
# federation instances. DELETE /archive/delete triggers the local shred and
# the peer erase handshake (POST /peer/contracts/erase); the justification
# becomes the recorded shred_reason.
#
# The shredded CEK rows are NOT deleted — the shredded row IS the
# destruction record, surfaced by GET /archive/erasure-status (asserted here
# instead of reading content_encryption_keys directly). KEY_SHREDDED audit
# events are instance-scoped so they survive the very erasure they record.
#
# Tamper evidence survives (ADR-16): audit leaf hashes are computed over the
# stored (encrypted) entry bytes, so shredding erases the bodies — readers
# get the defined {"erased": true} marker — while checkpoint inclusion
# proofs keep verifying. The proof is captured BEFORE the shred and
# recomputed (RFC 6962) against the checkpoint root AFTER it.
#
# The audit API never returns an entry's own CID; the captured entry CID is
# a chain predecessor (res_log_pred_cid) of the contract's own per-DID
# chain, polled until a checkpoint commits to it.

@DCS-NFR-COMP-03 @DCS-NFR-SEC-13 @DCS-IR-CSA-03 @two-instance
Feature: Key shredding — archive deletion destroys the contract CEKs on both instances while the audit chain keeps verifying

  Scenario: Deleting the archived contract shreds its keys federation-wide, erases the audit bodies, and leaves the checkpoint proof intact
    Given instance A and instance B are both running and trust each other
    And the dedicated organization "BDD Encryption Shredding Org" is used for this scenario's identities
    When the initiator on instance A creates and offers a contract with instance B as counterparty
    Then the contract appears on instance B in state OFFERED within a few seconds
    When instance A drives the contract to APPROVED through its own local workflow
    And instance A applies a ceremony-backed signature to the contract
    When an anchored audit entry CID of the cross-instance contract is captured on instance A
    And the Archive Manager of the dedicated organization deletes the archived cross-instance contract with justification "GDPR Art. 17 erasure request"
    Then the erasure status of the cross-instance contract reports local status "shredded" on instance A
    And the erasure status on instance A names peer confirmation from instance B
    And the erasure status of the cross-instance contract reports local status "shredded" on instance B
    And a KEY_SHREDDED audit event for the cross-instance contract is recorded on instance A
    And a KEY_SHREDDED audit event for the cross-instance contract is recorded on instance B
    And the workflow audit entry bodies of the cross-instance contract are erased on instance A
    And the captured audit entry's checkpoint inclusion proof still verifies on instance A
