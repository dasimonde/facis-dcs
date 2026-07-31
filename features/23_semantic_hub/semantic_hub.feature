# Semantic Hub (DCS-FR-TR-03, UC-02-08, backend/design/semantic_hub.go):
# versioned storage for the JSON-LD contexts, SHACL shapes, and validation
# profiles every DCS document is produced against. Seeded at startup with the
# FACIS DCS v1 profile (backend/internal/semantichub/assets, authored under
# docs/semantic-ontology). Reads are public — produced artifacts carry
# hub-served anchors external verifiers resolve without a DCS login —
# writes are Template Manager-scoped. The normalization layer anchors every
# produced canonical document to the hub (@context URL, sh:shapesGraph) and rejects
# documents that redefine a hub-declared ontology prefix.

@DCS-FR-TR-03 @UC-02-08
Feature: Semantic Hub — versioned schema storage, anchoring, and enforcement

  Scenario: The genesis FACIS DCS context is seeded, active, and publicly resolvable
    When the active "context" schema "facis-dcs" is retrieved from the Semantic Hub
    Then get http 200:Success code
    And the retrieved schema is version 1, active, of kind "context"
    And the retrieved schema content declares the "dcs" ontology IRI "https://w3id.org/facis/dcs/ontology/v1#"
    When the JSON-LD context "facis-dcs" is resolved from the Semantic Hub without authentication
    Then get http 200:Success code
    And the resolved document carries a JSON-LD "@context" object

  Scenario: The genesis SHACL shapes are seeded and retrievable
    When the active "shapes" schema "facis-dcs" is retrieved from the Semantic Hub
    Then get http 200:Success code
    And the retrieved schema is version 1, active, of kind "shapes"

  # The dereference target of the dcst: taxonomy IRIs: w3id.org/facis/dcs/...
  # redirects (docs/w3id/facis) land on this public route. The dcs: envelope
  # terms have no ontology document; they dereference to the shapes graph
  # above, which is what actually constrains them.
  Scenario: The genesis domain-field ontology is seeded and publicly resolvable
    When the active "ontology" schema "facis-sla" is retrieved from the Semantic Hub
    Then get http 200:Success code
    And the retrieved schema is version 1, active, of kind "ontology"
    When the ontology "facis-sla" is resolved from the Semantic Hub without authentication
    Then get http 200:Success code

  @REQ-semantic-catalog-grouped-constraints-AC6 @DCS-FR-TR-03
  Scenario Outline: The default ontology provides multiple grouped catalogs for a constraint operand
    When the ontology "facis-sla" is resolved from the Semantic Hub without authentication
    Then get http 200:Success code
    And the ontology defines at least 2 labeled value catalogs with labeled concepts for ODRL left operand "<operand>"

    Examples:
      | operand |
      | purpose |
      | spatial |

  # Registered versions persist across suite runs (the hub is seeded once at
  # startup, not per run), so the assertions are relative to the versions
  # that existed when the scenario started — only the rollback target is
  # absolute, because version 1 is always the seeded genesis.
  @UC-02-08
  Scenario: A Template Manager registers a new context version and rolls back to the genesis version
    When the Template Manager registers a new active version of the "context" schema "facis-dcs" extending the genesis context
    Then get http 200:Success code
    And the schema registration reports a version above the genesis version as active
    And the Semantic Hub lists the registered version of the "context" schema "facis-dcs" as the single active one
    When the Template Manager rolls the "context" schema "facis-dcs" back to version 1
    Then get http 200:Success code
    And the Semantic Hub lists version 1 of the "context" schema "facis-dcs" as the single active one

  Scenario: A role outside the schema-management scope cannot register a schema version
    Given I am authenticated with roles: "Template Creator"
    When I attempt to register a Semantic Hub schema version with my current role
    Then the request is denied with a client error

  # The anchoring half of DCS-FR-TR-03: every produced canonical document
  # records the hub-served, versioned schema URLs it was produced against
  # (@context hub URL + sh:shapesGraph, injected by the normalization layer), and that anchor
  # RESOLVES against this instance's Semantic Hub.
  @DCS-FR-TR-03
  Scenario: A produced contract document is anchored to the Semantic Hub and the anchor resolves
    Given contract "Hub Anchored Contract" is in "Draft" status
    Then the contract "Hub Anchored Contract" carries a Semantic Hub schema anchor
    And the contract "Hub Anchored Contract"'s JSON-LD context anchor resolves to the hub's registered context

  # The enforcement half ("templating should use it"): the hub's active
  # context is authoritative for what the ontology prefixes mean — a template
  # redefining a hub-declared prefix to a different IRI is rejected at
  # creation.
  @DCS-FR-TR-03
  Scenario: A template redefining a hub-declared ontology prefix is rejected
    When a template is created whose "@context" redefines the "dcs" prefix to "https://evil.example/other-ontology#"
    Then the request is denied with a client error
    And the rejection names the Semantic Hub's active context

  # Externally anchored contexts: a document may reference context IRIs
  # beyond the hub's own, provided they are registered in the hub under
  # their original IRI — validation resolves them hermetically, never over
  # the network, so an unregistered IRI is rejected at creation.
  @DCS-FR-TR-03
  Scenario: A document can use an externally anchored context once it is registered in the hub
    When a template is created whose "@context" references the external context "https://example.org/bdd/unregistered-context"
    Then the request is denied with a client error
    When the Template Manager registers the external JSON-LD context "https://example.org/bdd/external-context" in the Semantic Hub
    Then get http 200:Success code
    When a template is created whose "@context" references the external context "https://example.org/bdd/external-context"
    Then get http 200:Success code

  # Phase 1 / ADR-8: before this, the SHACL shapes enforcing contract content
  # (PACM contract-content audit) were read straight off disk — registering,
  # activating, or rolling back a hub schema version changed nothing about
  # what was enforced. This is the scenario that proves the loop is closed:
  # activating a stricter shapes version changes findings for contracts
  # created afterward, while contracts already produced under the old
  # version keep revalidating exactly as before (sh:shapesGraph pins each
  # document to the hub version active at its own creation time). The engine
  # (ADR-9, goRDFlib) only reports non-conformance — a passing contract
  # produces no finding for a rule at all, not an "info" one.
  @DCS-FR-TR-03 @UC-02-08
  Scenario: Activating a stricter SHACL shapes version changes what NEW contracts get flagged for, while already-produced contracts stay pinned
    Given contract "Hub Pinned Pre-V2 Contract" is in "Draft" status
    When the Auditor triggers a process audit with scope "contracts"
    Then the contract content audit trail for "Hub Pinned Pre-V2 Contract" does not report an error for rule "title-InConstraintComponent"
    When the Template Manager registers a stricter version of the "shapes" schema "facis-dcs" that narrows the canonical contract title
    Then get http 200:Success code
    Given contract "Hub Strict Post-V2 Contract" is in "Draft" status
    When the Auditor triggers a process audit with scope "contracts"
    Then the contract content audit trail for "Hub Strict Post-V2 Contract" reports rule "title-InConstraintComponent" with severity "error"
    And the contract content audit trail for "Hub Pinned Pre-V2 Contract" does not report an error for rule "title-InConstraintComponent"
    When the Template Manager rolls the "shapes" schema "facis-dcs" back to version 1
    Then get http 200:Success code
    Given contract "Hub Restored Post-Rollback Contract" is in "Draft" status
    When the Auditor triggers a process audit with scope "contracts"
    Then the contract content audit trail for "Hub Restored Post-Rollback Contract" does not report an error for rule "title-InConstraintComponent"

  # Phase 3 (DCS-FR-TR-03/TR-04, ADR-10): the template builder's clause
  # palette is generated from this endpoint, not hand-authored — a clause
  # type is a real SHACL NodeShape in the hub (clause-catalog), pre-digested
  # server-side into a form-schema so the palette and what
  # validateAgainstHubShapes actually enforces on a submitted clause share
  # one source of truth. Typed-clause SHACL enforcement itself (a negative
  # sh:minInclusive amount rejected, a valid one accepted) is proven at the
  # Go unit level (TestAuditContractContentValidatesTypedClauses,
  # backend/internal/base/validation/contractcontentaudit_test.go), which
  # exercises the exact same concatenated shapes graph this endpoint serves.
  @DCS-FR-TR-03 @DCS-FR-TR-04
  Scenario: The clause catalog is seeded and publicly served as a generated form-schema
    When the Semantic Hub clause catalog is requested without authentication
    Then get http 200:Success code
    And the clause catalog lists a "dcs:PaymentClause" clause type whose shape declares "dcs:amount", "dcs:currency", "dcs:dueDays"
    And the clause catalog lists a "odrl:Duty" clause type whose shape declares "odrl:action", "odrl:constraint"
    And the clause catalog response carries the raw SHACL shapes it was derived from
