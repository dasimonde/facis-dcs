"""BDD bindings for reasoned signature revocation (DCS-IR-SM-06)."""

import json
import time

import requests
from behave import then, when

from steps.support.api_client import (
    post_json,
    signature_audit_url,
    signature_revoke_url,
    signature_view_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


def _signature_to_revoke(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_headers = AuthService.get_headers_for_roles(["Contract Manager"])
    response = requests.get(
        signature_view_url(context),
        params={"did": did},
        headers=manager_headers,
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code == 200, (
        f"Signature view failed for contract '{name}': "
        f"{response.status_code} {response.text}"
    )
    signatures = response.json().get("signatures") or []
    assert signatures, (
        f"Expected an applied signature on contract '{name}', got: {response.json()}"
    )
    return did, signatures[0]["signer_did"], manager_headers


def _request_revocation(context, name, reason_marker):
    did, signer_did, manager_headers = _signature_to_revoke(context, name)
    payload = {"did": did, "signer_did": signer_did}
    if reason_marker != "missing":
        payload["reason"] = reason_marker
    context.requests_response = post_json(
        context,
        signature_revoke_url(context),
        payload,
        headers=manager_headers,
    )


@when(
    'the Contract Manager revokes the applied signature of contract "{name}" '
    'with reason "{reason}"'
)
def step_when_revoke_signature_with_reason(context, name, reason):
    _request_revocation(context, name, reason)


@when(
    'the Contract Manager attempts to revoke the applied signature of contract '
    '"{name}" without a reason'
)
def step_when_revoke_signature_without_reason(context, name):
    _request_revocation(context, name, "missing")


@when(
    'the Contract Manager attempts to revoke the applied signature of contract '
    '"{name}" with a whitespace-only reason'
)
def step_when_revoke_signature_with_blank_reason(context, name):
    _request_revocation(context, name, "   ")


@then("the signature revocation is rejected because a nonblank reason is required")
def step_then_revocation_requires_reason(context):
    response = context.requests_response
    assert response.status_code == 400, (
        "Expected signature revocation without a nonblank reason to return 400, "
        f"got {response.status_code}: {response.text}"
    )
    assert "reason" in response.text.lower(), (
        "Expected the rejection to identify the mandatory revocation reason, "
        f"got: {response.text}"
    )


@then('the applied signature of contract "{name}" remains "{expected_status}"')
def step_then_signature_status_unchanged(context, name, expected_status):
    did, _, manager_headers = _signature_to_revoke(context, name)
    response = requests.get(
        signature_view_url(context),
        params={"did": did},
        headers=manager_headers,
        timeout=context.http_timeout_seconds,
    )
    assert response.status_code == 200, (
        f"Signature view failed for contract '{name}': "
        f"{response.status_code} {response.text}"
    )
    signatures = response.json().get("signatures") or []
    assert signatures, (
        f"Expected an applied signature on contract '{name}', got: {response.json()}"
    )
    actual_status = signatures[0].get("status")
    assert actual_status == expected_status, (
        f"Expected the signature of contract '{name}' to remain "
        f"{expected_status!r}, got {actual_status!r}"
    )


@then(
    'the "{event_type}" signature audit entry for contract "{name}" '
    'records exact reason "{reason}"'
)
def step_then_revoke_audit_records_exact_reason(
    context, event_type, name, reason
):
    did, _ = ContractService._contract_data(context, name)
    auditor_headers = AuthService.get_headers_for_roles(["Auditor"])
    deadline = time.monotonic() + 90
    entries = []
    while time.monotonic() < deadline:
        response = requests.get(
            signature_audit_url(context),
            params={"did": did},
            headers=auditor_headers,
            timeout=context.http_timeout_seconds,
        )
        assert response.status_code == 200, (
            f"Signature audit query failed for contract '{name}': "
            f"{response.status_code} {response.text}"
        )
        entries = [
            entry
            for entry in response.json()
            if str(entry.get("event_type", "")).upper() == event_type.upper()
        ]
        if entries:
            break
        time.sleep(1)

    assert entries, (
        f"Expected a '{event_type}' signature audit entry for contract '{name}'"
    )
    event_data = entries[-1].get("event_data")
    if isinstance(event_data, str):
        event_data = json.loads(event_data)
    event_data = event_data or {}
    assert event_data.get("reason") == reason, (
        f"Expected the '{event_type}' audit entry for contract '{name}' to "
        f"record exact reason {reason!r}, got: {event_data.get('reason')!r} "
        f"(event_data: {event_data})"
    )
