# ADR-0005: Single-Writer-per-Aggregate for DCS-to-DCS Synchronization

**Status:** Accepted (derived from existing code)
**Affects:** `backend/internal/dcstodcs/`, `backend/internal/contractworkflowengine/remotesync/`

## Context

A contract can have multiple responsible parties (reviewer, approver, negotiator) registered with different DCS operators. Every involved instance must be able to read the contract and, within its own remit, trigger actions on it (e.g. an approver at peer B approves a contract created by peer A). Without coordination, this is a classic multi-writer problem: two instances could concurrently make contradictory changes to the same contract resource. A distributed consensus algorithm (Raft/Paxos) across organizational boundaries would be disproportionately complex for this use case.

## Decision

Every contract carries an **`Origin` field** — the peer DID of the DCS instance where it was created. That instance is the sole write master **for exactly this contract** (not globally — a node is master for its own contracts and slave for others).

- Every state-mutating command handler first checks whether the local node is the origin. If not, the entire command is forwarded unchanged via a signed RPC (`Action` endpoint) to the origin peer and executed there with the exact same handler code as a local call — there is no separate remote execution path.
- The origin validates authorization (peer-scoped task ownership, see architecture document section 10) and the timestamp (see [ADR-0007](0007-optimistic-concurrency-timestamp.md)) and performs the mutation.
- After a successful mutation, the origin broadcasts the **entire new state** (contract + all tasks of all peers) asynchronously via `PostSync` to all responsible peers — triggered through the local event bus, not as a synchronous reply to the original RPC call.
- Failed syncs are held in a retry queue (`sync_fails`) and retried by a periodic scheduler.

## Alternatives Considered

- **Distributed consensus (Raft/Paxos) between DCS instances:** rejected due to complexity and because the peers are independent organizations running their own operations (no shared cluster management possible or desired).
- **CRDTs for conflict-free merges:** rejected; the existing contract fields (free text, status enums) offer no practical merge operator without domain-specific special-casing.
- **Optimistic multi-writer with after-the-fact conflict resolution:** rejected in favor of single-writer, because contracts represent a legally binding, sequential approval process where after-the-fact resolution of contradictory writes cannot be cleanly defined.

## Consequences

- **Positive:** No conflict-resolution mechanism is needed — at any point there is exactly one authoritative source for a contract's state.
- **Positive:** Reusing the identical handler code for local and forwarded calls reduces duplication and the risk of divergence between execution paths.
- **Negative:** The origin peer is an availability single point for all state-mutating actions on "its" contracts — if it's unreachable, other peers can only read, not write.
- **Negative:** There is no ownership transfer (the `Origin` field is never modified anywhere in the code) — if an origin instance fails permanently, there is currently no provided path to transfer write authority to another peer.
- **Negative:** There is an eventual-consistency window between the mutation at the master and the slaves receiving the `PostSync` broadcast; a slave acting on a stale state in the meantime is rejected via the timestamp check and must re-sync.
