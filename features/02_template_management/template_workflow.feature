@UC-02-10 @REQ-remove-free-template-state-selection-AC5 @DCS-FR-UC-02-1
Feature: Template Approval Workflow
  Templates progress through submission, review, and approval
  before becoming available for contract generation.

  @clean_db
  Scenario: Submit template for review
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Draft" status
    When I submit template "Standard NDA"
    Then the template status is "Submitted"

  @clean_db @DCS-IR-TR-03
  Scenario: Approve submitted template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    And template "Standard NDA" is verified
    When I submit template "Standard NDA" with flag=approval
    Then the template status is "Reviewed"

  @clean_db
  Scenario: Reject submitted template
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    And template "Standard NDA" is verified
    When I submit template "Standard NDA" with flag=draft
    Then the template status is "Rejected"

  @clean_db
  Scenario: Reject reviewed template without reason
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I reject template "Standard NDA" without reason
    And I retrieve template "Standard NDA" by did
    Then the template status is "Reviewed"

  @clean_db @DCS-IR-TR-05 @DCS-FR-TR-15
  Scenario: Reject reviewed template with reason
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I reject template "Standard NDA" with reason "Missing compliance clause"
    Then the template status is "Rejected"
    And the rejection reason is recorded

  @clean_db @DCS-IR-TR-05
  Scenario: Resubmit reviewed template
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I resubmit template "Standard NDA"
    Then the template status is "Submitted"

  @clean_db @DCS-IR-TR-05 @DCS-FR-TR-15
  Scenario: Approve reviewed template
    Given I am authenticated with roles: "Template Approver"
    And template "Standard NDA" is in "Reviewed" status
    When I approve template "Standard NDA"
    Then the template status is "Approved"

  @clean_db @DCS-IR-TR-06
  Scenario: Register approved template
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" is in "Approved" status
    When I register template "Standard NDA"
    Then the template status is "Registered"

  @clean_db
  Scenario: Resubmit template for review
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Rejected" status
    When I submit template "Standard NDA"
    Then the template status is "Submitted"

  @clean_db
  Scenario: Archive submitted template
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" is in "Submitted" status
    When I delete template "Standard NDA"
    Then the template status is "Deleted"

  @clean_db
  Scenario: Archive approved template
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" is in "Registered" status
    When I delete template "Standard NDA"
    Then the template status is "Deprecated"

  @clean_db @DCS-FR-TR-06
  Scenario: Unauthorized role cannot approve template
    Given I am authenticated with roles: "Template Creator"
    And template "Standard NDA" is in "Reviewed" status
    When I approve template "Standard NDA"
    Then the request is denied with an authorization error

  # DCS-FR-TR-16: updating a registered template runs through the
  # copy-on-version scheme (POST /template/copy inherits the source's
  # base_template lineage at version+1); the updated copy is re-registered as
  # v2, the pre-update revision stays retrievable via
  # GET /template/history/{did}, and the predecessor's dashboard entry points
  # at the successor via latest_did.
  @clean_db @DCS-FR-TR-16
  Scenario: Updating a registered template produces a linked successor version
    Given I am authenticated with roles: "Template Creator"
    And template "Lineage NDA" is in "Registered" status
    When I register an updated version of template "Lineage NDA" named "Lineage NDA v2"
    Then template "Lineage NDA v2" has version 2
    And the template history for "Lineage NDA v2" preserves its pre-update revision
    And template "Lineage NDA" links to "Lineage NDA v2" as its latest version

  # DCS-IR-TR-04: review supports returning a submitted template to draft
  # with comments; the comments ride the SUBMIT_CONTRACT_TEMPLATE event into
  # the template's audit trail (command/submit.go persists cmd.Comments).
  @clean_db @DCS-IR-TR-04
  Scenario: Reviewer returns a submitted template to draft with comments
    Given I am authenticated with roles: "Template Reviewer"
    And template "Standard NDA" is in "Submitted" status
    And template "Standard NDA" is verified
    When I return template "Standard NDA" to draft with comment "Missing liability clause"
    Then the template status is "Rejected"
    And the template audit log for "Standard NDA" records the review comment "Missing liability clause"
