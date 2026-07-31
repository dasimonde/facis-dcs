# ADR-9: goRDFlib as the SHACL engine, replacing the hand-rolled subset matcher

## Context

`AuditContractContent`'s SHACL enforcement (DCS-FR-TR-03) was a hand-rolled,
regex/string-based structural matcher (`parseContractSHACLShapesTTL`,
`contractSHACLStatementHasType`, `auditContractSHACLProperty`) — it
supported only type checks, `sh:in`, `sh:minCount`/`sh:maxCount`,
`sh:datatype`, `sh:minInclusive`, `sh:class`, `sh:node`, was tolerant of
Turtle that isn't actually valid (`sh:path did` — a bare token with no
prefix separator, not a legal Turtle prefixed name), and produced no
machine-readable `sh:ValidationReport`. ADR-8 closed the loop on *where*
shapes come from (the Semantic Hub); this ADR replaces *what evaluates
them* with a conformant SHACL-core processor.

## Engine decision

**`github.com/tggo/goRDFlib` (`shacl` package) — pure Go, in-process.**

- License: BSD-3-Clause (`LICENSE` in the module, "Copyright (c) 2025-2026,
  goRDFlib Contributors"), compatible with this project's Apache-2.0; the
  BSD notice is retained via the vendored `go.sum` entry (no code is
  copied/forked into this repo). As an `eclipse-xfsc`-adjacent dependency
  pulled into an Eclipse-hosted project's supply chain, BSD-3-Clause is on
  the Eclipse Foundation's pre-approved license list — a Dash IP due
  diligence entry is procedural, not a blocker.
- Transitive footprint verified by import graph, not by go.mod inspection
  alone: `go list -deps ./shacl/... ./turtle/... ./jsonld/...` (run against
  the pinned commit) pulls in `github.com/cayleygraph/quad` (Apache-2.0,
  lightweight N-Quads utilities) but **not** `github.com/dgraph-io/badger/v4`
  or `modernc.org/sqlite` — goRDFlib's own `store` package (the persistent
  KV/SQL-backed graph stores) is not imported by `shacl`/`graph`/`turtle`/
  `jsonld`. `go.sum` carries checksum entries for badger/sqlite anyway
  (Go's minimal-version-selection resolves the *whole* module's go.mod
  graph, not just the imported subset) but neither is compiled into the
  binary — confirmed with `go build` producing no reference to either
  package. This is the IP-review-relevant fact: the persistent stores are
  not linked, only their checksums appear in `go.sum`.

### Verification gate (mandatory, run before wiring in)

Cloned `github.com/tggo/goRDFlib` at tag `v0.1.13`
(commit `202c6675e2f8d650fd49006e72622d9e7a630a61`, 2026-07-02) with
submodules (`git clone --recurse-submodules`; the W3C test data
— `testdata/w3c/data-shapes` — is a submodule of
`https://github.com/w3c/data-shapes.git`, `gh-pages` branch, not fetched by
a bare clone) and ran the library's own W3C SHACL/SHACL-1.2 conformance
suite locally:

```
go test ./shacl/ -run TestW3C -v -count=1
```

Result: **388/388 sub-tests pass, 0 failures** — `TestW3CCoreTests`
(98/98), `TestW3CSPARQLTests` (22/22), `TestW3CSHACL12CoreTests` (132/132),
`TestW3CSHACL12SPARQLTests` (24/24), `TestW3CSHACL12NodeExprTests` (24/24),
`TestW3CSHACL12NodeExprConstraintTests` (2/2), plus the SHACL-1.2 rules
suites (`TestW3CSRLSyntaxTests`, `TestW3CSRLWellformedTests`,
`TestW3CSRLStratificationTests`, `TestW3CSRLEvalTests`) all green. The full
package test suite (`go test ./shacl/...`, including non-W3C unit tests)
also passes. Pinned by exact version (`v0.1.13`) in `go.mod`; every future
bump is a deliberate upgrade with the W3C suite re-run before merging.

Gate passed — proceeded to wire the engine in rather than falling back to
the pySHACL-sidecar escape hatch.

## Integration

