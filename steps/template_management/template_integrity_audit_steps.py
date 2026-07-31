"""BDD steps for template integrity verification and the template audit log
(DCS-FR-TR-20, DCS-FR-TR-21, DCS-FR-TR-05): POST /template/verify and
GET /template/audit (backend/design/template_repository.go), both already
implemented — this module only adds coverage that neither
template_workflow.feature nor template_archive.feature exercises.
"""

import json
import time

import requests
from behave import then

from steps.support.api_client import template_audit_url
from steps.support.services.auth_service import AuthService
from steps.support.services.template_service import TemplateService


@then("the template verification reports no findings")
def step_then_verification_no_findings(context):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    findings = body.get("findings")
    assert isinstance(findings, list) and len(findings) == 0, (
        f"Expected an approved template's integrity verification to report no findings, got: {findings}"
    )


@then('the template audit log for "{name}" includes an action of type "{event_type}"')
def step_then_template_audit_includes_action(context, name, event_type):
    # The audit trail is anchored asynchronously by the outbox processor
    # (~1s poll interval) — same polling convention as
    # contract_state_machine_steps.py's audit-event step.
    t = TemplateService.named(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    event_types = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = requests.get(
            template_audit_url(context),
            params={"did": t["did"]},
            headers=headers,
            timeout=context.http_timeout_seconds,
        )
        assert resp.status_code == 200, f"Template audit query failed for '{name}': {resp.status_code} {resp.text}"
        entries = resp.json()
        assert isinstance(entries, list), f"Expected a list of template audit entries, got: {entries}"
        event_types = [str(e.get("event_type", "")).upper() for e in entries]
        if event_type.upper() in event_types:
            return
        time.sleep(1)
    assert event_type.upper() in event_types, (
        f"Expected a '{event_type}' audit event for template '{name}', got event types: {event_types}"
    )


@then('the template audit log for "{name}" records the review comment "{comment}"')
def step_then_template_audit_records_review_comment(context, name, comment):
    # DCS-IR-TR-04: a reviewer's return-to-draft comments ride the
    # SUBMIT_CONTRACT_TEMPLATE event's payload (command/submit.go persists
    # cmd.Comments on the emitted SubmitEvent). Same outbox polling convention
    # as the action-type step above.
    t = TemplateService.named(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    recorded_comments = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = requests.get(
            template_audit_url(context),
            params={"did": t["did"]},
            headers=headers,
            timeout=context.http_timeout_seconds,
        )
        assert resp.status_code == 200, f"Template audit query failed for '{name}': {resp.status_code} {resp.text}"
        entries = resp.json()
        assert isinstance(entries, list), f"Expected a list of template audit entries, got: {entries}"
        recorded_comments = []
        for entry in entries:
            if str(entry.get("event_type", "")).upper() != "SUBMIT_CONTRACT_TEMPLATE":
                continue
            data = entry.get("event_data") or {}
            if isinstance(data, str):
                try:
                    data = json.loads(data)
                except ValueError:
                    continue
            recorded_comments.extend(data.get("comments") or [])
        if comment in recorded_comments:
            return
        time.sleep(1)
    assert comment in recorded_comments, (
        f"Expected review comment '{comment}' in the template audit log for '{name}', "
        f"got comments: {recorded_comments}"
    )
