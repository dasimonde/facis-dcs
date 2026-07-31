# Machine-readable ODRL soundness and server-side policy enforcement
# (SRS: DCS-FR-PACM-03).
#
# Scope: a contract's rules live inside ONE enclosing policy node (an
# odrl:Offer while unsigned, sealed into the odrl:Agreement the signatures
# bind by the first signature; @id anchored to the contract DID,
# odrl:profile declared); every rule carries exactly one odrl:action plus
# odrl:assigner/odrl:assignee/odrl:target; and constraint violations are
# enforced server-side at both the approval and the signing entry points. The Scenario Outline proves operator evaluation for all 8
# ODRL operators (eq, neq, isAnyOf, isNoneOf, gteq, lteq, gt, lt); operator
# evaluation is additionally covered by the Go unit tests in
# backend/internal/base/validation/contractcontentaudit_test.go. The bare-
# Duty shape (no action, no enclosing policy node) is rejected by structural
# validation.
#
# The "direct raw API call" scenario attempts the sign call BEFORE the
# contract is approved (state REVIEWED). This is deliberate: a constraint-
# violating contract can never legitimately reach APPROVED (the only state
# /signature/apply accepts), so a raw sign attempt against it must always
# be rejected — proving the signing path enforces policies independently of
# the approval gate.
#
# The peer-action entry path is not re-tested separately: it dispatches
# through the same approval command handler as the UI/API path
# (backend/internal/service/dcs_to_dcs.go), so it is covered by the same
# enforcement point.

@DCS-FR-PACM-03
Feature: Machine-readable ODRL soundness and server-side policy enforcement

  @DCS-FR-PACM-03
  Scenario: A contract's ODRL policies form one enclosing Offer with profile, action, parties, and target
    Given a fresh draft contract "ODRL Structure Contract"
    When the policies of contract "ODRL Structure Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "DEU"
    Then the policy update for contract "ODRL Structure Contract" is accepted
    And the stored policies of contract "ODRL Structure Contract" form a single enclosing odrl:Offer whose @id is anchored to the contract DID and which declares an odrl:profile
    And every stored policy rule of contract "ODRL Structure Contract" declares exactly one odrl:action
    And every stored policy rule of contract "ODRL Structure Contract" declares an odrl:assigner, odrl:assignee, and odrl:target

  @DCS-FR-PACM-03
  Scenario: The first signature seals the offered policy set into the Agreement the signatures bind
    Given a fresh draft contract "ODRL Seal Contract"
    When the policies of contract "ODRL Seal Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "DEU"
    Then the policy update for contract "ODRL Seal Contract" is accepted
    And the stored policies of contract "ODRL Seal Contract" form a single enclosing odrl:Offer whose @id is anchored to the contract DID and which declares an odrl:profile
    When contract "ODRL Seal Contract" is submitted, reviewed, approved, and signed via the standard workflow
    Then the stored policies of contract "ODRL Seal Contract" form a single enclosing odrl:Agreement whose @id is anchored to the contract DID and which declares an odrl:profile

  @DCS-FR-PACM-03 @DCS-FR-UC-10-2
  Scenario: A contract with a violated ODRL constraint cannot be approved
    Given a fresh draft contract "ODRL Violation Contract"
    When the policies of contract "ODRL Violation Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "FRA"
    Then the policy update for contract "ODRL Violation Contract" is accepted
    When approval is attempted for contract "ODRL Violation Contract"
    Then the approval is rejected because an ODRL constraint is violated

  @DCS-FR-PACM-03
  Scenario: A contract with a violated ODRL constraint cannot be signed via a direct raw API call
    Given a fresh draft contract "ODRL Raw Sign Contract"
    When the policies of contract "ODRL Raw Sign Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "FRA"
    Then the policy update for contract "ODRL Raw Sign Contract" is accepted
    When a direct signing API call is attempted against contract "ODRL Raw Sign Contract" before it is approved
    Then the sign attempt for contract "ODRL Raw Sign Contract" is rejected and the contract remains unsigned

  @DCS-FR-PACM-03
  Scenario: A contract with satisfied ODRL constraints is approved and signed normally
    Given a fresh draft contract "ODRL Satisfied Contract"
    When the policies of contract "ODRL Satisfied Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "DEU"
    Then the policy update for contract "ODRL Satisfied Contract" is accepted
    When the contract "ODRL Satisfied Contract" is submitted, reviewed, approved, and signed via the standard workflow
    Then the contract "ODRL Satisfied Contract" reaches SIGNED state

  @DCS-FR-PACM-03
  Scenario: A bare-Duty policy shape is rejected by structural validation
    Given a fresh draft contract "ODRL Bare Shape Contract"
    When the policies of contract "ODRL Bare Shape Contract" are updated to the bare-Duty form (field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "DEU"
    Then the policy update for contract "ODRL Bare Shape Contract" is rejected because the bare-Duty form lacks an action and enclosing policy

  @DCS-FR-PACM-03
  Scenario Outline: Server-side operator evaluation covers all 8 ODRL operators correctly
    Given contract "<name>" is a fresh draft whose ODRL policy constrains field "<field>" using operator "<operator>" against "<right_operand>" while the actual value is "<actual_value>"
    When approval is attempted for contract "<name>"
    Then the approval outcome for contract "<name>" is "<expect>"

    Examples: string operators
      | name                       | field   | operator | right_operand | actual_value | expect    |
      | ODRL Operator eq sat       | country | eq       | DEU            | DEU          | satisfied |
      | ODRL Operator eq viol      | country | eq       | DEU            | FRA          | violated  |
      | ODRL Operator neq sat      | country | neq      | DEU            | FRA          | satisfied |
      | ODRL Operator neq viol     | country | neq      | DEU            | DEU          | violated  |
      | ODRL Operator isAnyOf sat  | country | isAnyOf  | DEU,AUT,CHE    | AUT          | satisfied |
      | ODRL Operator isAnyOf viol | country | isAnyOf  | DEU,AUT,CHE    | FRA          | violated  |
      | ODRL Operator isNoneOf sat | country | isNoneOf | DEU,AUT,CHE    | FRA          | satisfied |
      | ODRL Operator isNoneOf viol| country | isNoneOf | DEU,AUT,CHE    | DEU          | violated  |

    Examples: numeric operators
      | name                       | field    | operator | right_operand | actual_value | expect    |
      | ODRL Operator gt sat       | coverage | gt       | 95             | 99           | satisfied |
      | ODRL Operator gt viol      | coverage | gt       | 95             | 90           | violated  |
      | ODRL Operator gteq sat     | coverage | gteq     | 95             | 95           | satisfied |
      | ODRL Operator gteq viol    | coverage | gteq     | 95             | 90           | violated  |
      | ODRL Operator lt sat       | coverage | lt       | 5              | 2            | satisfied |
      | ODRL Operator lt viol      | coverage | lt       | 5              | 8            | violated  |
      | ODRL Operator lteq sat     | coverage | lteq     | 5              | 5            | satisfied |
      | ODRL Operator lteq viol    | coverage | lteq     | 5              | 8            | violated  |
