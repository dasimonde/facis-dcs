# ADR-13: DCS-to-DCS federation is a PDF (+ JAdES) exchange, not a state broadcast

## Status

Accepted, rolling out (2026-07-19). **Supersedes the DCS-to-DCS synchronizer**
of [ADR-2](adr-2-contract-state-machine.md) (the origin-as-single-writer /
full-state-broadcast / peer-task-sync model). Builds on
[ADR-12](adr-12-wallet-driven-signing.md) (the signed PDF + JAdES are the
signature artifacts) and [ADR-4](adr-4-c2pa-embedding.md) (the PDF carries the
C2PA provenance chain and the embedded machine-readable JSON-LD).

**Superseded in part by [ADR-19](adr-19-federation-agreement-credential.md)
(2026-07-22).** The third trust layer this ADR relied on — the static
`trusted_peers` allowlist — is **withdrawn** and removed entirely, replaced
by a self-signed agreement credential plus a local policy endpoint. The PDF
(+ JAdES) exchange model described below, and the first two trust layers
(eIDAS certificate chain, per-request challenge-response), are unchanged.

## Context

The previous federation had the **origin** peer act as the single writer for a
contract: it broadcast the full contract state — the JSON-LD `ContractData`
**plus every review, approval, and negotiation task of every responsible peer**
— to each peer via `POST /peer/contracts/`, and non-origin peers forwarded
their mutations back to the origin via `POST /peer/contracts/action`. Peers
were addressed by putting their did:web DIDs into the contract's
reviewer/approver/negotiator lists (`Responsible.GetUniqueResponsibleList`).

This couples the two organizations' internal workflows: one operator's DCS
creates and owns the other operator's tasks, and role assignment leaks peer
DIDs into what should be internal RBAC. It also re-invents, over a bespoke JSON
protocol, transport of content the **PDF already carries**: pdf-core embeds the
canonical machine-readable JSON-LD (`compiler.ExtractEmbeddedJSONLD`), records
every lifecycle step in the C2PA provenance chain, and holds the signatures
([ADR-4](adr-4-c2pa-embedding.md), [ADR-12](adr-12-wallet-driven-signing.md)).

## Decision

**Two DCS instances exchange exactly two things: the contract PDF, per
lifecycle step, and the JAdES signature artifact, after signing. Nothing
else.** Each DCS runs its **own** workflow and RBAC on its **own** copy; tasks
never cross an instance boundary.

1. **The PDF is the wire format.** A lifecycle step that the counterparty must
   see (offer, a negotiation counter-proposal, a signature) ships the current
   PDF to the counterparty. The receiver asks pdf-core to extract the embedded
   JSON-LD (`POST /manifest/…` sibling — pdf-core owns all byte-level PDF work,
   the DCS never parses PDF bytes) and upserts its local contract copy from it.
   The C2PA provenance chain in the PDF is the authenticated history; the JAdES
   envelope authenticates the sender (did:web + eIDAS, unchanged).

2. **Each DCS is the writer of its own copy.** There is no single-writer
   origin and no forwarded `action`. A party changes terms on its own copy and
   ships the resulting PDF; the counterparty receives it as a proposal.

3. **Negotiation, then settlement, then signing — three distinct phases.**
   Signing must never overlap negotiation: a one-sided signature on a contract
   that is not yet mutually settled is still a signed, and therefore
   disputable, contract. So:
   - **Negotiate.** To counter, a party edits its copy and ships a new PDF
     version (a `proposed` C2PA lifecycle step). This is the only move while
     terms are open.
   - **Settle.** To accept the current terms, a party ships the same version
     stamped as an `agreed` C2PA lifecycle step (not a counter, not a
     signature). **Settlement is reached only when both parties have agreed to
     the same version.** The PDF's provenance carries who agreed to what, so
     settlement is provable from the artifact.
   - **Sign.** Only after settlement does the signing phase open. Both parties
     sign the settled version and ship the signed PDF + JAdES; two signatures
     over the settled content is the executed agreement.
   The receiver reads the latest `dcs.lifecycle` status from the PDF's C2PA
   chain (via pdf-core) to tell a proposal from an agreement, and detects a
   signature by the PAdES/JAdES it carries.

