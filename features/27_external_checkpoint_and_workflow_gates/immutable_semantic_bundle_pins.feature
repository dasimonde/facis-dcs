# A contract's Semantic Hub references are selected once by the server. Draft
# editing may replace the business document, but it must neither delete nor
# accept client-selected versions of that immutable bundle.

@DCS-FR-TR-03 @DCS-IR-CWE-01 @DCS-FR-UC-03-1 @UC-03-01
Feature: Preserve server-controlled semantic bundle pins while editing and submitting drafts

  @REQ-preserve-immutable-semantic-bundle-pins-AC1
  Scenario: Updating business content without pin fields preserves the bundle and remains submittable
    Given contract "Pinless Draft Update" is ready for the "submission" gate
    And the server-controlled semantic bundle of contract "Pinless Draft Update" is remembered
    When contract "Pinless Draft Update" is updated with business title "Pinless title accepted" and no semantic pin fields
    Then the draft update for contract "Pinless Draft Update" succeeds
    And contract "Pinless Draft Update" stores business title "Pinless title accepted"
    And contract "Pinless Draft Update" still has the remembered server-controlled semantic bundle
    Given the workflow-gate executor is reset and returns a valid empty success
    When the "submission" gate is requested for contract "Pinless Draft Update"
    Then the "submission" transition succeeds for contract "Pinless Draft Update"
    And contract "Pinless Draft Update" still has the remembered server-controlled semantic bundle

  @REQ-preserve-immutable-semantic-bundle-pins-AC2
  Scenario Outline: Removed or manipulated client pins cannot change a stored bundle during update
    Given contract "Protected Update <client pins>" is ready for the "submission" gate
    And the server-controlled semantic bundle of contract "Protected Update <client pins>" is remembered
    When contract "Protected Update <client pins>" is updated with business title "Protected <client pins> title" and "<client pins>" semantic pin fields
    Then the draft update for contract "Protected Update <client pins>" succeeds
    And contract "Protected Update <client pins>" stores business title "Protected <client pins> title"
    And contract "Protected Update <client pins>" still has the remembered server-controlled semantic bundle

    Examples:
      | client pins |
      | removed     |
      | manipulated |

  @REQ-preserve-immutable-semantic-bundle-pins-AC3
  Scenario Outline: Optional submit content cannot replace pins used by persistence or the workflow gate
    Given contract "Protected Submit <client pins>" is ready for the "submission" gate
    And the server-controlled semantic bundle of contract "Protected Submit <client pins>" is remembered
    And the workflow-gate executor is reset and returns a valid empty success
    When the submission gate for contract "Protected Submit <client pins>" includes business title "Submitted <client pins> title" and "<client pins>" semantic pin fields
    Then the "submission" transition succeeds for contract "Protected Submit <client pins>"
    And contract "Protected Submit <client pins>" stores business title "Submitted <client pins> title"
    And contract "Protected Submit <client pins>" still has the remembered server-controlled semantic bundle
    And one correlated "submission" workflow-gate request was observed
    And that workflow-gate request used the remembered server-controlled semantic bundle

    Examples:
      | client pins |
      | absent      |
      | manipulated |
