"""Step definitions for real_signing_vertical.feature's PAdES-B-B
fallback scenario: takes pdf-core's RFC 3161 TSA endpoint away at runtime,
signs through it while it's down (so pdf-core soft-falls-back to
PAdES-B-B), and restores it — then proves a fresh signing recovers
PAdES-B-T.

--- Why this patches pdf-core's TSA env rather than scaling the whole ORCE
deployment to 0 ---

Scaling the shared ORCE deployment to 0 replicas does
NOT work for this scenario, because /signature/apply has TWO independent
runtime dependencies on ORCE, only one of which has a fallback:

  1. The PAdES RFC3161 TIMESTAMP (pdf-core -> ORCE http://dcs-orce:1880/tsa/,
     via DCS_PDF_CORE_TSA_URL). This one DOES soft-fall-back to PAdES-B-B —
     pdf-core/compiler/pades.go's signPAdESWithFallback logs
     "WARN pades: TSA %s failed, falling back to PAdES-B-B (no timestamp)".
  2. The ARCHIVE NOTARY + archive TSA (backend -> ORCE
     http://dcs-orce:1880/archive/notary, in
     backend/internal/signingmanagement/command/apply.go's archive step).
     This one HARD-FAILS the whole /signature/apply transaction with a 500
     ("could not notarize archive entry: ... connect: connection refused")
     when ORCE is unreachable — it has no fallback.

So scaling ALL of ORCE to 0 makes dependency #2 abort /signature/apply
before a PAdES-B-B PDF is ever persisted or exportable — the B-B fallback
(#1) becomes unobservable at the black-box surface. (The archive-notary
step does not degrade gracefully the way PAdES timestamping does; changing
that would be backend work in apply.go.)

PAdES-B-B is specifically a pdf-core capability keyed off pdf-core's own
DCS_PDF_CORE_TSA_URL env var (deployment/helm/templates/pdf-core-deployment.
yaml:57). Repointing THAT env at an unreachable address and rolling pdf-core
takes the TSA away from the PAdES path ONLY, leaving ORCE (and hence the
backend's archive-notary, dependency #2) fully up — so /signature/apply
completes and yields a genuine, inspectable PAdES-B-B PDF. This isolates the
exact behavior the scenario is about.

--- Shared-cluster safety ---

pdf-core is PER-RELEASE (instance A's dcs-...-pdf-core is a separate
Deployment from instance B's dcs2-...-pdf-core), so this only affects
instance A — it never touches instance B or the shared ORCE. It nonetheless
MUST run under the suite-wide flock for the whole scenario, because any
OTHER agent signing against instance A during the
TSA-down window would unexpectedly get a B-B signature. Restoration is
layered: (1) a context.add_cleanup restores the original env value + rolls
pdf-core back even on failure; (2) the scenario itself also explicitly
restores and re-asserts PAdES-B-T recovery as part of its own Then steps.
"""

from __future__ import annotations

import os
import subprocess
import time

from behave import then, when

from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (
    _TIMESTAMP_TOKEN_OID_DER,
    _apply_signature,
    _pdf_bytes_for,
    _run_full_ceremony,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.pdf_service import PDFService
from steps.template_management.contract_state_machine_steps import _advance_to_approved


# An address that fails fast with "connection refused" from inside the
# pdf-core pod: TCP port 1 on loopback is not listened on, so the RFC3161
# HTTP POST errors immediately, deterministically triggering the PAdES-B-B
# fallback branch (rather than a long DNS/connect timeout).
_UNREACHABLE_TSA_URL = "http://127.0.0.1:1/tsa/"

_TSA_ENV_VAR = "DCS_PDF_CORE_TSA_URL"


def _kubectl_base() -> list:
    kubectl = os.environ.get("KUBECTL_BIN", "kubectl")
    namespace = os.environ.get("K8S_NAMESPACE")
    assert namespace, (
        "K8S_NAMESPACE is not set — required to target instance A's pdf-core "
        "deployment for the PAdES-B-B fallback scenario. Hard-failing rather than "
        "guessing a namespace for an infra mutation."
    )
    return [kubectl, "-n", namespace]


def _pdf_core_deployment_name() -> str:
    # Instance A's pdf-core deployment. Overridable for CI parity, defaulting
    # to the release-"dcs" name the BDD Helm chart produces.
    return os.environ.get("BDD_PDF_CORE_DEPLOYMENT", "dcs-digital-contracting-service-pdf-core")


def _run(cmd: list, **kwargs):
    timeout = kwargs.pop("timeout", 60)
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, **kwargs)


