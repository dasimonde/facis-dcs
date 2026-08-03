# FACIS DCS — Backup & Restore Integration Guide

This guide specifies what a DCS instance must back up, how, and in which
order it is restored. It is the administrator backup/restore documentation
mandated by SRS §2.6, and it implements [DCS-NFR-SF-03] (business continuity
and disaster recovery) and [DCS-NFR-PER-03] (availability and resilience).
Because DCS implements GDPR erasure as key destruction
([ADR-28](adr-28-ipfs-encryption-at-rest-key-shredding.md),
[key management concept](key-management-concept.md)), backups are not
neutral copies: they carry encryption keys whose destruction is a legal
commitment, so retention and restore have erasure obligations of their own
([DCS-NFR-COMP-03], [DCS-NFR-SEC-13]). Those are part of this guide, not a
separate concern.

## 1. Backup inventory

An instance's state lives in four places. Everything else in the chart is
re-provisioned by deployment.

### 1.1 PostgreSQL

The bundled Postgres server holds four databases:

| Database | Content |
|---|---|
| `dcs` | Contracts, templates, signatures, archive entries, CID references, audit chain heads and checkpoint metadata, the `content_encryption_keys` table (wrapped CEKs **and** their shredding records), the retry queues — PDF-shipment retries and the `contract_erasures` erase-handshake ledger — and key-version pointers |
| `hydra` | OAuth2/OIDC clients — including every issued machine credential ([ADR-27](adr-27-machine-credentials-issued-not-configured.md)) — consents, tokens |
| `statuslist` | Status-list service state (credential status/revocation lists) |
| `fc` | Federated Catalogue data for deployments that point FC at the bundled server |

Deployments that run the Federated Catalogue's own database (the
`fc-postgres` subchart) additionally back up that server's `postgres`
(fc-service) and `keycloak` databases.

The `dcs` database is the critical one: it is the only place the wrapped
CEKs exist. Losing it makes every IPFS artifact permanently unreadable —
by construction, that is exactly what key shredding does on purpose.

### 1.2 IPFS repository

The Kubo repository volume (the `ipfs` subchart's PVC) holds every stored
artifact as ciphertext — contract and template PDFs, archive snapshots,
reports, **and the entire audit-trail content**: Postgres keeps only chain
heads and checkpoint metadata, the entries themselves exist only in IPFS.
An IPFS loss without backup is therefore an audit-trail loss, not just a
document loss. The `ipfs-document-manager` gateway is stateless and needs
no backup.

### 1.3 HSM token

- **SoftHSM2 deployments** (dev/staging/CI-like): the token volume (the
  chart's HSM token PVC) contains all five private keys. Back it up as a
  file-level copy of the volume; it is small and changes only at
  provisioning or rotation.
- **Production HSM**: key material never resides in the cluster. Backup,
  redundancy and recovery of the token follow the HSM vendor's procedure
  (key ceremony, backup tokens, M-of-N custodians) and are out of scope for
  cluster backups — but they gate everything: without the `dcs-ecdh`
  private key, restored wrapped CEKs cannot be unwrapped and the restored
  archive is unreadable.

### 1.4 Deployment configuration

- The instance's Helm values files (kept outside the repository; they carry
  hostnames, DIDs and credentials).
- Kubernetes Secrets that are provisioned rather than rendered: the
  identity Secret (`did.json`), the C2PA x5chain, the HSM PIN Secret, the
  ORCE TSA Secret.

### 1.5 Remaining stateful components

Fuseki (semantic hub), ORCE flows and the Federated Catalogue's graph
store also persist on PVCs. They are re-provisionable from deployment
sources and imports; include them in volume-level backups for convenience,
but they carry no contract content and no keys, and nothing in the erasure
scheme depends on them.

## 2. Backup procedure

### 2.1 Encrypted targets

Backup destinations MUST be encrypted and access-controlled
([DCS-NFR-SEC-14] applies to backups exactly as to live storage). A
database backup contains the wrapped CEKs; a database backup **plus** an
HSM token backup is sufficient to read every artifact. Treat the
combination with the same custody discipline as the live system, and store
database and token backups under separate access control where the
deployment's threat model calls for it.

### 2.2 Ordering: database before IPFS

Artifacts are written to IPFS first and referenced from Postgres second. A
backup taken in the order **Postgres dump, then IPFS snapshot** therefore
never contains a CID reference whose content is missing: the IPFS snapshot
is a superset of what the dump references, and surplus IPFS blocks are
harmless (content-addressed, unreferenced, unreadable without keys). The
reverse order can produce dangling references and must not be used.

Concretely, per backup run:

1. Dump each Postgres database (`pg_dump` per database, or a base backup of
   the server) — `dcs`, `hydra`, `statuslist`, `fc`, plus the fc-postgres
   databases where that server runs.
2. Snapshot the IPFS repository volume (CSI volume snapshot, or a file-level
   copy).
3. Copy the SoftHSM token volume (unchanged since the last provisioning or
   rotation in almost every run; cheap to include always).
4. Version the values files and re-exportable Secrets alongside.

### 2.3 Frequency and retention

Frequency follows the deployment's RPO ([DCS-NFR-SF-03]: recovery point and
recovery time are the measured objectives). Retention, however, is not a
free parameter — it is bounded by the erasure commitment described in §4.1.
Set it deliberately and record it: the retention window is part of the
instance's Art. 17 answer.

