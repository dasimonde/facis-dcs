# Contract Storage & Archive (UC-07, backend/design/contract_storage_archive.go).
# Archive-entry creation itself (at SIGNED, not APPROVED) and its evidence
# content are covered by 05_contract_deployment/contract_deployment.feature
# — this file covers retrieval, search, RBAC scope, and the audit trail of
# the /archive/* endpoints themselves, which the deployment pack does not
# exercise.

@UC-07 @DCS-IR-CSA-01 @DCS-IR-CSA-05 @DCS-FR-UC-07-1
Feature: Contract storage and archive retrieval

  @UC-07-01 @DCS-IR-CSA-01 @DCS-FR-CSA-12 @DCS-FR-CWE-21 @DCS-FR-CSA-25
  Scenario: Archive Manager retrieves the full archive list
    Given contract "Archive Retrieve Contract" has reached contract state "SIGNED"
    When the Archive Manager retrieves the archive
    Then get http 200:Success code
    And the archive retrieval result includes contract "Archive Retrieve Contract"

  @UC-07-01 @DCS-IR-CSA-01 @DCS-FR-CSA-12 @DCS-FR-CWE-21
  Scenario: Archive search filters by contract state
    Given contract "Archive Search Contract" has reached contract state "SIGNED"
    When the Archive Manager searches the archive with state filter "SIGNED"
    Then get http 200:Success code
    And the archive search result includes contract "Archive Search Contract"

  @UC-07-02 @DCS-IR-CSA-05 @DCS-FR-CSA-02 @DCS-FR-CSA-12 @DCS-FR-CWE-21 @DCS-FR-UC-07-2 @DCS-FR-CSA-25
  Scenario: A role outside the archive scope cannot retrieve the archive
    Given I am authenticated with roles: "Template Creator"
    When I attempt to retrieve the archive with my current role
    Then the request is denied with a client error

  @UC-07-03 @DCS-IR-CSA-04
  Scenario: Auditor retrieves the archive audit log
    Given contract "Archive Audit Contract" has reached contract state "SIGNED"
    When the Auditor retrieves the archive audit log
    Then get http 200:Success code
    And the archive audit log is a non-empty list

  # DCS-IR-CSA-02: the dashboard's store-into-archive surface. The
  # /archive/store implementation is currently a stub (it logs and returns a
  # placeholder — backend/internal/service/contract_storage_archive.go; the
  # real archive entry is written automatically at SIGNED), so the assertable
  # contract here is the endpoint's RBAC gate: Archive Manager scope gets a
  # 200, any role outside the archive scope is refused.
  @UC-07-01 @DCS-IR-CSA-02
  Scenario: Archive Manager can invoke the archive store endpoint
    When the Archive Manager stores a contract in the archive
    Then get http 200:Success code

  @UC-07-02 @DCS-IR-CSA-02 @DCS-FR-CSA-02
  Scenario: A role outside the archive scope cannot store into the archive
    Given I am authenticated with roles: "Template Creator"
    When I attempt to store a contract in the archive with my current role
    Then the request is denied with a client error

  # DCS-NFR-SEC-13 (secure data disposal): a successful delete also removes
  # the entries' archived snapshots from the IPFS store, and that removal is
  # a hard failure — so the 200 asserted here covers the disposal call. The
  # snapshot-CID selection and the hard-fail removal semantics are unit
  # tested in backend/internal/service/contract_storage_archive_delete_test.go
  # (no existing BDD step pattern reads raw IPFS CIDs).
  @UC-07-03 @DCS-FR-CSA-17 @DCS-FR-CSA-02 @DCS-IR-CSA-03 @DCS-NFR-SEC-13
  Scenario: Archive Manager deletes an archived contract with a logged justification
    Given contract "Archive Deletion Contract" has reached contract state "SIGNED"
    When the Archive Manager deletes the archived contract "Archive Deletion Contract" with justification "no longer needed for compliance retention"
    Then get http 200:Success code
    And the archive deletion of contract "Archive Deletion Contract" is recorded in the archive audit log

  @UC-07-03 @DCS-FR-CSA-17 @DCS-IR-CSA-03
  Scenario: A role outside the archive scope cannot delete an archived contract
    Given contract "Unauthorized Archive Deletion Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Template Creator"
    When I attempt to delete the archived contract "Unauthorized Archive Deletion Contract" with my current role
    Then the request is denied with a client error

  # DCS-IR-CSA-06: read-only users (Contract Observer) can view archived
  # records but MUST NOT be able to modify or delete entries. The design
  # scopes /archive/retrieve+search to Archive Manager AND Contract Observer,
  # while /archive/store and /archive/delete are Archive Manager only
  # (backend/design/contract_storage_archive.go) — this scenario asserts
  # both halves of that contract against the running service.
  @UC-07-03 @DCS-IR-CSA-06 @DCS-FR-CSA-02
  Scenario: A read-only Observer can view the archive but cannot delete from it
    Given contract "Observer Readonly Archive Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Contract Observer"
    When I attempt to retrieve the archive with my current role
    Then get http 200:Success code
    And the archive retrieval result includes contract "Observer Readonly Archive Contract"
    When I attempt to delete the archived contract "Observer Readonly Archive Contract" with my current role
    Then the request is denied with a client error

  # DCS-FR-CSA-13: full-text search across archived contract CONTENT (not just
  # name/description metadata). The contracts table keeps a stored tsvector
  # over the entire contract JSON-LD (search_vector, GIN-indexed) and
  # /archive/search?contract_data=... queries it via plainto_tsquery — this
  # scenario drives that path with a term that exists in the contract's data
  # payload, and asserts a nonsense term yields no hit for the same contract.
  # "Confidentiality" is a word inside the standard template's clause text
  # (embedded in the contract JSON-LD, therefore in the stored search_vector),
  # NOT a metadata field — so a hit proves the tsvector covers document
  # CONTENT. The nonsense term proves it is a real full-text match, not a
  # match-everything.
  @UC-07-01 @DCS-FR-CSA-13
  Scenario: Archive full-text search finds a contract by terms inside its content
    Given contract "Fulltext Corpus Archive Contract" has reached contract state "SIGNED"
    When the Archive Manager searches the archive with full-text query "Confidentiality"
    Then get http 200:Success code
    And the archive search result includes contract "Fulltext Corpus Archive Contract"
    When the Archive Manager searches the archive with full-text query "xyzzyplugh nonexistent"
    Then get http 200:Success code
    And the archive search result does not include contract "Fulltext Corpus Archive Contract"

  # DCS-FR-CSA-10 / DCS-FR-CSA-13: structured search over the indexed party
  # metadata. The contract's two parties (creator/counterparty) live in the
  # responsible JSONB column and /archive/search?party=... matches either
  # slot. The counterparty is seeded via the shared test DB connection (the
  # same accepted seam the expiry-window step uses) because the
  # single-instance BDD creation flow assigns no distinct counterparty DID.
  @UC-07-01 @DCS-FR-CSA-10 @DCS-FR-CSA-13
  Scenario: Archive search by party returns only contracts with that party
    Given contract "Party Search Match Contract" has reached contract state "SIGNED"
    And contract "Party Search Other Contract" has reached contract state "SIGNED"
    And contract "Party Search Match Contract" has counterparty "did:web:party-search-bdd.example" set directly in the database (party-search test seam)
    When the Archive Manager searches the archive by party "did:web:party-search-bdd.example"
    Then get http 200:Success code
    And the archive search result includes contract "Party Search Match Contract"
    And the archive search result does not include contract "Party Search Other Contract"

  # DCS-FR-CSA-10 / DCS-FR-CSA-13: structured search over the indexed
  # validity period. valid_from/valid_until bound the contract's
  # start_date/exp_date: a contract matches when its validity period lies
  # within the queried range. Validity dates are seeded via the shared test
  # DB connection (no API path sets start_date/exp_date post-creation) and
  # kept in the future so the expiry cron never touches these contracts.
  @UC-07-01 @DCS-FR-CSA-10 @DCS-FR-CSA-13
  Scenario: Archive search by validity period returns only contracts valid within the range
    Given contract "Validity Range Inside Contract" has reached contract state "SIGNED"
    And contract "Validity Range Outside Contract" has reached contract state "SIGNED"
    And contract "Validity Range Inside Contract" has validity period "2027-01-01T00:00:00Z" to "2027-12-31T00:00:00Z" set directly in the database (validity-search test seam)
    And contract "Validity Range Outside Contract" has validity period "2029-01-01T00:00:00Z" to "2029-12-31T00:00:00Z" set directly in the database (validity-search test seam)
    When the Archive Manager searches the archive with validity period from "2026-12-01T00:00:00Z" until "2028-01-01T00:00:00Z"
    Then get http 200:Success code
    And the archive search result includes contract "Validity Range Inside Contract"
    And the archive search result does not include contract "Validity Range Outside Contract"

  # An unparsable date parameter is a hard 400, never silently ignored.
  @UC-07-01 @DCS-FR-CSA-10 @DCS-FR-CSA-13
  Scenario: Archive search with an unparsable validity date is rejected
    When the Archive Manager searches the archive with validity period from "not-a-date" until "2028-01-01T00:00:00Z"
    Then the request is denied with a client error

  # DCS-FR-CSA-11: each archived contract can carry a summary (manual or
  # system-generated) and user-assigned tags for thematic categorization and
  # discovery. Annotation is Archive Manager-scoped, mutates ONLY the
  # annotation columns (the archive entry's snapshot/evidence stay immutable,
  # enforced by DB trigger), and is recorded in the archive audit log.
  @UC-07-01 @DCS-FR-CSA-11 @DCS-FR-CSA-25
  Scenario: Archive Manager annotates an archived contract with a manual summary and tags
    Given contract "Annotated Archive Contract" has reached contract state "SIGNED"
    When the Archive Manager annotates the archived contract "Annotated Archive Contract" with summary "Pilot supply agreement archived for the BDD suite" and tags "pilot-agreement,bdd-supply"
    Then get http 200:Success code
    And the archive entry for contract "Annotated Archive Contract" carries summary "Pilot supply agreement archived for the BDD suite" and tags "pilot-agreement,bdd-supply"
    And the archive annotation of contract "Annotated Archive Contract" is recorded in the archive audit log

  @UC-07-01 @DCS-FR-CSA-11
  Scenario: Searching the archive by tag returns only contracts carrying that tag
    Given contract "Tagged Archive Contract" has reached contract state "SIGNED"
    And contract "Untagged Archive Contract" has reached contract state "SIGNED"
    And the Archive Manager annotates the archived contract "Tagged Archive Contract" with summary "tagged for discovery" and tags "quarterly-review-bdd"
    When the Archive Manager searches the archive by tag "quarterly-review-bdd"
    Then get http 200:Success code
    And the archive search result includes contract "Tagged Archive Contract"
    And the archive search result does not include contract "Untagged Archive Contract"

  # The "automatic ... generation of a summary" half of DCS-FR-CSA-11: when no
  # summary is supplied, the system derives one from the archived contract's
  # own metadata (name, version, state, creator).
  @UC-07-01 @DCS-FR-CSA-11
  Scenario: Annotating without a summary generates one from the contract metadata
    Given contract "Auto Summary Archive Contract" has reached contract state "SIGNED"
    When the Archive Manager annotates the archived contract "Auto Summary Archive Contract" with tags "auto-summary-bdd" and no summary
    Then get http 200:Success code
    And the archive entry for contract "Auto Summary Archive Contract" carries a generated summary derived from its version and state

  @UC-07-02 @DCS-FR-CSA-11
  Scenario: A read-only role cannot annotate an archived contract
    Given contract "Unauthorized Annotation Contract" has reached contract state "SIGNED"
    And I am authenticated with roles: "Contract Observer"
    When I attempt to annotate the archived contract "Unauthorized Annotation Contract" with my current role
    Then the request is denied with a client error

  @UC-07-01 @DCS-FR-CSA-21 @DCS-FR-CWE-10 @DCS-FR-UC-07-3
  Scenario: Archive dashboard statistics reflect the archive and flag expiring contracts
    Given contract "Archive Statistics Contract" has reached contract state "SIGNED"
    And contract "Archive Statistics Contract" is set to expire in 7 days directly in the database (expiry-window test seam)
    When the Archive Manager retrieves the archive statistics
    Then get http 200:Success code
    And the archive statistics count at least one archived contract with positive storage volume
    And the archive statistics report a compliant archive entry
    And the archive statistics list contract "Archive Statistics Contract" as expiring
    And the archive statistics include a recent archive action for contract "Archive Statistics Contract"

  @UC-07-02 @DCS-FR-CSA-21
  Scenario: A role outside the archive scope cannot read the archive statistics
    Given I am authenticated with roles: "Template Creator"
    When I attempt to retrieve the archive statistics with my current role
    Then the request is denied with a client error

  # DCS-FR-CSA-04: expiration alerts follow a configurable threshold — the
  # expiring-contracts window is DCS_ARCHIVE_EXPIRING_WINDOW_DAYS
  # (backend/internal/base/conf), default 30 days. The suite must not change
  # the deployed environment, so this asserts both sides of the default
  # window's boundary: a contract expiring inside it is flagged, one expiring
  # beyond it is not. The accessor's override behavior is unit tested in
  # backend/internal/base/conf/conf_test.go.
  @UC-07-01 @DCS-FR-CSA-04 @DCS-FR-CSA-21
  Scenario: Expiring-contract alerts respect the configured expiry window
    Given contract "Expiry Window Inside Contract" has reached contract state "SIGNED"
    And contract "Expiry Window Outside Contract" has reached contract state "SIGNED"
    And contract "Expiry Window Inside Contract" is set to expire in 7 days directly in the database (expiry-window test seam)
    And contract "Expiry Window Outside Contract" is set to expire in 90 days directly in the database (expiry-window test seam)
    When the Archive Manager retrieves the archive statistics
    Then get http 200:Success code
    And the archive statistics list contract "Expiry Window Inside Contract" as expiring
    And the archive statistics do not list contract "Expiry Window Outside Contract" as expiring
