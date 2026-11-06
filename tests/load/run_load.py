"""Load test of the kind BDD stack (DCS-NFR-PER-01, DCS-NFR-PER-02).

Runs from tests/bdd/scripts/run_bdd_helm.sh with RUN_MODE=load (make -C
tests/bdd run_load_kind_once), which supplies the environment and the venv.

1. Creates an approved template and one contract through the API.
2. Runs tests/load/dcs_load.js (k6 container) against one DCS replica.
3. Scales the DCS deployment to two replicas and runs it again at the same
   arrival rate. The same limits apply, and each replica must serve at least
   a quarter of the requests, so the added replica takes load.
4. Capacity (DCS-NFR-PER-02): with one and then two replicas, raises the read
   rate step by step (LOAD_CAPACITY_RATES) until a step breaks a limit. The
   capacity is the highest rate that held every limit; at that step the
   requests each replica served give its throughput at saturation. The test
   fails when two replicas hold less than half of one replica: on one node
   the steps double, so one step is runner noise.
5. Restores the replica count and the per-caller rate limit, which the run
   lifts so that the phases measure the stack and not the limit of one caller.

What one runner shows: the CI runner is one 4-vCPU node that also runs kind,
PostgreSQL, IPFS, Keycloak, Hydra, ORCE, DSS, pdf-core and k6. Both replicas
share its CPUs with that stack, so the capacity of two replicas cannot be
twice that of one there: the test shows that the load spreads, that a replica
keeps its throughput next to a second one, and the absolute capacity of this
node. Linear scaling needs a replica per node; run this script against a
cluster with one node per replica (LOAD_CAPACITY_RATES and the kind config
are the knobs) to measure it.

The k6 summaries go to tests/bdd/.reports/load/. Exit code 1 when a limit is
exceeded.
"""

import json
import os
import subprocess
import sys
import time
from pathlib import Path
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / "steps"))

from steps.support import localhost_resolver  # noqa: E402
from steps.support.services.auth_service import AuthService  # noqa: E402
from steps.support.services.contract_service import ContractService  # noqa: E402

K6_IMAGE = os.getenv("K6_IMAGE", "grafana/k6:1.3.0")
KUBECTL = os.getenv("KUBECTL_BIN", "kubectl")
NAMESPACE = os.environ["K8S_NAMESPACE"]
DEPLOYMENT = os.environ["DCS_DEPLOYMENT"]
RELEASE = os.environ["HELM_RELEASE"]
BASE_URL = os.environ["BDD_DCS_BASE_URL"].rstrip("/")
READ_RATE = int(os.getenv("LOAD_READ_RATE", "20"))
WRITE_RATE = int(os.getenv("LOAD_WRITE_RATE", "2"))
DURATION = os.getenv("LOAD_DURATION", "60s")
CAPACITY_RATES = [int(r) for r in os.getenv("LOAD_CAPACITY_RATES", "40,80,160,320").split(",")]
STEP_DURATION = os.getenv("LOAD_STEP_DURATION", "30s")
REPORTS = ROOT / "tests" / "bdd" / ".reports" / "load"
MIN_SHARE = 0.25


def kubectl(*args):
    return subprocess.run([KUBECTL, "-n", NAMESPACE, *args], check=True, capture_output=True, text=True).stdout


def backend_pods():
    selector = f"app.kubernetes.io/component=backend,app.kubernetes.io/instance={RELEASE}"
    return kubectl("get", "pod", "-l", selector, "--field-selector=status.phase=Running",
                   "-o", "jsonpath={.items[*].metadata.name}").split()


def requests_served(pod):
    """Sum of http_requests_total of one pod, read through the API server proxy."""
    raw = subprocess.run(
        [KUBECTL, "get", "--raw", f"/api/v1/namespaces/{NAMESPACE}/pods/{pod}:9464/proxy/metrics"],
        check=True, capture_output=True, text=True,
    ).stdout
    return sum(float(line.rsplit(" ", 1)[1]) for line in raw.splitlines() if line.startswith("http_requests_total{"))


def scale(replicas):
    kubectl("scale", f"deployment/{DEPLOYMENT}", f"--replicas={replicas}")
    kubectl("rollout", "status", f"deployment/{DEPLOYMENT}", "--timeout=300s")
    deadline = time.monotonic() + 120
    while len(backend_pods()) != replicas:
        assert time.monotonic() < deadline, f"expected {replicas} running DCS pods, got {backend_pods()}"
        time.sleep(2)


def run_k6(phase, setup, read_rate, write_rate, duration=DURATION):
    REPORTS.mkdir(parents=True, exist_ok=True)
    # The k6 image runs as its own user; it writes the summary into this directory.
    REPORTS.chmod(0o777)
    (REPORTS / f"{phase}.json").unlink(missing_ok=True)
    env = {
        "DCS_BASE_URL": BASE_URL,
        # A fresh token per phase: the access token outlives one phase only.
        "DCS_TOKEN": AuthService.get_headers_for_roles(["Contract Creator"])["Authorization"].split(" ", 1)[1],
        "CONTRACT_DID": setup["contract_did"],
        "TEMPLATE_DID": setup["template_did"],
        "PARTIES": json.dumps(setup["parties"]),
        "READ_RATE": str(read_rate),
        "WRITE_RATE": str(write_rate),
        "DURATION": duration,
        "MAX_VUS": str(max(100, read_rate * 2)),
    }
    cmd = ["docker", "run", "--rm", "--network", "host", "-v", f"{ROOT / 'tests' / 'load'}:/scripts:ro", "-v", f"{REPORTS}:/out"]
    for key, value in env.items():
        cmd += ["-e", f"{key}={value}"]
    cmd += [K6_IMAGE, "run", "--summary-export", f"/out/{phase}.json", "/scripts/dcs_load.js"]
    print(f"== k6 {phase}: {read_rate} reads/s, {write_rate} writes/s for {duration}", flush=True)
    return subprocess.run(cmd).returncode == 0


