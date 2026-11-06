#!/usr/bin/env bash
# Render checks for the deployment hardening of the chart defaults and of the
# prod and acceptance profiles. Run: bash deployment/helm/tests/platform_hardening_test.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# prod and acceptance refuse to render without these operator acknowledgements.
ACK=(--set federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true)

render() { helm template dcs "$CHART_DIR" -n dcs "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# DCS-NFR-SEC-12: no fixed default credentials.
pin_a="$(render --show-only templates/hsm-pin-secret.yaml | grep 'PKCS11_PIN:')"
pin_b="$(render --show-only templates/hsm-pin-secret.yaml | grep 'PKCS11_PIN:')"
grep -q '"1234"' <<<"$pin_a" && fail "default PKCS#11 PIN is 1234"
[[ "$pin_a" != "$pin_b" ]] || fail "default PKCS#11 PIN is not random"
so_pin="$(render --show-only templates/hsm-provision-job.yaml | grep 'SO_PIN=')"
grep -q '"12345678"' <<<"$so_pin" && fail "default SO PIN is 12345678"
grep -q 'SO_PIN=""' <<<"$so_pin" && fail "default SO PIN is empty"
grep -rq 'YourStrongPassword' "$CHART_DIR/values.yaml" "$CHART_DIR/prom-values.yaml" && fail "fixed Grafana admin password in the chart values"
# An explicit PIN (values.bdd.yml) is used as given.
render -f "$CHART_DIR/values.bdd.yml" --show-only templates/hsm-pin-secret.yaml | grep -q 'PKCS11_PIN: "1234"' \
  || fail "explicit pkcs11.pin is not used"

# Signing needs a DSS to validate against (ADR-12): prod and acceptance get DSS_URL.
for profile in prod acceptance; do
  render -f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml" --show-only templates/deployment.yaml | grep -q 'name: DSS_URL' \
    || fail "values.$profile.yml gives the backend no DSS_URL"
done

# DCS-IR-CI-01 / DCS-NFR-SEC-01: TLS 1.3 minimum by default where Traefik terminates TLS.
traefik_prod="$(render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --api-versions traefik.io/v1alpha1/TLSOption)"
grep -q 'kind: TLSOption' <<<"$traefik_prod" || fail "no TLSOption for a TLS ingress on Traefik"
grep -q 'minVersion: "VersionTLS13"' <<<"$traefik_prod" || fail "TLSOption minimum is not TLS 1.3"
grep -q 'router.tls.options: "dcs-dcs-digital-contracting-service-tls13@kubernetescrd"' <<<"$traefik_prod" \
  || fail "the ingress does not name the TLSOption"
# No TLSOption where Traefik is absent (the CRD would not exist) or the ingress serves no TLS (BDD).
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" | grep -q 'kind: TLSOption' && fail "TLSOption without the Traefik CRD"
render -f "$CHART_DIR/values.bdd.yml" | grep -q 'router.tls.options' && fail "TLS option on a plain-HTTP ingress"
render "${ACK[@]}" --api-versions traefik.io/v1alpha1/TLSOption --set ingress.enabled=true \
  --set 'ingress.hosts[0].host=a.example' --set 'ingress.tls[0].hosts[0]=a.example' --set 'ingress.tls[0].secretName=t' \
  --set 'ingress.tlsOptions.cipherSuites[0]=TLS_AES_128_GCM_SHA256' >/dev/null || fail "TLSOption with cipherSuites does not render"

# DCS-IR-CI-09 / DCS-NFR-BR-06: access tokens live 5 minutes in prod and
# acceptance; the chart default keeps the Hydra default.
for profile in prod acceptance; do
  render -f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml" --show-only charts/hydra/templates/deployment.yaml \
    | grep -A1 'name: TTL_ACCESS_TOKEN$' | grep -q 'value: "5m"' || fail "values.$profile.yml: Hydra access-token TTL is not 5m"
done
render -f "$CHART_DIR/values.bdd.yml" --show-only charts/hydra/templates/deployment.yaml | grep -q TTL_ACCESS_TOKEN \
  && fail "access-token TTL set without hydra.config.accessTokenTTL"

# DCS-FR-CSA-14 / DCS-FR-CSA-19: archive retention policy of 10 years in prod
# and acceptance; none by default.
for profile in prod acceptance; do
  render -f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml" --show-only templates/deployment.yaml \
    | grep -A1 'name: DCS_ARCHIVE_RETENTION_YEARS$' | grep -q 'value: "10"' || fail "values.$profile.yml: no 10-year archive retention"
done
render "${ACK[@]}" --show-only templates/deployment.yaml | grep -q DCS_ARCHIVE_RETENTION_YEARS \
  && fail "archive retention set without archive.retentionYears"

# DCS-NFR-PER-03: liveness probe and, with more than one replica, a PodDisruptionBudget.
prod="$(render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml")"
grep -A4 'livenessProbe:' <<<"$prod" | grep -q 'path: /livez' || fail "no liveness probe on /livez"
grep -A4 'startupProbe:' <<<"$prod" | grep -q 'path: /livez' || fail "no startup probe on /livez"
grep -q 'kind: PodDisruptionBudget' <<<"$prod" || fail "no PodDisruptionBudget for 3 replicas"
render "${ACK[@]}" | grep -q 'kind: PodDisruptionBudget' && fail "PodDisruptionBudget for a single replica blocks drains"

