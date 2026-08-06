# Digital Contracting Service (DCS)

[![Behavior Tests](https://github.com/eclipse-xfsc/facis-dcs/actions/workflows/bdd-kind.yml/badge.svg)](https://github.com/eclipse-xfsc/facis-dcs/actions/workflows/bdd-kind.yml)

The **Digital Contracting Service** provides an **open-source platform** for creating, signing, and managing contracts digitally.  
It is designed for integration with the **European Digital Identity Wallet (EUDI)** ecosystem and targets **eIDAS 2.0 Advanced Electronic Signatures (AES)**, so that organizations can streamline business processes, reduce paperwork, and establish trust across federated partners.

> **Status.** DCS is not certified and has not been conformance-tested against a
> production EUDI wallet or a qualified trust service provider. Wallet
> interaction is exercised against the project's own test wallet, signatures are
> AES-targeted rather than qualified, and no legal effect is claimed for them.
> The decision records under [`docs/`](docs/) state what each mechanism does and
> does not establish.

**The detailed specifications for the Digital Contracting Service (DCS) can be found: [SRS_FACIS_DCS](https://github.com/eclipse-xfsc/facis/tree/main/DCS/specification/SRS_FACIS_DCS.pdf).**

## Repository Layout

| Path | Contents |
|------|----------|
| `backend/` | Go service (Goa v3) serving the API on :8991 — [backend/README.md](./backend/README.md) |
| `frontend/ClientApp/` | Vue 3 + Vite + Pinia frontend (:5173 in dev) |
| `pdf-core/` | PDF assembly service (PAdES envelope, C2PA embedding, :8080 in dev) |
| `deployment/` | Docker build and Helm chart — [deployment/README.md](./deployment/README.md) |
| `tests/bdd/`, `features/`, `steps/` | Python/behave BDD suite (runs on kind in CI) |
| `testWallet/` | Dev wallet used for OpenID4VP presentation flows in dev and tests |
| `scripts/` | Provisioning helpers (SoftHSM2 token, C2PA/PAdES cert chains, CRL) |

## Development Setup

Run `npm install` in the **project root**. This installs **Husky** and registers the pre-commit hooks. If you skip this, you cannot commit code, and your Pull Request will fail the CI pipeline.

```bash
# Execute this in the root directory
npm install
```

### Prerequisites

- A local Kubernetes with `kubectl` and `helm` (Rancher Desktop with Kubernetes enabled works)
- Go 1.25+ and [air](https://github.com/air-verse/air) (backend hot reload)
- Node 22+
- SoftHSM2 (`libsofthsm2.so`, e.g. `apt install softhsm2`) — dev private keys live in a PKCS#11 token, not in files
- `make`, `curl`, OpenSSL

### Quick Start

From the project root:

```bash
bash dev-stack.sh
```

The script is idempotent (`helm upgrade --install`) and, in order:

1. Deploys the Helm dependency stack to your current Kubernetes context (PostgreSQL, Keycloak, Hydra, NATS, Neo4j, ORCE, Federated Catalogue, IPFS + document manager, statuslist-service, Traefik, kube-prometheus-stack) and waits for the slow pods.
2. Installs testWallet dependencies and initializes the dev status list.
3. Copies `backend/.env.dev1` → `backend/.env`, provisions the SoftHSM2 token (`~/.dcs/softhsm-8991`) and regenerates the instance DID document from the token key.
4. Issues the C2PA and PAdES certificate chains for pdf-core and publishes the dev CA's initial CRL.
5. Starts **pdf-core** (air, :8080, log at `/tmp/pdf-core-live.log`), the **frontend** (Vite, :5173), and the **backend** (goa gen + air, :8991, log at `/tmp/backend-live.log`).

Ctrl+C stops all three processes. The frontend proxies `/api` to the backend, so http://localhost:5173 is the place to start.

### Second Instance (inter-organization demo)

For flows that need two independent organizations (offer/accept across instances), start instance B **after** instance A is up — pdf-core is shared:

```bash
bash dev-stack2.sh
```

This deploys its own Helm release (`dcs2`), provisions a separate SoftHSM2 token and DID (`~/.dcs/softhsm-8992`), and runs a second backend on :8992 with its frontend on :5174.

For step-by-step manual commands and troubleshooting, see [deployment/README.md](./deployment/README.md#local-development).

## Contract Signing

The DCS holds no contract-signing key: a contract is signed by the
signatory's own wallet over a standard OID4VP "Document Retrieval" ceremony
(prepare → publish → wallet fetches and signs → callback validates and
records) — see [backend/README.md](./backend/README.md#contract-signing) and
[docs/adr-12-wallet-driven-signing.md](./docs/adr-12-wallet-driven-signing.md)
/ [docs/adr-20-signing-acceptance-hardening.md](./docs/adr-20-signing-acceptance-hardening.md).
There is no other signing path — `testWallet/` plays the wallet+QTSP stand-in
for local dev and CI (see [testWallet/](./testWallet/)).

## Tests

Backend unit tests:

```bash
cd backend && go test ./...
```

The BDD suite lives in `tests/bdd/` (scenarios in `features/`). It runs hermetically on a kind cluster in CI (`bdd-kind.yml`); locally:

```bash
cd tests/bdd
make help          # all targets
make run_bdd_helm  # full run against a freshly deployed stack
make run_bdd_fast  # iterate on scenarios against an already-running stack
```

To run the suite against two DCS deployments you operate yourself instead of the
ephemeral kind stack, see [tests/bdd/README.md](./tests/bdd/README.md).
