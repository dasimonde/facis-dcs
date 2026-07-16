# Administrator Guide

## Installation

### Via Helm directly

```bash
helm install dcs deployment/helm -f deployment/helm/values.yaml -f deployment/helm/values.acceptance.yml
# or values.prod.yml for production
```

### Via ArgoCD

Apply `deployment/argocd/application.yaml` (points at `values.acceptance.yml`
by default; duplicate and swap `valueFiles`/`destination.namespace` for a
production Application). Automated sync/self-heal is intentionally left off
by default — see the manifest's comment for why.

### Local development

`bash dev-stack.sh` brings up a full single-instance stack (Helm release
`dcs`, backend via `air`, frontend via Vite) against a local Rancher
Desktop/kind cluster. `bash dev-stack2.sh` (run after `dev-stack.sh`) adds a
second instance ("instance B", release `dcs2`, backend on :8992) for the
two-instance inter-org demo — both instances share the pdf-core deployment.

## HSM / PKCS#11 configuration

Every private key in DCS lives in a PKCS#11 token (ADR-1) — there is no
code path that holds a raw key in memory or on disk outside the token.

**Dev/CI:** a SoftHSM2 software token is co-deployed and auto-provisioned
by the `hsm-provision` Helm hook job (`pkcs11.provisioning.enabled: true`,
the default). It generates five keys on every fresh install:

| Key label | Purpose |
|---|---|
| `dcs-did` | This instance's `did:web` document signing key. |
| `dcs-vc` | Lifecycle Verifiable Credential signing (`ecdsa-rdfc-2019`). |
| `dcs-oid4vp-jar` | OID4VP JWT-secured Authorization Request signing. |
| `dcs-contract-pades` | PAdES CMS signature over the contract PDF. |
| `dcs-c2pa` | C2PA claim/COSE signature. |

**Production:** disable provisioning and point at a real, pre-provisioned
HSM instead:

```yaml
pkcs11:
  modulePath: "/usr/lib/pkcs11/your-vendor-module.so"
  tokenLabel: "your-token-label"
  pinSecretRef:
    name: dcs-prod-hsm-pin
    key: PKCS11_PIN
  provisioning:
    enabled: false
```

The five key labels above must already exist on that HSM under the
configured token before installing this release — the chart does not
create HSM keys in production (see `values.prod.yml`'s comments). Swapping
vendors is this configuration change only; no DCS code references a
specific HSM vendor's API.

### Key rotation

Rotate a key by provisioning a new key under the same label's slot per
your HSM vendor's rotation procedure, then restart the DCS pods. Signatures
made with the old key continue to validate (trust anchoring is versioned,
not pinned to "the current key") — this is the drill A5 requires to be
executed and recorded at least once before acceptance.

### Trust anchor

Verification trusts one configurable chain of trust, swappable by
configuration: `customCA` (a self-issued dev/test CA with CRL) in
dev/acceptance, the EU List of Trusted Lists (`eutrustpool.go`) in
production. This is the "swap-back path" ADR-1/ADR-5 reference.

## Signing ceremony (EUDIPLO / OID4VP)

DCS does not talk to a specific wallet product; it speaks standard OID4VP
against an ARF-conformant, OIDF-conformance-tested wallet-facing layer
(EUDIPLO), configured via the `oid4vp.*` values block:

```yaml
oid4vp:
  trust:
    enabled: true
    dataPath: /app/config/oid4vp/trust.dev.json   # baked into the image, see deployment/docker/Dockerfile
    xfscAllowUnsignedFallback: false
```

`trust.dataPath` points at the trust configuration (accepted issuers,
credential schemas) the verifier checks presentations against. Replace the
baked-in dev trust file with your production trust configuration by
building a custom image layer or mounting a ConfigMap over that path.

## Trusted-peer configuration (two-instance / inter-org)

An instance only accepts peer-originated contract actions
(`POST /peer/contracts/action`) from `did:web` identities listed in
`DCS_TRUSTED_PEERS` (comma-separated), checked as part of the did:web
challenge-response handshake (`backend/internal/service/dcs_to_dcs.go`).
There is no runtime API to add a trusted peer — it is startup
configuration:

```bash
DCS_TRUSTED_PEERS=did:web:partner-a.example.org,did:web:partner-b.example.org
```

## Backup and restore

- **Database:** standard PostgreSQL backup/restore (`pg_dump`/`pg_restore`
  or your managed Postgres provider's snapshot mechanism) covers contracts,
  templates, signatures, and the archive table.
- **Archive immutability:** `contract_archive_entries` has a DB trigger
  rejecting `UPDATE` on tamper-evidence-relevant columns (including
  `evidence`) — a restore must not attempt to "fix up" archive rows;
  restore the whole table from the same backup point, or not at all.
- **HSM:** back up per your HSM vendor's key-backup procedure — DCS holds
  no key material outside the token, so there is nothing DCS-side to back
  up beyond the token's own key backup.
- **IPFS:** contract/template documents pinned to IPFS are content-addressed;
  losing the pin (not the content) is recoverable by re-pinning from the
  database's stored copy.
- **Audit trail (IPFS is the primary copy):** audit-log entries live ONLY in
  IPFS — the database (`audit_trail_log`) stores just each chain's head CID,
  and every entry links its predecessor by CID. Losing IPFS block storage
  therefore punches permanent holes in existing chains: anchoring continues
  (the head CID is read from the database, so new entries append past the
  gap), but any read that walks a chain across the gap hard-fails on the
  first missing link — deliberately, since a partial answer would present an
  incomplete trail as complete. Component-wide audit queries walk EVERY
  resource's chain, so a single damaged chain fails them all. IPFS block
  storage must be on durable volumes and backed up from the same point in
  time as the database; after an unrecoverable loss, the only reset is to
  clear the affected `audit_trail_log` rows, acknowledging the evidence
  loss. While IPFS is merely unavailable (rather than lost), the outbox
  processor retries in order and anchors the backlog once it returns —
  events are never dropped.

## Environment variable reference

The full set is documented inline in `backend/.env.dev1` (instance A) /
`backend/.env.dev2` (instance B) and `deployment/helm/values.yaml`'s
comments — those are the source of truth kept current with the code, not
duplicated here. Groups: `DATABASE_URL`; `HYDRA_*` (OIDC); `NATS_URL`;
`PKCS11_*` + `DCS_HSM_KEY_*` (see "HSM / PKCS#11 configuration" above);
`OID4VP_*` (see "Signing ceremony" above); `IPFS_*`; `TSA_URL`;
`DCS_TRUSTED_PEERS` (see "Trusted-peer configuration" above);
`FEDERATED_CATALOGUE_*`.