def p95(phase, kind):
    metrics = json.loads((REPORTS / f"{phase}.json").read_text())["metrics"]
    value = metrics.get(f"http_req_duration{{kind:{kind}}}", {}).get("p(95)")
    return "n/a" if value is None else round(value)


def seconds(duration):
    return float(duration.rstrip("s"))


def capacity(replicas, setup):
    """Highest read rate of CAPACITY_RATES that held every limit, and the
    requests per second each replica served at that step."""
    scale(replicas)
    pods = backend_pods()
    best, per_replica = 0, {}
    for rate in CAPACITY_RATES:
        before = {pod: requests_served(pod) for pod in pods}
        held = run_k6(f"capacity-{replicas}-{rate}", setup, rate, 0, STEP_DURATION)
        if not held:
            break
        best = rate
        per_replica = {pod: round((requests_served(pod) - before[pod]) / seconds(STEP_DURATION), 1) for pod in pods}
    return best, per_replica


def main():
    localhost_resolver.install()
    ctx = SimpleNamespace(base_url=BASE_URL, http_timeout_seconds=60.0)
    ContractService._create_contract_in_draft(ctx, "load")
    setup = {
        "contract_did": ctx.contract_dids["load"],
        # The writes create contracts from a template of their own.
        "template_did": ContractService._create_approved_template_for_contract(ctx),
        "parties": ContractService._reading_parties(ctx)["parties"],
    }

    original = int(kubectl("get", f"deployment/{DEPLOYMENT}", "-o", "jsonpath={.spec.replicas}"))
    # k6 is one caller, and the per-caller budget (3000 requests a minute)
    # would cap every phase at 50 requests/s. The load run lifts it, so the
    # phases measure the stack; the 429 at the budget has its own scenario
    # (features/12_system_based_contract_management/contract_lifecycle_via_api.feature).
    kubectl("set", "env", f"deployment/{DEPLOYMENT}", "DCS_API_RATE_LIMIT_PER_MINUTE=1000000")
    ok = True
    try:
        scale(1)
        ok &= run_k6("replicas-1", setup, READ_RATE, WRITE_RATE)

        scale(2)
        pods = backend_pods()
        before = {pod: requests_served(pod) for pod in pods}
        ok &= run_k6("replicas-2", setup, READ_RATE, WRITE_RATE)
        served = {pod: requests_served(pod) - before[pod] for pod in pods}
        total = sum(served.values())
        print(f"== requests served per replica: {served}", flush=True)
        for pod, count in served.items():
            if total == 0 or count / total < MIN_SHARE:
                print(f"FAIL: replica {pod} served {count} of {total} requests (< {MIN_SHARE:.0%})", flush=True)
                ok = False

        capacities = {replicas: capacity(replicas, setup) for replicas in (1, 2)}
        print(f"== capacity (reads/s that held every limit, per-replica requests/s): {capacities}", flush=True)
        # Both replicas share one node, and the steps double, so one step of
        # runner noise halves the result. Fail only below that step.
        if capacities[2][0] * 2 < capacities[1][0]:
            print(f"FAIL: two replicas held {capacities[2][0]} reads/s, less than half of one replica ({capacities[1][0]})", flush=True)
            ok = False
    finally:
        kubectl("set", "env", f"deployment/{DEPLOYMENT}", "DCS_API_RATE_LIMIT_PER_MINUTE-")
        scale(original)

    lines = [
        "## DCS load test (NFR-PER-01, NFR-PER-02)",
        "",
        "| DCS replicas | load (reads/s, writes/s) | read p95 ms | write p95 ms |",
        "|---|---|---|---|",
    ]
    for phase, replicas, factor in (("replicas-1", 1, 1), ("replicas-2", 2, 1)):
        if (REPORTS / f"{phase}.json").exists():
            lines.append(f"| {replicas} | {factor * READ_RATE}, {factor * WRITE_RATE} | "
                         f"{p95(phase, 'read')} | {p95(phase, 'write')} |")
    if "capacities" in locals():
        lines += ["", "| DCS replicas | capacity (reads/s held) | requests/s per replica at that rate |", "|---|---|---|"]
        for replicas, (best, per_replica) in capacities.items():
            lines.append(f"| {replicas} | {best} | {', '.join(str(v) for v in per_replica.values()) or 'n/a'} |")
        lines += ["", f"Capacity steps: {', '.join(map(str, CAPACITY_RATES))} reads/s for {STEP_DURATION} each. "
                  "One CI node runs both replicas and the whole stack, so this shows the spread and the "
                  "per-replica throughput at saturation, not linear scaling."]
    lines += ["", "Limits: read p95 < 500 ms, write p95 < 2000 ms, < 1 % failed, no dropped iterations, "
              f"each of 2 replicas serves >= {MIN_SHARE:.0%}, 2 replicas hold at least half the capacity of 1 (one step of runner noise).",
              f"Result: {'passed' if ok else 'FAILED'}"]
    print("\n".join(lines))
    summary = os.getenv("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as f:
            f.write("\n".join(lines) + "\n")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
