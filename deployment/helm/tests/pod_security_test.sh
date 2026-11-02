#!/usr/bin/env bash
# Render checks for the security context of every pod the chart owns
# (DCS-NFR-SEC-05) and for image digests. Needs helm and python3 with PyYAML.
# Run: bash deployment/helm/tests/pod_security_test.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ACK=(--set federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true)
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

render() { helm template dcs "$CHART_DIR" -n dcs "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Every chart-owned pod of the BDD, prod and acceptance profiles, of the
# optional components (ACME renewal, flow import job, XFSC statuslist-service)
# and of the standalone issuer release.
render -f "$CHART_DIR/values.bdd.yml" >"$TMP/bdd.yaml"
render -f "$CHART_DIR/values.prod.yml" -f "$CHART_DIR/tests/prod.prerequisites.yml" >"$TMP/prod.yaml"
render -f "$CHART_DIR/values.acceptance.yml" -f "$CHART_DIR/tests/acceptance.prerequisites.yml" >"$TMP/acceptance.yaml"
render -f "$CHART_DIR/values.bdd.yml" --set orce.flowsFile.enabled=false --set statuslistService.enabled=true \
  --set acmeRenewal.enabled=true --set acmeRenewal.host=dcs.example.org --set acmeRenewal.email=ops@example.org \
  --set acmeRenewal.secretName=tls --set backup.enabled=true --set logging.enabled=true \
  --set fcservice.portal.enabled=true >"$TMP/optional.yaml"
grep -q 'name: fc-demo-portal$' "$TMP/optional.yaml" || fail "FC portal not rendered"
helm template dcs-issuer "$CHART_DIR/charts/orce" -f "$CHART_DIR/values.issuer-base.yml" \
  -f "$CHART_DIR/values.issuer.bdd.yml" >"$TMP/issuer.yaml"

python3 - "$TMP"/*.yaml <<'EOF'
import sys, yaml

# Containers that do not get the full restricted context, and why
# (deployment/README.md, "Pod security").
EXCEPTIONS = {
    # kc.sh start builds Quarkus into /opt/keycloak at startup.
    ("fc-keycloak", "keycloak"): {"readOnlyRootFilesystem"},
    # The user of the XFSC image is not published.
    ("dcs-statuslist-service", "statuslist-service"): {"runAsNonRoot", "readOnlyRootFilesystem"},
    # Gives a hostPath volume (kind BDD stack) to the ORCE user: root, CAP_CHOWN only.
    ("dcs-orce", "volume-permissions"): {"runAsNonRoot"},
    ("dcs-issuer-orce", "volume-permissions"): {"runAsNonRoot"},
}
THIRD_PARTY_CHARTS = ("traefik", "kube-prometheus-stack", "grafana", "prometheus")

def pod_spec(x):
    s = x["spec"]
    if x["kind"] == "CronJob":
        return s["jobTemplate"]["spec"]["template"]["spec"]
    if x["kind"] == "Pod":
        return s
    return s["template"]["spec"]

errors, seen = [], 0
for f in sys.argv[1:]:
    for x in yaml.safe_load_all(open(f)):
        if not x or x["kind"] not in ("Deployment", "StatefulSet", "Job", "CronJob", "Pod", "DaemonSet"):
            continue
        chart = x["metadata"].get("labels", {}).get("helm.sh/chart", "")
        if chart.startswith(THIRD_PARTY_CHARTS):
            continue
        name = x["metadata"]["name"]
        spec = pod_spec(x)
        psc = spec.get("securityContext") or {}
        for c in spec.get("initContainers", []) + spec["containers"]:
            seen += 1
            csc = c.get("securityContext") or {}
            eff = {**psc, **csc}
            skip = EXCEPTIONS.get((name, c["name"]), set())
            where = "%s: %s/%s" % (f.rsplit("/", 1)[-1], name, c["name"])
            if "runAsNonRoot" not in skip and not (eff.get("runAsNonRoot") is True and eff.get("runAsUser", 1) != 0):
                errors.append(where + ": not runAsNonRoot with a non-root UID")
            if "readOnlyRootFilesystem" not in skip and csc.get("readOnlyRootFilesystem") is not True:
                errors.append(where + ": root filesystem writable")
            if csc.get("allowPrivilegeEscalation") is not False:
                errors.append(where + ": privilege escalation allowed")
            if "ALL" not in (csc.get("capabilities") or {}).get("drop", []):
                errors.append(where + ": capabilities not dropped")
            if (eff.get("seccompProfile") or {}).get("type") != "RuntimeDefault":
                errors.append(where + ": no RuntimeDefault seccomp profile")
if errors:
    sys.exit("\n".join(errors))
assert seen > 30, "only %d containers checked" % seen
EOF

# The backend runs as the user of the DCS image, and its /tmp is an emptyDir.
grep -q '^ARG DOCKER_UID=10001$' "$CHART_DIR/../docker/Dockerfile" || fail "DCS image user is not 10001"
grep -q '^USER ${DOCKER_UID}:${DOCKER_UID}$' "$CHART_DIR/../docker/Dockerfile" || fail "DCS image does not run as DOCKER_UID"
grep -q '^USER 10001:10001$' "$CHART_DIR/../../pdf-core/Dockerfile" || fail "pdf-core image does not run as 10001"
# DCS-NFR-SEC-05, DCS-NFR-SQ-02: the runtime base is pinned by digest, the
# default target is the production image, and the BDD helper and the dev trust
# fixtures exist only in the ci stage (tests/bdd/Makefile builds --target ci).
python3 - "$CHART_DIR/../docker/Dockerfile" "$CHART_DIR/../../tests/bdd/Makefile" <<'EOF' || fail "DCS image stages"
import re, sys
text = open(sys.argv[1]).read()
stages = re.split(r"^FROM ", text, flags=re.M)[1:]
heads = [s.split("\n", 1)[0] for s in stages]
runtime = next(s for s in stages if s.split("\n", 1)[0].endswith(" AS runtime"))
assert re.match(r"debian:stable-slim@sha256:[0-9a-f]{64} AS runtime$", runtime.split("\n", 1)[0]), heads
assert heads[-1] == "runtime AS production", heads
ci = next(s for s in stages if s.split("\n", 1)[0] == "runtime AS ci")
for name in ("hsmsign", "trust.dev.json", "x5c-trust-anchors.dev.pem"):
    assert name in ci, name
    assert not re.search(r"^COPY .*" + re.escape(name), runtime, flags=re.M), name + " in the runtime image"
assert "--target ci" in open(sys.argv[2]).read(), "the BDD image is not the ci target"
EOF
python3 - "$TMP/prod.yaml" <<'EOF' || fail "backend pod user or /tmp"
import sys, yaml
d = next(x for x in yaml.safe_load_all(open(sys.argv[1])) if x and x["kind"] == "Deployment"
         and x["metadata"]["name"] == "dcs-digital-contracting-service")
spec = d["spec"]["template"]["spec"]
assert spec["securityContext"]["runAsUser"] == 10001, spec["securityContext"]
c = next(c for c in spec["containers"] if c["name"] == "digital-contracting-service")
assert {"name": "tmp", "mountPath": "/tmp"} in c["volumeMounts"], c["volumeMounts"]
assert {"name": "tmp", "emptyDir": {}} in spec["volumes"], spec["volumes"]
EOF

# nginx containers start nginx directly: the configuration scripts of the
# image entrypoint try to edit /etc/nginx/conf.d on the read-only filesystem.
python3 - "$TMP/bdd.yaml" <<'EOF' || fail "nginx container runs the image entrypoint"
import sys, yaml
n = 0
for x in yaml.safe_load_all(open(sys.argv[1])):
    if not x or x["kind"] != "Deployment":
        continue
    for c in x["spec"]["template"]["spec"]["containers"]:
        if c["image"].startswith("nginx:"):
            n += 1
            assert c.get("command", [""])[0] in ("nginx", "/bin/sh"), "%s/%s: %s" % (x["metadata"]["name"], c["name"], c.get("command"))
assert n >= 6, n
EOF

# Image digests: a digest value pins the image as <repository>:<tag>@<digest>.
D=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
digests=()
for k in image pdfCore.image global.internalTLS.proxy.image statusListLocalhostProxy.image backup.image \
  postgresql.image hydra.image nats.image orce.image dss.image ipfs.image logging.loki.image \
  logging.collector.image logging.gateway.image statuslistService.image; do
  digests+=(--set "$k.digest=$D")
done
render -f "$CHART_DIR/values.bdd.yml" --set statuslistService.enabled=true --set backup.enabled=true \
  --set logging.enabled=true "${digests[@]}" >"$TMP/digest.yaml"
python3 - "$TMP/digest.yaml" <<'EOF' || fail "image without digest"
import sys, yaml
bad = set()
for x in yaml.safe_load_all(open(sys.argv[1])):
    if not x or x["kind"] not in ("Deployment", "StatefulSet", "Job", "CronJob"):
        continue
    if x["metadata"].get("labels", {}).get("helm.sh/chart", "").startswith(("traefik", "kube-prometheus-stack")):
        continue
    s = x["spec"]
    spec = s["jobTemplate"]["spec"]["template"]["spec"] if x["kind"] == "CronJob" else s["template"]["spec"]
    for c in spec.get("initContainers", []) + spec["containers"]:
        if "@sha256:" not in c["image"]:
            bad.add("%s/%s: %s" % (x["metadata"]["name"], c["name"], c["image"]))
sys.exit("\n".join(sorted(bad)) if bad else 0)
EOF
grep -q "image: \"\[registry\]:1.16.0@$D\"" "$TMP/digest.yaml" || fail "backend image does not carry the digest"
render -f "$CHART_DIR/values.bdd.yml" | grep -q '@sha256:0123' && fail "digest rendered without a digest value"

echo 'pod security render tests passed'
