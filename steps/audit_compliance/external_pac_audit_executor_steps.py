"""Executable contract tests for the external PAC audit executor.

The control/observation endpoints steer and inspect the real bundled ORCE flow;
they do not replace it with an in-process HTTP mock.
"""

from __future__ import annotations

import csv
import json
from datetime import datetime
from urllib.parse import urlencode

import hashlib
import io
import os
import time
import uuid

import requests
from behave import given, then, when

from steps.support.api_client import get_with_headers, origin_url, pac_audit_url, pac_report_url
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


CONTRACT_VERSION = "facis-pac-audit-executor/v1"
REFERENCE_EXECUTOR = "facis-orce-reference"


def _control_url(context) -> str:
    configured = os.getenv("BDD_ORCE_AUDIT_CONTROL_URL", "").strip()
    return configured.rstrip("/") if configured else f"{origin_url(context.base_url)}/orce/audit-executor/test"


def _executor_url(context) -> str:
    configured = os.getenv("BDD_ORCE_AUDIT_EXECUTOR_URL", "").strip()
    return configured or f"{origin_url(context.base_url)}/orce/audit/run"


def _control(context, path: str, payload: dict | None = None):
    url = f"{_control_url(context)}{path}"
    if payload is None:
        response = requests.get(url, timeout=context.http_timeout_seconds)
    else:
        response = requests.post(url, json=payload, timeout=context.http_timeout_seconds)
    assert response.status_code in (200, 204), (
        f"ORCE audit control endpoint {url} failed: {response.status_code} {response.text}"
    )
    return response


def _reset(context) -> None:
    _control(context, "/reset", {})


def _set_mode(context, mode: str) -> None:
    _control(context, "/mode", {"mode": mode})


def _observations(context) -> list[dict]:
    response = _control(context, "/requests")
    body = response.json()
    observations = body.get("requests") if isinstance(body, dict) else body
    assert isinstance(observations, list), f"Expected ORCE request observations, got: {body!r}"
    return observations


def _contract_did(context, name: str) -> str:
    return ContractService._contract_data(context, name)[0]


def _resource_did(context, scope: str, name: str) -> str:
    if scope == "templates":
        contract = ContractService._refresh_contract(context, name)
        did = contract.get("template_did") or contract.get("templateDid")
        assert did, f"Contract {name!r} has no source template DID: {contract!r}"
        return did
    return _contract_did(context, name)


def _audit(context, scope: str, justification: str, did: str | None = None, headers: dict | None = None):
    payload = {"scope": scope, "justification": justification}
    if did:
        payload["did"] = did
    context.last_external_audit_request = payload
    context.requests_response = requests.post(
        pac_audit_url(context),
        json=payload,
        headers=headers if headers is not None else AuthService.get_headers_for_roles(["Auditor"]),
        timeout=context.http_timeout_seconds,
    )


def _json(response) -> dict:
    body = response.json()
    assert isinstance(body, dict), f"Expected an executor-backed audit result object, got: {body!r}"
    return body


@given("the reference ORCE audit executor is reachable and its observations are reset")
def step_reference_executor_reset(context):
    _reset(context)
    _set_mode(context, "success")
    context.orce_direct_request = None


@given("the ORCE audit executor observations are reset")
def step_executor_observations_reset(context):
    _reset(context)


@given('the reference ORCE audit executor is reachable and returns "{mode}"')
def step_reference_executor_mode(context, mode):
    _reset(context)
    _set_mode(context, mode.replace(" ", "_").replace("-", "_"))


@given("the reference ORCE audit executor is reachable and returns a valid result with a receipt")
def step_reference_executor_receipt(context):
    _reset(context)
    _set_mode(context, "success_with_receipt")


@given("the reference ORCE audit executor is reachable and returns a valid empty result")
def step_reference_executor_empty(context):
    _reset(context)
    _set_mode(context, "success_empty")


@when('the "{request}" PAC audit request is submitted')
def step_invalid_request(context, request):
    if request == "unauthenticated contracts scope":
        _audit(context, "contracts", "pre-dispatch auth BDD", headers={})
    elif request == "authenticated unsupported scope":
        _audit(context, "unsupported", "pre-dispatch validation BDD")
    else:
        raise AssertionError(f"Unknown request fixture: {request!r}")


