# ADR-7: Contract hierarchy links point child → parent only, never the reverse

## Context

DCS's frame-agreement architecture requires that an overarching DCS
instance can push a frame agreement to counterparty instances, each of
which holds its own child contract, without those counterparties learning
about each other. If a child document (or the frame document itself)
carried an enumerable list of sibling children, any party who could read
one child's document could discover every sibling — breaking the isolation
the whole architecture exists to provide. The link already existed in the
data model (`dcs:parentContract`,
`frontend/ClientApp/src/models/dcs-jsonld.ts`); what was missing was making
the *direction* an enforced invariant rather than a convention someone
could accidentally violate by adding a `children` field.

## Decision

- A contract document may carry `dcs:parentContract` (child → parent,
  singular) but **never** a child-enumerating property. Both
  `dcs:childContracts` and the ODRL/SHACL-adjacent term `hasPart` are
  explicitly rejected by validation
  (`backend/internal/base/validation/documentdata.go`, tested in
  `hierarchy_test.go`).
- More than one `dcs:parentContract` reference on a single document is
  also rejected — a contract has at most one parent.
- The overarching instance gets full scope not by any child enumerating
  its siblings, but because *every* child it can see already references
  the frame document by DID — scope is a property of what the overarching
  instance queries for (`parent_did` search filter,
  DCS-FR-CWE-29), not something embedded in the documents themselves.
- Bundle export (FR-CWE-30/FR-TR-24) walks the parent chain **upward**
  when assembling a ZIP: a child's bundle includes its own artifacts plus
  every ancestor up to the frame under `parents/`. Other family members —
  descendants of the topmost locally-known ancestor, e.g. siblings — are
  included under `related/` only when they are **locally known and readable
  by the requester** under the same party read-scoping as a direct
  retrieve; anything else is silently absent. Documents still never
  enumerate children — the family is resolved by querying local children,
  never from a field on any document. Export is refused with a findings
  list, not partial output, when a referenced component is missing
  (FR-TR-26/FR-PACM-06).

## Consequences

- Sibling isolation is a structural guarantee (a document a peer can never
  legally hold cannot leak sibling identities), not a discipline convention
  that a future feature could accidentally violate.
- The bundle's `related/` directory does not weaken this: it only packages
  contracts the requester could already read individually on this instance,
  so it reveals nothing beyond what direct retrieval already grants. A
  counterparty instance never holds the siblings in the first place; on the
  originator instance the initiator is party to them anyway.
- The invariant is grep-checkable: `git grep -n "childContracts\|hasPart"
  backend frontend` must return zero *document-model* hits (ODRL's own
  `odrl:hasPart` operator is a separate, allowed vocabulary term and is
  excluded from this check by context, not by name).
