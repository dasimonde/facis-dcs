# ADR-38: Recorded deviations of the pilot

Status: Accepted (2026-10-31).

## Context

Some SRS requirements are met in part or not at all by decision of the
product owner. Other ADRs record most of these decisions (ADR-17, ADR-21,
ADR-24, ADR-25, ADR-28, ADR-35, ADR-37). The decisions below have no other
ADR. The deviation register in `docs/TRACEABILITY_SRS_BDD.md` links here.

## Decisions

### 1. The instance signs the template provenance credential (DCS-FR-TR-09)

At registration, the DCS instance issues one W3C VC in JSON-LD
(ecdsa-rdfc-2019) per template version. The credential records the creator,
reviewer, approver and registrar claims, the content hash and a link to the
credential of the previous version
(`backend/internal/templaterepository/command/register.go`,
`IssueTemplateProvenanceVC` in `provenance.go`). The instance key signs it.
The role holders do not sign their own claims.

Reason (product-owner scope decision): each role holder acts after a
verified OID4VP login, and the audit trail records each act. A signature per
role holder needs a wallet signing ceremony for every review and approval
step. The pilot does not ask the holder for that ceremony; the instance
attests the claims that it recorded.

### 2. Signatures are applied in sequence, not in parallel (DCS-FR-SM-17)

Several signers sign the same contract, each in a separate PAdES signature,
and each signature is verifiable on its own. The signatures are applied one
after the other. A signature over a superseded revision is refused
(`assertLatestRevision`, `TestAssertLatestRevisionRefusesAStaleRevision`).

Reason (product-owner scope decision): each PAdES signature is an
incremental update whose byte range covers all earlier revisions. Two
signatures prepared over the same revision collide: the second one does not
cover the first one. Parallel signing needs a different container (for
example detached CAdES signatures per signer), which the pilot does not
provide.

Concurrent signatures on two instances are detected and refused, never
merged or overwritten. Each instance signs its own copy of the contract, so
two instances can each sign the same revision before either copy reaches the
other. The receiver of a peer's PDF (`PeerPdfReceiver`, under the same
per-contract lock as the local signing path) replaces its copy only if the
shipped PDF extends the signed revision that it holds
(`provenance.ExtendsSignedRevision`, also used by `assertLatestRevision`):

- An unsigned local copy accepts the peer's PDF.
- A shipped PDF that starts with the local signed revision replaces it.
- A shipped PDF that the local copy already contains changes nothing.
- A revocation is adopted; the local signed copy stays.
- Any other shipped PDF is refused with the `post_pdf` error `conflict`
  (HTTP 409). The receiver keeps its copy and records a
  `PAC_SIGNING_CONFLICT` audit event (direction `RECEIVED`).

The sender keeps its copy too. It records a `PAC_SIGNING_CONFLICT` event
(direction `SENT`) and gives the refused ship no retry entry, because a
retry gets the same answer. The compliance monitor reports each recorded
conflict as a `SIGNING_CONFLICT` risk on both instances
(`TestAPeerCopyThatDoesNotExtendTheLocalSignatureIsRefused`,
`TestASignedCopyThePeerRefusesAsAConflictIsRecordedAndNotRetried`,
`TestMonitorFlagsSigningConflictsOnBothSides`).

### 3. Hardware roots of trust and secure boot are platform scope (DCS-IR-HI-03, DCS-NFR-SEC-17)

The DCS does not seal service credentials to a TPM 2.0 or Secure Enclave, does
not do remote attestation, and does not check secure boot. Service keys are in
the PKCS#11 token (DCS-IR-HI-01, ADR-1). The DCS runs as containers on the
operator's Kubernetes nodes.

Reason (product-owner scope decision): TPM sealing, attestation and secure
boot are properties of the host firmware, the operating system and the
cluster. The operator who runs the nodes provides them; a container cannot
provide them for its host.

### 4. The EU trusted lists and the time-stamp authority are reached over TLS 1.2 or 1.3 (DCS-IR-CI-01)

Every outbound HTTPS client of the backend requires TLS 1.3, except two. The
RFC 3161 time-stamp client accepts TLS 1.2 and TLS 1.3 (`NewClient` in
`backend/internal/base/tsa/tsa.go`, `TestTSAClientAcceptsTLS12`); the TSA signs
every time-stamp token, and the backend verifies that signature against the
configured TSA certificate. The client that downloads the EU List of Trusted
Lists and the national trusted lists also accepts TLS 1.2 and TLS 1.3 and
verifies the server certificate (`tslClient` in
`backend/internal/base/identity/eutrustpool.go`,
`TestTrustedListClientAcceptsTLS12`). The backend also verifies the XAdES
enveloped signature of each list and reads only the signed content
(`verifyTSL` in `eutrustpool.go`, `verifyEnvelopedSignature` in
`backend/internal/base/identity/tslsignature.go`):

