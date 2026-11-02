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
- `kind` — `go install sigs.k8s.io/kind@v0.30.0` (its default CNI enforces NetworkPolicy; `tests/bdd/kind-config.yaml` pins the node image) or see [kind releases](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
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

Archive retention (DCS-FR-CSA-14, DCS-FR-CSA-19): `archive.retentionYears` sets `DCS_ARCHIVE_RETENTION_YEARS` (10 in `values.prod.yml` and `values.acceptance.yml`; 0, the default, sets no retention policy). An archive entry cannot be deleted before its retention ends.

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

The acceptance profile runs its 2 backend pods on one node (required `podAffinity` in `values.acceptance.yml`), because the SoftHSM2 token volume is ReadWriteOnce; prod uses a real HSM and has no such volume.

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
- Create a Secret with a high-entropy system secret (`openssl rand -hex 32`, key `secretsSystem`) and set `hydra.secrets.systemSecretRef.name`. Outside dev mode the chart refuses to render without it.
- Persistent state (DCS-NFR-PER-03, DCS-NFR-SF-01): Hydra keeps its clients, signing keys and sessions in PostgreSQL, in a database of its own. `hydra.config.dsnSecretRef` names a Secret key with the whole DSN (`postgres://<user>:<password>@<host>:5432/hydra?sslmode=verify-full`); `hydra.config.coDeployedPostgresql.enabled` uses the `hydra` database of the co-deployed PostgreSQL (list `hydra` in `postgresql.additionalDatabases`, the default) with the password from its Secret. Outside dev mode the chart refuses `dsn: memory`, and `hydra.replicaCount` above 1 needs a persistent DSN. `values.prod.yml` runs 2 replicas.
- Clients: every Hydra pod registers `hydra.clients` through its admin API on loopback when it starts (upsert by `client_id`), and the pod is ready only when they exist. A restart or a new replica therefore has every client, without a Helm hook.
- Client secrets (DCS-NFR-SEC-12): `hydra.clients[].client_secretSecretRef` (`name`, `key`) names an existing Secret; an inline `client_secret` goes into the chart Secret `<release>-hydra-clients` (key: the `client_id`). The backend (`HYDRA_CLIENT_SECRET`) and the registration read it through `secretKeyRef`; no pod spec holds it.
- Admin API (DCS-NFR-SEC-03): it has no authentication of its own and accepts logins and consents with any roles. With `global.internalTLS` the Service offers it only on the TLS proxy (port 8443), and the NetworkPolicy admits only the backend ("Network policies").
- Hydra encrypts stored keys with the system secret and does not re-encrypt existing data. For an existing Hydra with a persistent database, set the Secret value to `<new>,<old>`: Hydra encrypts with the first value and decrypts with all of them. The old value is the previous `hydra.secrets.system`, or `<release>-hydra-system-secret-change-me` if none was set.
- Access-token lifetime (DCS-IR-CI-09, DCS-NFR-BR-06): `hydra.config.accessTokenTTL` (`5m` in `values.prod.yml` and `values.acceptance.yml`; empty keeps the Hydra default of 1h). A revoked credential ends the session at the next token refresh, so this is the longest time an access token issued before the revocation stays valid.
- Register `dcs-client` redirect URIs via `hydra.clients` (see `values.dev.yml`):
  - **Valid Redirect URIs**: `https://<domain>/<path>/api/auth/callback`
  - **Valid Post Logout Redirect URIs**: `https://<domain>/<path>/api/auth/logout-complete`

### Lockout, rate limit and token checks
- Lockout (DCS-FR-UC-01-4): 5 invalid access tokens from one client address within 15 minutes lock that address for 15 minutes; 5 refused login presentations lock the holder key of a credential that a trusted issuer signed (or the posting address otherwise). Expired and missing tokens do not count, and refusals during a lock do not extend it. A Sys. Administrator lists the active locks and ends one under **Admin > Lockouts** (`GET /admin/lockouts`, `POST /admin/lockouts/unlock`; the unlock is audited as `LOCKOUT_LIFTED`).
- Client address: the backend reads `X-Forwarded-For` only from a trusted proxy and takes the right-most entry that is not a trusted proxy. `DCS_TRUSTED_PROXIES` (comma-separated CIDRs, set through `extraEnv`) names the trusted proxies; the default is loopback (the TLS proxy in the pod) and the private ranges `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7` (the ingress controller). When the clients themselves reach the ingress from private addresses, or other pods in the cluster must not choose their address, set it to the addresses of the ingress controller pods. The ingress controller must see the real client address: with a `LoadBalancer` or `NodePort` Service in front of it, set `externalTrafficPolicy: Local` (or use the PROXY protocol); with the default `Cluster` policy all clients share the node addresses, and one client can lock them all out. A locked address refuses also a valid token ([ADR-38](../docs/adr-38-recorded-deviations-of-the-pilot.md) §10).
- Rate limit (DCS-FR-CWE-28): `DCS_API_RATE_LIMIT_PER_MINUTE` (default 3000) requests per minute per authenticated caller (holder DID or machine client), counted after the token is validated and shared by all replicas. Over the limit the API answers 429 with `Retry-After`.
- `JWT_ALG_NONE_SUPPORTED=true` turns off the access token signature check. The backend refuses to start with it unless `DCS_ALLOW_DEV_TRUST=true` declares a development or test stack, and logs a warning when it runs with it. Never set either in production. No profile of the chart sets it: the BDD stacks check signatures and sign the tokens of their expired-credential scenarios with a development key, whose public half `DCS_DEV_ACCESS_TOKEN_KEY` names (PEM, ES256). That variable is refused without `DCS_ALLOW_DEV_TRUST=true` too.
- Peer trust (DCS-IR-SI-06): the federation trust gate decides (ADR-19: agreement credential plus the trust policy configured in ORCE; peer issuers anchored per ADR-35). `DCS_FORCE_EIDAS_CERT=true` adds the eIDAS check: the DID certificates of peers and of this instance must chain to a qualified trust service provider in the XAdES-verified EU trusted lists, which needs outbound HTTPS to `ec.europa.eu` and the national trusted-list servers.

### Metrics
- The backend serves Prometheus `/metrics` on `service.metricsPort` (default 9464), not on the API port. The port has its own ClusterIP Service `<fullname>-metrics`, which `service.type` does not change. Do not route it through an ingress or load balancer.
- `http_requests_total` and `http_request_duration_seconds` carry the labels `method` (a standard HTTP method, else `OTHER`), `path` (the route pattern, for example `/contract/{did}`; `/` when no route matched) and `status`, so unknown paths add no series.
- Alerts (DCS-NFR-SEC-11): where the Prometheus Operator CRDs exist (or `monitoring.enabled`), the chart writes the PrometheusRule and the ServiceMonitor whose series the rules evaluate. Both carry `alerts.labels`; empty, that is `release: <release>` with `monitoring.enabled` (the co-deployed kube-prometheus-stack) and `release: prometheus` otherwise. Set it to the `ruleSelector` and `serviceMonitorSelector` of your Prometheus. `DCSBackendDown` also fires when no backend pod is left (`absent(up{...})`); `DCSBackupStale` and `DCSBackupNeverSucceeded` fire when no backup succeeded for 26 hours (needs kube-state-metrics).

### Network policies
- `networkPolicy.enabled` (on by default) writes NetworkPolicies for the pods of the release (`templates/networkpolicy.yaml`). They take effect only on a cluster whose CNI enforces NetworkPolicy (Calico, Cilium, kindnet from kind 0.24, ...); that CNI is a prerequisite of a production deployment (DCS-NFR-SEC-03, DCS-NFR-SEC-14, DCS-NFR-SF-02).
- Every pod the chart owns is under a default deny for ingress. The policies admit:
  - backend API (`http`, `https`), ORCE and DSS (every port): with `networkPolicy.ingressFrom` (the pods of the ingress controller), only the ingress, the pods of the release and `networkPolicy.peersFrom`; peers of other organizations reach the backend through the ingress. Without `ingressFrom`, every source. A production-like release with an ingress does not render without `ingressFrom` ("Production prerequisites"). `/metrics`: `networkPolicy.metricsFrom` (default: pods labelled `app.kubernetes.io/name: prometheus` in any namespace).
  - Hydra public API: every source. Hydra admin API (`admin`, `https`): the backend only.
  - IPFS RPC API and gateway: the backend only; the swarm port: every source. The RPC API also requires a bearer token (`ipfs.api.auth`, below).
  - PostgreSQL: the backend, Hydra, the backup and the compliance monitor. NATS and pdf-core: the backend.
  - The Federated Catalogue stack (fc-service, Keycloak, portal): every source, because the ingress routes them, and they authenticate their callers. Fuseki: fc-service; the catalogue PostgreSQL: fc-service and Keycloak. Loki: its own policy (push from the collector, query from `logging.queryFrom`).
- Kubelet probes come from the node, which a NetworkPolicy does not block. Where a CNI blocks them, add the node CIDR as an `ipBlock` to `networkPolicy.extraFrom`; it is added to every restricted rule.
- Port-forwards (`kubectl port-forward`, used by the BDD harness) and `kubectl exec` do not pass through NetworkPolicies.
- The BDD and e2e kind stacks (kind v0.30.0, `tests/bdd/kind-config.yaml`) enforce the policies. Their values name the Traefik ingress in `kube-system` as `ingressFrom` and the other BDD release as `peersFrom`: instance B (release `dcs2`) calls the ORCE and DSS of instance A, and A's ORCE posts the deployment callbacks of B's target to B. Before the suite, `tests/bdd/scripts/run_bdd_helm.sh` checks that a pod of the release reaches the backend API and is dropped at the IPFS RPC API, that a pod outside the release is dropped at the backend API, and that the API server proxy to the backend `/metrics` (used by the load test) works. kindnet does not filter traffic that root processes on the node start (the API server, kubelet probes), so the BDD values add nothing to `networkPolicy.extraFrom`.
- The dev profiles set `networkPolicy.enabled: false`: the dev backend runs on the host and reaches the services through NodePorts.
- Kubo RPC token (DCS-NFR-SF-02): with `ipfs.api.auth.enabled` (on by default) the co-deployed Kubo accepts RPC calls only with the bearer token of the Secret `<release>-ipfs-api` (key `token`, random on install, kept on upgrade; Kubo `API.Authorizations`). The backend gets it as `IPFS_API_TOKEN`. For a Kubo outside the release, set `ipfs.apiTokenSecretRef`. The `ipfs` CLI in the IPFS pod needs `--api-auth "bearer:$IPFS_API_TOKEN"`.

### Keycloak (Federated Catalogue only)
- FC integration uses `fcKeycloak.realmURL` / `FC_KEYCLOAK_REALM_URL`

### TLS
- Use certificates from a trusted Certificate Authority
- Recommend [cert-manager](https://cert-manager.io/) for automatic renewal
- TLS 1.3 minimum (DCS-IR-CI-01, DCS-NFR-SEC-01). When Traefik terminates the ingress TLS, the chart enforces it: it writes a `TLSOption` with `minVersion: VersionTLS13` and names it on the ingress (`ingress.tlsOptions`, on by default). For another controller, set the minimum on the controller. For ingress-nginx, set `ssl-protocols: "TLSv1.3"` in the controller ConfigMap.
- Verify after install. The first command must fail, the second must connect:
  ```bash
  openssl s_client -connect dcs.example.org:443 -tls1_2 </dev/null
  openssl s_client -connect dcs.example.org:443 -tls1_3 </dev/null
  ```
- Outbound TLS (DCS-IR-CI-01): every HTTPS client of the backend requires TLS 1.3, except the download of the EU national trusted lists and the RFC 3161 time-stamp requests, which accept TLS 1.2 because not all member-state servers and TSAs offer TLS 1.3; both responses are signed, and the backend verifies the signature of each time-stamp token and of each trusted list ([ADR-38](../docs/adr-38-recorded-deviations-of-the-pilot.md) §4). PostgreSQL and NATS clients connect with the driver defaults; the co-deployed servers accept TLS 1.3 only, and an external database server sets its own minimum.
- HTTPS redirect and HSTS (DCS-IR-CI-03): when `route.publicBaseURL` is `https://`, the backend answers a request that the proxy received over plain HTTP (`X-Forwarded-Proto: http`) with a redirect to HTTPS, and sends `Strict-Transport-Security` on HTTPS responses.
- PostgreSQL: put `sslmode=verify-full` (with `sslrootcert`) in the `DATABASE_URL` of the external database. `database.sslmode` sets the mode of a URL the chart derives.
- In-cluster TLS (DCS-NFR-SEC-14, `global.internalTLS`, on by default). The chart generates a CA on install (Secret `<release>-internal-ca`, kept on upgrade, ECDSA P-256) and a server certificate for the co-deployed services (`<release>-internal-tls`). The co-deployed PostgreSQL accepts TCP connections only with TLS 1.3 (`hostssl`), and the derived `DATABASE_URL` uses `sslmode=verify-full`. NATS requires TLS 1.3, and the derived `NATS_URL` is `tls://`. IPFS RPC, pdf-core, DSS and ORCE serve plain HTTP only: a TLS proxy container in each pod serves TLS 1.3 on port 8443, and the backend calls it there. Their plain ports stay for probes, ingress routes and port-forwards, except the IPFS RPC API and the Hydra admin API: their Services offer only the TLS port, and the NetworkPolicies admit only the backend ("Network policies"). The backend pod and the Hydra pod have the same TLS proxy: in-cluster callers reach the backend API (ORCE callbacks, `DCS_URL`) and the Hydra admin API (the backend's `HYDRA_ADMIN_URL`) on port 8443. The backend calls the Hydra public API (OIDC discovery, token) on port 8444 of the Hydra pod (`HYDRA_INTERNAL_ISSUER_URL`). ORCE trusts the CA (`NODE_EXTRA_CA_CERTS`). The certificate CRL distribution point that the provisioning job writes stays an `http://` URL of the backend service (RFC 5280; the CRL is signed; [ADR-38](../docs/adr-38-recorded-deviations-of-the-pilot.md) §5). The backend, the backup and the compliance monitor trust the CA (`SSL_CERT_DIR`, `sslrootcert`). Another client of the co-deployed PostgreSQL, for example Hydra, needs `sslmode=verify-full&sslrootcert=/etc/dcs-internal-tls/ca.crt` in its DSN; Hydra mounts the CA there. URLs set in values (`signing.tsaURL`, `auditExecutor.url`, `federation.trustPdpURL`, ...) are used as given. The BDD and e2e kind stacks (`values.bdd.yml`, `values.bdd2.yml`) run with it on. Instance B calls the ORCE and DSS it shares with instance A on their plain ports, because they belong to another release with another CA. The dev profiles set `global.internalTLS.enabled: false`, because the dev backend runs on the host and reaches the services through NodePorts.
- Ingress to the pods (DCS-NFR-SEC-01, DCS-NFR-SEC-14). With `global.internalTLS` on, the ingress routes go to the TLS proxies: the backend on port 8443, the Hydra public API on port 8444 (`hydra.service.publicTLSPort`; port 8443 of the Hydra pod stays the admin API) and the Federated Catalogue on port 8443. The controller verifies the proxies against the internal CA, under the name `<fullname>.<namespace>.svc`, which the one internal certificate carries next to the names of Hydra and `fc-service`. `ingress.className` selects the controller, as for the TLS 1.3 option:
  - `traefik`, or empty when the Traefik `ServersTransport` CRD exists: the chart writes a `ServersTransport` `<release>-internal-tls` with the CA of `<release>-internal-tls` as `rootCAs`, and the backend, Hydra and `fc-service` Services name it (`traefik.ingress.kubernetes.io/service.serverstransport`). Traefik uses HTTPS for these routes because the Service ports are named `https` and `https-public`. The BDD and e2e kind stacks run this way.
  - `nginx` (ingress-nginx): the ingress carries `backend-protocol: "HTTPS"`, `proxy-ssl-secret: <namespace>/<release>-internal-tls`, `proxy-ssl-verify: "on"`, `proxy-ssl-name` and `proxy-ssl-protocols: "TLSv1.3"`.
  - Any other class: the routes use the plain ports. Set `ingress.className` to `traefik` or `nginx` to get TLS on this hop.
- Co-deployed PostgreSQL password: with `postgresql.auth.password` empty, the chart generates a random password on install and keeps it on upgrade (Secret `<release>-postgresql`). The backend, the backup and the compliance monitor get it as `PGPASSWORD`.

### Pod security and image integrity (DCS-NFR-SEC-05)
- Every pod the chart and its subcharts own runs with a `RuntimeDefault` seccomp profile, with all capabilities dropped and without privilege escalation. The containers run as a fixed non-root UID on a read-only root filesystem and write only to their volumes and `emptyDir` mounts (`/tmp` and the paths each component writes). Chart test: `deployment/helm/tests/pod_security_test.sh`.
- UIDs: DCS backend, compliance monitor, SoftHSM2 provisioning job and pdf-core 10001 (set in `deployment/docker/Dockerfile` and `pdf-core/Dockerfile`); TLS proxies and the status-list proxy 101 (nginx); PostgreSQL and the backup 70; Hydra 100; NATS, ORCE, DSS, IPFS (1000:100) and the Federated Catalogue stack 1000; Loki 10001; the log collector 473; the Hydra client job, the Federated Catalogue realm jobs and the ACME renewal 65532. The pod and container contexts are values (`podSecurityContext`, `securityContext` of each chart) and can be changed per deployment.
- ORCE and DSS images run as root by default. The chart runs them as 1000: Node-RED seeds `/data` with `cp -R`, and an init container copies the DSS WAR into an `emptyDir` that Tomcat expands. The ORCE hook jobs use the ORCE image, which carries curl, jq and openssl, so nothing is installed at run time. On a volume that ignores `fsGroup` (the `hostPath` volume of the kind BDD stack), `orce.volumePermissions.enabled` adds an init container that runs as root with only `CAP_CHOWN` and gives `/data` to the ORCE user.
- Exceptions: Keycloak (Federated Catalogue only) keeps a writable root filesystem, because `kc.sh start` builds Quarkus into `/opt/keycloak` at startup; a pre-built image removes that. The XFSC statuslist-service subchart, which no profile deploys, gets only the seccomp profile and the dropped capabilities: its image publishes no user.
- Image digests: each image value has a `digest` key (`image.digest`, `pdfCore.image.digest`, `global.internalTLS.proxy.image.digest`, `postgresql.image.digest`, ...). When set (`sha256:<hex>`), the image is pulled as `<repository>:<tag>@<digest>`. Pin every image by digest in production. The Federated Catalogue, Keycloak and the realm-provisioning images carry their digest in the tag; `acmeRenewal.image` takes a full reference, digest included.
- Remote attestation of the running workload (SRS: MAY) is operator scope: it needs node and runtime support (measured boot, a TPM, an attestation service) that the chart cannot provide.

### Logs (DCS-NFR-SEC-09)
- `logging.enabled` (on in `values.prod.yml` and `values.acceptance.yml`) co-deploys Loki and a collector (Grafana Alloy). The collector reads the logs of every pod in the release namespace through the Kubernetes API (a namespaced Role, no node access) and pushes them to Loki over TLS.
- Retention: `logging.retention` (default `2160h`, 90 days). The Loki compactor deletes older lines. Set `logging.loki.persistence.storageClassName` to an encrypted class.
- Access: Loki has no ingress and no authentication of its own, and listens on loopback only. A gateway in the Loki pod serves TLS 1.3 and admits pushes (`/loki/api/v1/push`) on port 3101 and the query APIs on port 3100; every other API, the delete API included, answers 403. A NetworkPolicy lets only the collector connect to 3101 and only the pods in `logging.queryFrom` (default: Grafana in the same namespace) to 3100. The chart writes a Grafana data source ConfigMap (label `grafana_datasource: "1"`) that the Grafana of `monitoring.enabled` loads; Grafana requires a login. For a Grafana in another namespace, add its namespace to `logging.queryFrom` and load the data source there.
- A production deployment needs a Grafana to read the logs: `monitoring.enabled`, or a Grafana the operator runs and adds to `logging.queryFrom`. `values.prod.yml` co-deploys none.
- The audit trail is not in Loki: it stays in the database (`audit_trail_log`, hash-chained, anchored by checkpoints) without expiry and is part of the database backup.

### Production prerequisites

`values.prod.yml` and `values.acceptance.yml` are skeletons: hostnames (`dcs.example.org`), the registry and the Secrets are the operator's. A release with a real HSM (`pkcs11.provisioning.enabled=false`) or a Hydra outside dev mode does not render until the values below are set; the chart error names each missing one (`templates/prerequisites.yaml`), instead of a backend that crash-loops. `deployment/argocd/application.yaml` shows them filled for acceptance.

Values (production and acceptance):

| Value | Why |
|---|---|
| `federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary: true` | A remote Federated Catalogue: its service account has `ADMIN_ALL`, so it is safe only inside one trusted administrative boundary ([ADR-18](../docs/adr-18-fc-catalogue-single-service-account.md)). Without it the chart refuses to render. |
| `signing.issuerDID` (or `signing.issuerDIDSecretRef`) | The DID of this instance (`ISSUER_DID`), `did:web:<host>`. |
| `networkPolicy.ingressFrom` | The pods of the ingress controller (namespace and pod selector). The backend, ORCE and DSS then admit no client outside the release, the ingress and `networkPolicy.peersFrom` (DCS-NFR-SEC-14). Required when `networkPolicy.enabled` and `ingress.enabled`. |
| `signing.tsaURL` | The RFC 3161 timestamp authority (`TSA_URL`). The backend does not start without it. A deployment without its own TSA can use the ORCE TSA (`orce.enabled`, `http://<release>-orce:1880/tsa/`); `signing.tsaTrustCertSecret` then names the Secret with its certificate (key `tsa-cert.pem`). |
| `messaging.natsURL`, `messaging.secretRef` or `nats.enabled` | The NATS server (`NATS_URL`), with JetStream enabled and a persistent store: the backend creates the stream `DCS_EVENTS` and a durable consumer per consumer group on it (ADR-39). `values.prod.yml` co-deploys none. |
| `oid4vp.trust.enabled: true`, `oid4vp.trust.existingConfigMap` | The trusted credential issuers ("Credential issuers", "The trust document"). |
| `configIntegrity.pins.oid4vp-trust-data` | The SHA-256 of that trust document (`sha256sum` of the ConfigMap key); the backend refuses to start when the ConfigMap differs (DCS-NFR-SEC-04). |
| `hydra.clients` | The login client of the backend (`dcs-client`, redirect URIs under `route.publicBaseURL`); both profiles carry it with `client_secretSecretRef`. |
| `hydra.config.dsnSecretRef` | The DSN of the Hydra database ("Hydra"); both profiles carry it. |
| `identity.existingSecret` (real HSM) or `identity.enabled: true` (SoftHSM2, acceptance) | `did.json` of the HSM key `dcs-did` (`DCS_DID`). With a real HSM, create it with `gendid` against that HSM: `PKCS11_MODULE_PATH=... PKCS11_TOKEN_LABEL=... PKCS11_PIN=... gendid -did did:web:dcs.example.org -endpoint https://dcs.example.org/digital-contracting-service/api -out did.json` (the tool is in the DCS image at `/app/gendid`), then `kubectl create secret generic dcs-prod-did --from-file=did.json`. |
| `pkcs11.modulePath`, `pkcs11.tokenLabel` (real HSM) | The PKCS#11 module, mounted into the pod (`volumes`, `volumeMounts`), and the token. |
| `signing.issuerX5ChainPath` (real HSM) | The certificate chain of the HSM key `dcs-c2pa`, mounted into the pod. |
| `identity.eidas.enforce` (optional) | Builds the EU trust pool and requires the DID certificates of this instance and of every peer to chain to a qualified trust service (`DCS_FORCE_EIDAS_CERT`). It needs a qualified certificate for the DID of this instance; off by default ([ADR-38](../docs/adr-38-recorded-deviations-of-the-pilot.md) §4). |
| `image.repository`, `image.tag`, `pdfCore.image.*` | The images, pinned by tag and digest. |
| `ingress.hosts`, `ingress.tls`, `route.publicBaseURL`, the Hydra URLs | Your hostname instead of `dcs.example.org`. |

Secrets the profiles reference (create them before install; `values.acceptance.yml` uses the prefix `dcs-acceptance-`):

| Secret | Keys |
|---|---|
| `dcs-prod-db-credentials` | `DATABASE_URL` (`postgres://...?sslmode=verify-full`) |
| `dcs-prod-hydra-secrets` | `secretsSystem` (`openssl rand -hex 32`), `dsn` (the Hydra database), `dcs-client-secret` |
| `dcs-prod-hsm-pin` | `PKCS11_PIN` |
| `dcs-prod-fc-credentials` | `FEDERATED_CATALOGUE_API_URL`, `FEDERATED_CATALOGUE_CLIENT_SECRET` |
| `dcs-prod-ipfs-credentials` | `IPFS_MFS_BASE_URL` (the Kubo RPC URL, for example `https://<release>-ipfs:8443`) |
| `dcs-prod-tls` | `tls.crt`, `tls.key` (type `kubernetes.io/tls`, the ingress certificate) |
| `dcs-prod-did` (`identity.existingSecret`) | `did.json` |
| the NATS Secret of `messaging.secretRef` | `NATS_URL` |

The chart generates the other credentials (the internal CA and certificate, the PostgreSQL password of a co-deployed server, the Kubo API token, the SoftHSM2 PIN) and keeps them on upgrade.

A production cluster also needs a CNI that enforces NetworkPolicy ("Network policies") and a Grafana for the logs ("Logs").

Database (DCS-OE-05): the backend needs PostgreSQL. PostgreSQL 16 is the version the chart deploys and the tests run on; other versions are not tested. The backend keeps its replicas consistent with PostgreSQL functions that a "PostgreSQL-compatible" server may not have: transaction-level advisory locks (`pg_advisory_xact_lock`, `pg_try_advisory_xact_lock`) keyed with `hashtext`, `LISTEN`/`NOTIFY` (`pg_notify`), `gen_random_uuid`, `jsonb` and full-text search (`tsvector`). A server without them, for example CockroachDB, is not supported ([ADR-38](../docs/adr-38-recorded-deviations-of-the-pilot.md) §9).

Availability (DCS-NFR-PER-03): `values.prod.yml` runs 3 backend replicas, which prefer different nodes, and 2 Hydra replicas. The co-deployed IPFS node, NATS server, Loki and DSS run one replica each: a restart of one of them interrupts what depends on it (artifact reads and writes, events, log ingestion, signature validation) until the pod is back; the outbox delivers the events written meanwhile once NATS is back, and JetStream keeps each event on the NATS volume (PersistentVolumeClaim `<release>-nats`) until every consumer group has it (ADR-39). The database is the operator's external PostgreSQL (`database.secretRef`) with its own availability.

Other values for every profile:

```yaml
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
