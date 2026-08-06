# Peer CEK handshake (ADR-28): the first PDF ship carries the contract CEK
# wrapped to the peer's keyAgreement key (`wrapped_cek` on the post_pdf
# payload, backend/design/dcs_to_dcs.go). The receiver unwraps via its own
# HSM, re-wraps to its own key, persists, and stores the inbound PDF
# encrypted under that same CEK — adoption is idempotent across re-ships.
#
# The wrap NAMES the key it was made for (`wrapped_cek.kid`, the recipient's own
# keyAgreement verification method), so the sender resolves a key the receiver
# published instead of assuming it published exactly one, and the receiver
# refuses a wrap addressed to any other key. A byte-identical export on both
# sides is therefore also the proof that the named key was the right one.
#
# The observable proof both instances hold a usable CEK for the same
# contract is behavioral, not DB-level: each instance must decrypt its OWN
# stored copy back to the exact shipped bytes (the verbatim-inbound-PDF rule
# makes the two stored artifacts byte-identical, so the two authorized
# exports must be too). This also proves the envelope is invisible to the
# C2PA/JAdES chains — the strict C2PA assertions of the existing suites
# (19_c2pa_conformance, 22_real_signing_vertical) remain in force unchanged
# and would catch any byte-tampering by the storage path.
#
# Setup reuses the canonical cross-instance steps of 17_peer_trust; the
# erasure-related reads run under this scenario's dedicated organization.
# Both instances settle their own copy before A signs, because neither may
# sign a version the other has not settled. B's settlement round folds into
# B's copy before A's signed PDF arrives, and the export compared below is
# still the same artifact on both sides: B adopts the inbound signed PDF
# verbatim (prefer-inbound-PDF rule) rather than re-rendering its own.

@DCS-NFR-SEC-14 @two-instance
Feature: Peer CEK handshake — the shipped wrapped CEK lets the counterparty serve the identical contract

  Scenario: After signing, both instances export the byte-identical contract PDF from their encrypted stores
    Given instance A and instance B are both running and trust each other
    And the dedicated organization "BDD Encryption Handshake Org" is used for this scenario's identities
    When the initiator on instance A creates and offers a contract with instance B as counterparty
    Then the contract appears on instance B in state OFFERED within a few seconds
    When instance A drives the contract to APPROVED through its own local workflow
    And instance B drives its own copy of the contract to APPROVED through its own local workflow
    And instance A applies a ceremony-backed signature to the contract
    Then the cross-instance contract's PDF export is byte-identical on instance A and instance B
    And the erasure status of the cross-instance contract reports local status "live" on instance A
