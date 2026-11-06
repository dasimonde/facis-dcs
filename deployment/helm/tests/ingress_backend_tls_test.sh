#!/usr/bin/env bash
# Render checks for TLS from the ingress controller to the backend, the Hydra
# public API and the Federated Catalogue (DCS-NFR-SEC-01, DCS-NFR-SEC-14).
# Needs helm, openssl and python3 with PyYAML.
# Run: bash deployment/helm/tests/ingress_backend_tls_test.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ACK=(--set federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true)
CRD=(--api-versions traefik.io/v1alpha1/ServersTransport)
FC=(--set fcservice.enabled=true --set fcservice.route.path=/fc --set hydra.enabled=true
  --set hydra.config.selfIssuerURL=https://dcs.example.org --set hydra.config.dev=true)
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

render() { helm template dcs "$CHART_DIR" -n dcs "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Prints, per rendered Ingress, Service and ServersTransport, what this test asserts.
summary() {
  python3 - "$1" <<'EOF'
import sys, yaml
for x in yaml.safe_load_all(open(sys.argv[1])):
    if not x:
        continue
    k, n = x['kind'], x['metadata']['name']
    ann = x['metadata'].get('annotations') or {}
    if k == 'Ingress' and n == 'dcs-digital-contracting-service':
        for r in x['spec']['rules']:
            for p in r['http']['paths']:
                s = p['backend']['service']
                print('route %s %s' % (s['name'], s['port']['number']))
        for a, v in ann.items():
            print('ingress-annotation %s=%s' % (a, v))
    if k == 'Service' and n in ('dcs-digital-contracting-service', 'dcs-hydra', 'fc-service'):
        for p in x['spec']['ports']:
            print('port %s %s %s' % (n, p['name'], p['port']))
        print('transport %s %s' % (n, ann.get('traefik.ingress.kubernetes.io/service.serverstransport', '-')))
    if k == 'ServersTransport':
        print('serverstransport %s %s %s' % (n, x['spec']['serverName'], x['spec']['rootCAs'][0]['secret']))
    if k == 'Deployment' and n in ('dcs-hydra', 'fc-service'):
        for c in x['spec']['template']['spec']['containers']:
            if c['name'] == 'tls-proxy':
                for p in c['ports']:
                    print('proxy %s %s %s' % (n, p['name'], p['containerPort']))
                print('proxy-conf %s %s' % (n, ' '.join(c['args'][0].split())))
    if k == 'Deployment' and n == 'dcs-digital-contracting-service':
        for c in x['spec']['template']['spec']['containers']:
            for e in c.get('env', []):
                if e['name'] == 'HYDRA_INTERNAL_ISSUER_URL':
                    print('env HYDRA_INTERNAL_ISSUER_URL %s' % e.get('value'))
    if k == 'Secret' and n == 'dcs-internal-tls':
        import base64
        open(sys.argv[1] + '.leaf.pem', 'wb').write(base64.b64decode(x['data']['tls.crt']))
EOF
}

# ---- Traefik (BDD stack: className traefik, the CRD exists) -----------------
render -f "$CHART_DIR/values.bdd.yml" "${CRD[@]}" --set fcservice.route.path=/fc >"$TMP/bdd.yaml"
summary "$TMP/bdd.yaml" >"$TMP/bdd.txt"
grep -q '^route dcs-digital-contracting-service 8443$' "$TMP/bdd.txt" || fail "traefik: backend route is not on the TLS proxy port"
grep -q '^route dcs-digital-contracting-service 8991$' "$TMP/bdd.txt" && fail "traefik: a backend route uses the plain port"
grep -q '^route dcs-hydra 8444$' "$TMP/bdd.txt" || fail "traefik: Hydra route is not on the public TLS port"
grep -q '^route fc-service 8443$' "$TMP/bdd.txt" || fail "traefik: Federated Catalogue route is not on the TLS proxy port"
# Traefik uses https for a Service port whose name starts with "https".
grep -q '^port dcs-digital-contracting-service https 8443$' "$TMP/bdd.txt" || fail "traefik: backend port 8443 is not named https"
grep -q '^port dcs-hydra https-public 8444$' "$TMP/bdd.txt" || fail "traefik: Hydra port 8444 is not named https-public"
grep -q '^port fc-service https 8443$' "$TMP/bdd.txt" || fail "traefik: fc-service port 8443 is not named https"
for svc in dcs-digital-contracting-service dcs-hydra fc-service; do
  grep -q "^transport $svc dcs-dcs-internal-tls@kubernetescrd$" "$TMP/bdd.txt" || fail "traefik: Service $svc does not name the ServersTransport"
done
grep -q '^serverstransport dcs-internal-tls dcs-digital-contracting-service.dcs.svc dcs-internal-tls$' "$TMP/bdd.txt" \
  || fail "traefik: no ServersTransport with the internal CA and the server name"
grep -q '^ingress-annotation nginx' "$TMP/bdd.txt" && fail "traefik: ingress-nginx annotations on a Traefik ingress"
# The Hydra public listener forwards to the public API, the admin listener stays on the admin API.
grep -q '^proxy dcs-hydra https-public 8444$' "$TMP/bdd.txt" || fail "Hydra TLS proxy does not listen on 8444"
grep '^proxy-conf dcs-hydra' "$TMP/bdd.txt" | grep -q 'listen 8443 ssl; .* proxy_pass http://127.0.0.1:4445; .* listen 8444 ssl; .* proxy_pass http://127.0.0.1:4444;' \
  || fail "Hydra TLS proxy: 8443 is not the admin API or 8444 is not the public API"
grep '^proxy-conf fc-service' "$TMP/bdd.txt" | grep -q 'listen 8443 ssl; ssl_certificate /etc/dcs-internal-tls/tls.crt; .* ssl_protocols TLSv1.3; .* proxy_pass http://127.0.0.1:8081;' \
  || fail "fc-service has no TLS 1.3 proxy to its http port"
# The TLS proxy passes the peer address on, so callers behind it do not all
# share the loopback address the lockout is keyed on (FR-UC-01-4).
grep '^proxy-conf fc-service' "$TMP/bdd.txt" | grep -q 'proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for; proxy_pass' \
  || fail "the TLS proxy does not set X-Forwarded-For"
# The backend calls the Hydra public API (discovery, token) on its TLS proxy too.
grep -q '^env HYDRA_INTERNAL_ISSUER_URL https://dcs-hydra:8444$' "$TMP/bdd.txt" \
  || fail "the backend calls the Hydra public API on the plain port"
# The one leaf certificate carries the names the controller verifies.
sans="$(openssl x509 -in "$TMP/bdd.yaml.leaf.pem" -noout -ext subjectAltName)"
for n in dcs-digital-contracting-service.dcs.svc dcs-hydra.dcs.svc fc-service.dcs.svc; do
  grep -q "DNS:$n\b" <<<"$sans" || fail "internal certificate lacks $n"
done

# ---- ingress-nginx (className nginx) -----------------------------------------
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" "${FC[@]}" --set ingress.className=nginx >"$TMP/nginx.yaml"
summary "$TMP/nginx.yaml" >"$TMP/nginx.txt"
grep -q '^route dcs-digital-contracting-service 8443$' "$TMP/nginx.txt" || fail "nginx: backend route is not on the TLS proxy port"
grep -q '^route dcs-hydra 8444$' "$TMP/nginx.txt" || fail "nginx: Hydra route is not on the public TLS port"
grep -q '^route fc-service 8443$' "$TMP/nginx.txt" || fail "nginx: Federated Catalogue route is not on the TLS proxy port"
for a in 'backend-protocol=HTTPS' 'proxy-ssl-secret=dcs/dcs-internal-tls' 'proxy-ssl-verify=on' \
  'proxy-ssl-name=dcs-digital-contracting-service.dcs.svc' 'proxy-ssl-protocols=TLSv1.3'; do
  grep -q "^ingress-annotation nginx.ingress.kubernetes.io/$a$" "$TMP/nginx.txt" || fail "nginx: ingress lacks $a"
done
grep -q '^serverstransport' "$TMP/nginx.txt" && fail "nginx: ServersTransport without the Traefik CRD"

# ---- no backend TLS controller: plain ports, no TLS annotations --------------
for args in "--set global.internalTLS.enabled=false --set ingress.className=nginx" "--set ingress.className=haproxy" ""; do
  # shellcheck disable=SC2086
  render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" "${FC[@]}" $args >"$TMP/plain.yaml"
  summary "$TMP/plain.yaml" >"$TMP/plain.txt"
  grep -q '^route dcs-digital-contracting-service 8991$' "$TMP/plain.txt" || fail "[$args] backend route is not on the plain port"
  grep -q '^route dcs-hydra 4444$' "$TMP/plain.txt" || fail "[$args] Hydra route is not on the public port"
  grep -q '^route fc-service 8081$' "$TMP/plain.txt" || fail "[$args] Federated Catalogue route is not on the plain port"
  grep -q '^ingress-annotation nginx.ingress.kubernetes.io/backend-protocol' "$TMP/plain.txt" && fail "[$args] backend-protocol HTTPS without TLS proxies"
done
render -f "$CHART_DIR/values.bdd.yml" "${CRD[@]}" --set global.internalTLS.enabled=false >"$TMP/notls.yaml"
summary "$TMP/notls.yaml" | grep -q '^env HYDRA_INTERNAL_ISSUER_URL http://dcs-hydra:4444$' \
  || fail "without global.internalTLS the backend does not call the Hydra public port"
# An empty className with the Traefik CRD selects Traefik, as for the TLSOption.
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" "${FC[@]}" "${CRD[@]}" >"$TMP/prodt.yaml"
summary "$TMP/prodt.yaml" | grep -q '^route dcs-digital-contracting-service 8443$' || fail "empty className with the Traefik CRD: backend route is plain"

echo "ingress_backend_tls_test: OK"
