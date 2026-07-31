"""Black-box bindings for Semantic Hub-backed workflow transition gates."""

from __future__ import annotations

import copy
import re
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from threading import Barrier
from urllib.parse import parse_qs, urlparse

import requests
from psycopg2.extras import Json
from behave import given, step, then, when

from steps.contract_deployment.dcs_contract_deployment_steps import (
    _ensure_target_designated,
)
from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (
    _apply_signature,
    _run_full_ceremony,
)
from steps.support.api_client import (
    contract_approve_url,
    contract_offer_url,
    contract_retrieve_by_id_url,
    contract_submit_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.orce_audit_control_service import (
    OrceAuditControlService,
)
from steps.support.services.workflow_gate_service import WorkflowGateService
from steps.template_management.contract_state_machine_steps import (
    _advance_to_approved,
    _advance_to_reviewed,
)


CHANNEL = "workflow_gate"


def _mode(value: str) -> str:
    return value.strip().lower().replace("-", "_").replace(" ", "_")


def _state(context, name: str) -> str:
    did, _ = ContractService._contract_data(context, name)
    response = get_with_headers(
        context,
        contract_retrieve_by_id_url(context, did),
        headers=AuthService.get_headers_for_roles(["Contract Manager"]),
    )
    assert response.status_code == 200, response.text
    return str(response.json().get("state", "")).upper()


def _alias_contract_context(context, physical_name: str, logical_name: str) -> None:
    """Expose one uniquely created contract under the stable Gherkin name."""
    for store_name in (
        "contract_dids",
        "contract_updated_at",
        "contract_seed_headers",
        "ceremony_ids",
        "pid_presentations",
        "pdf_bytes",
    ):
        store = getattr(context, store_name, None)
        if isinstance(store, dict) and physical_name in store:
            store[logical_name] = store[physical_name]


def _prepare(context, name: str, gate: str) -> None:
    OrceAuditControlService.reset(context, CHANNEL)
    OrceAuditControlService.set_mode(context, CHANNEL, "success_empty")
    physical_name = f"{name} {uuid.uuid4()}"
    if gate == "submission":
        ContractService._create_contract_in_draft(context, physical_name)
    elif gate == "offer":
        ContractService._create_contract_in_draft(context, physical_name)
    elif gate == "approval":
        ContractService._create_contract_in_draft(context, physical_name)
        _advance_to_reviewed(context, physical_name)
    elif gate == "signature":
        ContractService._create_contract_in_draft(context, physical_name)
        _advance_to_approved(context, physical_name)
        _run_full_ceremony(
            context,
            physical_name,
            ContractService._local_peer_did(context),
            "BDD Workflow Gate Signer",
        )
    elif gate == "deployment":
        ContractService._create_contract_in_draft(context, physical_name)
        _advance_to_approved(context, physical_name)
        _ensure_target_designated(context, physical_name)
        ContractService._refresh_contract(context, physical_name)
        _run_full_ceremony(
            context,
            physical_name,
            ContractService._local_peer_did(context),
            "BDD Workflow Gate Signer",
        )
    else:
        raise AssertionError(f"Unknown workflow gate {gate!r}")
    ContractService._refresh_contract(context, physical_name)
    pre_state = _state(context, physical_name)
    _alias_contract_context(context, physical_name, name)
    context.workflow_pre_states = getattr(context, "workflow_pre_states", {})
    context.workflow_pre_states[name] = pre_state


def _request_gate(context, gate: str, name: str):
    did, updated_at = ContractService._contract_data(context, name)
    context.last_workflow_gate_request_token = updated_at
    if gate == "submission":
        response = post_json(
            context,
            contract_submit_url(context),
            ContractService._contract_submit_payload(context, did, updated_at),
            headers=AuthService.get_headers_for_roles(["Contract Creator"]),
        )
    elif gate == "offer":
        response = post_json(
            context,
            contract_offer_url(context),
            {"did": did, "updated_at": updated_at},
            headers=AuthService.get_headers_for_roles(["Contract Creator"]),
        )
    elif gate == "approval":
        response = post_json(
            context,
            contract_approve_url(context),
            {"did": did, "updated_at": updated_at},
            headers=AuthService.get_headers_for_roles(["Contract Approver"]),
        )
    elif gate == "signature":
        presentation = context.pid_presentations[name]
        response = _apply_signature(
            context,
            name,
            signer_did=presentation["subject_did"],
            ceremony_id=context.ceremony_ids[name],
            signatory="BDD Workflow Gate Signer",
        )
    elif gate == "deployment":
        review_deployment = getattr(context, "workflow_gate_mode", None) == "review"
        if review_deployment:
            # Signature is its own synchronous gate. Let that gate succeed,
            # then switch the executor before the asynchronous auto-deploy
            # subscriber evaluates the deployment gate.
            OrceAuditControlService.set_mode(context, CHANNEL, "success_empty")
        presentation = context.pid_presentations[name]
        response = _apply_signature(
            context,
            name,
            signer_did=presentation["subject_did"],
            ceremony_id=context.ceremony_ids[name],
            signatory="BDD Workflow Gate Signer",
        )
        if review_deployment:
            ContractService._refresh_contract(context, name)
            context.workflow_pre_states[name] = "SIGNED"
            OrceAuditControlService.set_mode(context, CHANNEL, "review")
    else:
        raise AssertionError(f"Unknown workflow gate {gate!r}")
    context.requests_response = response
    if response.status_code == 200:
        ContractService._refresh_contract(context, name)
    body = {}
    try:
        body = response.json()
    except ValueError:
        pass
    context.last_workflow_gate_run_id = (
        body.get("gate_run_id")
        or body.get("run_id")
        or response.headers.get("X-Workflow-Gate-Run-ID")
    )
    return response


def _restore_injected_hub_fault(context) -> None:
    contract_restore = getattr(context, "workflow_fault_contract_restore", None)
    hub_rows = getattr(context, "workflow_fault_hub_rows", None) or []
    if not contract_restore and not hub_rows:
        return
    cursor = context.db.cursor()
    try:
        if contract_restore:
            did, contract_data = contract_restore
            cursor.execute(
                "UPDATE contracts SET contract_data=%s WHERE did=%s",
                (Json(contract_data), did),
            )
        for row in hub_rows:
            cursor.execute(
                "INSERT INTO semantic_schemas "
                "(name, kind, version, media_type, content, active, created_by, created_at) "
                "VALUES (%s,%s,%s,%s,%s,%s,%s,%s)",
                row,
            )
        context.db.commit()
    except Exception:
        context.db.rollback()
        raise
    finally:
        cursor.close()
        context.workflow_fault_contract_restore = None
        context.workflow_fault_hub_rows = []


def _observations(context):
    return OrceAuditControlService.observations(context, CHANNEL)


def _anchored_asset_name(snapshot: dict, term: str, route: str) -> str:
    reference = snapshot.get(term)
    if isinstance(reference, dict):
        reference = reference.get("@id")
    assert isinstance(reference, str) and reference, (
        f"Workflow snapshot has no {term} anchor: {snapshot}"
    )
    marker = f"/semantic/{route}/"
    path = urlparse(reference).path
    assert marker in path, f"Unexpected {term} anchor {reference!r}"
    name = path.split(marker, 1)[1].strip("/")
    assert name, f"Could not derive an asset name from {reference!r}"
    return name


def _semantic_asset_ref(value, term: str, route: str) -> tuple[str, int, str]:
    if isinstance(value, dict):
        value = value.get("@id")
    assert isinstance(value, str) and value, f"Missing {term} semantic pin: {value!r}"
    parsed = urlparse(value)
    marker = f"/semantic/{route}/"
    assert marker in parsed.path, f"Unexpected {term} semantic pin: {value!r}"
    name = parsed.path.split(marker, 1)[1].strip("/")
    versions = parse_qs(parsed.query).get("version") or []
    assert name and len(versions) == 1 and versions[0].isdigit(), (
        f"Semantic pin has no exact asset name/version: {value!r}"
    )
    return name, int(versions[0]), value


def _semantic_bundle_pins(document: dict) -> dict:
    canonical = _semantic_asset_ref(
        document.get("sh:shapesGraph"),
        "sh:shapesGraph",
        "shapes",
    )
    effective = document.get("dcs:effectiveShapes")
    assert isinstance(effective, list) and effective, (
        f"Document has no immutable dcs:effectiveShapes bundle: {document}"
    )
    shapes = tuple(
        sorted(
            _semantic_asset_ref(value, "dcs:effectiveShapes", "shapes")
            for value in effective
        )
    )
    assert canonical in shapes, (
        f"Canonical shapes pin {canonical} is absent from effective bundle {shapes}"
    )
    profile = _semantic_asset_ref(
        document.get("dcterms:conformsTo"),
        "dcterms:conformsTo",
        "profile",
    )
    return {"canonical": canonical, "shapes": shapes, "profile": profile}


def _assert_semantic_pins_exist(context, pins: dict) -> None:
    assets = {
        (name, "shapes", version)
        for name, version, _url in pins["shapes"]
    }
    assets.add((pins["profile"][0], "profile", pins["profile"][1]))
    cursor = context.db.cursor()
    for name, kind, version in sorted(assets):
        cursor.execute(
            "SELECT COUNT(*) FROM semantic_schemas "
            "WHERE name=%s AND kind=%s AND version=%s",
            (name, kind, version),
        )
        assert cursor.fetchone()[0] == 1, (
            f"Pinned Semantic Hub asset does not resolve: "
            f"{name!r}/{kind}/version={version}"
        )
    cursor.close()


@given('contract "{name}" is ready for the "{gate}" gate')
def step_contract_ready(context, name, gate):
    _prepare(context, name, gate)


@given("the workflow-gate executor is reset and returns a valid empty success")
def step_executor_empty(context):
    OrceAuditControlService.reset(context, CHANNEL)
    OrceAuditControlService.set_mode(context, CHANNEL, "success_empty")
    context.workflow_gate_mode = "success_empty"


@given("the workflow-gate executor is reset and returns a valid FAILED result")
def step_executor_failed(context):
    OrceAuditControlService.reset(context, CHANNEL)
    OrceAuditControlService.set_mode(context, CHANNEL, "failed")
    context.workflow_gate_mode = "failed"


@given("the workflow-gate executor is reset and returns a valid REVIEW result")
def step_executor_review(context):
    OrceAuditControlService.reset(context, CHANNEL)
    OrceAuditControlService.set_mode(context, CHANNEL, "review")
    context.workflow_gate_mode = "review"


@given('the workflow-gate executor is reset and returns "{fault}"')
def step_executor_fault(context, fault):
    OrceAuditControlService.reset(context, CHANNEL)
    mode = _mode(fault)
    OrceAuditControlService.set_mode(context, CHANNEL, mode)
    context.workflow_gate_mode = mode


@given("the workflow-gate executor observations are reset")
def step_executor_reset(context):
    OrceAuditControlService.reset(context, CHANNEL)


@when('the "{gate}" gate is requested for contract "{name}"')
def step_request_gate(context, gate, name):
    try:
        _request_gate(context, gate, name)
    finally:
        _restore_injected_hub_fault(context)


@when(
    'two concurrent clients request the "{gate}" gate with the same snapshot token for contract "{name}"'
)
def step_concurrent_same_snapshot_claims(context, gate, name):
    assert gate == "submission", (
        f"Concurrent same-snapshot fixture is only defined for submission, got {gate!r}"
    )
    did, updated_at = ContractService._contract_data(context, name)
    payload = ContractService._contract_submit_payload(context, did, updated_at)
    headers = AuthService.get_headers_for_roles(["Contract Creator"])
    barrier = Barrier(3)

    def submit():
        barrier.wait(timeout=5)
        return requests.post(
            contract_submit_url(context),
            json=payload,
            headers=headers,
            timeout=context.http_timeout_seconds,
        )

    responses = []
    try:
        with ThreadPoolExecutor(max_workers=2) as pool:
            futures = [pool.submit(submit) for _ in range(2)]
            barrier.wait(timeout=5)
            responses = [
                future.result(timeout=context.http_timeout_seconds + 10)
                for future in futures
            ]
    finally:
        OrceAuditControlService.set_mode(context, CHANNEL, "success_empty")
    context.concurrent_gate_responses = responses
    context.requests_response = responses[0]


@then("both concurrent requests fail closed with the same workflow-gate run ID")
def step_concurrent_claims_share_failed_run(context):
    responses = context.concurrent_gate_responses
    assert len(responses) == 2
    run_ids = []
    for response in responses:
        assert response.status_code in (409, 422, 502, 503, 504), (
            f"Concurrent workflow-gate claim did not fail closed: "
            f"{response.status_code} {response.text}"
        )
        body = response.json()
        assert body.get("name") == "workflow_gate_blocked", body
        run_id = body.get("gate_run_id")
        assert run_id, f"Blocked concurrent claim exposes no gate_run_id: {body}"
        run_ids.append(run_id)
    assert run_ids[0] == run_ids[1], (
        f"Concurrent claims did not converge on one run: {run_ids}"
    )
    context.concurrent_gate_run_id = run_ids[0]


@then('the "{gate}" transition succeeds for contract "{name}"')
def step_transition_succeeds(context, gate, name):
    expected = {
        "submission": {"NEGOTIATION"},
        "offer": {"OFFERED"},
        "approval": {"APPROVED"},
        "signature": {"SIGNED", "ACTIVE"},
        "deployment": {"ACTIVE"},
    }[gate]
    assert context.requests_response.status_code == 200, (
        f"{gate} gate request with updated_at token "
        f"{getattr(context, 'last_workflow_gate_request_token', None)!r} failed: "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )
    deadline = time.monotonic() + (30 if gate == "deployment" else 0)
    actual = _state(context, name)
    while actual not in expected and time.monotonic() < deadline:
        time.sleep(0.5)
        actual = _state(context, name)
    assert actual in expected, (
        f"Expected {gate} transition to reach {sorted(expected)}, got {actual!r}; "
        f"gate_run_id={getattr(context, 'last_workflow_gate_run_id', None)!r}; "
        f"last_response={context.requests_response.status_code} "
        f"{context.requests_response.text[:500]}"
    )


@then('one correlated "{gate}" workflow-gate request was observed')
def step_one_correlated_request(context, gate):
    deadline = time.monotonic() + 15
    observations = []
    while True:
        observations = [
            item
            for item in _observations(context)
            if item.get("payload", {}).get("gate") == gate
        ]
        if len(observations) == 1:
            break
        assert len(observations) < 2, (
            f"Expected at most one {gate} dispatch, got {observations}"
        )
        if time.monotonic() >= deadline:
            break
        time.sleep(0.5)
    assert len(observations) == 1, f"Expected one {gate} dispatch, got {observations}"
    payload = observations[0]["payload"]
    assert payload.get("correlation_id") and payload.get("snapshot_id"), payload
    context.last_gate_observation = observations[0]
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT run_id::text FROM pac_workflow_gate_runs WHERE correlation_id=%s",
        (payload["correlation_id"],),
    )
    persisted = cursor.fetchone()
    cursor.close()
    assert persisted, (
        f"No persisted workflow-gate run for correlation {payload['correlation_id']!r}"
    )
    context.last_workflow_gate_run_id = persisted[0]


