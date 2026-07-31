"""Regression steps for removing legacy document numbers."""

from behave import given, then, when

from steps.support.api_client import (
    contract_create_url,
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
    template_approve_url,
    template_create_url,
    template_register_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService


def _create_legacy_template(context, register=False):
    creator = AuthService.get_headers_for_roles(["Template Creator"])
    response = post_json(
        context,
        template_create_url(context),
        {
            "document_number": "LEGACY-123",
            "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
            "name": "BDD Legacy Number Template",
            "description": "Document number removal regression",
            "template_data": TemplateService.canonical_document_data("BDD Legacy Number Template"),
        },
        headers=creator,
    )
    assert response.status_code == 200, response.text
    did = response.json()["did"]
    template = TemplateService.fetch_template(context, did, headers=creator)
    if register:
        updated_at = TemplateService.do_submit(context, did, template["updated_at"])
        updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)
        approver = AuthService.get_headers_for_roles(["Template Approver"])
        response = post_json(
            context,
            template_approve_url(context),
            {"did": did, "updated_at": updated_at},
            headers=approver,
        )
        assert response.status_code == 200, response.text
        updated_at = TemplateService.fetch_template(context, did, headers=approver)["updated_at"]
        manager = AuthService.get_headers_for_roles(["Template Manager"])
        response = post_json(
            context,
            template_register_url(context),
            {"did": did, "updated_at": updated_at},
            headers=manager,
        )
        assert response.status_code == 200, response.text
    return did, template


def _assert_no_document_number(value):
    if isinstance(value, dict):
        forbidden = {"document_number", "documentNumber", "DocumentNumber"}.intersection(value)
        assert not forbidden, f"Legacy document number fields remain: {forbidden}"
        for child in value.values():
            _assert_no_document_number(child)
    elif isinstance(value, list):
        for child in value:
            _assert_no_document_number(child)


@when("I create a template with a legacy document number")
def step_create_template_with_legacy_number(context):
    context.legacy_template_did, context.requests_body = _create_legacy_template(context)


@then("the template is identified by DID and version without a document number")
def step_template_without_document_number(context):
    assert context.requests_body.get("did") == context.legacy_template_did
    assert context.requests_body.get("version") is not None
    _assert_no_document_number(context.requests_body)


@given("a registered template with a legacy document number exists")
def step_registered_template_with_legacy_number(context):
    context.legacy_template_did, _ = _create_legacy_template(context, register=True)


@when("I generate a contract from that legacy template")
def step_generate_contract_from_legacy_template(context):
    headers = AuthService.get_headers_for_roles(["Contract Creator"])
    peer_did = ContractService._local_peer_did(context)
    response = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": context.legacy_template_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        },
        headers=headers,
    )
    assert response.status_code == 200, response.text
    context.requests_response = get_with_headers(
        context,
        contract_retrieve_by_id_url(context, response.json()["did"]),
        headers=headers,
    )


@then("the contract keeps its template reference without a document number")
def step_contract_without_document_number(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    body = context.requests_response.json()
    assert body.get("template_did") == context.legacy_template_did
    assert body.get("template_version") is not None
    _assert_no_document_number(body)
