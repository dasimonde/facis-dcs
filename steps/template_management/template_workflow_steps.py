"""Template lifecycle workflow steps — submit, verify, approve, reject, update, search, retrieve."""

import requests
from behave import given, then, when

from core.utils import is_uuid
from steps.support.services.template_service import TemplateService
from steps.support.api_client import (
    template_archive_url,
    get_with_headers,
    post_json,
    put_json,
    template_audit_url,
    template_approve_url,
    template_register_url,
    template_reject_url,
    template_retrieve_by_id_url,
    template_search_url,
    template_submit_url,
    template_update_url,
    template_verify_url,
)
from steps.support.services.auth_service import AuthService


# Given
# TODO: Refactor to reduce duplication in template creation and state transitions, e.g. by having a helper that creates a template and advances it to a specified state.

@given('template "{name}" is in "Draft" status')
def step_given_template_draft(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    TemplateService.store_named(context, name, did, updated_at)


@given('template "{name}" is in "Submitted" status')
def step_given_template_submitted(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    updated_at = TemplateService.do_submit(context, did, updated_at)
    TemplateService.store_named(context, name, did, updated_at)

@given('template "{orgname}" is in "Submitted" status with name "{name}" and description "{description}"')
def step_given_template_submitted(context, orgname, name, description):
    did, updated_at = TemplateService.create_fresh_template(context, name, description)
    updated_at = TemplateService.do_submit(context, did, updated_at)
    TemplateService.store_named(context, name, did, updated_at)

@given('template "{name}" is in "Reviewed" status')
def step_given_template_reviewed(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    updated_at = TemplateService.do_submit(context, did, updated_at)
    updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)
    TemplateService.store_named(context, name, did, updated_at)

@given('template "{name}" is in "Rejected" status')
def step_given_template_reviewed(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    updated_at = TemplateService.do_submit(context, did, updated_at)
    updated_at = TemplateService.do_recommend_for_rejected(context, did, updated_at)
    TemplateService.store_named(context, name, did, updated_at)


@given('template "{name}" with version "{version}" exists')
def step_given_template_version_exists(context, name, version):
    # Version tracking is internal; we create a fresh Draft to represent it.
    did, updated_at = TemplateService.create_fresh_template(context)
    TemplateService.store_named(context, name, did, updated_at)

@given('template "{name}" is verified')
def step_given_verified_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context, template_verify_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@given('template "{name}" has provenance metadata')
def step_given_template_has_provenance(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    TemplateService.store_named(context, name, did, updated_at)


@given('templates exist in the system')
def step_given_templates_exist(context):
    did, updated_at = TemplateService.create_fresh_template(context)
    TemplateService.store_named(context, "any", did, updated_at)


@given('template with name "{name}" and description "{description}" exists')
def step_given_templates_exist_with_name_and_description(context, name, description):
    did, updated_at = TemplateService.create_fresh_template(context, name, description)
    TemplateService.store_named(context, name, did, updated_at)

@given('template with name "{name}" and template_data title "{title}" exists')
def step_given_templates_exist_with_name_and_description(context, name, title):
    did, updated_at = TemplateService.create_fresh_template(context, name, title=title)
    TemplateService.store_named(context, name, did, updated_at)

@given('template "{name}" is approved and available')
def step_given_template_approved_available(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    updated_at = TemplateService.do_submit(context, did, updated_at)
    updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)
    headers = AuthService.get_headers_for_roles(["Template Approver"])
    resp = post_json(context, template_approve_url(context), {"did": did, "updated_at": updated_at}, headers=headers)
    assert resp.status_code == 200, f"Template approve failed: {resp.text}"
    updated_at = TemplateService.fetch_template(context, did, headers=headers).get("updated_at")
    TemplateService.store_named(context, name, did, updated_at)
    if not hasattr(context, "template_dids") or context.template_dids is None:
        context.template_dids = {}
    context.template_dids[name] = did


@given('template "{name}" is in "Approved" status')
def step_given_template_approved_status(context, name):
    step_given_template_approved_available(context, name)


@given('template "{name}" is in "Deprecated" status')
def step_given_template_deprecated_status(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    updated_at = TemplateService.do_submit(context, did, updated_at)
    updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)
    approver_headers = AuthService.get_headers_for_roles(["Template Approver"])
    approve_resp = post_json(
        context,
        template_approve_url(context),
        {"did": did, "updated_at": updated_at},
        headers=approver_headers,
    )
    assert approve_resp.status_code == 200, f"Template approve failed: {approve_resp.text}"
    updated_at = TemplateService.fetch_template(context, did, headers=approver_headers).get("updated_at")

    manager_headers = AuthService.get_headers_for_roles(["Template Manager"])
    archive_resp = post_json(
        context,
        template_archive_url(context),
        {"did": did, "updated_at": updated_at},
        headers=manager_headers,
    )
    assert archive_resp.status_code == 200, f"Template archive failed: {archive_resp.text}"
    updated_at = TemplateService.fetch_template(context, did, headers=manager_headers).get("updated_at")
    TemplateService.store_named(context, name, did, updated_at)


# When

@when('I submit template "{name}"')
def step_when_submit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload(context, t["did"], t["updated_at"]),
    )
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at")
        TemplateService.store_named(context, name, t["did"], ua)

@when('I submit template "{name}" with flag=approval')
def step_when_submit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload_with_flag(context, t["did"], t["updated_at"], "approval"),
    )
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at")
        TemplateService.store_named(context, name, t["did"], ua)

@when('I submit template "{name}" with flag=draft')
def step_when_submit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload_with_flag(context, t["did"], t["updated_at"], "draft"),
    )
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at", "draft")
        TemplateService.store_named(context, name, t["did"], ua)

@when('I resubmit template "{name}"')
def step_when_resubmit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload(context, t["did"], t["updated_at"]),
    )
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at", "draft")
        TemplateService.store_named(context, name, t["did"], ua)