@then(
    "exactly one successful empty workflow-gate run is persisted for that snapshot, gate, and correlation"
)
def step_one_empty_persisted_run(context):
    payload = context.last_gate_observation["payload"]
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT correlation_id::text, status, response_json "
        "FROM pac_workflow_gate_runs WHERE snapshot_id=%s AND gate=%s",
        (payload["snapshot_id"], payload["gate"]),
    )
    rows = cursor.fetchall()
    cursor.close()
    assert len(rows) == 1, (
        f"Expected exactly one persisted run for snapshot {payload['snapshot_id']!r} "
        f"and gate {payload['gate']!r}, got {rows}"
    )
    correlation_id, status, response = rows[0]
    assert correlation_id == payload["correlation_id"], (
        f"Persisted correlation {correlation_id!r} does not match dispatched "
        f"{payload['correlation_id']!r}"
    )
    assert status == "SUCCESS", f"Expected successful persisted gate run, got {status!r}"
    assert isinstance(response, dict) and response.get("findings") == [], (
        f"Expected the exact valid empty executor result to be persisted, got {response!r}"
    )


@then(
    "exactly one PostgreSQL workflow-gate run exists for that snapshot, gate, correlation, and run ID"
)
def step_one_postgres_run_for_concurrent_claims(context):
    payload = context.last_gate_observation["payload"]
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT run_id::text, correlation_id::text, snapshot_id, gate, status "
        "FROM pac_workflow_gate_runs WHERE snapshot_id=%s AND gate=%s",
        (payload["snapshot_id"], payload["gate"]),
    )
    rows = cursor.fetchall()
    cursor.close()
    assert len(rows) == 1, (
        f"Concurrent claims created {len(rows)} PostgreSQL runs: {rows}"
    )
    run_id, correlation_id, snapshot_id, gate, status = rows[0]
    assert run_id == context.concurrent_gate_run_id, (
        f"HTTP run ID {context.concurrent_gate_run_id!r} != PostgreSQL {run_id!r}"
    )
    assert correlation_id == payload["correlation_id"]
    assert snapshot_id == payload["snapshot_id"]
    assert gate == payload["gate"]
    assert status == "BLOCKED", (
        f"Timed-out concurrent workflow-gate run did not fail closed: {rows[0]}"
    )


