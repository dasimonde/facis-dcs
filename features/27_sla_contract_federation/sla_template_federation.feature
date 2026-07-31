# One authored SLA travels the whole federation path.
#
# The artefact is features/fixtures/sla_hosting_template.jsonld — a Managed
# Kubernetes Hosting SLA between Nordwind Cloud Services GmbH (Provider, on
# instance A) and Hansa Logistics AG (Customer, on instance B). It declares 9
# ODRL 2.2 rules over 13 contract fields across 7 sections, including a nested
# logical constraint tree, a duty whose non-fulfilment triggers a consequence,
# and 9 negotiated boundaries — right operands that reference a contract field
# instead of a literal, which is what makes a boundary negotiable at all.
#
# What these scenarios measure is not that a file copied. It is that instance
# A's AUTHORED ENFORCEMENT SEMANTICS crossed the Federated Catalogue intact and
# then BOUND instance B, which never saw instance A's code: the same rules, the
# same constraint tree, the same consequence, the same boundaries — and the
# same refusal when a negotiated value falls outside them.
#
# The @two-instance scenarios need a second real DCS process and
# BDD_DCS_BASE_URL_A/_B rather than the single-instance BDD_DCS_BASE_URL
# (dev-stack2.sh locally, the dcs-a/dcs-b Helm releases in CI). Instance B
# consumes instance A's Federated Catalogue rather than standing up its own
# (deployment/helm/values.bdd2.yml), which is precisely the A -> catalogue -> B
# path scenario 1 exercises.
#
# Every structural number below is asserted against BOTH copies and against the
# authored template; the step definitions count, they do not assume. A fixture
# that gains a rule therefore shows up as a diff in this file.

