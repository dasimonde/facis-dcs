# ADR-14: One JSON-LD form internally — expanded, expand-at-edges

Status: Accepted (2026-07-19). Implementation: pending — team task for the week.

## Context

Two JSON-LD contexts describe the same FACIS DCS ontology namespace
(`https://w3id.org/facis/dcs/ontology/v1#`):

- The DCS uses `internal/semantichub/assets/facis-dcs-context.jsonld` —
  `dcs:`-prefixed terms plus aliases (`derivedFromTemplate`, `parameterValue`,
  …). This form is pervasive: ~234 Go `dcs:` string sites, ~441 frontend sites,
  and the SQL JSON paths (`contract_data->'dcs:parentContract'`,
  `dcs:metadata->>'dcs:title'`).
- pdf-core canonicalizes against its LinkML-generated context (`@vocab` → bare
  terms: `parentContract`, `metadata`, `policies`) and **embeds that** in the
  contract PDF (the authoritative artifact that crosses the DCS-to-DCS boundary,
  ADR-13).

A peer that rebuilds its copy from an extracted payload therefore receives the
bare-term form while the originator holds the `dcs:` form — the two copies
diverge and every `dcs:…` JSON path breaks. The current mitigation is a
**throwaway re-compaction bridge** (`internal/base/jsonld/CompactToFacis`,
called from `contractworkflowengine/command/receivepdf.go`): the receiver
re-compacts the extracted payload back to the `dcs:` form offline. It works but
it is exactly the expand/compact dance we do not want to keep.

## Decision

Work with **EXPANDED JSON-LD internally throughout** (full IRIs). At ingestion
**edges** — contract creation, peer PDF receipt, any external payload — run a
single json-gold **Expand** (piprate/json-gold) that both normalizes to the
internal form and asserts the payload is semantically valid JSON-LD. No
compaction dance internally; you only expand, at the edge.

DCS and pdf-core keep **separate ontologies** (pdf-core = document rendering,
its LinkML pipeline; DCS = domain: placeholders, requirements, parties,
policies). They are not merged; the edge Expand is what bridges them.

## Consequences / migration (the actual work)

- SQL JSON paths move from `dcs:X` to the full IRI (`…/v1#X`).
- Go reads of `contract_data`/`template_data` (`doc["dcs:X"]`, the ~234 sites)
  move to full IRIs.
- The frontend either consumes the expanded form or the API compacts once at the
  presentation edge (decide during implementation — presentation-edge compaction
  is acceptable; ingestion-edge is always Expand).
- Delete the interim bridge: `internal/base/jsonld/CompactToFacis`,
  `facis-context.jsonld`, `pdfcore-context.jsonld`, and the receiver call in
  `receivepdf.go`.
- Greenfield: no dual-path/compat — flip the whole app to expanded in one wave
  (the DB may be wiped).

See ADR-13 (PDF-exchange federation) for why the PDF is the boundary artifact,
and `receivepdf.go` for the bridge to remove.