- The LOTL: against the LOTL signing certificates of the Official Journal
  publication C/2026/1944 (`backend/internal/base/identity/certs/lotl-oj.pem`;
  `DCS_LOTL_OJ_CERTS_FILE` and `DCS_LOTL_OJ_URL` replace them when a new
  publication appears). The backend follows the pivot LOTLs that the LOTL
  lists before that publication; each pivot names the signing certificates
  of the next LOTL (`TestFetchLOTLFollowsThePivotLOTLs`).
- Each national list: against the signing certificates that the LOTL names
  in its pointer to that list. A list whose signature does not verify is
  not used; the lists of the other member states stay in the trust pool
  (`TestBuildEUTrustPoolRejectsANationalListWithAWrongSignature`,
  `TestVerifyTSLAcceptsOnlyTheNamedSignerAndUntamperedContent`). With
  `INTEGRATION=1`, `TestEUTrustPoolLive` verifies the live LOTL and national
  lists.

Peer trust is decided by the federation trust gate (ADR-19: the agreement
credential and the trust policy the operator configures in ORCE) with the
peer issuers anchored per ADR-35. The EU trust pool is an additional eIDAS
check that `DCS_FORCE_EIDAS_CERT=true` (chart value `identity.eidas.enforce`,
off by default) turns on: the backend then builds the
pool at startup, checks the DID certificate of each peer and of this
instance against it (chain to a qualified trust service provider and the
QcCompliance statement), and needs outbound HTTPS to the EU trusted-list
servers.

Reason: not every time-stamp authority offers TLS 1.3. Each member state
publishes its national trusted list on its own server, and not all of these
servers offer TLS 1.3. With TLS 1.3 only, the
lists of these member states are not available, and the signer certificates
of their trust service providers are not trusted. The signature of each list
protects its content whatever the TLS version.

### 5. The CRL distribution point is an `http://` URL (DCS-IR-CI-01, DCS-NFR-SEC-01)

The certificates that the provisioning job issues name the CRL of the
instance in their `crlDistributionPoints` extension as an `http://` URL of
the backend Service (`CRL_URL` in
`deployment/helm/templates/hsm-provision-job.yaml`,
`scripts/c2pa-cert-provision.sh`).

Reason: RFC 5280 CRLs are signed by the issuing CA, and a relying party
verifies that signature. CRLs are served over plain HTTP so that revocation
checking does not depend on TLS: a check over HTTPS needs the certificate
of the CRL server, which can itself need a revocation check.

### 6. The internal TLS keys and the Hydra token keys are not in the PKCS#11 token (DCS-IR-HI-01)

The PKCS#11 token holds the keys that the DCS attests and protects with: the
DID identity key, the VC key, the OpenID4VP request key, the C2PA key and the
ECDH key ([key management concept](key-management-concept.md), §2). Two
groups of private keys are held outside the token:

- The internal TLS CA key and the one TLS server key that the in-cluster TLS
  endpoints share (backend, Postgres, NATS, Hydra, ORCE, pdf-core, DSS,
  Federated Catalogue). Helm generates them into the Kubernetes Secrets
  `<release>-internal-ca` and `<release>-internal-tls`
  (`deployment/helm/templates/internal-tls-secret.yaml`).
- The keys that Hydra signs access and ID tokens with. Hydra keeps them in
  its own database, encrypted with the `SECRETS_SYSTEM` value from a Secret
  (`deployment/helm/charts/hydra/templates/deployment.yaml`).

Reason: these keys belong to platform components. The TLS keys protect the
transport inside the cluster; the Hydra keys sign session tokens that the
backend checks against Hydra's JWKS. Hydra and the TLS endpoints read their
keys from files, Secrets or a database, not from a PKCS#11 token. Access to
the Secrets is controlled by Kubernetes RBAC.

### 7. Identity providers of other organizations take part as credential issuers (DCS-NFR-SEC-16)

