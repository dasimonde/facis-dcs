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
Where the backend reads the certificate chain that publishes its own signing key
(ADR-34). An operator-supplied path wins; otherwise it is the chain the
provisioning job leaves on the shared token volume. Empty when neither exists,
which leaves the deployment unable to serve a status list and says so at startup
rather than serving an unverifiable one.
*/}}
{{- define "digital-contracting-service.issuerX5ChainPath" -}}
{{- if .Values.signing.issuerX5ChainPath -}}
{{- .Values.signing.issuerX5ChainPath -}}
{{- else if .Values.pkcs11.provisioning.enabled -}}
{{- printf "%s/c2pa-x5chain.pem" .Values.pkcs11.provisioning.tokenDir -}}
{{- end -}}
{{- end }}

{{/*
The origin this deployment serves its own status list under (ADR-34): the
did:web hostname with publicBaseURL's scheme, and NO api path. The list sits at
the origin root the way did.json does, because a verifier holding only a
credential has the URL that credential names and nothing else — it cannot be
asked to know this deployment's API prefix. This is the `iss` of the served
token and the identifier its certificate leaf must name.
*/}}
{{- define "digital-contracting-service.statusListIssuerURL" -}}
{{- if .Values.route.publicBaseURL -}}
{{- $u := urlParse .Values.route.publicBaseURL -}}
{{- printf "%s://%s" $u.scheme (include "digital-contracting-service.didHostname" .) -}}
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
DATABASE_URL override or derived from postgres settings. The co-deployed
server is reached with sslmode=verify-full against the internal CA when
global.internalTLS.enabled. Without an explicit password the URL has none and
PGPASSWORD comes from the postgresql Secret (databaseEnv).
*/}}
{{- define "digital-contracting-service.databaseURL" -}}
{{- if .Values.database.url -}}
{{- .Values.database.url -}}
{{- else if include "digital-contracting-service.postgresqlHost" . -}}
{{- $host := include "digital-contracting-service.postgresqlHost" . -}}
{{- $port := default 5432 .Values.database.port -}}
{{- $user := default (default "dcs" .Values.postgresql.auth.username) .Values.database.user -}}
{{- $password := default .Values.postgresql.auth.password .Values.database.password -}}
{{- $dbname := default (default "dcs" .Values.postgresql.auth.database) .Values.database.name -}}
{{- $sslmode := default "disable" .Values.database.sslmode -}}
{{- if and .Values.global.internalTLS.enabled .Values.postgresql.enabled (not .Values.serviceDiscovery.postgresqlHost) -}}
{{- $sslmode = printf "verify-full sslrootcert=%s/ca.crt" (include "digital-contracting-service.internalTLSDir" .) -}}
{{- end -}}
{{- printf "host=%s port=%v user=%s" $host $port $user -}}
{{- with $password }}{{ printf " password=%s" . }}{{ end -}}
{{- printf " dbname=%s sslmode=%s" $dbname $sslmode -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Env entries that give a container its database connection: DATABASE_URL and,
when the co-deployed server has a generated password, PGPASSWORD.
*/}}
{{- define "digital-contracting-service.databaseEnv" -}}
- name: DATABASE_URL
  {{- if .Values.database.secretRef.name }}
  valueFrom:
    secretKeyRef:
      name: {{ .Values.database.secretRef.name | quote }}
      key: {{ .Values.database.secretRef.key | quote }}
  {{- else }}
  value: {{ include "digital-contracting-service.databaseURL" . | quote }}
  {{- end }}
{{- if and (not .Values.database.secretRef.name) (not .Values.database.url) .Values.postgresql.enabled (not (default .Values.postgresql.auth.password .Values.database.password)) }}
- name: PGPASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ printf "%s-postgresql" .Release.Name | quote }}
      key: postgres-password
{{- end }}
{{- end }}

{{/*
In-cluster TLS (DCS-NFR-SEC-14, DCS-NFR-SEC-01): Secret with the server
certificate and the CA, and the directory a client mounts the CA in.
*/}}
{{- define "digital-contracting-service.internalTLSSecretName" -}}
{{- printf "%s-internal-tls" .Release.Name -}}
{{- end }}

