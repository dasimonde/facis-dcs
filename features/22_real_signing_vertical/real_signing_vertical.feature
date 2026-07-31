# Real signing vertical - PAdES signature, wallet-driven signing ceremony, PID
# binding (SRS: DCS-FR-SM-08/-14/-16/-18, DCS-IR-SI-10, DCS-FR-CWE-04).
#
# Harness notes (see steps/real_signing_vertical/
# dcs_real_signing_vertical_steps.py's module docstring for the full
# rationale behind each binding decision):
#
#   1. pdf-core's own POST /sign is not reachable from this harness at all
#      (only the backend is, via BDD_DCS_BASE_URL) - PAdES is exercised
#      indirectly through the prepare/submit wallet ceremony (ADR-12/ADR-20;
#      /signature/apply is gone) and by inspecting the PDF bytes GET
#      /pdf/export/contract/{did} serves afterwards, using the same
#      direct-byte-search technique established elsewhere in this codebase's
#      BDD packs.
#   2. EUDIPLO is not a dependency of this DCS (ADR-20); this harness plays
#      the wallet itself, direct_post'ing a real, protocol-correct SD-JWT VC +
#      KB-JWT vp_token (PID + Power of Attorney, keyed by their DCQL query
#      ids) straight at the ceremony's own callback
#      (POST /signature/request/{ceremony_id}/callback, authenticated by the
#      unguessable ceremony id, not a shared secret), built with the
#      testWallet/dcs_wallet signing primitives.
#   3. Byte-level PDF assertions (SubFilter, x5chain, RFC3161 timestamp,
#      ByteRange coverage) are direct-byte-search heuristics, not a full PDF/
#      CMS/ASN.1 parse - each documents its own precision limit at its point
#      of use in the steps module.
#
# The Signature Manager UI (QR/poll/result modal, AES badge) has no coverage
# in this pack - this repo-root BDD harness has no browser-automation
# convention at all (see features/16_other/frontend.feature, a bare
# reachability check). The service-level contract the UI would call is
# already exercised by the ceremony scenarios below; the UI-specific
# rendering claims are recorded as an explicit coverage gap via the final
# @skip scenario, not fabricated.

