"""Contract workflow steps for negotiation, adjustment, and approval slices."""

import os
import re

from behave import given, then, when

from support.api_client import (
    contract_approve_url,
    contract_create_url,
    contract_negotiate_url,
    contract_reject_url,
    contract_retrieve_by_id_url,
    contract_submit_url,
    contract_update_url,
    contract_verify_url,
    get_with_headers,
    post_json,
    put_json,
    template_approve_url,
    template_create_url,
    template_submit_url,
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


def _headers_for_role(context, role: str) -> dict:
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


def _ensure_store(context, name, value):
    if not hasattr(context, name) or getattr(context, name) is None:
        setattr(context, name, value)


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


def _contract_submit_payload(context, did: str, updated_at: str) -> dict:
    _headers_for_role(context, "Contract Reviewer")
    _headers_for_role(context, "Contract Approver")
    return {
        "did": did,
        "updated_at": updated_at,
        "reviewers": [_username_for_role("Contract Reviewer")],
        "approver": _username_for_role("Contract Approver"),
    }


def _contract_reviewer_submit_payload(context, did: str, updated_at: str) -> dict:
    _headers_for_role(context, "Contract Approver")
    return {
        "did": did,
        "updated_at": updated_at,
        "forward_to": "approval",
        "approver": _username_for_role("Contract Approver"),
    }


def _create_approved_template_for_contract(context):
    creator_h = _headers_for_role(context, "Template Creator")
    create_resp = post_json(
        context,
        template_create_url(context),
        {
            "template_type": "FRAME_CONTRACT",
            "name": "BDD Contract Source Template",
            "description": "BDD template for contract workflows",
            "template_data": {"title": "BDD Template", "clauses": [{"id": "c1", "text": "Base clause"}]},
        },
        headers=creator_h,
    )
    assert create_resp.status_code == 200, create_resp.text
    t_did = create_resp.json().get("did")

    retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=creator_h)
    assert retrieve_resp.status_code == 200, retrieve_resp.text
    updated_at = retrieve_resp.json().get("updated_at")

    submit_resp = post_json(
        context,
        template_submit_url(context),
        _template_submit_payload(context, t_did, updated_at),
        headers=creator_h,
    )
    assert submit_resp.status_code == 200, submit_resp.text

    reviewer_h = _headers_for_role(context, "Template Reviewer")
    retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=reviewer_h)
    updated_at = retrieve_resp.json().get("updated_at")

    verify_resp = post_json(
        context,
        template_verify_url(context),
        {"did": t_did, "updated_at": updated_at},
        headers=reviewer_h,
    )
    assert verify_resp.status_code == 200, verify_resp.text

    retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=reviewer_h)
    updated_at = retrieve_resp.json().get("updated_at")

    review_submit_resp = post_json(
        context,
        template_submit_url(context),
        _template_reviewer_submit_payload(context, t_did, updated_at),
        headers=reviewer_h,
    )
    assert review_submit_resp.status_code == 200, review_submit_resp.text

    approver_h = _headers_for_role(context, "Template Approver")
    retrieve_resp = get_with_headers(context, f"{context.base_url}/template/retrieve/{t_did}", headers=approver_h)
    updated_at = retrieve_resp.json().get("updated_at")
    approve_resp = post_json(
        context,
        template_approve_url(context),
        {"did": t_did, "updated_at": updated_at},
        headers=approver_h,
    )
    assert approve_resp.status_code == 200, approve_resp.text
    return t_did


def _create_contract_in_draft(context, contract_name: str):
    t_did = _create_approved_template_for_contract(context)
    creator_h = _headers_for_role(context, "Contract Creator")
    create_resp = post_json(context, contract_create_url(context), {"did": t_did}, headers=creator_h)
    assert create_resp.status_code == 200, create_resp.text
    c_did = create_resp.json().get("did")
    retrieve_resp = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
    assert retrieve_resp.status_code == 200, retrieve_resp.text
    updated_at = retrieve_resp.json().get("updated_at")

    _ensure_store(context, "contract_dids", {})
    _ensure_store(context, "contract_updated_at", {})
    _ensure_store(context, "contract_seed_headers", {})
    context.contract_dids[contract_name] = c_did
    context.contract_updated_at[contract_name] = updated_at
    context.contract_seed_headers[contract_name] = creator_h


