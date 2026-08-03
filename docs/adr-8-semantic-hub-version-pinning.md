# ADR-8: Semantic Hub version pinning for enforcement

## Context

The Semantic Hub (`backend/internal/semantichub`, DCS-FR-TR-03, UC-02-08) has
stored versioned JSON-LD contexts, SHACL shapes, and validation profiles
since it was introduced, and produced documents have always recorded which
hub version they were anchored to (`dcs:schemaRefs`). But nothing on the
enforcement side ever consulted the hub: `AuditContractContent`
(`backend/internal/base/validation/contractcontentaudit.go`) loaded its
canonical SHACL shapes and validation profile straight off disk
(`docs/semantic-ontology/...`) on every call. Registering, activating, or
rolling back a hub schema version changed nothing about what was actually
enforced — the hub was a write-only ledger.

## Decision

- A new `ShapeSource` interface
  (`backend/internal/base/validation/shapesource.go`) is the enforcement-time
  source for the canonical SHACL shapes and validation profile.
  `HubShapeSource` (`backend/internal/semantichub/shapesource.go`) is the
  only implementation, backed onto `semantichub.Repo`. This is a greenfield
  system with no deployed instance to keep a disk-file enforcement fallback
  for: `docs/semantic-ontology/...` exists solely to seed the hub at startup
  (`semantichub.Seed`, `go:embed`). If the hub is unreachable or
  unconfigured, `AuditContractContent` hard-fails
  (`requireShapeSource`/`SetShapeSource`) rather than silently falling back
  to anything — there is no `SEMANTIC_ENFORCEMENT_SOURCE` flag.
- **Anchor vocabulary — standard linked-data terms only**: a produced
  document carries its hub anchors in vocabulary external JSON-LD tooling
  understands outright, not a proprietary `dcs:schemaRefs` object:
  - `"@context"` **is** the context anchor: the hub-served, versioned
    context URL itself (`/semantic/context/{name}?version=N`), kept in
    standard array form alongside any client-supplied inline prefix map
    (`normalizeCanonicalContext`). Dereferencing it yields the exact
    registered JSON-LD context document.
  - `"sh:shapesGraph"` (`http://www.w3.org/ns/shacl#shapesGraph` — SHACL's
    own data-graph→shapes-graph link) carries the versioned shapes URL
    (`/semantic/shapes/{name}?version=N`) and is the **pin carrier** for
    revalidation.
  - `"dcterms:conformsTo"` (Dublin Core, as used by DCAT/PROF) names the
    versioned validation profile URL
    (`/semantic/profile/{name}?version=N`).
  - The former `dcs:ontology` ref is dropped: ontology terms are
    follow-your-nose dereferenceable from the context's term IRIs.
  All three anchor paths are public, unauthenticated hub routes
  (`resolve_context`/`resolve_shapes`/`resolve_profile`), so an external
  verifier can resolve everything a document claims without a DCS login.
- **Version pinning**: a document is validated against the hub SHACL shapes
  version that was **active at the document's own creation time**. Anchors
  are written once, at production time, never re-normalized — this ADR
  makes it load-bearing: `AuditContractContent` parses the pinned version
  out of the document's own `sh:shapesGraph` anchor
  (`pinnedHubShapesVersion`) and, when present, revalidates against that
  exact version via `ShapeSource.ShapesAt`, not whatever is active now.
  JSON-LD expansion during validation likewise resolves the context version
  pinned in the document's own `@context` URL (`ShapeSource.ContextAt`),
  hermetically (no network fetch). New documents (no existing pin) validate
  against whatever is currently active. Hub versions are immutable (no
  deletion), so a pinned version is always resolvable — an already-produced
  artifact never silently starts failing (or silently starts passing)
  because someone changed the hub later.
- Fixed a latent bug this pinning depends on: `cmd/dcs/main.go` was anchoring
  the SHACL shapes ref to the **context's** active version
  (`hubContextVersion`) rather than the shapes' own — harmless while both
  always moved in lockstep at v1, but wrong the moment either is
  registered/rolled back independently. Now sourced via
  `semantichub.ActiveVersion(ctx, db, ShapesName, "shapes")`.
- Existence of the pinned version is verified by construction at write time
  (the anchored version is read straight from the hub via `Repo.Get` moments
  before `SetSchemaAnchorRefs` installs it) and, where it actually matters —
  revalidation — by the natural error path: `ShapesAt` on a version the hub
  doesn't hold returns `semantichub.ErrSchemaNotFound`, which
  `AuditContractContent` propagates as a hard failure rather than silently
  falling back to the active version.
