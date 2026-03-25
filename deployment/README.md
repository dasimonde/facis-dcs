[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](../LICENSE)

# Digital Contracting Service

An automated orchestration workspace that deploys a [Digital Contracting Service](https://github.com/eclipse-xfsc/facis/tree/main/DCS) instance to a Kubernetes cluster.

---

## 🚀 Overview

The Digital Contracting Service (DCS) provides an open-source platform for creating, signing, and managing contracts digitally.
Integrated with the European Digital Identity Wallet (EUDI), it guarantees that all digital transactions are secure, legally binding, and interoperable.
DCS allows organizations to streamline business processes, reduce paperwork, and ensure compliance with eIDAS 2.0 regulations, while fostering trust across federated partners.

Key components of the Digital Contracting Service include:
- Multi-Contract Signing: Enables multi-party contract execution within a single integrated workflow.
- Automated Workflows: Automates contract generation, execution, and deployment to ensure legal
consistency and efficiency.
- Lifecycle Management: Monitors contracts with alerts for renewals, expirations, or required actions.
- Signature Management: Links contract signatures to verifiable digital identities to maintain legal validity
and trust.
- Secure Archiving: Stores signed contracts in a tamper-evident archive compliant with retention policies.
- Machine Signing: Supports automated signing for high-volume or routine transactions.

This module allows you to set up and interact with the Digital Contracting Service visually inside the ORCE environment. You don’t need to write any code or handle any complex API integration manually—just install the Node-RED node for Digital Contracting Service, drop it into your flow, and configure the endpoint and query.

Thanks to ORCE’s orchestration features, deploying a Digital Contracting Service instance and querying it happens in just a few clicks. Upload your configs, drag your node, and start the Digital Contracting Service

---

## Helm Chart Composition (Bundled + Optional Dependencies)

The chart bundles `postgresql`, `keycloak`, and `nats` as dependencies, but each can be enabled or disabled independently.

- Enable bundled dependencies when needed:
  - `postgresql.enabled=true`
  - `keycloak.enabled=true`
  - `nats.enabled=true`
- Or disable them and point DCS to external services:
  - `serviceDiscovery.postgresqlHost`
  - `serviceDiscovery.keycloakHost`
  - `serviceDiscovery.natsHost`

For DCS routing paths, configure:

- `route.basePath` for a single base route (example: `/tenant-a/dcs`)
- `paths.api` and `paths.ui` for explicit API/UI path overrides

Ingress is disabled by default and uses standard Kubernetes Ingress resources.

---

## ⚡️ Click-to-Deploy

---
## Prerequisites

Before running the deploy script, ensure you have the following:

### System Tools
The following CLI tools must be installed and accessible in your PATH:
- **kubectl** - Kubernetes command-line tool
- **helm** - Package manager for Kubernetes
- **jq** - Command-line JSON processor
- **curl** - Data transfer tool
- **sed** - Stream editor
- **openssl** - For generating TLS certificates and private keys
- **ssh-keygen** - For SSH key generation

### Kubernetes Cluster
- A working Kubernetes cluster
- An ingress controller installed in the cluster (for example NGINX or Traefik) when `ingress.enabled=true`

### Files & Credentials
- **Kubeconfig file**: Path to your Kubernetes cluster configuration (e.g., `~/.kube/config`)
- **TLS Private Key**: Path to your TLS certificate private key (PEM format)
- **TLS Certificate**: Path to your TLS certificate (PEM format, must match your domain)
- **Domain**: A domain name where the DCS service will be accessible
- **URL Path**: A unique path identifier for this DCS instance

**Note**: See the **"Dev Setup"** section below for instructions on generating self-signed certificates for development.

### Keycloak Setup
- **Running Keycloak instance** accessible to both:
  - Your Kubernetes cluster (for backend validation)
  - End-user browsers (for authentication flow)
- A configured realm with an OIDC client
- The Keycloak issuer URL must be reachable from both contexts

**For development setup**, see the **"Dev Setup: Windows + Rancher Desktop + WSL"** section below for detailed Keycloak deployment and configuration steps.

**For production**, use a properly secured external Keycloak instance with TLS and valid DNS.

---

## 🖥️ Dev Setup: Windows + Rancher Desktop + WSL

This guide walks through setting up DCS for local development on Windows with Rancher Desktop and WSL2.

### Prerequisites
- **Windows 10/11** with WSL2 enabled
- **Rancher Desktop** installed and running (with Kubernetes enabled)
- **WSL2 distro** (Ubuntu recommended) with kubectl, helm, jq, curl installed

### Step 1: Generate Self-Signed Certificates

First, generate the TLS certificates that will be used for both Keycloak and DCS:

```bash
# Create certs directory
mkdir -p ./certs

# Generate private key
openssl genrsa -out ./certs/dev.key 2048

# Generate self-signed certificate (valid for 365 days)
openssl req -new -x509 -key ./certs/dev.key -out ./certs/dev.crt -days 365 \
  -subj "/CN=*.xfsc.local/O=Dev/C=US" \
  -addext "subjectAltName=DNS:*.xfsc.local,DNS:xfsc.local,DNS:keycloak.xfsc.local,DNS:dcs.xfsc.local"
```

### Step 2: Trust the Certificate

#### Browser
On edge, this is under Settings -> Privacy,search and services -> Security -> Manage certificates -> Custom -> Import

#### Windows

1. Copy the certificate out of the WSL into the Windows host:
   ```bash
   cp ./certs/dev.crt /mnt/c/Users/$USER/Downloads/
   ```

2. In Windows:
   - Open **PowerShell as Administrator**
   - Import the certificate:
     ```powershell
     Import-Certificate -FilePath "$env:USERPROFILE\Downloads\dev.crt" -CertStoreLocation Cert:\LocalMachine\Root
     ```
   - Or double-click `dev.crt` → Install Certificate → Local Machine → Place in "Trusted Root Certification Authorities"

#### Ubuntu

Copy the certificate into the ca-certificates custom folder.

```bash
sudo mkdir -p /usr/local/share/ca-certificates/custom/ # create custom folder, -p skips if already there
sudo cp ./certs/dev.crt /usr/local/share/ca-certificates/custom/ # copy cert into that custom folder
sudo update-ca-certificates # update certificate store
```

### Step 3: Deploy Keycloak with HTTPS

Deploy Keycloak using the provided script which automatically handles TLS configuration:

```bash
cd /path/to/DCS/implementation/deployment
./deploy-dev-keycloak.sh
```

**What this script does:**
- Creates the `keycloak` namespace
- Deploys Keycloak from keycloak-quickstarts
- Creates TLS secret `dev-wildcard-tls` with your certificate
- Creates Traefik ingress with HTTPS (websecure entrypoint)
- Configures TLS termination for `keycloak.xfsc.local`

The script uses these environment variables (with defaults):
- `KEYCLOAK_HOST` (default: `keycloak.xfsc.local`)
- `KEYCLOAK_NAMESPACE` (default: `keycloak`)
- `TLS_CERT_FILE` (default: `./certs/dev.crt`)
- `TLS_KEY_FILE` (default: `./certs/dev.key`)

### Step 4: Set Up WSL Port Forwarding for HTTPS

To avoid requiring `sudo` for kubectl port-forward, we bind to unprivileged port 8443, then use `sudo socat` to redirect privileged port 443 to 8443.

```bash
# 1. Start kubectl port-forward for HTTPS (tunnels WSL → Traefik)
kubectl port-forward -n kube-system svc/traefik 8443:443 --address=0.0.0.0 > /tmp/traefik-forward-https.log 2>&1 &
echo $! > /tmp/traefik-forward-https.pid

# 2. Install socat (if needed)
sudo apt-get update && sudo apt-get install -y socat

# 3. Get your WSL IP
WSL_IP=$(ip addr show eth0 | grep "inet " | awk '{print $2}' | cut -d/ -f1)
echo "WSL IP: $WSL_IP"

# 4. Forward port 443 → 8443 using socat
sudo socat TCP-LISTEN:443,bind=$WSL_IP,reuseaddr,fork TCP:$WSL_IP:8443 > /tmp/socat-443.log 2>&1 &
echo $! | sudo tee /tmp/socat-443.pid
```

### Step 5: Hosts File Changes

You must manually map the shared hostnames to your WSL IP so both WSL and Windows can resolve Keycloak and DCS.

#### WSL (/etc/hosts)
1. Open the hosts file with sudo (use your editor of choice):
   ```bash
   sudo nano /etc/hosts
   ```
2. Add or update a single line (replace with actual IP that your development host can be reached by):
   ```
   172.29.35.79 keycloak.xfsc.local xfsc.local
   ```
3. Save and close.

4. Same steps apply to Windows (C:\Windows\System32\drivers\etc\hosts)

### Step 6: Verify Keycloak Access

Test from both WSL and Windows:

```bash
# From WSL
curl -Ik https://keycloak.xfsc.local/
```

From Windows browser: Open [https://keycloak.xfsc.local](https://keycloak.xfsc.local)

You should see the Keycloak login page. Default admin credentials: `admin/admin`

### Step 7: Configure Keycloak

#### 7.1 Create a Realm
1. Log in to Keycloak at [https://keycloak.xfsc.local](https://keycloak.xfsc.local) - default `admin:admin` credentials.
2. Click the realm dropdown (top-left, says "master")
3. Click **"Create Realm"**
4. Enter realm name: `dcs`
5. Click **"Create"**

#### 7.2 Create the OIDC Client
1. In the `dcs` realm, go to **Clients** (left sidebar)
2. Click **"Create client"**
3. **Client ID**: `digital-contracting-service`
4. **Client type**: OpenID Connect
5. Click **"Next"**
6. Click **"Save"**

#### 7.3 Create Client Roles

The DCS backend enforces role-based access control using Keycloak **client roles** (read from the `resource_access.<client_id>.roles` JWT claim). You must create the following roles under the `digital-contracting-service` client:

1. In the `dcs` realm, go to **Clients** → **digital-contracting-service**
2. Go to the **Roles** tab
3. Click **"Create role"** and add each of the following roles (one at a time):

| Role | Description |
|------|-------------|
| `Archive Manager` | Manage archived contracts and evidence |
| `Contract Observer` | Read-only access to archived contracts |
| `Contract Creator` | Create new contract drafts |
| `Sys. Contract Creator` | System-level contract creation |
| `Contract Negotiator` | Negotiate contract terms |
| `Contract Reviewer` | Review submitted contracts |
| `Sys. Contract Reviewer` | System-level contract review |
| `Contract Approver` | Approve or reject contracts |
| `Sys. Contract Approver` | System-level contract approval |
| `Contract Manager` | Manage contract lifecycle |
| `Sys. Contract Manager` | System-level contract management |
| `Contract Signer` | Sign contracts digitally |
| `Sys. Contract Signer` | System-level contract signing |
| `Template Creator` | Create new templates |
| `Template Reviewer` | Review submitted templates |
| `Template Approver` | Approve or reject templates |
| `Template Manager` | Manage template lifecycle |
| `Auditor` | Perform audits and generate reports |
| `Compliance Officer` | Monitor compliance and report incidents |
| `System Administrator` | Maintains system configurations, permissions, and user access |

4. Assign roles to users:
   - Go to **Users** → select a user → **Role mapping** tab
   - Click **"Assign role"**
   - Filter by client: **digital-contracting-service**
   - Select the desired roles and click **"Assign"**

> **Note**: A user's client roles appear in the JWT under `resource_access.digital-contracting-service.roles`. The DCS backend checks these roles against the required scopes declared for each API endpoint. If a user lacks the required role, the request is rejected with **403 Forbidden**.

#### 7.4 Configure Redirect URIs (Required for OAuth)

For the OAuth authorization code flow to work, you must configure valid redirect URIs in your OIDC client:

1. In your client settings, scroll to **Valid redirect URIs**
2. Add your application's callback URL based on your environment:

   **Development (localhost):**
   ```
   http://localhost:8991/auth/callback
   ```

   **Production (domain-based):**
   ```
   https://<domain>/<path>/auth/callback
   ```
   Example: `https://xfsc.local/dcs/auth/callback`

3. Add **Valid post logout redirect URIs** (optional but recommended):
   ```
   https://<domain>/<path>/*
   ```
   Example: `https://xfsc.local/dcs/*`

4. Click **"Save"**

> **Important**: The redirect URI must match exactly what's configured via `OIDC_REDIRECT_URI` environment variable with `/auth/callback` appended. If you get "Incorrect redirect_uri" error, verify the callback URL matches what's registered in Keycloak.

**OAuth Flow Overview:**
```
User → Frontend (/auth/login) → Keycloak login
                                   ↓ (user authenticates)
Frontend ← Keycloak (redirects with auth code to /auth/callback)
    ↓
    ↓ (exchange code for tokens at /auth/callback endpoint)
    ↓
Keycloak → /auth/callback endpoint (returns access + refresh tokens)
    ↓
/auth/callback sets refresh token as HttpOnly cookie,
displays access token to the frontend
    ↓
DCS API ← Frontend (calls API with access token)
    ↓
When access token expires:
Frontend → POST /auth/refresh (cookie sent automatically)
    ↓
/auth/refresh exchanges refresh token at Keycloak → new access token
```

**Note**: If you skip this step, the OAuth authorization code flow will fail with "Incorrect redirect_uri" error. Only the direct grant (password) flow works without redirect URIs, but that's not recommended for production apps.

#### 7.5 Create a Test User
1. Go to **Users** (left sidebar)
2. Click **"Create new user"**
3. **Username**: `test`
4. Click **"Create"**
5. Go to the **Credentials** tab
6. Click **"Set password"**
7. Enter password: `test`
8. Toggle OFF: **Temporary** (so you don't need to reset on first login)
9. Click **"Save"**

#### 7.6 Configure `participant-id` claim (required by DCS)

The DCS backend reads the JWT claim named `participant-id` to determine the “current participant” of the DCS instance (used e.g. by `/catalogue/participant/current` and as the participant identifier for Federated Catalogue calls).

Configure this claim on the `digital-contracting-service` client in Keycloak:

1. In Keycloak, open **Clients** → **digital-contracting-service** → **Client scopes** → **digital-contracting-service-dedicated**
2. Click **Configure a new mapper**, select **Hardcoded claim**:
   - **Mapper type**: `Hardcoded claim`
   - **Name**: `participant-id`
   - **Token Claim Name**: `participant-id`
   - **Claim value**: your participant DID, e.g. `did:web:<dcs-domain>:facis:participant:<participant-uuid>`
   - **Claim JSON Type**: `String`
3. Enable:
   - **Add to ID token**: `On`
   - **Add to access token**: `On`
4. Click **Save**

### Step 8: Deploy DCS image in the cluster

Deploy the DCS service with automated HTTPS, CA trust, and in-cluster DNS configuration:

```bash
# Set environment variables for custom registry (optional - defaults to upstream)
export DOCKER_REGISTRY="h6s71ks6.c1.de1.container-registry.ovh.net"
export DOCKER_REPO="facis"
export DOCKER_TAG="oidc"

# Enable custom CA certificate trust
export CUSTOM_CA_ENABLED="true"
export CUSTOM_CA_CONFIGMAP="dev-ca-cert"
export CUSTOM_CA_CERT_FILE="./certs/dev.crt"

# Run the deployment script
./deploy.sh \
  ~/.kube/config \
  ./certs/dev.key \
  ./certs/dev.crt \
  xfsc.local \
  dcs \
  https://keycloak.xfsc.local/realms/dcs \
  digital-contracting-service
```

**What this script does:**
- Creates the DCS namespace
- Creates CA ConfigMap (if `CUSTOM_CA_ENABLED=true`)
- Detects Traefik ClusterIP and configures hostAliases for in-cluster DNS resolution
- Deploys DCS via Helm with HTTPS ingress (websecure entrypoint + TLS)
- Creates TLS secrets for both the application and ingress
- Waits for deployment to be ready

**Important**: The OIDC issuer URL must use the shared hostname (`keycloak.xfsc.local`) so that both:
- The DCS backend running in Kubernetes can reach it (via hostAlias → Traefik ClusterIP)
- Your browser/frontend can reach it (via hosts file → WSL IP)

This ensures the JWT token's `iss` claim matches what the backend expects.

Once deployed, you can access:
- Keycloak: [https://keycloak.xfsc.local](https://keycloak.xfsc.local)
- DCS API: [https://xfsc.local/dcs/digital-contracting-service/](https://xfsc.local/dcs/digital-contracting-service/)

The DCS API will require valid JWT tokens from Keycloak. Requests without authentication will receive a **401 Unauthorized** response.

### Restarting After WSL Shutdown

The port forwards will stop when WSL restarts. To restart them:

```bash
# Forward HTTPS (443)
kubectl port-forward -n kube-system svc/traefik 8443:443 --address=0.0.0.0 > /tmp/traefik-forward-https.log 2>&1 &
WSL_IP=$(ip addr show eth0 | grep "inet " | awk '{print $2}' | cut -d/ -f1)
sudo socat TCP-LISTEN:443,bind=$WSL_IP,reuseaddr,fork TCP:$WSL_IP:8443 > /tmp/socat-443.log 2>&1 &
```

**Tip**: Create a shell script to automate this or use a systemd service.

---

## ⚙️ Advanced: Production Setup

For production deployments:

### Keycloak Configuration
- Use a properly secured external Keycloak instance (not the quickstart)
- Configure valid redirect URIs in your client settings:
  - **Valid Redirect URIs**: For login callback (backend)
    - Add: `https://<your-domain>/<path>/api/auth/callback`
    - Example: `https://example.com/dcs/api/auth/callback`
  - **Valid Post Logout Redirect URIs**: For logout callback (backend)
    - Add: `https://<your-domain>/<path>/api/auth/logout-complete`
    - Example: `https://example.com/dcs/api/auth/logout-complete`
- Enable **Client authentication**, **Authorization**, **Standard flow enabled**
- Consider using a service account with proper RBAC for automation

### TLS Certificates
- Use certificates from a trusted Certificate Authority (not self-signed)
- Ensure certificates match your domain name
- Set up automatic certificate renewal (e.g., with cert-manager)

---

## 🛠️ How to Use

### 1. Prepare the environment and prerequisites
You'll need:
1.1. A Kubernetes cluster to host the child instances
1.2. A local ORCE as the parent to host the initial developing environment

### 1.1. Kubernetes
"Orchestration Engine" node requires a working Kubernetes cluster with ingress installed on it. Initiate a K8s cluster and install nginx-ingress on it using this command.
```bash
export KUBECONFIG=`<YOUR KUBECONFIG PATH>`
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.12.3/deploy/static/provider/cloud/deploy.yaml
```
You can learn more by reading the [official documentation](https://kubernetes.github.io/ingress-nginx/deploy/)
After this step, you can proceed to step 1.2 (Installing a local ORCE)

### 1.2. Local ORCE
Install ORCE as described in the [ORCE page](https://github.com/eclipse-xfsc/orchestration-engine):
```bash
docker run -d --name xfsc-orce-instance -p 1880:1880 leanea/facis-xfsc-orce:1.0.16
```
Go to [http://localhost:1880](http://localhost:1880).

### Install Digital Contracting Service Node
Click on "New Node" in the sidebar.

![new button](./docImage/add-new-node.jpg?raw=true)

Upload `node-red-contrib-digital-contracting-service-0.0.1.tgz` from this repository and install. Refresh to activate the node.


### 2. Run the Deploy Script

Once all prerequisites are in place, you can deploy the Digital Contracting Service using the deploy script:

```bash
./deploy.sh \
  <kubeconfig> \
  <private_key_path> \
  <crt_path> \
  <domain> \
  <path> \
  <realm> \
  <oidc_client_id>
```

**Parameters:**

1. **`<kubeconfig>`** - Path to your Kubernetes configuration file
   - Example: `~/.kube/config`
   - This file contains cluster credentials and connection details
   - The script will use this to deploy resources to your cluster

2. **`<private_key_path>`** - Path to your TLS private key file (PEM format)
   - Example: `./certs/server.key`
   - Used to create the TLS secret for HTTPS ingress
   - Must match the certificate in the next parameter

3. **`<crt_path>`** - Path to your TLS certificate file (PEM format)
   - Example: `./certs/server.crt`
   - Used to create the TLS secret for HTTPS ingress
   - Must be valid for the domain specified in parameter 4

4. **`<domain>`** - The base domain where your DCS will be accessible
   - Example: `example.com` or `xfsc.local`
   - The full URL will be: `https://<domain>/<path>/`
   - Must have DNS pointing to your ingress controller's external IP

5. **`<path>`** - URL path prefix for this DCS instance
   - Example: `dcs` (creates `https://example.com/dcs/`)
   - Allows multiple DCS instances on the same domain
   - Used as namespace identifier: `digital-contracting-service-<path>`

6. **`<realm>`** - Keycloak realm name for authentication
   - Example: `dcs`
   - Must match the realm you created in Keycloak
   - Used to construct the OIDC issuer URL

7. **`<oidc_client_id>`** - OIDC client ID registered in Keycloak
   - Example: `digital-contracting-service`
   - Must match the client you created in the Keycloak realm
   - Used by the backend to validate JWT tokens

**Environment Variables (optional):**

- **`DOCKER_REGISTRY`** - Docker registry URL
  - Default: Docker Hub
  - Use if your image is in a private registry

- **`DOCKER_REPO`** - Docker repository namespace
  - Default: `facis`
  - Example: `myorg`
  - Combined with registry to form: `<registry>/<repo>/digital-contracting-service`

- **`DOCKER_TAG`** - Image tag to deploy
  - Default: `latest`
  - Example: `v1.2.3` or `oidc`
  - Use specific tags for version control

- **`OIDC_ISSUER_URL`** - Full OIDC issuer URL
  - **No default - must be set explicitly**
  - Example: `https://keycloak.example.com/realms/dcs` or `https://keycloak.xfsc.local/realms/dcs`
  - **Critical**: Must be a URL reachable by both:
    - The backend pods running in Kubernetes
    - End-user browsers accessing your application
  - The JWT token's `iss` claim must exactly match this URL
  - **Do not use in-cluster URLs** (like `keycloak.default.svc.cluster.local`) - they only work inside the cluster and will cause token validation to fail

- **`OIDC_CLIENT_ID`** - OIDC client ID registered in Keycloak
  - **No default - must be set explicitly**
  - Example: `digital-contracting-service`
  - Must match the client you created in the Keycloak realm
  - Used by the backend to validate JWT tokens

- **`OIDC_REDIRECT_URI`** - The base URI for OIDC authentication flow (**required**)
  - Example: `https://xfsc.local/dcs` or `http://localhost:8991`
  - Must be registered as a valid redirect URI in your Keycloak client configuration
  - Used by the login and callback handlers to build the authorization and token exchange URLs
  - Note: `deploy.sh` defaults to `http://localhost:8991` if unset, but the backend requires it explicitly

- **`OIDC_LOGOUT_REDIRECT_URI`** - The post-logout redirect URI when user logs out
  - **Optional** - if not set, defaults to `https://<domain>/<path>/api/auth/logout-complete`
  - This is a **backend URL** where Keycloak redirects after logout
  - Example: `https://example.com/api/dcs/auth/logout-complete` or `http://localhost:8991/api/auth/logout-complete`
  - Must be registered as a valid post-logout redirect URI in your Keycloak client configuration
  - The backend's `/api/auth/logout-complete` endpoint receives this redirect, clears the cookie, and redirects to frontend home
  - **In development**: Add the backend logout URL to Keycloak
  - **In production**: Ensure the backend logout URL is registered in the Keycloak client settings

- **`API_PATH_PREFIX`** - Optional API base path prefix added by reverse proxies
  - Default: empty
  - Example: `/api` or `/gateway/dcs`
  - Used e.g. by backend cookie path construction: `<API_PATH_PREFIX>/auth/refresh`

- **`FEDERATED_CATALOGUE_API_URL`** - Base URL of the Federated Catalogue API used by the Template Catalogue integration
  - Default: empty
  - Must be reachable from the backend pods
  - If unset, calls to the Federated Catalogue endpoints will fail

**Example:**
```bash
# Development deployment with shared hostname
export OIDC_ISSUER_URL="https://keycloak.xfsc.local/realms/dcs"
export OIDC_REDIRECT_URI="http://localhost:8991"
./deploy.sh \
  ~/.kube/config \
  ./certs/server.key \
  ./certs/server.crt \
  xfsc.local \
  dcs \
  dcs \
  digital-contracting-service

# Production deployment with external Keycloak
export OIDC_ISSUER_URL="https://keycloak.example.com/realms/dcs"
export OIDC_REDIRECT_URI="https://example.com/dcs"
./deploy.sh \
  ~/.kube/config \
  ./certs/server.key \
  ./certs/server.crt \
  example.com \
  dcs \
  dcs \
  digital-contracting-service
```

**What the script does:**
1. Verifies all required CLI tools are installed
2. Validates the kubeconfig file and cluster connectivity
3. Checks for Traefik ingress controller in the cluster
4. Creates the deployment namespace: `digital-contracting-service-<path>`
5. Replaces placeholders in Helm chart values
6. Creates TLS secrets from your certificate files
7. Deploys the Helm chart with all configurations
8. Waits for pods to be ready and reports the service URL

---

### 3. Install your node
Click on the "Install" tab. Then on the upload icon. The node will be successfully installed.
![step two (flow)](./docImage/newstep.png?raw=true)


### 4. Create your flow
Drag in an Inject node, the **Digital Contracting Service** node, and a Debug node. Connect them:

![step three (flow)](./docImage/create-your-flow.png?raw=true)


### 5. Name your instance and configure the node
Double-click on the Digital Contracting Service node to open the edit dialog.
In this step, you must choose a **Digital Contracting Service Name**. This will become your instance’s unique identifier, so it must be:
- Unique (not used by any other instance)
- Free of special characters (letters and numbers only)
For example, if you name it `mydcs`, it will be used internally for instance referencing and must remain distinct.
![step four (flow)](./docImage/step2.png?raw=true)


### 6. Provide your kubeconfig file
In this tab, you need to provide the **kubeconfig** file of your target Kubernetes cluster.
This file allows the DCS node to access your Kubernetes environment and deploy the DCS instance correctly.
![step five (flow)](./docImage/step3.png?raw=true)


### 7. Provide domain address and TLS credentials
In this tab, you must enter the **domain address** where the DCS will be accessible. You’ll also need to upload your **TLS certificate** and **private key**.

The final accessible URL is formed by combining this domain with the DCS instance name you set earlier. For example:
- Instance Name: `mydcs`
- Domain: `example.com`
- Resulting URL: `example.com/mydcs`
Make sure your TLS credentials match the provided domain.
![step six (flow)](./docImage/step4.png?raw=true)


### 8. Information tab
After the service is successfully deployed, you can switch to the **Information** tab.
Here, the final URL of your deployed catalogue instance will be shown—ready to be copied and used for access or integration.
Click **Done** and then **Deploy**. Activate the Inject node.
![step eight (flow)](./docImage/step7.png?raw=true)
You should see JSON output in the Debug panel, showing catalogue entries.

---

## ⚙️ Configuration

Before running:

1. **DCS URL**  
   Set the URL of your DCS instance.

2. **Query Parameters**  
   Provide any filters or search strings in the node editor or in `msg.payload`.

3. **Authorization Token (optional)**  
   The DCS endpoints require auth headers (Bearer token).

---

## 📁 Directory Contents
```
.
├── node-red-contrib-digital-contracting-service-0.0.1.tgz
├── DigitalContractingService.html
├── DigitalContractingService.js
├── package.json
```

- **node-red-contrib-digital-contracting-service-0.0.1.tgz**  
  Installable node package.

- **DigitalContractingService.html**  
  Node-RED UI form.

- **DigitalContractingService.js**  
  Backend logic to send API requests and return results.

- **package.json**  
  Metadata and dependencies.

---

## 📦 Dependencies

```json
"node": ">=14.0.0",
"node-red": ">=3.0.0"
```

---

## 🔗 Links & References

- [Digital Contracting Service - XFSC](https://github.com/eclipse-xfsc/facis/tree/main/DCS)


---

## 🔧 Troubleshooting

### WSL2 Port Forwarding for Rancher Desktop (Windows + WSL Development)

The port-forwarding steps and restart instructions are covered in **Dev Setup: Windows + Rancher Desktop + WSL** (see Step 4 and "Restarting After WSL Shutdown"). Use those steps if WSL cannot reach the Traefik LoadBalancer IP.

---

### Image Pull Errors with Rancher Desktop

If you're using **Rancher Desktop** and encounter `ImagePullBackOff` errors when deploying with private registries:

1. **Root Cause**: Rancher Desktop uses containerd which is isolated from Docker's credential store. Even if Docker can pull an image, containerd may not have access to the registry credentials.

2. **Solution**: Manually import the image into Rancher Desktop:
   - Build or pull the image locally: `docker pull <registry>/<image>:tag`
   - Open **Rancher Desktop GUI** → Images
   - Click **Import** and select the image
   - The image will now be available to Kubernetes

---

## �📝 License

This project is licensed under the Apache License 2.0. See the [LICENSE](../LICENSE) file for details.
