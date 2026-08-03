"""Executable red specifications for the minimal audit/archive/ORCE slice.

The test-only SQL seams below deliberately corrupt immutable archive evidence
after a genuine SIGNED workflow. They run only against the disposable BDD
database and restore the immutability trigger immediately after each update.
"""

from __future__ import annotations

import hashlib
import io
import json
import os
import subprocess
import time
import uuid

import requests
from behave import given, step, then, when

from steps.support.api_client import archive_audit_url, archive_search_url, pac_audit_url, pac_report_url, post_json
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.orce_audit_control_service import OrceAuditControlService


AUDIT_REASON = "BDD audit verification"
RULES = {
    "DB snapshot": "ARCHIVE_DB_SNAPSHOT",
    "content hash": "ARCHIVE_CONTENT_HASH",
    "IPFS snapshot": "ARCHIVE_IPFS_SNAPSHOT",
    "ORCE receipt": "ARCHIVE_ORCE_RECEIPT",
    "ORCE chain": "ARCHIVE_ORCE_CHAIN",
    "RFC-3161 TSA": "ARCHIVE_TSA_RFC3161",
}


def _headers(role: str) -> dict:
    return AuthService.get_headers_for_roles([role])


def _last_contract_name(context) -> str:
    names = list(getattr(context, "contract_dids", {}).keys())
    assert names, "No contract has been prepared in this scenario"
    return names[-1]


def _did(context, name: str) -> str:
    return ContractService._contract_data(context, name)[0]


def _source_template_did(context, name: str) -> str:
    contract = ContractService._refresh_contract(context, name)
    template_did = contract.get("template_did") or contract.get("templateDid")
    assert template_did, f"Contract {name!r} has no source template DID: {contract!r}"
    return template_did


def _audit_entries(body) -> list[dict]:
    groups = body if isinstance(body, list) else []
    return [
        entry
        for group in groups
        if isinstance(group, dict)
        for entry in (group.get("audit_trail") or [])
        if isinstance(entry, dict)
    ]


def _audit_body(context):
    groups = getattr(context, "last_audit_groups", None)
    return groups if groups is not None else context.requests_response.json()


def _event_data(entry: dict) -> dict:
    data = entry.get("event_data")
    if isinstance(data, dict):
        return data
    if isinstance(data, str):
        try:
            parsed = json.loads(data)
            return parsed if isinstance(parsed, dict) else {}
        except json.JSONDecodeError:
            return {}
    return {}


def _finding_rule(entry: dict) -> str:
    return str(entry.get("rule_id") or entry.get("ruleId") or _event_data(entry).get("rule_id") or "")


def _finding_result(entry: dict) -> str:
    return str(entry.get("result") or _event_data(entry).get("result") or "").upper()


def _finding_reason(entry: dict) -> str:
    return str(entry.get("reason") or _event_data(entry).get("reason") or "").strip()


def _entry_kind(entry: dict) -> str:
    return str(entry.get("kind") or _event_data(entry).get("kind") or "").upper()


def _request_audit(context, role: str, scope: str, justification: str | None, did: str | None = None):
    payload = {"scope": scope}
    if justification is not None:
        payload["justification"] = justification
    if did:
        payload["did"] = did
    context.last_audit_request = payload
    context.last_access_request = payload
    context.last_audit_role = role
    context.last_audit_groups = []
    observe_executor = role in {"Auditor", "Archive Manager"} and bool(justification)
    if observe_executor:
        OrceAuditControlService.reset(context, "audit")
        mode = getattr(context, "next_audit_executor_mode", "success")
        OrceAuditControlService.set_mode(context, "audit", mode)
        context.next_audit_executor_mode = "success"
    context.requests_response = post_json(context, pac_audit_url(context), payload, headers=_headers(role))
    if observe_executor and context.requests_response.status_code == 200:
        body = context.requests_response.json()
        if isinstance(body, dict) and body.get("audit_id"):
            observations = OrceAuditControlService.observations(context, "audit")
            matching = [
                observation.get("request", observation)
                for observation in observations
                if isinstance(observation, dict)
                and (observation.get("request", observation) or {}).get("audit_id") == body["audit_id"]
            ]
            if matching:
                evidence = matching[-1].get("evidence") or {}
                context.last_audit_groups = [
                    group
                    for groups in evidence.values()
                    if isinstance(groups, list)
                    for group in groups
                    if isinstance(group, dict)
                ]