{{/*
The internal CA as JSON {"Cert","Key"}: from the <release>-internal-ca Secret
when it exists, else generated (ECDSA P-256, 10 years). The result is kept in
.Values for the render, so every template of this chart gets the same CA.
*/}}
{{- define "digital-contracting-service.internalCA" -}}
{{- $cfg := .Values.global.internalTLS -}}
{{- if not (hasKey $cfg "renderedCA") -}}
{{- $existing := lookup "v1" "Secret" .Release.Namespace (printf "%s-internal-ca" .Release.Name) -}}
{{- $ca := dict -}}
{{- if $existing.data -}}
{{- $ca = dict "Cert" (index $existing.data "ca.crt" | b64dec) "Key" (index $existing.data "ca.key" | b64dec) -}}
{{- else -}}
{{- $gen := genCAWithKey (printf "%s internal CA" .Release.Name) 3650 (genPrivateKey "ecdsa") -}}
{{- $ca = dict "Cert" $gen.Cert "Key" $gen.Key -}}
{{- end -}}
{{- $_ := set $cfg "renderedCA" $ca -}}
{{- end -}}
{{- $cfg.renderedCA | toJson -}}
{{- end }}

{{- define "digital-contracting-service.internalTLSDir" -}}
/etc/dcs-internal-tls
{{- end }}

{{/* Volume with only the internal CA, for clients. */}}
{{- define "digital-contracting-service.internalCAVolume" -}}
- name: internal-ca
  secret:
    secretName: {{ include "digital-contracting-service.internalTLSSecretName" . | quote }}
    items:
      - key: ca.crt
        path: ca.crt
{{- end }}

{{- define "digital-contracting-service.internalCAVolumeMount" -}}
- name: internal-ca
  mountPath: {{ include "digital-contracting-service.internalTLSDir" . | quote }}
  readOnly: true
{{- end }}

{{/*
TLS proxy container for a component that serves plain HTTP only (IPFS RPC,
pdf-core, DSS, ORCE). It terminates TLS 1.3 on global.internalTLS.proxy.port
and forwards to the component on 127.0.0.1:<upstreamPort>. It runs as the nginx
user of the image (UID 101) on a read-only root filesystem and writes only to
/tmp. The pod must have the volumes of internalTLSVolume. Subcharts include
this template only when global.internalTLS.enabled, so they render on their
own as well. Optional "extra": more listeners, each a dict with "name" (port
name), "port" and "upstreamPort".
Call: include "digital-contracting-service.internalTLSProxy" (dict "root" . "upstreamPort" 5001)
*/}}
{{- define "digital-contracting-service.internalTLSProxy" -}}
{{- $p := .root.Values.global.internalTLS.proxy -}}
{{- $servers := prepend (.extra | default list) (dict "name" "https" "port" $p.port "upstreamPort" .upstreamPort) -}}
- name: tls-proxy
  image: "{{ $p.image.repository }}:{{ $p.image.tag }}{{ with $p.image.digest }}@{{ . }}{{ end }}"
  imagePullPolicy: {{ $p.image.pullPolicy }}
  securityContext:
    runAsNonRoot: true
    runAsUser: 101
    runAsGroup: 101
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop: ["ALL"]
    seccompProfile:
      type: RuntimeDefault
  command: ["/bin/sh", "-c"]
  args:
    - |
      cat > /tmp/nginx.conf <<'EOF'
      pid /tmp/nginx.pid;
      events {}
      http {
        client_body_temp_path /tmp/client_body;
        proxy_temp_path /tmp/proxy;
        fastcgi_temp_path /tmp/fastcgi;
        uwsgi_temp_path /tmp/uwsgi;
        scgi_temp_path /tmp/scgi;
        access_log off;
        {{- range $servers }}
        server {
          listen {{ .port }} ssl;
          ssl_certificate /etc/dcs-internal-tls/tls.crt;
          ssl_certificate_key /etc/dcs-internal-tls/tls.key;
          ssl_protocols TLSv1.3;
          client_max_body_size 0;
          proxy_request_buffering off;
          proxy_buffering off;
          proxy_read_timeout 600s;
          proxy_send_timeout 600s;
          location / {
            proxy_http_version 1.1;
            proxy_set_header Host $http_host;
            # The backend keys the lockout on the client address (FR-UC-01-4): pass the peer address on.
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_pass http://127.0.0.1:{{ .upstreamPort }};
          }
        }
        {{- end }}
      }
      EOF
      exec nginx -c /tmp/nginx.conf -g 'daemon off;'
  ports:
    {{- range $servers }}
    - name: {{ .name }}
      containerPort: {{ .port }}
      protocol: TCP
    {{- end }}
  readinessProbe:
    tcpSocket:
      port: https
    periodSeconds: 5
  {{- with $p.resources }}
  resources:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  volumeMounts:
    - name: internal-tls
      mountPath: /etc/dcs-internal-tls
      readOnly: true
    - name: tls-proxy-tmp
      mountPath: /tmp
{{- end }}

