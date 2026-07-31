# Contract deployment, execution evidence, and KPIs (SRS: DCS-FR-SM-10/-12,
# DCS-FR-CWE-06/-09/-20/-31, DCS-IR-SI-02/-05).
#
# Endpoint surface exercised here:
#   - POST /contract/deploy (manual deploy trigger, UC-05-01)
#   - POST /contract/deployment/callback (target -> DCS ack/status/KPI,
#     authenticated as the target's own registered client, ADR-27)
#   - GET /contract/retrieve/{did} "kpis" field
#   - archive entries (GET /archive/search) with an
#     evidence.deployment{correlation_id, payload_hash, receipt_hash,
#     tsa_token, activated_at} sub-object
# See steps/contract_deployment/dcs_contract_deployment_steps.py's module
# docstring for the force-set DB test seam and the ORCE contract-target-flow
# wiring.

@DCS-FR-UC-05-1 @DCS-FR-UC-13-1
Feature: Contract deployment, execution evidence, and KPIs

  @DCS-FR-CWE-20 @DCS-FR-CSA-08 @DCS-FR-UC-07-1
  Scenario: Archive entry is created only when the contract reaches SIGNED, not at APPROVED
    Given contract "Archive Trigger Contract" has reached contract state "APPROVED"
    Then the archive has no entry for contract "Archive Trigger Contract"
    When the counterparty signer applies a signature to contract "Archive Trigger Contract"
    Then get http 200:Success code
    And the archive has an entry for contract "Archive Trigger Contract"

  @DCS-FR-CWE-31 @DCS-FR-CWE-20
  Scenario: An archived contract in state ACTIVE still appears in the live contract list
    Given I am authenticated with roles: "Contract Manager"
    And contract "Live Archived Contract" has reached contract state "SIGNED"
    And contract "Live Archived Contract" is force-set to state "ACTIVE" directly in the database (pre-deploy test seam, bypassing the deployment chain)
    When the contract search endpoint is queried with state filter "ACTIVE"
    Then the search results include contract "Live Archived Contract"
    And the archive has an entry for contract "Live Archived Contract"

  @DCS-FR-SM-12 @UC-05-01
  Scenario: An authorized user deploys a SIGNED contract to the configured Contract Target System
    Given contract "Deploy Signed Contract" has reached contract state "SIGNED"
    When an authorized user deploys contract "Deploy Signed Contract" to the configured contract target
    Then get http 200:Success code
    And the deployment response includes a correlation ID

  @DCS-FR-SM-12 @DCS-IR-SI-05
  Scenario: The deployment payload declares the machine-readable JSON-LD, DID, version, hash, timestamp, and odrl:Set
    Given contract "Deploy Payload Contract" has reached contract state "SIGNED"
    When an authorized user deploys contract "Deploy Payload Contract" to the configured contract target
    Then get http 200:Success code
    And the deployment response declares the contract DID, version, content hash, timestamp, and the odrl:Set policy for "Deploy Payload Contract"

  @DCS-NFR-BR-03 @DCS-FR-SM-12
  Scenario: A contract that is not SIGNED is rejected for deployment
    Given contract "Draft Deploy Rejection Contract" has reached contract state "DRAFT"
    When an authorized user deploys contract "Draft Deploy Rejection Contract" to the configured contract target
    Then the request is denied with a client error

  @DCS-FR-CWE-06
  Scenario: Deployment is triggered automatically once the signing workflow completes
    Given contract "Auto Deploy Contract" has reached contract state "APPROVED"
    And contract "Auto Deploy Contract" deploys to the configured target system
    When the counterparty signer applies a signature to contract "Auto Deploy Contract"
    Then get http 200:Success code
    And the archive entry for contract "Auto Deploy Contract" records an automatic deployment correlation ID

  @DCS-IR-SI-05
  Scenario: The deployment callback rejects a caller that is not this deployment's target
    Given contract "Callback Auth Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "Callback Auth Contract" to the configured contract target
    And get http 200:Success code
    When another registered system sends a deployment callback for contract "Callback Auth Contract"
    Then the callback request is rejected because that caller is not this deployment's target

  @DCS-IR-SI-02 @DCS-IR-SI-05 @DCS-IR-CI-07
  Scenario: The shipped ORCE contract-target-flow verifies the content hash and returns a matching ack
    Given contract "ORCE Ack Contract" has reached contract state "SIGNED"
    And the example ORCE contract-target-flow is reachable
    When a deployment payload for contract "ORCE Ack Contract" is posted directly to the ORCE contract-target-flow
    Then the ORCE flow acknowledges with correlation_id, payload_hash, and activated_at matching the sent payload

  # The acknowledgement below is sent by the REAL contract target: the
  # backend dispatches to the shipped ORCE contract-target-flow
  # (CONTRACT_TARGET_URL), which verifies the payload hash and POSTs the
  # authoritative ack callback itself — no harness-simulated callback.
  @DCS-FR-SM-10 @DCS-IR-SI-02 @DCS-FR-CSA-03
  Scenario: The execution-evidence receipt is TSA-timestamped and appended to the archive entry
    Given contract "TSA Evidence Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "TSA Evidence Contract" to the configured contract target
    And get http 200:Success code
    When the contract target acknowledges the deployment of contract "TSA Evidence Contract"
    Then the archive entry for contract "TSA Evidence Contract" contains an RFC-3161 TSA timestamp over the execution-evidence receipt

  @DCS-FR-SM-12 @DCS-IR-SI-02
  Scenario: An acknowledged deployment moves the contract from SIGNED to ACTIVE
    Given contract "Ack Activates Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "Ack Activates Contract" to the configured contract target
    And get http 200:Success code
    When the contract target acknowledges the deployment of contract "Ack Activates Contract"
    Then the contract "Ack Activates Contract" is in state "ACTIVE"

  # The target system itself reports a KPI it genuinely measures — the
  # latency between receiving the dispatch and activating the contract —
  # over the callback channel it authenticates on (DCS-FR-CWE-31: "KPIs ...
  # sent from the target system"). No rule in the deployed policy constrains
  # that latency, so the flow concludes nothing about it and states no
  # verdict; the DCS records the measurement as not evaluated rather than
  # reading silence as compliance (ADR-33).
  @DCS-FR-CWE-31 @DCS-FR-CWE-09 @DCS-IR-SI-02 @DCS-IR-SI-05
  Scenario: The contract target itself reports a measured KPI over the callback channel
    Given contract "Target Reported KPI Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "Target Reported KPI Contract" to the configured contract target
    And get http 200:Success code
    When the contract target acknowledges the deployment of contract "Target Reported KPI Contract"
    Then the contract detail for "Target Reported KPI Contract" shows a target-reported KPI "activation_latency_ms" recorded as "not_evaluated"

  @DCS-FR-CWE-31 @DCS-FR-CWE-09
  Scenario: A KPI reported via callback for an ACTIVE contract appears on the contract detail
    Given contract "KPI Dashboard Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "KPI Dashboard Contract" to the configured contract target
    And get http 200:Success code
    And the contract target acknowledges the deployment of contract "KPI Dashboard Contract"
    When the target reports a KPI value "uptime_percent" = "99.5" for contract "KPI Dashboard Contract"
    Then get http 200:Success code
    And the contract detail for "KPI Dashboard Contract" shows KPI "uptime_percent" with value "99.5"

  # The target system executes the contract, so it is the component that
  # classifies (ADR-33): the callback carries its verdict on the rule it
  # concluded about, and the DCS records that verdict instead of deriving a
  # second opinion from a value it cannot see the context of.
  @DCS-FR-CWE-09 @DCS-FR-UC-06-1
  Scenario: A KPI the target concludes breaches a contractual rule is recorded as violated
    Given contract "KPI Violation Contract" is a fresh draft whose ODRL policy constrains field "coverage" using operator "gteq" against "95" while the actual value is "95"
    And contract "KPI Violation Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And an authorized user deploys contract "KPI Violation Contract" to the configured contract target
    And get http 200:Success code
    And the contract target acknowledges the deployment of contract "KPI Violation Contract"
    When the target reports KPI "coverage_percent" = "80" for contract "KPI Violation Contract", concluding "violated" on the rule it deployed
    Then get http 200:Success code
    And the contract detail for "KPI Violation Contract" records KPI "coverage_percent" with verdict "violated"
    And the semantic KPI observations for "KPI Violation Contract" record "coverage_percent" as "violated" against the rule it deployed

  # A verdict is evidence about ONE term of the signed contract, so it has to
  # name that term: the rule @id the target quotes back is the @id that
  # travelled to it verbatim in the deployment envelope's odrl:policy, which
  # is what makes the recorded row traceable to the clause it judges.
  @DCS-FR-CWE-09 @DCS-FR-CWE-31 @DCS-IR-SI-05
  Scenario: A recorded verdict names the ODRL rule it concerns
    Given contract "KPI Attribution Contract" is a fresh draft whose ODRL policy constrains field "coverage" using operator "gteq" against "95" while the actual value is "95"
    And contract "KPI Attribution Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And an authorized user deploys contract "KPI Attribution Contract" to the configured contract target
    And get http 200:Success code
    And the contract target acknowledges the deployment of contract "KPI Attribution Contract"
    When the target reports KPI "coverage_percent" = "99" for contract "KPI Attribution Contract", concluding "satisfied" on the rule it deployed
    Then get http 200:Success code
    And the contract detail for "KPI Attribution Contract" attributes KPI "coverage_percent" to the rule it deployed
    And the semantic KPI observations for "KPI Attribution Contract" record "coverage_percent" as "satisfied" against the rule it deployed

  # An untraceable conclusion is a malformed report, not a verdict: the DCS
  # may refuse it (ADR-33), which is the only way the rule @id on a recorded
  # row can be relied on afterwards.
  @DCS-FR-CWE-09 @DCS-IR-SI-05
  Scenario: A verdict naming a rule the contract never deployed is refused
    Given contract "KPI Foreign Rule Contract" is a fresh draft whose ODRL policy constrains field "coverage" using operator "gteq" against "95" while the actual value is "95"
    And contract "KPI Foreign Rule Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And an authorized user deploys contract "KPI Foreign Rule Contract" to the configured contract target
    And get http 200:Success code
    And the contract target acknowledges the deployment of contract "KPI Foreign Rule Contract"
    When the target reports KPI "coverage_percent" = "80" for contract "KPI Foreign Rule Contract", concluding "violated" on rule "urn:uuid:rule-from-another-contract"
    Then the response status is 400

  # The DCS holds the terms and the target holds the events, so a report that
  # states no conclusion leaves the rule unobserved. Recording that as
  # compliance would turn a check that never ran into a green row, which is
  # precisely what ADR-33 removes.
  @DCS-FR-CWE-09 @DCS-FR-CWE-31
  Scenario: A KPI report carrying no verdict is recorded as not evaluated, never as compliance
    Given contract "KPI Unevaluated Contract" is a fresh draft whose ODRL policy constrains field "coverage" using operator "gteq" against "95" while the actual value is "95"
    And contract "KPI Unevaluated Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And an authorized user deploys contract "KPI Unevaluated Contract" to the configured contract target
    And get http 200:Success code
    And the contract target acknowledges the deployment of contract "KPI Unevaluated Contract"
    When the target reports a KPI value "coverage_percent" = "80" for contract "KPI Unevaluated Contract"
    Then get http 200:Success code
    And the contract detail for "KPI Unevaluated Contract" records KPI "coverage_percent" with verdict "not_evaluated"
    And the contract detail for "KPI Unevaluated Contract" attributes no rule to KPI "coverage_percent"
    And the semantic KPI observations for "KPI Unevaluated Contract" record "coverage_percent" as "not_evaluated" naming no rule

  # DCS-FR-CWE-31 requires more than recording the breach: "Alerts MUST be
  # raised for underperformance or missed targets". A verdict sitting on a
  # contract nobody opens is not an alert, so the compliance monitor — the
  # surface an officer actually watches — must surface it too. Alerting on a
  # reported breach stays the DCS's own competence under ADR-33; only the
  # classification moved.
  @DCS-FR-CWE-31 @DCS-FR-PACM-03 @DCS-IR-PACM-03
  Scenario: A breached KPI raises an underperformance alert on the compliance monitor
    Given contract "KPI Alert Contract" is a fresh draft whose ODRL policy constrains field "coverage" using operator "gteq" against "95" while the actual value is "95"
    And contract "KPI Alert Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And an authorized user deploys contract "KPI Alert Contract" to the configured contract target
    And get http 200:Success code
    And the contract target acknowledges the deployment of contract "KPI Alert Contract"
    And the target reports KPI "coverage_percent" = "80" for contract "KPI Alert Contract", concluding "violated" on the rule it deployed
    And get http 200:Success code
    When the Compliance Officer requests continuous monitoring
    Then get http 200:Success code
    And the monitoring sweep flags contract "KPI Alert Contract" with a "CONTRACT_UNDERPERFORMANCE" compliance risk
    And the flagged risk for contract "KPI Alert Contract" is recorded in the PAC audit trail
