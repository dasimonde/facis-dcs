# Federated Catalogue deployment and operation

## Purpose

The DCS retains the XFSC Federated Catalogue for publishing and discovering
contract templates. The supported co-deployment consists of FC service,
Fuseki, PostgreSQL and Keycloak. Neo4j and its n10s plugin are not part of this
runtime (`deployment/helm/values.yaml:392-403`,
`deployment/helm/charts/federated-catalogue/templates/deployment.yaml:41-53`).

This guide describes the deployment lifecycle. For the catalogue's identity and
authorization boundary, see [ADR-18](../adr-18-fc-catalogue-single-service-account.md).

## Root cause of the former readiness failure

The previous development deployment combined Neo4j with n10s. FC invoked the
n10s procedure `n10s.graphconfig.show` during startup, but available n10s builds
were not compatible with the drifting Neo4j patch image. FC therefore exited
instead of ever satisfying readiness. Increasing a timeout could only delay the
same terminal result.

The supported chart selects Fuseki explicitly and disables the unused Neo4j
health indicator (`deployment/helm/charts/federated-catalogue/templates/deployment.yaml:41-53`).
The FC, Fuseki, Keycloak and PostgreSQL images are immutable digest references;
the Helm lifecycle contract checks those references and deterministic rendering
from the committed lock (`deployment/helm/tests/federated_catalogue_lifecycle_test.sh:16-29`,
`deployment/helm/tests/federated_catalogue_lifecycle_test.sh:59-87`).

## Operating modes

### Catalogue disabled

Set:

```yaml
federatedCatalogue:
  enabled: false
fcservice:
  enabled: false
```

No FC client settings are injected into the backend when integration is
disabled (`deployment/helm/templates/deployment.yaml:201-226`). This is the
chart's deliberately optional default
(`deployment/helm/values.yaml:404-426`), not the configured state of the
supported development and BDD environments. Enabling the integration requires
the complete API, realm and credential configuration described below.

### Owned catalogue

The development and BDD profiles both enable and co-deploy the catalogue. The
BDD profile is an executable example of the required wiring:

```yaml
federatedCatalogue:
  enabled: true
  apiURL: "http://fc-service:8081"
  oauth:
    clientID: "federated-catalogue"
    clientSecret: "<secret>"

fcKeycloak:
  realmURL: "http://fc-keycloak-http:8080/auth/realms/federated-catalogue-realm"

fcservice:
  enabled: true
  realmProvision:
    enabled: true
```

The development configuration is at
`deployment/helm/values.dev.yml:110-124`; the BDD configuration is at
`deployment/helm/values.bdd.yml:213-226`. For
non-ephemeral environments, supply the client secret through the existing
Secret references instead of an inline value
(`deployment/helm/values.yaml:411-417`). Configure persistence for FC's file
store, Fuseki and FC PostgreSQL according to the environment
(`deployment/helm/charts/federated-catalogue/values.yaml:62-67`,
`deployment/helm/values.yaml:494-502`).

The semantic verification cold path is CPU-bound. The supported floor is a
500m CPU request with a 2 CPU limit
(`deployment/helm/charts/federated-catalogue/values.yaml:69-78`); the chart's
top-level FC defaults carry the same values
(`deployment/helm/values.yaml:425-434`).

### Remote catalogue

Configure the remote API, its Keycloak realm and client credentials, leave
`fcservice.enabled=false`, and explicitly acknowledge the current
`ADMIN_ALL` boundary:

```yaml
federatedCatalogue:
  enabled: true
  apiURL: "https://catalogue.example"
  remote:
    acknowledgeAdminAllTrustBoundary: true
fcservice:
  enabled: false
fcKeycloak:
  realmURL: "https://identity.example/realms/federated-catalogue-realm"
```

Without the acknowledgement, Helm rendering fails
(`deployment/helm/templates/deployment.yaml:1-3`). It is acceptable only when
all users of that catalogue share one trusted administrative boundary; it does
not provide tenant isolation, as recorded in
[ADR-18](../adr-18-fc-catalogue-single-service-account.md).

## Startup and readiness contract

The startup path is event- and function-driven:

1. Keycloak becomes ready only when its OIDC discovery endpoint answers
   (`deployment/helm/charts/keycloak/templates/deployment.yaml:114-122`).
2. The realm provisioning Job observes Keycloak's native `Available`
   condition, imports the realm idempotently and assigns the FC client roles.
   The Job has `backoffLimit: 0`
   (`deployment/helm/templates/fc-realm-provision-job.yaml:41-70`,
   `deployment/helm/templates/fc-realm-provision-job.yaml:102-160`).
3. DCS init containers first observe Job creation, then race the native
   `Complete` and `Failed` conditions. A failed Job emits its Kubernetes reason
   and terminates startup (`deployment/helm/templates/deployment.yaml:48-123`).
   Their service account can only read/watch the required Deployments and Jobs
   (`deployment/helm/templates/fc-lifecycle-rbac.yaml:1-37`).
4. FC becomes workload-ready from its native `/actuator/health` response
   (`deployment/helm/charts/federated-catalogue/templates/deployment.yaml:124-134`).
5. DCS validates complete FC configuration, performs the native health check
   for an owned catalogue, sends exactly one semantic `/verification` request
   and synchronizes schemas once
   (`backend/cmd/dcs/main.go:509-547`,
   `backend/internal/templatecatalogueintegration/client/client.go:108-183`).
6. Until all initialization succeeds, `/readyz` returns 503. The final server
   returns 200 only after the startup sequence completes
(`backend/cmd/dcs/readiness.go:19-28`,
`backend/cmd/dcs/main.go:829-832`).

There is no FC warm-up loop, schema-sync retry or fixed multi-minute FC sleep.
The FC HTTP client applies a 30-second bound to each request
(`backend/internal/templatecatalogueintegration/client/client.go:203-211`).

