# DCS Backend Architecture Documentation

Technical architecture documentation for the `backend/` component of the Digital Contracting Service (DCS), written for both the client and the engineering team.

- 🇬🇧 **English:** [en/architecture.md](en/architecture.md) · [Architecture Decision Records](en/decisions/)

## Contents

1. Purpose, technology stack, high-level architecture (C4-style component diagram)
2. Folder structure of `backend/` and the recurring domain skeleton under `internal/`
3. Domain overview: which domains implement CQRS, which are query-only, and which are pure infrastructure/orchestration
4. The CQRS command/query handler pattern in detail
5. Event-sourced, tamper-evident audit trail (transactional outbox → TSA → IPFS → NATS)
6. Contract Workflow Engine state machine and negotiation merging
7. Template Repository state machine and copy-on-version versioning
8. Comparison of template vs. contract versioning strategies
9. DCS-to-DCS federation: trust model (`did:web` + eIDAS) and single-writer-per-aggregate synchronization

## Architecture Decision Records

| # | Title |
|---|---|
| 0001 | Domain-separated CQRS in a modular monolith |
| 0002 | Goa design-first API development |
| 0003 | Transactional outbox with an IPFS/TSA-anchored audit trail |
| 0004 | did:web + eIDAS certificates as peer trust anchor |
| 0005 | Single-writer-per-aggregate for DCS-to-DCS synchronization |
| 0006 | Diverging versioning strategies for templates and contracts |
| 0007 | Optimistic concurrency control via client-supplied timestamp |

> These documents describe the architecture as it exists in the codebase today; they are not a specification of intended future behavior. Update them alongside significant architectural changes.