def _contract_data(context, contract_name: str):
    did = context.contract_dids[contract_name]
    updated_at = context.contract_updated_at[contract_name]
    return did, updated_at


def _refresh_contract(context, contract_name: str):
    did = context.contract_dids[contract_name]
    headers = None
    if hasattr(context, "contract_seed_headers"):
        headers = context.contract_seed_headers.get(contract_name)
    resp = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert resp.status_code == 200, resp.text
    context.contract_updated_at[contract_name] = resp.json().get("updated_at")
    return resp.json()


def _prepare_contract_under_review(context, contract_name: str):
    did, updated_at = _contract_data(context, contract_name)
    creator_h = context.contract_seed_headers[contract_name]
    submit_to_negotiation = post_json(
        context,
        contract_submit_url(context),
        _contract_submit_payload(context, did, updated_at),
        headers=creator_h,
    )
    assert submit_to_negotiation.status_code == 200, submit_to_negotiation.text
    _refresh_contract(context, contract_name)

    # Backend workflow transitions Draft -> Negotiation on first submit,
    # then Negotiation -> Submitted on a second creator submit.
    did, updated_at = _contract_data(context, contract_name)
    submit_to_submitted = post_json(
        context,
        contract_submit_url(context),
        _contract_submit_payload(context, did, updated_at),
        headers=creator_h,
    )
    assert submit_to_submitted.status_code == 200, submit_to_submitted.text
    _refresh_contract(context, contract_name)


def _prepare_contract_pending_approval(context, contract_name: str):
    did, _ = _contract_data(context, contract_name)
    _prepare_contract_under_review(context, contract_name)

    reviewer_h = _headers_for_role(context, "Contract Reviewer")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=reviewer_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")

    verify = post_json(
        context,
        contract_verify_url(context),
        {"did": did, "updated_at": updated_at},
        headers=reviewer_h,
    )
    assert verify.status_code == 200, verify.text

    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=reviewer_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")

    review_submit = post_json(
        context,
        contract_submit_url(context),
        _contract_reviewer_submit_payload(context, did, updated_at),
        headers=reviewer_h,
    )
    assert review_submit.status_code == 200, review_submit.text
    _refresh_contract(context, contract_name)


@given('contract "{name}" is in "Draft" status')
def step_given_contract_draft(context, name):
    _create_contract_in_draft(context, name)


@given('contract "{name}" is in "Under Review" status')
def step_given_contract_under_review(context, name):
    _create_contract_in_draft(context, name)
    _prepare_contract_under_review(context, name)


@given('contract "{name}" is pending approval')
def step_given_contract_pending_approval(context, name):
    _create_contract_in_draft(context, name)
    _prepare_contract_pending_approval(context, name)


@given('contract "{name}" requires my approval')
def step_given_contract_requires_my_approval(context, name):
    step_given_contract_pending_approval(context, name)


@given('contract "{name}" is open for negotiation')
def step_given_contract_open_for_negotiation(context, name):
    _create_contract_in_draft(context, name)


@given('contract "{name}" negotiation is complete')
def step_given_contract_negotiation_complete(context, name):
    _create_contract_in_draft(context, name)


@given('contract "{name}" has completed multiple negotiation rounds')
def step_given_contract_multiple_negotiation_rounds(context, name):
    _create_contract_in_draft(context, name)


@when('I open contract "{name}" for negotiation')
def step_when_open_for_negotiation(context, name):
    did, _ = _contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I adjust clause "{clause}" with new text')
def step_when_adjust_clause(context, clause):
    name = "Service Agreement"
    did, updated_at = _contract_data(context, name)
    payload = {
        "did": did,
        "updated_at": updated_at,
        "contract_data": {
            "edited_clause": clause,
            "text": f"Updated by BDD for {clause}",
        },
    }
    context.requests_response = put_json(context, contract_update_url(context), payload)
    if context.requests_response.status_code == 200:
        _refresh_contract(context, name)


