@UC-03-04 @FR-CWE-15 @FR-CWE-16 @FR-CWE-25 @FR-PACM-03 @FR-PACM-02 @DCS-FR-UC-03-3
Feature: Contract Approval
  Contract Approvers and Contract Managers route contracts to required
  approvers before signing. The system logs approvals with timestamps,
  locks content upon completion, and transitions to signing phase.

  Scenario: Initiate approval process for finalized contract
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in "Under Review" status
    When I initiate the approval process for contract "Service Agreement"
    Then the contract is routed to required approvers
    And the contract status shows "Pending Approval"

  Scenario: Approve contract via approval interface
    Given I am authenticated with roles: "Contract Approver"
    And contract "Service Agreement" requires my approval
    When I access the approval interface for contract "Service Agreement"
    And I approve contract "Service Agreement"
    Then my approval is logged with timestamp
    And my approval is logged with digital credentials
    And the approval status is updated

  @DCS-IR-CWE-09
  Scenario: Reject contract with comments
    Given I am authenticated with roles: "Contract Approver"
    And contract "Service Agreement" requires my approval
    When I reject contract "Service Agreement" with reason "Missing compliance clause"
    Then the rejection is logged with comments and timestamp
    And the contract status shows "Rejected"
    And the contract is returned for revision

  # "All required approvals gathered" (partial-quorum proof) lives in
  # 17_peer_trust/two_instance_peer_trust.feature's approval-quorum scenario: approvers are PEERS
  # (CauserDID is always the executing instance's own peer DID and
  # UpdateState matches WHERE approver = CauserDID), so two observably
  # distinct approvals require two instances — that scenario approves
  # from A, proves the contract stays REVIEWED, approves from B, and proves
  # APPROVED replicates with both approval tasks recorded.

  @DCS-FR-CWE-19
  Scenario: Contract transitions to signing phase upon approval
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" has all required approvals
    When the approval process completes
    Then the contract is marked as "Approved"
    And the contract transitions to the signing phase

  Scenario: Approval interface supports highlighting and comments
    Given I am authenticated with roles: "Contract Approver"
    And contract "Service Agreement" requires my approval
    When I access the approval interface for contract "Service Agreement"
    Then I can highlight sections for attention
    And I can add comments to specific clauses
    And I can view previous reviewer comments

  Scenario: Automated compliance check during approval
    Given I am authenticated with roles: "Contract Approver"
    And contract "Service Agreement" is pending approval
    When automated compliance checks are performed
    Then the system validates against regulatory frameworks
    And the system validates against organizational policies
    And compliance issues are flagged for review

  # GET /pac/monitor (backend/internal/processauditandcompliance/query/
  # querymonitor.go) sweeps OPEN approval tasks and flags contracts in an
  # approval-pending state (SUBMITTED/REVIEWED) as MISSING_APPROVAL risks.
  # Approvers are responsible peers (peer DIDs), not individual user roles,
  # so the missing approval is attributed to a peer — the earlier draft of
  # this scenario ("from Risk Officer") assumed per-user approvers the
  # product does not have.
  @DCS-FR-PACM-03 @DCS-IR-PACM-03 @DCS-FR-UC-08-2 @DCS-FR-UC-10-2
  Scenario: Compliance monitoring detects risk during approval
    Given contract "Monitor Risk Contract" is pending approval
    And contract "Monitor Risk Contract" still has an open required approval task
    When the Compliance Officer requests continuous monitoring
    Then get http 200:Success code
    And the monitoring sweep flags contract "Monitor Risk Contract" with a "MISSING_APPROVAL" compliance risk
    And the flagged risk for contract "Monitor Risk Contract" is recorded in the PAC audit trail

  @DCS-FR-CWE-27
  Scenario: Track approval routing status
    Given I am authenticated with roles: "Contract Manager"
    And contract "Service Agreement" is in approval workflow
    When I view approval status for contract "Service Agreement"
    Then I see pending approvals
    And I see completed approvals with timestamps
    And I see the overall approval progress

  Scenario: Unauthorized role cannot approve contracts
    Given I am authenticated with roles: "Contract Observer"
    And contract "Service Agreement" is pending approval
    When I attempt to approve contract "Service Agreement"
    Then the request is denied with an authorization error
