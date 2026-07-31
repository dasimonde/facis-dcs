# Contract Review evidence (SRS 3.1.1 "Contract Review", DCS-IR-CWE-06):
# a Reviewer responds to a SUBMITTED contract via POST /contract/submit with
# the forward_to action flag (backend/design/contract_workflow_engine.go's
# ContractSubmitRequest: "approval" routes to REVIEWED, "reject" reopens
# NEGOTIATION). The reviewer's finding comment travels in the same payload's
# comments array and is persisted on the SUBMIT_CONTRACT audit event
# (SubmitEvent.Comments/ActionFlag in backend/internal/
# contractworkflowengine/event/event.go); the SUBMITTED branch of
# command/submit.go reopens the review/negotiation/approval task rows and
# returns the contract to NEGOTIATION.
#
# The happy forward-for-approval half of DCS-IR-CWE-06 is already exercised
# end-to-end by the standard-workflow scenarios in
# 03_contract_creation/contract_state_machine_refactor.feature (reviewer
# submit with forward_to=approval on the way to APPROVED) — this feature
# adds the findings/request-modification half.

@UC-03 @SRS-3.1.1
Feature: Contract review

  @DCS-IR-CWE-06
  Scenario: Reviewer returns a submitted contract for modification with a finding
    Given contract "Review Finding Contract" has reached contract state "SUBMITTED"
    When the reviewer returns contract "Review Finding Contract" for modification with finding "Clause 3 lacks a data-retention limit"
    Then get http 200:Success code
    And the contract "Review Finding Contract" is in state "NEGOTIATION"
    And the SUBMIT_CONTRACT audit event for contract "Review Finding Contract" records the reviewer finding "Clause 3 lacks a data-retention limit"
