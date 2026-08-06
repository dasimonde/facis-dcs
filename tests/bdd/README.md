# Running the BDD suite against your own DCS instances

The suite in `features/` normally runs against an ephemeral two-instance kind
stack that `make run_bdd_kind` builds from scratch. This document describes the
other way to run it: against two DCS deployments you operate yourself.

## Before you start

This is a white-box suite. It does not only call the public API: it connects
directly to the contract database, execs into the backend and IPFS pods,
restarts components mid-scenario, and deletes contract data between scenarios.

**Run it only against deployments whose data you can afford to lose.** 81
features carry `@clean_db`, which truncates contracts, templates, signatures,
signing ceremonies, negotiations, tasks and archive entries before the scenario
starts. There is no mode in which the suite leaves an existing dataset intact.

## What you need

| | |
|---|---|
| Two DCS deployments | installed from `deployment/helm`, referred to below as instance A and instance B |
| Cluster access | `kubectl` against A's namespace with `exec`, `port-forward` and `rollout restart` rights; B needs only to be reachable over HTTP(S) |
| Mutual reachability | A and B must each resolve and fetch the other's `did:web` document; the federation scenarios exchange documents directly between the two |
| Python 3.12 | plus the harness virtualenv, created by `make setup_environment` |

Instance B may be a second release in the same namespace (what the kind harness
does, see `values.bdd2.yml`) or a genuinely separate deployment in separate
infrastructure. The suite does not care which, as long as both are reachable.

## 1. Configure trust

The harness authenticates by minting its own Power-of-Attorney credential AS
the ORCE credential issuer `ISSUER_BASE_URL` names — signed with the committed
issuer key that release is handed
(`deployment/helm/charts/orce/pki-dev/issuer.key`), carrying the certificate
chain in the credential header — and presenting it over OpenID4VP. The issuer
is that one because a status list is believed only from the issuer that
publishes it, and every credential here points at that issuer's list. Your
deployment must be configured to trust it, or every scenario fails at its first
authenticated request with:

```
401 vp verification failed: credential jwt: token is unverifiable:
    issuer "http://localhost:18080/issuer" is not trusted
```

Trust is purpose-scoped and bound to an organization (ADR-31). The suite's
credential is a `urn:dcs:poa:v1` presented for purpose `login`.

**Set `DCS_ALLOW_DEV_TRUST=true` on both instances.** The backend otherwise
refuses any trust entry keyed to material committed in this repository
(`backend/internal/auth/oid4vp/trust_config.go`), which is what the shipped dev
trust config is. The flag exists so that no deployment inherits that trust by
omission. See `deployment/helm/values.bdd.yml` for the reference setting.

> The `issuer-dev.jwk` key is public. Anyone with a clone of this repository can
> mint credentials that a deployment with `DCS_ALLOW_DEV_TRUST=true` will
> accept, in any role. Enable it only on deployments that are isolated and
> disposable, never on one holding real contracts.

If you need to run against instances that must keep production trust settings,
the harness needs a credential from your own issuer instead. There is no client
for that today: `testWallet/dcs_wallet/` self-issues and has no OpenID4VCI
fetch path.

## 2. Configure the two instances

Beyond trust, the kind harness sets these on both releases
(`deployment/helm/values.bdd.yml`). The suite's timing assumptions depend on the
last two.

| Setting | Why the suite needs it |
|---|---|
| `DCS_ALLOW_DEV_TRUST=true` | §1 |
| `DCS_TRUST_PDP_URL` | The federation trust gate (ADR-19) is consulted fail-closed on every inbound and outbound interaction. Point it at an ORCE `trust-pdp` flow; the peer-trust scenarios steer that flow between allow/deny/silent and read back what it captured. |
| `DCS_SYNC_FAIL_RETRY_INTERVAL=10s` | Peer document shipping is retried from the `sync_fails` table. The production default is minutes; scenarios wait seconds for replication. |
| `DCS_PDF_REGENERATION_RETRY_INTERVAL` | Same reasoning for background PDF regeneration. |
| `DCS_DIDWEB_INSECURE_HOSTS` | Only if your instances address each other over plain HTTP on non-loopback names. `did:web` is HTTPS by default and the resolver makes an exception only for loopback. |

Peer identity needs no configuration: the suite fetches each instance's DID from
its own `/.well-known/did.json` at runtime.

## 3. Establish the environment

`scripts/run_bdd_helm.sh` does this for the kind stack: it opens the
port-forwards, resolves pod names, reads the ORCE token out of its pod and
exports everything below. Running against your own deployments means setting the
same contract yourself, because that script assumes the kind namespace, release
names and ingress origin.

```bash
export KUBECONFIG=/path/to/your/kubeconfig
NS=<namespace of instance A>

# The database and the in-cluster-only services the suite reaches directly.
kubectl -n "$NS" port-forward svc/<release>-postgresql 5432:5432 &
kubectl -n "$NS" port-forward svc/<release>-dss        18099:8080 &
kubectl -n "$NS" port-forward svc/<release>-orce       18880:1880 &

BACKEND_POD=$(kubectl -n "$NS" get pod \
  -l app.kubernetes.io/component=backend,app.kubernetes.io/instance=<release> \
  --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')
IPFS_POD=$(kubectl -n "$NS" get pod -l app.kubernetes.io/component=ipfs \
  --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')
```

Select the backend pod by the `component=backend` **and** `instance=<release>`
labels together. The DCS deployment's selector also matches pdf-core pods, and
when two releases share a namespace an unscoped selector will sign through the
wrong instance's token.