# Acceptance runs 2 backend pods on the ReadWriteOnce SoftHSM2 token volume: they
# must share one node, and the budget must let a drain evict both.
render -f "$CHART_DIR/values.acceptance.yml" -f "$CHART_DIR/tests/acceptance.prerequisites.yml" | python3 -c '
import sys, yaml
docs = [d for d in yaml.safe_load_all(sys.stdin) if d]
dep = next(d for d in docs if d["kind"] == "Deployment" and d["metadata"]["name"] == "dcs-digital-contracting-service")
pod = dep["spec"]["template"]
aff = pod["spec"].get("affinity") or {}
terms = (aff.get("podAffinity") or {}).get("requiredDuringSchedulingIgnoredDuringExecution") or []
ok = any(t.get("topologyKey") == "kubernetes.io/hostname"
         and all(pod["metadata"]["labels"].get(k) == v for k, v in t["labelSelector"]["matchLabels"].items())
         for t in terms)
assert ok, "acceptance backend pods are not required on the node of the other backend pods"
assert "podAntiAffinity" not in aff, "acceptance backend pods carry an anti-affinity"
assert any(c["name"] == "hsm-token" and c["persistentVolumeClaim"] for c in pod["spec"]["volumes"] if "persistentVolumeClaim" in c), "no shared token volume"
job = next(d for d in docs if d["kind"] == "Job" and d["metadata"]["name"] == "dcs-digital-contracting-service-hsm-provision")
assert job["spec"]["template"]["spec"].get("affinity") == aff, "the provisioning Job can land on another node than the token volume"
pdb = next(d for d in docs if d["kind"] == "PodDisruptionBudget")
assert pdb["spec"]["maxUnavailable"] >= dep["spec"]["replicas"], "PodDisruptionBudget blocks the drain of the shared node"
' || fail "acceptance backend pods are not pinned to the node of their token volume"

# DCS-NFR-SF-03: scheduled database backup in prod and acceptance, off by default.
for profile in prod acceptance; do
  backup="$(render -f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml" --show-only templates/backup-cronjob.yaml 2>/dev/null || true)"
  grep -q 'kind: CronJob' <<<"$backup" || fail "values.$profile.yml has no backup CronJob"
  grep -q 'pg_dump --format=custom --dbname="$2"' <<<"$backup" && grep -q 'dump dcs "$DATABASE_URL"' <<<"$backup" \
    || fail "backup does not dump DATABASE_URL"
  grep -A3 'name: DATABASE_URL' <<<"$backup" | grep -q "name: \"dcs-$profile-db-credentials\"" \
    || fail "backup does not read the $profile database Secret"
done
render "${ACK[@]}" | grep -q 'db-backup' && fail "backup rendered while backup.enabled is false"
# The Hydra database (OAuth2 clients, issued machine credentials) is dumped
# with the DCS database: from the DSN Secret in prod, from the co-deployed
# PostgreSQL in the BDD profile.
for prof in prod bdd; do
  case "$prof" in
    prod) args=(-f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml") ;;
    bdd) args=(-f "$CHART_DIR/values.bdd.yml" --set backup.enabled=true) ;;
  esac
  backup="$(render "${args[@]}" --show-only templates/backup-cronjob.yaml)"
  grep -q 'dump hydra "$HYDRA_DSN"' <<<"$backup" || fail "$prof backup does not dump the Hydra database"
  grep -q "find /backup .*'hydra-\*.dump'" <<<"$backup" || fail "$prof backup keeps Hydra dumps forever"
done
grep -A4 'name: HYDRA_DSN' <<<"$(render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --show-only templates/backup-cronjob.yaml)" \
  | grep -q 'name: "dcs-prod-hydra-secrets"' || fail "prod backup does not read the Hydra DSN Secret"
memory="$(render -f "$CHART_DIR/values.bdd.yml" --set backup.enabled=true --set hydra.config.coDeployedPostgresql.enabled=false \
  --set hydra.config.dsn=memory --show-only templates/backup-cronjob.yaml)"
grep -q 'dump dcs' <<<"$memory" || fail "no backup rendered for the in-memory Hydra check"
grep -q HYDRA_DSN <<<"$memory" && fail "backup dumps an in-memory Hydra"
# The dev profile runs the backend on the host (replicaCount 0) and renders.
render -f "$CHART_DIR/values.dev.yml" >/dev/null || fail "values.dev.yml does not render"
# deploy.sh asserts the StorageClass of the backup and Loki claims too: the
# dumps hold the wrapped content keys, the logs can hold personal data.
manifest="$(mktemp)"
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" \
  --set backup.persistence.storageClassName=enc-backup --set logging.loki.persistence.storageClassName=enc-loki >"$manifest"
eval "$(sed -n '/^manifest_pvc_storageclass() {/,/^}/p' "$CHART_DIR/deploy.sh")"
NAMESPACE=dcs RELEASE=dcs
helm() { cat "$manifest"; }
[[ "$(manifest_pvc_storageclass dcs-digital-contracting-service-db-backup)" == enc-backup ]] || fail "deploy.sh does not read the backup claim class"
[[ "$(manifest_pvc_storageclass dcs-loki StatefulSet)" == enc-loki ]] || fail "deploy.sh does not read the Loki claim class"
unset -f helm
rm -f "$manifest"
grep -q '^storageclass_check "backup" *"${RELEASE}-digital-contracting-service-db-backup"$' "$CHART_DIR/deploy.sh" \
  && grep -q '^storageclass_check "loki" *"data-${RELEASE}-loki-0" "${RELEASE}-loki" "StatefulSet"$' "$CHART_DIR/deploy.sh" \
  || fail "deploy.sh does not check the backup and Loki claims"

# DCS-IR-CI-01 (UC-IF-28): identity.eidas.enforce turns on the EU trust pool
# and the qualified-certificate check of the DID certificates; off by default.
render -f "$CHART_DIR/values.bdd.yml" --set identity.eidas.enforce=true --show-only templates/deployment.yaml \
  | grep -A1 'name: DCS_FORCE_EIDAS_CERT$' | grep -q 'value: "true"' || fail "identity.eidas.enforce does not set DCS_FORCE_EIDAS_CERT"
render -f "$CHART_DIR/values.bdd.yml" --show-only templates/deployment.yaml | grep -q DCS_FORCE_EIDAS_CERT \
  && fail "DCS_FORCE_EIDAS_CERT set without identity.eidas.enforce"