def _request_report(context, role: str, scope: str, fmt: str, justification: str):
    params = {"scope": scope, "format": fmt, "justification": justification}
    context.last_access_request = params
    context.requests_response = requests.get(
        pac_report_url(context),
        params=params,
        headers=_headers(role),
        timeout=context.http_timeout_seconds,
    )


@when('the Auditor runs scope "{scope}" for DID "{did}" with justification "{justification}"')
def step_auditor_scope_did(context, scope, did, justification):
    _request_audit(context, "Auditor", scope, justification, did)


@when('the Auditor runs scope "{scope}" for the source template of that contract with justification "{justification}"')
def step_auditor_scope_source_template(context, scope, justification):
    name = _last_contract_name(context)
    _request_audit(context, "Auditor", scope, justification, _source_template_did(context, name))


@when('the Auditor runs scope "{scope}" for that contract with justification "{justification}"')
def step_auditor_scope_last_contract(context, scope, justification):
    _request_audit(context, "Auditor", scope, justification, _did(context, _last_contract_name(context)))


@when('the Archive Manager runs scope "{scope}" with justification "{justification}"')
def step_archive_manager_scope(context, scope, justification):
    _request_audit(context, "Archive Manager", scope, justification)


@when('the Auditor runs scope "{scope}" with justification "{justification}"')
def step_auditor_scope(context, scope, justification):
    _request_audit(context, "Auditor", scope, justification)


@when('the Auditor runs scope "{scope}" without a justification')
def step_auditor_without_reason(context, scope):
    _request_audit(context, "Auditor", scope, None)


@when('a Contract Manager runs scope "{scope}" with justification "{justification}"')
def step_manager_scope(context, scope, justification):
    _request_audit(context, "Contract Manager", scope, justification)


@when('a Contract Manager exports scope "{scope}" as "{fmt}" with justification "{justification}"')
def step_manager_report(context, scope, fmt, justification):
    _request_report(context, "Contract Manager", scope, fmt, justification)


@when('the Auditor exports scope "{scope}" as "{fmt}" with justification "{justification}"')
def step_auditor_report(context, scope, fmt, justification):
    _request_report(context, "Auditor", scope, fmt, justification)


