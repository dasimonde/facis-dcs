# Contract state machine — Offer, Withdraw, and the extended transition
# table: the first-class contract states OFFERED, WITHDRAWN, ACTIVE, and
# REVOKED alongside DRAFT/NEGOTIATION/SUBMITTED/REVIEWED/APPROVED/SIGNED/
# TERMINATED/EXPIRED (authoritative table:
# backend/internal/contractworkflowengine/datatype/contractstate; see also
# docs/adr-2-contract-state-machine.md).
#
# Out of scope for this pack:
#   - The ACTIVE/REVOKED C2PA lifecycle mapping: reaching ACTIVE requires
#     the deployment/ORCE flow (covered in 05_contract_deployment) and
#     REVOKED is covered in 15_access_revocation, so only OFFERED/
#     NEGOTIATION/SUBMITTED/REVIEWED/APPROVED/SIGNED/TERMINATED are
#     exercised here.
#   - Frontend status label rendering: this BDD stack has no browser
#     automation (no Selenium/Playwright), only HTTP-level checks. The
#     search-filter scenario below only proves the backend search/filter
#     contract the frontend Status-Filter depends on; it does NOT observe
#     the Vue label.

@UC-03
Feature: Contract state machine refactor — Offer, Withdraw, and the extended transition table

  @SRS-2.2.6 @SRS-1.2 @UC-03
  Scenario: Initiator offers a draft contract and it becomes OFFERED
    Given I am authenticated with roles: "Contract Creator"
    And contract "Offer Draft Contract" is in "Draft" status
    When the initiator offers contract "Offer Draft Contract"
    Then get http 200:Success code
    And the contract "Offer Draft Contract" is in state "OFFERED"

  # SRS 1.2 defines an offer as "a clear and definite proposal", and SRS 2.2.2
  # requires Contract Generation to end with a filled-out contract "ready to
  # be sent to the Responder" — so the offer transmission enforces contract
  # closedness (backend/internal/contractworkflowengine/command/offer.go
  # validateOfferReady): a draft still carrying an unresolved required
  # placeholder is rejected with a client error and stays in DRAFT until the
  # placeholder is filled.
  @SRS-1.2 @SRS-2.2.2 @SRS-2.2.6 @UC-03
  Scenario: Offering a draft with an unresolved required placeholder is rejected until the draft is filled out
    Given I am authenticated with roles: "Contract Creator"
    And contract "Open Placeholder Contract" is a draft with an unfilled required placeholder
    When the initiator offers contract "Open Placeholder Contract"
    Then I receive error "contract is not closed"
    And the contract "Open Placeholder Contract" is in state "DRAFT"
    When the initiator fills the required placeholder of contract "Open Placeholder Contract" with "20000"
    And the initiator offers contract "Open Placeholder Contract"
    Then get http 200:Success code
    And the contract "Open Placeholder Contract" is in state "OFFERED"

  @SRS-1.2 @SRS-2.2.6 @UC-03
  Scenario Outline: Initiator withdraws a contract from a pre-approval state
    Given I am authenticated with roles: "Contract Creator"
    And contract "<name>" has reached contract state "<state>"
    When the initiator withdraws contract "<name>"
    Then get http 200:Success code
    And the contract "<name>" is in state "WITHDRAWN"

    Examples: Pre-approval states from which Withdraw must succeed
      | name                          | state       |
      | Withdraw From Offered         | OFFERED     |
      | Withdraw From Negotiation     | NEGOTIATION |
      | Withdraw From Submitted       | SUBMITTED   |
      | Withdraw From Reviewed        | REVIEWED    |

  @DCS-NFR-BR-08 @SRS-1.2
  Scenario: Withdraw is rejected once the contract has been approved
    Given I am authenticated with roles: "Contract Creator"
    And contract "Withdraw After Approval" has reached contract state "APPROVED"
    When the initiator withdraws contract "Withdraw After Approval"
    Then the withdraw request is rejected
    And the contract "Withdraw After Approval" is in state "APPROVED"

  @DCS-IR-CWE-05 @DCS-IR-CWE-06 @DCS-IR-CWE-07 @DCS-IR-CWE-08 @DCS-IR-CWE-09 @DCS-IR-CWE-10 @SRS-3.1.1
  Scenario: Approve on a draft contract is rejected via the UI-API entry path
    Given I am authenticated with roles: "Contract Approver"
    And contract "Invalid Transition UI Path" is in "Draft" status
    When I attempt to approve contract "Invalid Transition UI Path"
    Then the request is denied with a client error
    And the contract "Invalid Transition UI Path" is in state "DRAFT"

  # NOTE: the peer-action entry path (`POST /peer/contracts/action`) requires
  # a successful did:web challenge-response handshake (hostname resolution +
  # eIDAS check + signature verify, see backend/internal/service/dcs_to_dcs.go
  # Action()) before the transition table is ever reached. This scenario runs
  # single-instance, so it simulates a trusted peer by having the instance
  # authenticate as its own did:web identity (checked-in dev DID document,
  # backend/certs/dev/did-8991.json, signed via the per-instance SoftHSM2
  # token — see steps/template_management/contract_state_machine_steps.py
  # _self_peer_action_credentials / _dev_signing_token_dir). Because the
  # contract under test is also
  # created locally (Origin == this same DID), Approver.Handle's
  # single-writer forwarding check is a no-op and the exact same
  # `contractstate.ValidateTransition` the UI-API path hits is reached
  # directly — so this scenario's rejection genuinely evidences the
  # transition table, not a peer-auth failure (the Then step asserts the
  # error message names the transition rejection, not an auth error).
  # This still doesn't exercise cross-operator peer trust (did:web hostname
  # resolution across two independently-run instances, a real
  # eIDAS-certificate-chain check, or the local trusted_peers allowlist) —
  # that is the @two-instance peer-trust pack's job (features/17_peer_trust).
  @DCS-IR-CWE-05 @DCS-IR-CWE-06 @DCS-IR-CWE-07 @DCS-IR-CWE-08 @DCS-IR-CWE-09 @DCS-IR-CWE-10 @UC-03
  Scenario: Submit, review, and approve still reach APPROVED under the new transition table
    Given I am authenticated with roles: "Contract Creator"
    And contract "Full Approval Path" is in "Draft" status
    When contract "Full Approval Path" is submitted, reviewed, and approved via the standard workflow
    Then the contract "Full Approval Path" is in state "APPROVED"

  @SRS-1.2 @FR-SM
  Scenario: Signing an approved contract transitions it to SIGNED
    Given I am authenticated with roles: "Contract Creator"
    And contract "Signing Flow" has reached contract state "APPROVED"
    When the counterparty signer applies a signature to contract "Signing Flow"
    Then get http 200:Success code
    And the contract "Signing Flow" has completed signing

  @SRS-2.2.6 @DCS-NFR-BR-08
  Scenario: Offering a contract emits a typed OFFER outbox event
    Given I am authenticated with roles: "Contract Creator"
    And contract "Offer Event Contract" is in "Draft" status
    When the initiator offers contract "Offer Event Contract"
    Then get http 200:Success code
    And the contract "Offer Event Contract" has an audit event of type "OFFER_CONTRACT"

  @SRS-2.2.6 @DCS-NFR-BR-08
  Scenario: Withdrawing a contract emits a typed WITHDRAW outbox event
    Given I am authenticated with roles: "Contract Creator"
    And contract "Withdraw Event Contract" has reached contract state "OFFERED"
    When the initiator withdraws contract "Withdraw Event Contract"
    Then get http 200:Success code
    And the contract "Withdraw Event Contract" has an audit event of type "WITHDRAW_CONTRACT"

  @DCS-OR-C2PA-003
  Scenario Outline: C2PA lifecycle mapping is valid for every reachable new-machine contract state
    Given I am authenticated with roles: "Contract Manager"
    And contract "<name>" has reached contract state "<state>"
    When contract "<name>" is exported and verified as PDF
    Then get http 200:Success code
    And the C2PA lifecycle_status for contract "<name>" is "<c2pa_status>"

    Examples: States covered by this requirement's own endpoints
      | name                 | state       | c2pa_status |
      | C2PA State Offered   | OFFERED     | draft       |
      | C2PA State Negotiate | NEGOTIATION | draft       |
      | C2PA State Submitted | SUBMITTED   | draft       |
      | C2PA State Reviewed  | REVIEWED    | draft       |
      | C2PA State Approved  | APPROVED    | draft       |
      | C2PA State Signed    | SIGNED      | active      |
      | C2PA State Terminate | TERMINATED  | terminated  |

  # Partial coverage only — see the feature-level comment above: this proves
  # the backend `state` filter accepts the new lifecycle values (the
  # contract on which the frontend Status-Filter and status-label components
  # depend), not the actual Vue rendering.
  @SRS-3.1.1
  Scenario Outline: The contract search endpoint recognizes the new lifecycle states as a filter value
    Given I am authenticated with roles: "Contract Manager"
    And contract "<name>" has reached contract state "<state>"
    When the contract search endpoint is queried with state filter "<state>"
    Then get http 200:Success code
    And the search results include contract "<name>"

    Examples: New states the frontend filter/list must support
      | name                     | state     |
      | Filter By Offered State  | OFFERED   |
      | Filter By Withdrawn State| WITHDRAWN |