def _current_tsa_env() -> str:
    proc = _run(_kubectl_base() + [
        "get", "deployment", _pdf_core_deployment_name(),
        "-o", (
            "jsonpath={.spec.template.spec.containers[0].env"
            f"[?(@.name=='{_TSA_ENV_VAR}')].value}}"
        ),
    ])
    assert proc.returncode == 0, (
        f"could not read {_TSA_ENV_VAR} from {_pdf_core_deployment_name()}: {proc.stderr}"
    )
    value = proc.stdout.strip()
    assert value, (
        f"{_TSA_ENV_VAR} is unset/empty on {_pdf_core_deployment_name()} — cannot run "
        f"the PAdES-B-B fallback scenario without a configured TSA to take away"
    )
    return value


def _set_tsa_env(value: str):
    proc = _run(_kubectl_base() + [
        "set", "env", f"deployment/{_pdf_core_deployment_name()}",
        f"{_TSA_ENV_VAR}={value}",
    ])
    assert proc.returncode == 0, (
        f"kubectl set env {_TSA_ENV_VAR}={value} on {_pdf_core_deployment_name()} failed: "
        f"{proc.stderr}"
    )


def _wait_pdf_core_ready(timeout_s: int = 180):
    proc = _run(
        _kubectl_base() + [
            "rollout", "status", f"deployment/{_pdf_core_deployment_name()}",
            f"--timeout={timeout_s}s",
        ],
        timeout=timeout_s + 30,
    )
    assert proc.returncode == 0, (
        f"pdf-core deployment {_pdf_core_deployment_name()} did not roll out to Ready: "
        f"{proc.stdout}\n{proc.stderr}"
    )


def _wait_pdf_core_pod_tsa_env(expected: str, timeout_s: int = 150):
    """Wait until every RUNNING pdf-core pod actually serves the expected TSA
    env value. `kubectl set env` + `rollout status` can report the previous
    generation ready before the new pod takes over, which would let a sign
    hit a pod with the old TSA setting — so confirm on the live pods, not just
    the deployment, before signing."""
    deadline = time.time() + timeout_s
    last = ""
    while time.time() < deadline:
        proc = _run(_kubectl_base() + [
            "get", "pods",
            "-l", "app.kubernetes.io/component=pdf-core,app.kubernetes.io/instance=dcs",
            "--field-selector=status.phase=Running",
            "-o", f"jsonpath={{.items[*].spec.containers[?(@.name=='pdf-core')]"
                  f".env[?(@.name=='{_TSA_ENV_VAR}')].value}}",
        ])
        last = proc.stdout
        values = proc.stdout.split()
        if values and all(v == expected for v in values):
            return
        time.sleep(2)
    raise AssertionError(
        f"pdf-core running pods did not converge on {_TSA_ENV_VAR}={expected!r} within "
        f"{timeout_s}s (last seen: {last!r})"
    )


def _pdf_core_log_lines(since_seconds: int) -> str:
    proc = _run(_kubectl_base() + [
        "logs", "-l", "app.kubernetes.io/component=pdf-core,app.kubernetes.io/instance=dcs",
        "-c", "pdf-core", f"--since={since_seconds}s",
    ])
    assert proc.returncode == 0, f"could not read pdf-core logs: {proc.stderr}"
    return proc.stdout


# ---------------------------------------------------------------------------
# When — take the TSA away / restore it
# ---------------------------------------------------------------------------


@when("pdf-core's RFC3161 TSA endpoint is made unavailable for this scenario")
def step_when_tsa_unavailable(context, ):
    original = _current_tsa_env()
    context.pdf_core_original_tsa_url = original

    def _restore():
        if getattr(context, "pdf_core_tsa_broken", False):
            _set_tsa_env(original)
            _wait_pdf_core_ready()
            _wait_pdf_core_pod_tsa_env(original)
            context.pdf_core_tsa_broken = False

    context.add_cleanup(_restore)

    context.tsa_down_since = time.time()
    _set_tsa_env(_UNREACHABLE_TSA_URL)
    _wait_pdf_core_ready()
    _wait_pdf_core_pod_tsa_env(_UNREACHABLE_TSA_URL)
    context.pdf_core_tsa_broken = True


