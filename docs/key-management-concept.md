# FACIS DCS — Key Management Concept

This document is the delivered key-management concept for the Digital
Contracting Service (DCS). It describes every cryptographic key a DCS
instance holds, where each key lives, how it is provisioned and rotated, and
how the content-encryption scheme turns GDPR erasure into a verifiable key
destruction. It is the evidence document for [DCS-NFR-SEC-06],
[DCS-NFR-SEC-13], [DCS-NFR-SEC-14] and [DCS-IR-HI-01]; the architectural
decisions behind it are recorded in
[ADR-1](adr-1-key-custody.md) (PKCS#11 as the single custody mechanism),
[ADR-12](adr-12-wallet-driven-signing.md) (no DCS-held contract-signing key)
and [ADR-28](adr-28-ipfs-encryption-at-rest-key-shredding.md) (encryption at
rest and key-shredding erasure).

## 1. Custody model

All DCS-held private keys live in one PKCS#11 token and never leave it. The
backend opens the token via `PKCS11_MODULE_PATH`, `PKCS11_TOKEN_LABEL` and
`PKCS11_PIN`; keys are located by `CKA_LABEL` and are generated
non-extractable. Development, CI and staging use a SoftHSM2 software token;
production points the same three variables at a real HSM's PKCS#11 module —
a configuration change, not a code change ([DCS-IR-HI-01]).

The DCS holds **no contract-signing key**. Contract signatures are produced
by the signatory's own wallet/QTSP ([ADR-12](adr-12-wallet-driven-signing.md),
[ADR-17](adr-17-machine-signing-is-not-an-aes-signature.md)); every key
below is the DCS attesting or protecting as itself.

## 2. Key inventory

One token per instance, five EC P-256 keypairs:

| Label | Curve / mechanism | Purpose | Published in did.json | Env override |
|---|---|---|---|---|
| `dcs-did` | P-256, ECDSA | Instance DID identity: peer challenge-response authentication, JAdES transport envelope; carries the eIDAS x5c | verification method `#dev-key-1`, `assertionMethod` | `DCS_HSM_KEY_DID` |
| `dcs-vc` | P-256, ECDSA | Lifecycle verifiable credentials (signing-summary VC and related attestations) | verification method `#dcs-vc`, `assertionMethod` | `DCS_HSM_KEY_VC` |
| `dcs-oid4vp-jar` | P-256, ECDSA | OpenID4VP request objects (JAR) for wallet authentication | — | `DCS_HSM_KEY_JAR` |
| `dcs-c2pa` | P-256, ECDSA | C2PA COSE claim signatures; bound to the C2PA x5chain | — | `DCS_HSM_KEY_C2PA` |
| `dcs-ecdh` | P-256, ECDH (`CKA_DERIVE`) | Key agreement for content-encryption-key (CEK) wrapping — the encryption-at-rest anchor ([ADR-28](adr-28-ipfs-encryption-at-rest-key-shredding.md)) | verification method `#dcs-ecdh`, `keyAgreement` | `DCS_HSM_KEY_ECDH` |

`dcs-ecdh` is the only key with `CKA_DERIVE` usage and the only one
referenced from the DID document's `keyAgreement` relationship. It performs
no signatures; the four signing keys perform no key agreement. Peers discover
the public half by resolving the instance's `did.json` and selecting the
`keyAgreement` verification method.

Custody is identical for all five: private halves non-extractable in the
token, public halves either published in `did.json` (`dcs-did`, `dcs-vc`,
`dcs-ecdh`) or bound into certificate chains (`dcs-c2pa`).

## 3. Provisioning

### 3.1 Development, CI, staging — SoftHSM2 in-cluster

With `pkcs11.provisioning.enabled=true` the Helm chart runs a
post-install/post-upgrade hook job that executes `scripts/hsm-provision.sh`:
it initialises the SoftHSM2 token idempotently, generates the five P-256
keypairs by label (passing the derive usage flag for `dcs-ecdh`), issues the
C2PA x5chain, and regenerates `did.json` — including the `#dcs-ecdh`
verification method and the `keyAgreement` relationship — from the token's
actual public keys. The backend refuses to start when the served `did.json`
does not match the token, so a drifted identity fails loudly at boot rather
than silently at first peer contact.

Local development provisions the same token layout on the host
(`bash dev-stack.sh`, one token directory per instance).

### 3.2 Production — external HSM, provisioned out-of-band

Production sets `pkcs11.provisioning.enabled=false`; the chart creates no
key material. The operator provisions the token on the real HSM before the
first deployment. The prerequisites are checkable, and each has a symptom
when missed:

1. **Five EC P-256 keypairs exist under the exact labels above** (or the
   `DCS_HSM_KEY_*` overrides match the labels chosen). Missing key: the
   backend fails at startup when locating keys by label.
2. **`dcs-ecdh` carries `CKA_DERIVE`, and the module supports
   `CKM_ECDH1_DERIVE` on P-256.** Verify before go-live with
   `pkcs11-tool --module <module> --token-label <label> --login
   --list-objects --type privkey` (the key's usage attributes must include
   derive) and a test derivation. A token without it accepts writes but can
   never unwrap a CEK — every stored artifact is unreadable.
3. **Private keys are non-extractable** ([DCS-NFR-SEC-06]); `CKA_SIGN` on
   the four signing keys.
4. **`did.json` is generated from this token's public keys** and served at
   the instance's `/.well-known/did.json`; peers wrap CEKs to whatever
   `keyAgreement` key is published there, so a mismatch breaks federation
   in both directions.

## 4. Content encryption at rest

Every artifact written to IPFS — contract and template PDFs, archive
snapshots, audit-entry bodies, checkpoints, reports — is encrypted before
storage ([DCS-NFR-SEC-14]); the rationale and trade-offs are in
[ADR-28](adr-28-ipfs-encryption-at-rest-key-shredding.md).

### 4.1 Scopes

The encryption unit equals the erasure unit:

- **Contract scope** (contract IRI): the contract's PDFs, archive snapshot,
  and audit-entry bodies. One CEK per contract; erasable.
- **Template scope** (template IRI): the template PDF. One CEK per template;
  erasable.
- **Instance scope**: Merkle checkpoints, instance-level audit and
  compliance report artifacts, and events not attributable to a contract —
  including the `KEY_SHREDDED` and `DELETE_ARCHIVED` erasure records, which
  must not lie under the very key whose destruction they prove. One CEK per
  instance; **not erasable** (see §7).

### 4.2 Content algorithm

AES-256-GCM under the scope's CEK, fresh random 96-bit nonce per object,
stored as nonce ∥ ciphertext, with the scope identifier as GCM associated
data — a ciphertext replayed under a different scope fails authentication.
Decryption returns the byte-identical original: the envelope is invisible to
everything above it, so C2PA hard bindings, JAdES signatures and the
verbatim-inbound-PDF rule of the federation
([ADR-13](adr-13-pdf-exchange-federation.md),
[ADR-26](adr-26-provenance-reanchored-after-signing.md)) are unaffected.

### 4.3 CEK wrap algorithm — ECDH-ES+A256KW

A CEK exists at rest only in wrapped form. Wrapping follows RFC 7518 §4.6
with an ephemeral sender key, so it needs nothing but the recipient's public
key; unwrapping requires the recipient's HSM:

1. **Ephemeral keypair.** The wrapper generates a fresh P-256 keypair in
   software; the ephemeral private key is discarded after step 2.
2. **ECDH.** Shared secret Z = ECDH(ephemeral private, recipient's static
   `keyAgreement` public key).
3. **Concat-KDF** (SHA-256, single round for 256 output bits) over Z with
   AlgorithmID `ECDH-ES+A256KW`, empty PartyUInfo/PartyVInfo, SuppPubInfo =
   256. The result is the key-encryption key.
4. **AES key wrap** (RFC 3394) of the 256-bit CEK under the KEK.

The stored record is `{alg: "ECDH-ES+A256KW", epk: <ephemeral public JWK>,
wrapped: <RFC 3394 blob>}`.

**Unwrap** reverses this with the roles swapped: the holder's HSM performs
`CKM_ECDH1_DERIVE` (`CKD_NULL`) between its static `dcs-ecdh` private key and
the ephemeral public key from the record — the static key never leaves the
token, and the derived session secret is destroyed immediately after the KDF
— then Concat-KDF and RFC 3394 unwrap yield the CEK. The RFC 3394 integrity
register rejects both a tampered blob and a wrong key-agreement key.

## 5. CEK lifecycle

### 5.1 Creation and storage

On the first write into a scope, the instance generates a random 256-bit
CEK, wraps it to its **own** `keyAgreement` key, and persists the wrapped
record in the `content_encryption_keys` table: `scope_kind` and `scope_id`
identify the scope, `recipient_did` the holder the record is wrapped to,
`wrapped_cek` carries the wrap record of §4.3, `created_at` the creation
time, and `shredded_at` / `shredded_by` / `shred_reason` the destruction
record of §5.3. A partial unique index guarantees at most one **live**
(un-shredded) row per (scope, recipient). Key creation and shredding are
serialized through a per-scope database lock, so a first write cannot race
a concurrent shred; a scope that already carries shredded rows never
receives a new CEK — not from a local write and not from a peer shipment.