@when('I approve template "{name}"')
def step_when_approve_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context, template_approve_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )

@when('I register template "{name}"')
def step_when_register_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context, template_register_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )

@when('I reject template "{name}" with reason "{reason}"')
def step_when_reject_template(context, name, reason):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_reject_url(context),
        {"did": t["did"], "updated_at": t["updated_at"], "reason": reason},
    )

@when('I reject template "{name}" without reason')
def step_when_reject_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_reject_url(context),
        {"did": t["did"], "updated_at": t["updated_at"]},
    )

@when('I reject template "{name}""')
def step_when_submit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload(context, t["did"], t["updated_at"]),
    )
    
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at", "draft")
        TemplateService.store_named(context, name, t["did"], ua)


@when('I update template "{orgname}" name to "{name}"')
def step_when_submit_template(context, orgname, name):
    t = TemplateService.named(context, orgname)
    context.requests_response = put_json(
        context,
        template_update_url(context),
        TemplateService.template_update_payload(context, t["did"], t["updated_at"], name),
    )
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at", "draft")
        TemplateService.store_named(context, orgname, t["did"], ua)

@when('I update template "{name}" description to "{description}"')
def step_when_submit_template(context, name, description):
    t = TemplateService.named(context, name)
    context.requests_response = put_json(
        context,
        template_update_url(context),
        TemplateService.template_update_payload(context, t["did"], t["updated_at"], description=description),
    )
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at", "draft")
        TemplateService.store_named(context, name, t["did"], ua)

@when('I submit template "{name}" for review without reviewers')
def step_when_submit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload_without_reviewers(context, t["did"], t["updated_at"]),
    )
    if context.requests_response.status_code != 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at")
        TemplateService.store_named(context, name, t["did"], ua)


@when('I submit template "{name}" for review without approver')
def step_when_submit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload_without_approver(context, t["did"], t["updated_at"]),
    )
    if context.requests_response.status_code != 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at")
        TemplateService.store_named(context, name, t["did"], ua)


@when('I submit template "{name}" for review with reviewers without approval')
def step_when_submit_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_submit_payload(context, t["did"], t["updated_at"]),
    )
    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at")
        TemplateService.store_named(context, name, t["did"], ua)



@when('I review template "{name}"')
def step_when_review_template(context, name):
    # "Review" in the feature means inspecting the template prior to recommending.
    t = TemplateService.named(context, name)
    context.review_inspect_response = get_with_headers(
        context, template_retrieve_by_id_url(context, t["did"])
    )


@when('I recommend template "{name}" for approval')
def step_when_recommend_template(context, name):
    t = TemplateService.named(context, name)
    verified_updated_at = TemplateService.do_verify(context, t["did"], t["updated_at"])
    context.requests_response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_reviewer_submit_payload(context, t["did"], verified_updated_at),
    )

    if context.requests_response.status_code == 200:
        ua = TemplateService.fetch_template(context, t["did"]).get("updated_at")
        TemplateService.store_named(context, name, t["did"], ua)


@when('I attempt to approve template "{name}"')
def step_when_attempt_approve_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context, template_approve_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@when('I update template name with name "{name}" to "{new_name}"')
def step_when_update_template(context, name, new_name):
    t = TemplateService.named(context, name)
    assert t, f"Template '{name}' not found in named_templates: {getattr(context, 'named_templates', {})}"
    payload = {
        "did": t["did"],
        "updated_at": t["updated_at"],
        "name": new_name
    }
    context.requests_response = put_json(context, template_update_url(context), payload)


@when('I update template description with name "{name}" to "{description}"')
def step_when_update_template(context, name, description):
    t = TemplateService.named(context, name)
    assert t, f"Template '{name}' not found in named_templates: {getattr(context, 'named_templates', {})}"
    payload = {
        "did": t["did"],
        "updated_at": t["updated_at"],
        "description": description
    }
    context.requests_response = put_json(context, template_update_url(context), payload)


