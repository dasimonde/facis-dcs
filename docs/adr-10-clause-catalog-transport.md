# ADR-10: Clause catalog transport — pre-digested JSON, raw Turtle alongside

## Context

Phase 3 (DCS-FR-TR-03/TR-04) needs the template builder's clause palette to
be generated from hub-stored SHACL NodeShapes rather than a hand-authored
list, and a submitted typed clause to be validated by the same shapes
server-side — one source of truth. That requires a transport for
`GET /semantic/clauses`: raw SHACL Turtle for the client to interpret, or a
form-schema the backend derives from it.

## Decision

**Both**, in one response (`ClauseCatalogResponse`): `clauses` is a
pre-digested JSON array (`ParseClauseCatalog`,
`backend/internal/semantichub/clausecatalog.go`) — each clause type's
`@type`, label, and properties with their datatype/`sh:in`/min-max
constraints, walked directly off the parsed SHACL graph (goRDFlib, ADR-9) —
and `shapes` is the raw Turtle it was derived from.

- The pre-digested JSON is what the frontend renders today (task 3.3/3.4):
  a plain Vue form generated from `clauses`, no SHACL-aware client library
  required. This was the pragmatic choice given the time budget — evaluating
  `@ulb-darmstadt/shacl-form` (a framework-agnostic web component that
  consumes raw SHACL directly) was in scope per the plan, but wiring a new
  external web-component dependency into the existing Vue 3 builder UI and
  validating it doesn't fight the existing block/placement model is a
  bigger integration than fits alongside Phases 1/2/4 in the same pass.
- The raw Turtle travels alongside it anyway, so a future `shacl-form`
  integration (or a completely different client) is not blocked on a second
  server-side change — the transport already carries what it would need.
- The digest is derived server-side, not authored separately: there is
  exactly one place (the clause-catalog SHACL, `backend/internal/
  semantichub/assets/facis-dcs-clause-catalog.ttl`) that defines a clause
  type. The palette and enforcement (`HubShapeSource.ActiveShapes`
  concatenates the clause catalog into the same graph
  `validateAgainstHubShapes` checks contracts against, ADR-8/ADR-9) both
  derive from it — they cannot drift apart the way a hand-maintained JSON
  schema alongside hand-maintained SHACL would.
- Versioned like every other hub kind="shapes" entry
  (`semantichub.ClauseCatalogName = "clause-catalog"`), independently of the
  canonical contract shapes (`semantichub.ShapesName = "facis-dcs"`):
  registering a stricter clause-catalog version changes what new clause
  instances validate against without touching the canonical contract shape.

## Consequences

- `GET /semantic/clauses` is a Semantic Hub read, public like
  `resolve_context` — a produced contract's typed clauses need to be
  independently re-verifiable by an external party the same way the rest of
  the document's hub anchors (`@context` URL, `sh:shapesGraph`,
  `dcterms:conformsTo`) are.
- The clause catalog's own hub version is not pinned in a produced
  document (only the canonical shapes' version is, via `sh:shapesGraph`,
  ADR-8) — `HubShapeSource.ActiveShapes`/`ShapesAt` always concatenate the
  clause catalog's *current* active version, even during a pinned
  canonical-shapes revalidation. Acceptable for this phase: clause
  instances are new, additive content, not yet a first-class pinned
  artifact the way the canonical contract shape is. A future phase could
  add a second `sh:shapesGraph` entry (the property is naturally
  multi-valued) naming the pinned clause-catalog version if that
  divergence needs closing.
