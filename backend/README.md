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
Run the following command in **DCS/implementation/backend** to initialize all needed dependencies:
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
```

```
go test -v ./...
```
**Note:** Every time you modify files in **DCS/implementation/backend/design**, you must regenerate the code.

## Running the API Server

### Environment Variables
```bash
# Database configuration
export DATABASE_URL="user=username password=password dbname=postgres sslmode=disable"

# API routing
export API_PATH_PREFIX="/api"

# Federated Catalogue
export FEDERATED_CATALOGUE_API_URL="http://localhost:8081"

# OIDC/Keycloak Authentication
export OIDC_ISSUER_URL="https://keycloak.example.com/realms/yourrealm"
export OIDC_CLIENT_ID="digital-contracting-service"
export OIDC_REDIRECT_URI="http://localhost:5173/api/auth/callback"
export OIDC_LOGOUT_REDIRECT_URI="http://localhost:8991/api/auth/logout-complete"
```

### Start the DCS backend service
```bash
go run ./cmd/dcs
```

#### Example Request
```bash
curl http://0.0.0.0:8991/template/search
```

### Build a Docker image
To build a Docker image, you can use the helper script [build-image.sh](./build-image.sh).

**Important:** The Docker image embeds the frontend application. The build process:
1. Builds the Vue.js frontend from `../frontend/ClientApp`
2. Copies the built frontend into the backend image at `/app/web/dist`
3. The backend serves the frontend at `/ui` (root `/` redirects to `/ui`), keeping API routes at the root level

The build script must be run from the `backend/` directory, as it uses the parent directory (`implementation/`) as the Docker build context to access both backend and frontend code.

**Parameters:**
- `TAG` – Sets the image tag (default: `latest`)
- `REGISTRY` – Docker registry (environment variable)
- `REPO` – Docker repository (environment variable)

**Example:**
```bash
REGISTRY="your-registry" REPO="your-repo" ./build-image.sh v1.0.0
```

This builds a Docker image with the name: **your-registry/your-repo/digital-contracting-service:v1.0.0**