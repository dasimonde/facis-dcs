# ADR-15: One typed placeholder node — SHACL-typed, @id-linked, self-contained

Status: Accepted (2026-07-20).

> **Superseded by [ADR-22](adr-22-contract-field-model.md) (2026-07-24).**
> The typed node survives, renamed `dcs:Placeholder` → `dcs:ContractField`
> (a finalized contract carries no placeholders — its terms are agreed), and
> the flat registry moves from `dcs:contractData` to `dcs:contractFields`;
> `dcs:contractData` now holds typed domain objects that bind to fields by
> bare `@id` reference.

## Context

A negotiable/fillable data point in a contract (e.g. a payment amount) is today
expressed through four layers of indirection in the contract JSON-LD:

- a `dcs:Placeholder` in the human-readable clause content that `dcs:bindsTo` an
  `@id`;
- a `dcs:DataRequirement` wrapper carrying a `dcs:conditionId`;
- a `dcs:RequirementField` with a dot-path `dcs:parameterName`
  (`contract.payment.amount`), a `dcs:domainField` reference, and no inline type;
- the actual datatype only reachable by chasing `dcs:domainField` → taxonomy →
  the SHACL shape (`sh:datatype`).

The nodes are addressed with template-DID-laden `@id`s
(`…/api/template/<uuid>#a0bc1650-…`). To read a field's TYPE a renderer must
resolve four hops across two graphs; to fill it, a fifth. Worse, when a clause
comes from a composed component (sub-template) the field definition lives only in
the sub-template snapshot — top-level `dcs:contractData` is empty — and the render
merge only fires for `dcs:ApprovedTemplate` blocks, not flattened clauses
(`useContractDataPreprocess.ts`). The placeholder never resolves to a typed
parameter, so no input renders in the contract / negotiate / signing views. This
is what blocked the two-instance vertical's Stage-6 negotiation: the counterparty
had nothing to edit.

The indirection buys nothing the graph does not already give us: JSON-LD is a
graph, and one node can be referenced by `@id` from as many places as need it.

## Decision

A placeholder is **one typed graph node**. It carries its type straight from the
SHACL shape and is linked by `@id` into BOTH the human-readable clause and the
ODRL policy — the graph does the joining:

```json
// declared once, self-contained, type inlined from the shape's sh:datatype:
{ "@id": "#payment-amount", "@type": "dcs:Placeholder",
  "dcs:label": "Payment Amount",
  "dcs:datatype": "xsd:decimal",
  "dcs:shape": { "@id": "…#PaymentClauseShape" },
  "dcs:value": 15000 }
// human-readable clause references it by @id:
"dcs:content": { "@list": [ "…payment amount of ", { "@id": "#payment-amount" } ] }
// ODRL constraint references the SAME @id:
"odrl:rightOperand": { "@id": "#payment-amount" }
```

Removed entirely: `dcs:RequirementField`, `dcs:DataRequirement`,
`dcs:domainField`, `dcs:conditionId`, dot-path `dcs:parameterName`, and
DID-laden `@id`s (use clean local fragment `@id`s).

The contract document is **self-contained at top level**: flattening a
sub-template's clause into a contract also places its placeholder node(s) — with
the inline `dcs:datatype` — at top level. A renderer (frontend preview and
pdf-core) resolves the node by `@id`, reads `dcs:datatype`, and picks the input
with no sub-template or taxonomy chasing. Type resolution against the shape
(`sh:datatype` → `dcs:datatype`) happens once, at authoring / derivation.

## Consequences / migration (the actual work)

- Authoring (clause-editor) emits the typed node + the `@id` reference in the
  clause and the ODRL rule (`OdrlRuleBuilder`).
- Backend template→contract derivation
  (`ConvertTemplateDataToContractData`, `validation/documentdata.go`,
  `NormalizeContractDataForPersistence`) produces the self-contained top-level
  form and inlines flattened placeholder nodes.
- pdf-core flatten/render (`pdfcore/flatten.go`, client.go, renderer) reads the
  typed node.
- Frontend render (`PreviewClauseBlock`, `PreviewParamInput`, `TemplatePreview`,
  `useContractDataPreprocess`, `dcsDraftStore`, `semantic-parameter-label`,
  `ontology-domain-fields`) resolves the `@id`-linked typed node.
- ODRL / OPA evaluation references the placeholder by `@id`.
- **Hard-fail** when a placeholder's shape carries no datatype — that is an
  authoring error, never a silent default.
- Greenfield: no dual-path/compat/mappers — delete the old layered form in one
  wave (the DB may be wiped). Fixtures and BDD steps that encode the old shape
  move to the new node.

Complements ADR-14 (one expanded JSON-LD form) and ADR-9 (SHACL as the type
authority); the shape stays the type source, the placeholder just carries the
answer. Values remain inline on the node (unchanged).
