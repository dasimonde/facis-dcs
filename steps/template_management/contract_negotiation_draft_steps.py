"""Steps for the party-scoped negotiation draft (SRS §3.1.1 Contract
Negotiation UI "Save draft" control): a negotiator stages a change request
(PUT /contract/negotiation_draft), retrieves and discards it, and proposing
via POST /contract/negotiate consumes it. Drafts never create negotiation
change-request rows and never move the contract state; they are keyed by the
party's participant ID, so same-party colleagues share the staged position
while nothing reaches the counterparty until proposed.
"""

import requests as _requests
from behave import then, when

from steps.support.api_client import (
    contract_negotiate_url,
    contract_negotiation_draft_url,
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


def _creator_headers(context, name):
    seed = getattr(context, "contract_seed_headers", None) or {}
    return seed.get(name) or getattr(context, "headers", None)


def _get_draft(context, name, headers):
    did, _ = ContractService._contract_data(context, name)
    return get_with_headers(context, contract_negotiation_draft_url(context, did), headers=headers)


@when('the negotiator saves a negotiation draft for contract "{name}" renaming it to "{staged_name}"')
def step_when_save_negotiation_draft(context, name, staged_name):
    did, _ = ContractService._contract_data(context, name)
    headers = _creator_headers(context, name)
    context.requests_response = _requests.put(
        contract_negotiation_draft_url(context),
        json={"did": did, "change_request": {"name": staged_name}},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when('the negotiator discards the negotiation draft for contract "{name}"')
def step_when_discard_negotiation_draft(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = _creator_headers(context, name)
    context.requests_response = _requests.delete(
        contract_negotiation_draft_url(context, did),
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when('the negotiator proposes the staged draft for contract "{name}"')
def step_when_propose_staged_draft(context, name):
    headers = _creator_headers(context, name)
    draft = _get_draft(context, name, headers)
    assert draft.status_code == 200, f"draft retrieve failed: {draft.status_code} {draft.text}"
    change_request = draft.json().get("change_request")
    assert change_request, f"no staged draft to propose for '{name}': {draft.text}"

    ContractService._refresh_contract(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": AuthService.username_for_roles(["Contract Creator"]),
            "change_request": change_request,
        },
        headers=headers,
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@then('the negotiation draft for contract "{name}" contains the staged name "{staged_name}"')
def step_then_draft_contains_staged_name(context, name, staged_name):
    resp = _get_draft(context, name, _creator_headers(context, name))
    assert resp.status_code == 200, f"draft retrieve failed: {resp.status_code} {resp.text}"
    change_request = resp.json().get("change_request") or {}
    assert change_request.get("name") == staged_name, (
        f"Expected staged name '{staged_name}', got: {resp.text}"
    )


@then('the negotiation draft for contract "{name}" is empty')
def step_then_draft_is_empty(context, name):
    resp = _get_draft(context, name, _creator_headers(context, name))
    assert resp.status_code == 200, f"draft retrieve failed: {resp.status_code} {resp.text}"
    assert not resp.json().get("change_request"), (
        f"Expected no stored draft, got: {resp.text}"
    )


@then(
    'a same-party user with roles "{roles}" sees the negotiation draft for contract "{name}" '
    'with the staged name "{staged_name}"'
)
def step_then_same_party_user_sees_draft(context, roles, name, staged_name):
    """Drafts are party-scoped (saved_by = participant ID / ext.iss): a
    colleague of the same party — a different user, same organization —
    retrieves the party's staged position."""
    other_headers = AuthService.get_headers_for_roles([r.strip() for r in roles.split(",")])
    resp = _get_draft(context, name, other_headers)
    assert resp.status_code == 200, f"draft retrieve failed: {resp.status_code} {resp.text}"
    change_request = resp.json().get("change_request") or {}
    assert change_request.get("name") == staged_name, (
        f"Expected the same-party user ({roles}) to see the staged name '{staged_name}', got: {resp.text}"
    )


@then('the contract "{name}" has no recorded negotiation change requests')
def step_then_no_negotiation_rows(context, name):
    did, _ = ContractService._contract_data(context, name)
    resp = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=_creator_headers(context, name))
    assert resp.status_code == 200, resp.text
    negotiations = resp.json().get("negotiations") or []
    assert not negotiations, (
        f"Expected no negotiation change requests (a draft must stay private), got: {negotiations}"
    )


@then('the contract "{name}" has a recorded negotiation change request renaming it to "{staged_name}"')
def step_then_negotiation_row_with_name(context, name, staged_name):
    did, _ = ContractService._contract_data(context, name)
    resp = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=_creator_headers(context, name))
    assert resp.status_code == 200, resp.text
    negotiations = resp.json().get("negotiations") or []
    staged = [
        negotiation
        for negotiation in negotiations
        if (negotiation.get("change_request") or {}).get("name") == staged_name
    ]
    assert staged, (
        f"Expected a negotiation change request renaming to '{staged_name}', got: {negotiations}"
    )