- `backend/internal/base/validation/shaclengine.go`:
  `validateAgainstHubShapes(ctx, contract)` fetches the hub's SHACL shapes
  (active, or pinned per ADR-8) and active JSON-LD context via
  `ShapeSource`, parses the contract document through
  `goRDFlib/shacl.LoadJsonLDString` and the shapes through
  `shacl.LoadTurtleString`, and calls `shacl.Validate(dataGraph,
  shapesGraph)`.
- **Hermetic context resolution** (commit 1fa4a097 established hermetic
  runtime deps for this codebase — SHACL validation must not regress it):
  `hermeticContextLoader` wraps `piprate/json-gold`'s `ld.DocumentLoader`
  with a static, in-process cache of the hub's own active JSON-LD context.
  Any other context IRI a document references hard-fails
  (`"network fetch during validation is disallowed"`) rather than
  triggering an HTTP round-trip. In practice the loader is rarely even
  invoked: the canonical envelope (`normalizeCanonicalContext`) always
  embeds `@context` inline as a JSON object, which needs no dereferencing
  at all — the offline cache is a defensive backstop for anything that
  references a context by URL.
- **Finding mapping** (`mapShaclReport`/`shaclResultFinding`): each
  `shacl.ValidationResult` becomes a `PolicyFinding` — the same struct every
  other audit source in this package produces — so the PACM
  contract-content audit trail and the signature/compliance viewer (SM-26)
  consume goRDFlib's results unchanged. Rule IDs are built from
  `ResultPath`'s local name (a real predicate IRI whenever the violation is
  a property constraint) plus the constraint component's local name (e.g.
  `title-MinCountConstraintComponent`), not `SourceShape` — inline
  `sh:property [...]` shapes are anonymous blank nodes, not a stable
  cross-run identifier. `PolicyFinding.ShapesVersion` (new field) records
  which hub SHACL version produced the finding.
- SHACL itself only reports non-conformance (`sh:Violation`/`sh:Warning`/
  `sh:Info` results) — there is no "this property conforms" result to
  synthesize, unlike the deleted subset matcher's noisier per-property
  "X conforms" info entries. A fully compliant document now produces **zero**
  SHACL findings, not N info findings. This is more correct (matches real
  SHACL semantics) and is called out explicitly in the BDD scenarios that
  read findings.
- The Semantic Hub's canonical shapes
  (`backend/internal/semantichub/assets/facis-dcs-shapes.ttl`) were
  rewritten as valid Turtle
  while doing this: `sh:path did` (invalid — no legal prefixed name) became
  `sh:nodeKind sh:IRI` on the shape itself (SHACL's real equivalent of
  "requires a stable @id" — a JSON-LD document's `@id` is its RDF subject;
  `sh:nodeKind sh:IRI` checks that subject isn't a blank node), and
  `sh:path dcs:metadata.dcs:title` (not a real SHACL path — SHACL has no
  dot-separated compound path syntax) became a proper `sh:node`-linked
  nested shape (`dcs:CanonicalContractShape` → `sh:property [sh:path
  dcs:metadata; sh:node dcs:ContractMetadataShape]` →
  `dcs:ContractMetadataShape` constrains `dcs:title`). Behaviorally
  equivalent to the old rule's intent; the goRDFlib gate (above) is what
  surfaced that the old TTL was never valid Turtle in the first place — the
  hand-rolled matcher just didn't care.

## Fallback (not exercised)

Since the verification gate passed, the pySHACL-sidecar escape hatch
(mirroring the DSS subchart pattern, commit 43469128) was not built —
building it speculatively against a passing gate would itself be the kind
of unnecessary dual-path scaffolding this system's greenfield status
argues against. If a future goRDFlib version bump fails the W3C gate, that
is the trigger to build it, not before.

## Consequences

- Real `sh:datatype`, `sh:minInclusive`/`sh:maxInclusive`, `sh:pattern`,
  `sh:minCount`/`sh:maxCount`, `sh:node`, `sh:nodeKind`, and SHACL-1.2
  node-expression/SPARQL-based constraints are all enforceable now — the
  subset matcher's ceiling no longer bounds what the Semantic Hub can
  express (DCS-FR-TR-03).
- No dual-engine flag: this is a greenfield system, so the old matcher was
  deleted outright rather than kept behind `SEMANTIC_ENGINE=gordflib|subset`
  for regression comparison — there is no deployed data or behavior to
  regress against.
