@DCS-FR-CSA-02 @DCS-FR-PACM-04
Feature: Access decisions are auditable independently of credential validation

  @REQ-denied-access-audit-AC1 @REQ-denied-access-audit-AC2 @REQ-denied-access-audit-AC4
  Scenario Outline: Role and post-authentication scope denials retain complete request metadata
    Given an access audit probe authenticated as "<role>" for scope "contracts"
    When the access audit probe requests "<action>"
    Then get http 403:401 Unauthorized
    And exactly one DENIED access attempt records the probe metadata and successful credentials

    Examples:
      | role             | action |
      | Contract Manager | audit  |
      | Contract Manager | report |
      | Archive Manager  | audit  |
      | Archive Manager  | report |

  @REQ-denied-access-audit-AC3
  Scenario: Repeated role denials do not add credential failures
    Given an access audit probe authenticated as "Contract Manager" for scope "contracts"
    And the credential failure count is remembered
    When the access audit probe requests "audit" three times
    Then each probe request is forbidden
    And the credential failure count is unchanged
    And every probe attempt has DENIED outcome and successful credentials

  @REQ-denied-access-audit-AC5 @REQ-denied-access-audit-AC6
  Scenario: Auditor reads stable descending pages while reads are themselves audited
    Given an access audit probe authenticated as "Contract Manager" for scope "contracts"
    When the access audit probe requests "audit" three times
    And the Auditor reads the first access-attempt page with limit 2
    Then the access-attempt page is bounded and ordered newest first
    And the successful access-attempt read is logged with its justification
    When the Auditor follows the access-attempt cursor
    Then the next page continues strictly before the previous page without duplicates

  @REQ-denied-access-audit-AC5
  Scenario Outline: Other human roles cannot read access-attempt metadata
    Given I am authenticated with roles: "<role>"
    When I request access attempts with a unique justification
    Then get http 403:401 Unauthorized
    And the access-attempt response exposes no attempt list

    Examples:
      | role             |
      | Archive Manager  |
      | Contract Manager |

  @REQ-denied-access-audit-AC5
  Scenario: The System Auditor cannot read the human audit access log
    When the system client "dcs-orce-notary" obtains an access token
    And the System Auditor requests access attempts with a unique justification
    Then get http 403:401 Unauthorized
    And the access-attempt response exposes no attempt list

  @REQ-denied-access-audit-AC6
  Scenario Outline: Access-attempt reads require a meaningful justification
    Given I am authenticated with roles: "Auditor"
    When I request access attempts with "<purpose>" justification
    Then get http 400:Bad Request
    And the access-attempt response exposes no attempt list

    Examples:
      | purpose |
      | missing |
      | empty   |
      | blank   |