# DCS-NFR-SEC-04: configIntegrity.pins reach the backend as DCS_CONFIG_SHA256_PINS.
a=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
b=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
pins="$(render -f "$CHART_DIR/values.bdd.yml" --set "configIntegrity.pins.x5c-trust-anchors-poa=$a" \
  --set "configIntegrity.pins.did-document=$b" --show-only templates/deployment.yaml | grep -A1 'name: DCS_CONFIG_SHA256_PINS$')"
grep -q "value: \"did-document=$b,x5c-trust-anchors-poa=$a\"" <<<"$pins" || fail "configIntegrity.pins not rendered: $pins"
render -f "$CHART_DIR/values.bdd.yml" --show-only templates/deployment.yaml | grep -q DCS_CONFIG_SHA256_PINS \
  && fail "empty pins rendered"

# DCS-NFR-SEC-11: alert rules where the Prometheus Operator runs.
rules="$(render "${ACK[@]}" --set backup.enabled=true --api-versions monitoring.coreos.com/v1/PrometheusRule --show-only templates/prometheusrule.yaml)"
for alert in DCSBackendDown DCSServerErrors DCSAuthFailureSpike DCSSlowResponses DCSBackendRestarting DCSBackupFailed \
  DCSBackupStale DCSBackupNeverSucceeded; do
  grep -q "alert: $alert" <<<"$rules" || fail "no $alert alert rule"
done
# With every pod gone up has no series, so absent() fires.
sel='namespace="dcs", service="dcs-digital-contracting-service-metrics"'
grep -qF "expr: up{$sel} == 0 or absent(up{$sel})" <<<"$rules" \
  || fail "DCSBackendDown does not fire on a missing backend metrics service"
grep -qF 'kube_cronjob_status_last_successful_time{namespace="dcs", cronjob="dcs-digital-contracting-service-db-backup"} > 93600' <<<"$rules" \
  || fail "no alert when the last successful backup is older than 26h"
render "${ACK[@]}" | grep -q 'kind: PrometheusRule' && fail "PrometheusRule without the CRD"

# DCS-NFR-SEC-11, DCS-NFR-PER-03: the rules evaluate series of a scrape, so
# the prod render with the Operator CRDs has the ServiceMonitor, both carry the
# same selector labels, and the rule selects the Service the monitor scrapes.
PROM=(--api-versions monitoring.coreos.com/v1/PrometheusRule --api-versions monitoring.coreos.com/v1/ServiceMonitor)
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" "${PROM[@]}" | python3 -c '
import sys, yaml
docs = [d for d in yaml.safe_load_all(sys.stdin) if d]
mon = [d for d in docs if d["kind"] == "ServiceMonitor"]
rule = [d for d in docs if d["kind"] == "PrometheusRule"]
assert mon and rule, "prod renders %d ServiceMonitor and %d PrometheusRule" % (len(mon), len(rule))
assert mon[0]["metadata"]["labels"] == {"release": "prometheus"}, mon[0]["metadata"]["labels"]
assert rule[0]["metadata"]["labels"]["release"] == "prometheus"
svc = next(d for d in docs if d["kind"] == "Service" and all(d["metadata"].get("labels", {}).get(k) == v for k, v in mon[0]["spec"]["selector"]["matchLabels"].items()))
assert ("service=\"%s\"" % svc["metadata"]["name"]) in rule[0]["spec"]["groups"][0]["rules"][0]["expr"], "the rule does not select the scraped Service"
' || fail "prod alert rules have no matching ServiceMonitor"
# The co-deployed kube-prometheus-stack selects release: <release>.
for t in templates/servicemonitor.yaml templates/prometheusrule.yaml; do
  render "${ACK[@]}" --set monitoring.enabled=true --show-only "$t" | grep -q '^    release: dcs$' \
    || fail "$t does not match the co-deployed kube-prometheus-stack"
