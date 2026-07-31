# Signature retrieve/validate/audit/compliance (DCS-FR-SM-15, DCS-FR-SM-18,
# DCS-FR-SM-19, DCS-FR-SM-21, UC-04): GET /signature/retrieve/{did},
# POST /signature/validate, GET /signature/audit,
# POST /signature/compliance (backend/design/signature_management.go). All
# three are already implemented; only /signature/verify (contract integrity &
# envelope check, used as a setup step by other packs) and /signature/apply
# (the signing ceremony itself, covered by 22_real_signing_vertical) had
# scenario coverage before this file.

@UC-04 @DCS-FR-SM-18 @DCS-FR-SM-19 @DCS-FR-SM-21 @DCS-FR-UC-04-2
Feature: Signature validation, audit, and compliance

  @clean_db @DCS-FR-SM-18
  Scenario: Contract Manager validates a signed contract's signature
    Given contract "Signature Validation Contract" has reached contract state "SIGNED"
    When the contract manager validates the signature for contract "Signature Validation Contract"
    Then get http 200:Success code
    And the signature validation for contract "Signature Validation Contract" reports only passing checks

  @clean_db @DCS-FR-SM-19
  Scenario: Signature audit log records the apply-signature action
    Given contract "Signature Audit Contract" has reached contract state "SIGNED"
    When the contract manager validates the signature for contract "Signature Audit Contract"
    Then get http 200:Success code
    # APPLIED_SIGNATURE is the event the apply command actually emits
    # (signingmanagement/event/event.go); the APPLY_SIGNATURE constant in
    # eventtype.go is defined but never published.
    And the signature audit log for contract "Signature Audit Contract" includes an action of type "APPLIED_SIGNATURE"
    # DCS-FR-SM-19: the log entry itself must capture signer ID, credential
    # used, and timestamp — the ApplyEvent's applied_by / credential_type /
    # occurred_at fields (signingmanagement/event/event.go). applied_by is the
    # participant DID from the auth context, credential_type the achieved
    # signature level, occurred_at the RFC3339 signing time.
    And the "APPLIED_SIGNATURE" signature audit entry for contract "Signature Audit Contract" carries the signer DID, credential type "AES", and an RFC3339 timestamp

  # DCS-FR-SM-15: retrieving a contract for signing is itself an audited
  # read — GET /signature/retrieve/{did} records a RETRIEVE_CONTRACT_BY_ID
  # event (signingmanagement/query/querybyid.go) carrying the retrieving
  # signer, the timestamp, and the contract ID.
  @clean_db @DCS-FR-SM-15
  Scenario: Retrieving a contract for signing is logged with signer ID, timestamp, and contract ID
    Given contract "Signing Retrieval Contract" has reached contract state "SIGNED"
    When the contract signer retrieves contract "Signing Retrieval Contract" for signing
    Then get http 200:Success code
    And the retrieval of contract "Signing Retrieval Contract" is recorded with the retrieving signer, a timestamp, and the contract ID

  # /signature/compliance computes its findings (DCS-FR-SM-21:
  # signature level SES/AES/QES, signature status, active signed
  # credentials) and records the check — findings included — as a
  # ComplianceValidationEvent in the audit trail.
  @clean_db @DCS-FR-SM-21 @DCS-IR-SM-07
  Scenario: Contract Manager requests a compliance check for a signed contract
    Given contract "Signature Compliance Contract" has reached contract state "SIGNED"
    When the contract manager requests a compliance check for contract "Signature Compliance Contract"
    Then get http 200:Success code
    And the compliance check for contract "Signature Compliance Contract" reports that all checks passed

  @clean_db @DCS-FR-SM-21 @DCS-FR-SM-20 @DCS-IR-SM-06
  Scenario: The compliance check flags a revoked signature
    Given contract "Revoked Compliance Contract" has reached contract state "SIGNED"
    When the applied signature of contract "Revoked Compliance Contract" is revoked
    Then get http 200:Success code
    When the contract manager requests a compliance check for contract "Revoked Compliance Contract"
    Then get http 200:Success code
    And the compliance check for contract "Revoked Compliance Contract" flags a revoked signature

  # DCS-FR-SM-26 / DCS-IR-SM-05: the Signature Compliance Viewer's data —
  # per-signature signer identity, credential class, status, timestamps,
  # container format, and the contract's cryptographic integrity findings.
  @clean_db @DCS-FR-SM-26 @DCS-IR-SM-05
  Scenario: The signature view exposes signer identity, credential class, timestamp, and integrity
    Given contract "Signature View Contract" has reached contract state "SIGNED"
    When the signature view for contract "Signature View Contract" is requested as "Compliance Officer"
    Then get http 200:Success code
    And the signature view for contract "Signature View Contract" shows one "SIGNED" signature with signer identity, credential class "AES", timestamp, and intact integrity
    And the signature view for contract "Signature View Contract" shows the achieved level meeting the contract's required level "AES", not qualified, and a non-empty signing certificate subject

  @clean_db @DCS-FR-SM-26
  Scenario: A role outside the signature-viewing scope cannot request the signature view
    Given contract "Unauthorized Signature View Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Template Creator"
    When I attempt to request the signature view for contract "Unauthorized Signature View Contract" with my current role
    Then the request is denied with a client error

  # DCS-FR-SM-27: signed contracts MUST be exportable in PDF/A format with
  # embedded metadata and signature containers. pdf-core compiles PDF/A-3A
  # (pdfaid:part=3, pdfaid:conformance=A, ISO 19005-3) with the canonical
  # JSON-LD payload embedded as an associated file (AFRelationship /Source) —
  # asserted here on the actual exported bytes of a SIGNED contract.
  @clean_db @DCS-FR-SM-27 @UC-04
  Scenario: A signed contract exports as PDF/A with embedded metadata containers
    Given contract "PDFA Signed Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Contract Manager"
    And contract "PDFA Signed Contract" has an exported PDF
    Then the exported PDF for contract "PDFA Signed Contract" declares PDF/A-3 conformance in its XMP metadata
    And the exported PDF for contract "PDFA Signed Contract" embeds the canonical JSON-LD payload as an associated file

  # DECISION-ANCHORED SKIP (not a test limitation): the EU distributes the
  # DSS demonstration webapp as a ZIP/WAR with NO official container image,
  # so deployment/helm/charts/dss wraps a pinned community image and stays
  # DISABLED by default — enabling an unofficial third-party image in the
  # hermetic CI deployment is not defensible. The integration itself is
  # implemented and unit-tested (backend/internal/signingmanagement/dss:
  # report parsing, and the hard-fail when a CONFIGURED DSS is unreachable —
  # a configured external validator is never silently skipped); flipping
  # dss.enabled plus the backend's DSS_URL env activates this scenario's
  # behavior without code changes.
  @skip @DCS-FR-SM-18 @DCS-IR-SI-10 @DCS-IR-CI-08
  Scenario: Signature validation reports the EU DSS indication when a DSS instance is deployed
    Given contract "DSS Validated Contract" has reached contract state "SIGNED"
    And an EU DSS instance is deployed and configured via DSS_URL
    When the contract manager validates the signature for contract "DSS Validated Contract"
    Then get http 200:Success code
    And the signature validation findings include an EU DSS validation report indication