@then("that request carries the local Semantic Hub evaluation and immutable snapshot")
def step_request_has_local_evaluation(context):
    payload = context.last_gate_observation["payload"]
    local = payload.get("local_evaluation")
    snapshot = payload.get("snapshot")
    assert isinstance(local, dict) and local.get("result"), payload
    assert isinstance(snapshot, dict) and snapshot.get("content_hash"), payload
    assert snapshot.get("effective_shapes") and snapshot.get("profile_version"), payload


@then("the workflow transition is blocked by the failed gate")
@then("the workflow transition is blocked by the workflow-gate executor")
def step_gate_blocked(context):
    assert context.requests_response.status_code in (409, 422, 502, 503, 504), (
        context.requests_response.text
    )


@then('contract "{name}" remains in its pre-gate state')
def step_state_unchanged(context, name):
    assert _state(context, name) == context.workflow_pre_states[name]


@then("a pending PACM manual review is persisted for the correlated gate run")
def step_review_persisted(context):
    assert context.last_workflow_gate_run_id, (
        "Blocked REVIEW response did not expose its gate run ID"
    )
    response = WorkflowGateService.read_run(context, context.last_workflow_gate_run_id)
    assert response.status_code == 200, response.text
    body = response.json()
    assert body.get("status") == "REVIEW", body
    assert body.get("review", {}).get("status") == "PENDING", body
    context.pending_review_run = body


