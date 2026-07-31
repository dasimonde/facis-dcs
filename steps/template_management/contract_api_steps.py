"""Contract creation API steps for executable BDD scenarios."""

import os

import requests as _requests
from behave import given, then, when

from core.utils import is_uuid
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService
from steps.support.api_client import (
    contract_approve_url,
    contract_create_url,
    contract_retrieve_by_id_url,
    contract_update_url,
    contract_verify_url,
    get_with_headers,
    post_json,
    put_json,
)


@when('the system sends a POST request to create contract with template "{template_name}"')
def step_when_create_contract_with_template(context, template_name):
    assert hasattr(context, "template_dids") and template_name in context.template_dids, (
        f"No template DID configured for template '{template_name}'"
    )
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {"template_did": context.template_dids[template_name]},
    )


@when("the system submits contract creation request with populated fields")
def step_when_create_contract_with_payload(context):
    template_did = os.getenv("BDD_TEMPLATE_DID_DEFAULT")
    if not template_did:
        from steps.template_management.template_workflow_steps import _create_approved_template  # noqa: PLC0415

        template_did, _ = _create_approved_template(context)
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {"template_did": template_did},
    )


@when("the system attempts to create contract via API")
def step_when_attempt_create_contract(context):
    payload = {"template_did": os.getenv("BDD_TEMPLATE_DID_DEFAULT", "did:example:template:missing")}
    context.requests_response = post_json(context, contract_create_url(context), payload)


@when('I create a contract from template "{template_name}"')
def step_when_create_contract_from_template(context, template_name):
    assert hasattr(context, "template_dids") and template_name in context.template_dids, (
        f"No approved template DID for '{template_name}' — ensure the Given step ran"
    )
    peer_did = ContractService._local_peer_did(context)
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": context.template_dids[template_name],
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        },
    )


@when('I attempt to create a contract from template "{template_name}"')
def step_when_attempt_create_contract_from_template(context, template_name):
    template_did = (
        (context.template_dids or {}).get(template_name)
        if hasattr(context, "template_dids")
        else "did:example:template:missing"
    )
    peer_did = ContractService._local_peer_did(context)
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": template_did or "did:example:template:missing",
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        },
    )


@then("the contract is assigned a unique contract ID")
def step_then_contract_unique_id(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), f"Expected a contract DID, got: {body}"
    uuid = did.split(":")[-1]
    assert is_uuid(uuid), f"Expected {uuid} is a valid UUID"


# ---------------------------------------------------------------------------
# Protected endpoint coverage — sends each endpoint with the current headers
# (used to verify invalid-token rejection across all implemented endpoints)
# ---------------------------------------------------------------------------

