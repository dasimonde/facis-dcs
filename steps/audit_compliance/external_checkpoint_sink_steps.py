"""BDD bindings for sequential external publication of audit checkpoints."""

from __future__ import annotations

import os
import re
import subprocess
import time
from datetime import datetime

from behave import given, then, when

from steps.support.services.checkpoint_sink_service import CheckpointSinkService
from steps.support.services.contract_service import ContractService
from steps.support.services.orce_audit_control_service import (
    OrceAuditControlService,
)


CHANNEL = "checkpoint_sink"
PUBLIC_FIELDS = {
    "seq",
    "root",
    "prev_root",
    "leaf_count",
    "created_at",
    "tsa_timestamp",
    "timestamped_at",
}


def _mode(value: str) -> str:
    return value.strip().lower().replace("-", "_").replace(" ", "_")


def _checkpoint_seq(context) -> int:
    cursor = context.db.cursor()
    cursor.execute("SELECT COALESCE(MAX(seq), 0) FROM audit_checkpoints")
    seq = int(cursor.fetchone()[0])
    cursor.close()
    return seq


def _create_checkpoint_after(context, previous: int) -> int:
    name = f"External Checkpoint {time.time_ns()}"
    ContractService._create_contract_in_draft(context, name)
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        seq = _checkpoint_seq(context)
        if seq > previous:
            return seq
        time.sleep(1)
    raise AssertionError(
        f"No audit checkpoint newer than sequence {previous} was created within 90s"
    )


def _observations(context) -> list[dict]:
    return CheckpointSinkService.observations()


@given("the external checkpoint sink is reset and accepts checkpoints")
def step_sink_accepts(context):
    CheckpointSinkService.reset()
    CheckpointSinkService.set_mode("accept")
    OrceAuditControlService.reset(context, CHANNEL)


@given("the external checkpoint sink records the request and loses its response")
def step_sink_loses_response(context):
    CheckpointSinkService.reset()
    CheckpointSinkService.set_mode("lost_response")
    OrceAuditControlService.reset(context, CHANNEL)


@given(
    'the external checkpoint sink is reset and rejects the next checkpoint with "{fault}"'
)
def step_sink_rejects_chain(context, fault):
    CheckpointSinkService.reset()
    CheckpointSinkService.set_mode(_mode(fault))
    OrceAuditControlService.reset(context, CHANNEL)


@when("two new audit checkpoints are created and the ORCE anchor worker runs")
def step_two_checkpoints(context):
    start = _checkpoint_seq(context)
    first = _create_checkpoint_after(context, start)
    OrceAuditControlService.run_once(context, CHANNEL)
    second = _create_checkpoint_after(context, first)
    OrceAuditControlService.run_once(context, CHANNEL)
    context.expected_checkpoint_sequences = [start + 1, second]


@when("a new audit checkpoint is created and the ORCE anchor worker runs")
@when("another audit checkpoint is created and the ORCE anchor worker runs")
def step_one_checkpoint(context):
    before = _checkpoint_seq(context)
    created = _create_checkpoint_after(context, before)
    context.last_created_checkpoint_seq = created
    OrceAuditControlService.run_once(context, CHANNEL)


@when("the ORCE anchor worker runs")
@when("the ORCE anchor worker runs again")
def step_worker_runs(context):
    OrceAuditControlService.run_once(context, CHANNEL)


@when("the external checkpoint sink accepts checkpoints")
def step_sink_now_accepts(context):
    CheckpointSinkService.set_mode("accept")