@when(
    'the Compliance Officer approves that gate run with justification "{justification}"'
)
def step_approve_review(context, justification):
    context.review_justification = justification
    context.requests_response = WorkflowGateService.decide_review(
        context,
        run_id=context.last_workflow_gate_run_id,
        decision="approve",
        justification=justification,
        roles=["Compliance Officer"],
    )


@then("the PACM manual review records the decision and justification")
def step_review_decision_recorded(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    body = context.requests_response.json()
    assert body.get("decision") == "approve", body
    assert body.get("justification") == context.review_justification, body


@then(
    "the approved PACM review is append-only and records one successful continuation attempt"
)
def step_review_is_append_only_with_successful_continuation(context):
    run_id = context.last_workflow_gate_run_id
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT decision_id::text, decision, justification, decided_by, decided_at "
        "FROM pac_workflow_gate_review_decisions WHERE run_id=%s "
        "ORDER BY decided_at",
        (run_id,),
    )
    decisions_before = cursor.fetchall()
    cursor.execute(
        "SELECT status, failure_reason, completed_at "
        "FROM pac_workflow_gate_continuation_attempts WHERE run_id=%s "
        "ORDER BY started_at",
        (run_id,),
    )
    attempts = cursor.fetchall()
    cursor.close()
    assert len(decisions_before) == 1, (
        f"Expected one append-only review decision for run {run_id}, got {decisions_before}"
    )
    assert decisions_before[0][1] == "approve"
    assert decisions_before[0][2] == context.review_justification
    assert len(attempts) == 1, (
        f"Expected one continuation attempt for run {run_id}, got {attempts}"
    )
    assert attempts[0][0] == "SUCCESS" and attempts[0][1] is None
    assert attempts[0][2] is not None, (
        f"Successful continuation attempt has no completion timestamp: {attempts[0]}"
    )

    rejected_rewrite = WorkflowGateService.decide_review(
        context,
        run_id=run_id,
        decision="reject",
        justification="attempt to replace the persisted approval",
        roles=["Compliance Officer"],
    )
    assert rejected_rewrite.status_code in (400, 409, 422, 500), (
        f"A completed review accepted a replacement decision: "
        f"{rejected_rewrite.status_code} {rejected_rewrite.text}"
    )
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT decision_id::text, decision, justification, decided_by, decided_at "
        "FROM pac_workflow_gate_review_decisions WHERE run_id=%s "
        "ORDER BY decided_at",
        (run_id,),
    )
    decisions_after = cursor.fetchall()
    cursor.close()
    assert decisions_after == decisions_before, (
        f"Persisted review decision was not append-only: "
        f"before={decisions_before}, after={decisions_after}"
    )


