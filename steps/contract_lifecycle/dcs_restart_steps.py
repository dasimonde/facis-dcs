"""DCS restart (DCS-NFR-SF-01): the DCS deployment is restarted mid-flow and a
contract created before the restart is read and continued afterwards. All
contract state is in Postgres and IPFS, so the new pod needs no recovery.

The restart follows the ORCE restart step
(steps/audit_compliance/external_checkpoint_sink_steps.py step_worker_restarts).
The harness forwards and the HSM exec seam resolve the pod at each call
(tests/bdd/scripts/run_bdd_helm.sh), so they survive the new pod.
"""

import os
import subprocess
import time

import requests
from behave import then, when

from steps.support.services.contract_service import ContractService


def _backend_pods(kubectl, namespace, release):
    out = subprocess.run(
        [
            kubectl, "-n", namespace, "get", "pod",
            "-l", f"app.kubernetes.io/component=backend,app.kubernetes.io/instance={release}",
            "-o", "jsonpath={.items[*].metadata.name}",
        ],
        capture_output=True, text=True, timeout=30,
    )
    assert out.returncode == 0, f"Could not list DCS pods: {out.stderr}"
    return set(out.stdout.split())


@when("the DCS deployment restarts")
def step_dcs_restarts(context):
    _replace_backend_pods(context, ["rollout", "restart", f"deployment/{_deployment()}"])


@when('the creator submits contract "{name}" for review and the DCS pod is killed at once')
def step_submit_then_kill(context, name):
    # DRAFT -> NEGOTIATION -> SUBMITTED; the outbox events of the submit are
    # written, and the pod goes before its jobs publish and anchor them.
    ContractService._prepare_contract_under_review(context, name)
    release = os.getenv("HELM_RELEASE", "dcs")
    pods = sorted(_backend_pods(os.getenv("KUBECTL_BIN", "kubectl"), os.getenv("K8S_NAMESPACE", "dcs-bdd"), release))
    _replace_backend_pods(context, ["delete", "pod", "--grace-period=0", "--force", "--wait=false", *pods])


@then('every outbox event of contract "{name}" is published and anchored')
def step_outbox_delivered(context, name):
    did, _ = ContractService._contract_data(context, name)
    deadline = time.monotonic() + 240
    while True:
        cursor = context.db.cursor()
        cursor.execute(
            "SELECT count(*), count(*) FILTER (WHERE NOT published OR NOT processed) FROM outbox_events WHERE did = %s",
            (did,),
        )
        total, pending = cursor.fetchone()
        cursor.close()
        if total > 0 and pending == 0:
            return
        assert time.monotonic() < deadline, (
            f"{pending} of {total} outbox events of contract {name!r} ({did}) are not published and anchored after the kill"
        )
        time.sleep(3)


def _deployment():
    return os.getenv("DCS_DEPLOYMENT", "dcs-digital-contracting-service")


def _replace_backend_pods(context, kubectl_args):
    """Runs kubectl_args, which replace the backend pods, and waits until new
    pods serve the API and the harness forwards reach them."""
    namespace = os.getenv("K8S_NAMESPACE", "dcs-bdd")
    deployment = _deployment()
    release = os.getenv("HELM_RELEASE", "dcs")
    kubectl = os.getenv("KUBECTL_BIN", "kubectl")

    before = _backend_pods(kubectl, namespace, release)
    restarted = subprocess.run(
        [kubectl, "-n", namespace, *kubectl_args],
        capture_output=True, text=True, timeout=60,
    )
    assert restarted.returncode == 0, (
        f"kubectl {' '.join(kubectl_args)} failed for DCS deployment {deployment!r}: {restarted.stderr}"
    )
    deadline = time.monotonic() + 120
    while not (_backend_pods(kubectl, namespace, release) - before):
        assert time.monotonic() < deadline, f"No new DCS pod replaced {sorted(before)}"
        time.sleep(2)
    ready = subprocess.run(
        [kubectl, "-n", namespace, "rollout", "status", f"deployment/{deployment}", "--timeout=300s"],
        capture_output=True, text=True, timeout=310,
    )
    assert ready.returncode == 0, f"DCS did not become ready after restart: {ready.stderr}"
    new_pods = _backend_pods(kubectl, namespace, release) - before
    assert new_pods, f"The restart created no new DCS pod (before: {sorted(before)})"
    context.dcs_restart_pods = new_pods

    # The ingress follows the new endpoints shortly after the rollout ends.
    health = f"{context.base_url}/auth/login"
    deadline = time.monotonic() + 120
    status = None
    while time.monotonic() < deadline:
        try:
            status = requests.post(health, json={}, timeout=10).status_code
        except requests.RequestException:
            status = None
        if status is not None and status < 500 and status != 404:
            break
        time.sleep(2)
    else:
        raise AssertionError(f"DCS did not answer at {health} after the restart (last HTTP {status})")
    _wait_for_harness_forwards()


def _wait_for_harness_forwards():
    """A kubectl port-forward to the replaced pod fails the first connection
    after the rollout and then exits; keep_port_forward.sh starts a new one to
    the new pod. Use up that first failure here, so the next scenario that
    calls a forward gets a response."""
    for origin, path in (
        (os.getenv("BDD_DCS_INTERNAL_ORIGIN", ""), "/readyz"),
        (os.getenv("BDD_DCS_METRICS_ORIGIN", ""), "/metrics"),
    ):
        origin = origin.strip().rstrip("/")
        if not origin:
            continue
        deadline = time.monotonic() + 60
        while True:
            try:
                requests.get(origin + path, timeout=10)
                break
            except requests.RequestException as exc:
                if time.monotonic() >= deadline:
                    raise AssertionError(f"Harness forward {origin} did not reconnect after the restart: {exc}") from exc
                time.sleep(2)


@when('the creator submits contract "{name}" for review after the restart')
def step_submit_after_restart(context, name):
    # DRAFT -> NEGOTIATION -> SUBMITTED, both submits on the new pod.
    ContractService._prepare_contract_under_review(context, name)