## Deployment

Use the supported wrapper so Helm dependencies are rebuilt from the committed
lock without refreshing it:

```bash
deployment/helm/deploy.sh \
  --values /path/to/environment-values.yml \
  --namespace dcs \
  --release dcs
```

The wrapper builds locked dependencies, deploys, gates an owned FC on native
readiness and then gates the backend on its complete startup contract
(`deployment/helm/deploy.sh:103-147`). Its `--timeout` controls Helm and rollout
command bounds; it does not add application retries or warm-up behavior
(`deployment/helm/deploy.sh:42-46`, `deployment/helm/deploy.sh:119-147`).

Do not add `helm --wait` to this flow. The HSM provisioner is a post-install
hook whose output is required before `/readyz` can become successful; waiting
inside the Helm install would deadlock that ordering
(`deployment/helm/values.yaml:153-159`,
`deployment/helm/tests/federated_catalogue_lifecycle_test.sh:248-256`).

## Upgrade from the obsolete Neo4j development stack

No released production or legacy catalogue data exists. Upgrade the release
with the current chart through the supported deployment wrapper. The current
chart replaces the obsolete Neo4j/n10s runtime with Fuseki without migrating
old development catalogue data. The acceptance scenario installs the historical
Neo4j chart, upgrades it and confirms that no obsolete workload remains and the
fresh readiness contract passes
(`features/25_federated_catalogue_deployment_lifecycle/federated_catalogue_deployment_lifecycle.feature:45-52`).

After upgrade, verify:

```bash
kubectl -n <namespace> get deployment,pod
kubectl -n <namespace> rollout status deployment/fc-service
kubectl -n <namespace> rollout status deployment/<release>-digital-contracting-service
```

Expected graph-store workload: `fc-fuseki`. Any remaining Neo4j/n10s workload
belongs to the obsolete installation and is not part of the current chart.

## Diagnosis

The deployment wrapper prints FC Deployment, Job and Pod state plus recent logs
when Helm, FC readiness or backend startup fails
(`deployment/helm/deploy.sh:90-101`, `deployment/helm/deploy.sh:119-147`).
Interpret the first terminal cause:

| Visible error | Meaning | Action |
|---|---|---|
| `remote Federated Catalogue requires ... acknowledgeAdminAllTrustBoundary=true` | A remote catalogue was selected without accepting ADR-18's administrator boundary. | Use an owned catalogue, or confirm that the remote catalogue is inside one mutually trusted administrative boundary before setting the acknowledgement. |
| `incomplete Federated Catalogue configuration; missing ...` | At least one API, realm or client-credential field is absent. | Supply every named value; partial FC configuration is rejected (`backend/cmd/dcs/main.go:509-538`). |
| `Federated Catalogue realm provisioning Job ... failed` | Keycloak realm import or client-role assignment reached a terminal Job failure. | Use the printed Job reason and provisioning logs; the backend does not wait for another attempt (`deployment/helm/templates/deployment.yaml:109-120`). |
| `federated catalogue health ... expected UP` | An owned FC actuator reports a failed dependency. | Inspect FC, Fuseki, FC PostgreSQL and Keycloak logs; do not extend the timeout (`backend/internal/templatecatalogueintegration/client/client.go:113-147`). |
| `federated catalogue functional verification failed` | Native health succeeded, but schema/semantic processing did not. | Inspect the returned status/body and FC logs; the single verification is intentionally not repeated (`backend/internal/templatecatalogueintegration/client/client.go:149-180`). |
| `/readyz` returns 503 with `initialization has not completed` | The backend has not completed HSM, FC and schema initialization. | Follow the first terminal error in backend/FC diagnostics (`backend/cmd/dcs/readiness.go:19-23`). |

## Verification and traceability

The completed implementation was independently verified with:

- full backend tests;
- Helm lint for development, second-development and BDD profiles;
- the FC lifecycle shell contract and immutable image digest checks;
- BDD AC1 (Fuseki/no n10s), AC3/5/6 (fresh install and immediate first
  publish/retrieve/search), AC4 (historical Neo4j upgrade) and AC7/8
  (enabled/disabled entry points and terminal fail-fast behavior).

The executable acceptance specification is
`features/25_federated_catalogue_deployment_lifecycle/federated_catalogue_deployment_lifecycle.feature:8-95`.

| Requirement | Evidence |
|---|---|
| DCS-PC-04, DCS-OE-01, DCS-OE-03 | Kubernetes/Helm lifecycle, locked dependencies and successful deployment tests (`deployment/helm/deploy.sh:103-147`, `deployment/helm/tests/federated_catalogue_lifecycle_test.sh:16-29`). |
| DCS-OE-06 | XFSC Federated Catalogue remains integrated through its native health, verification, schema, asset and query interfaces (`backend/internal/templatecatalogueintegration/client/client.go:100-183`). |
| DCS-NFR-PER-01 | Startup uses Kubernetes condition watches and one bounded functional request, with no polling warm-up or schema-sync retry (`deployment/helm/templates/deployment.yaml:48-123`, `backend/cmd/dcs/main.go:540-547`). |
| DCS-NFR-PER-03 | Terminal Job failure and FC functional failure are diagnosed and stop startup; `/readyz` cannot become green prematurely (`deployment/helm/templates/deployment.yaml:88-123`, `backend/cmd/dcs/readiness.go:19-28`). |
| DCS-NFR-SQ-02, DCS-NFR-SQ-03 | The repository provides locked Helm build/deploy scripts and digest-pinned container manifests (`deployment/helm/deploy.sh:103-126`, `deployment/helm/tests/federated_catalogue_lifecycle_test.sh:59-87`). |