@then('the workflow-gate executor still has exactly one correlated "{gate}" request')
def step_executor_still_has_one_request(context, gate):
    time.sleep(1)
    observations = [
        item
        for item in _observations(context)
        if item.get("payload", {}).get("gate") == gate
    ]
    assert len(observations) == 1, (
        f"Review continuation redispatched the {gate} executor request: {observations}"
    )
    assert observations[0]["payload"].get("correlation_id") == (
        context.last_gate_observation["payload"].get("correlation_id")
    )


@then("no successful workflow-gate result is persisted")
def step_no_success_result(context):
    run_id = context.last_workflow_gate_run_id
    if not run_id:
        return
    response = WorkflowGateService.read_run(context, run_id)
    assert response.status_code == 200, response.text
    assert response.json().get("status") != "SUCCESS", response.text


@given(
    'an immutable workflow snapshot "{alias}" is created from the active shapes, libraries, and profile'
)
@when(
    'an immutable workflow snapshot "{alias}" is created from the active shapes, libraries, and profile'
)
def step_create_snapshot(context, alias):
    name = f"Effective Bundle Snapshot {alias} {uuid.uuid4()}"
    ContractService._create_contract_in_draft(context, name)
    context.workflow_snapshot_contracts = getattr(
        context, "workflow_snapshot_contracts", {}
    )
    context.workflow_snapshot_contracts[alias] = name
    context.workflow_snapshot_bundles = getattr(context, "workflow_snapshot_bundles", {})
    document = copy.deepcopy(
        ContractService._refresh_contract(context, name).get("contract_data") or {}
    )
    context.workflow_snapshot_bundles[alias] = document
    context.workflow_snapshot_pins = getattr(context, "workflow_snapshot_pins", {})
    context.workflow_snapshot_pins[alias] = _semantic_bundle_pins(document)


