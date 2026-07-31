# System-Based Contract Management (UC-12, High priority per SRS §4/Table 6):
# create -> review -> approve -> sign -> archive, driven purely through the
# API (no UI), each step updating status with signature evidence stored and
# an archive entry created, full audit chain present. This BDD suite already
# talks to every endpoint over plain HTTP with no browser automation, so a
# "system-based" (machine-to-machine) flow is exercised identically to a
# human-triggered one — the distinguishing acceptance claim from Table 7
# (UC-12-01..05) is that each step is independently API-callable,
# authenticated, and produces the same observable state/evidence chain as the
# UI-driven workflows in 03_contract_creation and 22_real_signing_vertical.
# This file assembles the full chain in one scenario rather than duplicating
# the individual per-step assertions those packs already make.

@UC-12 @DCS-FR-CWE-13 @DCS-FR-CWE-28
Feature: Contract lifecycle driven entirely through the API

  @UC-12-01 @UC-12-02 @UC-12-03 @UC-12-05 @DCS-FR-UC-10-1 @DCS-FR-UC-11-1 @DCS-FR-UC-12-1 @DCS-FR-UC-12-2 @DCS-FR-UC-12-3 @DCS-FR-UC-12-5
  Scenario: Create, review, approve, sign, and archive a contract entirely via API calls
    Given contract "API Lifecycle Contract" is in "Draft" status
    Then the contract "API Lifecycle Contract" has an audit event of type "CREATE_CONTRACT"
    When contract "API Lifecycle Contract" is submitted, reviewed, and approved via the standard workflow
    Then the contract "API Lifecycle Contract" is in state "APPROVED"
    And the contract "API Lifecycle Contract" has an audit event of type "APPROVE_CONTRACT"
    When the counterparty signer applies a signature to contract "API Lifecycle Contract"
    Then get http 200:Success code
    And the contract "API Lifecycle Contract" has completed signing
    And the archive has an entry for contract "API Lifecycle Contract"

  @UC-12-04 @DCS-FR-UC-12-4
  Scenario: Contract metadata and history are queryable via API after the lifecycle completes
    Given contract "API Query Contract" has reached contract state "APPROVED"
    When the contract search endpoint is queried with state filter "APPROVED"
    Then the search results include contract "API Query Contract"

  # DCS-FR-CWE-28's rate-limits clause: authenticated API interactions are
  # budgeted per credential (middleware.RateLimitAuthenticated, fixed
  # one-minute window, DCS_API_RATE_LIMIT_PER_MINUTE, default 3000). The
  # hammer uses a DEDICATED credential so the rest of the suite's budgets
  # are untouched, and a second credential proves the limit is per-client,
  # not global.
  @DCS-FR-CWE-28
  Scenario: API requests beyond the per-credential rate limit are rejected with 429
    Given a dedicated API credential for rate-limit probing
    When that credential issues more API requests within a minute than the configured rate limit
    Then at least one request is rejected with http 429 naming the rate limit
    And API requests with a different credential still succeed