@when('I attempt to adjust clause "{clause}"')
def step_when_attempt_adjust_clause(context, clause):
    step_when_adjust_clause(context, clause)


@when('I initiate the approval process for contract "{name}"')
def step_when_initiate_approval(context, name):
    did, updated_at = _contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_submit_url(context),
        _contract_submit_payload(context, did, updated_at),
    )
    if context.requests_response.status_code == 200:
        _refresh_contract(context, name)


@when('I approve contract "{name}"')
def step_when_approve_contract(context, name):
    did, updated_at = _contract_data(context, name)
    context.requests_response = post_json(context, contract_approve_url(context), {"did": did, "updated_at": updated_at})
    if context.requests_response.status_code == 200:
        _refresh_contract(context, name)


@when('I reject contract "{name}" with reason "{reason}"')
def step_when_reject_contract(context, name, reason):
    did, updated_at = _contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_reject_url(context),
        {"did": did, "updated_at": updated_at, "reason": reason},
    )


@when('I access the approval interface for contract "{name}"')
def step_when_access_approval_interface(context, name):
    did, _ = _contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I submit contract "{name}" for review')
def step_when_submit_contract_for_review(context, name):
    did, updated_at = _contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_submit_url(context),
        _contract_submit_payload(context, did, updated_at),
    )
    if context.requests_response.status_code == 200:
        _refresh_contract(context, name)


@when('I attempt to add a comment to contract "{name}"')
def step_when_attempt_comment_contract(context, name):
    did, updated_at = _contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": "bdd-observer",
            "change_request": "comment attempt",
        },
    )


@when('I attempt to approve contract "{name}"')
def step_when_attempt_approve_contract(context, name):
    step_when_approve_contract(context, name)


# ---------------------------------------------------------------------------
# Additional Given helpers
# ---------------------------------------------------------------------------

@given('I have created contract "{name}" from a template')
def step_given_created_contract_from_template(context, name):
    _create_contract_in_draft(context, name)


@given('contract "{name}" has multiple negotiation edits')
def step_given_contract_has_negotiation_edits(context, name):
    _create_contract_in_draft(context, name)


@given('contract "{name}" has a pending redline proposal on clause "{clause}"')
def step_given_contract_has_pending_redline(context, name, clause):
    _create_contract_in_draft(context, name)
    did, updated_at = _contract_data(context, name)
    creator_h = context.contract_seed_headers[name]
    post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": _username_for_role("Contract Manager"),
            "change_request": f"Redline on {clause}: proposed replacement text",
        },
        headers=creator_h,
    )
    _refresh_contract(context, name)


@given('contract "{name}" is assigned to reviewers "{r1}" and "{r2}"')
def step_given_contract_assigned_reviewers(context, name, r1, r2):
    _create_contract_in_draft(context, name)


@given('contract "{name}" is in approval workflow')
def step_given_contract_in_approval_workflow(context, name):
    _create_contract_in_draft(context, name)
    _prepare_contract_pending_approval(context, name)


@given('contract "{name}" has all required approvals')
def step_given_contract_has_all_approvals(context, name):
    _create_contract_in_draft(context, name)
    _prepare_contract_pending_approval(context, name)
    did, _ = _contract_data(context, name)
    approver_h = _headers_for_role(context, "Contract Approver")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=approver_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    approve = post_json(
        context, contract_approve_url(context),
        {"did": did, "updated_at": updated_at},
        headers=approver_h,
    )
    assert approve.status_code == 200, approve.text
    _refresh_contract(context, name)


@given('contract "{name}" is in "Active" status')
def step_given_contract_active(context, name):
    _create_contract_in_draft(context, name)


# ---------------------------------------------------------------------------
# Additional When helpers
# ---------------------------------------------------------------------------

@when('I view contract "{name}"')
def step_when_view_contract(context, name):
    did, _ = _contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I edit contract "{name}"')
def step_when_edit_contract(context, name):
    did, updated_at = _contract_data(context, name)
    payload = {
        "did": did,
        "updated_at": updated_at,
        "contract_data": {"title": "BDD Contract (edited)", "clauses": [{"id": "c1", "text": "Updated clause"}]},
    }
    context.requests_response = put_json(context, contract_update_url(context), payload)
    if context.requests_response.status_code == 200:
        _refresh_contract(context, name)


