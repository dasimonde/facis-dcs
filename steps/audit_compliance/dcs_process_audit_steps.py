"""BDD steps for the Process Audit & Compliance Management endpoints (UC-08,
backend/design/process_audit_and_compliance.go): /pac/audit, /pac/report
(GET report + POST incident), /pac/monitor.
"""

import json

from behave import given, then, when

from steps.support.api_client import (
    contract_retrieve_url,
    get_with_headers,
    pac_audit_timeline,
    pac_audit_url,
    pac_monitor_url,
    pac_report_url,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.orce_audit_control_service import OrceAuditControlService


def _observed_audit_entries(context, evidence_scope: str) -> list[dict]:
    entries = []
    for observation in OrceAuditControlService.observations(context, "audit"):
        request = observation.get("request", observation) if isinstance(observation, dict) else {}
        for scope_result in ((request.get("evidence") or {}).get(evidence_scope) or []):
            if isinstance(scope_result, dict):
                entries.extend(
                    entry
                    for entry in (scope_result.get("audit_trail") or [])
                    if isinstance(entry, dict)
                )
    return entries


@when('the Auditor triggers a process audit with scope "{scope}"')
def step_when_auditor_triggers_audit(context, scope):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Auditor"])
    OrceAuditControlService.reset(context, "audit")
    OrceAuditControlService.set_mode(context, "audit", "success")
    # A process audit sweeps every contract the run has accumulated, so late in
    # the suite it legitimately outlives the per-request timeout the other
    # steps use. The scope of this step is the sweep completing, not completing
    # fast, so it gets a deadline sized to the whole run's state.
    context.requests_response = _requests.post(
        pac_audit_url(context),
        json={"scope": scope, "justification": "BDD process audit"},
        headers=headers,
        timeout=max(context.http_timeout_seconds * 4, 240),
    )


@when('the Auditor triggers a process audit for contract "{name}"')
def step_when_auditor_audits_one_contract(context, name):
    """Audit one named contract, via the endpoint's own `did` filter.

    A scope-wide audit gathers every contract the instance holds, so a scenario
    asserting on its own contract silently depended on how many others every
    preceding feature had created: with 62 contracts the target was already
    missing from the returned set, and in a full CI run the call ran past the
    60s client timeout. The filter narrows the query itself, not just the
    response, so the assertion stays about this contract at any suite size.
    """
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    OrceAuditControlService.reset(context, "audit")
    OrceAuditControlService.set_mode(context, "audit", "success")
    context.requests_response = post_json(
        context,
        pac_audit_url(context),
        {"scope": "contracts", "did": did, "justification": "BDD process audit"},
        headers=headers,
    )


@when('I attempt to trigger a process audit with scope "{scope}"')
def step_when_attempt_trigger_audit(context, scope):
    headers = getattr(context, "headers", {})
    context.requests_response = post_json(context, pac_audit_url(context), {"scope": scope, "justification": "BDD process audit"}, headers=headers)


@when('the Auditor requests an audit report for scope "{scope}" in format "{fmt}"')
def step_when_auditor_requests_report(context, scope, fmt):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Auditor"])
    context.requests_response = _requests.get(
        pac_report_url(context),
        params={"scope": scope, "format": fmt, "justification": "BDD audit report"},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when("the Compliance Officer requests continuous monitoring")
def step_when_compliance_officer_monitors(context):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Compliance Officer"])
    context.requests_response = _requests.get(
        pac_monitor_url(context), headers=headers, timeout=context.http_timeout_seconds
    )


@when("the Compliance Officer submits a non-compliance incident report")
def step_when_compliance_officer_submits_incident(context):
    headers = AuthService.get_headers_for_roles(["Compliance Officer"])
    context.requests_response = post_json(context, pac_report_url(context), {}, headers=headers)


@when(
    'the Compliance Officer submits a non-compliance incident report linking contract '
    '"{name}" with risk type "{risk_type}" and detail "{detail}"'
)
def step_when_compliance_officer_submits_incident_for_contract(context, name, risk_type, detail):
    """POST /pac/report with a typed, contract-linked finding (DCS-IR-PACM-04,
    non-compliance-investigation-ui AC5). The Goa design for incident_report
    currently only declares a Token payload attribute — contract_did/
    template_did/findings do not exist there yet, so today these extra JSON
    fields are silently dropped by the generated decoder and nothing is
    persisted (the handler is a no-op, see
    backend/internal/service/process_audit_and_compliance.go IncidentReport).
    This is the contract the implementer builds against: typed fields
    contract_did (or template_did) plus a findings list of {risk_type, detail}.
    """
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Compliance Officer"])
    payload = {
        "contract_did": did,
        "findings": [{"risk_type": risk_type, "detail": detail}],
    }
    context.requests_response = post_json(context, pac_report_url(context), payload, headers=headers)


@given('contract "{name}" still has an open required approval task')
def step_given_open_approval_task(context, name):
    """Asserts the precondition the monitor sweep is supposed to flag: the
    contract (driven to REVIEWED = pending approval by the previous Given)
    still carries an OPEN approval task, observed via GET /contract/retrieve's
    approval_tasks list. Approvers are responsible peers (see
    backend/internal/contractworkflowengine/db package doc), so the missing
    approval is attributed to a peer DID, not an individual user."""
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Contract Approver"])
    resp = get_with_headers(context, contract_retrieve_url(context), headers=headers)
    assert resp.status_code == 200, f"contract retrieve failed: {resp.status_code} {resp.text}"
    tasks = resp.json().get("approval_tasks") or []
    open_tasks = [
        t for t in tasks
        if t.get("did") == did and str(t.get("state", "")).upper() == "OPEN"
    ]
    assert open_tasks, (
        f"Expected contract '{name}' (did={did}) to still have an OPEN approval task "
        f"as the monitoring precondition, got approval tasks: {tasks}"
    )


@then('the monitoring sweep flags contract "{name}" with a "{risk_type}" compliance risk')
def step_then_monitor_flags_risk(context, name, risk_type):
    assert context.requests_response.status_code == 200, (
        f"Expected 200 from /pac/monitor, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    did, _ = ContractService._contract_data(context, name)
    body = context.requests_response.json()
    risks = body.get("risks")
    assert isinstance(risks, list), f"Expected a 'risks' list in the monitor response, got: {body}"
    matching = [r for r in risks if r.get("did") == did and r.get("risk_type") == risk_type]
    assert matching, (
        f"Expected /pac/monitor to flag contract '{name}' (did={did}) with a "
        f"{risk_type} risk, got risks: {risks}"
    )
    assert matching[0].get("detail"), f"Expected the flagged risk to carry a detail message, got: {matching[0]}"


@then(
    'the monitoring sweep flags contract "{name}" with an "{risk_type}" '
    'compliance risk attributed to actor "{actor}"'
)
def step_then_monitor_flags_risk_with_actor(context, name, risk_type, actor):
    """DCS-FR-PACM-02 unauthorized-access rule: beyond the flagged risk
    itself, its detail must attribute the denial to the denied actor
    (retrieved_by on the persisted CONTRACT_ACCESS_DENIED artifact — the
    OID4VP-disclosed organization claim, see querymonitor.go
    unauthorizedAccessRisks)."""
    step_then_monitor_flags_risk(context, name, risk_type)
    did, _ = ContractService._contract_data(context, name)
    risks = context.requests_response.json().get("risks") or []
    matching = [r for r in risks if r.get("did") == did and r.get("risk_type") == risk_type]
    assert any(actor in (r.get("detail") or "") for r in matching), (
        f"Expected the {risk_type} risk for contract '{name}' (did={did}) to attribute "
        f"the denial to actor '{actor}' in its detail, got: {matching}"
    )


@then('the flagged risk for contract "{name}" is recorded in the PAC audit trail')
def step_then_flagged_risk_audited(context, name):
    # Each flagged risk is anchored per affected contract as a
    # PAC_COMPLIANCE_RISK event (querymonitor.go). The sweep-level
    # PAC_COMPLIANCE_MONITOR event carries no resource DID and only enters
    # the global chain, so the per-contract risk event is the auditable
    # artifact a PAC-scope read can prove. Anchoring is async (outbox ->
    # TSA -> IPFS), hence the poll.
    import time  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    found = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        OrceAuditControlService.reset(context, "audit")
        OrceAuditControlService.set_mode(context, "audit", "success")
        resp = post_json(
            context, pac_audit_url(context), {"scope": "PROCESS_AUDIT_AND_COMPLIANCE", "justification": "BDD audit re-trigger"}, headers=headers
        )
        assert resp.status_code == 200, f"PAC-scope audit failed: {resp.status_code} {resp.text}"
        found = [
            (entry.get("event_type"), entry.get("did"))
            for entry in _observed_audit_entries(context, "PROCESS_AUDIT_AND_COMPLIANCE")
        ]
        if ("PAC_COMPLIANCE_RISK", did) in found:
            return
        time.sleep(2)
    raise AssertionError(
        f"Expected a PAC_COMPLIANCE_RISK audit event for contract '{name}' (did={did}) "
        f"in the PROCESS_AUDIT_AND_COMPLIANCE trail, got entries: {found}"
    )


@then('the PAC audit trail records exactly one "{risk_type}" risk for contract "{name}"')
def step_then_risk_audited_once(context, risk_type, name):
    """DCS-FR-PACM-02: a violation alerts when it is detected, not on every
    sweep that still sees it. The sweep response keeps listing the risk for as
    long as it holds — that answers "what is wrong now" — but the audit trail
    and the webhook alert fire once per incident (querymonitor.go reconcile,
    backed by compliance_risk_findings).

    Anchoring is async (outbox -> TSA -> IPFS), so a duplicate could simply not
    have landed yet: wait for the first event, then let the trail settle before
    counting, or this would pass without proving anything.
    """
    import time  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])

    def risk_events():
        resp = post_json(
            context,
            pac_audit_url(context),
            {"scope": "PROCESS_AUDIT_AND_COMPLIANCE", "justification": "BDD risk dedup audit re-trigger"},
            headers=headers,
        )
        assert resp.status_code == 200, f"PAC-scope audit failed: {resp.status_code} {resp.text}"
        return [
            entry
            for entry in pac_audit_timeline(resp)
            if entry.get("event_type") == "PAC_COMPLIANCE_RISK"
            and entry.get("did") == did
            and (entry.get("event_data") or {}).get("risk_type") == risk_type
        ]

    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        if risk_events():
            break
        time.sleep(2)
    else:
        raise AssertionError(
            f"No PAC_COMPLIANCE_RISK audit event for contract '{name}' (did={did}) with "
            f"risk_type '{risk_type}' appeared within 90s"
        )

    time.sleep(15)
    events = risk_events()
    assert len(events) == 1, (
        f"Expected exactly one PAC_COMPLIANCE_RISK event for contract '{name}' (did={did}) "
        f"with risk_type '{risk_type}' after two sweeps, got {len(events)}: {events}"
    )


@then('the incident report is recorded as a PAC audit event for contract "{name}" with risk type "{risk_type}"')
def step_then_incident_report_recorded(context, name, risk_type):
    """Asserts the typed link the incident report submitted (contract_did +
    findings[].risk_type) was actually accepted and persisted — not just
    acknowledged with a 200 no-op. Expected event_type contract for the
    implementer: PAC_INCIDENT_REPORT, anchored per-contract (component
    PROCESS_AUDIT_AND_COMPLIANCE, GetDID() == the linked contract_did) —
    mirrors the existing PAC_COMPLIANCE_RISK anchoring pattern
    (querymonitor.go) so a PAC-scope audit read can prove the finding was
    recorded, not just accepted.
    """
    import time  # noqa: PLC0415

    assert context.requests_response.status_code == 200, (
        f"Expected 200 from /pac/report, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    found = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        OrceAuditControlService.reset(context, "audit")
        OrceAuditControlService.set_mode(context, "audit", "success")
        resp = post_json(
            context,
            pac_audit_url(context),
            {"scope": "PROCESS_AUDIT_AND_COMPLIANCE", "justification": "BDD incident-report audit re-trigger"},
            headers=headers,
        )
        assert resp.status_code == 200, f"PAC-scope audit failed: {resp.status_code} {resp.text}"
        found = [
            (entry.get("event_type"), entry.get("did"), (entry.get("event_data") or {}).get("risk_type"))
            for entry in _observed_audit_entries(context, "PROCESS_AUDIT_AND_COMPLIANCE")
        ]
        if ("PAC_INCIDENT_REPORT", did, risk_type) in found:
            return
        time.sleep(2)
    raise AssertionError(
        f"Expected a PAC_INCIDENT_REPORT audit event for contract '{name}' (did={did}) with "
        f"risk_type '{risk_type}' in the PROCESS_AUDIT_AND_COMPLIANCE trail, got entries: {found}"
    )


@then("the monitoring response reports a checked_at timestamp and a risks list")
def step_then_monitor_response_shape(context):
    assert context.requests_response.status_code == 200, (
        f"Expected 200 from /pac/monitor, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("checked_at"), f"Expected a checked_at timestamp in the monitor response, got: {body}"
    assert isinstance(body.get("risks"), list), (
        f"Expected the monitor response to carry a risks list (empty when compliant), got: {body}"
    )


@then('the process audit response includes an audit trail entry for contract "{name}"')
def step_then_audit_response_includes_contract(context, name):
    # POST /pac/audit now returns the versioned external-executor result, not
    # the legacy PACAuditResponse list. The DCS-procured timeline remains in
    # the request observed by the bundled ORCE executor. Inspect that request
    # so this assertion still proves that the newly created contract was part
    # of the audited evidence rather than merely accepting any 200 response.
    import time  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    all_dids = []
    deadline = time.monotonic() + 90
    while True:
        body = context.requests_response.json()
        assert isinstance(body, dict) and body.get("contract_version"), (
            f"Expected a versioned external PAC audit result, got: {body}"
        )
        observations = OrceAuditControlService.observations(context, "audit")
        requests = [
            observation.get("request", observation)
            for observation in observations
            if isinstance(observation, dict)
        ]
        all_dids = [
            scope_result.get("did")
            for request in requests
            for scope_result in ((request.get("evidence") or {}).get("contracts") or [])
            if isinstance(scope_result, dict)
        ]
        if did in all_dids:
            return
        if time.monotonic() > deadline:
            break
        time.sleep(2)
        OrceAuditControlService.reset(context, "audit")
        OrceAuditControlService.set_mode(context, "audit", "success")
        context.requests_response = post_json(
            context, pac_audit_url(context), {"scope": "CONTRACT_WORKFLOW_ENGINE", "justification": "BDD audit re-trigger"}, headers=headers
        )
        assert context.requests_response.status_code == 200, (
            f"process audit re-trigger failed: {context.requests_response.status_code} "
            f"{context.requests_response.text}"
        )
    assert did in all_dids, (
        f"Expected the CONTRACT_WORKFLOW_ENGINE audit trail to include an entry for contract "
        f"'{name}' (did={did}), got dids: {all_dids}"
    )


@then("the audit outbox holds a CONFIG_INTEGRITY_ATTESTATION record hashing the DID document")
def step_then_config_attestation_recorded(context):
    # The attestation is written once at process startup (DCS-NFR-SEC-04), so
    # it is already committed by the time any scenario runs — no polling.
    cursor = context.db.cursor()
    cursor.execute(
        """SELECT event_data FROM outbox_events
           WHERE component = 'PROCESS_AUDIT_AND_COMPLIANCE'
             AND event_type = 'CONFIG_INTEGRITY_ATTESTATION'
           ORDER BY id DESC LIMIT 1"""
    )
    row = cursor.fetchone()
    cursor.close()
    assert row, "Expected a CONFIG_INTEGRITY_ATTESTATION outbox record from startup (DCS-NFR-SEC-04)"
    event_data = row[0] if isinstance(row[0], dict) else json.loads(row[0])
    hashes = event_data.get("file_hashes") or {}
    did_hash = hashes.get("did-document")
    assert did_hash and len(did_hash) == 64 and all(c in "0123456789abcdef" for c in did_hash), (
        f"Expected the attestation to carry a 64-hex SHA-256 for the DID document, got: {event_data}"
    )
    assert event_data.get("did", "").startswith("did:"), (
        f"Expected the attestation to name the attesting instance by DID, got: {event_data}"
    )
