# Architecture decisions

This document collects decisions that the software implements as a
solution, in particular the many small decisions that were never recorded
as an ADR. It complements the ADRs (`docs/adr-*.md`) and does not replace
them: where an ADR exists, this document links to it and does not repeat
it. The document is organized by system area, not by location in the code.

Scope: backend core (`backend/internal/base/`, `backend/cmd/`,
`backend/migrations/`), backend domain layers (the other packages in
`backend/internal/`, `backend/design/`), rendering service (`pdf-core/`)
and web client (`frontend/ClientApp/src/`).

## Templates and content (semantics, validation)

- **Rule:** Documents (templates, contracts) carry as anchors the
  versioned hub URLs for the JSON-LD context (`@context`) and the SHACL shapes
  (`sh:shapesGraph`). The anchors are set one time, at production. A
  document keeps the hub versions under which it was authored.
  **Reason:** Revalidation must run against exactly the schema version that
  was valid at that time (ADR-8). The document has no validation profile
  anchor: the profile name told nothing about the vocabulary of the actual
  contract data (ADR-23). The profile rules still run at validation time.
- **Rule:** DCS rejects a document whose `@context` redefines a prefix
  that the Semantic Hub declares to a different IRI
  (`ErrDocumentSchemaConflict`, HTTP 400). DCS also rejects external context
  IRIs that are not registered in the hub. **Reason:** Validation resolves
  contexts hermetically (without network access). An unregistered IRI would
  make each later audit of the document fail (DCS-FR-TR-03).
- **Rule:** The contract hierarchy is strictly child→parent: at most one
  `dcs:parentContract`, never properties that enumerate children
  (`dcs:childContracts`, `dcs:subContracts`, `dcs:hasPart`) in the document.
  **Reason:** A child list would leak siblings to each recipient of the
  parent document and would force a document rewrite for each new child
  (ADR-7).
- **Rule:** The contract data graph is self-contained: each domain object
  is a typed node that `@id` addresses. Values are literals, references to
  declared `ContractField`s or references to other domain objects in the
  document. Embedded blank nodes are not permitted. Absolute IRIs to
  external resources (sh:IRI leaves) are permitted.
  **Reason:** SHACL traversal and rendering must not consult anything
  outside the document (ADR-23).
- **Rule:** `dcs:policies` accepts only one single enclosing
  `odrl:Offer`/`odrl:Agreement` (template: Offer; contract: Offer, after
  signature Agreement) with `odrl:profile`, `@id` and complete rules
  (exactly one `odrl:action` plus assigner/assignee/target, each rule with a
  `dcs:prose` link back to the human-readable clause). DCS rejects bare rule
  arrays. **Reason:** Only the canonical form is consumable by a standard
  ODRL processor (ADR-6).
- **Rule:** The ODRL constraint semantics are written one time as Rego and
  run on the embedded OPA engine. A parity test makes sure that the verdicts
  are equal to the historical Go implementation. See ADR-11.
- **Rule:** SHACL validation runs on goRDFlib (ADR-9). Parsed shapes graphs
  are cached process-wide by content hash (max. 8 entries, then reset).
  **Reason:** A registered shapes library can be megabytes of Turtle.
  Re-parsing per request dominated the latency. The graph is read-only, and
  the cache fill warms the lazy indexes. Thus concurrent validations can
  share the graph.
- **Rule:** Peer-side validation of a received document runs against the
  Semantic Hub of the origin (`RemoteShapeSource` with an explicit
  ShapeSource instead of the process-wide state). **Reason:** A one-time
  remote hub validation must not mutate shared process state under
  concurrent requests (ADR-23, phase 4).
- **Rule:** At start, DCS reconciles the FC schemas (`migrations/fcschemas`)
  with the Federated Catalogue by content (matching by prefix/IRI
  occurrence, never a delete in the FC; an update resolves a create
  conflict). **Reason:** The FC assigns its own IDs. Content is the only
  stable identity.

- **Rule:** The Semantic Hub versions immutably: DCS never edits a
  registered version. When the content of the embedded assets drifts, the
  seed at start registers a new active version. Older versions stay
  resolvable. **Reason:** Documents pin hub versions (ADR-8). An asset
  update in the deployment must not reinterpret pinned anchors.
- **Rule:** Hub reads (`/semantic/...`) are public, like
  `/.well-known/did.json`. **Reason:** Produced artifacts carry hub anchors
  that external verifiers must dereference without a DCS login.
- **Rule:** DCS freezes schemas registered by URL (Gaia-X and similar,
  behind w3id/purl redirects) as a byte snapshot and never stores them as a
  live reference. The fetch is bound to RBAC and limited in scheme, size and
  time. A context version must at least parse as a JSON-LD document with
  `@context` before DCS activates it.
- **Rule:** The clause catalog is a second, independently versioned
  `kind="shapes"` entry (ADR-10). The template builder palette
  (`GET /semantic/clauses`) and the contract validation read the same
  shapes graph.
- **Rule:** DCS checks the Federated Catalogue readiness with exactly one
  functional request. The check payload is the embedded known-good
  credential from the upstream FC test suite. The native actuator check runs
  only for co-deployed catalogues. Remote operators usually do not expose
  actuator endpoints.

## Lifecycle (workflow, deployment, operational configuration)

- **Rule:** `contractstate.Transitions` is the single source of truth for
  the permitted (state, event) pairs and their target states. Each command
  handler action validates against it. The existing task orchestration only
  selects among the permitted exits. Violations wrap `ErrInvalidTransition`,
  and the HTTP edge maps them to 400 (not 500). No-op transitions
  (from == to) are always permitted.