@then("the PAC audit request is rejected before execution")
def step_rejected(context):
    assert context.requests_response.status_code in (400, 401, 403), (
        f"Expected pre-dispatch rejection, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("the ORCE audit executor has observed no request")
def step_no_observation(context):
    assert _observations(context) == [], "A rejected PAC request reached the external executor"


@when("a valid versioned audit request is posted directly to the bundled ORCE reference flow")
def step_direct_reference_request(context):
    audit_id = str(uuid.uuid4())
    request = {
        "contract_version": CONTRACT_VERSION,
        "audit_id": audit_id,
        "correlation_id": audit_id,
        "scope": "contracts",
        "requester": {"subject": "bdd-direct", "roles": ["Auditor"]},
        "justification": "reference flow contract BDD",
        "evidence": {"contracts": []},
    }
    context.orce_direct_request = request
    context.requests_response = requests.post(
        _executor_url(context), json=request, timeout=context.http_timeout_seconds
    )


@then("the ORCE response satisfies the versioned audit executor contract")
def step_reference_contract(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    body = _json(context.requests_response)
    request = context.orce_direct_request
    assert body.get("contract_version") == request["contract_version"], body
    assert body.get("audit_id") == request["audit_id"], body
    assert body.get("correlation_id") == request["correlation_id"], body
    assert body.get("scope") == request["scope"], body
    executor = body.get("executor") or {}
    assert executor.get("id") and executor.get("version"), body
    assert body.get("executed_at"), body
    assert isinstance(body.get("findings"), list), body


@then('the ORCE response identifies executor "{executor_id}"')
def step_reference_executor_identity(context, executor_id):
    body = _json(context.requests_response)
    assert (body.get("executor") or {}).get("id") == executor_id, body


@given('the archive storage of contract "{name}" is anchored in the audit trail')
def step_archive_storage_anchored(context, name):
    """Audit evidence reads only anchored entries, and the outbox anchors
    STORE_ARCHIVED_CONTRACT asynchronously after signing. The executor may
    observe exactly one request, so the scenario waits before it audits."""
    did = _contract_did(context, name)
    deadline = time.monotonic() + 90
    while True:
        cursor = context.db.cursor()
        cursor.execute(
            """
            SELECT bool_and(processed)
            FROM outbox_events
            WHERE event_type = 'STORE_ARCHIVED_CONTRACT' AND did = %s
            """,
            (did,),
        )
        anchored = cursor.fetchone()[0]
        cursor.close()
        context.db.rollback()
        if anchored:
            return
        assert time.monotonic() < deadline, f"STORE_ARCHIVED_CONTRACT of {did} was not anchored"
        time.sleep(1)


@when('the Auditor requests an executor-backed "{scope}" audit without a resource DID')
def step_audit_without_did(context, scope):
    _audit(context, scope, "executor-backed audit BDD")


@when(
    'the Auditor requests an executor-backed "{scope}" audit for contract "{name}" '
    'with justification "{justification}"'
)
def step_audit_for_contract(context, scope, name, justification):
    context.external_audit_contract_name = name
    _audit(context, scope, justification, _resource_did(context, scope, name))


@then('the PAC audit request is accepted with executor "{executor_id}"')
def step_accepted_executor(context, executor_id):
    assert context.requests_response.status_code == 200, context.requests_response.text
    body = _json(context.requests_response)
    assert (body.get("executor") or {}).get("id") == executor_id, body
    context.accepted_external_audit = body


@then("the PAC audit request is accepted with an empty findings list")
def step_accepted_empty(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    body = _json(context.requests_response)
    assert body.get("findings") == [], body
    context.accepted_external_audit = body


@then("the ORCE audit executor has observed exactly one request")
def step_one_observation(context):
    observations = _observations(context)
    assert len(observations) == 1, f"Expected exactly one executor request, got: {observations!r}"
    context.observed_external_audit = observations[0].get("request", observations[0])


@then('the observed request was handled by the compatible endpoint "{endpoint}"')
def step_observed_executor_endpoint(context, endpoint):
    observations = _observations(context)
    assert len(observations) == 1, f"Expected exactly one executor request, got: {observations!r}"
    observed_endpoint = observations[0].get("endpoint")
    assert observed_endpoint == endpoint, (
        f"Expected the DCS request at configured executor endpoint {endpoint!r}, "
        f"got {observed_endpoint!r}: {observations[0]!r}"
    )


@then("the ORCE audit executor has still observed exactly one request")
def step_still_one_observation(context):
    step_one_observation(context)


@then("the observed executor request carries version, audit correlation, requester, scope, resource, and justification")
def step_request_envelope(context):
    request = context.observed_external_audit
    expected = context.last_external_audit_request
    assert request.get("contract_version") == CONTRACT_VERSION, request
    uuid.UUID(request.get("audit_id", ""))
    uuid.UUID(request.get("correlation_id", ""))
    assert request["audit_id"] == request["correlation_id"], request
    assert request.get("scope") == expected["scope"], request
    assert (request.get("resource") or {}).get("did") == expected["did"], request
    requester = request.get("requester") or {}
    assert requester.get("subject") and "Auditor" in (requester.get("roles") or []), request
    assert request.get("justification") == expected["justification"], request


@then('the observed executor request contains DCS-procured "{evidence_type}" evidence')
def step_evidence(context, evidence_type):
    evidence = context.observed_external_audit.get("evidence") or {}
    values = evidence.get(evidence_type)
    assert isinstance(values, list) and values, (
        f"Expected non-empty DCS-procured {evidence_type!r} evidence, got: {evidence!r}"
    )
    if evidence_type != "signatures":
        return

    def wire_field(value, *names):
        if not isinstance(value, dict):
            return None
        by_normalized_key = {
            "".join(character for character in str(key).casefold() if character.isalnum()): nested
            for key, nested in value.items()
        }
        for name in names:
            normalized = "".join(character for character in name.casefold() if character.isalnum())
            if normalized in by_normalized_key:
                return by_normalized_key[normalized]
        return None

    signature_entries = [
        entry
        for resource in values
        if isinstance(resource, dict)
        for entry in (wire_field(resource, "audit_trail", "AuditTrail") or [])
        if isinstance(entry, dict)
        and wire_field(entry, "event_type", "EventType") == "SIGNATURE_EVIDENCE"
    ]
    assert signature_entries, (
        f"Expected at least one SIGNATURE_EVIDENCE audit-trail entry, got: {values!r}"
    )

    durable_metadata_keys = {
        "signatureId",
        "credentialType",
        "signedAt",
        "ipfsCid",
        "ceremonyId",
        "pdfHash",
        "contentHash",
        "fieldName",
    }
    for entry in signature_entries:
        event_data = wire_field(entry, "event_data", "EventData")
        assert isinstance(event_data, dict), (
            f"SIGNATURE_EVIDENCE event_data must be an object, got: {entry!r}"
        )
        assert wire_field(event_data, "signerDid", "signer_did"), entry
        assert wire_field(event_data, "status"), entry
        normalized_metadata_keys = {
            "".join(character for character in str(key).casefold() if character.isalnum())
            for key in event_data
        }
        expected_metadata_keys = {
            "".join(character for character in key.casefold() if character.isalnum())
            for key in durable_metadata_keys
        }
        assert expected_metadata_keys.intersection(normalized_metadata_keys), (
            f"SIGNATURE_EVIDENCE lacks durable provenance metadata: {entry!r}"
        )

    forbidden = {"signaturebytes", "jadessignature"}

    def assert_no_signature_material(value):
        if isinstance(value, dict):
            for key, nested in value.items():
                normalized = "".join(character for character in str(key).casefold() if character.isalnum())
                assert normalized not in forbidden, (
                    f"Signature evidence exposes forbidden cryptographic material under key {key!r}"
                )
                assert_no_signature_material(nested)
        elif isinstance(value, list):
            for nested in value:
                assert_no_signature_material(nested)

    assert_no_signature_material(values)


@then("the audit result contains executor version, execution time, correlated findings, and receipt")
def step_result_metadata(context):
    body = context.accepted_external_audit
    assert (body.get("executor") or {}).get("version"), body
    assert body.get("executed_at"), body
    assert body.get("receipt"), body
    findings = body.get("findings")
    assert isinstance(findings, list), body
    for finding in findings:
        assert finding.get("rule_id"), finding
        # NOT_EVALUATED is the fourth verdict (ADR-33): a rule carried into the
        # audit that nobody reached a conclusion about.
        assert finding.get("result") in (
            "PASSED",
            "FAILED",
            "REVIEW",
            "NOT_EVALUATED",
        ), finding
        assert finding.get("reason") and finding.get("severity"), finding
        assert isinstance(finding.get("evidence_refs"), list), finding
    context.persisted_audit_id = body.get("audit_id")


@then(
    "the PAC audit run stores audit and correlation IDs, executor provenance, an encrypted snapshot, "
    "and request and response hashes"
)
def step_persisted_run(context):
    expected = context.accepted_external_audit
    cursor = context.db.cursor()
    cursor.execute(
        """
        SELECT correlation_id::text, contract_version, scope, resource_did,
               request_hash, response_hash, snapshot_bytes, executor_id, executor_version,
               executed_at
        FROM pac_audit_runs
        WHERE audit_id = %s::uuid
        """,
        (expected["audit_id"],),
    )
    row = cursor.fetchone()
    cursor.close()
    assert row, f"No pac_audit_runs row for audit {expected['audit_id']}"
    (
        correlation_id,
        contract_version,
        scope,
        resource_did,
        request_hash,
        response_hash,
        snapshot_bytes,
        executor_id,
        executor_version,
        executed_at,
    ) = row
    assert correlation_id == expected["correlation_id"], row
    assert contract_version == expected["contract_version"], row
    assert scope == expected["scope"], row
    assert resource_did == (expected.get("resource") or {}).get("did"), row
    assert executor_id == expected["executor"]["id"], row
    assert executor_version == expected["executor"]["version"], row
    assert executed_at is not None, row
    for value in (request_hash, response_hash):
        assert isinstance(value, str) and value.startswith("sha256:") and len(value) == 71, row
        int(value[7:], 16)
    assert isinstance(snapshot_bytes, (bytes, bytearray, memoryview)) and snapshot_bytes, (
        "Expected a non-empty encrypted PAC snapshot"
    )
    snapshot = bytes(snapshot_bytes)
    try:
        plaintext = json.loads(snapshot)
    except (ValueError, UnicodeDecodeError):
        pass
    else:
        raise AssertionError(f"PAC snapshot exposes plaintext JSON: {type(plaintext).__name__}")
    assert expected["audit_id"].encode() not in snapshot, "Snapshot exposes the audit envelope"
    context.persisted_audit_response = {
        key: value for key, value in expected.items() if key != "timeline"
    }
    context.persisted_request_hash = request_hash
    context.persisted_response_hash = response_hash


@then("a matching PAC_AUDIT_EXECUTED outbox event records the persisted run hashes")
def step_audit_executed_outbox(context):
    expected = context.accepted_external_audit
    cursor = context.db.cursor()
    cursor.execute(
        """
        SELECT event_data
        FROM outbox_events
        WHERE event_type = 'PAC_AUDIT_EXECUTED'
          AND event_data->>'audit_id' = %s
        ORDER BY id DESC
        LIMIT 1
        """,
        (expected["audit_id"],),
    )
    row = cursor.fetchone()
    cursor.close()
    assert row, f"No PAC_AUDIT_EXECUTED outbox event for audit {expected['audit_id']}"
    event = row[0]
    assert event.get("correlation_id") == expected["correlation_id"], event
    assert event.get("executor_id") == expected["executor"]["id"], event
    assert event.get("executor_version") == expected["executor"]["version"], event
    assert event.get("request_hash") == context.persisted_request_hash, event
    assert event.get("response_hash") == context.persisted_response_hash, event


@then("the persisted PAC audit run rejects an update as append-only")
def step_run_append_only(context):
    cursor = context.db.cursor()
    rejected = False
    error_text = ""
    try:
        cursor.execute(
            "UPDATE pac_audit_runs SET justification = justification || ' mutated' "
            "WHERE audit_id = %s::uuid",
            (context.accepted_external_audit["audit_id"],),
        )
    except Exception as exc:  # psycopg2 raises the server-side trigger error
        rejected = True
        error_text = str(exc)
    finally:
        context.db.rollback()
        cursor.close()
    assert rejected and "append-only" in error_text, (
        f"Expected pac_audit_runs UPDATE to be rejected by append-only enforcement, got: {error_text!r}"
    )


@when('the Auditor reads the persisted "{scope}" report for contract "{name}"')
def step_read_report(context, scope, name):
    step_read_report_format(context, scope, name, "json")


@when('the Auditor exports format "{fmt}" from the persisted "{scope}" run for contract "{name}"')
def step_read_report_format(context, scope, name, fmt):
    params = urlencode({
        "scope": scope,
        "did": _resource_did(context, scope, name),
        "format": fmt,
        "justification": "persisted executor report BDD",
    })
    context.requests_response = get_with_headers(
        context, f"{pac_report_url(context)}?{params}",
        headers=AuthService.get_headers_for_roles(["Auditor"]),
    )
    context.external_report_bytes = context.requests_response.content
    context.external_report_format = fmt


@then("the report contains the same audit ID, executor metadata, findings, and receipt")
def step_report_same_run(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    report = _json(context.requests_response)
    expected = context.persisted_audit_response
    assert report.get("reportId") == expected["audit_id"], report
    assert report.get("contractVersion") == expected["contract_version"], report
    assert report.get("correlationId") == expected["correlation_id"], report
    assert report.get("executedAt") == expected["executed_at"], report
    assert report.get("scope") == expected["scope"], report
    assert report.get("did") == (expected.get("resource") or {}).get("did"), report
    assert report.get("executor") == expected.get("executor"), report
    assert report.get("receipt") == expected.get("receipt"), report
    assert report.get("requestHash") == context.persisted_request_hash, report
    assert report.get("responseHash") == context.persisted_response_hash, report
    assert report.get("generatedAt") and report.get("generatedBy"), report
    assert report.get("format") == "json", report
    findings = report.get("findings")
    assert isinstance(findings, list) and len(findings) == len(expected["findings"]), report
    for actual, original in zip(findings, expected["findings"]):
        assert actual.get("ruleId") == original["rule_id"], actual
        assert actual.get("result") == original["result"], actual
        assert actual.get("message") == original["reason"], actual
        assert actual.get("severity") == (original.get("severity") or original["result"]), actual
        assert (actual.get("evidenceRefs") or []) == (original.get("evidence_refs") or []), actual


@then('the "{fmt}" report represents the same persisted executor run')
def step_report_projection(context, fmt):
    response = context.requests_response
    assert response.status_code == 200, response.text
    assert response.content, f"Expected non-empty {fmt} report"
    expected = context.accepted_external_audit
    expected_rule = expected["findings"][0]["rule_id"]
    if fmt == "json":
        step_report_same_run(context)
        return
    if fmt == "csv":
        text = response.content.decode("utf-8")
        assert "section,timestamp,did,component,eventType" in text, text[:500]
        assert expected_rule in text, text[:1000]
        return
    if fmt == "pdf":
        from pypdf import PdfReader  # noqa: PLC0415

        text = "\n".join(
            page.extract_text() or ""
            for page in PdfReader(io.BytesIO(response.content)).pages
        )
        assert f"Report ID: {expected['audit_id']}" in text, text[:1000]
        assert expected_rule in text, text[:1000]
        return
    raise AssertionError(f"Unsupported report format fixture: {fmt!r}")


@then("the exact report bytes have a PAC_REPORT_GENERATED SHA-256 hash and IPFS CID")
def step_report_bytes_event(context):
    expected_hash = "sha256:" + hashlib.sha256(context.external_report_bytes).hexdigest()
    expected = context.accepted_external_audit
    cursor = context.db.cursor()
    cursor.execute(
        """
        SELECT event_data
        FROM outbox_events
        WHERE event_type = 'PAC_REPORT_GENERATED'
          AND event_data->>'report_id' = %s
          AND event_data->>'format' = %s
        ORDER BY id DESC
        LIMIT 1
        """,
        (expected["audit_id"], context.external_report_format),
    )
    row = cursor.fetchone()
    cursor.close()
    assert row, (
        f"No PAC_REPORT_GENERATED event for audit {expected['audit_id']} "
        f"and format {context.external_report_format}"
    )
    event = row[0]
    assert event.get("report_hash") == expected_hash, event
    assert event.get("report_cid"), event


@then("the audit fails with an executor infrastructure error")
def step_infrastructure_error(context):
    assert context.requests_response.status_code in (502, 503, 504), (
        f"Expected executor infrastructure error, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("no local fallback findings are returned")
def step_no_fallback(context):
    try:
        body = context.requests_response.json()
    except ValueError:
        body = {}
    assert not (isinstance(body, dict) and body.get("findings")), (
        f"Executor failure returned local fallback findings: {body!r}"
    )


@then("the observed executor request contains the lifecycle evidence expected in the persisted report")
def step_persisted_lifecycle_evidence(context):
    request = context.observed_external_audit
    entries = [
        entry
        for resource in request["evidence"][request["scope"]]
        for entry in resource.get("audit_trail", [])
        if entry.get("kind") == "TIMELINE"
    ]
    assert entries, "The workflow fixture must provide real persisted lifecycle events"
    accepted = [
        entry for entry in context.accepted_external_audit.get("timeline", [])
        if entry.get("kind") == "TIMELINE"
    ]
    assert {entry["id"] for entry in entries} == {entry["id"] for entry in accepted}
    assert any((entry.get("event_data") or {}).get("stored_by") for entry in entries), (
        "The archive workflow must supply an identifiable stored_by actor"
    )
    context.persisted_lifecycle_entries = sorted(
        entries, key=lambda entry: (datetime.fromisoformat(entry["created_at"].replace("Z", "+00:00")), entry["id"])
    )


@then('the "{fmt}" report has media type "{media_type}"')
def step_report_media_type(context, fmt, media_type):
    assert context.external_report_format == fmt
    actual = context.requests_response.headers.get("Content-Type", "").split(";", 1)[0].strip()
    assert actual == media_type, f"Expected {media_type}, got {actual}"


@then('the "{fmt}" report contains the chronological persisted lifecycle timeline')
def step_report_lifecycle(context, fmt):
    response = context.requests_response
    assert response.status_code == 200, response.text
    expected = context.persisted_lifecycle_entries
    if fmt == "json":
        report = response.json()
        events = report.get("events")
        assert isinstance(events, list) and len(events) == len(expected), report
        assert report["summary"]["totalEvents"] == len(expected), report
        for actual, entry in zip(events, expected):
            assert actual["id"] == entry["id"], actual
            actor = (entry.get("event_data") or {}).get("stored_by")
            if actor:
                assert actual.get("actor") == actor, actual
            assert actual["eventType"] == entry["event_type"], actual
            assert actual["component"] == entry["component"], actual
            assert actual.get("did") == entry.get("did"), actual
            assert datetime.fromisoformat(actual["timestamp"].replace("Z", "+00:00")) == datetime.fromisoformat(entry["created_at"].replace("Z", "+00:00")), actual
            assert (actual.get("details") or {}) == (entry.get("event_data") or {}), actual
        return
    if fmt == "csv":
        rows = list(csv.DictReader(io.StringIO(response.content.decode("utf-8"))))
        events = [row for row in rows if row["section"] == "event"]
        assert len(events) == len(expected), events
        for actual, entry in zip(events, expected):
            actor = (entry.get("event_data") or {}).get("stored_by")
            if actor:
                assert actual.get("actor") == actor, actual
            assert actual["eventType"] == entry["event_type"], actual
            assert actual["component"] == entry["component"], actual
            assert actual["did"] == (entry.get("did") or ""), actual
            assert datetime.fromisoformat(actual["timestamp"].replace("Z", "+00:00")) == datetime.fromisoformat(entry["created_at"].replace("Z", "+00:00")), actual
        return
    if fmt == "pdf":
        from pypdf import PdfReader  # noqa: PLC0415

        text = "".join("".join((page.extract_text() or "").split()) for page in PdfReader(io.BytesIO(response.content)).pages)
        assert "LifecycleEvents" in text, text
        timeline = text.split("LifecycleEvents", 1)[1]
        offset = 0
        for entry in expected:
            actor = (entry.get("event_data") or {}).get("stored_by")
            marker = "".join((entry["event_type"] + (entry.get("did") or "")).split())
            if actor:
                marker = "".join(("actor=" + actor).split()) + marker
            position = timeline.find(marker, offset)
            assert position >= offset, f"Missing or out-of-order lifecycle event {entry!r}"
            segment = timeline[offset:position]
            timestamp = datetime.fromisoformat(entry["created_at"].replace("Z", "+00:00"))
            assert timestamp.strftime("%Y-%m-%dT%H:%M:%S") in segment, entry
            offset = position + len(marker)
        assert "Nolifecycleevents." not in timeline, timeline
        return
    raise AssertionError(f"Unsupported report format: {fmt}")


@then("the persisted report is unavailable after resource key shredding")
def step_report_unavailable_after_shredding(context):
    response = context.requests_response
    assert response.status_code == 404, (
        f"Expected erased run to be unavailable, got {response.status_code}: {response.text}"
    )
    body = response.json()
    assert body.get("name") == "not_found", body