- `backend/internal/semantichub/assets/facis-dcs-shapes.ttl` (the embedded
  seed, now the single authoring source) had drifted from the then-separate
  docs copy — a different,
  never-actually-loaded shape set — and also was not valid Turtle
  (`sh:path did` is not a legal prefixed name; ADR-9 fixed it while
  replacing the enforcement engine, since the old hand-rolled matcher never
  actually parsed Turtle grammar). Fixed by making the asset a verbatim copy
  of the corrected authoring file again, restoring the invariant the package
  doc for `semantichub` already claimed ("assets/ copies of
  docs/semantic-ontology, the authoring source").
- Enforcement stays opt-in per audit call: the canonical shapes/profile
  apply only when the policy document being audited sets
  `enforceCanonicalShapes`/`enforceValidationProfile`
  (the default disk policy document, `docs/policies/
  facis-contract-content-audit-policies.json`, sets both — used whenever no
  explicit policy document is supplied, e.g. the PACM contract-content audit
  trail walk). Ad-hoc/test policies that want to exercise only ODRL
  evaluation leave both unset.
- **Consequential workflow gates use one central coordinator.** Submission,
  offer, approval, signature and deployment all pass an immutable contract
  snapshot through the same PAC workflow-gate coordinator before their
  transition handler runs
  (`backend/internal/service/contract_workflow_engine.go:114-140`,
  `backend/internal/service/contract_workflow_engine.go:410-430`,
  `backend/internal/service/contract_workflow_engine.go:1212-1220`,
  `backend/internal/service/contract_workflow_engine.go:1467-1475`,
  `backend/internal/service/signature_management.go:457-462`,
  `backend/internal/contractworkflowengine/deployevent/subscriber.go:62-74`).
  The snapshot identity covers the contract state, version, content hash,
  complete effective shape set and validation-profile version
  (`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:336-375`,
  `backend/internal/processauditandcompliance/workflowgate/workflowgate.go:418-426`).
  The coordinator resolves and evaluates this pinned bundle locally before one
  correlated dispatch to the configured executor. This preserves ADR-6 and
  ADR-11's server-side ODRL enforcement and ADR-9's SHACL result mapping in one
  local evidence set
  (`backend/internal/base/validation/contractcontentaudit.go:59-123`).
  Missing, unknown or
  unavailable bundle members fail closed without an executor fallback
  (`backend/internal/base/validation/shapesource.go:46-82`,
  `backend/internal/semantichub/shapesource.go:53-80`,
  `backend/internal/processauditandcompliance/workflowgate/workflowgate.go:234-282`).
- **Decision precedence is `BLOCKED > REVIEW > PASSED`.** A blocking local
  finding prevents dispatch; a failed executor result or finding blocks the
  transition; any review result holds it for a Compliance Officer; only the
  remaining case passes
  (`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:304-333`).
  The database admits one run per immutable snapshot and gate, which arbitrates
  concurrent callers and prevents a second executor dispatch
  (`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:429-456`,
  `backend/migrations/sql/20260729_external_checkpoint_workflow_gates.sql:1-23`).
- A manual review decision is persistent and immutable per run, includes actor
  and justification, and an approval resumes the stored transition directly;
  it does not call the workflow-gate executor again
  (`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:534-634`,
  `backend/migrations/sql/20260729_external_checkpoint_workflow_gates.sql:28-51`).
  A separate recovery/lease mechanism for a continuation attempt left in
  `DISPATCHING` by a hard process termination is intentionally deferred to a
  future ticket; the current implementation refuses a second continuation
  while that durable state exists
  (`backend/internal/processauditandcompliance/workflowgate/workflowgate.go:581-599`).

## Consequences

- Registering a stricter SHACL shapes version and activating it changes
  findings for contracts created afterward; contracts already produced
  under the previous version keep revalidating exactly as before. Rolling
  back restores the previous enforcement behavior for documents created
  after the rollback. This closes the loop UC-02-08 (register/rollback)
  promised but had no consumer for
  (`features/23_semantic_hub/semantic_hub.feature`, "Activating a stricter
  SHACL shapes version...").
- The enforcement engine is goRDFlib, a conformant SHACL-core processor
  (ADR-9) — replacing the hand-rolled structural-subset matcher this
  package used to carry (`parseContractSHACLShapesTTL` and friends, deleted
  outright: this is a greenfield system with no deployed rows depending on
  the old behavior, so there is no dual-engine flag to keep it alive as a
  fallback).
- The same active shapes, independently versioned shape libraries and active
  profile are pinned when a new contract artifact is produced. Activation and
  rollback therefore affect only snapshots created afterward; already persisted
  workflow runs keep resolving their original bundle
  (`backend/internal/base/validation/documentdata.go:46-78`,
  `backend/internal/semantichub/shapesource.go:64-80`).
