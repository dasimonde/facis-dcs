@UC-03-01 @FR-CWE-13 @FR-CWE-03 @FR-CWE-30 @FR-CWE-07 @DCS-FR-UC-03-1
Feature: Contract Creation
  Contract Creators generate contracts from predefined templates with
  auto-filled metadata. The system supports dynamic contract assembling
  from reusable clauses and contract package bundling.

  @clean_db @DCS-IR-CWE-01
  Scenario: Create contract from template
    Given I am authenticated with roles: "Contract Creator"
    And template "Service Agreement Template" is approved and available
    When I create a contract from template "Service Agreement Template"
    Then a draft contract is generated
    And the contract is assigned a unique contract ID
    And metadata is auto-filled
    And the creation is logged and traceable to the template version

  # AC1 (the Semantic-Hub SKOS vocabulary and label projection) and AC3 (the
  # UI's labeled, closed selection) are intentionally covered by focused
  # frontend component tests/grep gates. This Behave harness has no browser
  # automation, so an API scenario would not honestly verify those UI claims.
  @clean_db @REQ-contract-party-role-taxonomy-uris-AC2 @REQ-contract-party-role-taxonomy-uris-AC4 @DCS-IR-CWE-01 @DCS-IR-CWE-02 @DCS-FR-CWE-13 @DCS-FR-TR-03
  Scenario: Percent-encoded party fragments become URI-valued roles and bind only the selected originator
    Given a registered bilateral template whose top-level ODRL rules repeat the percent-encoded role URIs "https://w3id.org/facis/dcs/taxonomy/v1#role-provider" and "https://w3id.org/facis/dcs/taxonomy/v1#role-customer" in both directions
    When I create a contract from that template as originator role "https://w3id.org/facis/dcs/taxonomy/v1#role-provider"
    Then the derived contract contains exactly the roles "https://w3id.org/facis/dcs/taxonomy/v1#role-provider,https://w3id.org/facis/dcs/taxonomy/v1#role-customer" once each
    And every contractual role is stored as an absolute concept URI string
    And each open party fragment percent-decodes to its identical dcs:role URI
    And every top-level ODRL assigner and assignee still references the corresponding contractual party
    And the originator is bound only to the "https://w3id.org/facis/dcs/taxonomy/v1#role-provider" contractual party
    And the "https://w3id.org/facis/dcs/taxonomy/v1#role-customer" contractual party remains open for the counterparty

  @clean_db @REQ-contract-party-role-taxonomy-uris-AC5 @DCS-IR-CWE-01 @DCS-IR-CWE-02 @DCS-FR-CWE-13 @DCS-FR-TR-03
  Scenario Outline: The backend rejects short and unknown originator roles
    Given a registered bilateral template whose top-level ODRL rules repeat the percent-encoded role URIs "https://w3id.org/facis/dcs/taxonomy/v1#role-provider" and "https://w3id.org/facis/dcs/taxonomy/v1#role-customer" in both directions
    When I attempt to create a contract from that template as originator role "<originator_role>"
    Then contract creation is rejected because originator_role is not a role declared by the template

    Examples:
      | originator_role                                                          |
      | provider                                                                 |
      | https://w3id.org/facis/dcs/taxonomy/v1#role-custom-broker               |

  @clean_db @REQ-contract-readonly-event-admin-diff-regressions-AC2 @DCS-FR-UC-09-2 @UC-09-02
  Scenario: Successful contract creation records an effective audit timestamp
    Given I am authenticated with roles: "Contract Creator"
    And template "Audit Timestamp Template" is approved and available
    When I create a contract from template "Audit Timestamp Template"
    Then a draft contract is generated
    And the CREATE_CONTRACT audit event for the created contract has a non-zero RFC3339 occurred_at timestamp

  @clean_db
  Scenario: Created contract renders in both machine-readable and human-readable views
    Given I am authenticated with roles: "Contract Creator"
    And I have created contract "Service Agreement" from a template
    When I view contract "Service Agreement"
    Then the machine-readable view renders correctly
    And the human-readable view renders correctly

  @clean_db
  Scenario: Draft contract is editable and versioned
    Given I am authenticated with roles: "Contract Creator"
    And contract "Service Agreement" is in "Draft" status
    When I edit contract "Service Agreement"
    Then the changes are saved
    And a new version is created with timestamp and user attribution

  Scenario: Assemble contract from reusable clauses
    Given I am authenticated with roles: "Contract Creator"
    And reusable clauses "Payment Terms", "Liability", and "Confidentiality" exist
    When I assemble a contract using clauses "Payment Terms", "Liability", and "Confidentiality"
    Then the assembly process validates structure
    And the assembly process validates required metadata
    And the assembly process validates content logic
    And a draft contract is generated

  Scenario: Create contract with hierarchical structure
    Given I am authenticated with roles: "Contract Creator"
    And master agreement template "Framework Agreement" exists
    When I create a contract with sub-agreements and annexes
    Then the hierarchical structure is established
    And components are logically linked
    And components are version-controlled

  Scenario: Bundle multiple contracts into a package
    Given I am authenticated with roles: "Contract Manager"
    And contracts "Service Agreement" and "SLA Addendum" exist
    When I bundle contracts "Service Agreement" and "SLA Addendum" into package "Service Bundle"
    Then a contract package is created
    And the package maintains internal references
    And the package maintains shared metadata
    And the package tracks signature states

  Scenario: Auto-fill metadata from template
    Given I am authenticated with roles: "Contract Creator"
    And template "NDA Template" has predefined metadata fields
    When I create a contract from template "NDA Template"
    Then the contract inherits metadata from the template
    And I can override specific metadata values

  Scenario: Unauthorized role cannot create contracts
    Given I am authenticated with roles: "Contract Observer"
    When I attempt to create a contract from template "Service Agreement Template"
    Then the request is denied with an authorization error

  Scenario: Contract Creator can only create contracts for authorized parties
    Given I am authenticated with roles: "Contract Creator"
    And I am authorized to create contracts involving party "Acme Corp"
    When I create a contract from template "Service Agreement Template"
    And I specify party "Acme Corp" as a contract party
    Then the contract is created successfully
    And the contract is associated with party "Acme Corp"

  # Party read-scoping (query/contract/querybyid.go): the caller's
  # organization (the OID4VP-disclosed organization claim, the same value
  # persisted as created_by) must be the creating organization or listed in
  # the contract's dcs:parties to read it; Sys.* automation roles, the
  # Sys. Administrator, and the Auditor are org-independent readers. A
  # denial is HTTP 403 (retrieve_by_id's "forbidden" design error) and lands
  # in the audit trail as a CONTRACT_ACCESS_DENIED event.
  @DCS-NFR-SEC-03 @DCS-NFR-SEC-08 @UC-03-01
  Scenario: Created contract is accessible only to authorized parties
    Given I am authenticated with roles: "Contract Creator"
    And I have created contract "Party Scoped Contract" with parties "Acme Corp" and "TechVendor Inc"
    When a representative of party "TechVendor Inc" attempts to access contract "Party Scoped Contract"
    Then the contract is accessible and visible
    And when a representative of unrelated party "UnrelatedCorp" attempts to access contract "Party Scoped Contract"
    Then the access is denied with a "not authorized to access this contract" error

  @DCS-NFR-SEC-03 @DCS-NFR-SEC-08 @UC-03-01
  Scenario: Unauthorized party cannot access created contract
    Given I am authenticated with roles: "Contract Creator"
    And I have created contract "Party Denied Contract" with parties "Acme Corp" and "TechVendor Inc"
    When a representative of unrelated party "UnrelatedCorp" attempts to access contract "Party Denied Contract"
    Then the access is denied with a "not authorized to access this contract" error
    And the access denial for contract "Party Denied Contract" is logged in the audit trail
