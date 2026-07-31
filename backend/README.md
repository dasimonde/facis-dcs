# DCS Backend Service

## Backend Project Structure
```
.
├── cmd/
│   ├── dcs/          # HTTP API server entrypoint
│   └── dcs-cli/      # (optional) CLI tooling
├── design/           # Goa DSL (API contracts)
│   ├── contract_storage_archive.go         # Design description for the Contract Storage Archive API
│   ├── contract_workflow_engine.go          # Design description for the Contract Workflow Engine API
│   ├── dcs_to_dcs.go                       # Design description for the DCS to DCS communication API
│   ├── design.go                           # Goa main design description
│   ├── external_system_api.go              # Design description for the external system communication API
│   ├── orchestration_webhook.go            # Design description for the orchestration webhooks API
│   ├── process_audit_and_compliance.go     # Design description for the Process Audit & Compliance Management API
│   ├── signature_management.go             # Design description for the Signature Management API
│   ├── template_catalogue_integration.go   # Design description for the Template Catalogue integration API
│   └── template_repository.go              # Design description for the Template Repository API
├── gen/              # Goa-generated transport & types (DO NOT EDIT)
├── internal
│   └── base/         # Files that are used by every DCS component
│   └── datatype/     # Used data types for the application
│   └── service/      # Application endpoint implementations
│   └── template_repository     # Implementation for the template repository component
├── go.mod
├── go.sum
└── README.md
```

## Development