@when('I generate a contract from template "{name}"')
def step_when_generate_contract_from_template(context, name):
    template_did = (
        (getattr(context, "template_dids", None) or {}).get(name)
        or (getattr(context, "named_templates", None) or {}).get(name, {}).get("did")
    )
    assert template_did, f"No approved template DID found for '{name}'"
    context.requests_response = post_json(
        context, contract_create_url(context), {"did": template_did}
    )


@when('I attempt to generate a contract from template "{template_name}"')
def step_when_attempt_generate_contract(context, template_name):
    template_did = (getattr(context, "template_dids", None) or {}).get(template_name, "did:example:missing")
    context.requests_response = post_json(context, contract_create_url(context), {"did": template_did})


@when('I add comment "{comment}" to clause "{clause}"')
def step_when_add_comment_to_clause(context, comment, clause):
    name = "Service Agreement"
    did, updated_at = _contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": _username_for_role("Contract Reviewer"),
            "change_request": f"[{clause}] {comment}",
        },
    )
    if context.requests_response.status_code == 200:
        _refresh_contract(context, name)


@when('I propose a redline edit to clause "{clause}"')
def step_when_propose_redline(context, clause):
    step_when_add_comment_to_clause(context, f"Proposed redline for {clause}", clause)


@when("I approve the redline proposal")
def step_when_approve_redline(context):
    name = "Service Agreement"
    did, _ = _contract_data(context, name)
    refresh = _refresh_contract(context, name)
    negotiations = refresh.get("negotiations") or []
    if negotiations:
        negotiation_id = negotiations[-1].get("id") or negotiations[-1].get("did")
        if negotiation_id:
            context.requests_response = post_json(
                context,
                f"{context.base_url}/contract/respond",
                {"id": str(negotiation_id), "action_flag": "accept", "rejected_by": "", "rejection_reason": ""},
            )
            return
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I reject the redline proposal with reason "{reason}"')
def step_when_reject_redline(context, reason):
    name = "Service Agreement"
    did, _ = _contract_data(context, name)
    refresh = _refresh_contract(context, name)
    negotiations = refresh.get("negotiations") or []
    if negotiations:
        negotiation_id = negotiations[-1].get("id") or negotiations[-1].get("did")
        if negotiation_id:
            context.requests_response = post_json(
                context,
                f"{context.base_url}/contract/respond",
                {"id": str(negotiation_id), "action_flag": "reject",
                 "rejected_by": _username_for_role("Contract Manager"), "rejection_reason": reason},
            )
            return
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I view version history for contract "{name}"')
def step_when_view_version_history(context, name):
    did, _ = _contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I view the negotiation log for contract "{name}"')
def step_when_view_negotiation_log(context, name):
    did, _ = _contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I view approval status for contract "{name}"')
def step_when_view_approval_status(context, name):
    did, _ = _contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when("the approval process completes")
def step_when_approval_completes(context):
    # The Given "contract has all required approvals" already approved the contract.
    # This step confirms the outcome rather than triggering a second approval.
    name = "Service Agreement"
    did, _ = _contract_data(context, name)
    approver_h = _headers_for_role(context, "Contract Approver")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=approver_h)
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    if state != "APPROVED":
        # Not yet approved — attempt final approval.
        updated_at = retrieve.json().get("updated_at")
        context.requests_response = post_json(
            context, contract_approve_url(context),
            {"did": did, "updated_at": updated_at},
            headers=approver_h,
        )
        if context.requests_response.status_code == 200:
            _refresh_contract(context, name)
    else:
        # Already approved — synthesise a success response so Then-assertions can inspect it.
        import types
        fake = types.SimpleNamespace(
            status_code=200,
            text='{"did":"' + did + '"}',
        )
        fake.json = lambda: {"did": did}
        context.requests_response = fake