## 3. Restore

### 3.1 Order

1. **Namespace and configuration**: values files, then the provisioned
   Secrets (identity, x5chain, HSM PIN, TSA) so hooks and mounts find them.
2. **HSM token**: restore the token volume (SoftHSM) or verify the external
   HSM serves the expected token and labels. The token must correspond to
   the `did.json` about to be served and to the wrapped CEKs about to be
   restored — a mismatched token turns the whole restore into ciphertext.
3. **PostgreSQL**: restore all databases from the same backup run.
4. **IPFS**: restore the repository volume from the same run (or a later
   one — later is safe, earlier is not; see §2.2).
5. **Deploy** via `deployment/helm/deploy.sh`, which performs the instance
   health checks (backend startup gates, served DID matching the token and
   `ISSUER_DID`, well-known documents externally resolvable).
6. **Shred replay** — before returning the instance to service: §4.2.
7. **Verify**: run the archive integrity audit; fetch the audit checkpoint
   head and verify an inclusion proof for a known entry; export one
   contract end-to-end; confirm a known-erased contract still answers with
   its defined "content erased" condition rather than content, and that
   its erasure status reports the keys as shredded with the peer
   handshakes confirmed.

### 3.2 Consistency rules

- Database and IPFS restores must come from the same backup run, or IPFS
  from a later run than the database — never older.
- The `dcs` database, the HSM token and the served `did.json` form one
  consistency group: wrapped CEKs unwrap only with the token version that
  wrapped them, and peers wrap to the published `keyAgreement` key. Restore
  them as a set.
- Federation self-heals for traffic, not for keys: pending shipments and
  erase calls retry from the `dcs` database's retry queue after restore,
  but key destruction that the restore undid must be re-applied explicitly
  (§4.2).

## 4. Backups and erasure — the CEK interplay

### 4.1 Retention delays the completion of erasure

Shredding marks the wrapped-CEK rows destroyed on both instances — but
every database backup taken **before** the shred still contains the
un-shredded rows. As long as such a backup exists, the destroyed key is
recoverable by whoever holds backup plus HSM access, so:

- Erasure of a contract is **complete** only when (a) its wrapped-CEK rows
  are shredded on both federation instances and (b) every backup containing
  un-shredded copies of those rows has aged out of retention.
- The backup retention window is therefore a component of the erasure
  guarantee ([DCS-NFR-COMP-03], [DCS-NFR-SEC-13]): an instance that answers
  an Art. 17 request truthfully states destruction on the live systems at
  shred time and final completion at the end of the retention window.
  Choose the window with that sentence in mind, and do not silently keep
  backups past it.
- Within the window, the residual copies are protected by §2.1 (encrypted,
  access-controlled targets) and by the fact that reading them still
  requires the HSM. Deleting individual rows *inside* existing backups is
  deliberately not attempted; expiring whole backups is the mechanism.

### 4.2 Post-restore shred replay

Restoring a backup rewinds the `content_encryption_keys` table to the
backup point: every shred applied between backup and failure is undone, and
the "destroyed" keys are live again. Shred replay closes that gap before
the instance returns to service. The **shredded rows are the source**: each
one carries scope, recipient, `shredded_at`, `shredded_by` and
`shred_reason`, so a newer copy of the table is a complete, self-describing
replay log.

1. **Obtain the newest available copy** of `content_encryption_keys`: the
   crashed server's salvaged volume, the most recent database backup, or a
   surviving replica — whichever is newest, even if unsuitable for full
   restore.
2. **Select** from that copy all rows with `shredded_at` later than the
   restored backup's point in time.
3. **Re-apply** the shred markings to the corresponding rows in the
   restored database, preserving the original `shredded_at`,
   `shredded_by` and `shred_reason` — the destruction record keeps its true
   timeline; the replay does not re-date it.
4. **Reconcile with the peer.** For federated contracts the counterparty
   holds its own shredded rows for the same contract IRIs. Where no newer
   local copy exists (total loss of everything after the backup), the
   peer's records identify which contracts were erased; re-run the erase
   for those contract IRIs so both sides converge. Erase requests the peer
   had queued for this instance while it was down are delivered
   automatically: they stay pending in its `contract_erasures` ledger until
   this instance confirms the shred.
5. Only then re-open the instance for use. A restored instance that skips
   replay serves content whose destruction has already been recorded and
   possibly communicated — the `KEY_SHREDDED` audit events on both
   instances predate the restore and remain provable, so the live state
   must be brought back in line with them, never the other way around.
