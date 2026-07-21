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

**6. External anchoring via ORCE — pull wired, sink still TODO.** The ORCE flow
`deployment/helm/charts/orce/flows/audit-checkpoint-anchor-flow.json` polls
`GET /pac/audit/checkpoint/head` every 15 minutes and skips a `seq` it has
already seen. It currently drops each new head on a **debug node** carrying an
explicit TODO: a debug node anchors nothing, and evidence that never leaves the
operator's reach proves nothing against the operator. The sink — a notary, a
chain, a counterparty, anything third-party and append-only — is the deliberate
next step; the flow exists so that step is a rewire, not a rebuild.

Because every root chains to its predecessor, **one published head transitively
commits to the entire log before it**, so polling every few minutes is
sufficient — roughly a hundred writes a day rather than one per second. This is
the step that turns "tamper-evident to us" into "provable against the
operator", and it is the same argument a blockchain anchor makes; the sink is
an implementation detail.

A third party verifies with: the entry bytes it was given (nonce included), the
inclusion proof, and a head obtained **from the anchor, not from us**.

**7. Machine callers are SRS System Users authenticated by client credentials.**
ORCE has no wallet and no browser, so it cannot obtain a token the way a person
does — the OID4VP ceremony (an SD-JWT VC carrying the roles, wallet-signed,
bound to a Hydra login challenge, exchanged at the callback) is what fills a
human token's `ext.roles` and `ext.iss`. It therefore authenticates against
this deployment's Hydra with its own `client_id`/`client_secret`
(`client_credentials` grant) and calls DCS with that access token.

Such a token proves exactly one thing: Hydra saw this client's secret. It
carries no verifiable role claims and must not be trusted to assert any. So
`middleware.HydraJWTValidator` resolves a system caller's authority from
deployment configuration — `systemClients` in the Helm values, reaching the
backend as `DCS_SYSTEM_CLIENTS` — mapping `client_id` to a participant DID for
audit attribution and to a fixed set of DCS roles (SRS §2.4 Table 5, System
User classes). Unconfigured clients are not system users, and an empty
configuration accepts none at all. Roles are validated against
`userrole.IsValid` at startup, so a typo fails the boot rather than silently
granting nothing.

This replaces the `alg: none` token minted by the older DCS flows. That was
never a credential; it worked only because nothing verified it.

**8. A new System User class: System Auditor (`Sys. Auditor`).** SRS §2.4
Table 5 lists six System User classes, and all of them act on contracts —
System Contract Creator, Reviewer, Approver, Manager, Signer, and Contract
Target System. None describes a machine that only *reads integrity evidence*,
which is exactly what an external notary is: it must never create, approve,
sign or receive a contract, and it never needs to see contract content.

Assigning the human `Auditor` role to a machine would have worked and would
have been wrong twice over. It would grant an unattended client the full
auditor surface — every audit trail, every contract's history — to do a job
that needs one endpoint. And it would blur the SRS's own division between
Human Users, who authenticate with verifiable credentials, and System Users,
who do not; `userrole.IsHumanRole` would then answer yes for something with no
human behind it.

So Table 5 gains a seventh class:

| Role | Description |
|---|---|
| System Auditor | External integrity notary. Reads the audit trail's tamper-evidence surface — checkpoint heads — to anchor it outside the operator. No access to contract content, no write path anywhere in the system. |

It is a system role (`userrole.SystemAuditor`, `IsSystemRole() == true`), it is
distinct from the human `Auditor`, and holding one never implies the other.
Only `GET /pac/audit/checkpoint/head` accepts it; the inclusion-proof endpoint
stays human-Auditor-only, since building a proof requires naming an entry and
that is an auditor's question, not a notary's.

**One OAuth2 client per System User class, never one shared machine identity.**
Each ORCE flow authenticates as the capability it exercises
(`dcs-orce-notary`, `dcs-orce-creator`, `dcs-orce-manager`,
`dcs-orce-signer`), so a compromised or misbehaving flow reaches only what its
class may reach, and the audit trail attributes actions to the capability
rather than to "the integration". Every client is attributed to this operator's
own DID, which is what these flows are: part of this deployment, acting on its
behalf.

This is an addition to the SRS role model, not a reading of it. It belongs in
the SRS proper the next time that document is revised.

## Consequences

- Anchoring throughput is no longer bounded by `N × (TSA + IPFS round-trip)`.
- A poison event delays only itself. `outbox_events` counts the failed
  anchoring attempts and keeps the last error; after
  `conf.OutboxAnchorMaxAttempts` (50 — generous, because nearly all anchoring
  failures are transient) the event is dead-lettered, the anchoring loop stops
  selecting it, and it is logged as **not in the audit trail**. Dead-lettered
  events are found with `SELECT ... WHERE dead_lettered_at IS NOT NULL` and
  need an operator: they are gaps in the trail, and the checkpoint chain does
  not hide them, it simply never covered them.
- Published heads leak batch size and cadence, i.e. activity volume. Accepted;
  padding would blunt it if it ever matters.
- The audit trail's authority still rests on IPFS content addressing; the
  Postgres tables (`audit_checkpoints`, `audit_checkpoint_leaves`) are an index
  over it, holding the head, the walk order and the pending timestamps.
- Machine callers gain a real identity, but a coarse one: everything a system
  client may do is granted at deployment time and cannot be narrowed per
  request. That is the trade for having no credential ceremony; keep the role
  set minimal per client.
- Removed with this change: the per-entry global link
  (`GlobalLogPredCID`, exposed as `global_log_pred_cid` on four API types),
  the `SignedAuditLogEntry` wrapper whose per-entry TSA signature is subsumed
  by the checkpoint timestamp, and `conf.GlobalAuditTrailName`.