@when("the ORCE anchor worker restarts with its persisted confirmation state")
def step_worker_restarts(context):
    namespace = os.getenv("K8S_NAMESPACE", "dcs-bdd")
    deployment = os.getenv("BDD_ORCE_DEPLOYMENT", "dcs-orce")
    kubectl = os.getenv("KUBECTL_BIN", "kubectl")
    restarted = subprocess.run(
        [kubectl, "-n", namespace, "rollout", "restart", f"deployment/{deployment}"],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert restarted.returncode == 0, (
        f"Could not restart ORCE deployment {deployment!r}: {restarted.stderr}"
    )
    ready = subprocess.run(
        [
            kubectl,
            "-n",
            namespace,
            "rollout",
            "status",
            f"deployment/{deployment}",
            "--timeout=120s",
        ],
        capture_output=True,
        text=True,
        timeout=130,
    )
    assert ready.returncode == 0, (
        f"ORCE did not become ready after restart: {ready.stderr}"
    )


@then("the checkpoint sink has observed every sequence in order without a gap")
def step_sequences_in_order(context):
    observations = _observations(context)
    sequences = [int(item["payload"]["seq"]) for item in observations]
    expected = list(
        range(context.expected_checkpoint_sequences[0], context.expected_checkpoint_sequences[-1] + 1)
    )
    assert sequences == expected, f"Expected checkpoint sequences {expected}, got {sequences}"


@then(
    "each checkpoint attempt uses the configured sink path, timeout, and authentication"
)
def step_config_applied(context):
    observations = _observations(context)
    assert observations, "The configured checkpoint sink observed no request"
    for item in observations:
        assert item.get("path") == "/checkpoints", (
            f"Checkpoint was not posted to the configured sink path: {item}"
        )
        headers = item.get("headers") or {}
        assert headers.get("authorization") == "Bearer bdd-checkpoint-sink-token", (
            f"Missing configured sink authentication: {item}"
        )
    worker_status = OrceAuditControlService.status(context, CHANNEL)
    assert float(worker_status.get("sink_timeout_seconds", 0)) > 0, (
        f"Worker does not report a positive configured sink timeout: {worker_status}"
    )


@then('the checkpoint sink receives only "{fields}"')
def step_payload_allowlist(context, fields):
    expected = {field.strip() for field in fields.split(",")}
    assert expected == PUBLIC_FIELDS
    payload = _observations(context)[-1]["payload"]
    assert set(payload) == expected, (
        f"Expected exact checkpoint allowlist {sorted(expected)}, got {sorted(payload)}"
    )


@then("the checkpoint sink receives no contract data, CID, nonce, or identity")
def step_no_private_data(context):
    payload = _observations(context)[-1]["payload"]
    assert set(payload) == PUBLIC_FIELDS, (
        f"Checkpoint payload escaped its public field allowlist: {payload}"
    )

    def assert_no_private_keys(value, path="payload"):
        if isinstance(value, dict):
            for key, child in value.items():
                normalized = str(key).strip().lower().replace("-", "_")
                assert not normalized.startswith("contract"), (
                    f"Private contract field at {path}.{key}: {payload}"
                )
                assert normalized not in {
                    "cid",
                    "nonce",
                    "identity",
                    "participant",
                    "did",
                }, f"Private field at {path}.{key}: {payload}"
                assert_no_private_keys(child, f"{path}.{key}")
        elif isinstance(value, list):
            for index, child in enumerate(value):
                assert_no_private_keys(child, f"{path}[{index}]")

    assert_no_private_keys(payload)

    hex_hash = re.compile(r"^[0-9a-fA-F]{64}$")
    assert isinstance(payload["root"], str) and hex_hash.fullmatch(payload["root"]), (
        f"Checkpoint root is not a SHA-256 hex hash: {payload['root']!r}"
    )
    assert payload["prev_root"] is None or (
        isinstance(payload["prev_root"], str)
        and hex_hash.fullmatch(payload["prev_root"])
    ), f"Checkpoint prev_root is not null or a SHA-256 hex hash: {payload['prev_root']!r}"
    assert payload["tsa_timestamp"] is None or isinstance(
        payload["tsa_timestamp"],
        str,
    ), f"TSA evidence must remain an opaque string: {payload['tsa_timestamp']!r}"

    for field in ("created_at", "timestamped_at"):
        value = payload[field]
        if value is None:
            continue
        assert isinstance(value, str), f"{field} must be an RFC3339 string: {value!r}"
        datetime.fromisoformat(value.replace("Z", "+00:00"))
        lowered = value.lower()
        assert "did:" not in lowered and not lowered.startswith(("urn:", "qm", "bafy")), (
            f"Private identifier leaked through non-opaque field {field}: {value!r}"
        )


@then("both attempts carry the same checkpoint idempotency key")
def step_same_idempotency_key(context):
    attempts = _observations(context)
    assert len(attempts) >= 2, f"Expected the lost delivery and its retry, got {attempts}"
    lost, retry = attempts[:2]
    lost_payload = lost.get("payload") or {}
    retry_payload = retry.get("payload") or {}
    assert (
        lost_payload.get("seq"),
        lost_payload.get("root"),
    ) == (
        retry_payload.get("seq"),
        retry_payload.get("root"),
    ), f"The first two attempts do not address the same checkpoint: {attempts[:2]}"
    lost_key = (lost.get("headers") or {}).get("idempotency-key")
    retry_key = (retry.get("headers") or {}).get("idempotency-key")
    assert lost_key and lost_key == retry_key, (
        f"Lost-response retry changed its idempotency key: {[lost_key, retry_key]}"
    )
    for later in attempts[2:]:
        later_payload = later.get("payload") or {}
        later_key = (later.get("headers") or {}).get("idempotency-key")
        assert (
            later_payload.get("seq"),
            later_payload.get("root"),
        ) != (
            lost_payload.get("seq"),
            lost_payload.get("root"),
        ), f"The lost checkpoint was attempted more than twice: {attempts}"
        assert later_key != lost_key, (
            f"A later checkpoint reused the lost checkpoint's idempotency key: {attempts}"
        )


@then("the external checkpoint sink contains one append for that sequence and root")
def step_single_append(context):
    status = CheckpointSinkService.status()
    records = status.get("records") or []
    matching = [
        record
        for record in records
        if int(record.get("seq", -1)) == context.last_created_checkpoint_seq
    ]
    assert len(matching) == 1, f"Expected one durable append, got {matching}"


@then(
    "the ORCE anchor worker confirms that sequence only after the successful response"
)
def step_confirmed_after_success(context):
    status = OrceAuditControlService.status(context, CHANNEL)
    assert int(status.get("confirmed_seq", 0)) >= context.last_created_checkpoint_seq
    assert status.get("last_confirmation_status") == "2xx", status


@then(
    'the ORCE anchor worker reports a blocked external chain with "{fault}"'
)
def step_chain_blocked(context, fault):
    status = OrceAuditControlService.status(context, CHANNEL)
    assert status.get("blocked") is True, status
    assert _mode(fault) in _mode(str(status.get("reason", ""))), status
    context.blocked_checkpoint_attempt_count = len(_observations(context))


@then("no checkpoint after the blocked sequence is attempted")
def step_no_later_attempt(context):
    assert len(_observations(context)) == context.blocked_checkpoint_attempt_count
