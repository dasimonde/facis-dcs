# ADR-0007: Optimistic Concurrency Control via Client-Supplied Timestamp

**Status:** Accepted (derived from existing code)
**Affects:** `backend/internal/contractworkflowengine/command/`, `backend/design/contract_workflow_engine.go`

## Context

Since contracts can be read and (after forwarding to the origin, see [ADR-0005](0005-single-writer-peer-sync.md)) modified across multiple peers, it must be prevented that a client acts on a stale, locally cached state and unknowingly overwrites a concurrent change made by another actor. A classic DB lock held for the duration of a user interaction is impractical (requests arrive asynchronously, sometimes across peer boundaries with network latency).

## Decision

Every state-mutating API request for contracts must include `updated_at` as a **required** field (declared as `Required` in the Goa design). The command handler compares this value against the resource's server-side stored `UpdatedAt`:

```go
if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
    if localPeer != cmd.CauserDID {
        return errors.New("contract was updated elsewhere, please force synchronisation and reload")
    }
    return errors.New("contract was updated elsewhere, please reload")
}
```

If the supplied timestamp is older than the currently stored one, the change is rejected (compare-and-reject, no lock). Two distinct error messages distinguish whether the stale state originated locally (a simple reload suffices) or from a remote peer (a full re-sync is required).

## Alternatives Considered

- **Pessimistic locking (a DB row lock held for the duration of the user interaction):** rejected, since requests arrive asynchronously and with peer network latency — a lock spanning that time would be impractical and would block other actions' availability.
- **A monotonically increasing version counter instead of a timestamp (the classic ETag/`If-Match` pattern):** functionally equivalent, but not chosen — likely because `updated_at` was already present as a display field on the client and could be reused directly without introducing an additional version field.
- **Last-write-wins with no check at all:** rejected, since an unnoticed overwrite is not tolerable for legally binding contract changes.

## Consequences

- **Positive:** Simple to implement and easy for clients to reason about (the timestamp is already part of the displayed resource anyway).
- **Positive:** Distinguishing "stale locally" from "stale via a peer" gives the client a clear signal for the required recovery action (reload vs. force-sync).
- **Negative:** The comparison uses `.Unix()` (second resolution) rather than nanoseconds — two writes within the same second are not detected as conflicting. Rarely relevant in practice given inter-peer network latency, but a known precision gap.
- **Negative:** The pattern is duplicated individually in every affected handler (no central concurrency helper) — changes to the error message or logic must be applied consistently in multiple places. Coverage is also not fully uniform across all actions: `AcceptNegotiation`/`RejectNegotiation` use the same origin-forwarding mechanism as other actions but, unlike Approve/Submit/Reject/Terminate/Negotiate/RecordEvidence, do not require `updated_at` and therefore skip this check entirely.
