# ADR-0001: Domain-Separated CQRS in a Modular Monolith

**Status:** Accepted (derived from existing code)
**Affects:** `backend/internal/<domain>/{command,query}`

## Context

DCS covers several clearly separable business areas (template management, contract workflow, signing, federated-catalogue integration, audit). These areas have different read/write load profiles, different consistency requirements, and should evolve independently without unintended cross-domain side effects.

At the same time, the system needs to be operated as a single deployable Go process (one binary, one database) — a move to microservices was not justified at decision time (operational overhead, and distributed transactions across domains would have been required, e.g. contract ↔ template).

## Decision

Every business domain (`templaterepository`, `contractworkflowengine`, `signingmanagement`, `templatecatalogueintegration`, ...) gets its own package under `internal/`, with a strict internal split into:

- `command/` — one file per write use case (`<Verb>Cmd` struct + `<Verb>er` handler)
- `query/` — one file per read use case
- `db/` — repository interfaces, with `db/pg/` as the Postgres implementation
- `datatype/`, `event/` — domain-owned enums and events

Coupling between domains happens exclusively through explicit imports (rare, e.g. `processauditandcompliance` reads `templaterepository` repositories for audit purposes) or through the internal event bus (NATS) — never through direct access to another domain's tables.

Not every domain implements the full pattern: `pdfgeneration` and `processauditandcompliance` deliberately have no `command/` folder because they are purely event-driven or purely read-only. Infrastructure packages (`base`, `middleware`) and the adapter layer (`service/`) don't follow the pattern at all, because they own no business aggregate.

## Alternatives Considered

- **Microservices per domain:** rejected due to operational overhead and because several domains (contract ↔ template, contract ↔ signing) are transactionally tightly coupled.
- **A single "service" layer without CQRS separation (classic layered CRUD):** rejected because read and write paths differ substantially in complexity (e.g. `GetAllMetadataHandler` aggregates review/approval tasks, while `Create` writes a single record), and separate handlers markedly improve testability and traceability per use case.

## Consequences

- **Positive:** High consistency throughout the codebase — every new use case follows a known pattern (BeginTx → validation → mutation → event → commit), speeding up onboarding and reviews.
- **Positive:** Business changes stay local to one domain.
- **Negative:** Handler-level dependency injection produces many small, very similar struct definitions (boilerplate) — a deliberate trade-off for explicitness over a "god service" holding every repository.
- **Negative:** Without technical process separation there is no hard isolation level between domains (a bug in one domain could in theory crash the entire process) — mitigated by idiomatic Go error handling (`error` return values instead of panics) in the handlers.