### Dependencies
- Go **1.25+** – Installation: Follow the instructions on [Install Go](https://go.dev/learn/)
- Goa **v3** – Installation: Follow the instructions on [Goa Quickstart](https://goa.design/docs/1-goa/quickstart/)

### Setup the Backend

#### Initialize all dependencies
Run the following command in **backend/** to initialize all needed dependencies:
```bash
go mod tidy
```

#### Generate Go code with Goa
Generate the required glue code under `gen/` with the Goa CLI:
```bash
goa gen digital-contracting-service/design
```

## Running tests
```
export DATABASE_URL="user=username password=password dbname=test_postgres sslmode=disable"
export FC_KEYCLOAK_REALM_URL="http://localhost:30080/realms/gaia-x"
export FEDERATED_CATALOGUE_API_URL="http://localhost:30081"
export FEDERATED_CATALOGUE_CLIENT_ID="dcs-fc-client"
export FEDERATED_CATALOGUE_CLIENT_SECRET="dcs-fc-client-secret"
```

```
go test -v ./...
```
**Note:** Every time you modify files in **backend/design**, you must regenerate the code.

## Contract Signing

The DCS is the OID4VP relying party and signature validator; it holds no
contract-signing key (ADR-12, ADR-20). The only signing path is the ceremony:

1. `POST /signature/request` — start a ceremony, request a PID (+ Power of
   Attorney) presentation from the signer's wallet.
2. The wallet presents PID+PoA (direct_post `vp_token`, keyed by the DCQL
   credential query ids) to the ceremony's own callback
   (`POST /signature/request/{ceremony_id}/callback`) — verified
   cryptographically against the ceremony's nonce and the configured PID
   issuer trust anchors (`OID4VP_TRUST_DATA_PATH`) before anything is
   persisted. A PID whose issuer credential carries an x5c certificate
   (a real EUDI wallet, rather than this project's JWKS-only dev issuer) is
   only accepted if its chain verifies against `OID4VP_X5C_TRUST_ANCHORS_PATH`
   (a PEM bundle of trusted roots) — unset in dev/BDD, where no PID is ever
   x5c-signed; an x5c-bearing credential presented with none configured is
   refused outright, never trusted off its own embedded certificate.
3. `POST /signature/request/{ceremony_id}/publish` — prepare the to-be-signed
   PDF and JSON-LD payload (evidence embedded, bytes pinned), and publish a
   standard OID4VP Document-Retrieval request object as a QR/deep link.
4. The wallet fetches the request object, fetches both documents, signs the
   PDF (PAdES, via its own SCA/QTSP) and the JSON-LD payload (JAdES, with the
   ceremony's request nonce bound into the protected header), and posts both
   back to the SAME callback endpoint from step 2 (`documentWithSignature[]`
   / `signatureObject[]`).
5. The callback validates: the submitted PDF's initial revision byte-matches
   what was pinned at prepare, the DSS-validated signature meets the
   contract's declared level (AES or QES, per `dcs:requiredCredentialType` on
   the signature field), the certificate names the ceremony's verified PID,
   and (once a ceremony is published) the JAdES's nonce claim matches. Only
   then does it finalize and transition the contract to SIGNED.

`POST /signature/prepare` + `POST /signature/submit` are the same
prepare/validate steps without the QR/publish layer, for a JWT-authenticated
caller who already has the signatory's signed PDF in hand (e.g. a desktop
PAdES signer) — same acceptance gate, no separate ceremony webhook.

There is no DCS-signed-PAdES fallback: `POST /signature/apply` and the
`dcs-contract-pades` HSM key are removed. See
[docs/adr-12-wallet-driven-signing.md](../docs/adr-12-wallet-driven-signing.md)
and [docs/adr-20-signing-acceptance-hardening.md](../docs/adr-20-signing-acceptance-hardening.md)
for the full acceptance-path decision record, and
[testWallet/](../testWallet/) for the wallet+QTSP stand-in that drives this
path in dev and CI.

## Running the API Server

For the local Helm stack, see [deployment/README.md](../deployment/README.md).

### Environment Variables
```bash
# Database configuration
export DATABASE_URL="user=username password=password dbname=postgres sslmode=disable"

# API routing
export API_PATH_PREFIX="/api"

# Federated Catalogue
export FC_KEYCLOAK_REALM_URL="http://localhost:30080/realms/gaia-x"
export FEDERATED_CATALOGUE_API_URL="http://localhost:8081"
export FEDERATED_CATALOGUE_CLIENT_ID="dcs-fc-client"
export FEDERATED_CATALOGUE_CLIENT_SECRET="dcs-fc-client-secret"

# Hydra Authentication
export HYDRA_PUBLIC_ISSUER_URL="http://localhost:5173"
export HYDRA_INTERNAL_ISSUER_URL="http://localhost:30444"
export HYDRA_CLIENT_ID="dcs-client"
export HYDRA_CLIENT_SECRET="dcs-secret"
export HYDRA_REDIRECT_URI="http://localhost:5173/api/auth/callback"
export HYDRA_POST_LOGOUT_REDIRECT_URI="http://localhost:5173/api/auth/logout-complete"
export HYDRA_ADMIN_URL="http://localhost:30085"
```

When using the local Helm stack, copy `backend/.env.dev1` to `backend/.env` (done automatically by `dev-stack.sh`).

### Start the DCS backend service
```bash
go run ./cmd/dcs
```

### Development with Live Reload
To enable live reloading during development, install and use [air](https://github.com/cosmtrek/air):

```bash
# Install air (one-time)
go install github.com/cosmtrek/air@latest

# Run backend with live reload
air
```

Air watches for file changes in the backend and automatically rebuilds and restarts the service. Configuration is defined in `.air.toml`.

#### Example Request
```bash
curl http://0.0.0.0:8991/template/search
```

### Build a Docker image
To build a Docker image, use the helper script [deployment/docker/build-image.sh](../deployment/docker/build-image.sh).

**Important:** The Docker image embeds the frontend application. The build process:
1. Builds the Vue.js frontend from `frontend/ClientApp`
2. Copies the built frontend into the backend image at `/app/web/dist`
3. The backend serves the frontend at `/ui` (root `/` redirects to `/ui`), keeping API routes at the root level

The Dockerfile and build script live in `deployment/docker/`. The script resolves the repo root automatically as the Docker build context.

**Parameters:**
- `TAG` – Sets the image tag (default: `latest`)
- `REGISTRY` – Docker registry (environment variable)
- `REPO` – Docker repository (environment variable)

**Example:**
```bash
REGISTRY="your-registry" REPO="your-repo" ./deployment/docker/build-image.sh v1.0.0
```

This builds a Docker image with the name: **your-registry/your-repo/digital-contracting-service:v1.0.0**

## Linting

This project uses **[golangci-lint](https://golangci-lint.run)** for static code analysis.

Linting is automatically executed via a **pre-commit hook** before each commit, but you can also run it manually using the commands below.

### Prerequisites

Ensure `golangci-lint` is installed in the project's `./bin` directory. If it is not already present, follow the installation steps below.

### Installation (Optional)

> **Note:**
> If you have already committed code in this repository, the pre-commit hook should have automatically installed the linter for you.

If `golangci-lint` is not yet installed in `./bin`, run:

```bash
# Ensure the ./bin directory exists and install golangci-lint to the ./bin directory
mkdir -p ./bin && curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b ./bin v2.12.2
```

Alternatively, if you have `golangci-lint` installed globally, you can copy it:

```bash
# Create directory and copy the global installation to ./bin
mkdir -p ./bin && cp $(which golangci-lint) ./bin/golangci-lint
```

### Manual Linting

To run the linter manually, use the following commands:

```bash
# Run linter on all files
./bin/golangci-lint run

# Run linter on a specific package
./bin/golangci-lint run ./cmd/...

# Run linter with verbose output
./bin/golangci-lint run -v

# Run linter and fix auto-fixable issues
./bin/golangci-lint run --fix
```

You can also check the official [golangci-lint documentation](https://golangci-lint.run/docs) for additional configuration and command options.