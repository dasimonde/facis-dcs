{{/*
Expand the name of the chart.
*/}}
{{- define "digital-contracting-service.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "digital-contracting-service.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "digital-contracting-service.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "digital-contracting-service.labels" -}}
helm.sh/chart: {{ include "digital-contracting-service.chart" . }}
{{ include "digital-contracting-service.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "digital-contracting-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "digital-contracting-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "digital-contracting-service.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "digital-contracting-service.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Normalize a route base path to always start with "/" and never end with "/".
*/}}
{{- define "digital-contracting-service.baseRoutePath" -}}
{{- $base := default "digital-contracting-service" .Values.route.basePath -}}
{{- printf "/%s" (trimAll "/" ($base | toString)) -}}
{{- end }}

{{/*
The /.well-known documents this instance publishes at the HOST ROOT, as a YAML array.

Root-relative on purpose and not derived from route.basePath: a peer resolves a
did:web by appending /.well-known/did.json to the bare hostname, so these three
have no base path to inherit. The backend mounts each of them (backend/design/did.go)
and the ingress must route them to the backend ahead of any broader /.well-known
claim (Hydra's OIDC discovery, notably).
*/}}
{{- define "digital-contracting-service.wellKnownPaths" -}}
- /.well-known/did.json
- /.well-known/dcs-agreement-credential.json
- /.well-known/dcs-federation-rules.md
{{- end }}

{{/*
Where pdf-core reads the C2PA x5chain from: the projected Secret when the
provisioning hook publishes one, otherwise the file the hook leaves on the
shared token volume.
*/}}
{{- define "digital-contracting-service.pdfCoreX5ChainPath" -}}
{{- if .Values.pkcs11.provisioning.publishSecrets -}}
{{- printf "/x5chain/%s" (include "digital-contracting-service.pdfCoreX5ChainSecretKey" .) -}}
{{- else -}}
{{- printf "%s/c2pa-x5chain.pem" .Values.pkcs11.provisioning.tokenDir -}}
{{- end -}}
{{- end }}

{{/*
Resolve PostgreSQL host (explicit override or in-chart default).
*/}}
{{- define "digital-contracting-service.postgresqlHost" -}}
{{- if .Values.serviceDiscovery.postgresqlHost -}}
{{- .Values.serviceDiscovery.postgresqlHost -}}
{{- else if .Values.postgresql.enabled -}}
{{- printf "%s-postgresql" .Release.Name -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Resolve NATS host (explicit override or in-chart default).
*/}}
{{- define "digital-contracting-service.natsHost" -}}
{{- if .Values.serviceDiscovery.natsHost -}}
{{- .Values.serviceDiscovery.natsHost -}}
{{- else if .Values.nats.enabled -}}
{{- printf "%s-nats" .Release.Name -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
DATABASE_URL override or derived from postgres settings.
*/}}
{{- define "digital-contracting-service.databaseURL" -}}
{{- if .Values.database.url -}}
{{- .Values.database.url -}}
{{- else if include "digital-contracting-service.postgresqlHost" . -}}
{{- $host := include "digital-contracting-service.postgresqlHost" . -}}
{{- $port := default 5432 .Values.database.port -}}
{{- $user := default (default "dcs" .Values.postgresql.auth.username) .Values.database.user -}}
{{- $password := default (default "dcs" .Values.postgresql.auth.password) .Values.database.password -}}
{{- $dbname := default (default "dcs" .Values.postgresql.auth.database) .Values.database.name -}}
{{- $sslmode := default "disable" .Values.database.sslmode -}}
{{- printf "host=%s port=%v user=%s password=%s dbname=%s sslmode=%s" $host $port $user $password $dbname $sslmode -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
NATS_URL override or derived from nats settings.
*/}}
{{- define "digital-contracting-service.natsURL" -}}
{{- if .Values.messaging.natsURL -}}
{{- .Values.messaging.natsURL -}}
{{- else if include "digital-contracting-service.natsHost" . -}}
{{- $host := include "digital-contracting-service.natsHost" . -}}
{{- $port := default 4222 .Values.messaging.natsPort -}}
{{- printf "nats://%s:%v" $host $port -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Hydra OAuth2/OIDC issuer (URLs issuer / discovery). Requires hydra.enabled.
*/}}
{{- define "digital-contracting-service.hydraIssuerURL" -}}
{{- if .Values.hydra.enabled -}}
{{- if .Values.hydra.config.selfIssuerURL -}}
{{- .Values.hydra.config.selfIssuerURL -}}
{{- else -}}
{{- printf "http://%s-hydra:%d" .Release.Name (.Values.hydra.service.publicPort | int) -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
In-cluster Hydra public API (OIDC discovery, token) for DCS backend HTTP calls.
*/}}
{{- define "digital-contracting-service.hydraInternalIssuerURL" -}}
{{- if .Values.hydra.enabled -}}
{{- if .Values.hydra.config.internalIssuerURL -}}
{{- .Values.hydra.config.internalIssuerURL -}}
{{- else -}}
{{- printf "http://%s-hydra:%d" .Release.Name (.Values.hydra.service.publicPort | int) -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Hydra admin API base URL (login/consent accept).
*/}}
{{- define "digital-contracting-service.hydraAdminURL" -}}
{{- if .Values.hydra.enabled -}}
{{- printf "http://%s-hydra:%d" .Release.Name (.Values.hydra.service.adminPort | int) -}}
{{- end -}}
{{- end }}

{{/*
Keycloak realm URL for Federated Catalogue integration only.
*/}}
{{- define "digital-contracting-service.fcKeycloakRealmURL" -}}
{{- if .Values.fcKeycloak.realmURL -}}
{{- .Values.fcKeycloak.realmURL -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
API path override or derived default.
*/}}
{{- define "digital-contracting-service.apiPath" -}}
{{- if .Values.paths.api -}}
{{- .Values.paths.api -}}
{{- else -}}
{{- printf "%s/api" (include "digital-contracting-service.baseRoutePath" .) -}}
{{- end -}}
{{- end }}

{{/*
UI path override or derived default.
*/}}
{{- define "digital-contracting-service.uiPath" -}}
{{- if .Values.paths.ui -}}
{{- .Values.paths.ui -}}
{{- else -}}
{{- printf "%s/ui" (include "digital-contracting-service.baseRoutePath" .) -}}
{{- end -}}
{{- end }}

{{/*
IPFS Document Manager tenant base URL (auto-wired when ipfsDocumentManager sub-chart is enabled).
*/}}
{{- define "digital-contracting-service.ipfsTenantBaseURL" -}}
{{- if .Values.ipfsClient.tenantBaseURL -}}
{{- .Values.ipfsClient.tenantBaseURL -}}
{{- else if .Values.ipfsDocumentManager.enabled -}}
{{- $host := printf "%s-ipfs-document-manager" .Release.Name -}}
{{- $port := default 8080 .Values.ipfsDocumentManager.service.port -}}
{{- $tenant := default "tenant_space" .Values.ipfsClient.tenantName -}}
{{- printf "http://%s:%v/v1/tenants/%s" $host $port $tenant -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
IPFS MFS base URL - Kubo RPC API (auto-wired when ipfs sub-chart is enabled).
*/}}
{{- define "digital-contracting-service.ipfsMfsBaseURL" -}}
{{- if .Values.ipfsClient.mfsBaseURL -}}
{{- .Values.ipfsClient.mfsBaseURL -}}
{{- else if .Values.ipfs.enabled -}}
{{- $host := printf "%s-ipfs" .Release.Name -}}
{{- $port := default 5001 .Values.ipfs.service.apiPort -}}
{{- printf "http://%s:%v" $host $port -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
ISSUER_DID: explicit value or secret ref.
*/}}
{{- define "digital-contracting-service.issuerDID" -}}
{{- .Values.signing.issuerDID -}}
{{- end }}

{{/*
Name of the Kubernetes Secret holding the SoftHSM2 token PIN (PKCS11_PIN).
Auto-created by the chart when pkcs11.pinSecretRef.name is unset.
*/}}
{{- define "digital-contracting-service.hsmPinSecretName" -}}
{{- default (printf "%s-hsm-pin" (include "digital-contracting-service.fullname" .)) .Values.pkcs11.pinSecretRef.name -}}
{{- end }}

{{/*
Name of the Secret the provisioning job writes the C2PA x5chain PEM into and
that pdf-core mounts. SoftHSM2 is a software token for dev/staging/CI only.
*/}}
{{- define "digital-contracting-service.hsmX5ChainSecretName" -}}
{{- printf "%s-hsm-c2pa-x5chain" (include "digital-contracting-service.fullname" .) -}}
{{- end }}

{{/*
Normalize the vendored fc-service route path (leading slash, no trailing slash).
*/}}
{{- define "digital-contracting-service.fcserviceRoutePath" -}}
{{- if .Values.fcservice.route.path -}}
{{- printf "/%s" (trimAll "/" (.Values.fcservice.route.path | toString)) -}}
{{- end -}}
{{- end }}

{{/*
PDF-Core internal service URL — auto-wired when pdfCore.enabled=true.
*/}}
{{- define "digital-contracting-service.pdfCoreURL" -}}
{{- if .Values.pdfCore.url -}}
{{- .Values.pdfCore.url -}}
{{- else if .Values.pdfCore.enabled -}}
{{- printf "http://%s-pdf-core:%v" (include "digital-contracting-service.fullname" .) .Values.pdfCore.service.port -}}
{{- end -}}
{{- end }}

{{/*
Name of the Secret that holds the pdf-core C2PA signing material.
*/}}
{{- define "digital-contracting-service.pdfCoreSigningSecretName" -}}
{{- default (printf "%s-pdf-core-signing" (include "digital-contracting-service.fullname" .)) .Values.pdfCore.signing.existingSecret -}}
{{- end }}

{{/*
Name of the Secret that holds the x5chain PEM for pdf-core C2PA signing.
When pkcs11.provisioning is enabled the chain is derived from the SoftHSM2
dcs-c2pa token key by the provisioning job; otherwise the inline dev secret.
*/}}
{{- define "digital-contracting-service.pdfCoreX5ChainSecretName" -}}
{{- if .Values.pdfCore.signing.existingSecret -}}
{{- .Values.pdfCore.signing.existingSecret -}}
{{- else if .Values.pkcs11.provisioning.enabled -}}
{{- include "digital-contracting-service.hsmX5ChainSecretName" . -}}
{{- else -}}
{{- include "digital-contracting-service.pdfCoreSigningSecretName" . -}}
{{- end -}}
{{- end }}

{{/*
Key within the x5chain Secret for pdf-core C2PA signing.
*/}}
{{- define "digital-contracting-service.pdfCoreX5ChainSecretKey" -}}
{{- if and (not .Values.pdfCore.signing.existingSecret) .Values.pkcs11.provisioning.enabled -}}
{{- "x5chain-pem" -}}
{{- else if .Values.pdfCore.signing.existingSecretX5ChainKey -}}
{{- .Values.pdfCore.signing.existingSecretX5ChainKey -}}
{{- else -}}
{{- "x5chain-pem" -}}
{{- end -}}
{{- end }}

{{/*
The host:port a did:web identifier encodes for THIS instance's own did.json
(DCS-OR-C2PA-008). route.didHostname is an explicit override (needed when the
did:web hostname differs from route.publicBaseURL's host — e.g. the BDD
two-instance suite's cluster-routable dcs-a.localhost/dcs-b.localhost
hostnames, which resolve via a CoreDNS rewrite rather than being the literal
ingress host callers use for every path); falling back to publicBaseURL's
host, then the in-cluster default, keeps every existing single-host
deployment unchanged.
*/}}
{{- define "digital-contracting-service.didHostname" -}}
{{- if .Values.route.didHostname -}}
{{- .Values.route.didHostname -}}
{{- else if .Values.route.publicBaseURL -}}
{{- (urlParse .Values.route.publicBaseURL).host -}}
{{- else -}}
{{- printf "localhost:%v" .Values.service.port -}}
{{- end -}}
{{- end }}

{{/*
Name of the Secret the hsm-provision Job publishes did.json into and that the
deployment mounts as the 'identity' volume (DCS_DID) when identity.enabled is
true. Derived from <fullname> so two releases sharing one namespace (e.g. the
BDD two-instance suite's 'dcs' / 'dcs2' releases) never collide on a shared
literal name.
*/}}
{{/*
Public base URL for the absolute IRIs a produced document carries (schema
anchors, C2PA remote manifests): the did:web hostname — resolvable both
in-cluster and externally — combined with publicBaseURL's scheme and path.
*/}}
{{- define "digital-contracting-service.publicAnchorBaseURL" -}}
{{- if .Values.route.publicBaseURL -}}
{{- $u := urlParse .Values.route.publicBaseURL -}}
{{- printf "%s://%s%s" $u.scheme (include "digital-contracting-service.didHostname" .) $u.path -}}
{{- end -}}
{{- end }}

{{/*
Path the backend reads its OID4VP issuer trust document from. An
operator-supplied ConfigMap wins over the image's baked-in dev fixture, because
a deployment that must trust a real credential issuer cannot express that in the
image. The file is at <mountPath>/<key>, matching the volumeMount.
*/}}
{{/*
Path to the OID4VP trust document.

The chart default points at the dev fixture baked into the image, which is keyed
to repository-committed material. The backend refuses to load it without
DCS_ALLOW_DEV_TRUST, so a release that supplies neither an operator trust
document nor that flag would install and then crash-loop on a startup error. It
is the correct refusal reached at the least useful moment, so it is caught here
instead, where the message can say what to set.
*/}}
{{- define "digital-contracting-service.oid4vpTrustDataPath" -}}
{{- if .Values.oid4vp.trust.existingConfigMap -}}
{{- printf "%s/%s" (trimSuffix "/" .Values.oid4vp.trust.existingConfigMapMountPath) .Values.oid4vp.trust.existingConfigMapKey -}}
{{- else -}}
{{- $devFixture := contains "trust.dev.json" .Values.oid4vp.trust.dataPath -}}
{{- $devAllowed := false -}}
{{- range .Values.extraEnv -}}
{{- if and (eq .name "DCS_ALLOW_DEV_TRUST") (eq (toString .value) "true") -}}
{{- $devAllowed = true -}}
{{- end -}}
{{- end -}}
{{/* A release supplying env from a ConfigMap or Secret may set the flag there,
     which is not readable at render time. Refusing then would report a
     misconfiguration the operator has already corrected, so the check defers to
     the backend's own content-based guard, which is the real enforcement. */}}
{{- if .Values.extraEnvFrom -}}
{{- $devAllowed = true -}}
{{- end -}}
{{- if and $devFixture (not $devAllowed) -}}
{{- fail "oid4vp.trust: this release would run on the dev trust fixture baked into the image, whose issuer keys are committed to the repository. Set oid4vp.trust.existingConfigMap to a ConfigMap holding this deployment's trust document (see deployment/README.md, \"Credential issuers\"), or, for a dev or CI stack only, add DCS_ALLOW_DEV_TRUST=true to extraEnv." -}}
{{- end -}}
{{- .Values.oid4vp.trust.dataPath -}}
{{- end -}}
{{- end }}

{{- define "digital-contracting-service.identitySecretName" -}}
{{- default (printf "%s-identity" (include "digital-contracting-service.fullname" .)) .Values.identity.secretName -}}
{{- end }}

{{/*
STATUSLIST_SERVICE_URL — auto-derived from the statuslistService sub-chart when
enabled=true, otherwise falls back to the explicit statuslistService.url override.
*/}}
{{- define "digital-contracting-service.statuslistServiceURL" -}}
{{- if .Values.statuslistService.url -}}
{{- .Values.statuslistService.url -}}
{{- else if .Values.statuslistService.enabled -}}
{{- printf "http://%s-statuslist-service:%v" .Release.Name .Values.statuslistService.service.port -}}
{{- end -}}
{{- end }}

{{/*
PDF_CORE_CONTEXT_IRI — the @context IRI embedded in every JSON-LD envelope.
Set pdfCore.contextIRI to override (e.g. a registered w3id IRI once available).
Default: auto-derived as <pdfCoreURL>/ontology/dcs-pdf-core.
*/}}
{{- define "digital-contracting-service.pdfCoreContextIRI" -}}
{{- if .Values.pdfCore.contextIRI -}}
{{- .Values.pdfCore.contextIRI -}}
{{- else -}}
{{- printf "%s/ontology/dcs-pdf-core" (include "digital-contracting-service.pdfCoreURL" .) -}}
{{- end -}}
{{- end }}
