[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](../LICENSE)

# Digital Contracting Service

An automated orchestration workspace that deploys a [Digital Contracting Service](https://github.com/eclipse-xfsc/facis/tree/main/DCS) instance to a Kubernetes cluster.

---

## Overview

The Digital Contracting Service (DCS) provides an open-source platform for creating, signing, and managing contracts digitally.
Integrated with the European Digital Identity Wallet (EUDI), it guarantees that all digital transactions are secure, legally binding, and interoperable.

Key components:
- **Multi-Contract Signing** — multi-party contract execution within a single workflow
- **Automated Workflows** — contract generation, execution, and deployment
- **Lifecycle Management** — contract monitoring with renewal/expiration alerts
- **Signature Management** — signatures linked to verifiable digital identities
- **Secure Archiving** — tamper-evident archive compliant with retention policies
- **Machine Signing** — automated signing for high-volume transactions

---

## Helm Chart

The parent chart bundles `postgresql`, `keycloak`, `nats`, `neo4j`, and `federated-catalogue` as optional sub-charts, each toggled via `<subchart>.enabled`.

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

### 1. Deploy dependencies

```bash
helm dependency build ./deployment/helm
helm install dcs ./deployment/helm -f ./deployment/helm/values.dev.yml
```

This starts all dependencies as NodePort services forwarded to `localhost`:

| Service              | Address                          |
|----------------------|----------------------------------|
| PostgreSQL           | `localhost:30432`                |
| Keycloak             | `http://localhost:30080`         |
| NATS                 | `nats://localhost:30422`         |
| Neo4j HTTP           | `http://localhost:30474`         |
| Neo4j Bolt           | `bolt://localhost:30687`         |
| Federated Catalogue  | `http://localhost:30081`         |

The Keycloak `gaia-x` realm is imported automatically on first start.

> To upgrade after chart changes: `helm upgrade dcs ./deployment/helm -f ./deployment/helm/values.dev.yml`

### 2. Run the backend

```bash
cp backend/.env.dev backend/.env
cd backend && air
```

The backend listens on `http://localhost:8991`.

### 3. Run the frontend

```bash
cd frontend/ClientApp
npm install
npm run dev
```

The Vite dev server starts at `http://localhost:5173` and proxies `/api` requests to the backend automatically.

---

## BDD Tests

BDD scenarios live in `features/` at the project root. Tests are run against a full stack in an ephemeral [kind](https://kind.sigs.k8s.io/) cluster.

### Prerequisites
- `kind` — `go install sigs.k8s.io/kind@v0.23.0` or see [kind releases](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- `kubectl` and `helm`
- Docker (to build the DCS image)
- Python 3.10+

### Run locally

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

## Production Deployment

### Keycloak
- Use a properly secured external Keycloak instance (not the bundled sub-chart)
- Configure valid redirect URIs in your client settings:
  - **Valid Redirect URIs**: `https://<domain>/<path>/api/auth/callback`
  - **Valid Post Logout Redirect URIs**: `https://<domain>/<path>/api/auth/logout-complete`
- Enable **Client authentication**, **Standard flow enabled**

### TLS
- Use certificates from a trusted Certificate Authority
- Recommend [cert-manager](https://cert-manager.io/) for automatic renewal

### Values
Override the following at minimum:

```yaml
oidc:
  issuerURL: "https://keycloak.example.com/realms/gaia-x"
  clientID: "dcs-client"
  redirectURI: "https://example.com/dcs/ui/"
  logoutRedirectURI: "https://example.com/dcs/ui/"

route:
  basePath: "/dcs"
```

---

## License

Apache License 2.0. See [LICENSE](../LICENSE).