Plaintext CEKs exist only in process memory: unwrapped keys live in a
bounded in-memory cache of at most 256 scopes and are dropped from it the
moment their scope is shredded. Nothing persistent holds an unwrapped CEK.

### 5.2 Federation — both parties hold the same CEK

Every shipment of a contract to the counterparty carries the contract CEK
wrapped to the peer's `keyAgreement` public key (taken from the peer
`did.json` the sender already resolves for authentication). The receiver —
after the exchange's existing authentication and trust gates — adopts it on
first receipt: it unwraps the record through its own HSM, immediately
**re-wraps it to its own key**, persists that row, and encrypts the received
PDF under it. On every later shipment the included copy is ignored — an
existing live record wins — so adoption is idempotent and a receiver that
lost it self-heals on the next ship. A shredded scope never adopts: the
peer copy cannot resurrect a destroyed key. From then on both instances
hold the same contract CEK, each copy unwrappable only by its own HSM. The
CEK crosses the wire only in wrapped form, openable solely by the
addressed peer's HSM; no CID crosses the peer interface at all.

### 5.3 Shredding — erasure as key destruction

Erasure of a contract ([DCS-NFR-COMP-03] Art. 17, [DCS-NFR-SEC-13],
[DCS-IR-CSA-03]) is executed from the archive deletion flow and consists of:

1. **Local shred.** Every wrapped-CEK row for the contract scope is marked
   with `shredded_at`, `shredded_by` and `shred_reason` (fed by the deletion
   justification). Rows are **never deleted** — the shredded row is the
   destruction record. (A scope that never held content has no rows to
   mark, so shredding it leaves no record; in practice this is unreachable,
   because the CEK is created by the scope's first artifact write — before
   there is anything to erase.)
2. **Peer handshake.** The instance calls `POST /peer/contracts/erase` on
   the counterparty, authenticated with the same body-level did:web
   challenge-response used for PDF shipment. The peer shreds its rows for
   the same contract IRI. An unreachable peer does not lose the request:
   each requested peer erase is a row in the `contract_erasures` ledger,
   pending until the peer confirms and re-delivered by a retry scheduler
   with the same mechanics as PDF shipment — but a separate queue, so
   shipments and erasures never mix.
3. **Audit evidence.** Both sides emit a `KEY_SHREDDED` audit event (actor,
   contract IRI, scope, reason — no content), recorded — like the
   `DELETE_ARCHIVED` event of the deletion flow that triggered it — under
   the instance scope so the evidence survives the destruction it
   documents.

After shredding, the contract's IPFS ciphertext is permanently opaque on
both instances; export, verification and bundling answer with a defined
"content erased" condition — a not-found answer, never an internal error,
presented to the user as "Content erased — encryption keys destroyed" —
while list and metadata views keep operating on Postgres-side metadata.

The erasure state is inspectable without touching content:
`GET /archive/erasure-status` reports to the Archive Manager and the
Auditor the contract's local key state — live, or shredded with the
destruction record's time, actor and reason — plus the per-peer handshake
state (pending or confirmed) from the `contract_erasures` ledger, and the
archive view surfaces the same state as a key-destruction badge. Together
with the `KEY_SHREDDED` events this makes an Art. 17 answer verifiable
end-to-end.

Backups extend the life of wrapped CEKs beyond the shred: see the retention
and shred-replay procedures in the
[backup integration guide](backup-integration-guide.md), which are part of
the erasure commitment.

## 6. Interplay with the audit trail

Audit entries are split into a public header and a private body
([ADR-16](adr-16-audit-checkpoints-external-anchoring.md),
[ADR-28](adr-28-ipfs-encryption-at-rest-key-shredding.md)):

- **Stays readable forever:** component, event type, DID, timestamp, the
  hash-chain predecessor CID and the blinding nonce — everything the chain
  and the Merkle checkpoints are computed over.
- **Becomes unreadable on shred:** the event payload (`event_data`),
  encrypted under the owning contract's CEK.
- **Why proofs keep verifying:** leaf hashes are computed over the stored
  entry as-is — with its body in ciphertext. Shredding changes nothing about
  the stored bytes; it only destroys the ability to decrypt them. Inclusion
  proofs against the TSA-anchored checkpoint chain therefore verify
  identically before and after erasure. Tamper evidence ([DCS-NFR-SEC-10])
  and secure disposal ([DCS-NFR-SEC-13]) coexist because the proof never
  required the plaintext.

