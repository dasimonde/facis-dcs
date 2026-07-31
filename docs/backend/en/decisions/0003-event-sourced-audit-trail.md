# ADR-0003: Transactional Outbox with an IPFS/TSA-Anchored Audit Trail

**Status:** Accepted (derived from existing code)
**Affects:** `backend/internal/base/event/`, `backend/internal/processauditandcompliance/`

## Context

DCS manages legally binding contracts in a Gaia-X context. It must be **provably** verifiable afterwards that an audit log has not been tampered with — a plain database table of log entries could be altered after the fact by a DB admin or a successful attack without detection. At the same time, writing an audit entry must not block the same transaction that performs the business change (availability/latency), and no event may be lost even if IPFS or the TSA service is briefly unreachable.

## Decision

1. Every command/query handler writes its domain event **into the same database transaction** as the business change, into an `outbox_events` table (transactional outbox pattern — no event is lost, no inconsistency between business data and event).
2. A background process (`OutboxProcessor`) processes unprocessed events via polling (`FOR UPDATE SKIP LOCKED`), asynchronously and decoupled from the originating request.
3. For each entry, a **hash chain** is formed from the previous CID (both resource-specific and global), the entry is signed by a **Time-Stamping Authority (TSA)**, the signature is verified immediately (sanity check), and the result is written to **IPFS** (immutable, content-addressed storage).
4. The event is additionally published on **NATS** (CloudEvents format) to asynchronously notify other domains (PDF provenance, webhook platform, DCS-to-DCS sync).

## Alternatives Considered

- **Pure event sourcing as the primary persistence mechanism** (state reconstructed exclusively from events): rejected in favor of classic state tables plus an event log as a byproduct — lower migration effort for a team that primarily thinks relationally, and simpler read models for reporting/search.
- **Synchronous write to IPFS/TSA within the same request:** rejected due to latency and availability risk (an IPFS/TSA outage would directly block every user action).
- **Plain DB table without external anchoring:** rejected as insufficiently tamper-evident for the regulatory requirement (provability towards third parties/auditors).

## Consequences

- **Positive:** The business transaction and the audit anchor are decoupled — an IPFS outage doesn't block user actions, it only delays anchoring.
- **Positive:** Retroactive tampering with an audit entry breaks the hash chain and is therefore detectable.
- **Negative:** There is a window between the business change and completed anchoring (eventual consistency) — during this window an event is persistent (in the outbox) but not yet externally anchored.
- **Negative:** The `OutboxProcessor` processes sequentially per run (max. 100 events per batch) — under very high event volume, anchoring can lag behind.
