# Lifecycle notifications through the DCS webhook platform
# (backend/internal/webhookplatform, mounted at /orce/ on the service
# root), which fans contract/template lifecycle events out to registered
# subscribers; the ORCE monitoring flow
# (charts/orce/flows/event-webhook-orce-flow.json, POST /dcs-dispatch) is
# the deployed receiver these scenarios deliver to.
# - DCS-FR-TR-22: the system SHOULD notify Template Users when a contract
#   template they have used has been updated or deprecated.
# - DCS-FR-CSA-20 / DCS-FR-CWE-10: rules-based monitoring MUST trigger
#   alerts for expired contracts, deliverable via API.
# Each notification payload carries the subject DID, which is how a
# subscriber matches notifications to the templates/contracts it uses.
# GET /deliveries is the platform's monitoring surface: per-notification
# callback URL, HTTP status, and acknowledgement.

@UC-02
Feature: Lifecycle notifications through the webhook platform

  @DCS-FR-TR-22
  Scenario: A subscriber is notified when a template it uses is updated
    Given I am authenticated with roles: "Template Creator"
    And template "Notify Update Template" is in "Draft" status
    And a webhook subscription for "template.updated" events pointing at the ORCE monitoring flow
    When I update template "Notify Update Template" name to "Notify Update Template v2"
    Then get http 200:Success code
    And the "template.updated" notification for template "Notify Update Template" is delivered to the ORCE receiver

  @DCS-FR-TR-22
  Scenario: A subscriber is notified when a template it uses is deprecated
    # Archiving a REGISTERED template flips it to DEPRECATED and emits
    # ARCHIVE_CONTRACT_TEMPLATE (templaterepository/command/archive.go),
    # which the platform maps to "template.deprecated".
    Given I am authenticated with roles: "Template Manager"
    And template "Notify Deprecate Template" is in "Registered" status
    And a webhook subscription for "template.deprecated" events pointing at the ORCE monitoring flow
    When I deprecate template "Notify Deprecate Template"
    Then the template status is "Deprecated"
    And the "template.deprecated" notification for template "Notify Deprecate Template" is delivered to the ORCE receiver

  @DCS-FR-CSA-20 @DCS-FR-CWE-10
  Scenario: A subscriber is alerted when a contract expires
    # The expiry trigger reuses the exp_date-backdate DB seam + cron wait
    # from the C2PA "expired" banner scenario (see that step's docstring):
    # the backdate step itself waits for the already-running expiry cron
    # (1-minute poll) to persist EXPIRED, and persisting EXPIRED is what
    # emits the CONTRACT_EXPIRED event the webhook platform maps to
    # "contract.expired" — hence the subscription is registered before the
    # backdate, and the scenario's whole trigger sits in the Given block.
    Given contract "Notify Expired Contract" has reached contract state "SIGNED"
    And a webhook subscription for "contract.expired" events pointing at the ORCE monitoring flow
    And contract "Notify Expired Contract" carries the expiration policy "ARCHIVING" (cron-processing test seam)
    And contract "Notify Expired Contract" has an expiry date in the past
    Then the "contract.expired" notification for contract "Notify Expired Contract" is delivered to the ORCE receiver

  @DCS-FR-TR-22
  Scenario: Subscribing to an event outside the published catalogue is rejected
    When a webhook subscription for the unknown event "template.exploded" is attempted
    Then the webhook subscription is rejected as unknown

  @DCS-FR-TR-22
  Scenario: The webhook platform rejects unauthenticated subscriptions
    When an unauthenticated webhook subscription for "template.updated" is attempted
    Then the webhook subscription is rejected as unauthorized