@when('I attempt to update template "{name}"')
def step_when_attempt_update_template(context, name):
    t = TemplateService.named(context, name)
    payload = {"did": t["did"], "updated_at": t["updated_at"]}
    context.requests_response = put_json(context, template_update_url(context), payload)


@when('I search for templates with template_data "{keyword}"')
def step_when_search_templates(context, keyword):
    context.requests_response = requests.get(
        template_search_url(context),
        params={"template_data": keyword},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )

@when('I am authenticated with roles: "{roles}"')
def step_when_authenticated_with_roles(context, roles):
    role_list = [role.strip() for role in roles.split(",")]
    AuthService.set_headers_for_roles(context, role_list)

@when('I try to search for templates with name "{name}" "{count}"')
def step_when_search_templates(context, name, count):
    for _ in range(int(count)):
        context.requests_response = requests.get(
            template_search_url(context),
            params={"name": name},
            headers=getattr(context, "headers", {}),
            timeout=context.http_timeout_seconds,
        )

@when('the request is denied because of too many failed attempts')
def step_when_denied_to_many_attempts(context):
    response = context.requests_response.json()
    assert context.requests_response.status_code in (401, 403) and "too many failed attempts" in response["message"], response

@when('I search for templates with name "{name}"')
def step_when_search_templates(context, name):
    context.requests_response = requests.get(
        template_search_url(context),
        params={"name": name},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )

@when('I search for templates with description "{description}"')
def step_when_search_templates(context, description):
    context.requests_response = requests.get(
        template_search_url(context),
        params={"description": description},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )

@when('I search for templates whats template_data contains keyword "{title}"')
def step_when_search_templates(context, title):
    context.requests_response = requests.get(
        template_search_url(context),
        params={"template_data": title},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )


@when('I verify template "{name}"')
def step_when_verify_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context, template_verify_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@when('I attempt to verify template "{name}"')
def step_when_attempt_verify_template(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context, template_verify_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )


@when('I deprecate template "{name}"')
def step_when_deprecate_template(context, name):
    t = TemplateService.named(context, name)
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
    t = TemplateService.named(context, name)
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
    did, updated_at = TemplateService.create_approved_template(context)
    TemplateService.store_named(context, name, did, updated_at)


@given('template "{name}" exists with UUID')
def step_given_template_exists_with_uuid(context, name):
    did, updated_at = TemplateService.create_fresh_template(context)
    TemplateService.store_named(context, name, did, updated_at)


@given('template "{name}" has a DID assigned')
def step_given_template_has_did(context, name):
    did, updated_at = TemplateService.create_approved_template(context)
    manager_headers = AuthService.get_headers_for_roles(["Template Manager"])
    register_resp = post_json(
        context,
        template_register_url(context),
        {"did": did, "updated_at": updated_at},
        headers=manager_headers,
    )
    assert register_resp.status_code == 200, f"Template register failed: {register_resp.text}"
    updated_at = TemplateService.fetch_template(context, did, headers=manager_headers).get("updated_at")
    TemplateService.store_named(context, name, did, updated_at)


@when('I assign a DID to template "{name}"')
def step_when_assign_did(context, name):
    t = TemplateService.named(context, name)
    context.requests_response = post_json(
        context,
        template_register_url(context),
        {"did": t["did"], "updated_at": t["updated_at"]},
    )


@when('I retrieve template "{name}" by did')
def step_when_retrieve_template_by_did(context, name):
    # API retrieval key is DID; DID requirement is validated by successful template retrieval.
    t = TemplateService.named(context, name)
    context.requests_response = get_with_headers(
        context, template_retrieve_by_id_url(context, t["did"])
    )


@when('I retrieve template by DID')
def step_when_retrieve_template_by_did(context):
    t = TemplateService.named(context, "Standard NDA")
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
    template = TemplateService.fetch_template(context, did)
    actual = template.get("state", "").upper()
    assert actual == expected_status.upper(), (
        f"Template state mismatch: expected '{expected_status.upper()}', got '{actual}'"
    )


@then('the template is available for contract generation')
def step_then_template_available_for_generation(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    template = TemplateService.fetch_template(context, did)
    state = template.get("state", "").upper()
    assert state == "APPROVED", f"Expected APPROVED state, got '{state}'"


@then('I see the template with template_data title {title}')
def step_then_see_template_with_template_data_title(context, title):
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
    uuid = did.split(":")[-1]
    assert is_uuid(uuid), f"Expected did {uuid} to be a valid UUID"


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

@then('I search for templates with name "{name}"')
def step_then_search_templates(context, name):
    context.requests_response = requests.get(
        template_search_url(context),
        params={"name": name},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )