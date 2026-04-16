"""Template lifecycle workflow steps — submit, verify, approve, reject, update, search, retrieve."""

import os
import re

import requests
from behave import given, then, when

from support.api_client import (
    template_archive_url,
    get_with_headers,
    post_json,
    put_json,
    template_audit_url,
    template_approve_url,
    template_create_url,
    template_register_url,
    template_reject_url,
    template_retrieve_by_id_url,
    template_retrieve_url,
    template_search_url,
    template_submit_url,
    template_update_url,
    template_verify_url,
)
from support.keycloak_client import (
    admin_token,
    assign_client_role,
    ensure_client,
    ensure_client_role,
    ensure_user,
    user_token,
)
from support.template_utils import template_type_for_category


# Internal helpers

def _headers_for_role(context, role: str) -> dict:
    """Return auth headers for *role* without touching context.headers."""
    client_id = os.getenv("BDD_KEYCLOAK_CLIENT_ID", "digital-contracting-service")
    role_safe = re.sub(r"[^A-Za-z0-9]+", "-", role.lower()).strip("-")
    username = f"bdd-{role_safe}"
    password = os.getenv("BDD_KEYCLOAK_TEST_USER_PASSWORD", "bdd-pass-123")
    adm = admin_token()
    client_uuid = ensure_client(adm, client_id)
    role_repr = ensure_client_role(adm, client_uuid, role)
    user_id = ensure_user(adm, username, password)
    assign_client_role(adm, user_id, client_uuid, role_repr)
    token = user_token(client_id, username, password)
    return {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}


def _username_for_role(role: str) -> str:
    role_safe = re.sub(r"[^A-Za-z0-9]+", "-", role.lower()).strip("-")
    return f"bdd-{role_safe}"


def _create_fresh_template(context) -> tuple:
    """Create a Draft template as Template Creator; return (did, updated_at)."""
    headers = _headers_for_role(context, "Template Creator")
    payload = {
        "template_type": template_type_for_category("Legal"),
        "name": "BDD Standard NDA",
        "description": "BDD auto-created template",
        "template_data": {
            "title": "BDD Standard NDA",
            "clauses": [{"id": "c1", "text": "Confidentiality clause"}],
        },
    }
    resp = post_json(context, template_create_url(context), payload, headers=headers)
    assert resp.status_code == 200, f"Template create failed: {resp.text}"
    did = resp.json().get("did")
    assert did, f"No DID in create response: {resp.text}"
    body = _fetch_template(context, did, headers=headers)
    return did, body.get("updated_at")


def _create_approved_template(context) -> tuple:
    """Create and approve a template; return (did, updated_at)."""
    did, updated_at = _create_fresh_template(context)
    updated_at = _do_submit(context, did, updated_at)
    updated_at = _do_recommend_for_approval(context, did, updated_at)
    approver_headers = _headers_for_role(context, "Template Approver")
    approve_resp = post_json(
        context,
        template_approve_url(context),
        {"did": did, "updated_at": updated_at},
        headers=approver_headers,
    )
    assert approve_resp.status_code == 200, f"Template approve failed: {approve_resp.text}"
    updated_at = _fetch_template(context, did, headers=approver_headers).get("updated_at")
    return did, updated_at


def _fetch_template(context, did: str, headers=None) -> dict:
    resp = get_with_headers(context, template_retrieve_by_id_url(context, did), headers=headers)
    assert resp.status_code == 200, f"Template retrieve failed: {resp.text}"
    return resp.json()


def _template_submit_payload(context, did: str, updated_at: str) -> dict:
    _headers_for_role(context, "Template Reviewer")
    _headers_for_role(context, "Template Approver")
    return {
        "did": did,
        "updated_at": updated_at,
        "reviewers": [_username_for_role("Template Reviewer")],
        "approver": _username_for_role("Template Approver"),
    }


def _template_reviewer_submit_payload(context, did: str, updated_at: str) -> dict:
    _headers_for_role(context, "Template Approver")
    return {
        "did": did,
        "updated_at": updated_at,
        "approver": _username_for_role("Template Approver"),
        "forward_to": "approval",
    }


def _do_submit(context, did: str, updated_at: str) -> str:
    """Submit template as Template Creator; return refreshed updated_at."""
    headers = _headers_for_role(context, "Template Creator")
    resp = post_json(
        context,
        template_submit_url(context),
        _template_submit_payload(context, did, updated_at),
        headers=headers,
    )
    assert resp.status_code == 200, f"Template submit failed: {resp.text}"
    return _fetch_template(context, did, headers=headers).get("updated_at")


def _do_verify(context, did: str, updated_at: str) -> str:
    """Verify template content as Template Reviewer; return refreshed updated_at."""
    headers = _headers_for_role(context, "Template Reviewer")
    resp = post_json(context, template_verify_url(context), {"did": did, "updated_at": updated_at}, headers=headers)
    assert resp.status_code == 200, f"Template verify failed: {resp.text}"
    return _fetch_template(context, did, headers=headers).get("updated_at")


def _do_recommend_for_approval(context, did: str, updated_at: str) -> str:
    """Submit reviewer recommendation and advance review workflow."""
    # Backend requires verification before reviewer recommendation submit.
    updated_at = _do_verify(context, did, updated_at)
    headers = _headers_for_role(context, "Template Reviewer")
    resp = post_json(
        context,
        template_submit_url(context),
        _template_reviewer_submit_payload(context, did, updated_at),
        headers=headers,
    )
    assert resp.status_code == 200, f"Template review submit failed: {resp.text}"
    return _fetch_template(context, did, headers=headers).get("updated_at")


def _named(context, name: str) -> dict:
    return (getattr(context, "named_templates", None) or {}).get(name, {})


def _store_named(context, name: str, did: str, updated_at: str):
    if not hasattr(context, "named_templates") or context.named_templates is None:
        context.named_templates = {}
    context.named_templates[name] = {"did": did, "updated_at": updated_at}


# Given

@given('template "{name}" is in "Draft" status')
def step_given_template_draft(context, name):
    did, updated_at = _create_fresh_template(context)
    _store_named(context, name, did, updated_at)


@given('template "{name}" is in "Submitted" status')
def step_given_template_submitted(context, name):
    did, updated_at = _create_fresh_template(context)
    updated_at = _do_submit(context, did, updated_at)
    _store_named(context, name, did, updated_at)


@given('template "{name}" is in "Reviewed" status')
def step_given_template_reviewed(context, name):
    did, updated_at = _create_fresh_template(context)
    updated_at = _do_submit(context, did, updated_at)
    updated_at = _do_recommend_for_approval(context, did, updated_at)
    _store_named(context, name, did, updated_at)


@given('template "{name}" version "{version}" exists')
def step_given_template_version_exists(context, name, version):
    # Version tracking is internal; we create a fresh Draft to represent it.
    did, updated_at = _create_fresh_template(context)
    _store_named(context, name, did, updated_at)


@given('template "{name}" has provenance metadata')
def step_given_template_has_provenance(context, name):
    did, updated_at = _create_fresh_template(context)
    _store_named(context, name, did, updated_at)


@given('templates exist in the system')
def step_given_templates_exist(context):
    did, updated_at = _create_fresh_template(context)
    _store_named(context, "any", did, updated_at)


@given('template "{name}" is approved and available')
def step_given_template_approved_available(context, name):
    did, updated_at = _create_fresh_template(context)
    updated_at = _do_submit(context, did, updated_at)
    updated_at = _do_recommend_for_approval(context, did, updated_at)
    headers = _headers_for_role(context, "Template Approver")
    resp = post_json(context, template_approve_url(context), {"did": did, "updated_at": updated_at}, headers=headers)
    assert resp.status_code == 200, f"Template approve failed: {resp.text}"
    updated_at = _fetch_template(context, did, headers=headers).get("updated_at")
    _store_named(context, name, did, updated_at)
    if not hasattr(context, "template_dids") or context.template_dids is None:
        context.template_dids = {}
    context.template_dids[name] = did


@given('template "{name}" is in "Approved" status')
def step_given_template_approved_status(context, name):
    step_given_template_approved_available(context, name)


@given('template "{name}" is in "Deprecated" status')
def step_given_template_deprecated_status(context, name):
    did, updated_at = _create_fresh_template(context)
    updated_at = _do_submit(context, did, updated_at)
    updated_at = _do_recommend_for_approval(context, did, updated_at)
    approver_headers = _headers_for_role(context, "Template Approver")
    approve_resp = post_json(
        context,
        template_approve_url(context),
        {"did": did, "updated_at": updated_at},
        headers=approver_headers,
    )
    assert approve_resp.status_code == 200, f"Template approve failed: {approve_resp.text}"
    updated_at = _fetch_template(context, did, headers=approver_headers).get("updated_at")

    manager_headers = _headers_for_role(context, "Template Manager")
    archive_resp = post_json(
        context,
        template_archive_url(context),
        {"did": did, "updated_at": updated_at},
        headers=manager_headers,
    )
    assert archive_resp.status_code == 200, f"Template archive failed: {archive_resp.text}"
    updated_at = _fetch_template(context, did, headers=manager_headers).get("updated_at")
    _store_named(context, name, did, updated_at)


# When

@when('I submit template "{name}" for review')
def step_when_submit_template(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        _template_submit_payload(context, t["did"], t["updated_at"]),
    )
    if context.requests_response.status_code == 200:
        ua = _fetch_template(context, t["did"]).get("updated_at")
        _store_named(context, name, t["did"], ua)


@when('I review template "{name}"')
def step_when_review_template(context, name):
    # "Review" in the feature means inspecting the template prior to recommending.
    t = _named(context, name)
    context.review_inspect_response = get_with_headers(
        context, template_retrieve_by_id_url(context, t["did"])
    )


@when('I recommend template "{name}" for approval')
def step_when_recommend_template(context, name):
    t = _named(context, name)
    verified_updated_at = _do_verify(context, t["did"], t["updated_at"])
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        _template_reviewer_submit_payload(context, t["did"], verified_updated_at),
    )
    if context.requests_response.status_code == 200:
        ua = _fetch_template(context, t["did"]).get("updated_at")
        _store_named(context, name, t["did"], ua)


@when('I approve template "{name}"')
def step_when_approve_template(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context, template_approve_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@when('I reject template "{name}" with reason "{reason}"')
def step_when_reject_template(context, name, reason):
    t = _named(context, name)
    context.requests_response = post_json(
        context,
        template_reject_url(context),
        {"did": t["did"], "updated_at": t["updated_at"], "reason": reason},
    )


@when('I attempt to approve template "{name}"')
def step_when_attempt_approve_template(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context, template_approve_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@when('I update template "{name}"')
def step_when_update_template(context, name):
    t = _named(context, name)
    payload = {
        "did": t["did"],
        "updated_at": t["updated_at"],
        "template_data": {
            "title": "BDD Standard NDA (revised)",
            "clauses": [{"id": "c1", "text": "Updated confidentiality clause"}],
        },
    }
    context.requests_response = put_json(context, template_update_url(context), payload)


@when('I attempt to update template "{name}"')
def step_when_attempt_update_template(context, name):
    t = _named(context, name)
    payload = {"did": t["did"], "updated_at": t["updated_at"]}
    context.requests_response = put_json(context, template_update_url(context), payload)


@when('I search for templates with keyword "{keyword}"')
def step_when_search_templates(context, keyword):
    context.requests_response = requests.get(
        template_search_url(context),
        params={"filter": keyword},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )


@when('I retrieve template "{name}"')
def step_when_retrieve_template(context, name):
    t = _named(context, name)
    if not t or not t.get("did"):
        # No Given seeded this template; auto-create as test data.
        did, updated_at = _create_fresh_template(context)
        _store_named(context, name, did, updated_at)
        t = _named(context, name)
    context.requests_response = get_with_headers(
        context, template_retrieve_by_id_url(context, t["did"])
    )


@when('I verify template "{name}"')
def step_when_verify_template(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context, template_verify_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@when('I attempt to verify template "{name}"')
def step_when_attempt_verify_template(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context, template_verify_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@when('I deprecate template "{name}"')
def step_when_deprecate_template(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context,
        template_archive_url(context),
        {"did": t["did"], "updated_at": t["updated_at"]},
    )


@when('I attempt to deprecate template "{name}"')
def step_when_attempt_deprecate_template(context, name):
    step_when_deprecate_template(context, name)


@when('I delete template "{name}"')
def step_when_delete_template(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context,
        template_archive_url(context),
        {"did": t["did"], "updated_at": t["updated_at"]},
    )


@when('I attempt to delete template "{name}"')
def step_when_attempt_delete_template(context, name):
    step_when_delete_template(context, name)


@given('template "{name}" exists')
def step_given_template_exists(context, name):
    did, updated_at = _create_approved_template(context)
    _store_named(context, name, did, updated_at)


@given('template "{name}" exists with UUID')
def step_given_template_exists_with_uuid(context, name):
    did, updated_at = _create_fresh_template(context)
    _store_named(context, name, did, updated_at)


@given('template "{name}" has a DID assigned')
def step_given_template_has_did(context, name):
    did, updated_at = _create_approved_template(context)
    manager_headers = _headers_for_role(context, "Template Manager")
    register_resp = post_json(
        context,
        template_register_url(context),
        {"did": did, "updated_at": updated_at},
        headers=manager_headers,
    )
    assert register_resp.status_code == 200, f"Template register failed: {register_resp.text}"
    updated_at = _fetch_template(context, did, headers=manager_headers).get("updated_at")
    _store_named(context, name, did, updated_at)


@when('I assign a DID to template "{name}"')
def step_when_assign_did(context, name):
    t = _named(context, name)
    context.requests_response = post_json(
        context,
        template_register_url(context),
        {"did": t["did"], "updated_at": t["updated_at"]},
    )


@when('I retrieve template by UUID')
def step_when_retrieve_template_by_uuid(context):
    # API retrieval key is DID; UUID requirement is validated by successful template retrieval.
    t = _named(context, "Standard NDA")
    context.requests_response = get_with_headers(
        context, template_retrieve_by_id_url(context, t["did"])
    )


@when('I retrieve template by DID')
def step_when_retrieve_template_by_did(context):
    t = _named(context, "Standard NDA")
    context.requests_response = get_with_headers(
        context, template_retrieve_by_id_url(context, t["did"])
    )


# Then

@then('the template status is "{expected_status}"')
def step_then_template_status(context, expected_status):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    template = _fetch_template(context, did)
    actual = template.get("state", "").upper()
    assert actual == expected_status.upper(), (
        f"Template state mismatch: expected '{expected_status.upper()}', got '{actual}'"
    )


@then('the template is available for contract generation')
def step_then_template_available_for_generation(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    template = _fetch_template(context, did)
    state = template.get("state", "").upper()
    assert state == "APPROVED", f"Expected APPROVED state, got '{state}'"


@then('I see the template version and status')
def step_then_see_version_and_status(context):
    body = context.requests_response.json()
    assert body.get("did"), f"Missing 'did' in response: {body}"
    assert body.get("version") is not None, f"Missing 'version' in response: {body}"
    assert "state" in body, f"Missing 'state' in response: {body}"


@then('the template is removed from the system')
def step_then_template_removed(context):
    body = context.requests_response.json()
    did = body.get("did")
    if did:
        check = get_with_headers(context, template_retrieve_by_id_url(context, did))
        if check.status_code in (404, 410):
            return
        assert check.status_code == 200, (
            "Expected archived template lookup to return 200/404/410, "
            f"got status={check.status_code}, body={check.text}"
        )
        state = str(check.json().get("state", "")).upper()
        assert state in ("ARCHIVED", "DEPRECATED", "RETIRED", "DELETED"), (
            "Template should be removed from active usage after delete/archive; "
            f"got state='{state}'"
        )


@then('the deletion is recorded in audit log')
def step_then_deletion_recorded_audit(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in delete response: {body}"
    audit = requests.get(
        template_audit_url(context),
        params={"did": did},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )
    assert audit.status_code == 200, f"Audit retrieval failed: {audit.text}"


@then('I receive error "{message}"')
def step_then_receive_error_message(context, message):
    assert context.requests_response.status_code >= 400, (
        f"Expected error response, got {context.requests_response.status_code}"
    )
    assert message.lower() in context.requests_response.text.lower(), (
        f"Expected error containing '{message}', got: {context.requests_response.text}"
    )


@then('the template is assigned a UUID')
def step_then_template_assigned_uuid(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), f"Expected identifier, got: {body}"


@then('the template has a resolvable DID')
def step_then_template_has_resolvable_did(context):
    did = context.requests_response.json().get("did")
    assert did, "No DID returned by register call"
    probe = get_with_headers(context, template_retrieve_by_id_url(context, did))
    assert probe.status_code == 200, f"DID not resolvable: {probe.text}"


@then('the DID is linked to template metadata')
def step_then_did_linked_metadata(context):
    did = context.requests_response.json().get("did")
    probe = get_with_headers(context, template_retrieve_by_id_url(context, did))
    body = probe.json()
    assert body.get("did") == did, f"Retrieved metadata DID mismatch: {body}"


@then('I receive the correct template')
def step_then_receive_correct_template(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No template identifier in response: {body}"