done
render "${ACK[@]}" --set 'alerts.labels.prometheus=ops' "${PROM[@]}" --show-only templates/servicemonitor.yaml | grep -q '^    prometheus: ops$' \
  || fail "alerts.labels not on the ServiceMonitor"
# A long fullname: the rule selects the truncated Service name.
long="$(render "${ACK[@]}" --set fullnameOverride=dcs-a-very-long-name-that-is-longer-than-fifty-five-characters-x "${PROM[@]}")"
svc="$(grep -A6 'kind: Service$' <<<"$long" | grep -o 'name: .*-metrics' | head -1 | cut -d' ' -f2)"
[[ -n "$svc" ]] && grep -qF "service=\"$svc\"" <<<"$long" || fail "rule and metrics Service names differ: $svc"
render "${ACK[@]}" --set alerts.enabled=false "${PROM[@]}" | grep -q 'kind: ServiceMonitor' && fail "ServiceMonitor without alerts or serviceMonitor.enabled"

# DCS-NFR-SQ-03: prod and acceptance name every value the backend needs to
# start, instead of rendering a release that crash-loops.
err="$(render -f "$CHART_DIR/values.prod.yml" "${ACK[@]}" 2>&1 >/dev/null || true)"
for v in signing.issuerDID signing.tsaURL messaging.natsURL oid4vp.trust.existingConfigMap pkcs11.modulePath \
  identity.existingSecret signing.issuerX5ChainPath "Production prerequisites"; do
  grep -qF "$v" <<<"$err" || fail "prod without prerequisites does not name $v: $err"
done
err="$(render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --set configIntegrity.pins=null 2>&1 >/dev/null || true)"
grep -qF 'configIntegrity.pins.oid4vp-trust-data' <<<"$err" || fail "prod renders an unpinned trust ConfigMap"
for profile in prod acceptance; do
  render -f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml" >/dev/null \
    || fail "values.$profile.yml with its prerequisites does not render"
done
render "${ACK[@]}" >/dev/null || fail "the chart defaults do not render"
render -f "$CHART_DIR/values.bdd.yml" >/dev/null || fail "values.bdd.yml does not render"
grep -q 'lookup' <<<"$(sed -n '/define "digital-contracting-service.configPins"/,/end }}/p' "$CHART_DIR/templates/_helpers.tpl" | grep -v '^{{/\*\|^[A-Za-z(]')" \
  && fail "the trust ConfigMap pin is taken from the live object"
# U4: a real HSM takes did.json from the operator Secret.
prod="$(render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml")"
python3 -c '
import sys, yaml
dep = next(d for d in yaml.safe_load_all(sys.stdin) if d and d["kind"] == "Deployment" and d["metadata"]["name"] == "dcs-digital-contracting-service")
spec = dep["spec"]["template"]["spec"]
c = spec["containers"][0]
env = {e["name"]: e for e in c["env"]}
assert env["DCS_DID"]["value"] == "/app/identity/did.json", env.get("DCS_DID")
assert any(v["name"] == "identity" and v["secret"]["secretName"] == "dcs-prod-did" for v in spec["volumes"]), "no identity volume"
assert any(m["name"] == "identity" and m["mountPath"] == "/app/identity" for m in c["volumeMounts"]), "no identity mount"
# DCS-NFR-PER-03: the 3 replicas prefer different nodes.
term = spec["affinity"]["podAntiAffinity"]["preferredDuringSchedulingIgnoredDuringExecution"][0]["podAffinityTerm"]
assert term["topologyKey"] == "kubernetes.io/hostname"
assert all(dep["spec"]["template"]["metadata"]["labels"].get(k) == v for k, v in term["labelSelector"]["matchLabels"].items())
# DCS-NFR-SEC-12: the Hydra client secret comes from the operator Secret.
assert env["HYDRA_CLIENT_SECRET"]["valueFrom"]["secretKeyRef"] == {"name": "dcs-prod-hydra-secrets", "key": "dcs-client-secret"}, env["HYDRA_CLIENT_SECRET"]
' <<<"$prod" || fail "prod backend identity, anti-affinity or Hydra client secret"
# U5: the shipped ArgoCD Application renders with exactly what it lists.
python3 - "$CHART_DIR/../argocd/application.yaml" "$CHART_DIR" <<'EOF' || fail "the ArgoCD Application does not render"
import subprocess, sys, tempfile, yaml
app = yaml.safe_load(open(sys.argv[1]))
src = app["spec"]["source"]
assert src["repoURL"] == "https://github.com/eclipse-xfsc/facis-dcs.git", src["repoURL"]
cmd = ["helm", "template", "dcs", sys.argv[2], "-n", app["spec"]["destination"]["namespace"]]
for f in src["helm"]["valueFiles"]:
    cmd += ["-f", sys.argv[2] + "/" + f]
