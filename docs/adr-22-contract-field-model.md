# ADR-22: Contract fields — the typed node is a field, not a placeholder

Status: Accepted (2026-07-24). **Supersedes
[ADR-15](adr-15-placeholder-typed-node.md)**, keeping its core (one typed,
SHACL-derived, @id-linked, self-contained node per negotiable data point) and
replacing its naming and top-level layout.

> **Amended by [ADR-23](adr-23-generic-contract-data-graph.md) (2026-07-26).**
> The "field references only" restriction on domain-object properties is
> lifted: `dcs:contractData` is a generic object graph whose properties hold
> literals, field references, or references to other domain objects.

## Context

ADR-15 collapsed the four-hop placeholder indirection (`dcs:bindsTo`,
`dcs:DataRequirement`, `dcs:RequirementField`, dot-paths) into one typed
`dcs:Placeholder` node under `dcs:contractData`. Two problems remained:

1. **"Placeholder" is the wrong terminus.** A placeholder is an authoring-time
   concept: a slot awaiting a value. A finalized contract carries no
   placeholders — its terms were agreed. The same node must describe both the
   open slot on a template and the agreed value on a signed contract, so it is
   a *contract field*, of which "unfilled" is merely a state
   (`dcs:required` without `dcs:value`).
2. **`dcs:contractData` was a misnomer.** It held field *declarations*, not
   business data, leaving no place for typed domain objects (a payment clause,
   an SLA target) that machine consumers actually want to read.

## Decision

Canonical templates and contracts separate field declarations from business
data:

- `dcs:contractFields` is a flat list of `dcs:ContractField` nodes. Every
  field has `@id`, `dcs:label`, `dcs:datatype`, and boolean `dcs:required`;
  it may additionally carry `dcs:shape` and, on a filled contract,
  `dcs:value`.
- `dcs:contractData` contains typed domain objects such as
  `dcs:PaymentClause`. Their business properties bind to fields using bare
  `{"@id":"…"}` references — no literals and no nested objects, so every
  value has exactly one home. A domain object graph is fully coherent on its
  own: it is SHACL-validated against the shape libraries registered in the
  Semantic Hub (`dcs:shape` on the field links the governing shape), so
  arbitrary vocabularies pulled into the hub drive the template editor,
  contract editor, and negotiation with proper typed data.
- `dcs:documentStructure` is the human-readable structure. Layout children
  use `{"@list":[{"@id":"…"}]}` and clause content uses only bare field
  references.
- ODRL operands reference the same field identifiers.

During templating a field may be open — declared but without `dcs:value`.
Open required fields must be filled during negotiation: the offer gate
(`validateOfferReady` / `ValidateContractClosed`) rejects a contract that
still carries an unfilled required field, so neither party can sign a
document with open terms.

Example:

```json
{
  "@type": "dcs:Contract",
  "dcs:contractFields": [
    {
      "@id": "#payment-amount",
      "@type": "dcs:ContractField",
      "dcs:label": "Payment amount",
      "dcs:datatype": "xsd:decimal",
      "dcs:required": true,
      "dcs:value": 15000
    }
  ],
  "dcs:contractData": [
    {
      "@id": "#payment",
      "@type": "dcs:PaymentClause",
      "dcs:amount": {"@id": "#payment-amount"}
    }
  ]
}
```

The document is self-contained: authoring, validation, rendering, and policy
evaluation resolve a field by `@id` without consulting a template snapshot.

## Consequences

- Validation enforces the two-level model: every `dcs:ContractField` carries
  `@type`, `dcs:label`, `dcs:datatype`, and boolean `dcs:required`
  (`canonicalFieldIDs`); every business property of a `dcs:contractData`
  domain object must reference a declared field
  (`validateContractDataFieldReferences`); clause content may reference only
  declared fields.
- pdf-core resolves clause `{"@id"}` references directly against the
  top-level `dcs:contractFields` registry (unfilled required fields render as
  `_____`); the DCS-side inline-copy step (`flatten.go`) is deleted.
- ADR-15's invariants stand unchanged: inline SHACL-derived `dcs:datatype`,
  @id-linking from clause content and ODRL, self-contained top level after
  composition, explicit `{"@list"}` for `dcs:blocks` / `dcs:layout` /
  `dcs:children` / `dcs:content`.
