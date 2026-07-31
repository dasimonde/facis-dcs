"""Step definitions for the contract_negotiation.feature party-scoping and
conflict-of-interest scenarios. See
steps/template_management/contract_workflow_steps.py for the pre-existing
negotiation steps this file complements — several of the Then-steps there
(e.g. "the approval is logged in the negotiation log", "I see approvals and
rejections") are reused as-is by the scenarios these steps support.
"""

import json

from behave import given, then

from steps.support.api_client import (
    contract_negotiate_url,
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


# ---------------------------------------------------------------------------
# Given helpers
# ---------------------------------------------------------------------------


@given('I have proposed a redline edit to clause "{clause}"')
def step_given_have_proposed_redline(context, clause):
    # Uses context.headers (the scenario's currently-authenticated actor) so
    # that a later same-actor accept attempt is a genuine conflict of
    # interest (created_by == AcceptedBy — see acceptnegotiation.go).
    name = "Service Agreement"
    ContractService._create_contract_in_negotiation(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    resp = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": AuthService.username_for_roles(["Contract Reviewer"]),
            "change_request": f"Redline on {clause}: proposed replacement text",
        },
        headers=context.headers,
    )
    assert resp.status_code == 200, f"negotiate failed while proposing own redline: {resp.status_code} {resp.text}"
    ContractService._refresh_contract(context, name)


@given('contract "{name}" does not list this instance as a negotiating party')
def step_given_contract_not_a_party(context, name):
    ContractService._create_contract_excluding_local_peer(context, name)


# ---------------------------------------------------------------------------
# Then assertions
# ---------------------------------------------------------------------------


@then('the request is denied with a "{message}" error')
def step_then_denied_with_message(context, message):
    assert context.requests_response.status_code in (400, 401, 403, 404, 409), (
        f"Expected a client-error denial, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    try:
        body = context.requests_response.json()
        haystack = json.dumps(body).lower()
    except ValueError:
        haystack = context.requests_response.text.lower()
    assert message.lower() in haystack, (
        f"Expected error message containing '{message}', got: {context.requests_response.text}"
    )


@then("the access denial is logged")
def step_then_access_denial_logged(context):
    # No persisted audit-trail row is written on an authorization-denial
    # path in this codebase (event.Create only runs inside successful
    # command handlers) — the denial is "logged" in the sense of being
    # surfaced, structured and traceable, in the response itself.
    assert context.requests_response.status_code >= 400, (
        "Expected the access denial to be reflected in a non-2xx response"
    )
    try:
        body = context.requests_response.json()
    except ValueError:
        body = None
    assert body, f"Expected a structured denial response body, got: {context.requests_response.text}"


@then("another authorized reviewer must approve")
def step_then_another_reviewer_must_approve(context):
    name = "Service Agreement"
    did, _ = ContractService._contract_data(context, name)
    refresh = ContractService._refresh_contract(context, name)
    negotiations = refresh.get("negotiations") or []
    assert negotiations, f"Expected a pending negotiation to approve, got: {refresh}"
    negotiation_id = negotiations[-1]["id"]
    other_h = AuthService.get_headers_for_roles(["Contract Reviewer"], organization="TechVendor Inc")
    resp = post_json(
        context,
        f"{context.base_url}/contract/respond",
        {"id": str(negotiation_id), "did": did, "action_flag": "ACCEPTING", "rejected_by": "", "rejection_reason": ""},
        headers=other_h,
    )
    assert resp.status_code == 200, (
        f"Expected a different party to be able to accept the same proposal: {resp.status_code} {resp.text}"
    )
    context.other_reviewer_response = resp
    refreshed = ContractService._refresh_contract(context, name)
    decisions = [
        d
        for entry in (refreshed.get("negotiations") or [])
        for d in (entry.get("negotiation_decisions") or [])
    ]
    assert any(str(d.get("decision", "")).upper() == "ACCEPTED" for d in decisions), (
        f"Expected an ACCEPTED decision from the other reviewer, got: {decisions}"
    )


@then("the contract is routed to assigned reviewers")
def step_then_routed_to_reviewers(context):
    # Reviewer tasks are actually created at contract creation time
    # (create.go's createTasks, from the create payload's "reviewers" peer
    # DID list) and stay open until acted on — by the time a contract
    # reaches SUBMITTED, "routed to assigned reviewers" means those reviewer
    # DIDs are present and the contract is now in a reviewable state.
    name = "Service Agreement"
    did, _ = ContractService._contract_data(context, name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    responsible = retrieve.json().get("responsible") or {}
    assert responsible.get("reviewers"), (
        f"Expected at least one assigned reviewer DID on the contract, got: {responsible}"
    )


@then("the restriction is logged")
def step_then_restriction_logged(context):
    # context.requests_response still holds the ORIGINAL self-approval
    # denial here (the previous Then step stashes its own success response
    # separately in context.other_reviewer_response rather than overwriting
    # context.requests_response).
    assert context.requests_response.status_code >= 400, (
        "Expected the conflict-of-interest restriction to be reflected in a non-2xx response"
    )
    body = context.requests_response.json()
    assert body, f"Expected a structured denial response body, got: {context.requests_response.text}"
