"""Authentication and scenario setup steps for executable BDD scenarios."""

import os

import requests
from behave import given

from steps.support.services.template_service import TemplateService
from support.api_client import template_search_url
from support.services.auth_service import AuthService

@given('I hold an expired credential with roles: "{roles}"')
def step_given_expired_credential_with_roles(context, roles):
    role_list = [role.strip() for role in roles.split(",")]
    AuthService.set_headers_for_roles(context, role_list, use_expired_jwt=True)

@given('I am authenticated with roles: "{roles}"')
def step_given_authenticated_with_roles(context, roles):
    role_list = [role.strip() for role in roles.split(",")]
    AuthService.set_headers_for_roles(context, role_list)


@given('a system service is authenticated via API with roles: "{roles}"')
def step_given_authenticated_service_with_role(context, roles):
    role_list = [role.strip() for role in roles.split(",")]
    AuthService.set_headers_for_roles(context, role_list, username_prefix="bdd-service")


@given("a system service is authenticated via API")
def step_given_authenticated_service(context):
    token = os.getenv("BDD_DCS_TOKEN")
    assert token, "BDD_DCS_TOKEN must be set for authenticated API scenarios"
    context.headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
    }

@given('the request is denied because of too many failed attempts')
def step_given_denied_to_many_attempts(context):
    response = context.requests_response.json()
    assert context.requests_response.status_code in (401, 403) and "too many failed attempts" in response["message"], response

@given("a system service provides an invalid API key")
def step_given_invalid_api_key(context):
    context.headers = {
        "Authorization": "Bearer invalid-token",
        "Content-Type": "application/json",
    }

@given('I try to search for templates with name "{name}" "{count}"')
def step_given_search_templates(context, name, count):
    for _ in range(int(count)):
        context.requests_response = requests.get(
            template_search_url(context),
            params={"name": name},
            headers=getattr(context, "headers", {}),
            timeout=context.http_timeout_seconds,
        )

@given('template "{template_name}" is available')
def step_given_template_available(context, template_name):
    env_key = TemplateService.template_env_key(template_name)
    template_did = os.getenv(env_key)
    if not template_did:
        from steps.template_management.template_workflow_steps import (  # noqa: PLC0415
            _create_approved_template,
            _store_named,
        )

        did, updated_at = _create_approved_template(context)
        template_did = did
        _store_named(context, template_name, did, updated_at)
    if not hasattr(context, "template_dids"):
        context.template_dids = {}
    context.template_dids[template_name] = template_did


@given("the service provides contract data in the request payload")
def step_given_payload_data(context):
    context.contract_payload_extra = {"source": "bdd"}