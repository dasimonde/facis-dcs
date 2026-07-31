# ADR-0002: Goa Design-First API Development

**Status:** Accepted (derived from existing code)
**Affects:** `backend/design/`, `backend/gen/`

## Context

DCS exposes a substantial number of HTTP endpoints across multiple domains (template repository, contract workflow engine, signing, DCS-to-DCS, auth, ...). These endpoints need to be:

- consistently typed (request/response validation, required fields),
- automatically documented (Swagger/OpenAPI),
- structurally uniform across domains so that frontend and peer integrations stay predictable.

Hand-written HTTP handlers with manual (de)serialization would only reach this level of consistency with significant manual maintenance effort.

## Decision

The API is specified **design-first** using [Goa v3](https://goa.design): each domain has exactly one DSL file under `backend/design/` (e.g. `template_repository.go`, `contract_workflow_engine.go`) that declaratively describes types, endpoints, required fields, and error cases. `goa gen` generates the entire transport layer from this DSL under `backend/gen/` (HTTP encoding/decoding, client/server stubs, Swagger).

`internal/service/*.go` implements the Goa-generated service interface and is the **only** place where generated code meets hand-written domain logic (command/query handlers).

## Consequences

- **Positive:** Request validation (required fields such as `updated_at`, see [ADR-0007](0007-optimistic-concurrency-timestamp.md)) is declared in the design and doesn't need to be re-checked in every handler.
- **Positive:** Swagger documentation and client stubs (including for DCS-to-DCS communication, see [ADR-0005](0005-single-writer-peer-sync.md)) stay current because they're generated from the same source as the server.
- **Negative:** Every design change requires an explicit regeneration step (`goa gen digital-contracting-service/design`) before the next build — a forgotten regeneration leads to compile errors between `design/` and `internal/service/`.
- **Negative:** `gen/` must never be edited manually — this needs explicit communication during team onboarding and in `README.md`, since IDE autocompletion can otherwise accidentally modify generated code.