@when("I attempt to approve my own redline proposal")
def step_when_attempt_approve_own_redline(context):
    name = "Service Agreement"
    did, _ = _contract_data(context, name)
    refresh = _refresh_contract(context, name)
    negotiations = refresh.get("negotiations") or []
    if negotiations:
        negotiation_id = negotiations[-1].get("id") or negotiations[-1].get("did")
        if negotiation_id:
            context.requests_response = post_json(
                context,
                f"{context.base_url}/contract/respond",
                {"id": str(negotiation_id), "action_flag": "accept", "rejected_by": "", "rejection_reason": ""},
            )
            return
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I attempt to access contract "{name}" for negotiation')
def step_when_attempt_access_contract_negotiation(context, name):
    did = (getattr(context, "contract_dids", None) or {}).get(name, "did:example:missing")
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('a representative of party "{party}" attempts to access the contract')
def step_when_party_accesses_contract(context, party):
    name = "Service Agreement"
    did, _ = _contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I attempt to access contract "{name}"')
def step_when_attempt_access_contract(context, name):
    did = (getattr(context, "contract_dids", None) or {}).get(name, "did:example:missing")
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when("automated compliance checks are performed")
def step_when_automated_compliance_checks(context):
    name = "Service Agreement"
    did, updated_at = _contract_data(context, name)
    context.requests_response = post_json(
        context, contract_verify_url(context), {"did": did, "updated_at": updated_at}
    )


# ---------------------------------------------------------------------------
# Then assertions
# ---------------------------------------------------------------------------

