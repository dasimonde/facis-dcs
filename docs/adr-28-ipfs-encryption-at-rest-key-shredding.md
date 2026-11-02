# ADR-28: IPFS artifacts are encrypted with random per-contract keys, and erasure is the logged destruction of their wrapped copies

Status: accepted, 2026-07-28.

Related: [ADR-1](adr-1-key-custody.md) for the PKCS#11 custody interface the
wrapping key lives behind, [ADR-16](adr-16-audit-checkpoints-external-anchoring.md)
for the audit chain the erasure record must not break,
[ADR-13](adr-13-pdf-exchange-federation.md) for the peer exchange the key
handshake rides on.

## Context

Every artifact the DCS produces — contract and template PDFs, archive
snapshots, audit-trail entries, checkpoints, compliance reports — is stored in
IPFS. IPFS is content-addressed and replicating by design: unpinning removes a
local pin, it does not delete. Three requirements collide on that property:

- [DCS-NFR-SEC-14] requires sensitive data to be encrypted at rest.
- [DCS-NFR-COMP-03] requires GDPR compliance, which includes the Art. 17
  right to erasure, and [DCS-NFR-SEC-13] requires that sensitive data is
  properly deleted when no longer needed. [DCS-IR-CSA-03] requires the
  Archive Manager to be able to delete archived entries under defined
  policies.
- The audit trail ([ADR-16](adr-16-audit-checkpoints-external-anchoring.md))
  requires that stored entries remain hash-chained and provable — a deletion
  that breaks inclusion proofs converts an erasure obligation into a
  tamper-evidence failure.

A store that cannot delete, a regulation that demands deletion, and a trail
that must prove nothing was deleted cannot be reconciled by deleting bytes.
They are reconciled by controlling who can ever read them.

A second constraint comes from federation: a contract exists on two instances,
each holding its own copy of the artifacts. Erasure that only takes effect on
the instance that received the request is not erasure.

## Decision

**Every IPFS artifact is encrypted before storage under a random per-scope
content-encryption key (CEK), the CEK is held only in HSM-wrapped form, and
Art. 17 erasure is implemented as the logged destruction of those wrapped
copies on both federation instances.**

Concretely:

- **One random 256-bit CEK per contract** (keyed by contract IRI), covering
  its PDFs, archive snapshot, and audit-entry bodies; one per template; one
  instance-wide scope for checkpoints and instance-level reports. Content is
  AES-256-GCM with the scope identifier as associated data, stored as
  nonce ∥ ciphertext.
- **The CEK is random, not derived.** A key derived from static-static ECDH
  between the two instances' key-agreement keys could be re-derived forever
  from key material that erasure deliberately leaves alive — shredding the
  stored copies would destroy nothing, and the erasure claim collapses.
  Randomness is what makes destruction final.
- **CEKs exist at rest only wrapped.** Each holder wraps the CEK to its own
  `keyAgreement` public key via ECDH-ES+A256KW; the matching static private
  key (`dcs-ecdh`) is non-extractable in the PKCS#11 token
  ([ADR-1](adr-1-key-custody.md)), so unwrapping requires that instance's
  HSM. Every shipment of a contract additionally carries the CEK wrapped to
  the peer's published `keyAgreement` key; the receiver adopts it once —
  unwrap, re-wrap to its own key — and ignores the copy on every later
  shipment, so a lost adoption heals on the next ship. Both instances then
  hold the same CEK, each copy openable only by its own HSM.
- **Shredding destroys the key and keeps the record.** Erasure sets
  `wrapped_cek` to NULL and sets `shredded_at`, `shredded_by` and
  `shred_reason` on every wrapped-CEK row for the scope; the row itself is
  retained as the destruction record, and a database constraint refuses a
  shredded row that still holds a wrapped key. In the same transaction the
  plaintext copies of the contract content that Postgres holds are erased:
  `contracts.contract_data`, `contract_history.contract_data`, negotiation
  change requests and drafts, the archive entry's `contract_snapshot` (the
  only change the archive trigger permits, and only once the scope is
  shredded; `content_hash` stays as evidence) and the payload pinned for
  signing. The same transaction erases the other copies of the content and
  of the signers' personal data: the prepared PDF, PID claims and
  presentations of the signing ceremonies; the payload of every JAdES
  signature over the contract (`contract_signatures`,
  `contract_sync_signatures`), which becomes a detached JWS
  (RFC 7515 Appendix F) whose protected header and signature value stay as
  evidence; and the outbox copy of every event body that the audit trail
  encrypts under the contract's key, replaced by the erased marker. The audit
  chain hashes the stored IPFS entry, not the outbox row, so this change
  breaks no chain or checkpoint proof; the erasure records and the bodies of
  other scopes keep their outbox copy, as they keep their audit body.
  `contracts.contract_data` keeps one thing: the `dcs:legalName` of each
  `dcs:parties` node. These organization names are what the contract read
  policy authorizes on, so the named parties keep read access to the erased
  contract's metadata, erasure status and audit trail. Key creation and
  shredding are serialized per scope, and a shredded scope is final: neither
  a local write nor a peer-shipped wrapped copy ever creates a new CEK for
  it. A peer erase call
  (`POST /peer/contracts/erase`, authenticated with the same body-level
  did:web challenge-response as the PDF exchange) extends the destruction to
  the counterparty, and both sides emit a `KEY_SHREDDED` audit event
  carrying actor, contract IRI, scope and reason — never content.
- **The audit trail splits public header from private body.** Chain-bearing
  fields of an audit entry (component, event type, DID, timestamp,
  predecessor CID, blinding nonce) stay plaintext; the event payload is
  encrypted under the contract's CEK. Leaf hashes and Merkle checkpoints are
  computed over the stored object as-is, so shredding renders the bodies
  permanently unreadable while every inclusion proof continues to verify.
  Tamper evidence and erasure coexist because the proof never depended on
  reading the plaintext.
- **The instance scope is not shreddable, and says so.** Events not
  attributable to a contract — checkpoints, instance-level reports, and the
  erasure records themselves, the `KEY_SHREDDED` and `DELETE_ARCHIVED`
  events — live under the instance CEK, which no erasure request destroys.
  An erasure record that could erase itself would prove nothing, and a
  deletion proof must not lie under the key whose destruction it proves.

### Complete audit reports (P06 decision, 2026-10-15)

Complete audit runs and their exported reports may contain private evidence
from several resources. They depend on all of those resource CEKs: the
existing encryption operation is applied successively in sorted, deduplicated
scope order. A versioned envelope names those scopes and the audit ID; outer
instance encryption authenticates that envelope. No additional key hierarchy
is introduced. Request and executor response bytes survive decryption exactly,
including whitespace; there are no additional plaintext response or receipt
columns in `pac_audit_runs`.

Shredding any participating resource makes the entire affected saved run and
report unreadable. `GET /pac/report` returns 404 for that selected run, without
falling back to an older one. This consequence for reports covering several
resources was explicitly accepted. Public headers, already-erased bodies and
the existing instance-scoped deletion proofs do not create new resource-key
dependencies, so past erasures alone do not prevent future audits. Evidence
of a shredded resource that the database still holds, such as an archive
entry summary or a synthetic check, is replaced by the erased marker before
the run is sent and sealed.

The report hash covers the final downloaded plaintext bytes. Its CID names
the encrypted report envelope in IPFS. Retrieving that artifact requires both
the existing instance decryption and the PAC envelope decryption. Encryption
work grows with the number of participating resource scopes.

Two deliberate exclusions:

- **P-256, not X25519.** Every key in the system is EC P-256, the PKCS#11
  path for `CKM_ECDH1_DERIVE` on P-256 is proven, and the DID-document
  parser accepts only P-256 JWKs. X25519 would add a second curve and parser
  surgery for no security gain at this profile.
- **No pgcrypto column layer in Postgres.** Artifact plaintext lives in IPFS
  behind the CEK scheme; wrapped-CEK rows are ciphertext by construction.
  [DCS-NFR-SEC-14] for the database is met by an encrypted StorageClass on
  the persistent volumes — a platform property the deployment's post-deploy
  check asserts where a class is configured, and names as an operator
  prerequisite where none is —
  rather than by a second application-level key hierarchy that would double
  the key-management surface for data that is either already ciphertext or
  non-sensitive metadata.

## Consequences

Erasure becomes an auditable act instead of an impossible byte-deletion: the
shredded rows and the paired `KEY_SHREDDED` events on both instances are the
evidence [DCS-NFR-SEC-13] asks for, and the ciphertext left in IPFS is
permanently opaque because no unwrappable copy of its key exists anywhere.

CIDs for the same contract now diverge between instances: each side encrypts
the byte-identical PDF under its own nonce, so the content addresses differ.
This is harmless — no CID ever crosses the peer interface; contracts travel
as raw bytes and each side addresses only its own store.

Reads become key-dependent. Export, verification and bundling of a shredded
contract fail with a defined shredded-key condition — answered as not-found,
never as an internal error, and shown to the user as "Content erased —
encryption keys destroyed" — while list and metadata views keep working from
Postgres. Anything that re-reads stored artifacts — integrity audits included
— must go through the decrypting store, or ciphertext masquerades as
corruption; the archive integrity audit accordingly excludes entries this
instance has deleted, whose snapshots are disposed and whose keys are
destroyed, while its timeline still documents the deletion itself.

Backups delay the completion of erasure. A wrapped-CEK row shredded today
still exists un-shredded in yesterday's database backup; the erasure promise
is only as strong as the backup retention window, and after a restore the
shred markings applied since the backup must be re-applied. The backup guide
(docs/backup-integration-guide.md) makes retention and shred-replay part of
the erasure procedure rather than a footnote.

The `dcs-ecdh` key becomes the single point through which every stored
artifact is reachable. Its rotation is an unwrap-all/re-wrap-all operation,
documented as an operator procedure in the key-management concept; losing the
key (or a production HSM without `CKA_DERIVE` on P-256) makes the entire
store unreadable, which is the same property that makes shredding work.

## Alternatives considered

**Derive per-contract keys from static-static ECDH.** No key storage, no
wrapped rows, deterministic recovery on both sides. Rejected for exactly that
reason: deterministic recovery is the opposite of erasability. As long as
both static keys exist — and they must, for every other contract — the
"erased" key can be re-derived, so nothing was destroyed.

**Delete the IPFS blocks instead of the keys.** Unpin plus garbage collection
removes local blocks, but content-addressed storage gives no guarantee about
replicas, caches, or the shared dev/CI node, and the audit chain would lose
the very leaves its checkpoints committed to. Key destruction erases every
copy at once — including ones this instance never knew about — and leaves the
proofs intact.

**Encrypt Postgres columns with pgcrypto as well.** Defense in depth on
paper; in practice a second key hierarchy whose keys must live somewhere the
database can reach, protecting rows that are already ciphertext or metadata
needed for list views. Disk-level encryption covers the residual risk
(stolen volume) without a second custody chain.

**A single instance-wide CEK for everything.** One wrap, one row, trivial
management — and one erasure request would destroy the readability of every
contract, or none. The CEK scope must equal the erasure unit, and the
erasure unit under [DCS-IR-CSA-03] is the contract.
