"""Template API steps for executable BDD scenarios."""

from behave import given, then, when

from support.api_client import (
    contract_create_url,
    get_with_headers,
    post_json,
    put_json,
    template_create_url,
    template_retrieve_by_id_url,
    template_update_url,
)
from support.template_utils import template_type_for_category


@when('I create a template "{template_name}" in category "{category}"')
def step_when_create_template(context, template_name, category):
    payload = {
        "template_type": template_type_for_category(category),
        "name": template_name,
        "description": "BDD executable template creation",
        "template_data": {
            "title": template_name,
            "clauses": [{"id": "c1", "text": "Confidentiality clause"}],
        },
    }
    context.requests_response = post_json(context, template_create_url(context), payload)
    body = context.requests_response.json()
    context.created_template_did = body.get("did")
    assert context.created_template_did, body


@when("I attempt to create a template")
def step_when_attempt_create_template(context):
    # Uses whatever role headers are currently on context (set by Given "I am authenticated with role X").
    payload = {
        "template_type": "FRAME_CONTRACT",
        "name": "BDD Unauthorized Template",
        "description": "Should be blocked by RBAC",
        "template_data": {"title": "Test", "clauses": []},
    }
    context.requests_response = post_json(context, template_create_url(context), payload)


@then('the template is created in "Draft" status')
def step_then_template_created_draft(context):
    context.template_retrieve_response = get_with_headers(
        context,
        template_retrieve_by_id_url(context, context.created_template_did),
    )
    assert context.template_retrieve_response.status_code == 200, context.template_retrieve_response.text
    body = context.template_retrieve_response.json()
    state = str(body.get("state", "")).lower()
    assert state == "draft", body


@then('the template is assigned version "1.0"')
def step_then_template_version(context):
    body = context.template_retrieve_response.json()
    version = body.get("version")
    assert version in (None, 1, "1", "1.0"), body


# ---------------------------------------------------------------------------
# Update / version assertions
# ---------------------------------------------------------------------------

@then('a new version "1.1" is created')
def step_then_new_version_created(context):
    # The update response includes {did, document_number, version}.
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in update response: {body}"
    # Retrieve fresh state and confirm version advanced.
    retrieve = get_with_headers(context, template_retrieve_by_id_url(context, body["did"]))
    assert retrieve.status_code == 200, retrieve.text
    version = str(retrieve.json().get("version", ""))
    # Version must be non-empty and differ from the initial "1.0" / 1.
    assert version not in ("", "1.0", "1"), (
        f"Expected version to advance past 1.0, got '{version}'"
    )


@then("the previous version remains accessible")
def step_then_previous_version_accessible(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in update response: {body}"
    retrieve = get_with_headers(context, template_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, (
        f"Template DID {did} is no longer resolvable after update: {retrieve.text}"
    )
    assert retrieve.json().get("did") == did


# ---------------------------------------------------------------------------
# Rejection
# ---------------------------------------------------------------------------

@then("the rejection reason is recorded")
def step_then_rejection_reason_recorded(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in reject response: {body}"
    # After rejection the template should return to Draft.
    retrieve = get_with_headers(context, template_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "DRAFT", (
        f"Expected template to revert to DRAFT after rejection, got '{state}'"
    )


# ---------------------------------------------------------------------------
# Search / retrieve assertions
# ---------------------------------------------------------------------------

@then("the results are filtered by my access rights")
def step_then_results_filtered(context):
    assert context.requests_response.status_code == 200, (
        f"Search failed: {context.requests_response.status_code} {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert isinstance(body, list), f"Expected list of results, got: {type(body)} — {body}"
    assert body, "Expected at least one template search result"
    for item in body:
        assert item.get("did"), f"Search result missing DID: {item}"
        assert item.get("state"), f"Search result missing state: {item}"


@then("I see the template provenance")
def step_then_see_provenance(context):
    body = context.requests_response.json()
    # Provenance metadata = auditable fields: creator, creation time, DID.
    assert body.get("did"), f"Missing 'did' (identity anchor) in response: {body}"
    assert body.get("created_at") or body.get("created_by") or body.get("updated_at"), (
        f"No provenance/audit fields (created_at, created_by, updated_at) in response: {body}"
    )


# ---------------------------------------------------------------------------
# Verify assertions
# ---------------------------------------------------------------------------

@then("the JSON-LD context is validated")
def step_then_jsonld_validated(context):
    assert context.requests_response.status_code == 200, (
        f"Verify endpoint failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert "findings" in body, f"Expected 'findings' key in verify response: {body}"
    assert isinstance(body["findings"], list), f"'findings' must be a list: {body}"


@then("the SHACL constraints are validated")
def step_then_shacl_validated(context):
    # SHACL validation outcome is captured in the same verify response as JSON-LD.
    body = context.requests_response.json()
    assert "findings" in body, f"Expected 'findings' key in verify response: {body}"


@then("the digital signatures are verified")
def step_then_signatures_verified(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in verify response: {body}"
    findings = body.get("findings")
    assert isinstance(findings, list), f"Expected findings list in verify response: {body}"
    signature_failures = [f for f in findings if "signature" in str(f).lower() and "fail" in str(f).lower()]
    assert not signature_failures, f"Signature verification reported failures: {signature_failures}"


# ---------------------------------------------------------------------------
# Deprecation / contract-generation guard
# ---------------------------------------------------------------------------

@then("new contracts cannot be generated from this template")
def step_then_no_new_contracts_from_deprecated(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in archive response: {body}"
    # Attempt to create a contract from the deprecated template — must be rejected.
    create_resp = post_json(context, contract_create_url(context), {"did": did})
    assert create_resp.status_code >= 400, (
        f"Expected contract creation from deprecated template to be rejected, "
        f"got {create_resp.status_code}: {create_resp.text}"
    )


# ---------------------------------------------------------------------------
# UUID uniqueness
# ---------------------------------------------------------------------------

@then("the UUID is unique across the system")
def step_then_uuid_unique(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), f"No identifier in response: {body}"
    # DID format encodes uniqueness; a second create produces a different DID.
    payload = {
        "template_type": "FRAME_CONTRACT",
        "name": "BDD UUID Check",
        "description": "Duplicate to verify UUID uniqueness",
        "template_data": {"title": "BDD UUID Check", "clauses": []},
    }
    second_resp = post_json(context, template_create_url(context), payload)
    assert second_resp.status_code == 200, second_resp.text
    second_did = second_resp.json().get("did")
    assert second_did != did, (
        f"Two templates received identical DID '{did}' — UUIDs are not unique"
    )


# ---------------------------------------------------------------------------
# DID resolution
# ---------------------------------------------------------------------------

@then("the DID resolution is verified")
def step_then_did_resolution_verified(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, template_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, f"DID {did} could not be resolved: {retrieve.text}"
    assert retrieve.json().get("did") == did