4. **Review/approval are internal.** Between settlement and signing, each DCS
   takes its own copy through its own review/approval gates under its own RBAC.
   These transitions are local and never synced.

5. **Counterparty is a single peer, not a role list.** A contract records the
   one counterparty DCS it is offered to (a did:web), which is where its PDFs
   are shipped. Reviewer/approver/negotiator are internal user roles, isolated
   per instance, and never carry peer DIDs.

## The touch point

```
   DCS A (owns its copy, own RBAC)          │   DCS B (owns its copy, own RBAC)
 ─────────────────────────────────────────────┼──────────────────────────────────────
  offer: ship PDF(v1, proposed) ──────────────┼──▶ extract JSON-LD, create local copy
                                              │    counter: ship PDF(v2, proposed) ◀────
  extract, update copy                         │
  counter: ship PDF(v3, proposed) ────────────┼──▶ extract, update copy
                                              │    agree: ship PDF(v3, agreed)   ◀──────
  agree: ship PDF(v3, agreed)   ──────────────┼──▶ both agreed → SETTLED
   both agreed → SETTLED                       │    internal review/approve (local)
  internal review/approve (local)             │    sign v3, ship signed PDF + JAdES ◀───
  see B's signature, sign v3                  │
  ship signed PDF + JAdES        ─────────────┼──▶ two signatures over settled v3 → executed
```

## What is removed

- `POST /peer/contracts/` full-state broadcast (`ContractData` + tasks) and its
  payload types (`remotesync` task/negotiation data).
- `POST /peer/contracts/action` (origin-forwarded mutation) and the
  single-writer-origin rule.
- `GET /peer/contracts/sync` (force a refresh from the origin).
- The peer **task-sync** (`localpeerupdatecmd`/`peerupdaterequestcmd` task
  upserts) — each DCS creates its own tasks locally.
- Peer DIDs in `Responsible` reviewer/approver/negotiator lists and their use
  as the sync recipient list (`GetUniqueResponsibleList` for routing). The
  party set for signature-field seeding is `[origin, counterparty]`.

## What is kept

- did:web + eIDAS peer authentication (challenge-response over the JAdES
  envelope) — the sender of a PDF is a verified, trusted peer.
- The JAdES artifact — now the **signature** exchanged after signing (SM-02),
  not a transport envelope around a JSON broadcast.
- pdf-core as the sole owner of PDF byte work (embed, extract, deterministic
  re-render, C2PA), [ADR-4](adr-4-c2pa-embedding.md); the DCS orchestrates.

## Consequences

- Federation state is derivable from artifacts alone: a received PDF is the
  contract at that step, self-verifying via its C2PA chain and JAdES; there is
  no separate task ledger to reconcile.
- The two operators' internal processes are decoupled — one cannot create or
  observe the other's tasks. This is the correct trust boundary for instances
  run by different organizations.
- The contract state machine ([ADR-2](adr-2-contract-state-machine.md)) keeps
  its per-instance states; the cross-instance transitions (offer, negotiate,
  sign) are driven by shipping/receiving a PDF, and review/approval become
  purely local.

## Implementation state (verified shipped 2026-07-30)

Every row below was "pending" as of 2026-07-19 and has since shipped. The table
is kept only so the earlier revision is not mistaken for current scope.

| Piece | State |
| --- | --- |
| ADR + design | this document |
| New peer endpoint carrying the PDF (+ JAdES); receiver extracts JSON-LD via pdf-core | shipped — `POST /peer/contracts/pdf`, `backend/design/dcs_to_dcs.go` |
| Remove `action`, `get_sync`, task-sync payloads + single-writer-origin | shipped — no `get_sync` remains in `backend/` |
| Counterparty (single did:web) replaces peer-DIDs-in-roles; party set = [origin, counterparty] | shipped |
| Localize tasks/RBAC; negotiation = turn-based PDF exchange, sign = accept | shipped |
| BDD (peer_trust, real_signing_vertical) + two-instance Playwright vertical on the new base | shipped — `features/17_peer_trust`, `frontend/ClientApp/e2e/full-vertical-2dcs.spec.ts` |