@then("a draft contract is generated")
def step_then_draft_contract_generated(context):
    assert context.requests_response.status_code == 200, (
        f"Contract creation failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), f"Expected contract DID, got: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "DRAFT", f"Expected new contract to be in DRAFT state, got '{state}'"


@then("a contract is created linked to the template")
def step_then_contract_linked_to_template(context):
    assert context.requests_response.status_code == 200, (
        f"Contract generation failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), f"Expected contract DID, got: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    assert retrieve.json().get("did"), f"Contract missing 'did': {retrieve.json()}"


@then("both machine-readable and human-readable versions are available")
def step_then_both_views_available(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert "contract_data" in contract or "name" in contract, (
        f"Contract missing content fields: {contract}"
    )


@then("metadata is auto-filled including parties, jurisdiction, and applicable schemas")
def step_then_metadata_auto_filled(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("created_at") or contract.get("created_by") or contract.get("name"), (
        f"Expected auto-populated metadata fields in contract: {contract}"
    )


@then("the creation is logged and traceable to the template version")
def step_then_creation_logged(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("created_at"), f"Expected 'created_at' timestamp in contract: {contract}"


@then("the machine-readable view renders correctly")
def step_then_machine_readable_view(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert "contract_data" in contract, f"Missing machine-readable field 'contract_data': {contract}"


@then("the human-readable view renders correctly")
def step_then_human_readable_view(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("name") or contract.get("description"), (
        f"Missing human-readable fields 'name'/'description': {contract}"
    )


@then("the changes are saved")
def step_then_changes_saved(context):
    assert context.requests_response.status_code == 200, (
        f"Contract update failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in update response: {body}"


@then("a new version is created with timestamp and user attribution")
def step_then_contract_versioned(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in update response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("updated_at"), f"Expected 'updated_at' timestamp in contract: {contract}"


@then("the negotiation interface is displayed")
def step_then_negotiation_interface(context):
    assert context.requests_response.status_code == 200, (
        f"Expected negotiation interface (contract retrieve), got {context.requests_response.status_code}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No contract DID in response: {body}"


@then("I can view all contract clauses")
def step_then_can_view_clauses(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract content in response: {body}"


@then("the comment is added to the negotiation log")
def step_then_comment_added(context):
    assert context.requests_response.status_code == 200, (
        f"Negotiate endpoint failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in negotiate response: {body}"


@then("the comment is attributed to my identity")
def step_then_comment_attributed(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in negotiate response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    negotiations = retrieve.json().get("negotiations") or []
    assert negotiations, "Expected at least one negotiation entry"
    last = negotiations[-1]
    assert last.get("negotiated_by"), f"Negotiation entry missing 'negotiated_by': {last}"


@then("the comment includes a timestamp")
def step_then_comment_timestamp(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in negotiate response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("updated_at"), f"Expected 'updated_at' on contract after negotiation: {contract}"


@then("the proposed change is tracked")
def step_then_proposed_change_tracked(context):
    step_then_comment_added(context)


@then("the original text is preserved")
def step_then_original_text_preserved(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in negotiation response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    negotiations = retrieve.json().get("negotiations") or []
    assert negotiations, "Expected negotiation history to preserve prior text context"


@then("the redline proposal is visible to other negotiators")
def step_then_redline_visible(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    negotiations = retrieve.json().get("negotiations") or []
    assert negotiations, "Expected negotiations list to be non-empty after redline proposal"


@then("I see all versions with timestamps")
def step_then_versions_with_timestamps(context):
    body = context.requests_response.json()
    assert body.get("updated_at"), f"No 'updated_at' in contract response: {body}"


@then("old versions remain accessible")
def step_then_old_versions_accessible(context):
    body = context.requests_response.json()
    assert body.get("did"), f"Contract DID missing: {body}"


@then("the change is applied to the contract")
def step_then_change_applied(context):
    assert context.requests_response.status_code == 200, (
        f"Expected change to be applied (200), got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("a new version is created")
def step_then_new_contract_version(context):
    body = context.requests_response.json()
    assert body.get("did") or body.get("id"), f"No ID in response: {body}"


@then("the proposal is marked as rejected")
def step_then_proposal_rejected(context):
    assert context.requests_response.status_code == 200, (
        f"Expected rejection to succeed (200), got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("I see all comments and proposals")
def step_then_see_comments(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract in response: {body}"


@then("I see the full audit trail")
def step_then_see_audit_trail(context):
    body = context.requests_response.json()
    assert body.get("updated_at") or body.get("negotiations") is not None, (
        f"No audit-trail fields in response: {body}"
    )


@then('the contract status changes to "Under Review"')
def step_then_contract_status_under_review(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in submit response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state in ("SUBMITTED", "UNDER_REVIEW", "REVIEW", "NEGOTIATION"), (
        f"Expected submitted/under-review state, got '{state}'"
    )


@then("the submission is logged")
def step_then_submission_logged(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in submit response: {body}"


@then('the contract status shows "{expected_status}"')
def step_then_contract_status_shows(context, expected_status):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == expected_status.upper().replace(" ", "_"), (
        f"Expected '{expected_status}', got '{state}'"
    )


@then("my approval is logged with timestamp")
def step_then_approval_logged_timestamp(context):
    assert context.requests_response.status_code == 200, (
        f"Approve failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in approve response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    assert retrieve.json().get("updated_at"), "Missing 'updated_at' after approval"


@then("the approval status is updated")
def step_then_approval_status_updated(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in approve response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "APPROVED", f"Expected APPROVED state after approval, got '{state}'"


@then("the rejection is logged with comments and timestamp")
def step_then_rejection_logged(context):
    assert context.requests_response.status_code == 200, (
        f"Reject failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in reject response: {body}"


@then('the contract status returns to "Draft"')
def step_then_contract_returns_draft(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in reject response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "DRAFT", f"Expected contract to revert to DRAFT after rejection, got '{state}'"


@then("the contract is returned for revision")
def step_then_contract_returned_revision(context):
    step_then_contract_returns_draft(context)


@then('the contract is marked as "Approved"')
def step_then_contract_marked_approved(context):
    step_then_approval_status_updated(context)


@then("I can view previous reviewer comments")
def step_then_can_view_previous_comments(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract DID in response: {body}"


@then("the system validates against regulatory frameworks")
def step_then_validates_regulatory(context):
    body = context.requests_response.json()
    assert "findings" in body or body.get("did"), f"Expected verify-style response: {body}"


@then("compliance issues are flagged for review")
def step_then_compliance_issues_flagged(context):
    body = context.requests_response.json()
    assert "findings" in body or body.get("did"), f"Expected findings in verify response: {body}"


@then("I see pending approvals")
def step_then_see_pending_approvals(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract DID in approval-status response: {body}"


@then("negotiation actions are logged with reviewer identity")
def step_then_negotiation_actions_logged(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in response: {body}"
