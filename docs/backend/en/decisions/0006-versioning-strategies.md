# ADR-0006: Diverging Versioning Strategies for Templates and Contracts

**Status:** Accepted (derived from existing code)
**Affects:** `backend/internal/templaterepository/db/pg/`, `backend/internal/contractworkflowengine/db/pg/`

## Context

Both templates and contracts go through multiple versions over time, but have very different business lifecycles:

- **Templates** are catalogue artifacts: multiple versions can be in circulation/registered in parallel (an already-published template may still be referenced while the next version is being worked on), and other contracts may reference a specific historical version.
- **Contracts** are a single, ongoing process per contractual relationship — at any point there is only "the one" current version; earlier versions are pure history with no independent external referenceability.

## Decision

Two different versioning models were deliberately chosen for the two domains:

**Templates — Copy-on-Version:** every version is its own database row with its own DID. The `Copy` command creates, depending on the source's state, either a new independent lineage (source not yet registered/published → `version = 1`, new `base_template` key) or the next version of an existing lineage (source already registered/published → `version + 1`, inherited `base_template` key). A SQL guard prevents competing version numbers within the same lineage. "Is this the newest version?" is computed at read time via a window function, not persisted.

**Contracts — Mutable Current + Append-Only History:** there is exactly one database row per contract DID for its entire lifetime; `contract_version` is incremented within that same row. Before every version bump (triggered by the negotiation-merge logic), the current state is copied via `CreateHistoryEntryForDID` as an immutable snapshot into a separate `contract_history` table.

## Alternatives Considered

- **A unified model for both domains (e.g. contracts also as copy-on-version):** rejected, because contracts need no independently referenceable prior versions, and a new DID per contract version would have introduced unnecessary complexity into DCS-to-DCS synchronization (every version change would have meant a new peer-sync target resource).
- **Templates also as a mutable-current model:** rejected, because it would make it impossible to reference/register two versions of a template at the same time — a core requirement for the catalogue use case (the Federated Catalogue can carry multiple versions in parallel).

## Consequences

- **Positive:** Both models are optimally tailored to their respective business purpose, rather than a compromise model that would be ideal for neither case.
- **Negative:** Developers switching between the two domains have to keep two different mental models of "versioning" in mind — `ContractVersion` and template `version` behave fundamentally differently despite similar naming.
- **Negative:** There is no shared, reusable "versioning building block" (no shared repository pattern) — each domain implements its SQL logic independently, doubling maintenance effort whenever either model needs to change.
