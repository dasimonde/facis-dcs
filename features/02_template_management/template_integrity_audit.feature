# Template integrity verification and audit log (DCS-FR-TR-20, DCS-FR-TR-21,
# DCS-FR-TR-05): POST /template/verify and GET /template/audit
# (backend/design/template_repository.go). Verify itself is already exercised
# as a setup step elsewhere (template_workflow.feature's "template is
# verified" Given), but no scenario asserts the actual claim of DCS-FR-TR-20
# ("integrity confirmed") or that the verify action lands in the template's
# own audit trail (DCS-FR-TR-21/DCS-FR-TR-05) - this file adds both.

@DCS-FR-TR-20 @DCS-FR-TR-21 @DCS-FR-TR-05 @UC-02
Feature: Template integrity verification and audit log

  @clean_db @DCS-FR-TR-20
  Scenario: Verifying an approved template confirms its integrity
    Given I am authenticated with roles: "Template Manager"
    And template "Integrity Verify Template" is in "Approved" status
    When I verify template "Integrity Verify Template"
    Then get http 200:Success code
    And the template verification reports no findings

  @clean_db @DCS-FR-TR-21 @DCS-FR-TR-05 @DCS-FR-TR-08
  Scenario: Template audit log records a verify action
    Given I am authenticated with roles: "Template Manager"
    And template "Audit Log Template" is in "Approved" status
    When I verify template "Audit Log Template"
    Then get http 200:Success code
    And the template audit log for "Audit Log Template" includes an action of type "VERIFY_CONTRACT_TEMPLATE"

  # DCS-FR-TR-21: the audit log records creation, modification, approval, and
  # deletion activities — one full lifecycle pass, then each action type is
  # asserted in the template's own audit trail. Deleting from Approved is a
  # hard archive (state DELETED); only REGISTERED/PUBLISHED archive to
  # DEPRECATED (backend command/archive.go), so the ARCHIVE event covers the
  # deletion activity here.
  @clean_db @DCS-FR-TR-21
  Scenario: Template audit log records creation, modification, approval, and deletion
    Given I am authenticated with roles: "Template Creator"
    And template "Audit Lifecycle Template" is in "Draft" status
    When I update template "Audit Lifecycle Template" name to "Audit Lifecycle Template Revised"
    And I submit template "Audit Lifecycle Template"
    And I am authenticated with roles: "Template Reviewer"
    And I verify template "Audit Lifecycle Template"
    And I submit template "Audit Lifecycle Template" with flag=approval
    And I am authenticated with roles: "Template Approver"
    And I approve template "Audit Lifecycle Template"
    And I am authenticated with roles: "Template Manager"
    And I delete template "Audit Lifecycle Template"
    Then the template audit log for "Audit Lifecycle Template" includes an action of type "CREATE_CONTRACT_TEMPLATE"
    And the template audit log for "Audit Lifecycle Template" includes an action of type "UPDATE_CONTRACT_TEMPLATE"
    And the template audit log for "Audit Lifecycle Template" includes an action of type "APPROVE_CONTRACT_TEMPLATE"
    And the template audit log for "Audit Lifecycle Template" includes an action of type "ARCHIVE_CONTRACT_TEMPLATE"