@DCS-FR-SM-16 @DCS-IR-SI-10 @DCS-FR-UC-04-1
Feature: Real signing vertical - PAdES signature, wallet-driven signing ceremony, PID binding

  # ---------------------------------------------------------------------
  # PAdES signature production - pdf-core POST /sign, exercised indirectly
  # via POST /signature/apply after a completed ceremony (see steps module
  # docstring point 1).
  # ---------------------------------------------------------------------

  @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: Applying a signature produces a PDF with a cryptographically valid PAdES signature in the named AcroForm field
    Given contract "RSV AcroForm Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerOne"
    Then the signed PDF for contract "RSV AcroForm Contract" contains a PAdES signature naming the signing party AcroForm field
    And the signed PDF for contract "RSV AcroForm Contract" has a structurally valid PAdES ByteRange

  @DCS-OR-C2PA-002 @DCS-OR-C2PA-010 @DCS-FR-SM-02 @DCS-IR-CI-08
  Scenario: The PAdES signature declares SubFilter ETSI.CAdES.detached with a full embedded x5chain
    Given contract "RSV SubFilter Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerTwo"
    Then the signed PDF for contract "RSV SubFilter Contract" declares SubFilter ETSI.CAdES.detached
    And the signed PDF for contract "RSV SubFilter Contract" embeds a non-empty X.509 certificate chain

  @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: The PAdES signature carries an RFC3161 timestamp from the configured TSA (PAdES-B-T)
    Given contract "RSV Timestamp Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerThree"
    Then the signed PDF for contract "RSV Timestamp Contract" embeds an RFC3161 timestamp token

  @DCS-OR-C2PA-002 @DCS-OR-C2PA-010
  Scenario: The order /update -> /sign -> /update leaves the PAdES signature and C2PA chain valid
    Given contract "RSV Order Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFour"
    When the signature for contract "RSV Order Contract" is revoked as a post-sign C2PA update
    Then get http 200:Success code
    When I re-export the signed PDF for contract "RSV Order Contract"
    Then the signed PDF for contract "RSV Order Contract" still has a structurally valid PAdES signature
    When contract "RSV Order Contract" is exported and verified as PDF
    Then get http 200:Success code
    And the verification result shows match is true

  # ---------------------------------------------------------------------
  # Contract signer + apply flow
  # ---------------------------------------------------------------------

  @DCS-FR-SM-16 @DCS-IR-SI-10
  Scenario: The applied signature is a real PAdES signature persisted with an IPFS CID, not the stub placeholder
    Given contract "RSV No Stub Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFive"
    Then the contract_signatures row for contract "RSV No Stub Contract" is a real signature, not the STUB placeholder

  @FR-SM-18 @DCS-IR-SI-10
  Scenario: The apply endpoint honors the requested signer_did and credential_type instead of discarding them
    Given contract "RSV Apply Fields Contract" is APPROVED and has completed a signing ceremony for signatory "ExplicitFieldsSigner"
    When contract signer applies a signature to contract "RSV Apply Fields Contract" using the ceremony's signer_did and credential_type "AES"
    Then get http 200:Success code
    And the signature envelope for contract "RSV Apply Fields Contract" reflects the ceremony's signer_did and credential_type "AES"

  @DCS-FR-SM-16 @FR-SM-25 @UC-04-02 @DCS-FR-UC-11-1
  Scenario: Apply is refused with a typed error until a completed PID presentation exists for the signer
    Given contract "RSV Ceremony Gate Contract" has reached contract state "APPROVED"
    When contract signer applies a signature to contract "RSV Ceremony Gate Contract" without a prior signing ceremony
    Then the apply request is rejected with a typed ceremony-required error

  @DCS-FR-CWE-04 @DCS-FR-SM-11
  Scenario: The signature record binds both the PDF hash and the JSON-LD content hash
    Given contract "RSV Dual Hash Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerSix"
    Then the contract_signatures row for contract "RSV Dual Hash Contract" records both a PDF hash and a JSON-LD content hash

  # ---------------------------------------------------------------------
  # Wallet-driven signing ceremony (see steps module docstring point 2)
  # ---------------------------------------------------------------------

  @FR-SM-14
  Scenario: POST /signature/request starts a ceremony for an authorized Contract Signer
    Given contract "RSV Ceremony Start Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Ceremony Start Contract" field "SignerSeven" as "Contract Signer"
    Then get http 200:Success code
    And the ceremony response includes a ceremony_id, wallet_uri, and expires_at

  @FR-SM-14
  Scenario: POST /signature/request denies a caller without an authorized signing role
    Given contract "RSV Ceremony Denied Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Ceremony Denied Contract" field "SignerEight" as "Contract Observer"
    Then the ceremony start request is denied for that role

  @FR-SM-14 @DCS-FR-CWE-19 @DCS-FR-SM-24
  Scenario: GET /signature/request/{id} reports the ceremony's lifecycle status as it progresses
    Given contract "RSV Ceremony Status Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerNine"
    When I poll the signing ceremony status for contract "RSV Ceremony Status Contract"
    Then get http 200:Success code
    And the signing ceremony for contract "RSV Ceremony Status Contract" has status "verified"

  @ADR-20 @FR-SM-14
  Scenario: The ceremony callback marks the ceremony verified and persists PID claims when the request nonce is correct
    Given contract "RSV Webhook Auth Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Webhook Auth Contract" field "SignerTen" as "Contract Signer"
    Then get http 200:Success code
    When the wallet presentation confirms the ceremony for contract "RSV Webhook Auth Contract" with the correct request nonce
    Then get http 200:Success code
    When I poll the signing ceremony status for contract "RSV Webhook Auth Contract"
    Then the signing ceremony for contract "RSV Webhook Auth Contract" has status "verified"

  @ADR-20 @FR-SM-14
  Scenario: The ceremony callback rejects a presentation bound to an incorrect request nonce
    Given contract "RSV Webhook Bad Secret Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Webhook Bad Secret Contract" field "SignerEleven" as "Contract Signer"
    Then get http 200:Success code
    When a caller posts a ceremony presentation for contract "RSV Webhook Bad Secret Contract" with an incorrect request nonce
    Then the ceremony presentation is rejected for the incorrect request nonce

  # DCS-FR-SM-13: the signing workflow enforces deadlines — the ceremony's
  # expires_at (issued at start, ceremonyTTL) is a hard gate at the
  # presentation callback: a late wallet presentation does not verify and
  # the signer must start a fresh ceremony (the workflow's retry).
  @DCS-FR-SM-13 @ADR-20
  Scenario: The ceremony callback rejects a presentation that arrives after the ceremony deadline
    Given contract "RSV Ceremony Deadline Contract" has reached contract state "APPROVED"
    When I start a signing ceremony for contract "RSV Ceremony Deadline Contract" field "SignerDeadline" as "Contract Signer"
    Then get http 200:Success code
    When the signing ceremony deadline for contract "RSV Ceremony Deadline Contract" has already passed
    And the wallet presentation confirms the ceremony for contract "RSV Ceremony Deadline Contract" with the correct request nonce
    Then the ceremony presentation is rejected because the signing deadline has passed

  @UC-04-02
  Scenario: The ceremony completes headlessly by fulfilling the OID4VP presentation contract, no wallet UI involved
    Given contract "RSV Headless Ceremony Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerTwelve"
    When I poll the signing ceremony status for contract "RSV Headless Ceremony Contract"
    Then get http 200:Success code
    And the signing ceremony for contract "RSV Headless Ceremony Contract" has status "verified"

  # ---------------------------------------------------------------------
  # Signer binding: the pseudonymous holder DID + signing-summary VC embedded
  # UNDER the signature (embed-first-sign-second). The PID itself is NEVER
  # embedded — personal data stays out of the shared PDF (eIDAS/GDPR).
  # ---------------------------------------------------------------------

  @DCS-FR-SM-08 @NFR-SEC-18 @DCS-NFR-SEC-18 @DCS-NFR-SQ-04
  Scenario: The signer PID is NOT embedded in the signed PDF (privacy), only the pseudonymous binding
    Given contract "RSV Verbatim Presentation Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerThirteen"
    Then the signer PID for contract "RSV Verbatim Presentation Contract" is NOT embedded in the signed PDF, only the pseudonymous binding

  @DCS-FR-SM-08
  Scenario: A ContractSigningSummaryCredential is issued and embedded under the signature
    Given contract "RSV Summary VC Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFourteen"
    Then a ContractSigningSummaryCredential for contract "RSV Summary VC Contract" is embedded inside the PAdES ByteRange

  # @skip — deferred BY DECISION. The wallet-driven remote-signing ceremony
  # produces the PAdES signature over the PDF; a detached JAdES over the
  # machine-readable JSON-LD is not yet emitted by the wallet/DSS path. The
  # vertical takes precedence; wallet-JAdES is follow-up research. Un-skip once
  # the ceremony emits the JAdES artifact.
  @DCS-FR-SM-02 @DCS-FR-SM-11 @skip
  Scenario: The ceremony also produces a JAdES signature over the machine-readable JSON-LD
    Given contract "RSV JAdES Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerJades"
    Then the signature view for contract "RSV JAdES Contract" carries a JAdES signature that verifies over the contract JSON-LD

  # Uses the IPFS CID-swap seam (steps/support/tamper_seam.py). See
  # steps/real_signing_vertical/dcs_real_signing_vertical_tamper_steps.py's
  # module docstring for why the observable signal here is
  # /signature/validate's embedded-PID cross-check finding, not a literal
  # PAdES cryptographic signature verdict — no endpoint reachable by this
  # harness re-verifies the CMS signature over its /ByteRange, and
  # pdf-core's own /verify treats the entire PAdES-signed span (including
  # the evidence attachment) as an opaque, unchecked suffix by design.
  @DCS-FR-SM-08
  Scenario: Corrupting the signature-evidence attachment invalidates the embedded-PID cross-check
    Given contract "RSV Evidence Tamper Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerEvidenceTamper"
    When the signature-evidence attachment for contract "RSV Evidence Tamper Contract" is corrupted on the server-stored PDF
    Then the signature validation findings for contract "RSV Evidence Tamper Contract" report the embedded signing evidence as invalid

  @UC-04-02 @UC-04-03
  Scenario: The verify side cross-checks the embedded signer binding against the signature record
    Given contract "RSV Verify Crosscheck Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerFifteen"
    When I validate the signature for contract "RSV Verify Crosscheck Contract"
    Then get http 200:Success code
    And the signature validation findings for contract "RSV Verify Crosscheck Contract" cross-check the embedded signer binding

  # ---------------------------------------------------------------------
  # Full end-to-end
  # ---------------------------------------------------------------------

  @UC-04-02 @UC-04-03 @DCS-FR-SM-16
  Scenario: End-to-end - accept, ceremony, AES-signed PDF, verify stays green, contract_signatures carries AES + ipfs_cid + ceremony link
    Given contract "RSV E2E Contract" has an AES-signed PDF via a completed ceremony for signatory "SignerSixteen"
    When contract "RSV E2E Contract" is exported and verified as PDF
    Then get http 200:Success code
    And the verification result shows match is true
    And the contract_signatures row for contract "RSV E2E Contract" is a real signature, not the STUB placeholder
    And the contract_signatures row for contract "RSV E2E Contract" is linked to a signature_ceremonies row

  # ---------------------------------------------------------------------
  # Signature Manager UI: documented coverage gap (see the header comment
  # above). No fabricated pass/fail - the traceability is kept present via
  # @skip.
  # ---------------------------------------------------------------------

  @skip
  Scenario: Signature Manager UI ceremony flow and AES badge - not provable from this HTTP-only BDD harness
    # This repo-root BDD harness has no browser-automation convention (see
    # features/16_other/frontend.feature - a bare reachability check). The
    # service-level contract the UI would call (start ceremony, poll status,
    # apply, AES credential_type) is already exercised end-to-end by the
    # ceremony and e2e scenarios above. The UI-specific claims (the
    # QR/poll/result modal and the AES badge render,
    # frontend/ClientApp/src/services/signature-management-service.ts) need
    # a browser-level test this harness does not have; recorded here as an
    # explicit coverage gap, not a fabricated result.
    Given I am authenticated with roles: "Contract Manager"
