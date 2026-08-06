[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](../LICENSE)

# Digital Contracting Service

An automated orchestration workspace that deploys a [Digital Contracting Service](https://github.com/eclipse-xfsc/facis/tree/main/DCS) instance to a Kubernetes cluster.

---

## Overview

The Digital Contracting Service (DCS) provides an open-source platform for creating, signing, and managing contracts digitally.
It is designed for integration with the European Digital Identity Wallet (EUDI) ecosystem and targets eIDAS 2.0 Advanced Electronic Signatures. It is not certified, has not been conformance-tested against a production wallet or a qualified trust service provider, and claims no legal effect for the signatures it produces.

Key components:
- **Multi-Contract Signing** — multi-party contract execution within a single workflow
- **Automated Workflows** — contract generation, execution, and deployment
- **Lifecycle Management** — contract monitoring with renewal/expiration alerts
- **Signature Management** — signatures linked to verifiable digital identities, produced by the signatory's own wallet, never a DCS-held key (see "Contract signing" below)
- **Secure Archiving** — tamper-evident archive compliant with retention policies

---

## Helm Chart

The parent chart bundles `postgresql`, `keycloak`, `hydra`, `nats`, `neo4j`, and `federated-catalogue` as optional sub-charts, each toggled via `<subchart>.enabled`.

When sub-charts are disabled, point DCS to external services via:
- `serviceDiscovery.postgresqlHost`
- `serviceDiscovery.keycloakHost`
- `serviceDiscovery.natsHost`

Routing is configured with `route.basePath` (e.g. `/tenant-a/dcs`) or explicit `paths.api` / `paths.ui` overrides.

---

## Local Development

### Prerequisites
- [Rancher Desktop](https://rancherdesktop.io/) with Kubernetes enabled (provides `kubectl`, `helm`, and NodePort forwarding to `localhost`)
- Go with [air](https://github.com/air-verse/air) (`go install github.com/air-verse/air@latest`)
- Node.js 20+
- Python 3.10+
- Goa **v3** – Installation: Follow the instructions on [Goa Quickstart](https://goa.design/docs/1-goa/quickstart/)


#### Initialize all dependencies
Run the following command in **backend** to initialize all needed dependencies:
```bash
go mod tidy
```

#### Generate Go code with Goa
Generate the required glue code under `gen/` with the Goa CLI:
```bash
goa gen digital-contracting-service/design
```

### Recommended: One-command full stack startup

From the project root:

```bash
bash dev-stack.sh
```

What this command does:
1. Runs Helm dependency update and upgrade using `deployment/helm/values.dev.yml`
2. Creates `backend/.env` from `backend/.env.dev1` if missing
3. Provisions a local SoftHSM2 token with the five DCS keys and issues the
   C2PA/PAdES x5chains the backend signs and publishes under
   (`scripts/hsm-provision.sh`, `scripts/c2pa-cert-provision.sh`)
4. Starts frontend Vite dev server
5. Starts backend with air hot reload

Stop everything with `Ctrl+C` in the same terminal.

### Manual startup (equivalent steps)

Use this if you prefer separate terminals or step-by-step debugging.

#### 1. Deploy dependencies

```bash
helm dependency update ./deployment/helm

# First setup
helm install dcs ./deployment/helm -f ./deployment/helm/values.dev.yml

# Upgrade current installation
helm upgrade dcs ./deployment/helm -f ./deployment/helm/values.dev.yml

# For uninstalling the installation
`helm uninstall dcs`
```

The dev values run the backend natively (replicaCount 0), so the PKCS#11 token
is provisioned locally by `dev-stack.sh` rather than in-cluster.

This starts all dependencies as NodePort services forwarded to `localhost`:

| Service              | Address                          |
|----------------------|----------------------------------|
| PostgreSQL           | `localhost:30432`                |
| Keycloak             | `http://localhost:30080`         |
| Hydra (public OIDC)  | `http://localhost:30444`         |
| Hydra (admin API)    | `http://localhost:30085`         |
| NATS                 | `nats://localhost:30422`         |
| Neo4j HTTP           | `http://localhost:30474`         |
| Neo4j Bolt           | `bolt://localhost:30687`         |
| Federated Catalogue  | `http://localhost:30081`         |
| IPFS Document Manager | `http://localhost:30481`        |
| IPFS Kubo RPC        | `http://localhost:30501`         |

The Keycloak `gaia-x` realm is imported automatically on first start.

> To upgrade after chart changes: `helm upgrade dcs ./deployment/helm -f ./deployment/helm/values.dev.yml`

#### 2. Prepare backend runtime config and PKCS#11 token

```bash
cp backend/.env.dev1 backend/.env
bash scripts/hsm-provision.sh "$HOME/.dcs/softhsm-8991" dcs 1234 12345678
```

This provisions the SoftHSM2 token with the five DCS keys; the backend opens it
via the `PKCS11_*` / `SOFTHSM2_CONF` variables in `.env`. `dev-stack.sh` performs
this step automatically.

#### 3. Run backend and frontend

Terminal 1:

```bash
cd backend && air
```

Terminal 2:

```bash
cd frontend/ClientApp
npm install
npm run dev
```

The backend listens on `http://localhost:8991`.

The Vite dev server starts at `http://localhost:5173` and proxies `/api` requests to the backend automatically.

### 4. Sign in with the demo wallet

```bash
python3 testWallet/demo_wallet.py
```

---

## BDD Tests

BDD scenarios live in `features/` at the project root. Tests are run against a full stack in an ephemeral [kind](https://kind.sigs.k8s.io/) cluster.

### Prerequisites
- `kind` — `go install sigs.k8s.io/kind@v0.23.0` or see [kind releases](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- `kubectl` and `helm`
- Docker (to build the DCS image)
- Python 3.10+

### Run locally

These steps are primarily for working on tests. Start the deployment, run the tests as often as you want, stop the deployment.
```bash
# 1. Start the environment
make -C tests/bdd kind_up

# 2. Run tests
# All
make -C tests/bdd run_bdd_kind_once
# File/Folder
make -C tests/bdd run_bdd_kind_once F=features/<PATH>

# 3. Stop the environment and reset
make -C tests/bdd kind_down
```

These step is for deploy and auto-run all tests a single time
```bash
# Build DCS image, spin up kind cluster, deploy via Helm, run all scenarios
make -C tests/bdd run_bdd_kind_ci
```

This single command:
1. Builds the DCS Docker image (`digital-contracting-service:bdd`)
2. Creates a kind cluster named `dcs-bdd`
3. Loads the image into the cluster
4. Deploys the full stack via `deployment/helm` with `values.bdd.yml`
5. Port-forwards DCS and Keycloak into the cluster network
6. Runs all `features/**/*.feature` scenarios with behave

Tear down the cluster afterwards:
```bash
make -C tests/bdd kind_delete
```

### Run against an already-deployed Helm release

If you have a release running (e.g. via Rancher Desktop):

```bash
make -C tests/bdd run_bdd_helm_dev \
  K8S_NAMESPACE=default \
  HELM_RELEASE=dcs
```

### CI

The `bdd-kind.yml` GitHub Actions workflow runs:

```yaml
make -C tests/bdd run_bdd_kind_ci
```

JUnit reports are published as check annotations and uploaded as workflow artifacts.

---

## Deploying an instance (`helm/deploy.sh`)

```bash
./helm/deploy.sh --values /path/to/values.my-instance.yml \
                 --namespace dcs --release dcs \
                 --public-url https://dcs.example.org
```

Repeatable `--values` files layer over the chart's `values.yaml`. Keep them
**outside this repository**: they carry hostnames, DIDs and credentials that are
specific to one deployment, and nothing environment-specific belongs in the
chart. `--dry-run` validates values, templates and API acceptance without
changing anything; `--release` matters because subchart service names derive from
it, so values referring to `http://<release>-orce:1880` must agree with it.

Re-running converges (`helm upgrade --install`), so the script is the same for a
first install and an update.

### Why a script rather than a GitOps sync

Three properties of this chart make an unattended reconciler a poor fit, all of
them observed on live deployments:

**The FC realm hook must run before the app can become healthy.** Realm
provisioning is a `post-install,post-upgrade` Helm hook. A reconciler that maps
Helm hooks onto its own phases typically runs `post-*` only after the sync is
*healthy* — but the backend treats a failed Federated Catalogue schema sync as
fatal, so it crash-loops until that hook grants its service account the
`SCHEMA_*` roles. The hook that unblocks the app never runs, because the app is
never healthy. Plain `helm` does not gate post-install hooks on pod health, so
the ordering resolves itself.

**Some of the chart's own components patch what the chart renders.** The realm
hook writes the FC issuer URL into `fc-service` after the manifests are applied.
A reconciler comparing live state against rendered state sees permanent drift
there and may fight it.

**Provisioning is a first-boot side effect, not declarative state.** Key
material, `did.json` and the C2PA x5chain are generated once into a volume. A
reconciler has no way to know that re-running provisioning is safe only because
the scripts are idempotent.

None of this argues against GitOps in general. It argues for treating a DCS
install as a sequenced operation, which is what the script does.

### What it checks, and why those checks exist

Each check corresponds to a way an instance can look healthy while being unable
to do its job. All were observed on a running deployment that reported no errors:

| Check | Why it is not obvious |
|---|---|
| Backend logged `HTTP server listening` | A pod is `Running` before it clears its startup gates, because key material arrives from a hook *after* scheduling. |
| No `failed to sync federated catalogue` in the log | The backend treats this as fatal and restarts, so the symptom is a restart count rather than an error surfaced anywhere. |
| `DCS_TRUST_PDP_URL` set **and** answering 2xx | The ADR-19 trust gate is fail-closed. Unset or non-2xx means federation is entirely off — nothing ships, nothing is accepted — and the instance reports nothing unusual. A reachable URL is not enough; the endpoint must actually answer. |
| `DSS_URL` set | Without a validator the instance refuses every externally produced signature. Contract creation still works, so this surfaces only when someone tries to sign. |
| All three `/.well-known` documents resolve **from outside** | A peer resolves this instance by appending `/.well-known/did.json` to the bare hostname. Anything else claiming that prefix on the ingress — Hydra's OIDC discovery, notably — shadows them. Everything looks correct in-cluster; only an external fetch shows the 404. |
| Served DID matches `ISSUER_DID` | A mismatch makes peers reject the instance, with the rejection visible only on the far side. |

The external checks need `--public-url`; without it the script says so rather
than passing silently.

### Conditions worth knowing before you run it

- **The namespace must already exist.** The script does not create it, so it
  works with credentials that cannot see cluster-scoped objects. For the same
  reason it probes reachability with `kubectl version`, not `kubectl get
  namespace`.
- **Restricted RBAC.** If the installer may not create `roles`/`rolebindings`,
  set `pkcs11.provisioning.publishSecrets=false`. The provisioning hook then
  writes `did.json` to the shared token volume instead of publishing a Secret,
  and needs no API access at all. The ORCE TSA
  hook still publishes its own Secret, so also set
  `orce.localTSA.autoProvision=false` and pre-create that Secret — its key and
  certificate are self-signed and depend on nothing in-cluster.
- **Constrained namespaces.** A full instance is roughly ten workloads and ten
  Services. Under a `ResourceQuota` check `limits.cpu` in particular: the chart's
  default container limits are generous, and a namespace capped at a couple of
  CPUs fits only a couple of containers, which shows up as Deployments that exist
  with zero replicas rather than as an install error. A `services` cap also
  starves cert-manager's ACME HTTP-01 solver, which needs one of its own, so TLS
  silently never issues. Either raise the quota or set explicit small
  `resources.limits` and consume shared services from another instance.
- **Editing a subchart requires repackaging.** Helm prefers a packaged
  `charts/*.tgz` over the source directory, so a stale archive deploys old
  templates. The script runs `helm dependency update` every time for this reason;
  remember it when rendering by hand.
- **Federation needs both sides configured, and lockstep builds.** Each instance
  consults only its own policy endpoint. Both must also publish
  `/.well-known/dcs-agreement-credential.json`, and the agreement compares a hash
  of the embedded federation rules — so two instances federate only when running
  the same build.

### Adopting an instance that was deployed another way

If resources were created by something other than this script — a reconciler, or
an earlier manual install — there is no Helm release to upgrade, and running the
script will attempt a fresh install that collides with the existing objects.
Either let `helm upgrade --install` adopt them deliberately, or uninstall and
reinstall in a maintenance window. Plan this rather than discovering it.

## Credential issuers

A `dcs` release contains no credential issuer. The two credentials a signatory
presents — the Power of Attorney that authorizes them to sign for their
organization, and the person identification credential (PID) that says who they
are — come from OID4VCI issuers deployed as **separate Helm releases** of the
`orce` subchart, each with its own volume, its own key and its own DID. The
chart ships example values for all three:

| Release | Values files | Issues | Installed |
|---|---|---|---|
| `dcs-issuer` | `values.issuer-base.yml` + `values.issuer.yml` | Power of Attorney (`urn:dcs:poa:v1`) | beside the first DCS instance, under its hostname |
| `dcs-issuer` | `values.issuer-base.yml` + `values.issuer2.yml` | the same, for the second instance | beside the second DCS instance, in its namespace |
| `dcs-issuer` | `values.issuer-base.yml` + `values.issuer.dev.yml` | the same, for `dev-stack.sh` | by `dev-stack.sh`, on a NodePort |
| `dcs-issuer` | `values.issuer-base.yml` + `values.issuer.bdd.yml` | the same, for the kind BDD stack | by `tests/bdd` `kind_deploy` |
| `dcs-pid-issuer` | `values.pid-issuer.yml` | demo PID (`urn:dcs:pid:demo:v1`) | **once** for the whole deployment |

`values.issuer-base.yml` is the half that is the same everywhere — the flow set,
the `/admin` endpoint, the volume — and is always layered first; the second file
says only how that cluster reaches the issuer. Development, CI and production
therefore install the same chart under the same release name with the same base,
which is the point: a stack that differs from production cannot test it.

`flowsDir` selects which flow set a release runs (`flows-issuer` /
`flows-pid-issuer`, both under `charts/orce/`); a release serves one set, not
both, because they claim overlapping routes.

### The status list

Each issuer serves and signs the status list its own credentials name (ADR-34):
`<issuer base>/status-list/1`, ES256 over `typ: statuslist+jwt`, `iss` its base
URL, and its `x5c` chain in the header. `<issuer base>/admin` revokes and
un-revokes an index, which is what makes a revocation testable — with no
unsigned fallback, a set bit and a list that failed to load both refuse the
credential, and only one of them proves anything.

Two properties decide whether a verifier can use it, and both fail silently:

- The served token's `sub` must equal the URI the credential names. The issuer
  builds it from the request's `Host` and `X-Forwarded-Prefix`, so an ingress
  that routes `/issuer` without announcing the prefix produces a `sub` no
  credential mentions, and every credential naming that list is refused unread.
- The leaf must NAME the issuer — a chain proves the anchor vouched for the
  certificate, not whose it is. The public URL is only knowable from a request,
  so the leaf is minted on the first one that arrives and re-minted whenever the
  URL changes. Nothing mints one at boot: a SAN-less leaf chains perfectly and
  verifies nowhere.

### Anchors: production collects them, dev and CI are handed them

A production issuer generates its root on its own volume, so its fingerprint
cannot be known before it has booted. `tmp/redeploy/build-x5c-anchors.py` reads
each root back out of the `x5c` header of the list its issuer is actually
serving, refuses anything that is not a self-signed CA, and de-duplicates by
fingerprint. Re-run it after anything that re-mints a root — a wiped volume, a
fresh install — or verification fails closed against a certificate that no
longer signs anything.

Development and CI cannot work that way: both stacks are rebuilt from nothing,
and the backend reads its anchors before any issuer has booted. So the
relationship is inverted. `scripts/orce-dev-root-ca.py` mints one root ONCE, into
`deployment/helm/charts/orce/pki-dev/` and into the committed bundle
`backend/config/oid4vp/x5c-trust-anchors.dev.pem`; the issuer is handed that root
(`pkiRootCA.devFixture: true`) instead of generating one. Nothing runs at stack
start, so no version-controlled file is rewritten by bringing a stack up, and
every run anchors the same fingerprint. Its private half is in the repository,
which is why the backend refuses the bundle unless `DCS_ALLOW_DEV_TRUST` says
this is a development stack.

What does run at stack start is a check, not a rebuild: `make -C testWallet
check-status-list` (dev-stack.sh, and `tests/bdd/scripts/check_status_list.py` in
CI) fetches the served list and refuses to continue unless its chain ends at a
root the committed bundle holds — compared by SHA-256, never by subject — and its
leaf names the issuer. Without that check the mismatch first appears as a refused
login with the list unread, which is indistinguishable from a revoked credential.

### Why the PID issuer is its own release, and only one of them

A PID attests who a natural person is, and the value of that attestation is
that somebody other than the relying party makes it. A DCS that issued the
identity credential it later accepts as proof of its signatory has attested
nothing. So the PID issuer is a third party to *every* DCS instance: never part
of a `dcs` release, and installed once for the whole demo the way a national or
QTSP issuer exists once for many relying parties. Each DCS trusts it for `pid`
and for nothing else — an identity document must not grant access.

The Power of Attorney issuer is the opposite case: it speaks *for* one
organization about who may sign on its behalf, so each instance runs its own.

The demo PID issuer proofs nobody. Its credential is not an EUDI PID and must
not be presented as one.

### What you must supply

The repo gives you the shape of an issuer, never an identity:

- **Your own hostnames.** Every host in the example values is a placeholder
  (`dcs.example.org`, `dcs-b.example.org`, `pid.example.org`), as are the
  ingress class and the empty `tls` list. A credential issuer is published under
  the hostname of the DCS instance it serves, so its DID resolves as
  `did:web:<host>:issuer`.
- **Your own keys and DIDs.** Each issuer generates its root CA and signing key
  into its own volume on first boot and publishes them under its path prefix at
  `pki/root-ca.pem`, `pki/jwks.json` and `.well-known/did.json`. No key material
  is in this repository, and a clone is therefore not deployable as-is.
- **Your own trust document per DCS instance** (below). Nothing derives it
  automatically: it names issuers by identifier, which only exist once the
  issuers are running.

### Order and commands

Issuers first — the trust document a `dcs` release mounts quotes each issuer's
published identifier and key, which do not exist until the issuer has booted.

```bash
# 1. the issuer beside each DCS instance (in that instance's namespace/cluster)
helm install dcs-issuer ./deployment/helm/charts/orce \
  -n <namespace> \
  -f ./deployment/helm/values.issuer-base.yml \
  -f ./deployment/helm/values.issuer.yml

# 2. the PID issuer — once, wherever it is convenient to publish it
helm install dcs-pid-issuer ./deployment/helm/charts/orce \
  -n <namespace> -f ./deployment/helm/values.pid-issuer.yml

# 3. read back what they published (per issuer)
curl https://<host>/issuer/.well-known/did.json
curl https://<host>/issuer/pki/jwks.json
curl https://<host>/issuer/pki/root-ca.pem
curl https://<pid-host>/pid-issuer/pki/root-ca.pem

# 4. the status list, through the URL a credential will name. This request is
#    also what mints the leaf for that URL, so do it before collecting anchors.
#    Check `sub` is the URL you asked for and the leaf's SAN carries `iss`:
curl -H 'Accept: application/statuslist+jwt' https://<host>/issuer/status-list/1
```

Collect the root CA of **every** issuer this instance verifies — its own login
issuer as well as the PID issuer — into one PEM bundle. Both sign with a chain:
the PID's is on the credential, the login issuer's is on the status list a login
credential names, and a status list is only believed if it is signed and that
signature verifies.

Identify these roots by fingerprint, never by subject. Each issuer generates its
own and they all carry `CN = FACIS Demo Root CA` while holding different keys,
so a bundle assembled by name anchors one issuer and silently refuses the rest —
with every log line naming the same CA.

```bash
openssl x509 -in <each root> -noout -fingerprint -sha256 -subject
```

Then write each DCS instance's trust document, create it as a ConfigMap, and
only afterwards install or upgrade the `dcs` release pointing at it:

```bash
kubectl create configmap dcs-oid4vp-trust -n <namespace> --from-file=trust.json=<your file>
```

```yaml
oid4vp:
  trust:
    existingConfigMap: dcs-oid4vp-trust
    # every x5c chain — the PID credential's and the login issuer's status list —
    # is verified against this bundle; mount it via the chart's
    # `volumes`/`volumeMounts` values and point here
    x5cAnchorsPath: /etc/dcs/oid4vp-x5c/root-ca.pem
```

### The trust document

Trust is granted per purpose (ADR-31): `login` may grant a session here, `peer`
lets a credential be verified in a signing ceremony, `pid` may attest a natural
person. An issuer is listed under the identifier it puts in its credentials'
`iss`, which for both issuer flows is their `did:web:` identifier.

Every issuer here declares `mechanism: x5c`, and both of them sign that way: the
chain is on the credential, verified against the bundle above, and the leaf
carries the issuer's DID as a URI SAN so it can only speak for that issuer. This
is the same chain to the same anchor that the issuer's status list is signed
with, which is the point — a credential believed by certificate whose revocation
status is believed by a bundled key has no relationship between the two, and the
status list is where that relationship matters most (ADR-34).

The mechanism is authoritative, not the credential: a credential arriving with a
chain from an issuer configured as `jwks` or `did:web` is refused, and so is one
arriving without a chain from an issuer configured as `x5c`. So this is not a
free choice per issuer — it has to match how the issuer signs.

The status list needs no entry of its own. Its `iss` is the issuer's base URL
rather than its DID, but an `x5c`-signed list resolves through the chain and the
anchors, never through this document.

```json
{
  "vcts": ["urn:dcs:poa:v1", "urn:dcs:pid:demo:v1"],
  "peer_dynamic": false,
  "issuers": {
    "did:web:dcs.example.org:issuer": {
      "purposes": ["login", "peer"],
      "organizations": ["did:web:dcs.example.org"],
      "mechanism": "x5c"
    },
    "did:web:pid.example.org:pid-issuer": { "purposes": ["pid"], "mechanism": "x5c" }
  }
}
```

An instance grants `login`/`peer` to its **own** credential issuer: `peer` is
what a signing ceremony checks the local signatory's Power of Attorney against,
so granting it to another party's issuer would let that party authorize itself
off a document it publishes about itself. Whether a counterparty is dealt with
at all is the ADR-19 trust gate's decision, not a purpose granted here. The PID
issuer appears in both instances' documents, with `pid` only.

Leaving `existingConfigMap` unset makes the release fall back to the dev fixture
baked into the image (`backend/config/oid4vp/trust.dev.json`), whose issuer keys
are committed to this repository. The chart refuses to render in that case
unless the release also declares `DCS_ALLOW_DEV_TRUST=true`, which belongs to a
dev or CI stack only.

---

## Production Deployment

Backup and restore procedures — including how backup retention interacts with GDPR key-shredding erasure — are specified in the [backup integration guide](../docs/backup-integration-guide.md).

### Signing keys (PKCS#11) and the C2PA x5chain

Every DCS private key lives in a PKCS#11 token (DCS-IR-HI-01). For dev, staging
and CI the chart co-deploys a SoftHSM2 software token and provisions it in-cluster
(`pkcs11.provisioning.enabled=true`): a hook Job runs `scripts/hsm-provision.sh`
(token + five ECDSA P-256 keys) and `scripts/c2pa-cert-provision.sh` (the C2PA
x5chain bound to the `dcs-c2pa` key), leaving the chain on the shared token
volume. The backend waits for the token via an initContainer, then opens it
using `PKCS11_MODULE_PATH` / `PKCS11_TOKEN_LABEL` / `PKCS11_PIN`, and reads the
chain from `signing.issuerX5ChainPath`. pdf-core is configured with no signing
material: the backend sends the chain with each render request in the
`X-DCS-C2PA-X5Chain` header.

SoftHSM2 is a software token and is NOT a production HSM. For production set
`pkcs11.provisioning.enabled=false` and point `pkcs11` at a real external PKCS#11
module whose token already holds the keys:

```yaml
pkcs11:
  modulePath: /usr/lib/<vendor>/libpkcs11.so
  tokenLabel: dcs
  pinSecretRef:
    name: dcs-hsm-pin
    key: PKCS11_PIN
  provisioning:
    enabled: false
```

### Contract signing: PID trust anchors and QES trusted-list

The DCS holds no contract-signing key (ADR-12/ADR-20): a signature is produced
by the signatory's own wallet, and the DCS's job is to verify what comes back.
Two trust configurations follow from that:

- **PID issuer trust anchors** (`OID4VP_TRUST_DATA_PATH`, a JSON file shaped
  like `backend/config/oid4vp/trust.dev.json`): the DID/URL-keyed issuer
  public keys the backend accepts a PID (and Power of Attorney) credential
  from. **Dev/CI only** lists that stack's own ORCE credential issuer
  (`http://localhost:18080/issuer` for CI, `http://localhost:30181` for dev),
  pinned to the leaf key it is handed as a fixture and granted `pid` as well —
  which is what `testWallet/scripts/issue_pid_credentials.py` self-signs as,
  because a status list is believed only from the issuer that publishes it.
  Self-issued PIDs are a dev-edge substitution for the broken remote EUDIPLO
  PID service and must **never** appear in a production trust store, nor may a
  production deployment grant `pid` to an issuer it runs itself. A production deployment
  points this file at the real PID issuer registry's public keys instead —
  swapping the file is the entire change; the verification code
  (`oid4vp.Verifier.VerifyPID`) is identical either way.
- **QES trusted list**: a contract that requires QES for a signature field
  (`dcs:requiredCredentialType: "QES"` on its `dcs:SignatureField`, ADR-20) is
  only accepted when DSS's validation reports a `QESIG` qualification — a
  qualified certificate chaining to an EU trusted-list (LOTL/TL) CA. The DSS
  instance (`deployment/helm/charts/dss`) validates against whatever trusted
  list it is configured with; a production deployment points it at the real
  EU LOTL. **This chart does not yet provision a mock/custom trusted list for
  CI/dev QES testing** — that is tracked in ADR-20 §5 as a remaining
  CI-provisioning item, not a gap in the acceptance gate itself (the gate
  rejects a non-qualified signature regardless of what trusted list DSS runs
  against).
- **Per-contract signature-level requirement**: set on the contract's
  `dcs:SignatureField` node at authoring time (`dcs:requiredCredentialType`,
  `AES` or `QES`, default `AES` when absent) — no deployment-level
  configuration; it's contract content, enforced per field at prepare and
  submit.

### Hydra
- Enable `hydra.enabled` and set `hydra.config.selfIssuerURL` to the public issuer URL
- Register `dcs-client` redirect URIs via `hydra.clients` (see `values.dev.yml`):
  - **Valid Redirect URIs**: `https://<domain>/<path>/api/auth/callback`
  - **Valid Post Logout Redirect URIs**: `https://<domain>/<path>/api/auth/logout-complete`

### Keycloak (Federated Catalogue only)
- FC integration uses `fcKeycloak.realmURL` / `FC_KEYCLOAK_REALM_URL`

### TLS
- Use certificates from a trusted Certificate Authority
- Recommend [cert-manager](https://cert-manager.io/) for automatic renewal

### Values
Override the following at minimum:

```yaml
hydra:
  enabled: true
  config:
    selfIssuerURL: "https://hydra.example.com"
  clients:
    - client_id: dcs-client
      client_secret: "<secret>"
      redirect_uris: ["https://example.com/dcs/api/auth/callback"]
      post_logout_redirect_uris: ["https://example.com/dcs/api/auth/logout-complete"]

fcKeycloak:
  realmURL: "https://keycloak.example.com/realms/gaia-x"

route:
  basePath: "/dcs"
```

---

## License

Apache License 2.0. See [LICENSE](../LICENSE).

## TSA (Timestamp Authority)

DCS uses an RFC 3161 Timestamp Authority to cryptographically prove that a document or contract existed at a specific point in time. The timestamp is unforgeable and independent of DCS itself.

- Timestamps are requested via ORCE, which forwards requests to the upstream TSA provider
- The TSR (timestamp response) is verified by DCS using the TSA's CA certificate embedded in the binary (`backend/internal/base/tsa/certs/tsa.crt`)
- Stored TSRs can be re-verified at any time against the original data to prove it has not been altered

### Switching TSA providers

1. Update the TSA flow in ORCE to point to the new provider
2. Replace `backend/internal/base/tsa/certs/tsa.crt` with the new provider's CA certificate (PEM format) and rebuild the backend
