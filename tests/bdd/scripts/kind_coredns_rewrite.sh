#!/usr/bin/env bash
# Idempotently patches the kind cluster's CoreDNS Corefile with `rewrite`
# rules so the BDD did:web hostnames (dcs-a.localhost, dcs-b.localhost)
# resolve *in-cluster* to the shared Traefik Service, then verifies the
# rewrite actually took effect before returning.
#
# Why this exists: two DCS releases (dcs, dcs2) in namespace dcs-bdd each
# fetch the OTHER instance's (and, for the self-peer scenarios, their OWN)
# did:web document at https://<hostname>/.well-known/did.json from inside
# their own pod (backend/internal/service/dcs_to_dcs.go). Those hostnames
# are public origins (dcs-a.localhost:18080 / dcs-b.localhost:18080) that
# nothing in-cluster serves DNS for by default — CoreDNS's `rewrite` plugin
# maps them onto the in-cluster Traefik Service FQDN instead, which then
# routes back out to whichever release's Ingress matches the Host header.
#
# USER CONSTRAINT: no /etc/hosts writes, no sudo anywhere in this harness.
# Host-side (i.e. non-cluster) resolution of the same hostnames is handled
# separately, without sudo, by the RFC 6761 *.localhost fallback resolver in
# environment.py's before_all hook — this script only touches in-cluster DNS.
#
# Idempotent: safe to run on every `make kind_deploy` (not only once at
# cluster-create time) — a prior version that only ran once at cluster
# creation was observed to not reliably stick, so this always re-verifies
# actual resolution and fails loudly rather than silently leaving stale DNS.
set -euo pipefail

KUBECTL_BIN="${KUBECTL_BIN:-kubectl}"
NAMESPACE="${KIND_NAMESPACE:-dcs-bdd}"
TRAEFIK_FQDN="traefik.kube-system.svc.cluster.local"
HOSTS=("dcs-a.localhost" "dcs-b.localhost")

MARKER_BEGIN="# BEGIN dcs-bdd-hostname-rewrite (managed by kind_coredns_rewrite.sh)"
MARKER_END="# END dcs-bdd-hostname-rewrite"

log() { echo "kind_coredns_rewrite: $*"; }
fail() { echo "kind_coredns_rewrite: FATAL — $*" >&2; exit 1; }

corefile_current="$("$KUBECTL_BIN" -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}')" \
  || fail "could not read the coredns ConfigMap in namespace kube-system"

if grep -qF "$MARKER_BEGIN" <<<"$corefile_current"; then
  log "rewrite rules already present in the Corefile, skipping insert."
else
  log "inserting CoreDNS rewrite rules for: ${HOSTS[*]} -> ${TRAEFIK_FQDN}"

  hosts_csv="$(IFS=,; echo "${HOSTS[*]}")"
  new_corefile="$(
    CURRENT_COREFILE="$corefile_current" \
    TRAEFIK_FQDN="$TRAEFIK_FQDN" \
    HOSTS_CSV="$hosts_csv" \
    MARKER_BEGIN="$MARKER_BEGIN" \
    MARKER_END="$MARKER_END" \
    python3 <<'PYEOF'
import os
import sys

corefile = os.environ["CURRENT_COREFILE"]
traefik_fqdn = os.environ["TRAEFIK_FQDN"]
hosts = os.environ["HOSTS_CSV"].split(",")
marker_begin = os.environ["MARKER_BEGIN"]
marker_end = os.environ["MARKER_END"]

block_lines = [f"    {marker_begin}"]
block_lines += [f"    rewrite name {h} {traefik_fqdn}" for h in hosts]
block_lines.append(f"    {marker_end}")
block = "\n".join(block_lines)

lines = corefile.splitlines()
out = []
inserted = False
for line in lines:
    out.append(line)
    # The top-level server block, e.g. ".:53 {" — insert right after its
    # opening brace so `rewrite` runs before `forward`/`kubernetes`.
    if not inserted and line.strip().startswith(".") and line.rstrip().endswith("{"):
        out.append(block)
        inserted = True

if not inserted:
    sys.stderr.write("could not locate the top-level '.:53 {' server block in the Corefile\n")
    sys.exit(1)

sys.stdout.write("\n".join(out) + "\n")
PYEOF
  )" || fail "failed to compute the patched Corefile (see above)"

  patch_json="$(python3 - "$new_corefile" <<'PYEOF'
import json
import sys

print(json.dumps({"data": {"Corefile": sys.argv[1]}}))
PYEOF
  )"

  "$KUBECTL_BIN" -n kube-system patch configmap coredns --type merge --patch "$patch_json" >/dev/null \
    || fail "kubectl patch of the coredns ConfigMap failed"

  log "Corefile patched; restarting CoreDNS."
  "$KUBECTL_BIN" -n kube-system rollout restart deployment/coredns \
    || fail "could not restart the coredns Deployment"
fi

log "waiting for the CoreDNS rollout..."
"$KUBECTL_BIN" -n kube-system rollout status deployment/coredns --timeout=120s \
  || fail "coredns did not roll out within 120s"

log "verifying in-cluster DNS resolution actually took effect (this is the part a" \
  "Corefile-content check alone cannot prove)..."
verified=false
deadline=$(( $(date +%s) + 90 ))
check_pod="coredns-rewrite-check-$$"
while [[ "$(date +%s)" -lt "$deadline" ]]; do
  if "$KUBECTL_BIN" -n "$NAMESPACE" run "$check_pod" \
      --image=busybox:1.36 --restart=Never --rm -i --quiet \
      --command -- sh -c 'nslookup dcs-a.localhost && nslookup dcs-b.localhost' \
      >/tmp/kind_coredns_rewrite_check.$$ 2>&1; then
    verified=true
    break
  fi
  sleep 3
done
check_output="$(cat /tmp/kind_coredns_rewrite_check.$$ 2>/dev/null || true)"
rm -f "/tmp/kind_coredns_rewrite_check.$$"

if [[ "$verified" != "true" ]]; then
  echo "kind_coredns_rewrite: FATAL — dcs-a.localhost / dcs-b.localhost did NOT resolve" >&2
  echo "in-cluster after patching CoreDNS and waiting for its rollout. Last check output:" >&2
  echo "$check_output" >&2
  echo "Current Corefile:" >&2
  "$KUBECTL_BIN" -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}' >&2 || true
  exit 1
fi

log "OK — dcs-a.localhost and dcs-b.localhost resolve in-cluster via CoreDNS rewrite -> ${TRAEFIK_FQDN}."