@then("the report request is accepted")
def step_report_accepted(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    assert context.requests_response.content, "Expected non-empty report bytes"


@then("the process audit request is accepted")
def step_audit_accepted(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    body = context.requests_response.json()
    assert isinstance(body, dict) and body.get("contract_version"), context.requests_response.text


@then('every returned audit group belongs to scope "{scope}" and DID "{did}"')
def step_groups_filtered(context, scope, did):
    aliases = {
        "templates": "CONTRACT_TEMPLATE_REPOSITORY",
        "contracts": "CONTRACT_WORKFLOW_ENGINE",
        "signatures": "SIGNATURE_MANAGEMENT",
        "archive": "CONTRACT_STORAGE_ARCHIVE",
    }
    expected_component = aliases[scope]
    deadline = time.monotonic() + 90
    while True:
        body = _audit_body(context)
        if body and all(group.get("audit_trail") for group in body):
            for group in body:
                assert group.get("component") == expected_component, group
                assert group.get("did") == did, group
                entries = group.get("audit_trail") or []
                assert all(entry.get("did") == did for entry in entries), entries
            return
        if time.monotonic() >= deadline:
            raise AssertionError(f"Expected a non-empty {scope} audit result for {did}: {body!r}")
        time.sleep(2)
        _request_audit(context, context.last_audit_role, scope, context.last_audit_request["justification"], did)


@then('the filtered audit contains a non-empty "{scope}" group for the source template of that contract')
def step_source_template_group_non_empty(context, scope):
    step_groups_filtered(context, scope, _source_template_did(context, _last_contract_name(context)))


@then('the filtered audit contains a non-empty "{scope}" group for that contract')
def step_contract_group_non_empty(context, scope):
    step_groups_filtered(context, scope, _did(context, _last_contract_name(context)))


@then("the audit response distinguishes timeline events from integrity checks")
def step_kinds_distinguished(context):
    entries = _audit_entries(_audit_body(context))
    kinds = {_entry_kind(entry) for entry in entries}
    assert "CHECK" in kinds, f"No CHECK kind returned: {entries!r}"
    assert "TIMELINE" in kinds, f"No TIMELINE kind returned: {entries!r}"


@then("every integrity check has a result, rule reference, and reason")
def step_checks_structured(context):
    checks = [entry for entry in _audit_entries(_audit_body(context)) if _entry_kind(entry) == "CHECK"]
    assert checks, "Expected at least one integrity check"
    for check in checks:
        assert _finding_result(check) in {"PASSED", "FAILED"}, check
        assert _finding_rule(check), check
        assert _finding_reason(check), check


@then("the archive integrity result is passed")
def step_archive_passed(context):
    deadline = time.monotonic() + 90
    while True:
        checks = [entry for entry in _audit_entries(_audit_body(context)) if _entry_kind(entry) == "CHECK"]
        if checks and all(_finding_result(entry) == "PASSED" for entry in checks):
            return
        if time.monotonic() >= deadline:
            raise AssertionError(checks)
        time.sleep(2)
        _request_audit(
            context,
            context.last_audit_role,
            context.last_audit_request["scope"],
            context.last_audit_request["justification"],
            context.last_audit_request.get("did"),
        )


@then("the audit response is a successful empty result")
def step_empty_result(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    body = context.requests_response.json()
    assert isinstance(body, dict) and body.get("contract_version"), body
    assert body.get("findings") == [], body


@given("the audit executor returns a successful empty result")
def step_audit_executor_success_empty(context):
    context.next_audit_executor_mode = "success_empty"


@when(
    "the Auditor runs an archive audit while audit trail persistence is "
    'unavailable with justification "{justification}"'
)
def step_audit_with_unavailable_persistence(context, justification):
    """Exercise the API's infrastructure-error path against the disposable BDD DB.

    The table is restored before this step returns, including when the request
    itself fails unexpectedly, so subsequent scenarios see the normal schema.
    """
    original = "audit_trail_log"
    unavailable = f"audit_trail_log_bdd_unavailable_{uuid.uuid4().hex[:8]}"
    cursor = context.db.cursor()
    try:
        cursor.execute(f'ALTER TABLE {original} RENAME TO {unavailable}')
        context.db.commit()
        _request_audit(context, "Auditor", "archive", justification)
    finally:
        try:
            cursor.execute(f'ALTER TABLE {unavailable} RENAME TO {original}')
            context.db.commit()
        except Exception:
            context.db.rollback()
            raise
        finally:
            cursor.close()


@then("the audit request fails with an infrastructure error")
def step_audit_infrastructure_error(context):
    assert context.requests_response.status_code == 500, context.requests_response.text


@given('contract "{name}" exists in a pre-effective lifecycle state')
def step_pre_effective_contract(context, name):
    ContractService._create_contract_in_draft(context, name)
    contract = ContractService._refresh_contract(context, name)
    state = str(contract.get("state") or contract.get("status") or "").upper()
    assert state and state not in {"ACTIVE", "SIGNED", "TERMINATED", "EXPIRED"}, contract


@then("the contract audit contains lifecycle evidence for that contract")
def step_pre_effective_lifecycle_visible(context):
    did = _did(context, _last_contract_name(context))
    deadline = time.monotonic() + 90
    entries = []
    while time.monotonic() < deadline:
        entries = [entry for entry in _audit_entries(_audit_body(context)) if entry.get("did") == did]
        if any(_entry_kind(entry) == "TIMELINE" for entry in entries):
            return
        time.sleep(2)
        _request_audit(
            context,
            context.last_audit_role,
            context.last_audit_request["scope"],
            context.last_audit_request["justification"],
            did,
        )
        assert context.requests_response.status_code == 200, context.requests_response.text
    assert entries, f"No audit evidence returned for pre-effective contract {did}"
    raise AssertionError(f"No timeline audit evidence returned for pre-effective contract {did}: {entries!r}")


@then("no failed finding is caused solely by the contract being pre-effective")
def step_no_pre_effective_false_failure(context):
    did = _did(context, _last_contract_name(context))
    lifecycle_terms = ("lifecycle", "effective", "effectivity", "contract state", "status")
    false_failures = []
    for entry in _audit_entries(_audit_body(context)):
        if entry.get("did") != did or _finding_result(entry) != "FAILED":
            continue
        classification = f"{_finding_rule(entry)} {_finding_reason(entry)}".lower()
        if any(term in classification for term in lifecycle_terms):
            false_failures.append(entry)
    assert not false_failures, false_failures


@then("passed findings exist for DB snapshot, content hash, IPFS snapshot, ORCE receipt, ORCE chain, and RFC-3161 TSA")
def step_all_integrity_rules_pass(context):
    deadline = time.monotonic() + 90
    while True:
        findings = _audit_entries(_audit_body(context))
        passed = {_finding_rule(entry) for entry in findings if _finding_result(entry) == "PASSED"}
        missing = set(RULES.values()) - passed
        if not missing:
            return
        if time.monotonic() >= deadline:
            raise AssertionError(f"Missing passed rules: {missing}")
        time.sleep(2)
        _request_audit(
            context,
            context.last_audit_role,
            context.last_audit_request["scope"],
            context.last_audit_request["justification"],
            context.last_audit_request.get("did"),
        )


def _temporarily_disable_archive_immutability(context, statement: str, params: tuple):
    cursor = context.db.cursor()
    try:
        cursor.execute(
            "ALTER TABLE contract_archive_entries DISABLE TRIGGER contract_archive_entries_protect_immutable_fields"
        )
        cursor.execute(statement, params)
        assert cursor.rowcount == 1, f"Expected to corrupt exactly one archive row, changed {cursor.rowcount}"
        cursor.execute(
            "ALTER TABLE contract_archive_entries ENABLE TRIGGER contract_archive_entries_protect_immutable_fields"
        )
        context.db.commit()
    except Exception:
        context.db.rollback()
        cursor.execute(
            "ALTER TABLE contract_archive_entries ENABLE TRIGGER contract_archive_entries_protect_immutable_fields"
        )
        context.db.commit()
        raise
    finally:
        cursor.close()


@given('its archived evidence is corrupted as "{defect}"')
def step_corrupt_archive_evidence(context, defect):
    name = _last_contract_name(context)
    did = _did(context, name)
    context.corrupted_contract_name = name
    if defect == "content hash":
        _temporarily_disable_archive_immutability(
            context,
            "UPDATE contract_archive_entries SET content_hash = %s WHERE did = %s",
            ("sha256:" + "0" * 64, did),
        )
    elif defect == "missing receipt":
        # Make the persisted entry no longer match its receipt-bearing store
        # event. The product must classify this as a receipt finding, not abort.
        _temporarily_disable_archive_immutability(
            context,
            "UPDATE contract_archive_entries SET contract_version = contract_version + 100000 WHERE did = %s",
            (did,),
        )
    elif defect == "invalid TSA":
        _temporarily_disable_archive_immutability(
            context,
            "UPDATE contract_archive_entries SET tsa_receipt = %s::jsonb WHERE did = %s",
            (json.dumps({"token": "not-a-valid-rfc3161-token"}), did),
        )
    else:
        raise AssertionError(f"Unknown archive defect {defect!r}")


@then('a failed archive finding with rule "{rule}" and a non-empty reason is returned')
def step_failed_rule(context, rule):
    matches = [
        entry
        for entry in _audit_entries(_audit_body(context))
        if _finding_rule(entry) == rule and _finding_result(entry) == "FAILED"
    ]
    assert matches, f"No failed {rule} finding in {context.requests_response.text}"
    assert all(_finding_reason(entry) for entry in matches), matches


def _archive_entry(context, name: str) -> dict:
    did = _did(context, name)
    response = requests.get(
        archive_search_url(context),
        params={"did": did},
        headers=_headers("Archive Manager"),
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code == 200, response.text
    entries = response.json()
    assert isinstance(entries, list), entries
    return next((entry for entry in entries if entry.get("did") == did), {})


@then("its archive entry records signer, credential type, ceremony, field, signing time, PDF CID, and PDF hash")
def step_real_signing_evidence(context):
    did = _did(context, _last_contract_name(context))
    cursor = context.db.cursor()
    try:
        cursor.execute(
            "SELECT signature_metadata FROM contract_archive_entries "
            "WHERE did = %s ORDER BY contract_version DESC LIMIT 1",
            (did,),
        )
        row = cursor.fetchone()
    finally:
        cursor.close()
    assert row, f"No archive entry found for {did}"
    metadata = row[0]
    if isinstance(metadata, str):
        metadata = json.loads(metadata)
    required = ("signer", "credential_type", "ceremony_id", "field", "signed_at", "pdf_cid", "pdf_hash")
    missing = [key for key in required if not metadata.get(key)]
    assert not missing, f"Missing real signing evidence {missing}: {metadata!r}"
    assert metadata.get("status") == "SIGNED", metadata


@then("its archive entry stores credential hashes but no credential payload")
def step_credential_hashes_only(context):
    did = _did(context, _last_contract_name(context))
    cursor = context.db.cursor()
    try:
        cursor.execute(
            "SELECT credential_hashes FROM contract_archive_entries "
            "WHERE did = %s ORDER BY contract_version DESC LIMIT 1",
            (did,),
        )
        row = cursor.fetchone()
    finally:
        cursor.close()
    assert row, f"No archive entry found for {did}"
    hashes = row[0]
    if isinstance(hashes, str):
        hashes = json.loads(hashes)
    serialized = json.dumps(hashes).lower()
    assert "sha256:" in serialized, hashes
    assert "vp_token" not in serialized and "credential_payload" not in serialized and "raw_credential" not in serialized


def _orce_config(context):
    notary = os.getenv("BDD_ORCE_ARCHIVE_NOTARY_URL", os.getenv("ORCE_ARCHIVE_NOTARY_URL", "")).strip()
    audit_log = os.getenv("BDD_ORCE_ARCHIVE_AUDIT_LOG_URL", os.getenv("ORCE_ARCHIVE_AUDIT_LOG_URL", "")).strip()
    token = os.getenv(
        "BDD_ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN",
        os.getenv("ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN", ""),
    ).strip()
    assert notary and audit_log and token, (
        "BDD_ORCE_ARCHIVE_NOTARY_URL, BDD_ORCE_ARCHIVE_AUDIT_LOG_URL and "
        "BDD_ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN must be configured"
    )
    context.orce_notary_url, context.orce_audit_log_url, context.orce_token = notary, audit_log, token


def _auth_header(context) -> dict:
    return {"Authorization": f"Bearer {context.orce_token}", "Content-Type": "application/json"}


def _orce_request(method: str, url: str, **kwargs):
    """Retry only transient port-forward disconnects during an ORCE rollout.

    The persistent keep_port_forward supervisor reconnects to the replacement
    pod, but requests made in that short handover window can still see a
    ConnectionError. HTTP responses are returned immediately so all existing
    status and payload assertions retain their original semantics.
    """
    deadline = time.monotonic() + 90
    while True:
        try:
            return requests.request(method, url, **kwargs)
        except requests.ConnectionError:
            if time.monotonic() >= deadline:
                raise
            time.sleep(2)


def _orce_payload(context, archive_id: str, variant: str = "original") -> dict:
    unique_id = getattr(context, "orce_ids", {}).get(archive_id)
    if not unique_id:
        unique_id = f"{archive_id}-{uuid.uuid4()}"
        context.orce_ids = {**getattr(context, "orce_ids", {}), archive_id: unique_id}
    digest = hashlib.sha256(f"{unique_id}:{variant}".encode()).hexdigest()
    return {
        "eventType": "ARCHIVE_STORED",
        "did": f"did:web:bdd.example:{unique_id}",
        "contractVersion": 1,
        "archiveEntryId": unique_id,
        "contentHash": f"sha256:{digest}",
        "snapshotCid": f"bafy-bdd-{digest[:24]}",
        "storedBy": "did:web:bdd.example:auditor",
        "storedAt": "2026-07-13T10:00:00Z",
    }


@given("the configured ORCE archive notary is reachable with its bearer token")
def step_orce_reachable(context):
    _orce_config(context)
    response = _orce_request(
        "GET",
        context.orce_audit_log_url,
        headers=_auth_header(context),
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code in (200, 404), f"ORCE archive log is unreachable: {response.status_code} {response.text}"


@step('archive event "{archive_id}" is notarized')
def step_notarize(context, archive_id):
    response = _orce_request(
        "POST",
        context.orce_notary_url,
        json=_orce_payload(context, archive_id),
        headers=_auth_header(context),
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code == 200, response.text
    context.orce_receipts = {**getattr(context, "orce_receipts", {}), archive_id: response.json()}


@step("the ORCE archive flow is restarted")
def step_restart_orce(context):
    namespace = os.getenv("BDD_ORCE_NAMESPACE", "default")
    deployment = os.getenv("BDD_ORCE_DEPLOYMENT", "dcs-orce")
    kubectl = os.getenv("BDD_KUBECTL", "kubectl")
    subprocess.run([kubectl, "rollout", "restart", f"deployment/{deployment}", "-n", namespace], check=True)
    subprocess.run(
        [kubectl, "rollout", "status", f"deployment/{deployment}", "-n", namespace, "--timeout=120s"], check=True
    )


@then("the second ORCE receipt references the first receipt hash")
def step_receipt_chained(context):
    receipts = list(context.orce_receipts.values())
    assert len(receipts) >= 2, receipts
    assert receipts[-1].get("previousHash") == receipts[-2].get("eventHash"), receipts


@when("the current ORCE audit log is remembered")
def step_remember_orce_log(context):
    response = _orce_request(
        "GET",
        context.orce_audit_log_url,
        headers=_auth_header(context),
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code in (200, 404), response.text
    context.orce_log_before = response.content if response.status_code == 200 else b""


@step('archive event "{archive_id}" is posted without a bearer token')
def step_unauthorized_append(context, archive_id):
    context.requests_response = _orce_request(
        "POST",
        context.orce_notary_url,
        json=_orce_payload(context, archive_id),
        headers={"Content-Type": "application/json"},
        timeout=context.http_timeout_seconds,
    )


@then("the ORCE request is unauthorized")
def step_orce_unauthorized(context):
    assert context.requests_response.status_code in (401, 403), context.requests_response.text


@then("the ORCE audit log is unchanged")
def step_orce_log_unchanged(context):
    response = _orce_request(
        "GET",
        context.orce_audit_log_url,
        headers=_auth_header(context),
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code in (200, 404), response.text
    after = response.content if response.status_code == 200 else b""
    assert after == context.orce_log_before, "Unauthorized POST mutated the persistent ORCE audit log"


@when("the ORCE audit log is requested without a bearer token")
def step_unauthorized_log_get(context):
    context.requests_response = _orce_request(
        "GET",
        context.orce_audit_log_url,
        timeout=context.http_timeout_seconds,
    )


@when('archive event "{archive_id}" is notarized twice with identical content')
def step_duplicate_identical(context, archive_id):
    payload = _orce_payload(context, archive_id)
    first = _orce_request(
        "POST",
        context.orce_notary_url,
        json=payload,
        headers=_auth_header(context),
        timeout=context.http_timeout_seconds,
    )
    second = _orce_request(
        "POST",
        context.orce_notary_url,
        json=payload,
        headers=_auth_header(context),
        timeout=context.http_timeout_seconds,
    )
    assert first.status_code == 200 and second.status_code == 200, f"{first.text} / {second.text}"
    context.duplicate_receipts = (first.json(), second.json())


@then("both ORCE responses contain the same receipt")
def step_same_receipt(context):
    assert context.duplicate_receipts[0] == context.duplicate_receipts[1], context.duplicate_receipts


@when('archive event "{archive_id}" is notarized with different content')
def step_duplicate_conflict(context, archive_id):
    context.requests_response = _orce_request(
        "POST",
        context.orce_notary_url,
        json=_orce_payload(context, archive_id, variant="conflict"),
        headers=_auth_header(context),
        timeout=context.http_timeout_seconds,
    )


@then("the ORCE request is rejected as a conflict")
def step_orce_conflict(context):
    assert context.requests_response.status_code == 409, context.requests_response.text


@when('the Auditor exports scope "{scope}" for that contract as "{fmt}" with justification "{justification}"')
def step_export_contract(context, scope, fmt, justification):
    did = _did(context, _last_contract_name(context))
    _request_audit(context, "Auditor", scope, justification, did)
    assert context.requests_response.status_code == 200, context.requests_response.text
    response = requests.get(
        pac_report_url(context),
        params={"scope": scope, "format": fmt, "did": did, "justification": justification},
        headers=_headers("Auditor"),
        timeout=context.http_timeout_seconds,
    )
    context.requests_response = response
    context.report_bytes = response.content
    context.report_format = fmt
    context.report_did = did


def _report_text(context, fmt: str) -> str:
    raw = context.report_bytes
    if fmt == "pdf":
        try:
            from pypdf import PdfReader  # noqa: PLC0415

            return "\n".join(page.extract_text() or "" for page in PdfReader(io.BytesIO(raw)).pages)
        except ImportError as exc:
            raise AssertionError("pypdf is required to validate the PDF report content") from exc
    return raw.decode("utf-8")


@then('the "{fmt}" report contains timestamped audit results')
def step_report_timestamps(context, fmt):
    assert context.requests_response.status_code == 200, context.requests_response.text
    text = _report_text(context, fmt).lower()
    assert any(marker in text for marker in ("executed_at", "timestamp", "generated at")), text[:1000]


@then('the "{fmt}" report contains archive findings with rule references and results')
def step_report_findings(context, fmt):
    text = _report_text(context, fmt).upper()
    assert "ARCHIVE_" in text, text[:1000]
    assert "PASSED" in text or "FAILED" in text, text[:1000]


@then("the exact delivered report bytes have a recorded SHA-256 hash and IPFS CID")
def step_report_bytes_archived(context):
    expected_hash = "sha256:" + hashlib.sha256(context.report_bytes).hexdigest()
    deadline = time.monotonic() + 90
    matching = []
    while time.monotonic() < deadline:
        _request_audit(
            context,
            "Auditor",
            "archive",
            "verify archived report bytes",
            context.report_did,
        )
        assert context.requests_response.status_code == 200, context.requests_response.text
        matching = [
            _event_data(entry)
            for entry in _audit_entries(_audit_body(context))
            if str(entry.get("event_type", "")).upper() == "PAC_REPORT_GENERATED"
            and _event_data(entry).get("report_hash") == expected_hash
        ]
        if matching:
            break
        time.sleep(2)
    assert matching, f"No PAC_REPORT_GENERATED event records exact hash {expected_hash}"
    assert all(data.get("report_cid") for data in matching), matching


def _access_log_row(context, method: str, require_auth_success: bool | None) -> tuple:
    request = context.last_access_request
    success_clause = " AND success = TRUE" if require_auth_success else ""
    cursor = context.db.cursor()
    cursor.execute(
        f"""
        SELECT attempt_by, roles, attempted_at, scope, did, justification
        FROM access_attempts
        WHERE service = 'ProcessAuditAndCompliance'
          AND method = %s
          AND scope = %s
          AND justification = %s
          AND did IS NOT DISTINCT FROM %s
          {success_clause}
        ORDER BY attempted_at DESC LIMIT 1
        """,
        (method, request["scope"], request["justification"], request.get("did")),
    )
    row = cursor.fetchone()
    cursor.close()
    expected_success = " with authentication success=true" if require_auth_success else ""
    assert row is not None, (
        f"No {method} access-attempt row found{expected_success} for "
        f"scope={request['scope']!r}, justification={request['justification']!r}, did={request.get('did')!r}"
    )
    return row


def _assert_access_metadata(context, method: str, require_auth_success: bool | None):
    actor, roles, attempted_at, scope, did, justification = _access_log_row(
        context, method, require_auth_success
    )
    assert actor and roles and attempted_at and scope and justification
    assert scope == context.last_access_request["scope"]
    assert justification == context.last_access_request["justification"]
    assert did == context.last_access_request.get("did")


@then("the denied audit access is logged with actor, roles, time, scope, and justification")
def step_denied_access_logged(context):
    _assert_access_metadata(context, "audit", None)


@then("the audit action is logged with actor, roles, time, scope, and justification")
def step_allowed_access_logged(context):
    _assert_access_metadata(context, "audit", True)


@then("the denied report access is logged with actor, roles, time, scope, and justification")
def step_denied_report_access_logged(context):
    _assert_access_metadata(context, "audit_report", None)


@then("the report action is logged with actor, roles, time, scope, and justification")
def step_allowed_report_access_logged(context):
    _assert_access_metadata(context, "audit_report", True)


def _findings_for_did(body, did: str) -> list[tuple[str, str, str]]:
    return sorted(
        (_finding_rule(entry), _finding_result(entry), _finding_reason(entry))
        for entry in _audit_entries(body)
        if entry.get("did") == did and _entry_kind(entry) == "CHECK"
    )


@then('the archive audit contains a passed summary for "{name}"')
def step_mixed_valid(context, name):
    findings = _findings_for_did(_audit_body(context), _did(context, name))
    assert findings and all(result == "PASSED" for _, result, _ in findings), findings


@then('the archive audit contains a failed finding for "{name}"')
def step_mixed_damaged(context, name):
    findings = _findings_for_did(_audit_body(context), _did(context, name))
    assert any(result == "FAILED" for _, result, _ in findings), findings


@then("the PAC archive audit and archive audit endpoint return the same integrity findings")
def step_shared_archive_core(context):
    pac_body = _audit_body(context)
    response = requests.get(
        archive_audit_url(context),
        params={"justification": "shared integrity core BDD-4724"},
        headers=_headers("Auditor"),
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code == 200, response.text
    archive_body = response.json()
    for name in ("Healthy Archive Contract", "Damaged Archive Contract"):
        did = _did(context, name)
        assert _findings_for_did(pac_body, did) == _findings_for_did(archive_body, did)