| Variable | Set it to |
|---|---|
| `BDD_DCS_BASE_URL` | Instance A's API base, e.g. `https://a.example.org/api`. Note the path is whatever your release serves (`paths.api`), which is not necessarily the kind harness's `/digital-contracting-service/api`. |
| `BDD_DCS_BASE_URL_A` | Same value. The two-instance scenarios address A explicitly. |
| `BDD_DCS_BASE_URL_B` | Instance B's API base. |
| `BDD_PUBLIC_ORIGIN` | Instance A's origin, without the API path. |
| `ISSUER_BASE_URL` | The ORCE issuer serving the status list your credentials name (ADR-34). Must be the URL the **backend** fetches: the verifier requires the status token's `sub` to equal the credential's URI. |
| `DATABASE_URL` | `host=localhost port=5432 user=dcs password=dcs dbname=dcs sslmode=disable`, through the port-forward above. |
| `BDD_DSS_URL` | `http://localhost:18099`. This is the EU DSS webapp acting as the external signature creation application. |
| `BDD_HSMSIGN_EXEC` | `kubectl -n $NS exec $BACKEND_POD -c digital-contracting-service --` |
| `BDD_IPFS_EXEC` | `kubectl -n $NS exec -i $IPFS_POD --` |
| `BDD_KUBECTL` | `kubectl` |
| `BDD_ORCE_NAMESPACE` / `BDD_ORCE_DEPLOYMENT` | Namespace and deployment name of instance A's ORCE. |
| `BDD_ORCE_ARCHIVE_NOTARY_URL` | `http://localhost:18880/archive/notary` |
| `BDD_ORCE_ARCHIVE_AUDIT_LOG_URL` | `http://localhost:18880/archive-audit-events.jsonl` |
| `BDD_ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN` | A token accepted by that endpoint. |
| `BDD_ORCE_AUDIT_CONTROL_URL` | `http://localhost:18880/audit-executor/test` |
| `BDD_ORCE_AUDIT_EXECUTOR_URL` | `http://localhost:18880/audit/run`. Address it through the port-forward, not through the ingress: the DCS mux claims `/orce` on the same host and answers 404. |
| `BDD_TRUST_PDP_CONTROL_URL` | `http://localhost:18880` |
| `BDD_TRUST_PDP_DEFAULT_FLOW_WIRED` | `1`, if your ORCE runs the shipped `trust-pdp` flow. |
| `BDD_ORCE_TARGET_URL` | `$BDD_PUBLIC_ORIGIN/contract-target/deploy` |

The keys the harness signs with come from `testWallet/keys/`. Generate them once
with `python3 testWallet/scripts/generate_keys.py --yes`; override their location
with `BDD_TEST_WALLET_KEYS_DIR` if you keep them elsewhere.

Before the first run, confirm the status list resolves: a credential whose
status list cannot be verified is refused at login, and this fails faster than a
scenario does:

```bash
python tests/bdd/scripts/check_status_list.py
```

## 4. Run

```bash
cd tests/bdd
make setup_environment          # once
make run_bdd_test               # full suite, coverage + JUnit into .reports/
make run_bdd_fast T=@two-instance   # tag-scoped, stops at first failure
```

`run_bdd_test` writes JUnit XML to `tests/bdd/.reports/junit/` and a coverage
database at the repository root.

## 5. What each group of scenarios requires

If a capability is unavailable in your setup, exclude the scenarios that need it
with `--tags=~@...` or a feature path rather than letting them fail.

| Scenarios | Requirement |
|---|---|
| All authenticated scenarios | §1 trust, and a reachable issuer status list |
| `@clean_db` (81 features) | Direct database access. Destroys existing contract and template data. |
| `@two-instance` (`features/17_peer_trust`) | Both instances, mutually resolvable `did:web`, and a steerable trust PDP |
| `features/22_real_signing_vertical`, `04_contract_signing` | DSS reachable, and `BDD_HSMSIGN_EXEC` into the backend pod. Signing keys are non-extractable and PKCS#11-only, so the harness cannot sign locally |
| `features/19_c2pa_conformance`, tamper scenarios elsewhere | `BDD_IPFS_EXEC`. These add corrupted bytes to IPFS and repoint `contracts.pdf_ipfs_cid` at them, restoring the original CID on cleanup. |
| `features/08_audit_compliance` | ORCE endpoints, plus permission to `kubectl rollout restart` the ORCE deployment mid-scenario |
| `features/27_external_checkpoint_and_workflow_gates` | The machine running behave serves an HTTPS sink in-process, and **your cluster must be able to reach it**. Set `BDD_CHECKPOINT_SINK_BIND_HOST` and `BDD_CHECKPOINT_SINK_PORT` to an address routable from the cluster. If your cluster cannot route back to your workstation, these scenarios cannot pass. |
| `@isolated_stack` (`features/25_federated_catalogue_deployment_lifecycle`) | Creates and deletes **Kubernetes namespaces** and runs `helm install` against your cluster, including a chart from this repository's git history. It therefore needs a full clone (`fetch-depth: 0`), not a shallow one. Skip these unless you intend to grant that. |

## 6. Troubleshooting

**Every scenario fails at login.** Check trust first (§1). The failure text names
the issuer that was refused.

**`Could not connect to database`.** The harness opens the connection once at
startup and fails the whole run if it cannot. Confirm the port-forward survived.
The kind harness wraps its forwards in `scripts/keep_port_forward.sh` for that
reason.

**Signing scenarios fail but nothing else does.** Usually DSS: the Tomcat bundle
boots slowly and its readiness probe allows 90 seconds before it is available.

**Peer scenarios time out waiting for a document.** Check
`DCS_SYNC_FAIL_RETRY_INTERVAL` on both instances (§2). At the production default
the retry lands long after the scenario's wait expires.

**A scenario signs through the wrong instance.** Your backend pod selector is
not scoped by release. See §3.