for p in src["helm"].get("parameters", []):
    cmd += ["--set", "%s=%s" % (p["name"], p["value"])]
with tempfile.NamedTemporaryFile("w", suffix=".yaml") as obj:
    yaml.safe_dump(src["helm"].get("valuesObject", {}), obj)
    obj.flush()
    subprocess.run(cmd + ["-f", obj.name], check=True, stdout=subprocess.DEVNULL)
EOF

# DCS-NFR-PER-03, DCS-NFR-SF-01, DCS-NFR-SEC-12: Hydra keeps its state in
# PostgreSQL, takes the DSN and client secrets from Secrets and registers the
# clients in every pod.
hydra="$(render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --show-only charts/hydra/templates/deployment.yaml)"
python3 -c '
import sys, yaml
dep = yaml.safe_load(sys.stdin)
assert dep["spec"]["replicas"] == 2, dep["spec"]["replicas"]
cs = {c["name"]: c for c in dep["spec"]["template"]["spec"]["containers"]}
env = {e["name"]: e for e in cs["hydra"]["env"]}
assert env["DSN"]["valueFrom"]["secretKeyRef"] == {"name": "dcs-prod-hydra-secrets", "key": "dsn"}, env["DSN"]
reg = {e["name"]: e for e in cs["register-clients"]["env"]}
assert reg["CLIENT_SECRET_0"]["valueFrom"]["secretKeyRef"] == {"name": "dcs-prod-hydra-secrets", "key": "dcs-client-secret"}
assert "exec" in cs["register-clients"]["readinessProbe"]
' <<<"$hydra" || fail "prod Hydra is not persistent, replicated and Secret-fed"
{ render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --set hydra.config.dsnSecretRef.name= 2>&1 >/dev/null || true; } \
  | grep -q 'hydra.config.dsn is memory' || fail "prod renders Hydra with an in-memory DSN"
{ render -f "$CHART_DIR/values.dev.yml" --set hydra.config.dsn=memory --set hydra.replicaCount=2 2>&1 >/dev/null || true; } \
  | grep -q 'hydra.replicaCount > 1 needs a persistent DSN' || fail "replicas of an in-memory Hydra"
{ render -f "$CHART_DIR/values.bdd.yml" --set postgresql.additionalDatabases=null 2>&1 >/dev/null || true; } \
  | grep -q 'coDeployedPostgresql needs' || fail "Hydra on the co-deployed PostgreSQL without its database"
# No client secret in a workload spec: only in the Secret dcs-hydra-clients.
render -f "$CHART_DIR/values.bdd.yml" | python3 -c '
import sys, yaml
docs = [d for d in yaml.safe_load_all(sys.stdin) if d]
for d in docs:
    if d["kind"] != "Secret" and "dcs-orce-notary-secret" in yaml.safe_dump(d):
        sys.exit("client secret in %s %s" % (d["kind"], d["metadata"]["name"]))
sec = next(d for d in docs if d["kind"] == "Secret" and d["metadata"]["name"] == "dcs-hydra-clients")
assert sec["stringData"]["dcs-orce-notary"] == "dcs-orce-notary-secret"
' || fail "a Hydra client secret is in a workload spec"

echo 'platform hardening render tests passed'