- **Rule:** Some edges have this intentional cut: withdraw is not possible
  from APPROVED on. SIGNED→SIGNED covers more signatories of a multi-signer
  contract. ACTIVE→ACTIVE makes deploy idempotent (signature completion
  deploys automatically; a manual re-dispatch and its second ack stay
  valid). REVOKED→APPROVED opens the re-signing path (UC-15).
  OFFERED→NEGOTIATION exists twice: by submit for the creator, by negotiate
  for the counterparty.
- **Rule:** Negotiation authorization splits by ownership: for a received
  (inbound) offer, the right to negotiate comes from the role as the
  designated counterparty, not from local negotiator RBAC. Local RBAC
  applies only to contracts that the instance authored.
- **Rule:** DCS applies a structural redline immediately to `contract_data`
  and ships it to the peer as a newly rendered PDF (ADR-13). DCS stores a
  free-text change request only for the negotiation audit trail. The
  lost-update protection compares against `content_updated_at`. Only real
  content changes move this value. State transitions and background writes
  do not cause a false trip.
- **Rule:** Signature fields are materialized during negotiation (one
  `dcs:SignatureField` per participating instance, named after its DID: the
  AcroForm field of the signing ceremony, ADR-12). An explicit declaration
  wins: DCS never adds fields to a contract that already declares fields.
  The auto-seeding is idempotent.
- **Rule:** Deployment (ADR-25/ADR-27): the target is a property of the
  contract (thus the automatic trigger after signature has a target without
  a human). An operator override only redirects one delivery and does not
  change the designation. The dispatch record copies the registry endpoint
  at the time of dispatch. Later registry edits do not rewrite what a
  deployment actually did. The payload content hash is canonical
  (recursively key-sorted, compact, without HTML escaping), so that the
  target system can reproduce it from the parsed JSON.
- **Rule:** Multi-signer gate: deploy requires that each declared signature
  field is signed, except the fields of foreign parties. A counterparty
  signs in ITS deployment, and its signature record never reaches this
  instance. The received, content-verified artifact carries the signature
  of that party.