The archive integrity audit follows the same discipline. On the instance
that executed the deletion, entries it has deleted are excluded from the
re-fetch checks — their snapshots are disposed and their CEK is shredded,
so re-checking them would report the erasure as permanent corruption — while
the audit's timeline still documents the deletion itself (time, actor,
justification). The peer erase handshake destroys keys only: the
counterparty's own archive entries remain in place until its own deletion
flow removes them, and until then its integrity audit reports their
now-unreadable snapshots as failed re-fetch findings — the erasure is
visible there as evidence, not silently skipped. Whether and when the
counterparty deletes its archive records remains that instance's own
archival decision under [DCS-IR-CSA-03].

## 7. Non-shreddable scopes

The instance scope is deliberately outside every erasure path. It covers
Merkle checkpoints, instance-level reports, and audit events that belong to
no contract — including every `KEY_SHREDDED` event. This is a design
property, not a gap: destruction evidence that could destroy itself would
prove nothing, and checkpoints anchor the trails of all contracts at once,
so no single data subject's request may reach them. Erasure requests
resolve to contract (or template) scopes only.

## 8. Rotation

### 8.1 Signing keys — implemented

The four signing keys rotate via `scripts/rotate-hsm-key.sh`: it generates
the next-version keypair in the token (versioned label `<base>-v<N>`),
optionally issues the new x5chain, and advances the
`pki_active_key_version` pointer. New signatures use the new version;
historical signatures stay attributable to and verifiable against the
versions that produced them, which remain in the token
([DCS-OR-C2PA-007]). Rotation is an operator action, not an HTTP endpoint:
it provisions token material the running process cannot mint itself. All
five keys — `dcs-ecdh` included — appear with their active version in the
Sys. Administrator's read-only key inventory (`GET /admin/hsm-keys`), the
visibility proof of the version pointer without token access.

### 8.2 `dcs-ecdh` — documented operator procedure

Rotating the key-agreement key means every wrapped CEK must move to the new
key; a rotation that skipped this would strand all stored content on the old
key. The procedure is deliberately an operator-run maintenance operation and
not implemented as an automated job:

1. Generate the next `dcs-ecdh` version in the token (same script; the
   derive usage carries over to versioned labels). Do **not** remove the old
   version yet.
2. **Re-wrap all:** for every non-shredded row in `content_encryption_keys`
   held by this instance — unwrap the CEK via the old key version, wrap it
   to the new version's public key, replace the wrapped record. Shredded
   rows are never re-wrapped; destroyed stays destroyed.
3. Regenerate and serve `did.json` so the `keyAgreement` verification
   method carries the new public key; peers wrap future CEKs to it.
4. After verifying a sample of reads across all scope kinds, remove or
   disable the old key version. Until then it remains available as the
   unwrap path for any record the sweep missed.

The window in which both versions exist is the safety margin of the
procedure; the old version's removal is the point of no return and is left
to explicit operator judgment.

## 9. PostgreSQL at rest

[DCS-NFR-SEC-14] covers the relational side as well. The measure is
**disk-level encryption via an encrypted StorageClass** on the persistent
volumes — the Postgres data volume, the SoftHSM token volume (where used)
and the IPFS repository volume — configured per environment through the
charts' existing `storageClassName` values. The deployment script's
post-deploy verification checks each of these volumes: where a
StorageClass is configured, it asserts that the bound claim actually uses
it and flags a mismatch as a problem (a pre-existing claim keeps its
class and must be recreated to move); where none is configured, it states
the encrypted-class requirement as an operator prerequisite instead of
asserting it silently.

There is deliberately **no pgcrypto column-encryption layer**: artifact
plaintext lives in IPFS behind the CEK scheme, wrapped-CEK rows are
ciphertext by construction, and the remaining rows are metadata that list
views must read. A second application-level key hierarchy would double the
custody surface for no additional protection beyond what the encrypted
volume already gives against the relevant threat (a detached disk or
volume snapshot).

Database connections inside the cluster currently do not use TLS
(`sslmode`); enabling in-transit encryption toward the database is a
separate hardening item and is intentionally not part of this concept's
scope.

## 10. Out of scope

- **Postgres row erasure** is ordinary deletion under the application's
  own flows; key shredding concerns the IPFS-side ciphertext.
- **Ciphertext on the peer wire**: peer traffic is protected by TLS
  ([DCS-NFR-SEC-01]) and body-level challenge-response authentication; the
  envelope protects storage, not transport.
- **Backup and restore procedures**, including how retention interacts with
  erasure, are specified in the
  [backup integration guide](backup-integration-guide.md).
