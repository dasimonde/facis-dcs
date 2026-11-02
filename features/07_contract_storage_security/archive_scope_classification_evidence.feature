Feature: Archive snapshot, party classification, organization scope and evidence

  @DCS-FR-CSA-12 @DCS-IR-CSA-06
  Scenario: Observer reads the selected immutable archive version after the live record changes
    Given a dedicated contract "Frozen Contract" has reached SIGNED
    And the Observer remembers the archived version of contract "Frozen Contract"
    And the live contract "Frozen Contract" advances to a newer version through the database fixture seam
    When the Observer opens the remembered archived version of contract "Frozen Contract"
    Then the archive detail is the unchanged snapshot for contract "Frozen Contract" with matching identity and hash
    When the Observer requests a non-archived version of contract "Frozen Contract"
    Then the response status is 404

  @DCS-FR-TR-27 @DCS-IR-TR-02 @DCS-IR-SI-01
  Scenario Outline: Explicit party classification survives editing and copying and combines with local and catalogue text filters
    Given template "Party Match Classification" is created as "<template_type>" with party classification "<party_type>"
    And template "Party Match Other Party" is created as "<template_type>" with party classification "<other_party>"
    And template "Unrelated Name" is created as "<template_type>" with party classification "<party_type>"
    When the template "Party Match Classification" is edited without a party field and copied
    Then template "Party Match Classification", its history and its copy retain "<template_type>" and "<party_type>"
    When the local search combines name "Party Match" and party classification "<party_type>"
    Then the search includes template "Party Match Classification" and excludes "Party Match Other Party" and "Unrelated Name"
    When templates "Party Match Classification", "Party Match Other Party" and "Unrelated Name" are published to the catalogue
    And the catalogue search combines name "Party Match" and party classification "<party_type>"
    Then the search includes template "Party Match Classification" and excludes "Party Match Other Party" and "Unrelated Name"

    Examples:
      | template_type     | party_type   | other_party  |
      | CONTRACT_TEMPLATE | SINGLE_PARTY | MULTI_PARTY  |
      | CONTRACT_TEMPLATE | MULTI_PARTY  | SINGLE_PARTY |
      | COMPONENT         | SINGLE_PARTY | MULTI_PARTY  |
      | COMPONENT         | MULTI_PARTY  | SINGLE_PARTY |

  @DCS-FR-CWE-07 @DCS-NFR-SEC-03
  Scenario: Dashboard lists and open tasks are shared within an organization and hide unrelated organizations
    Given a dedicated contract "Visible Org Contract" has reached SIGNED
    And a dedicated contract "Foreign Org Contract" has reached SIGNED
    And contracts "Visible Org Contract" and "Foreign Org Contract" have open local review, approval and negotiation tasks through the database fixture seam
    When the organization of contract "Visible Org Contract" retrieves the contract dashboard
    Then the dashboard includes contract "Visible Org Contract" and its open tasks but excludes "Foreign Org Contract" and its tasks
    And the organization of contract "Visible Org Contract" sees it in contract, archive and signing searches but cannot see "Foreign Org Contract"
    When a Contract Manager from the organization of "Visible Org Contract" requests "/contract/retrieve/{did}" for contract "Visible Org Contract"
    Then the response status is 200
    And the cross-organization roles of an unrelated organization cannot retrieve contract "Foreign Org Contract"
    And a system client acting for the instance can retrieve contract "Foreign Org Contract"

  @DCS-FR-CWE-07 @DCS-NFR-SEC-03
  Scenario Outline: Indirect contract reads reject an unrelated organization with an otherwise valid role
    Given a dedicated contract "Authorized Org" has reached SIGNED
    And a dedicated contract "Indirect Foreign" has reached SIGNED
    When a Contract Manager from the organization of "Authorized Org" requests "<path>" for contract "Indirect Foreign"
    Then the response status is 403

    Examples:
      | path                        |
      | /contract/retrieve/{did}    |
      | /contract/history/{did}     |
      | /signature/retrieve/{did}   |
      | /pdf/export/contract/{did}  |
      | /contract/export/{did}      |

  # The archive deletion erases only a contract that is no longer in force, so
  # the signed contract is terminated before the deletion.
  @DCS-IR-CSA-02 @DCS-IR-CWE-12 @DCS-NFR-SEC-13
  Scenario: Contract Manager retrieves exact uploaded evidence until archive deletion shreds its key
    Given a dedicated contract "Evidence Contract" has reached SIGNED
    And the evidence file "evidence.bin" contains hexadecimal bytes "00ff0102030451612045766964656e63650ac3a4"
    When the Contract Manager uploads the file to contract "Evidence Contract" with description "Confidential evidence description"
    And the Contract Manager downloads that evidence from contract "Evidence Contract"
    Then the evidence has the original file bytes, metadata and server SHA256 for contract "Evidence Contract"
    When the Contract Manager terminates the signed contract "Evidence Contract"
    And the Archive Manager deletes contract "Evidence Contract" with an erasure justification
    Then the response status is 200
    And the contract "Evidence Contract" has a shredded key and its evidence is unreadable