@when("the Template Manager activates new effective shapes, libraries, and profile versions")
def step_activate_effective_bundle(context):
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT name, kind, version FROM semantic_schemas "
        "WHERE active AND kind IN ('shapes','profile') "
        "ORDER BY kind, name, version"
    )
    context.previous_active_bundle = {
        (str(name), str(kind), int(version))
        for name, kind, version in cursor.fetchall()
    }
    old_snapshot = context.workflow_snapshot_bundles["old"]
    old_pins = context.workflow_snapshot_pins["old"]
    old_shape_assets = {
        (name, "shapes", version)
        for name, version, _url in old_pins["shapes"]
    }
    assert old_shape_assets == {
        asset for asset in context.previous_active_bundle if asset[1] == "shapes"
    }, (
        f"Old contract did not pin the complete active shapes/library bundle: "
        f"pins={old_shape_assets}, active={context.previous_active_bundle}"
    )
    assert (
        old_pins["profile"][0],
        "profile",
        old_pins["profile"][1],
    ) in context.previous_active_bundle
    targets = (
        (_anchored_asset_name(old_snapshot, "sh:shapesGraph", "shapes"), "shapes"),
        (
            _anchored_asset_name(
                old_snapshot,
                "dcterms:conformsTo",
                "profile",
            ),
            "profile",
        ),
    )
    context.activated_target_versions = {}
    for name, kind in targets:
        cursor.execute(
            "SELECT version, media_type, content, created_by FROM semantic_schemas "
            "WHERE name=%s AND kind=%s AND active",
            (name, kind),
        )
        active = cursor.fetchone()
        assert active, f"No active Semantic Hub {kind} asset named {name!r}"
        version, media_type, content, created_by = active
        cursor.execute(
            "SELECT COALESCE(MAX(version), 0) + 1 FROM semantic_schemas "
            "WHERE name=%s AND kind=%s",
            (name, kind),
        )
        next_version = int(cursor.fetchone()[0])
        cursor.execute(
            "UPDATE semantic_schemas SET active=FALSE WHERE name=%s AND kind=%s",
            (name, kind),
        )
        cursor.execute(
            "INSERT INTO semantic_schemas "
            "(name, version, kind, media_type, content, active, created_by) "
            "VALUES (%s,%s,%s,%s,%s,TRUE,%s)",
            (name, next_version, kind, media_type, content, created_by),
        )
        context.activated_target_versions[(name, kind)] = next_version
    cursor.execute(
        "SELECT COALESCE(MAX(version), 0) + 1 FROM semantic_schemas "
        "WHERE name='bdd-effective-library' AND kind='shapes'"
    )
    next_library_version = int(cursor.fetchone()[0])
    cursor.execute(
        "UPDATE semantic_schemas SET active=FALSE "
        "WHERE name='bdd-effective-library' AND kind='shapes'"
    )
    cursor.execute(
        "INSERT INTO semantic_schemas "
        "(name, version, kind, media_type, content, active, created_by) "
        "VALUES ('bdd-effective-library',%s,'shapes','text/turtle',"
        "'@prefix sh: <http://www.w3.org/ns/shacl#> .',TRUE,'bdd')",
        (next_library_version,),
    )
    context.activated_target_versions[
        ("bdd-effective-library", "shapes")
    ] = next_library_version
    context.db.commit()
    cursor.execute(
        "SELECT name, kind, version FROM semantic_schemas "
        "WHERE active AND kind IN ('shapes','profile') "
        "ORDER BY kind, name, version"
    )
    context.activated_active_bundle = {
        (str(name), str(kind), int(version))
        for name, kind, version in cursor.fetchall()
    }
    cursor.close()