{{/* Volumes of the TLS proxy: the server certificate and key, and its /tmp. */}}
{{- define "digital-contracting-service.internalTLSVolume" -}}
- name: internal-tls
  secret:
    secretName: {{ printf "%s-internal-tls" .Release.Name | quote }}
- name: tls-proxy-tmp
  emptyDir: {}
{{- end }}

{{/* Service port of the TLS proxy. */}}
{{- define "digital-contracting-service.internalTLSServicePort" -}}
- name: https
  port: {{ .Values.global.internalTLS.proxy.port }}
  targetPort: https
  protocol: TCP
{{- end }}

{{/*
URL the backend calls a component on: https on the TLS proxy port when
global.internalTLS.enabled, else plain http on the component port.
Call: include "digital-contracting-service.componentURL" (dict "root" . "host" "x" "port" 8080)
*/}}
{{- define "digital-contracting-service.componentURL" -}}
{{- if .root.Values.global.internalTLS.enabled -}}
{{- printf "https://%s:%v" .host .root.Values.global.internalTLS.proxy.port -}}
{{- else -}}
{{- printf "http://%s:%v" .host .port -}}
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
{{- $scheme := "nats" -}}
{{- if and .Values.global.internalTLS.enabled .Values.nats.enabled (not .Values.serviceDiscovery.natsHost) -}}
{{- $scheme = "tls" -}}
{{- end -}}
{{- printf "%s://%s:%v" $scheme $host $port -}}
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
In-cluster Hydra public API (OIDC discovery, token) for DCS backend HTTP calls:
the TLS proxy of the public API when global.internalTLS.enabled.
*/}}
{{- define "digital-contracting-service.hydraInternalIssuerURL" -}}
{{- if .Values.hydra.enabled -}}
{{- if .Values.hydra.config.internalIssuerURL -}}
{{- .Values.hydra.config.internalIssuerURL -}}
{{- else if .Values.global.internalTLS.enabled -}}
{{- printf "https://%s-hydra:%d" .Release.Name (.Values.hydra.service.publicTLSPort | int) -}}
{{- else -}}
{{- printf "http://%s-hydra:%d" .Release.Name (.Values.hydra.service.publicPort | int) -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Hydra admin API base URL (login/consent accept): the TLS proxy of the Hydra
pod when global.internalTLS.enabled.
*/}}
{{- define "digital-contracting-service.hydraAdminURL" -}}
{{- if .Values.hydra.enabled -}}
{{- include "digital-contracting-service.componentURL" (dict "root" . "host" (printf "%s-hydra" .Release.Name) "port" (.Values.hydra.service.adminPort | int)) -}}
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
IPFS Kubo RPC URL: explicit URL before the enabled sub-chart endpoint.
*/}}
{{- define "digital-contracting-service.ipfsMfsBaseURL" -}}
{{- if .Values.ipfs.mfsBaseURL -}}
{{- .Values.ipfs.mfsBaseURL -}}
{{- else if .Values.ipfs.enabled -}}
{{- $port := default 5001 .Values.ipfs.service.apiPort -}}
{{- include "digital-contracting-service.componentURL" (dict "root" . "host" (printf "%s-ipfs" .Release.Name) "port" $port) -}}
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
Normalize the vendored fc-service route path (leading slash, no trailing slash).
*/}}
{{- define "digital-contracting-service.fcserviceRoutePath" -}}
{{- if .Values.fcservice.route.path -}}
{{- printf "/%s" (trimAll "/" (.Values.fcservice.route.path | toString)) -}}
{{- end -}}
{{- end }}

{{/*
PDF-Core internal service URL — auto-wired when pdfCore.enabled=true. It is
also the base of the ontology and context IRIs, so it stays the plain service
URL; the backend calls pdfCoreCallURL.
*/}}
{{- define "digital-contracting-service.pdfCoreURL" -}}
{{- if .Values.pdfCore.url -}}
{{- .Values.pdfCore.url -}}
{{- else if .Values.pdfCore.enabled -}}
{{- printf "http://%s-pdf-core:%v" (include "digital-contracting-service.fullname" .) .Values.pdfCore.service.port -}}
{{- end -}}
{{- end }}

{{/* URL the backend calls pdf-core on. */}}
{{- define "digital-contracting-service.pdfCoreCallURL" -}}
{{- if .Values.pdfCore.url -}}
{{- .Values.pdfCore.url -}}
{{- else if .Values.pdfCore.enabled -}}
{{- include "digital-contracting-service.componentURL" (dict "root" . "host" (printf "%s-pdf-core" (include "digital-contracting-service.fullname" .)) "port" .Values.pdfCore.service.port) -}}
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

{{/*
Ingress controller that reaches the backend, the Hydra public API and the
Federated Catalogue on their TLS proxies and verifies them against the internal
CA (DCS-NFR-SEC-01, DCS-NFR-SEC-14): "nginx" for ingress.className nginx,
"traefik" for ingress.className traefik or empty when the ServersTransport CRD
exists. Empty: the ingress uses the plain ports.
*/}}
{{- define "digital-contracting-service.ingressBackendTLS" -}}
{{- if .Values.global.internalTLS.enabled -}}
{{- $class := .Values.ingress.className -}}
{{- if eq $class "nginx" -}}
nginx
{{- else if and (or (eq $class "traefik") (not $class)) (include "digital-contracting-service.traefikServersTransportAnnotation" .) -}}
traefik
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Service annotation that names the Traefik ServersTransport of the release
(templates/ingress-backend-tls.yaml): Traefik then verifies the TLS proxy of
the Service against the internal CA. Set when global.internalTLS.enabled and
the ServersTransport CRD exists. Subcharts include it with their own context.
*/}}
{{- define "digital-contracting-service.traefikServersTransportAnnotation" -}}
{{- if and (dig "internalTLS" "enabled" false (.Values.global | default dict)) (.Capabilities.APIVersions.Has "traefik.io/v1alpha1/ServersTransport") -}}
traefik.ingress.kubernetes.io/service.serverstransport: {{ printf "%s-%s-internal-tls@kubernetescrd" .Release.Namespace .Release.Name | quote }}
{{- end -}}
{{- end }}

{{/*
"true" when the chart enforces the minimum TLS version itself: the ingress
terminates TLS and Traefik serves it (DCS-IR-CI-01, DCS-NFR-SEC-01).
*/}}
{{- define "digital-contracting-service.tlsOptionEnforced" -}}
{{- if and .Values.ingress.enabled .Values.ingress.tlsOptions.enabled .Values.ingress.tls (or .Values.traefik.enabled (.Capabilities.APIVersions.Has "traefik.io/v1alpha1/TLSOption")) -}}
true
{{- end -}}
{{- end }}

{{/*
DCS_CONFIG_SHA256_PINS (DCS-NFR-SEC-04): configIntegrity.pins. The pin of the
operator trust ConfigMap is an explicit value, required in production
(templates/prerequisites.yaml): a pin taken from the live ConfigMap at render
time would take over an edit of it, and helm template or ArgoCD would render
none.
*/}}
{{- define "digital-contracting-service.configPins" -}}
{{- $parts := list -}}
{{- range $name, $hash := (.Values.configIntegrity.pins | default dict) -}}
{{- $parts = append $parts (printf "%s=%s" $name $hash) -}}
{{- end -}}
{{- join "," $parts -}}
{{- end }}

{{/* Name of the /metrics Service; the alert rules select it by this name. */}}
{{- define "digital-contracting-service.metricsServiceName" -}}
{{- printf "%s-metrics" (include "digital-contracting-service.fullname" . | trunc 55 | trimSuffix "-") -}}
{{- end }}

{{/*
Labels of the ServiceMonitor and the PrometheusRule (DCS-NFR-SEC-11): the
selectors of the Prometheus that loads them. alerts.labels when set; else
release: <this release> for the co-deployed kube-prometheus-stack
(monitoring.enabled), whose selectors match its own release; else
release: prometheus, the release name of a separately installed stack.
*/}}
{{- define "digital-contracting-service.monitoringLabels" -}}
{{- if .Values.alerts.labels -}}
{{- toYaml .Values.alerts.labels -}}
{{- else if .Values.monitoring.enabled -}}
release: {{ .Release.Name }}
{{- else -}}
release: prometheus
{{- end -}}
{{- end }}

{{/* "true" when the Prometheus Operator CRDs exist or monitoring.enabled. */}}
{{- define "digital-contracting-service.prometheusOperator" -}}
{{- if or .Values.monitoring.enabled (.Capabilities.APIVersions.Has "monitoring.coreos.com/v1/PrometheusRule") -}}
true
{{- end -}}
{{- end }}