A user logs in with an OpenID4VP presentation of a role credential, and the
DCS's own Hydra issues the session (`backend/internal/service/auth_login.go`).
The identity provider of an organization takes part as an issuer of that
credential, admitted per purpose and organization in the trust configuration
([ADR-31](adr-31-purpose-scoped-issuer-trust.md)). No external OIDC or SAML
identity provider is federated into Hydra.

Reason: product decision. Login is bound to a verifiable credential that the
wallet holder presents; an OIDC or SAML assertion carries no such credential.

### 8. A revoked login credential keeps its session until the access token expires (DCS-NFR-BR-06, DCS-IR-CI-09)

The backend checks each access token locally: signature, issuer, client and
expiry (`backend/internal/middleware/oidc.go`). It checks the status of the
login credential at login and at each token refresh. So after a revocation,
an access token issued before it stays valid until it expires: at most
5 minutes in the prod and acceptance profiles (`hydra.config.accessTokenTTL`),
and Hydra's default of 1 hour where the profile sets no lifetime.

Reason: a status-list request per API call would make every call depend on
the issuer's status list endpoint. The lifetime bounds the delay.

### 9. The database is PostgreSQL (DCS-OE-05, DCS-PC-03)

The backend keeps its replicas consistent with PostgreSQL functions:
transaction-level advisory locks (`pg_advisory_xact_lock`,
`pg_try_advisory_xact_lock`) keyed with `hashtext`
(`backend/internal/base/db/exclusive.go`), `LISTEN`/`NOTIFY` for the Semantic
Hub cache, `gen_random_uuid`, `jsonb` and full-text search. A database that
calls itself PostgreSQL-compatible but lacks one of them, for example
CockroachDB, is not supported. PostgreSQL 16 is the version the chart deploys
and the tests run on.

Reason: these functions are what keeps several backend replicas from running
the same job twice or forking a document. An abstraction over them would
have one implementation.

### 10. A locked client address refuses every access token from it (DCS-FR-UC-01-4)

"Multiple invalid Credential Attempts MUST block out the user." An access
token carries no identity the backend may trust before it has verified it, so
the backend counts invalid access tokens per client address
(`backend/internal/auth/jwt_auth.go`). After 5 invalid tokens from one address
within 15 minutes, the address is locked for 15 minutes, and every request
from it is refused, also one with a valid token. The BDD scenario "Locked
account cannot authenticate even with valid credential" checks this. Refused
login presentations lock the holder key of a credential that a trusted issuer
signed, or else the posting address.

Consequence: all users behind one address (a NAT, a corporate egress, an
ingress that does not see the client address) share the lock, and one of
them can lock out the others for 15 minutes. The operator:

- gives the ingress controller the real client address:
  `externalTrafficPolicy: Local` on its `LoadBalancer` or `NodePort`
  Service, or the PROXY protocol between the load balancer and the ingress;
- names the ingress controller in `DCS_TRUSTED_PROXIES`, so the backend reads
  the client address from `X-Forwarded-For` and no other pod can choose it;
- ends a lock with the admin unlock (**Admin > Lockouts**,
  `POST /admin/lockouts/unlock`, audited as `LOCKOUT_LIFTED`).

## Consequences

- A template user verifies the provenance credential against the instance
  key, not against a key of each role holder.
- A multi-signer contract takes one signing turn per signer.
- If two instances sign the same revision, each instance keeps its own
  signed copy and reports a `SIGNING_CONFLICT` risk. No copy holds both
  signatures.
- An operator who needs a hardware root of trust or secure boot provides it
  in the platform that runs the DCS.
- The time-stamp requests and the trusted-list download are the outbound
  HTTPS connections of the backend that can use TLS 1.2. The signature of
  each token and of each list protects its content.
- A change of the LOTL signing certificates is followed through the pivot
  LOTLs while the LOTL lists the pinned Official Journal publication. When
  it no longer lists it, the trust-pool refresh fails and keeps the last
  good pool; the operator then sets `DCS_LOTL_OJ_CERTS_FILE` and
  `DCS_LOTL_OJ_URL` to the current publication, or takes a release that
  pins it.
- A CRL fetch is plain HTTP; the CRL signature protects its content.
- Whoever can read the internal TLS Secrets of a release can read the
  in-cluster transport of that release. The DCS attestation keys stay in
  the token.
- A login credential of an organization comes from an issuer that the trust
  configuration admits for it, not from a federated identity provider.
- An operator who needs a shorter delay after a revocation sets a shorter
  access-token lifetime.
- An operator runs PostgreSQL, not another database that speaks its wire
  protocol.
