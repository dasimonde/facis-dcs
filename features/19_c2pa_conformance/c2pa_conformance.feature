# C2PA remote manifest and verifier banner completeness
# (SRS: DCS-OR-C2PA-006/-008).
#
# Scope: the public (auth-free) manifest endpoint
# GET /c2pa/manifest/{contract_did} serving the raw C2PA manifest store
# (Content-Type: application/c2pa) and, with ?history=true, a parsed JSON
# enumeration of the manifest chain (labels + dcs.lifecycle assertions);
# the embedded manifest's dcs.remote_manifests assertion referencing that
# endpoint; the verification of a copy whose embedded manifest is removed,
# from the remote manifest it references; the verify endpoint's
# lifecycle_status banner per contract state; and the verify response's four
# independently named checks (PDF signature, C2PA manifest, VC signature,
# status list).
#
# Lifecycle banner mapping (provenance.MapCWEStateToC2PA,
# backend/internal/pdfgeneration/provenance/lifecycle.go): DRAFT -> draft,
# SIGNED/ACTIVE -> active, TERMINATED -> terminated, REVOKED -> suspended,
# EXPIRED -> expired.
#   - "Expired" needs its own scenario: contract/update refuses exp_date
#     values less than one day in the future, so the pack backdates
#     exp_date directly via the context.db test seam and gives the
#     already-running expiry cron (1-minute poll) a short window to flip
#     the contract to EXPIRED. Test-support seam only; no production
#     behavior is changed by it.
#   - "Replaced" is not a reachable ContractState via any wired command
#     (contractstate/transition.go); its scenario is @skip and the state is
#     a recorded deviation.
#
# Strip-then-verify: the step removes the content_credential.c2pa attachment
# from a local copy of the exported PDF and posts the copy to
# POST /pdf/verify/contract/{did}. The copy keeps its XMP dcterms:provenance
# reference; the verifier follows it to this contract's remote manifest,
# verifies the contract PDF that holds that manifest, and requires the copy
# to show the same pages and carry the same payload
# (backend/internal/pdfgeneration/query/verifycopy.go).

@DCS-OR-C2PA-008
Feature: C2PA remote manifest, verifier banner completeness

  Background:
    Given I am authenticated with roles: "Contract Manager"

  @DCS-OR-C2PA-008
  Scenario: The public C2PA manifest endpoint returns the raw manifest store without auth
    Given contract "C2PA Manifest Contract" has reached contract state "SIGNED"
    And contract "C2PA Manifest Contract" has an exported PDF
    When I request the public C2PA manifest for contract "C2PA Manifest Contract"
    Then get http 200:Success code
    And the response has Content-Type "application/c2pa"
    And the response body is a non-empty C2PA JUMBF manifest store

  @DCS-OR-C2PA-008
  Scenario: The manifest history query returns a parsed JSON chain enumeration instead of raw bytes
    Given contract "C2PA History Contract" has reached contract state "SIGNED"
    And contract "C2PA History Contract" has an exported PDF
    When I request the C2PA manifest history for contract "C2PA History Contract"
    Then get http 200:Success code
    And the response is a JSON list of manifest labels with dcs.lifecycle assertions

  @DCS-OR-C2PA-008
  Scenario: The embedded manifest's dcs.remote_manifests assertion references the public manifest endpoint
    Given contract "C2PA Remote Ref Contract" has reached contract state "SIGNED"
    And contract "C2PA Remote Ref Contract" has an exported PDF
    When I request the public C2PA manifest for contract "C2PA Remote Ref Contract"
    Then get http 200:Success code
    And the C2PA manifest response for contract "C2PA Remote Ref Contract" declares a remote_manifests field pointing to its own public manifest endpoint

  @DCS-OR-C2PA-008
  Scenario: A copy without its embedded manifest is verified from the remote manifest it references
    Given contract "C2PA Stripped Contract" has reached contract state "SIGNED"
    And contract "C2PA Stripped Contract" has an exported PDF
    And a local copy of contract "C2PA Stripped Contract"'s PDF with the content_credential.c2pa attachment removed
    When the local copy of contract "C2PA Stripped Contract" is verified via the backend verify endpoint
    Then the verification result reports c2pa_manifest_found is true, served from the remote manifest fallback
    And the verification result shows match is true

  @DCS-OR-C2PA-006 @DCS-IR-CWE-13
  Scenario Outline: The verify endpoint reports the matching lifecycle_status banner for each state
    Given contract "<name>" has reached contract state "<contract_state>"
    When contract "<name>" is exported and verified as PDF
    Then get http 200:Success code
    And the C2PA lifecycle_status for contract "<name>" is "<banner>"

    Examples: reachable via existing/extended wired commands
      | name                          | contract_state | banner     |
      | C2PA Banner Draft             | DRAFT           | draft      |
      | C2PA Banner Active-via-Signed | SIGNED          | active     |
      | C2PA Banner Terminated        | TERMINATED      | terminated |
      | C2PA Banner Suspended         | REVOKED         | suspended  |

  @DCS-OR-C2PA-006 @DCS-FR-CWE-10
  Scenario: The verify endpoint reports the "expired" lifecycle_status banner after the expiry cron fires
    # "Expired" needs an extra Given step beyond the shared
    # "has reached contract state" one-liner (see the header comment above):
    # the contract must first reach a non-terminal state (SIGNED), then have
    # its exp_date backdated via the context.db test seam, then the already
    # -running expiry cron (polls every 1 minute) is given a short window to
    # force-flip it to EXPIRED before verifying.
    Given contract "C2PA Banner Expired" has reached contract state "SIGNED"
    And contract "C2PA Banner Expired" has an expiry date in the past
    When contract "C2PA Banner Expired" is exported and verified as PDF
    Then get http 200:Success code
    And the C2PA lifecycle_status for contract "C2PA Banner Expired" is "expired"

  @DCS-OR-C2PA-006 @skip
  Scenario: Replaced lifecycle banner is a recorded deviation, not a BDD result
    # "Replaced" is not a reachable ContractState in
    # contractstate/transition.go (lifecycle.go passes the lowercase string
    # through for callers that already use SRS vocabulary), and no trigger
    # forces it here. The @skip keeps the scenario out of both the passed and
    # the failed counts. The state is a recorded deviation.
    Given I am authenticated with roles: "Contract Manager"

  @DCS-OR-C2PA-006
  Scenario: The verify response carries four independently named checks, and the PDF signature is validated by the EU DSS
    Given contract "C2PA Four Checks Contract" has reached contract state "SIGNED"
    And contract "C2PA Four Checks Contract" has an exported PDF
    When contract "C2PA Four Checks Contract" is exported and verified as PDF
    Then get http 200:Success code
    And the verify response for contract "C2PA Four Checks Contract" includes four named checks: PDF signature, C2PA manifest, VC signature, and status list
    And the PDF signature check for contract "C2PA Four Checks Contract" is reported valid by the EU DSS