@when("the Template Manager rolls the effective Semantic Hub bundle back")
def step_rollback_effective_bundle(context):
    cursor = context.db.cursor()
    cursor.execute(
        "UPDATE semantic_schemas SET active=FALSE "
        "WHERE kind IN ('shapes','profile')"
    )
    for name, kind, version in sorted(context.previous_active_bundle):
        cursor.execute(
            "UPDATE semantic_schemas SET active=TRUE "
            "WHERE name=%s AND kind=%s AND version=%s",
            (name, kind, version),
        )
    context.db.commit()
    cursor.close()
    step_create_snapshot(context, "rollback")


@then(
    'workflow snapshot "{alias}" still resolves its original effective bundle'
)
def step_old_bundle_resolves(context, alias):
    pins = _semantic_bundle_pins(context.workflow_snapshot_bundles[alias])
    assert pins == context.workflow_snapshot_pins[alias], (
        f"Existing contract pins changed after activation/rollback: "
        f"created={context.workflow_snapshot_pins[alias]}, current={pins}"
    )
    _assert_semantic_pins_exist(context, pins)


@then(
    'workflow snapshot "{alias}" still resolves the newly activated effective bundle'
)
def step_new_bundle_resolves(context, alias):
    pins = _semantic_bundle_pins(context.workflow_snapshot_bundles[alias])
    expected_shapes = {
        (name, version)
        for name, kind, version in context.activated_active_bundle
        if kind == "shapes"
    }
    actual_shapes = {
        (name, version) for name, version, _url in pins["shapes"]
    }
    assert actual_shapes == expected_shapes, (
        f"New contract did not pin the activated effective shapes/libraries: "
        f"expected={expected_shapes}, actual={actual_shapes}"
    )
    canonical_name = context.workflow_snapshot_pins["old"]["canonical"][0]
    assert pins["canonical"][:2] == (
        canonical_name,
        context.activated_target_versions[(canonical_name, "shapes")],
    )
    profile_name = context.workflow_snapshot_pins["old"]["profile"][0]
    assert pins["profile"][:2] == (
        profile_name,
        context.activated_target_versions[(profile_name, "profile")],
    )
    assert (
        "bdd-effective-library",
        context.activated_target_versions[
            ("bdd-effective-library", "shapes")
        ],
    ) in actual_shapes
    assert context.workflow_snapshot_pins["old"] != pins, (
        "New contract unexpectedly retained the complete old semantic bundle"
    )
    _assert_semantic_pins_exist(context, pins)


@then(
    "a workflow snapshot created after rollback deterministically resolves the restored bundle"
)
def step_rollback_bundle_resolves(context):
    old = context.workflow_snapshot_pins["old"]
    rollback = _semantic_bundle_pins(
        context.workflow_snapshot_bundles["rollback"]
    )
    assert rollback == old, (
        f"Rollback contract did not restore the exact previous semantic anchors: "
        f"old={old}, rollback={rollback}"
    )
    restored_active = {
        (name, "shapes", version)
        for name, version, _url in rollback["shapes"]
    }
    restored_active.add(
        (rollback["profile"][0], "profile", rollback["profile"][1])
    )
    assert restored_active == context.previous_active_bundle, (
        f"Rollback pins do not equal the exact pre-activation active bundle: "
        f"pins={restored_active}, expected={context.previous_active_bundle}"
    )
    _assert_semantic_pins_exist(context, rollback)


