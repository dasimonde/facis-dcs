# C2PA remote manifest and verifier banner completeness
# (SRS: DCS-OR-C2PA-006/-008).
#
# Scope: the public (auth-free) manifest endpoint
# GET /c2pa/manifest/{contract_did} serving the raw C2PA manifest store
# (Content-Type: application/c2pa) and, with ?history=true, a parsed JSON
# enumeration of the manifest chain (labels + dcs.lifecycle assertions);
# the embedded manifest's `remote_manifests` claim field referencing that
# endpoint; the verify endpoint's lifecycle_status banner per contract
# state; and the verify response's four independently named checks (PDF
# signature, C2PA manifest, VC signature, status list).
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
#     (contractstate/transition.go); its scenario stays @skip so the gap is
#     recorded as a deviation instead of being misread as a pass or a
#     failure.
#
# The strip-then-verify scenario (remote-manifest fallback) stays @skip:
# the verify path has no fallback branch (verifycontract.go calls
# pdfCore.Verify on the stored bytes), and the "remote" manifest is derived
# from the same stored PDF via the same pdf_ipfs_cid column that verify
# reads — there is no independently persisted manifest artifact to fall
# back to, so stripping the embedded attachment fails both endpoints
# identically. A genuine proof needs backend work first (persist the
# manifest as its own artifact plus a fallback branch in verify) or a
# Go-level test with a mocked IPFS client; see the scenario's inline
# comment.

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
  Scenario: The embedded manifest's remote_manifests claim field references the public manifest endpoint
    Given contract "C2PA Remote Ref Contract" has reached contract state "SIGNED"
    And contract "C2PA Remote Ref Contract" has an exported PDF
    When I request the public C2PA manifest for contract "C2PA Remote Ref Contract"
    Then get http 200:Success code
    And the C2PA manifest response for contract "C2PA Remote Ref Contract" declares a remote_manifests field pointing to its own public manifest endpoint

  @DCS-OR-C2PA-008 @skip
  Scenario: Verification succeeds from the remote manifest after the embedded attachment is stripped
    # See the header comment above for the full rationale. This scenario
    # documents the INTENDED behavior so the traceability still exists; it
    # is not executed as black-box proof today. A genuine automated proof
    # needs either:
    #   (a) a Go-level test in backend/internal/pdfgeneration/query (mocking
    #       IPFSClient.FetchFile to return a PDF whose
    #       `content_credential.c2pa` attachment has been stripped, then
    #       asserting VerifyContractPdfHandler falls back to
    #       pdf_manifest_ipfs_cid / the public remote-manifest bytes), or
    #   (b) a new, reviewed test-support seam (e.g. exposing
    #       IPFS_TENANT_BASE_URL, already present in backend/.env.dev1,
    #       to this BDD harness so a step could legitimately overwrite the
    #       contract's stored PDF pointer) — not added, since it is closer
    #       to infrastructure than to a step definition.
    Given contract "C2PA Stripped Contract" has reached contract state "SIGNED"
    And contract "C2PA Stripped Contract" has an exported PDF
    And a local copy of contract "C2PA Stripped Contract"'s PDF with the content_credential.c2pa attachment removed
    When contract "C2PA Stripped Contract" is verified via the backend verify endpoint
    Then the verification result reports c2pa_manifest_found is true, served from the remote manifest fallback

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
  Scenario: Replaced lifecycle banner is a documented gap, not a BDD result
    # "Replaced" is not a reachable ContractState in
    # contractstate/transition.go at all (only a lowercase pass-through
    # string in lifecycle.go for callers that already use SRS vocabulary
    # directly), and no ad hoc trigger is invented here to force it. This
    # scenario intentionally documents the gap without asserting anything —
    # the @skip keeps it out of both the green and red result counts, so it
    # cannot be misread as a genuine pass or a genuine failure. Tracked as
    # a recorded deviation, not as a BDD result.
    Given I am authenticated with roles: "Contract Manager"

  @DCS-OR-C2PA-006
  Scenario: The verify response carries four independently named checks, and PDF signature is honestly "not yet available"
    Given contract "C2PA Four Checks Contract" has reached contract state "SIGNED"
    And contract "C2PA Four Checks Contract" has an exported PDF
    When contract "C2PA Four Checks Contract" is exported and verified as PDF
    Then get http 200:Success code
    And the verify response for contract "C2PA Four Checks Contract" includes four named checks: PDF signature, C2PA manifest, VC signature, and status list
    And the PDF signature check for contract "C2PA Four Checks Contract" is marked as not yet available rather than passed