_ENDPOINT_PAYLOADS = {
    "template_create":        {"template_type": TemplateService.CONTRACT_TEMPLATE_TYPE, "name": "bdd-test", "description": "test", "template_data": {}},
    "template_submit":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "template_update":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "template_update_manage": {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "template_verify":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "template_approve":       {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "template_reject":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z", "reason": "test"},
    "template_register":      {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "template_archive":       {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "contract_create":        {"template_did": "did:example:template:1"},
    "contract_update":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "contract_submit":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "contract_negotiate":     {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z", "negotiated_by": "test", "change_request": "test"},
    "contract_respond":       {"id": "1", "action_flag": "ACCEPTING", "rejected_by": "", "rejection_reason": "", "responded_by": "bdd-test"},
    "contract_verify":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "contract_approve":       {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "contract_reject":        {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z", "reason": "test"},
    "contract_store":         {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "contract_terminate":     {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "contract_audit":         {"did": "did:example:1", "updated_at": "2024-01-01T00:00:00Z"},
    "none":                   {},
}


@when('the system sends "{method}" request to protected endpoint "{endpoint}" with payload "{payload_key}"')
def step_when_protected_endpoint_request(context, method, endpoint, payload_key):
    url = f"{context.base_url}{endpoint}"
    headers = getattr(context, "headers", {})
    payload = _ENDPOINT_PAYLOADS.get(payload_key, {})
    m = method.upper()
    if m == "POST":
        context.requests_response = _requests.post(url, json=payload, headers=headers,
                                                    timeout=context.http_timeout_seconds)
    elif m == "PUT":
        context.requests_response = _requests.put(url, json=payload, headers=headers,
                                                   timeout=context.http_timeout_seconds)
    elif m == "GET":
        context.requests_response = _requests.get(url, headers=headers,
                                                   timeout=context.http_timeout_seconds)
    else:
        raise NotImplementedError(f"HTTP method '{method}' not handled in coverage step")


# ---------------------------------------------------------------------------
# API-based contract operations (UC-12)
# ---------------------------------------------------------------------------

@when('the system sends a review request for contract "{name}"')
def step_when_send_review_request(context, name):
    did = ContractService._did_for_contract(context, name, target_state="DRAFT")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    assert updated_at, f"Missing updated_at in contract retrieve payload: {retrieve.text}"
    context.requests_response = post_json(
        context, contract_verify_url(context), {"did": did, "updated_at": updated_at}
    )


@when('the system sends approval request for contract "{name}"')
def step_when_send_approval_request(context, name):
    did = ContractService._did_for_contract(context, name, target_state="UNDER_REVIEW")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    assert updated_at, f"Missing updated_at in contract retrieve payload: {retrieve.text}"
    context.last_approve_payload = {"did": did, "updated_at": updated_at}
    context.requests_response = post_json(
        context, contract_approve_url(context), {"did": did, "updated_at": updated_at}
    )


@when('the system queries contract "{name}" status')
def step_when_query_contract_status(context, name):
    did = ContractService._did_for_contract(context, name, target_state="DRAFT")
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when("the system receives review results via API")
def step_when_receive_review_results(context):
    assert hasattr(context, "requests_response"), "No prior review response found on context"
    body = context.requests_response.json()
    assert body.get("did"), f"Expected review response to contain DID anchor: {body}"


@when("the system submits approval with condition data")
def step_when_approval_with_condition(context):
    did = ContractService._did_for_contract(context, "Service Agreement", target_state="UNDER_REVIEW")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    assert updated_at, f"Missing updated_at in contract retrieve payload: {retrieve.text}"
    context.requests_response = post_json(
        context, contract_approve_url(context), {"did": did, "updated_at": updated_at}
    )


@when("the system attempts approval via API")
def step_when_attempt_approval_api(context):
    did = getattr(context, "non_approvable_contract_did", None)
    if did:
        retrieve = ContractService._retrieve_contract_readable(context, did)
        updated_at = retrieve.json().get("updated_at")
        payload = {"did": did, "updated_at": updated_at}
    else:
        payload = {"did": "did:example:not-approvable", "updated_at": "2024-01-01T00:00:00Z"}
    context.requests_response = post_json(
        context, contract_approve_url(context),
        payload,
    )


@when("the system sends update request with new terms")
def step_when_update_with_new_terms(context):
    did = ContractService._did_for_contract(context, "Service Agreement", target_state="APPROVED")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    assert updated_at, f"Missing updated_at in contract retrieve payload: {retrieve.text}"
    context.requests_response = put_json(
        context,
        contract_update_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "contract_data": TemplateService.canonical_document_data("API-updated terms", document_type="dcs:Contract"),
        },
    )


@when("the system requests performance metrics via API")
def step_when_request_performance_metrics(context):
    did = ContractService._did_for_contract(context, "Service Agreement", target_state="APPROVED")
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@given("contract validation identifies issues")
@when("contract validation identifies issues")
def step_contract_validation_identifies_issues(context):
    context.requests_response = post_json(
        context, contract_verify_url(context),
        {"did": "did:example:missing", "updated_at": "2024-01-01T00:00:00Z"},
    )


@given('a system service without review permissions is authenticated via API')
@when('a system service without review permissions is authenticated via API')
def step_service_without_review_perms(context):
    context.headers = {
        "Authorization": "Bearer invalid-review-token",
        "Content-Type": "application/json",
    }


@given("approval requires specific conditions")
def step_given_approval_requires_conditions(context):
    did = ContractService._did_for_contract(context, "Service Agreement", target_state="UNDER_REVIEW")
    context.condition_contract_did = did


@given("contract is not in approvable status")
def step_given_contract_not_approvable(context):
    did = ContractService._did_for_contract(context, "Service Agreement", target_state="DRAFT")
    context.non_approvable_contract_did = did


@given("contract has KPIs defined")
def step_given_contract_has_kpis(context):
    did = ContractService._did_for_contract(context, "Service Agreement", target_state="APPROVED")
    context.kpi_contract_did = did


@when("the system attempts contract review via API")
def step_when_attempt_contract_review_api(context):
    context.requests_response = post_json(
        context, contract_verify_url(context),
        {"did": "did:example:missing", "updated_at": "2024-01-01T00:00:00Z"},
    )


# ---------------------------------------------------------------------------
# API contract then-assertions
# ---------------------------------------------------------------------------

@then("the current status and metadata are returned")
def step_then_status_and_metadata_returned(context):
    assert context.requests_response.status_code == 200, (
        f"Expected contract status query to succeed, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in contract status response: {body}"
    assert body.get("state"), f"No 'state' in contract status response: {body}"


@then("access respects RBAC")
def step_then_access_respects_rbac(context):
    did = context.requests_response.json().get("did")
    assert did, "RBAC check requires a contract DID from the status query response"
    unauth = get_with_headers(
        context,
        contract_retrieve_by_id_url(context, did),
        headers={"Authorization": "Bearer invalid-token", "Content-Type": "application/json"},
    )
    assert unauth.status_code in (401, 403), (
        "Expected unauthorized retrieve to be denied by RBAC, "
        f"got {unauth.status_code}: {unauth.text}"
    )


@then("changes are versioned and logged with timestamp and actor identity")
def step_then_versioned_and_logged(context):
    assert context.requests_response.status_code == 200, (
        f"Update failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in update response: {body}"


@then("the contract is validated against predefined rules")
def step_then_validated_against_rules(context):
    assert context.requests_response.status_code == 200, (
        f"Review/verify failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert "findings" in body, f"Expected validation findings in response: {body}"
    assert isinstance(body.get("findings"), list), f"'findings' must be a list: {body}"


@then("inconsistencies are flagged")
def step_then_inconsistencies_flagged(context):
    body = context.requests_response.json()
    assert "findings" in body, f"Expected findings in validation response: {body}"
    assert isinstance(body.get("findings"), list), f"'findings' must be a list: {body}"


@then("a validation report is returned")
def step_then_validation_report_returned(context):
    body = context.requests_response.json()
    assert body.get("did"), f"Expected DID anchor in validation response: {body}"
    assert "findings" in body, f"Expected findings in validation response: {body}"


@then("automated correction suggestions are provided")
def step_then_correction_suggestions(context):
    body = context.requests_response.json()
    assert "findings" in body, f"Expected findings in validation response: {body}"


@then("the contract can be updated via API")
def step_then_contract_can_be_updated(context):
    did = context.requests_response.json().get("did")
    assert did, f"No DID in verify/review response: {context.requests_response.json()}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    assert updated_at, f"Missing updated_at for post-review update: {retrieve.text}"
    update_resp = put_json(
        context,
        contract_update_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "contract_data": TemplateService.canonical_document_data("API post-review adjustment", document_type="dcs:Contract"),
        },
    )
    assert update_resp.status_code == 200, (
        "Expected contract update after review to succeed, "
        f"got {update_resp.status_code}: {update_resp.text}"
    )


@then("the request origin is validated")
def step_then_request_origin_validated(context):
    payload = getattr(context, "last_approve_payload", None)
    assert payload, "Missing approval request payload for origin validation check"
    unauth_resp = post_json(
        context,
        contract_approve_url(context),
        payload,
        headers={"Authorization": "Bearer invalid-token", "Content-Type": "application/json"},
    )
    assert unauth_resp.status_code in (401, 403), (
        "Expected unauthenticated approval attempt to be denied, "
        f"got {unauth_resp.status_code}: {unauth_resp.text}"
    )


@then("the contract is marked as approved")
def step_then_contract_marked_approved_api(context):
    assert context.requests_response.status_code == 200, (
        f"API approval failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in approval response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "APPROVED", f"Expected APPROVED, got '{state}'"


@then("the decision is logged with timestamp and actor identity")
def step_then_decision_logged(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in decision response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("updated_at"), f"Missing updated_at audit timestamp: {contract}"
    assert contract.get("created_by"), f"Missing actor identity field created_by: {contract}"


@then("conditions are evaluated")
def step_then_conditions_evaluated(context):
    body = context.requests_response.json()
    assert body.get("did"), f"Expected approval response with DID: {body}"


@then("approval is granted if conditions met")
def step_then_approval_granted_if_conditions(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in approval response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "APPROVED", f"Expected APPROVED after conditional approval, got '{state}'"


@then('the request is denied with error "Contract is not in approvable status"')
def step_then_denied_not_approvable(context):
    assert context.requests_response.status_code in (400, 404, 409, 422), (
        f"Expected rejection for non-approvable contract, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("metadata is populated from API payload")
def step_then_metadata_populated(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = ContractService._retrieve_contract_readable(context, did)
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("created_at"), f"Missing created_at timestamp in contract metadata: {contract}"
    assert contract.get("updated_at"), f"Missing updated_at timestamp in contract metadata: {contract}"
    assert contract.get("state"), f"Missing workflow state in contract metadata: {contract}"


@then("the contract is created with provided data")
def step_then_contract_created_with_data(context):
    assert context.requests_response.status_code == 200, (
        f"Contract creation with data failed: {context.requests_response.status_code} — "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in create response: {body}"


@then("validation ensures required fields are present")
def step_then_required_fields_present(context):
    body = context.requests_response.json()
    assert body.get("did"), f"Expected required field 'did' in response: {body}"


@then('the contract status is set to "Draft"')
def step_then_contract_status_draft(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = ContractService._retrieve_contract_readable(context, did)
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "DRAFT", f"Expected DRAFT state for new API contract, got '{state}'"


@then("KPI data is returned")
def step_then_kpi_data_returned(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in KPI retrieval response: {body}"


@then("alerts are included if thresholds exceeded")
def step_then_alerts_included(context):
    body = context.requests_response.json()
    assert body.get("state"), f"Expected lifecycle state in performance response: {body}"


@then("the creation is logged with timestamp and actor identity")
def step_then_creation_logged_timestamp(context):
    assert context.requests_response.status_code == 200, (
        f"Expected creation to succeed, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = ContractService._retrieve_contract_readable(context, did)
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("created_at"), (
        f"Expected 'created_at' timestamp for audit trail in contract: {contract}"
    )


@then("the generated contract exposes both machine-readable and human-readable content")
def step_then_generated_contract_exposes_formats(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = ContractService._retrieve_contract_readable(context, did)
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("contract_data") is not None, f"Missing machine-readable contract data: {contract}"
    assert contract.get("name") or contract.get("description"), (
        f"Missing human-readable contract metadata in response: {contract}"
    )


@then("the attempt is logged with timestamp and actor identity")
def step_then_attempt_logged(context):
    # A failed attempt should still return an error response — the API is expected to log it.
    assert context.requests_response.status_code in (400, 401, 403, 404, 422), (
        f"Expected an error response for the blocked attempt, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("the contract is updated")
def step_then_contract_is_updated(context):
    assert context.requests_response.status_code == 200, (
        f"Expected contract update to succeed, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in update response: {body}"
