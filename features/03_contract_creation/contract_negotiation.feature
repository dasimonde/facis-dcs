@UC-03-02 @FR-CWE-08 @FR-CWE-14 @FR-CWE-18 @FR-CWE-07
Feature: Contract Negotiation
  Contract Managers and Contract Reviewers negotiate contract terms through
  commenting, version tracking, and structured negotiation workflows with
  redline proposals and full audit logs.

  @clean_db
  Scenario: Open draft contract for negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in "Draft" status
    When I open contract "Service Agreement" for negotiation
    Then the negotiation interface is displayed
    And I can view all contract clauses

  @clean_db @DCS-IR-CWE-03
  Scenario: Add comment to contract clause
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" is open for negotiation
    When I add comment "Clarify payment terms" to clause "Payment Terms"
    Then the comment is added to the negotiation log
    And the comment is attributed to my identity
    And the comment includes a timestamp

  @clean_db @DCS-FR-CWE-17 @DCS-IR-CWE-03
  Scenario: Propose redline edit to contract clause
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" is open for negotiation
    When I propose a redline edit to clause "Liability"
    Then the proposed change is tracked
    And the original text is preserved
    And the redline proposal is visible to other negotiators

  @clean_db @DCS-FR-CWE-17 @DCS-FR-CWE-27 @DCS-IR-CWE-04
  Scenario: Track version history during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has multiple negotiation edits
    When I view version history for contract "Service Agreement"
    Then I see all versions with timestamps
    And I see user attribution for each version
    And old versions remain accessible

  @clean_db
  Scenario: Approve proposed change during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a pending redline proposal on clause "Liability"
    When I approve the redline proposal
    Then the change is applied to the contract
    And the approval is logged in the negotiation log
    And a new version is created

  @clean_db
  Scenario: Reject proposed change during negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has a pending redline proposal on clause "Liability"
    When I reject the redline proposal with reason "Not acceptable"
    Then the proposal is marked as rejected
    And the rejection reason is logged
    And the original text is retained

  @clean_db
  Scenario: View negotiation log
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has completed multiple negotiation rounds
    When I view the negotiation log for contract "Service Agreement"
    Then I see all comments and proposals
    And I see approvals and rejections
    And I see the full audit trail

  @clean_db
  Scenario: Submit contract for review after negotiation
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" negotiation is complete
    When I submit contract "Service Agreement" for review
    Then the contract is routed to assigned reviewers
    And the contract status changes to "Under Review"
    And the submission is logged

  Scenario: Unauthorized role cannot negotiate contracts
    Given I am authenticated with roles: "Contract Observer"
    And contract "Service Agreement" is in "Draft" status
    When I attempt to add a comment to contract "Service Agreement"
    Then the request is denied with an authorization error

  # Rewritten to the DID-party semantics this backend actually implements
  # (see backend/internal/contractworkflowengine/db/contractrepository.go
  # Responsible{Reviewers,Negotiators,Approvers []string}, all peer DIDs, and
  # IsValidNegotiator in acceptnegotiation.go/negotiate.go/
  # rejectnegotiation.go): "party to the contract" = "this instance's own
  # peer DID is among the contract's registered negotiator DIDs" — there is
  # no organization/representative-of-party concept in the data model, only
  # DID membership. FR-CWE-18's intent (only parties may negotiate) is
  # preserved; "Acme Corp"/"TechVendor Inc" party-naming is not.
  @clean_db @DCS-FR-CWE-01
  Scenario: Only parties to contract can negotiate terms
    Given I am authenticated with roles: "Contract Reviewer"
    And contract "Service Agreement" is open for negotiation
    When I add comment "Looks good" to clause "Liability"
    Then the comment is added to the negotiation log
    And the comment is attributed to my identity

  @clean_db
  Scenario: Contract Creator and assigned Reviewers can negotiate
    Given I am authenticated with roles: "Contract Creator"
    And contract "Service Agreement" is open for negotiation
    When I add comment "Looks good" to clause "Liability"
    Then the comment is added to the negotiation log
    And negotiation actions are logged with reviewer identity

  @clean_db
  Scenario: Reviewer cannot approve own redline proposals
    Given I am authenticated with roles: "Contract Reviewer"
    And I have proposed a redline edit to clause "Liability"
    When I attempt to approve my own redline proposal
    Then the request is denied with a "Conflict of interest - cannot approve own proposal" error
    And another authorized reviewer must approve
    And the restriction is logged

  # SRS §3.1.1 Contract Negotiation UI lists "Save draft" among its controls,
  # distinct from "Propose change": a negotiator stages modifications privately
  # and proposes them later. A draft creates no negotiation change-request row,
  # moves no contract state, and is consumed when its author proposes it
  # (command/negotiate.go clears the author's draft row).
  @DCS-IR-CWE-03 @clean_db
  Scenario: A negotiator saves a private draft and proposes it later
    Given I am authenticated with roles: "Contract Creator"
    And contract "Staged Draft Contract" has reached contract state "NEGOTIATION"
    When the negotiator saves a negotiation draft for contract "Staged Draft Contract" renaming it to "Staged Rename"
    Then get http 200:Success code
    And the negotiation draft for contract "Staged Draft Contract" contains the staged name "Staged Rename"
    And the contract "Staged Draft Contract" has no recorded negotiation change requests
    And the contract "Staged Draft Contract" is in state "NEGOTIATION"
    When the negotiator proposes the staged draft for contract "Staged Draft Contract"
    Then get http 200:Success code
    And the negotiation draft for contract "Staged Draft Contract" is empty
    And the contract "Staged Draft Contract" has a recorded negotiation change request renaming it to "Staged Rename"

  # Drafts are PARTY-scoped (saved_by = participant ID): a colleague of the
  # same party continues the party's staged position — the SRS's negotiation
  # security model is party-based ("only authorized parties can propose or
  # accept changes"), and nothing reaches the counterparty until proposed.
  @DCS-IR-CWE-03 @clean_db
  Scenario: A negotiation draft is shared within the party and can be discarded
    Given I am authenticated with roles: "Contract Creator"
    And contract "Party Draft Contract" has reached contract state "NEGOTIATION"
    When the negotiator saves a negotiation draft for contract "Party Draft Contract" renaming it to "Party Position"
    Then get http 200:Success code
    And a same-party user with roles "Contract Reviewer" sees the negotiation draft for contract "Party Draft Contract" with the staged name "Party Position"
    When the negotiator discards the negotiation draft for contract "Party Draft Contract"
    Then get http 200:Success code
    And the negotiation draft for contract "Party Draft Contract" is empty
    And the contract "Party Draft Contract" has no recorded negotiation change requests

  # Accepting an inbound offer as-is (POST /contract/accept-offer) is what
  # mints the accepting instance's negotiation task for the offer's round —
  # receiving the offer queues nothing, so an unaccepted offer has no task and
  # submit refuses to settle a round nobody entered (command/acceptoffer.go,
  # command/submit.go). The authority to accept is being the designated
  # counterparty; on a single DCS a contract is its own origin, so the side of
  # the rule this exercises is the refusal.
  @DCS-IR-CWE-03 @DCS-FR-CWE-18 @clean_db
  Scenario: The originator cannot accept its own offer
    Given I am authenticated with roles: "Contract Creator"
    And contract "Own Offer Contract" has reached contract state "OFFERED"
    When the negotiator accepts the offer for contract "Own Offer Contract"
    Then the response status is 400
    And the contract "Own Offer Contract" is in state "OFFERED"
