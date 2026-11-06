#!/usr/bin/env bash
# Render checks for in-cluster TLS (DCS-NFR-SEC-14, DCS-NFR-SEC-01) and the
# generated PostgreSQL password (DCS-NFR-SEC-12).
# Needs helm, openssl and python3 with PyYAML.
# Run: bash deployment/helm/tests/in_cluster_security_test.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ACK=(--set federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true)
ALL=(--set postgresql.enabled=true --set nats.enabled=true --set ipfs.enabled=true --set dss.enabled=true
  --set orce.enabled=true --set global.archiveAuditToken=t --set backup.enabled=true)
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

render() { helm template dcs "$CHART_DIR" -n dcs "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Prints one field of one rendered object: obj <file> <kind> <name> <python expression on x>
obj() {
  python3 - "$@" <<'EOF'
import sys, yaml
f, kind, name, expr = sys.argv[1:5]
for x in yaml.safe_load_all(open(f)):
    if x and x['kind'] == kind and x['metadata']['name'] == name:
        print(eval(expr))
        break
else:
    sys.exit('no %s %s' % (kind, name))
EOF
}
env_of() { # env_of <file> <kind> <name> <container> <env name>
  obj "$1" "$2" "$3" "next(((e.get('value') or str(e.get('valueFrom'))) for c in (x['spec']['template'] if 'template' in x['spec'] else x['spec']['jobTemplate']['spec']['template'])['spec']['containers'] if c['name'] == '$4' for e in c.get('env', []) if e['name'] == '$5'), None)"
}
B=dcs-digital-contracting-service
C=digital-contracting-service

# ---- DCS-NFR-SEC-12: the co-deployed PostgreSQL has no fixed password -------
render "${ALL[@]}" >"$TMP/a.yaml"
render "${ALL[@]}" >"$TMP/b.yaml"
pw_a="$(obj "$TMP/a.yaml" Secret dcs-postgresql "x['stringData']['postgres-password']")"
pw_b="$(obj "$TMP/b.yaml" Secret dcs-postgresql "x['stringData']['postgres-password']")"
[[ "$pw_a" != dcs && ${#pw_a} -ge 32 ]] || fail "default postgres password is fixed or short: $pw_a"
[[ "$pw_a" != "$pw_b" ]] || fail "default postgres password is not random"
grep -q "password=" <<<"$(env_of "$TMP/a.yaml" Deployment $B $C DATABASE_URL)" && fail "generated password in DATABASE_URL"
for w in "Deployment $B $C" "CronJob $B-db-backup pg-dump" "CronJob $B-pac-monitor pac-monitor"; do
  read -r kind name ctr <<<"$w"
  pg="$(env_of "$TMP/a.yaml" "$kind" "$name" "$ctr" PGPASSWORD)"
  grep -q "'name': 'dcs-postgresql', 'key': 'postgres-password'" <<<"$pg" || fail "$name gets no PGPASSWORD from the postgresql Secret: $pg"
done
# An explicit password (values.bdd.yml) is used as given.
render -f "$CHART_DIR/values.bdd.yml" >"$TMP/bdd.yaml"
[[ "$(obj "$TMP/bdd.yaml" Secret dcs-postgresql "x['stringData']['postgres-password']")" == dcs ]] || fail "explicit postgres password not used"
[[ "$(env_of "$TMP/bdd.yaml" Deployment $B $C DATABASE_URL)" == "host=dcs-postgresql port=5432 user=dcs password=dcs dbname=dcs sslmode=verify-full sslrootcert=/etc/dcs-internal-tls/ca.crt" ]] \
  || fail "BDD DATABASE_URL changed: $(env_of "$TMP/bdd.yaml" Deployment $B $C DATABASE_URL)"
[[ "$(env_of "$TMP/bdd.yaml" Deployment $B $C PGPASSWORD)" == None ]] || fail "PGPASSWORD with an explicit password"

# ---- DCS-NFR-SEC-14 / DCS-NFR-SEC-01: TLS for in-cluster traffic, on by default --
f="$TMP/a.yaml"
obj "$f" Secret dcs-internal-tls "__import__('base64').b64decode(x['data']['ca.crt']).decode()" >"$TMP/ca.crt"
obj "$f" Secret dcs-internal-tls "__import__('base64').b64decode(x['data']['tls.crt']).decode()" >"$TMP/tls.crt"
obj "$f" Secret dcs-internal-ca "__import__('base64').b64decode(x['data']['ca.crt']).decode()" >"$TMP/ca2.crt"
cmp -s "$TMP/ca.crt" "$TMP/ca2.crt" || fail "server Secret carries another CA"
openssl verify -CAfile "$TMP/ca.crt" "$TMP/tls.crt" >/dev/null || fail "server certificate not issued by the internal CA"
openssl x509 -in "$TMP/tls.crt" -noout -text | grep -q 'id-ecPublicKey' || fail "server key is not ECDSA"
sans="$(openssl x509 -in "$TMP/tls.crt" -noout -ext subjectAltName)"
for h in dcs-postgresql dcs-nats dcs-ipfs dcs-dss dcs-orce dcs-loki dcs-hydra $B $B-pdf-core dcs-postgresql.dcs.svc.cluster.local; do
  grep -q "DNS:$h\(,\|$\)" <<<"$sans" || fail "server certificate does not name $h"
done

# PostgreSQL: TLS 1.3, TCP only with TLS; the backend verifies the server name.
pg_args="$(obj "$f" Deployment dcs-postgresql "x['spec']['template']['spec']['containers'][0]['args'][0]")"
for a in "ssl=on" "ssl_min_protocol_version=TLSv1.3" "ssl_cert_file=/etc/dcs-internal-tls/tls.crt" "hba_file=/etc/postgresql-hba/pg_hba.conf"; do
  grep -q -- "-c $a" <<<"$pg_args" || fail "postgresql runs without -c $a"
done
hba="$(obj "$f" ConfigMap dcs-postgresql-hba "x['data']['pg_hba.conf']")"
grep -q '^hostssl all all all scram-sha-256$' <<<"$hba" || fail "pg_hba has no hostssl rule"
grep -q '^host ' <<<"$hba" && fail "pg_hba accepts TCP without TLS"
[[ "$(env_of "$f" Deployment $B $C DATABASE_URL)" == "host=dcs-postgresql port=5432 user=dcs dbname=dcs sslmode=verify-full sslrootcert=/etc/dcs-internal-tls/ca.crt" ]] \
  || fail "DATABASE_URL does not verify the server: $(env_of "$f" Deployment $B $C DATABASE_URL)"

# NATS: TLS 1.3 required.
grep -q 'min_version: "1.3"' <<<"$(obj "$f" ConfigMap dcs-nats "x['data']['nats.conf']")" || fail "NATS without TLS 1.3 minimum"
[[ "$(obj "$f" Deployment dcs-nats "x['spec']['template']['spec']['containers'][0]['args']")" == "['-js', '-sd', '/data', '-c', '/etc/nats/nats.conf']" ]] \
  || fail "NATS does not load its TLS config"
# NATS: the JetStream store is on the PersistentVolumeClaim (ADR-39).
[[ "$(obj "$f" Deployment dcs-nats "[v['persistentVolumeClaim']['claimName'] for v in x['spec']['template']['spec']['volumes'] if v['name'] == 'data']")" == "['dcs-nats']" ]] \
  || fail "the NATS JetStream store is not on a PersistentVolumeClaim"
[[ "$(obj "$f" PersistentVolumeClaim dcs-nats "x['spec']['resources']['requests']['storage']")" == 1Gi ]] \
  || fail "no PersistentVolumeClaim dcs-nats"
[[ "$(env_of "$f" Deployment $B $C NATS_URL)" == tls://dcs-nats:4222 ]] || fail "NATS_URL is not tls://"

# HTTP components: TLS proxy in each pod, the backend calls it and trusts the CA.
# The backend itself has one for in-cluster callers (ORCE).
for d in dcs-ipfs dcs-dss dcs-orce $B-pdf-core $B; do
  proxy="$(obj "$f" Deployment "$d" "next((c['args'][0] for c in x['spec']['template']['spec']['containers'] if c['name'] == 'tls-proxy'), None)")"
  grep -q 'ssl_protocols TLSv1.3;' <<<"$proxy" || fail "$d has no TLS 1.3 proxy"
  [[ "$(obj "$f" Service "$d" "[p['port'] for p in x['spec']['ports'] if p['name'] == 'https']")" == "[8443]" ]] || fail "service $d has no https port"
done
[[ "$(env_of "$f" Deployment $B $C IPFS_MFS_BASE_URL)" == https://dcs-ipfs:8443 ]] || fail "IPFS called without TLS"
[[ "$(env_of "$f" Deployment $B $C DSS_URL)" == https://dcs-dss:8443 ]] || fail "DSS called without TLS"
[[ "$(env_of "$f" Deployment $B $C PDF_CORE_URL)" == https://$B-pdf-core:8443 ]] || fail "pdf-core called without TLS"
for v in ORCE_ARCHIVE_NOTARY_URL ORCE_ARCHIVE_AUDIT_LOG_URL PAC_AUDIT_EXECUTOR_URL PAC_WORKFLOW_GATE_EXECUTOR_URL; do
  [[ "$(env_of "$f" Deployment $B $C $v)" == https://dcs-orce:8443/* ]] || fail "$v without TLS"
done
[[ "$(obj "$f" Service $B "[p['port'] for p in x['spec']['ports'] if p['name'] == 'http']")" == "[8991]" ]] || fail "backend http port (ingress, probes) removed"
# ORCE calls the backend and Hydra over https and trusts the internal CA.
[[ "$(env_of "$f" Deployment dcs-orce orce NODE_EXTRA_CA_CERTS)" == /etc/dcs-internal-tls/ca.crt ]] || fail "ORCE does not trust the internal CA"
obj "$f" Deployment dcs-orce "next(c for c in x['spec']['template']['spec']['containers'] if c['name'] == 'orce')['volumeMounts']" \
  | grep -q "'name': 'internal-ca', 'mountPath': '/etc/dcs-internal-tls'" || fail "ORCE does not mount the internal CA"
# The ontology and context IRIs stay the service identity.
[[ "$(env_of "$f" Deployment $B $C PDF_CORE_CONTEXT_IRI)" == http://$B-pdf-core:8080/ontology/dcs-pdf-core ]] || fail "context IRI changed"
[[ "$(env_of "$f" Deployment $B $C SSL_CERT_DIR)" == /etc/ssl/certs:/etc/dcs-internal-tls ]] || fail "backend does not trust the internal CA"
for w in "Deployment $B" "CronJob $B-db-backup" "CronJob $B-pac-monitor"; do
  read -r kind name <<<"$w"
  obj "$f" "$kind" "$name" "(x['spec']['template'] if 'template' in x['spec'] else x['spec']['jobTemplate']['spec']['template'])['spec']['containers'][0]['volumeMounts']" \
    | grep -q "'name': 'internal-ca', 'mountPath': '/etc/dcs-internal-tls'" || fail "$name does not mount the internal CA"
done

# A later upgrade keeps the CA: the template reads it back from the cluster.
grep -q 'lookup "v1" "Secret" .Release.Namespace (printf "%s-internal-ca" .Release.Name)' "$CHART_DIR/templates/_helpers.tpl" \
  || fail "internal CA not read back on upgrade"

# Off: no certificates and the plain endpoints.
render "${ALL[@]}" --set global.internalTLS.enabled=false >"$TMP/off.yaml"
grep -q 'internal-tls' "$TMP/off.yaml" && fail "internal TLS rendered while global.internalTLS.enabled=false"
[[ "$(env_of "$TMP/off.yaml" Deployment $B $C NATS_URL)" == nats://dcs-nats:4222 ]] || fail "NATS_URL without TLS changed"
[[ "$(env_of "$TMP/off.yaml" Deployment $B $C IPFS_MFS_BASE_URL)" == http://dcs-ipfs:5001 ]] || fail "IPFS URL without TLS changed"
grep -q 'name: tls-proxy' "$TMP/off.yaml" && fail "TLS proxy rendered while global.internalTLS.enabled=false"

# The kind CI (BDD and e2e: values.bdd.yml, values.bdd2.yml) runs the TLS paths
# of prod, with no XFSC statuslist-service.
for prof in "bdd dcs" "bdd2 dcs2"; do
  read -r v r <<<"$prof"
  helm template "$r" "$CHART_DIR" -n dcs -f "$CHART_DIR/values.$v.yml" >"$TMP/ci-$v.yaml"
  f="$TMP/ci-$v.yaml"
  rb="$r-digital-contracting-service"
  obj "$f" Secret "$r-internal-tls" "x['type']" >/dev/null || fail "values.$v.yml: no internal TLS"
  [[ "$(env_of "$f" Deployment "$rb" $C DATABASE_URL)" == *"sslmode=verify-full sslrootcert=/etc/dcs-internal-tls/ca.crt" ]] || fail "values.$v.yml: database without TLS"
  [[ "$(env_of "$f" Deployment "$rb" $C NATS_URL)" == "tls://$r-nats:4222" ]] || fail "values.$v.yml: NATS without TLS"
  [[ "$(env_of "$f" Deployment "$rb" $C IPFS_MFS_BASE_URL)" == "https://$r-ipfs:8443" ]] || fail "values.$v.yml: IPFS without TLS"
  [[ "$(env_of "$f" Deployment "$rb" $C PDF_CORE_URL)" == "https://$rb-pdf-core:8443" ]] || fail "values.$v.yml: pdf-core without TLS"
  [[ "$(env_of "$f" Deployment "$r-hydra" hydra DSN)" == *"@$r-postgresql:5432/hydra?sslmode=verify-full&sslrootcert=/etc/dcs-internal-tls/ca.crt" ]] \
    || fail "values.$v.yml: Hydra reaches the database without TLS"
  grep -q '^  name: [a-z0-9-]*statuslist-service$' "$f" && fail "values.$v.yml deploys the XFSC statuslist-service"
  # Hydra admin API: TLS 1.3 proxy in the pod, called over https by the
  # backend. The Service has no plain admin port; each Hydra pod registers the
  # clients over loopback.
  grep -q 'ssl_protocols TLSv1.3;' <<<"$(obj "$f" Deployment "$r-hydra" "next((c['args'][0] for c in x['spec']['template']['spec']['containers'] if c['name'] == 'tls-proxy'), None)")" \
    || fail "values.$v.yml: Hydra has no TLS 1.3 proxy"
  [[ "$(obj "$f" Service "$r-hydra" "[p['port'] for p in x['spec']['ports'] if p['name'] == 'https']")" == "[8443]" ]] || fail "values.$v.yml: Hydra service has no https port"
  [[ "$(env_of "$f" Deployment "$rb" $C HYDRA_ADMIN_URL)" == "https://$r-hydra:8443" ]] || fail "values.$v.yml: backend calls the Hydra admin API without TLS"
  [[ "$(obj "$f" Service "$r-hydra" "[p['port'] for p in x['spec']['ports'] if p['targetPort'] == 'admin' or p['port'] == 4445]")" == "[]" ]] || fail "values.$v.yml: plain Hydra admin port on the Service"
  [[ "$(obj "$f" Service "$r-ipfs" "[p['port'] for p in x['spec']['ports'] if p['targetPort'] == 'api' or p['port'] == 5001]")" == "[]" ]] || fail "values.$v.yml: plain IPFS RPC port on the Service"
  [[ "$(env_of "$f" Deployment "$r-hydra" register-clients HYDRA_ADMIN)" == "http://127.0.0.1:4445" ]] || fail "values.$v.yml: Hydra pods do not register the clients over loopback"
  obj "$f" Job "$r-hydra-clients" "x" >/dev/null 2>&1 && fail "values.$v.yml: Hydra clients still registered by a hook Job"
  proxy="$(obj "$f" Deployment "$rb" "next((c['readinessProbe']['httpGet']['path'] for c in x['spec']['template']['spec']['containers'] if c['name'] == 'statuslist-localhost-proxy'), None)")"
  [[ "$proxy" == /issuer/status-list/1 ]] || fail "values.$v.yml: localhost proxy readiness depends on $proxy"
done
for v in DSS_URL PAC_AUDIT_EXECUTOR_URL PAC_WORKFLOW_GATE_EXECUTOR_URL TSA_URL DCS_TRUST_PDP_URL ORCE_ARCHIVE_NOTARY_URL; do
  [[ "$(env_of "$TMP/ci-bdd.yaml" Deployment $B $C $v)" == https://dcs-*:8443* ]] || fail "values.bdd.yml: $v without TLS"
done
[[ "$(env_of "$TMP/ci-bdd.yaml" Deployment dcs-orce orce DCS_URL)" == https://$B:8443/* ]] || fail "values.bdd.yml: ORCE calls the backend without TLS"
# deploy.sh probes DCS_TRUST_PDP_URL with curl in the backend pod. curl does not
# read the internal CA from SSL_CERT_DIR, so the probe gives it the CA file.
pdp_probe="$(sed -n '/^pdp_url=/,/^fi$/p' "$CHART_DIR/deploy.sh")"
grep -q -- '--cacert' <<<"$pdp_probe" && grep -q '/etc/dcs-internal-tls/ca.crt' <<<"$pdp_probe" \
  || fail "deploy.sh probes the trust PDP without the internal CA"

# The issuer release renders the orce chart on its own, without the parent.
helm template dcs-issuer "$CHART_DIR/charts/orce" -f "$CHART_DIR/values.issuer-base.yml" -f "$CHART_DIR/values.issuer.bdd.yml" \
  | grep -q 'tls-proxy' && fail "standalone orce chart renders the TLS proxy"

# ---- DCS-NFR-SEC-03, DCS-NFR-SF-02, DCS-NFR-SEC-14: NetworkPolicies ----------
# Every restricted API admits only its clients, every policy selects real pods
# on ports they have, and every pod the chart owns is under the default deny.
for prof in "all" "bdd" "bdd2"; do
  case "$prof" in
    all) f="$TMP/a.yaml" ;;
    *) f="$TMP/ci-$prof.yaml" ;;
  esac
  python3 - "$f" "$prof" <<'EOF' || fail "NetworkPolicies of $prof"
import sys, yaml
docs = [d for d in yaml.safe_load_all(open(sys.argv[1])) if d]
nps = {d["metadata"]["name"]: d["spec"] for d in docs if d["kind"] == "NetworkPolicy"}
pods = []
for d in docs:
    s = d["spec"] if "spec" in d else {}
    if d["kind"] in ("Deployment", "StatefulSet", "DaemonSet", "Job"):
        t = s["template"]
    elif d["kind"] == "CronJob":
        t = s["jobTemplate"]["spec"]["template"]
    else:
        continue
    ports = {p["name"] for c in t["spec"]["containers"] for p in c.get("ports", []) if "name" in p}
    pods.append((d["metadata"]["name"], t.get("metadata", {}).get("labels", {}), ports))
rel = next(d["metadata"]["labels"]["app.kubernetes.io/instance"] for d in docs if d["kind"] == "NetworkPolicy")

def matches(sel, labels):
    if any(labels.get(k) != v for k, v in sel.get("matchLabels", {}).items()):
        return False
    return all(labels.get(e["key"]) in e["values"] for e in sel.get("matchExpressions", []))

def selected(sel):
    return [p for p in pods if matches(sel, p[1])]

def peer_pods(rule):
    return {p[0] for f in rule.get("from", []) if "podSelector" in f and "namespaceSelector" not in f for p in selected(f["podSelector"])}

errors = []
deny = nps[next(n for n in nps if n.endswith("-default-deny"))]
assert deny["policyTypes"] == ["Ingress"] and not deny.get("ingress"), "default deny admits traffic"
owned = [p for p in pods if p[1].get("app.kubernetes.io/instance") == rel and p[2]]
for p in owned:
    if p not in selected(deny["podSelector"]):
        errors.append("pod %s with ports is not under the default deny" % p[0])
for n, spec in nps.items():
    targets = selected(spec["podSelector"])
    if not targets:
        errors.append("%s selects no pod" % n)
    for r in spec.get("ingress", []):
        for port in r.get("ports", []):
            if not any(port["port"] in t[2] for t in targets):
                errors.append("%s: no selected pod has port %s" % (n, port["port"]))
        for f in r.get("from", []):
            other_release = f.get("podSelector", {}).get("matchLabels", {}).get("app.kubernetes.io/instance", rel) != rel
            if "podSelector" in f and "namespaceSelector" not in f and not other_release and not selected(f["podSelector"]):
                errors.append("%s: peer %s selects no pod" % (n, f))

def admitted(component, port):
    np = next((s for n, s in nps.items() if n.endswith("-" + component)), None)
    if np is None:
        return "absent"
    out = []
    for r in np.get("ingress", []):
        if any(p["port"] == port for p in r.get("ports", [])):
            out.append(None if "from" not in r else peer_pods(r))
    return out

# The "all" render has no Hydra; the BDD renders cover it.
deployed_elsewhere = {"hydra"} if not any(p[1].get("app.kubernetes.io/name") == "hydra" for p in pods) else set()
backend = {p[0] for p in pods if p[1].get("app.kubernetes.io/component") == "backend" and p[1].get("app.kubernetes.io/instance") == rel}
for component, port in (("hydra", "admin"), ("hydra", "https"), ("ipfs", "api"), ("ipfs", "https"), ("ipfs", "gateway"),
                        ("nats", "client"), ("pdf-core", "https")):
    got = admitted(component, port)
    if got != [backend] and not (got == "absent" and component in deployed_elsewhere):
        errors.append("%s port %s admits %s, not only the backend %s" % (component, port, got, backend))
clients = backend | {p[0] for p in pods if p[1].get("app.kubernetes.io/name") == "hydra" or p[0].endswith(("-db-backup", "-pac-monitor"))}
if admitted("postgresql", "postgres") != [clients]:
    errors.append("postgresql admits %s, not its clients %s" % (admitted("postgresql", "postgres"), clients))
if admitted("hydra", "public") != [None] and "hydra" not in deployed_elsewhere:
    errors.append("hydra port public is not open")
# DCS-NFR-SEC-14: without networkPolicy.ingressFrom the backend, ORCE and DSS
# admit every source; the BDD renders name the Traefik ingress and the other
# BDD release, and then they admit only those and the pods of the release.
prof = sys.argv[2]
other = {"bdd": "dcs2", "bdd2": "dcs"}.get(prof)
ingress = {"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "kube-system"}},
           "podSelector": {"matchLabels": {"app.kubernetes.io/name": "traefik"}}}
def peers(component):
    np = next((sp for n, sp in nps.items() if n.endswith("-" + component)), None)
    return None if np is None else [r_.get("from") for r_ in np.get("ingress", []) if "metrics" not in [x["port"] for x in r_.get("ports", [])]]
if other is None:
    if admitted("backend", "https") != [None]:
        errors.append("backend port https is not open without ingressFrom")
    if any(n.endswith(("-orce", "-dss")) for n in nps):
        errors.append("ORCE or DSS restricted without ingressFrom")
else:
    want = [ingress, {"podSelector": {"matchLabels": {"app.kubernetes.io/instance": rel}}},
            {"podSelector": {"matchLabels": {"app.kubernetes.io/instance": other}}}]
    for component in ("backend", "orce", "dss"):
        deployed = any(p[1].get("app.kubernetes.io/name") == component or (component == "backend" and p[0] in backend) for p in pods)
        if deployed and peers(component) != [want]:
            errors.append("%s admits %s, not only the ingress, release %s and release %s" % (component, peers(component), rel, other))
    public = next((sp for n, sp in nps.items() if n.endswith("-public")), None)
    if public and {"orce", "dss"} & set(next(e["values"] for e in public["podSelector"]["matchExpressions"] if e["key"] == "app.kubernetes.io/name")):
        errors.append("ORCE or DSS still open to every source")
if errors:
    sys.exit("\n".join(errors))
EOF
done
[[ "$(render "${ALL[@]}" --set networkPolicy.enabled=false | grep -c 'kind: NetworkPolicy')" == 0 ]] || fail "NetworkPolicies with networkPolicy.enabled=false"
# A production-like release with an ingress names the ingress controller.
out="$(render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --set networkPolicy.ingressFrom=null 2>&1 || true)"
grep -q 'networkPolicy.ingressFrom: the pods of the ingress controller' <<<"$out" || fail "a production render without networkPolicy.ingressFrom"
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" --show-only templates/networkpolicy.yaml \
  | python3 -c "
import sys, yaml
nps = {d['metadata']['name']: d['spec'] for d in yaml.safe_load_all(sys.stdin) if d}
backend = next(sp for n, sp in nps.items() if n.endswith('-backend'))
assert all('from' in r for r in backend['ingress']), 'the prod backend admits every source'
" || fail "the production backend admits every source"
# An empty peer list leaves the rule out instead of admitting every source.
render "${ALL[@]}" --set networkPolicy.metricsFrom=null --show-only templates/networkpolicy.yaml | grep -q 'port: metrics' \
  && fail "metrics port admitted without metricsFrom"

# ---- DCS-NFR-SF-02: the Kubo RPC API needs the bearer token --------------------
f="$TMP/a.yaml"
init="$(obj "$f" Deployment dcs-ipfs "x['spec']['template']['spec']['initContainers'][0]['args'][0]")"
grep -q 'API.Authorizations "{\\"dcs\\":{\\"AuthSecret\\":\\"bearer:$IPFS_API_TOKEN\\"' <<<"$init" || fail "Kubo API without authorization: $init"
for w in "Deployment dcs-ipfs ipfs" "Deployment $B $C"; do
  read -r kind name ctr <<<"$w"
  grep -q "'name': 'dcs-ipfs-api', 'key': 'token'" <<<"$(env_of "$f" "$kind" "$name" "$ctr" IPFS_API_TOKEN)" || fail "$name gets no IPFS_API_TOKEN"
done
[[ "$(obj "$f" Secret dcs-ipfs-api "len(x['stringData']['token'])")" -ge 32 ]] || fail "short Kubo API token"
render "${ALL[@]}" --set ipfs.api.auth.enabled=false >"$TMP/noauth.yaml"
grep -q "API.Authorizations '{}'" <<<"$(obj "$TMP/noauth.yaml" Deployment dcs-ipfs "x['spec']['template']['spec']['initContainers'][0]['args'][0]")" \
  || fail "Kubo keeps an authorization after api.auth.enabled=false"
[[ "$(env_of "$TMP/noauth.yaml" Deployment $B $C IPFS_API_TOKEN)" == None ]] || fail "IPFS_API_TOKEN without Kubo authorization"
render "${ALL[@]}" --set ipfs.apiTokenSecretRef.name=ext >"$TMP/ext.yaml"
[[ "$(env_of "$TMP/ext.yaml" Deployment $B $C IPFS_API_TOKEN)" == *"'name': 'ext', 'key': 'IPFS_API_TOKEN'"* ]] || fail "ipfs.apiTokenSecretRef not used"

echo 'in-cluster security render tests passed'
