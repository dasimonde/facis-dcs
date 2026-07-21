# ADR-16: Audit-trail tamper evidence is Merkle checkpoints, anchored externally

## Status

Accepted (2026-07-21). Refines
[ADR-3](docs/backend/en/decisions/0003-event-sourced-audit-trail.md) — the
event-sourced audit trail and its per-resource hash chains stand; what changes
is how tamper evidence is produced *across* resources, and where the evidence
is ultimately held.

## Context

Every outbox event that is audit-visible is anchored: the entry is written to
IPFS and linked to its predecessor by CID. Until now each entry carried **two**
links, one to the previous entry for the same resource and one to the previous
entry *globally*, and each entry was individually timestamped by the TSA.

Three problems followed from the global link.

1. **It serialised the whole system.** A global predecessor can only be read
   after the previous entry is written, so every audit-visible event queued
   behind one TSA round-trip plus one IPFS write, for the entire installation.
2. **One failure stalled everything.** The anchoring loop stopped at the first
   error, and a transient failure at the head of the backlog held back every
   event behind it for as long as it kept failing. This is not theoretical: a
   contract's audit trail read empty for 90 seconds in CI (run 29864154313),
   failing `Contract bundle export creates an audit log entry`.
3. **Nothing read it.** `ReadAllAuditLogEntries`, the only consumer of the
   global chain, had no callers at all.

Meanwhile the evidence itself was weaker than it looked. We hold the entries,
the chain heads and the database, so a chain we alone possess demonstrates
nothing *against the operator* — only against a third party who tampers with
storage we control.

## Decision

**1. One Merkle checkpoint per anchoring tick, instead of a global link per
event.**

A checkpoint commits to the batch with a single root over the entries in outbox
order, chains to the previous checkpoint's root, and is timestamped once. Leaf
and node hashing follow RFC 6962 domain separation. Per-resource chains
(`ResLogPredCID`) are unchanged and remain strict; entries of different
resources are written concurrently.

Per-entry evidence becomes: the leaf hash, an inclusion proof against the root,
and the timestamp over that root — the same guarantee, at one TSA round-trip
per batch rather than per event. Consecutive roots additionally prove the log is
append-only, which the per-event chain never did.

**2. Nothing is reordered.** Order within a checkpoint is the outbox sequence;
order across checkpoints is the root chain. An entry that cannot be written
drops out of this checkpoint and joins the next one, carrying its own
`created_at`. The log then states two separate facts — when the event happened,
and by when its existence was proven. That separation is honest: a timestamp
attests existence-no-later-than and never claims more.

**3. The TSA is off the critical path.** A root is immutable once computed, so a
checkpoint anchored while the TSA is unreachable is recorded with
`tsa_signature NULL` and timestamped by a later pass. A TSA outage delays
evidence; it no longer blocks the audit trail.

**4. Leaves are blinded.** Each entry carries a random `nonce` that enters its
leaf hash. Audit entries are highly guessable — component, event type, a DID, a
second-precision timestamp — so an unsalted leaf hash would be a commitment
anyone could brute-force to confirm what an entry says. Whoever is entitled to
the entry receives the nonce with it and can recompute the leaf; nobody else
can. This is what makes proofs publishable.

**5. Public head, private body.** A checkpoint splits in two:

| Published | Never published |
|---|---|
| `seq`, `root`, `prev_root`, `leaf_count`, `created_at`, RFC 3161 token | leaf CIDs (fetch capabilities into our store) |
| inclusion proofs (all hashes) | the entries themselves — `event_data` carries contract DIDs, participant/organization identities, workflow payloads |

`GET /pac/audit/checkpoint/head` returns only the head.
`GET /pac/audit/checkpoint/proof/{entry_cid}` returns a proof and the head of
the checkpoint that commits to the entry — never the entry.

**6. External anchoring via ORCE.** An ORCE flow polls the head endpoint and
stores what it retrieves outside our control. Because every root chains to its
predecessor, **one published head transitively commits to the entire log before
it**, so polling every few minutes is sufficient — roughly a hundred writes a
day rather than one per second. This is the step that turns "tamper-evident to
us" into "provable against the operator", and it is the same argument a
blockchain anchor would make; the sink is an implementation detail.

A third party verifies with: the entry bytes it was given (nonce included), the
inclusion proof, and a head obtained **from the anchor, not from us**.

## Consequences

- Anchoring throughput is no longer bounded by `N × (TSA + IPFS round-trip)`.
- A poison event delays only itself. It is retried each tick and must be
  surfaced operationally rather than retried silently forever — a dead-letter
  path is still to be built.
- Published heads leak batch size and cadence, i.e. activity volume. Accepted;
  padding would blunt it if it ever matters.
- The audit trail's authority still rests on IPFS content addressing; the
  Postgres tables (`audit_checkpoints`, `audit_checkpoint_leaves`) are an index
  over it, holding the head, the walk order and the pending timestamps.
- Removed with this change: the per-entry global link
  (`GlobalLogPredCID`, exposed as `global_log_pred_cid` on four API types),
  the `SignedAuditLogEntry` wrapper whose per-entry TSA signature is subsumed
  by the checkpoint timestamp, and `conf.GlobalAuditTrailName`.
