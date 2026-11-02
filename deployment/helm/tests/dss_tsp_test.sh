#!/usr/bin/env bash
# Render checks for the timestamp authority of DSS (ADR-40): dss.tsp.url
# replaces the in-process TSA of the image, and a production-like release does
# not render without it. Needs helm, python3 with PyYAML.
# Run: bash deployment/helm/tests/dss_tsp_test.sh
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ACK=(--set federatedCatalogue.remote.acknowledgeAdminAllTrustBoundary=true)

render() { helm template dcs "$CHART_DIR" -n dcs "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

# Checks the DSS ConfigMap and the DSS container mounts against an expected
# TSA URL ("" = the in-process TSA of the image).
check() {
  python3 -c '
import sys, yaml, xml.etree.ElementTree as ET
want = sys.argv[1]
docs = {(d["kind"], d["metadata"]["name"]): d for d in yaml.safe_load_all(sys.stdin) if d}
data = docs[("ConfigMap", "dcs-dss-custom-properties")]["data"]
props = data["dss-custom.properties"]
assert props.endswith("\n"), "dss-custom.properties must end each line with a newline"
lines = [l for l in props.splitlines() if l.strip()]
container = next(c for c in docs[("Deployment", "dcs-dss")]["spec"]["template"]["spec"]["containers"] if c["name"] == "dss")
mounts = {m["mountPath"]: m for m in container["volumeMounts"]}
if not want:
    assert not any(l.startswith("tsp-source") for l in lines), lines
    assert "tsp-config.xml" not in data, data.keys()
    assert "/dss/tsp-config.xml" not in mounts, mounts
    sys.exit(0)
assert "tsp-source=file:/dss/tsp-config.xml" in lines, lines
assert mounts["/dss/tsp-config.xml"]["subPath"] == "tsp-config.xml", mounts
bean = ET.fromstring(data["tsp-config.xml"]).find("{http://www.springframework.org/schema/beans}bean")
assert bean.get("class") == "eu.europa.esig.dss.service.tsp.OnlineTSPSource", bean.attrib
arg = bean.find("{http://www.springframework.org/schema/beans}constructor-arg")
assert arg.get("name") == "tspServer" and arg.get("value") == want, arg.attrib
' "$1"
}

render "${ACK[@]}" --set dss.enabled=true | check "" || fail "without dss.tsp.url the image TSA is replaced"
url='https://qtsa.example.org/tsr?policy=1&x=2'
render "${ACK[@]}" --set dss.enabled=true --set-string "dss.tsp.url=$url" | check "$url" \
  || fail "dss.tsp.url does not configure the DSS TSP source"

for profile in prod acceptance; do
  values=(-f "$CHART_DIR/values.$profile.yml" -f "$CHART_DIR/tests/$profile.prerequisites.yml")
  render "${values[@]}" | check "https://qtsa.example.org/tsr" || fail "$profile: the DSS TSP source is not the configured TSA"
  out="$(render "${values[@]}" --set dss.tsp.url= 2>&1 || true)"
  grep -q 'dss.tsp.url: a qualified RFC 3161 timestamp authority' <<<"$out" \
    || fail "$profile renders with DSS but without a qualified TSA: $out"
  render "${values[@]}" --set dss.tsp.url= --set dss.enabled=false --set dss.url=https://dss.example.org >/dev/null \
    || fail "$profile: an external DSS needs no dss.tsp.url"
done

echo 'DSS TSA render tests passed'