@UC-02 @DCS-IR-SI-01 @DCS-FR-TR-01 @DCS-FR-PACM-03 @NFR-BR-08
Feature: SLA contract federation — a template's enforcement semantics bind the instance that imported it

  # ---------------------------------------------------------------------
  # Federating the template itself.
  # ---------------------------------------------------------------------

  @two-instance @DCS-IR-SI-01 @UC-02
  Scenario: Instance A publishes the SLA template to the Federated Catalogue and instance B finds it
    Given instance A and instance B are both running and trust each other
    And the SLA hosting template is authored and registered on instance A
    When instance A publishes the SLA template to the Federated Catalogue
    And instance B searches the Federated Catalogue for the SLA template
    Then the catalogue search result on instance B includes the SLA template
    And the catalogued SLA entry carries the ODRL Offer and the sh:shapesGraph pin

  # An import mints a brand-new local DID on instance B and lands it in Draft
  # (backend/internal/templaterepository/command/register.go): the catalogue
  # hands over content, never authority — B's own repository governs what B
  # offers its customers. What must NOT change in transit is the ODRL.
  @two-instance @DCS-IR-SI-01 @DCS-FR-TR-01
  Scenario: Instance B registers the catalogued SLA template into its own repository
    Given instance A and instance B are both running and trust each other
    And the SLA hosting template is authored and registered on instance A
    And instance A publishes the SLA template to the Federated Catalogue
    When instance B registers the catalogued SLA template into its own repository
    Then the imported SLA template on instance B is in "Draft" status
    And the imported SLA template declares 2 permissions, 3 prohibitions and 4 obligations
    And the imported SLA template carries a logical constraint tree of depth 3
    And the imported SLA template carries 1 duty consequence
    And the imported SLA template carries 9 field-bound right operands
    And the imported SLA template pins the same sh:shapesGraph name and version as instance A's

  @two-instance @DCS-FR-TR-01 @UC-02
  Scenario: Instance B drives the imported SLA template through its own approval and registers it
    Given instance A and instance B are both running and trust each other
    And the SLA hosting template is authored and registered on instance A
    And instance A publishes the SLA template to the Federated Catalogue
    And instance B registers the catalogued SLA template into its own repository
    When instance B drives the imported SLA template through its own approval and registers it
    Then the imported SLA template on instance B is in "Registered" status

  # ---------------------------------------------------------------------
  # Instantiating and offering the contract. Instance B is the originator
  # here — the customer draws the agreement from the template it imported and
  # offers it back to the provider that published it.
  # ---------------------------------------------------------------------

  @two-instance @DCS-FR-CWE-03 @DCS-NFR-SQ-06
  Scenario: Instance B instantiates the SLA, fills every field, and offers it to instance A
    Given instance A and instance B are both running and trust each other
    And the SLA hosting template is authored and registered on instance A
    And instance A publishes the SLA template to the Federated Catalogue
    And instance B registers the catalogued SLA template into its own repository
    And instance B drives the imported SLA template through its own approval and registers it
    And instance B instantiates an SLA contract from its imported template with instance A as counterparty
    # A contract drawn from an imported template declares the upstream author's
    # shape library beside the canonical DCS graph. Every consequential
    # transition below is gated on an immutable snapshot built from that
    # declaration and the pinned bundle, so the two have to agree before the
    # offer is even attempted.
    Then the SLA contract on instance B pins an effective bundle covering every shapes graph it declares
    When instance B fills every open field of the SLA contract
    Then all 13 contract fields of the SLA contract carry a value on instance B
    And the SLA contract on instance B pins an effective bundle covering every shapes graph it declares
    When instance B offers the SLA contract to instance A
    Then the SLA contract is in state OFFERED on instance B
    And the SLA contract appears on instance A in state OFFERED within a few seconds
    And instance A's copy of the SLA contract carries the identical ODRL Offer
    # The copy that crossed carries instance B's pin under instance B's
    # hostname; instance A gates its own transitions on exactly that document.
    And the SLA contract on instance A pins an effective bundle covering every shapes graph it declares
    # What crossed must be the contract that was offered, not a render of it
    # taken before the fields were filled: the PDF the peer adopts as its copy
    # carries the document (ADR-13), so an offer whose PDF lags its own
    # contract_data would hand instance A a contract with 13 open fields.
    And all 13 contract fields of the SLA contract carry a value on instance A

  # ---------------------------------------------------------------------
  # THE LOAD-BEARING SCENARIO.
  #
  # Instance A — the instance that AUTHORED the template — counter-offers a
  # committed availability of 94.0, below the floor its own template sets
  # (odrl:gteq 99.5 on the availability duty). Negotiation is free to move
  # values, so the counter-offer itself is recorded and ships; the floor bites
  # at the approval gate (ValidateContractPolicySatisfaction, approve.go), and
  # it bites on BOTH copies.
  #
  # That is the whole demonstration. The refusal on instance B proves the
  # semantics travelled: B evaluates a constraint it never wrote, over a
  # vocabulary it received through the catalogue, and reaches the same verdict
  # as A. The refusal on instance A proves the floor is not a courtesy the
  # author can waive for itself.
  # ---------------------------------------------------------------------

  @two-instance @DCS-FR-PACM-03 @DCS-FR-CWE-18 @UC-03-01
  Scenario: A negotiated availability below the template's own floor is refused on both instances
    Given instance A and instance B are both running and trust each other
    And the SLA hosting template is authored and registered on instance A
    And instance A publishes the SLA template to the Federated Catalogue
    And instance B registers the catalogued SLA template into its own repository
    And instance B drives the imported SLA template through its own approval and registers it
    And instance B instantiates an SLA contract from its imported template with instance A as counterparty
    And instance B fills every open field of the SLA contract
    And instance B offers the SLA contract to instance A
    And the SLA contract appears on instance A in state OFFERED within a few seconds
    When instance A counter-offers a committed availability of "94.0" on the SLA contract
    Then the SLA contract shows a committed availability of "94.0" on instance A within a few seconds
    And the SLA contract's ODRL is structurally intact on instance A
    And the SLA contract shows a committed availability of "94.0" on instance B within a few seconds
    And the SLA contract's ODRL is structurally intact on instance B
    When instance A drives its copy of the SLA contract to REVIEWED
    And instance A attempts to approve the SLA contract
    Then the approval on instance A is refused naming the availability constraint
    When instance B drives its copy of the SLA contract to REVIEWED
    And instance B attempts to approve the SLA contract
    Then the approval on instance B is refused naming the availability constraint

  # ---------------------------------------------------------------------
  # The ping-pong the negotiated boundaries exist for: the template fixes the
  # boundary, the parties haggle the value inside it. A counters 99.9 -> 99.7,
  # B counters back 99.7 -> 99.8, each hop shipping a re-rendered PDF over the
  # DCS-to-DCS exchange (ADR-13). The assertion after every hop is that the
  # POLICY did not move — negotiation moves values, never structure.
  #
  # Both counters stay inside the floor, so both copies settle and approve, and
  # the agreement is then signed on both sides. The renegotiation attempt at
  # the end must fail: once the artefact carries the counterparty's signature a
  # redline would ship the old signed bytes against a moved document
  # (command.ErrAgreementSettled for a copy still in OFFERED/NEGOTIATION, the
  # missing EventNegotiate edge out of SIGNED for one that has signed). That is
  # why every hop above happens BEFORE anyone signs.
  # ---------------------------------------------------------------------

  @two-instance @DCS-FR-CWE-03 @DCS-NFR-BR-08 @UC-03-01
  Scenario: The federated SLA is negotiated across instances, reaches SIGNED on both, and seals as an odrl:Agreement
    Given instance A and instance B are both running and trust each other
    And the SLA hosting template is authored and registered on instance A
    And instance A publishes the SLA template to the Federated Catalogue
    And instance B registers the catalogued SLA template into its own repository
    And instance B drives the imported SLA template through its own approval and registers it
    And instance B instantiates an SLA contract from its imported template with instance A as counterparty
    And instance B fills every open field of the SLA contract
    And instance B offers the SLA contract to instance A
    And the SLA contract appears on instance A in state OFFERED within a few seconds

    When instance A counter-offers a committed availability of "99.7" on the SLA contract
    Then the SLA contract shows a committed availability of "99.7" on instance B within a few seconds
    And the SLA contract's ODRL is structurally intact on instance B

    When instance B counter-offers a committed availability of "99.8" on the SLA contract
    Then the SLA contract shows a committed availability of "99.8" on instance A within a few seconds
    And the SLA contract's ODRL is structurally intact on instance A

    When instance B drives its copy of the SLA contract to REVIEWED
    And instance B attempts to approve the SLA contract
    Then the approval on instance B succeeds
    When instance A drives its copy of the SLA contract to REVIEWED
    And instance A attempts to approve the SLA contract
    Then the approval on instance A succeeds

    When instance A applies a ceremony-backed signature to the contract
    And instance B applies a ceremony-backed signature to the contract
    Then the contract state "SIGNED" is replicated on both instance A and instance B
    And the sealed policy of the SLA contract on instance A is an odrl:Agreement
    And the sealed policy of the SLA contract on instance B is an odrl:Agreement
    And the SLA contract's ODRL is structurally intact on instance A
    And the SLA contract's ODRL is structurally intact on instance B
    And renegotiating the signed SLA contract on instance A is refused

  # ---------------------------------------------------------------------
  # The same authored SLA in operation, on one instance and with no Federated
  # Catalogue involved. The parties committed 99.9% availability; the target
  # system reports a measured 98.2%. The reported metric IS the committed
  # availability field's node IRI, which is how the KPI binds to the ODRL
  # constraint that governs it (DCS-FR-CWE-09) — and DCS-FR-CWE-31 requires
  # more than recording the breach, so the compliance monitor an officer
  # actually watches must surface it as well.
  # ---------------------------------------------------------------------

  @DCS-FR-CWE-09 @DCS-FR-CWE-31 @DCS-FR-PACM-03 @DCS-FR-UC-06-1
  Scenario: A reported availability below the committed level raises an underperformance alert
    Given I am authenticated with roles: "Contract Creator"
    And a filled SLA contract "Federated SLA Operation" exists on this instance
    And the committed availability of contract "Federated SLA Operation" is "99.9"
    And contract "Federated SLA Operation" is submitted, reviewed, approved, and signed via the standard workflow
    And an authorized user deploys contract "Federated SLA Operation" to the configured contract target
    And get http 200:Success code
    And the contract target acknowledges the deployment of contract "Federated SLA Operation"
    When the target reports an availability KPI value "98.2" for contract "Federated SLA Operation"
    Then get http 200:Success code
    And the contract detail for "Federated SLA Operation" shows a KPI violation flag for its committed availability
    When the Compliance Officer requests continuous monitoring
    Then get http 200:Success code
    And the monitoring sweep flags contract "Federated SLA Operation" with a "CONTRACT_UNDERPERFORMANCE" compliance risk
    And the flagged risk for contract "Federated SLA Operation" is recorded in the PAC audit trail