@given('its immutable workflow snapshot has a "{fault}" Semantic Hub asset')
def step_faulty_hub_asset(context, fault):
    name = next(reversed(context.workflow_pre_states))
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute("SELECT contract_data FROM contracts WHERE did=%s", (did,))
    data = cursor.fetchone()[0]
    context.workflow_fault_contract_restore = (did, copy.deepcopy(data))
    context.workflow_fault_hub_rows = []
    if fault == "missing":
        data.pop("sh:shapesGraph", None)
        data.pop("dcterms:conformsTo", None)
        assert "sh:shapesGraph" not in data and "dcterms:conformsTo" not in data
    elif fault == "unknown":
        ref = data.get("sh:shapesGraph")
        if isinstance(ref, dict):
            original = ref.get("@id", "")
            ref["@id"] = re.sub(r"version=\d+", "version=999999", original)
            changed = ref["@id"]
        elif isinstance(ref, str):
            original = ref
            data["sh:shapesGraph"] = re.sub(
                r"version=\d+",
                "version=999999",
                ref,
            )
            changed = data["sh:shapesGraph"]
        else:
            raise AssertionError(f"No shapes pin available to make unknown: {data}")
        assert changed != original and "version=999999" in changed, (
            f"Unknown-version fault did not alter the shapes pin: {original!r}"
        )
        shape_name = _anchored_asset_name(data, "sh:shapesGraph", "shapes")
        cursor.execute(
            "SELECT COUNT(*) FROM semantic_schemas "
            "WHERE name=%s AND kind='shapes' AND version=999999",
            (shape_name,),
        )
        assert cursor.fetchone()[0] == 0, (
            f"Version 999999 unexpectedly exists for shapes asset {shape_name!r}"
        )
    elif fault == "unavailable":
        ref = data.get("sh:shapesGraph")
        if isinstance(ref, dict):
            ref_object = ref
            ref = ref.get("@id")
        else:
            ref_object = None
        assert isinstance(ref, str) and ref, f"No pinned shapes asset in {data}"
        parsed = urlparse(ref)
        versions = parse_qs(parsed.query).get("version") or []
        assert len(versions) == 1 and versions[0].isdigit(), (
            f"Pinned shapes URL carries no exact version: {ref!r}"
        )
        shape_name = _anchored_asset_name(data, "sh:shapesGraph", "shapes")
        shape_version = int(versions[0])
        cursor.execute(
            "SELECT name, kind, version, media_type, content, active, created_by, created_at "
            "FROM semantic_schemas WHERE name=%s AND kind='shapes' AND version=%s",
            (shape_name, shape_version),
        )
        pinned_row = cursor.fetchone()
        assert pinned_row, (
            f"Pinned shapes asset {shape_name!r} version {shape_version} does not exist"
        )
        cursor.execute(
            "SELECT COALESCE(MAX(version), 0) + 1 FROM semantic_schemas "
            "WHERE name=%s AND kind='shapes'",
            (shape_name,),
        )
        unavailable_version = int(cursor.fetchone()[0])
        (
            _name,
            _kind,
            _version,
            media_type,
            content,
            _active,
            created_by,
            _created_at,
        ) = pinned_row
        cursor.execute(
            "INSERT INTO semantic_schemas "
            "(name, version, kind, media_type, content, active, created_by) "
            "VALUES (%s,%s,'shapes',%s,%s,FALSE,%s)",
            (
                shape_name,
                unavailable_version,
                media_type,
                content,
                created_by,
            ),
        )
        unavailable_ref = re.sub(
            r"version=\d+",
            f"version={unavailable_version}",
            ref,
        )
        assert unavailable_ref != ref, (
            f"Unavailable-version fault did not alter the shapes pin: {ref!r}"
        )
        if ref_object is not None:
            ref_object["@id"] = unavailable_ref
        else:
            data["sh:shapesGraph"] = unavailable_ref
        cursor.execute(
            "DELETE FROM semantic_schemas "
            "WHERE name=%s AND kind='shapes' AND version=%s",
            (shape_name, unavailable_version),
        )
        assert cursor.rowcount == 1
        cursor.execute(
            "SELECT COUNT(*) FROM semantic_schemas "
            "WHERE name=%s AND kind='shapes' AND version=%s",
            (shape_name, unavailable_version),
        )
        assert cursor.fetchone()[0] == 0, (
            "Freshly pinned unavailable shapes version was not removed"
        )
    else:
        raise AssertionError(f"Unknown Semantic Hub fault {fault!r}")
    cursor.execute(
        "UPDATE contracts SET contract_data=%s WHERE did=%s",
        (Json(data), did),
    )
    context.db.commit()
    cursor.close()


@then("the workflow transition is blocked by Semantic Hub resolution")
def step_hub_resolution_blocked(context):
    assert context.requests_response.status_code in (409, 422, 500, 503), (
        context.requests_response.text
    )


@then("the workflow-gate executor has observed no request")
def step_no_executor_request(context):
    assert _observations(context) == []


@then("no fallback shapes, library, or profile version is recorded")
def step_no_fallback(context):
    run_id = context.last_workflow_gate_run_id
    if not run_id:
        return
    response = WorkflowGateService.read_run(context, run_id)
    assert response.status_code == 200, response.text
    body = response.json()
    assert not body.get("fallback"), body