@when("pdf-core's RFC3161 TSA endpoint is restored")
def step_when_tsa_restored(context):
    original = getattr(context, "pdf_core_original_tsa_url", None)
    assert original is not None, (
        "No prior 'pdf-core's RFC3161 TSA endpoint is made unavailable' step recorded "
        "the original TSA URL for this scenario"
    )
    _set_tsa_env(original)
    _wait_pdf_core_ready()
    _wait_pdf_core_pod_tsa_env(original)
    context.pdf_core_tsa_broken = False


# ---------------------------------------------------------------------------
# When — sign (TSA down => B-B; TSA up => B-T). A dedicated @when step rather
# than the existing @given "has an AES-signed PDF via a completed ceremony"
# step because behave matches steps by (keyword_type, text): a @given step is
# not found when a scenario reaches these lines under a When/And.
# ---------------------------------------------------------------------------


@when(
    'contract "{name}" has an AES-signed PDF via a completed ceremony for signatory '
    '"{signatory_name}", signed while the TSA is unavailable'
)
def step_when_sign_while_tsa_down(context, name, signatory_name):
    _sign_contract_via_ceremony(context, name, signatory_name, tsa_down=True)


@when(
    'contract "{name}" has an AES-signed PDF via a completed ceremony for signatory '
    '"{signatory_name}", signed after the TSA is restored'
)
def step_when_sign_after_tsa_restored(context, name, signatory_name):
    _sign_contract_via_ceremony(context, name, signatory_name, tsa_down=False)


def _sign_contract_via_ceremony(context, name, signatory_name, *, tsa_down):
    party_did = ContractService._local_peer_did(context)
    ContractService._create_contract_in_draft(context, name)
    _advance_to_approved(context, name)
    _, _, subject_did = _run_full_ceremony(context, name, field_name=party_did, signatory_name=signatory_name)

    apply_resp = _apply_signature(context, name, signer_did=subject_did, credential_type="AES")
    if tsa_down:
        detail = (
            "with the TSA unavailable — PAdES-B-B fallback (pdf-core) should let signing "
            "SUCCEED without a timestamp, not fail outright"
        )
    else:
        detail = "after the TSA was restored"
    assert apply_resp.status_code == 200, (
        f"POST /signature/apply failed for contract '{name}' {detail}: "
        f"{apply_resp.status_code} {apply_resp.text}"
    )
    ContractService._refresh_contract(context, name)

    signed_did, _ = ContractService._contract_data(context, name)
    context.headers = AuthService.get_headers_for_roles(["Contract Manager"])
    export_resp = PDFService.export_contract_pdf(context, signed_did)
    assert export_resp.status_code == 200, (
        f"PDF export failed for signed contract '{name}': {export_resp.status_code} {export_resp.text}"
    )
    if not hasattr(context, "pdf_bytes"):
        context.pdf_bytes = {}
    context.pdf_bytes[name] = export_resp.content


# ---------------------------------------------------------------------------
# Then — B-B fallback signals
# ---------------------------------------------------------------------------


@then('the signed PDF for contract "{name}" carries no RFC3161 timestamp token')
def step_then_no_rfc3161_timestamp(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    hex_needle_lower = _TIMESTAMP_TOKEN_OID_DER.hex().encode()
    hex_needle_upper = _TIMESTAMP_TOKEN_OID_DER.hex().upper().encode()
    assert hex_needle_lower not in pdf_bytes and hex_needle_upper not in pdf_bytes, (
        "Expected NO RFC3161 signatureTimeStampToken (PAdES-B-B, TSA was unavailable) "
        f"in the signed PDF for contract '{name}', but its DER-encoded OID was found"
    )


@then('pdf-core logged a PAdES-B-B fallback WARN')
def step_then_pdf_core_logged_bb_fallback(context):
    started = getattr(context, "tsa_down_since", None)
    assert started is not None, "No recorded TSA-down window for this scenario"
    # The WARN is emitted during signing but can lag in `kubectl logs`; poll.
    deadline = time.time() + 30
    since_seconds = 30
    while True:
        since_seconds = max(int(time.time() - started) + 30, 30)
        logs = _pdf_core_log_lines(since_seconds)
        if "falling back to PAdES-B-B" in logs:
            return
        if time.time() >= deadline:
            break
        time.sleep(2)
    assert False, (
        "Expected pdf-core to have logged the WARN documented in "
        "pdf-core/compiler/pades.go ('WARN pades: TSA %s failed, falling back to "
        f"PAdES-B-B (no timestamp): %v') during the TSA-down window, found no such line "
        f"in the last {since_seconds}s of pdf-core logs"
    )