- **Rule:** The deployment callback is the authoritative success report (a
  failed outbound call is not fatal, but DCS marks it on the dispatch record
  and the compliance monitor raises an alarm). DCS accepts a callback only
  if the authenticated OAuth2 client matches the registry entry of the
  dispatched target. A target can acknowledge its own deployments and not
  those of others (ADR-27; this replaces the earlier rule "any target knew
  the shared secret"). The ack seals an RFC 3161-timestamped execution
  evidence receipt in the archive and switches SIGNED→ACTIVE.
- **Rule:** The webhook platform is a fan-out layer: a fixed list of
  subscribable events (`KnownEvents`), a mapping of internal NATS event
  types to webhook names (`DCSEventMap`), asynchronous delivery.
  Subscriptions are in Postgres (`webhook_subscriptions`) and belong to the
  organization (participant DID) of the creator. Create, list and delete
  require `Integration Manager` or `Sys. Administrator` (else 403).
  GET /webhooks shows only the subscriptions of the own organization. DELETE
  on a subscription of another organization returns 404. For each
  subscription, DCS generates a signature secret per Standard Webhooks
  (`whsec_` + Base64 of 32 random bytes) and returns it exactly one time, in
  the 201 response of POST /webhooks. DCS rejects a `secret` that the client
  sends with 400. Postgres holds the secret only in encrypted form:
  AES-256-GCM under a separate CEK per subscription, bound to the
  subscription ID. The CEK is wrapped with ECDH-ES+A256KW to the
  keyAgreement key of the instance. Only the HSM decrypts it, at each
  delivery. Each delivery carries the headers `webhook-id` (event ID),
  `webhook-timestamp` (Unix seconds) and
  `webhook-signature: v1,<Base64 HMAC-SHA256>` over
  `webhook-id.webhook-timestamp.Body`. The key is the Base64-decoded part
  after `whsec_`. `X-Event-ID` and `X-Correlation-ID` stay. The ORCE receiver
  `/dcs-dispatch` checks the signature over the received bytes
  (`msg.req.rawBody`) with the secrets from `WEBHOOK_SECRET` (one per
  subscription, separated by spaces or commas; `timingSafeEqual`, at most
  300 s time skew, each `webhook-id` only one time). Otherwise it returns
  401, also without `WEBHOOK_SECRET` or without a raw body. The `settings.js`
  that the chart mounts supplies the raw body (image default plus a JSON
  parser with `verify` as `httpAdminMiddleware` and `httpNodeMiddleware`).
  Otherwise the editor parser under `httpAdminRoot` `/` reads each body
  before the http-in routes. The BDD scenarios deliver to a receiver in the
  behave process, which checks the signature with the secret from the 201
  response. The operator copies the secret from the 201 response into a
  Kubernetes secret and gives it to ORCE through `orce.extraEnv` with
  `valueFrom.secretKeyRef` as `WEBHOOK_SECRET`. The ORCE subflow
  "DCS Subscribe" subscribes with the bearer token from `DCS_TOKEN`
  (Integration Manager of the subscribing organization).
  DCS delivers a contract event only to subscriptions of organizations that
  can read the contract (F03). Template events go to all.
  Callback URLs are absolute http or https URLs without credentials.
  The dispatcher reaches neither loopback nor link-local addresses and
  follows no redirects.
  Pending callbacks and the delivery log (last 512 entries, GET /deliveries
  with an administration role, only the own organization) stay in memory.
  **Reason:** A subscription connects a third-party system to DCS events.
  Per ADR-29, this is an integration task. The HMAC signature proves to
  the receiver the origin and integrity of each delivery, without a
  transfer of the secret. The timestamp in the signature limits replays.
  Encrypted storage with an HSM-wrapped CEK satisfies DCS-NFR-SEC-06 and
  DCS-NFR-SEC-14. Standard Webhooks is a widely used format, and receivers
  have ready-made libraries for it.

- **Rule:** Contract states are a Postgres enum that DCS extends only
  additively (`ALTER TYPE … ADD VALUE`). Views that reference new values are
  in a separate migration that sorts later.
  **Reason:** Postgres does not permit the use of new enum values in the
  transaction that creates them. The enum already carries live columns
  (ADR-2).
- **Rule:** The `contracts_effective` views override the stored state with
  `EXPIRED` after `exp_date`, unless the contract is already terminal
  (TERMINATED/REJECTED/EXPIRED/WITHDRAWN/REVOKED).
  **Reason:** Auto-expiry over a state that is already final would be
  misleading.
- **Rule:** At start, DCS seeds deployment targets (ADR-25) from
  `CONTRACT_TARGETS_FILE`. DCS does not overwrite existing registry entries.
  **Reason:** A fresh installation needs targets before the first admin
  login. The next restart must not overrule an operator who reconfigures a
  target.
- **Rule:** Time and interval configuration is centralized in `base/conf`.
  Values that env can override (for example
  `DCS_SYNC_FAIL_RETRY_INTERVAL`, `DCS_ARCHIVE_EXPIRING_WINDOW_DAYS`,
  `DCS_API_RATE_LIMIT_PER_MINUTE`) always fall back to the default for
  values that do not parse or are not positive.
- **Rule:** The retention period of archived contracts comes from
  `DCS_ARCHIVE_RETENTION_YEARS` (DCS-FR-CSA-14). At archiving,
  `retention_until` = expiry date (without expiry date: archiving time) plus
  the period. Without a value there is no period: `retention_until` stays
  empty, and delete stays permitted. **Reason:** The SRS requires a
  configured retention policy without a fixed value, and delete only
  "if permitted by policy" (DCS-FR-CSA-17). A default would block each
  delete and the GDPR erasure (key shredding) until the end of the period.

## Identity (did:web, peer trust, machine identities)

- **Rule:** Each instance publishes a did:web document with three
  verification methods from the PKCS#11 token: the eIDAS/JAdES identity key
  (with x5c), the VC signing key (own method, own x5c) and the keyAgreement
  key (without x5c, because it never signs). **Reason:** A verifier must be
  able to tell the identity key from the VC key (ADR-19,
  ADR-ocmw-vc-signing).
- **Rule:** Peer trust has three layers: (1) the eIDAS certificate chain
  in the DID document, validated against the EU trust pool; (2) a
  challenge-response with the DID key per request; (3) the federation trust
  gate (agreement credential + policy endpoint, ADR-19). **Reason:** There
  is no common auth authority across independent operators, thus no shared
  token.
- **Rule:** DCS builds the EU trust pool from the LOTL and the national
  trusted lists (only CA/QC with status "granted") and updates it daily. A
  failed refresh keeps the last good pool. DCS checks the XAdES signature of
  the LOTL against the LOTL signing certificates published in the Official
  Journal (through the pivot LOTLs; `certs/lotl-oj.pem`, which
  `DCS_LOTL_OJ_CERTS_FILE` and `DCS_LOTL_OJ_URL` can override), and the
  signature of each national list against the certificates that the LOTL
  names for it. A list without a valid signature does not go into the pool.
  DCS never accepts self-signed certificates from the submitted chain as a
  trust anchor. DCS builds the pool only when `DCS_FORCE_EIDAS_CERT` is set.
  Without it, DCS does not do the chain check AND the QCStatements check;
  the hostname and JWK comparison stays. This is not an eIDAS validation.
  **Reason:** QCStatements are a self-declaration of the issuer. The check
  becomes legally sound only with the EU trusted lists as the anchor.
- **Rule:** did:web resolution follows the method specification exactly:
  bare authority → `/.well-known/did.json`, path segments → segments without
  `.well-known`. More than one instance can share a host. **Reason:** An
  incorrect path derivation would resolve each DID under a host to the same
  document.
- **Rule:** DCS resolves the keyAgreement key of a peer as the only
  keyAgreement of the foreign DID document (DCS rejects ambiguous
  documents). Locally, DCS resolves by label suffix, never by array
  position. **Reason:** Foreign HSM labels are unknown. A document that adds
  or reorders methods must still supply the correct key.
- **Rule:** DCS resolves machine identities from the registry at request
  time. `DCS_SYSTEM_CLIENTS` is only a declarative seed that DCS reconciles
  at each start (ADR-27). Roles of system clients come from configuration
  or registry, never from token claims.
- **Rule:** The IP lockout counts only failed AUTHENTICATION: a valid token
  without the required role is an authorization decision (403). DCS logs it
  as an authenticated access event, and it does not count toward the lock.
  Otherwise a user would lock themselves out at a legitimate role change
  from one IP (DCS-FR-UC-01-4). Attempt logging and lock maintenance are
  best-effort: a DB problem does not block the login flow.
- **Rule:** A deactivated system client registry entry resolves to
  "no machine caller" instead of to an error: the token falls through into
  the human path and is rejected there. Deactivation revokes an integration
  immediately, without a wait for the expiry of its secret.
- **Rule:** The background PDF regenerator authenticates with the
  in-cluster service credential (`SystemToken`). This credential carries
  exactly the scopes of the internally required endpoints and never leaves
  the cluster. The regenerator runs on NATS events without a user JWT, but
  must reach the internal PKCS#11 signing primitives (DCS-IR-HI-01).
- **Rule:** All outbound fetches of the identity layer (did.json of a peer,
  x5c chain links by URL) use a client with a 10-second timeout.
  **Reason:** `http.DefaultClient` has no timeout. A peer that accepts the
  connection and never responds would otherwise make the verification hang
  without limit.

## Signatures (HSM, JAdES, TSA)

- **Rule:** All private key operations go through the PKCS#11 token
  (`base/hsm`). There is intentionally no software fallback: an incorrect
  module, token or PIN configuration prevents the process from becoming
  healthy (DCS-NFR-SEC-02, ADR-1). Only the token open at start is retried
  instead of immediately fatal: the HSM provisioner runs as a post-install
  hook and can still be pending on a fresh installation. Meanwhile, a
  bootstrap server holds the listen address and returns 503 on `/readyz`.
- **Rule:** There are five key purposes with fixed default labels (dcs-did,
  dcs-vc, dcs-oid4vp-jar, dcs-c2pa, dcs-ecdh; each can be overridden with
  `DCS_HSM_KEY_*`). Rotation creates `-v<N>` labels next to the base label.
  Old versions stay in the token for the verification of historical
  signatures (`pki_active_key_version`, DCS-OR-C2PA-007).
- **Rule:** All keys are ECDSA P-256, all signatures SHA-256. crypto11
  supplies ASN.1 DER. JOSE (ES256) and COSE need the 64-byte r||s format.
  The conversion is central in `hsm.SignES256`.
- **Rule:** ECDH derivation (`CKM_ECDH1_DERIVE`, CKD_NULL) opens a separate
  PKCS#11 session next to the crypto11 signing context and never finalizes
  the Cryptoki state. **Reason:** crypto11 does not expose a derive API.
  The Cryptoki state is process-wide.
- **Rule:** At start, a wrap/unwrap self-test runs against the keyAgreement
  key of the own did.json. **Reason:** It proves that the published key and
  the HSM key match before the first artifact CEK comes into existence.
- **Rule:** DCS-to-DCS broadcasts are JAdES baseline-B (compact JWS, x5c in
  the protected header, critical `sigT`) over the JCS-canonicalized
  contract representation (RFC 8785). The receiver verifies the signature
  and the binding to the did:web key of the sender before acceptance
  (DCS-FR-SM-02).
  **Reason for JCS:** Each conformant JCS implementation in each language
  reproduces it bit-identically.
- **Rule:** Timestamps come as an RFC 3161 TSR through ORCE (no direct TSA
  contact). DCS verifies against the compiled-in TSA certificate
  (`certs/tsa.crt`, which `TSA_TRUST_CERT_FILE` can override; DCS reads it
  only after it loads the env configuration, not in the package init). TSA
  requests are idempotent hash-keyed GETs, and DCS retries them up to 3×
  with backoff. A provider change = change the ORCE flow + replace the
  certificate + rebuild.
- **Rule:** An ops job (`cmd/crlcheck`) materializes the certificate
  revocation of the (shared) dev signing certificate fleet-wide in
  `contract_signatures.cert_revoked_at`, not an HTTP endpoint per request
  (DCS-OR-C2PA-007).

Signature acceptance chain (ceremony → check → application), principle
ADR-20:

- **Rule:** A PAdES signature requires a verified signing ceremony: the
  wallet presents the PID directly through OID4VP (EUDIPLO is not a
  dependency). DCS verifies the presentation cryptographically against the
  nonce of the ceremony and the configured trust anchors before it persists
  the presentation. The persistence handler itself verifies nothing more.
  An expired ceremony (`ceremonyTTL`) never verifies. The retry is a fresh
  ceremony.
- **Rule:** A valid power of attorney is a hard precondition (UC-14,
  FR-SM-03) and must authorize the party that actually signs: the signature
  field carries the organization DID, and the PoA organization must match
  it.
- **Rule:** At each prepare, DCS pins the exact to-be-signed bytes (PDF +
  canonical JAdES payload) together with the finalize metadata. Submit
  validates against the committed bytes and does not derive them again. DCS
  also pins the signature level requirement of the field that the contract
  itself sets (`dcs:requiredCredentialType`, default AES). Submit gates on
  it, never on the caller parameter.
- **Rule:** A published signing request is single-use: the consumption
  marker (`consumed_at IS NULL` guard) commits in the same transaction as
  the finalize. Two concurrent callbacks can never both finalize.
- **Rule:** The EU DSS is an ADDITIONAL external AdES validator next to the
  internal checks. Configured but unreachable is an error, never a skip.
  AES acceptance intentionally does NOT require DSS TOTAL-PASSED:
  TOTAL-PASSED requires a qualified EU trust list CA (a QES property). AES
  needs only integrity and unique signatory binding (eIDAS Art. 26). DCS
  accepts an INDETERMINATE result whose only cause is the trust or POE
  status. DCS does not accept any crypto or integrity error. DCS records
  the subject and serial of the signing certificate on the ceremony record
  as sole-control evidence (DCS-FR-SM-26).
- **Rule:** The submission that completes the local signatures of a
  contract that carries a QES stores the PDF that DSS extended to
  PAdES-B-LTA, before the regeneration lock. A failed extension writes
  nothing and is answered as retryable (ADR-40).
- **Rule:** The verify endpoint reports the PDF signature check as
  `not_available` instead of "passed": the path does not cryptographically
  re-verify the PAdES CMS signature over its /ByteRange, and a check that
  did not run must never be reported as passed (DCS-OR-C2PA-006).

## Documents and provenance (artifact storage, encryption, erasure)

Principle decision on encryption and key shredding: ADR-28. The detail
decisions of the implementation:

- **Rule:** Before IPFS, DCS encrypts each artifact with a random CEK per
  scope (AES-256-GCM). The scope string is bound into each ciphertext as
  GCM AAD. Scopes: `contract:<IRI>` (the Art. 17 erasure unit: PDFs, archive
  snapshot, audit bodies), `template:<IRI>`, `instance:<DID>` (checkpoints,
  reports; not shreddable).
- **Rule:** At rest, the CEK exists only as a wrapped record in
  `content_encryption_keys` (ECDH-ES+A256KW to the keyAgreement P-256 key:
  ephemeral sender key pair → ECDH → Concat KDF → RFC 3394 key wrap).
  Wrapping needs only the public key (pure Go). Unwrapping goes through the
  HSM. Plaintext CEKs exist only in a limited in-memory cache (256
  entries), which DCS clears at shred.
- **Rule:** Shredding marks all live records of the scope as destroyed
  (`shredded_at/by/reason`). The record stays as evidence of destruction,
  and DCS never hard-deletes it. A shredded scope never again supplies a
  key, also not for writes, and peer ships do not revive it.
  **Reason:** DCS-NFR-COMP-03/SEC-13: erasure must be final and provable.
  Read paths report a `ShreddedError` (4xx, never 500).
- **Rule:** CEK insert and shred serialize through a Postgres advisory lock
  on the scope. The shred runs in the same transaction as the
  `KEY_SHREDDED` audit event. **Reason:** After a shred, a live record must
  never come into existence again. Destruction and audit evidence commit
  atomically.
- **Rule:** For federation, DCS wraps the CEK of a scope to the
  keyAgreement key of the peer, and the CEK travels in the ship payload. The
  receiver unwraps with its own HSM, re-wraps to its own key and persists
  idempotently (an existing own live record wins). The local recipient row
  records which foreign instances hold the CEK. This is the basis of the
  peer erase handshake.
- **Rule:** IPFS access uses two tracks: the tenant store (XFSC) plus a
  synchronously pinned Kubo/MFS copy. A read first tries the tenant one
  time, then immediately Kubo, and only after that the tenant retry with
  backoff. **Reason:** The tenant store is eventually consistent and loses
  DataIdentifier mappings transiently under load. The audit chain walk must
  not return 404 and must not run into the request deadline. The Kubo copy
  is content-addressed and identical, so the hash chain still verifies.
  `files/cp` collisions (the peer already copied the same CID) count as
  success after a CID comparison.

- **Rule:** DCS never again sends a PDF that already carries a PAdES
  signature through the C2PA lifecycle stamping: standard-conformant PAdES
  validators evaluate each incremental update to a referenced embedded file
  object after the signature as an unexplained modification, also if the
  update keeps the byte range. Thus DCS stamps the "active" state BEFORE
  signing (update-then-sign). From a C2PA state other than "draft" on, the
  PDF is frozen.
- **Rule:** DCS never generates contract PDFs on demand: each CWE
  transition that is relevant to content or lifecycle (incl. update)
  triggers the background regeneration, which extends the C2PA manifest
  chain.
- **Rule:** The bundle export (ZIP) reuses only existing retrieval paths
  and packages the locally known hierarchy family: the parent chain
  recursively upward under `parents/`, the other locally known family
  members flat under `related/`. The same party read scoping rule as a
  direct retrieval filters them (ADR-7: only what the requester could read
  individually anyway). Members that are not local are simply absent; DCS
  fetches nothing remotely. A structural integrity pre-flight (FR-PACM-06)
  refuses the export with a findings list (422) instead of supplying an
  incomplete ZIP. `bundle-manifest.json` indexes each entry with SHA-256.
- **Rule:** Archive deletion (DCS-FR-CSA-17) is a soft delete (the row
  stays as evidence) plus a hard disposal chain: DCS saves the snapshot
  CIDs before the soft delete (after it, there would be no evidence of
  which IPFS objects are affected). The snapshots leave the IPFS store.
  Then the CEK erasure runs locally plus the peer handshake. The erasure
  status can be queried (live/shredded locally, pending/confirmed per peer).

### Rendering and provenance embedding (pdf-core)

pdf-core is intentionally a simple, stateless, KEYLESS component (project
invariant): it never holds signing material and makes no domain
decisions. It renders, embeds and extracts.

- **Rule:** Rendering is deterministic: the same JSON-LD payload → a
  byte-identical PDF. For this, the render epoch is fixed
  (`CanonicalCompiledAt`; the trusted contract time is the PAdES-B-T
  timestamp, not the render epoch). The visible page comes only from the
  `@list`-ordered `documentStructure` of the payload, without a json-gold
  expand/compact round trip, which reorders multiple values
  non-deterministically. **Reason:** /verify proves "the human-readable
  document IS the render of its machine-readable payload" by recompile and
  byte comparison (DCS-OR-C2PA-002/-010).
- **Rule:** pdf-core embeds the payload VERBATIM: exactly the submitted
  bytes, never a re-canonicalized form. The FileID, the rendered backlink
  and the CIDv1 content address are sha256 over exactly these bytes.
  **Reason:** Only a byte commitment is reproducible by each verifier.
  URDNA2015 graph hashing is not suitable for the self-reference (blank
  node labeling is not byte-deterministic; the A and B instances calculated
  different hashes).
- **Rule:** C2PA signing has two steps, prepare/embed: the render signs
  with a CapturingSigner (it records each COSE Sig_structure and leaves
  zeroed 64-byte slots). The DCS backend signs the Sig_structures with the
  dcs-c2pa HSM key and posts them to the stateless `/c2pa/embed`, which only
  fills byte runs. pdf-core knows only the x5chain (public material) for the
  protected header. **Reason:** Key material stays fully in the
  backend/HSM (DCS-IR-HI-01). Embed depends on nothing but its inputs, and
  each replica can serve it. The ES256 signatures are the only
  non-deterministic part of a PDF. `ZeroCOSESignatures` masks them for all
  byte comparisons.
- **Rule:** Changes are amend instead of re-render: each content change is
  a PDF incremental update that keeps the original bytes unchanged as a
  prefix and supersedes objects (pages, payload, C2PA manifest).
  **Reason:** Existing C2PA hard bindings and a PAdES signature, if
  already attached (/ByteRange), stay verifiable over the original bytes.
  The C2PA manifest chain grows further with each hop. `/verify` deterministically
  re-applies the FULL hop history (base compile plus each update), not only
  the last step.
- **Rule:** Over a PAdES-signed PDF, an update is provenance-only: pdf-core
  never emits again the pages, the AcroForm and the signed field (with /V
  link). /Root stays on the catalog of the signer.
  **Reason:** A re-stamping would discard the /V of the signed field and
  invalidate the signature (DCS-OR-C2PA-010).
- **Rule:** Re-anchor exception (ADR-26): DCS attaches the PAdES signature
  AFTER the lifecycle manifest (thus the signature commits to the
  provenance). As a result, the whole-file binding of the manifest is
  shorter than the signed file. Thus `/render/reanchor` appends a
  provenance-only manifest with an unchanged payload. This is the only path
  that intentionally passes the no-changes guard. /verify identifies such a
  hop by the unchanged payload and replays it as a re-anchor.
- **Rule:** Interop decisions against c2pa-rs/c2patool: amendments are
  standard manifests (c2ma), never C2PA update manifests (c2um), whose
  parent binding can never match again after an append. Manifest labels
  come from the hard binding hash (not the payload hash), so that two
  amendments of the same payload do not create a cyclic ingredient chain.
  Remote manifest discovery is a separate `dcs.remote_manifests` assertion
  plus an XMP `dcterms:provenance` link, NOT a claim field, which c2pa-rs
  hard-rejects as unknown. The leaf certificate of the x5chain MUST carry an
  organizationName; otherwise all C2PA verifiers report
  claimSignature.mismatch. pdf-core makes this a startup configuration
  error.
- **Rule:** Tamper evidence in verify: an unsigned base must be
  byte-EQUAL to its recompile. Appended bytes without a dcs update marker
  are an offline amendment outside of /update, and pdf-core rejects them
  (409). In contrast, `/verify/content` checks ONLY the page contents
  against the recompile of the embedded payload and tolerates appended
  manifest and signature layers. This is the check that an instance applies
  to a PDF that it received from a peer and that is already amended.
- **Rule:** pdf-core monitors one invariant itself: no byte of a page
  content stream can ever be in a C2PA exclusion window (otherwise visible
  content would be without provenance). A violation is a compiler bug and
  causes a panic, or the verification explicitly fails.
- **Rule:** The asserting instance (DID) travels as a per-request header
  (`X-DCS-Lifecycle-Authority`) into each `dcs.lifecycle` assertion, and
  verify reads it back from the document itself. **Reason:** pdf-core is a
  stateless renderer that more than one instance can share. The recompile
  of a foreign document must reproduce the same bytes without prior
  knowledge.
- **Rule:** PDF/A-3a conformance is compiled in: an embedded font program
  (Liberation Sans), a synthetic sRGB ICC profile in the OutputIntent, /ID in
  each trailer (also incremental), listed associated files (/AF) for each
  attachment relationship, XMP with a pdfaid declaration and a PDF/A
  extension schema for dcterms.
- **Rule:** The JSON-LD canonicalization at the edge (`CanonicalizePayload`)
  explicitly compensates json-gold behavior: without an API-level base IRI,
  json-gold would silently discard relative `@id`s (payload hash over zero
  quads). `@container: @list`/`@type:@id` from the context do not apply
  with compact IRI notation, and the function applies them afterwards. DCS
  resolves the registered context in-process: canonicalization and SHACL
  validation do no HTTP fetch.

## Web client (frontend)

- **Rule:** Sessions use the Hydra access token in localStorage. At
  reload, the client rehydrates the identity (sub, roles, issuer) directly
  from the stored, unexpired token instead of through `/auth/refresh`.
  **Reason:** Hydra rotates refresh tokens single-use. A reload must not
  consume a refresh token only to restore the identity.
- **Rule:** A session is valid only with at least one mapped DCS role:
  roles come only from the JWT claims, and the client maps them to the
  UserRole vocabulary. A token without a mappable role does not create a
  user. The router gates routes declaratively through role metadata.
- **Rule:** The OID4VP login binds the Hydra login_challenge idempotently
  to the running presentation through a sessionStorage state machine
  (`hydra-login-guard`) (the challenge can arrive before or after the
  presentation state: pending latch). The client passes consent_challenge
  directly to the backend and then removes the challenge query from the
  URL. All Hydra/OIDC paths go through the origin proxy. The browser needs
  no direct Hydra address.
- **Rule:** The template and contract editor (`dcsDraftStore`) holds the
  document DIRECTLY in canonical JSON-LD (blocks, layout, ContractFields,
  contractData, ODRL rules as JSON-LD nodes). The stored document is a
  trivial assembly of this state, and there is no conversion layer.
  **Reason:** What the editor edits is exactly what the backend validation
  and pdf-core consume. There is no format drift between the UI model and
  the document.
- **Rule:** The client inserts components flatten-on-compose: it copies
  the blocks, placeholders and ODRL rules of an approved component with
  fresh `@id`s (all internal references consistently rewritten) into the
  target document. There is no reference block and no snapshot. The
  document stays self-contained (ADR-23), and two insertions of the same
  component never collide.
- **Rule:** A machine-readable rule never outlives its prose: each ODRL
  rule carries `dcs:prose` to the clause that carries it. Deleting the
  clause removes the rule, and DCS rejects a rule without binding clause
  text already at authorization. The client assembles the single
  enclosing `odrl:Offer` (ADR-6 canonical form).
- **Rule:** The client maps errors of the signature endpoints to user
  texts through the typed Goa error name (`name` field), never through
  string matching of the free `message`. The message is not a stable
  contract.
- **Rule:** The signing UI enforces the ADR-20 byte pinning coupling:
  prepare and submit travel with the same `ceremony_id`. The field to sign
  is the party slot of the OWN instance (identified through its did:web,
  not through the issuer of the logged-in user: that issuer names the
  organization of the signatory and never matches a party slot).

## Federation (DCS to DCS)

- **Rule:** The federation rule set (`federation/rules.md`) is compiled
  into the binary with `go:embed`. Each agreement credential names it by
  public policy URL and SHA-256 hash (ADR-19). **Reason:** Two instances of
  the same build embed byte-identical rules and calculate the same hash. A
  manipulated rule set, or a rule set with a different version, shows at
  the hash comparison.
- **Rule:** DCS builds the agreement credential one time at start and signs
  it itself (issuer = own instance DID, ecdsa-rdfc-2019). The DCS-specific
  terms are explicitly mapped in the JSON-LD context. **Reason:** Otherwise
  RDFC-1.0 canonicalization would silently drop unmapped terms, and the
  signature would not cover them.
- **Rule:** Reliable delivery of contract ships comes from the DB-backed
  sync-fail scheduler (`sync_fails` table, default 5 min), not from the
  NATS event bus. **Reason:** NATS delivers at-most-once. The
  reconciliation must run well within one negotiation round.
- **Rule:** DCS ships exactly the states OFFERED, NEGOTIATION, SIGNED and
  REVOKED (ADR-13; revocation must propagate immediately,
  DCS-NFR-BR-06). Internal states (DRAFT, SUBMITTED, REVIEWED, APPROVED,
  ACTIVE, TERMINATED) stay local. Review and approval never cross the
  boundary. The PDF is the wire format (JSON-LD + C2PA chain + signatures).
  A signed contract also carries the JAdES.
- **Rule:** An offer is a pure state transition without a new PDF (DRAFT
  and OFFERED both map to C2PA "draft"). Thus the offer event itself
  triggers the ship of the PDF that is already stored. If the contract is
  shippable but the PDF is not yet stored (asynchronous regeneration), DCS
  never silently discards the ship: DCS creates a `sync_fails` entry for
  the retry scheduler.
- **Rule:** Trust gate semantics in sync (ADR-19): DCS retries an
  agreement credential failure (layer 3a) through `sync_fails`, with
  incident dedup through the `gate_incident_recorded` latch of the row
  (exactly one incident per entry, independent of interleavings). A policy
  endpoint rejection (layer 3b) is terminal: no retry entry. DCS deletes
  existing retry entries atomically with the incident. Incidents dedup per
  (contract, peer, direction): offer and PDF_REGENERATED of the same offer
  fire milliseconds apart.
- **Rule:** Peer authentication in the ship is a body-level did:web
  challenge-response (random value, signed with the own DID key), not a
  JWT. **Reason:** There is no common end-user identity across
  independently operated instances.
- **Rule:** The receiver takes the shipped PDF EXACTLY as its copy (never
  regenerated), so that the C2PA provenance chain of the counterparty
  stays. DCS upserts the local copy from the JSON-LD that pdf-core
  extracts, and the copy goes into NEGOTIATION with its own local tasks
  (ADR-13: each instance has its own workflow and RBAC). The only exception
  to intrinsic state privacy: DCS takes a ship declared as REVOKED as a
  revocation. The revocation of the counterparty voids the agreement,
  independent of the local workflow progress.
- **Rule:** Peer erasure uses a separate `contract_erasures` queue with a
  retry scheduler (same mechanism as sync_fails, separate queue): a local
  shred with a KEY_SHREDDED event is a hard error. Peer requests that DCS
  cannot deliver stay pending. Party resolution runs before each
  destruction: DCS destroys nothing if the contract is not readable.

## Audit (tamper evidence, outbox, Merkle checkpoints)

Principle decision on checkpoints and external anchoring: ADR-16. The
detail decisions:

- **Rule:** Audit logging is a transactional outbox: DCS persists domain
  events in `outbox_events` in the business transaction. An asynchronous
  processor publishes them on NATS and anchors them separately in IPFS/TSA.
  Publishing (`published`, 100 ms interval) is intentionally decoupled from
  anchoring (`processed`, 1 s interval). **Reason:** Subscribers consume
  only the JSON payload, never an anchor value. Publishing must not wait
  behind the sequential, network-bound TSA/IPFS round trips.
- **Rule:** Per resource, entries form a strict hash chain (reference to
  the CID of the predecessor). Globally, one Merkle checkpoint commits per
  batch (RFC 6962 domain separation, root chain through `prev_root`,
  RFC 3161 timestamp on the root). **Reason:** The earlier global per-event
  chain forced each event through a sequential TSA+IPFS round trip, and one
  hanging event blocked the full trail.
- **Rule:** The TSA signature is not part of the checkpoint bytes. DCS
  stores a checkpoint without a timestamp and stamps it later.
  **Reason:** The root is immutable. The timestamp only attests "existed
  at this time at the latest". A TSA outage must not block the trail.
- **Rule:** Each entry separates a public header (component, event type,
  DID, timestamps, chain link, nonce) from the private body: `event_data` is
  encrypted under the CEK scope of the entry. A shred deletes the body
  without a move of a stored byte. The chain and the inclusion proofs still
  verify. Readers see the defined erased marker (`{"erased":true}`).
  Erasure records themselves (KEY_SHREDDED, DELETE_ARCHIVED) are in the
  instance scope, which is not shreddable, although they carry the contract
  DID: they must survive the erasure that they record.
- **Rule:** A 16-byte nonce blinds each Merkle leaf. **Reason:** Without
  salt, the leaf hash would be a commitment over highly guessable content.
  A published proof would permit brute-force confirmation of candidate
  entries.
- **Rule:** DCS re-verifies each written entry or checkpoint immediately
  through the read path before it becomes a chain link. DCS dead-letters
  events that are permanently not anchorable after 50 attempts (logged
  visibly; they are then absent from the trail). DCS publishes
  RETRIEVE_*/SEARCH_* events but never anchors them. **Reason:** Lookup
  traces would dominate the batches and starve real audit events. Audit
  reads filter them out anyway (`IsAuditVisibleEventType`).
- **Rule:** Trail reads run chain-parallel (independent resource chains
  side by side, limit 128; checkpoint leaves limit 32), and full-trail reads
  go back over a maximum of 500 checkpoints.
  **Reason:** O(events) sequential IPFS round trips exceed each request
  deadline as soon as the trail has history.

- **Rule:** Compliance monitoring knows four risk classes:
  MISSING_APPROVAL (contract in an approval-pending state with an open
  mandatory approval task), UNAUTHORIZED_ACCESS (persisted
  CONTRACT_ACCESS_DENIED artifact of the party read scoping),
  CONTRACT_UNDERPERFORMANCE (KPI value reported by the target system that
  violates the ODRL SLA constraint of the contract) and
  CONTRACT_DEPLOYMENT_FAILED (dispatch that never reached the target
  system). DCS deduplicates risks per (contract, actor) or (contract,
  approver). DCS anchors the sweep itself as an audit event, and each risk
  also on the PAC chain of the affected contract.
- **Rule:** The monitor REPORTS persisted verdicts and does not form a
  second verdict: the deployment callback evaluates KPI violations on
  arrival against the contract ODRL and persists the verdict. The deploy
  path marks deployment failures. **Reason:** A re-evaluation could
  diverge. For underperformance, the immutable inline contract values would
  never be violated anyway (a contract that violates them would never have
  been approved). Only reported actual values can raise an alarm.
  Monitoring does not block: the contract is in force, and the violation is
  a fact to record (in contrast to the approval gate, which refuses).

## Operations (startup, HTTP, configuration)

- **Rule:** Start order: migrations → Semantic Hub seed + anchor refresh →
  bootstrap server (503 on /readyz) → HSM open with retry → DID self-test →
  config attestation → workers/services → handover of the listen address to
  the real server. Helm intentionally runs without `--wait` (the HSM
  provisioner is a post-install hook).
- **Rule:** At start, DCS hashes the security-critical mounted
  configuration files (DID document, OID4VP trust data, x5c anchors),
  checks them against operator pins (`DCS_CONFIG_SHA256_PINS`) and writes
  them as an attestation into the audit outbox. A pin mismatch stops the
  start (DCS-NFR-SEC-04).
- **Rule:** `DCS_PUBLIC_URL` is mandatory and is the basis of each absolute
  IRI that produced documents carry (resource IRIs, `@context`,
  `sh:shapesGraph`, C2PA remote manifests). External consumers must be able
  to dereference these IRIs. Resource identity is uniformly
  `{DCS_PUBLIC_URL}/<kind>/<key>` (`base.ResourceIRI`). Identifiers that are
  already absolute (IRI, did:, urn:) pass unchanged.
- **Rule:** At start, DCS polls mandatory external dependencies (pdf-core,
  status list service) for up to 3 minutes instead of an immediate crash.
  **Reason:** Under CI resource pressure, dependencies are often only slow,
  not broken. A crash loop would cost more than the wait.
- **Rule:** did.json is mounted at the origin root (did:web well-known),
  outside of the configurable API prefix (`DCS_API_PATH`). ORCE webhooks
  are under `/orce/*` before the Goa mux. Frontend statics with an
  index.html fallback are under `DCS_UI_PATH`.
- **Rule:** Error mapping at the HTTP edge: DCS maps named Goa
  ServiceErrors to the correct status codes. DCS handles
  `BundleExportRefusedError` before the generic heuristic, so that the
  `findings` array in the response body stays (422).
- **Rule:** Migrations are embedded SQL files, sorted alphabetically, each
  in its own transaction, tracked in `schema_migrations`. There are no down
  migrations.
