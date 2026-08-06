# Peer-erase retry ledger (ADR-28): the erase handshake towards each
# counterparty is tracked in the contract_erasures ledger — one row per
# (contract, peer) with requested_at, retry accounting, and confirmed_at
# once the peer acknowledged POST /peer/contracts/erase. An unreachable peer
# leaves the row pending; the retry job re-delivers until confirmation. The
# ledger is surfaced per peer by GET /archive/erasure-status (status
# pending|confirmed, requested_at, confirmed_at, retry_count, last_tried_at)
# — asserted here through that API, not the table.
#
# OPEN POINT: a genuine peer-down window is not honestly simulatable in this
# suite today — instance B is a Helm release the harness keeps running, and
# the suite has no scale-down/unreachable seam for a whole DCS instance
# (the existing "unreachable" patterns redirect single env-var URLs, which
# does not exist for the peer base URL derived from the counterparty's
# did:web). This scenario therefore pins the OBSERVABLE ledger contract —
# the request is recorded with its timestamp and reaches confirmed with
# confirmation timestamp and retry accounting — and the pending-with-
# retries path is covered at unit level (backend/internal/dcstodcs/
# eraser_test.go). Flagged rather than faked with a scenario name promising
# a peer outage this suite cannot produce.

@DCS-NFR-COMP-03 @DCS-NFR-SEC-13 @two-instance
Feature: Peer erase ledger — the erase handshake is tracked from request to confirmation

  Scenario: The peer erase request is recorded in the erasure ledger and reaches confirmed with a timestamp
    Given instance A and instance B are both running and trust each other
    And the dedicated organization "BDD Encryption Retry Org" is used for this scenario's identities
    When the initiator on instance A creates and offers a contract with instance B as counterparty
    Then the contract appears on instance B in state OFFERED within a few seconds
    When instance A drives the contract to APPROVED through its own local workflow
    And instance B drives its own copy of the contract to APPROVED through its own local workflow
    And instance A applies a ceremony-backed signature to the contract
    When the Archive Manager of the dedicated organization deletes the archived cross-instance contract with justification "BDD erase ledger probe"
    Then the erasure ledger on instance A records the peer erase request with its request timestamp
    And the erasure ledger on instance A reaches peer status "confirmed" with a confirmation timestamp and retry accounting
