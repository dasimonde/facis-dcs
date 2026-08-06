"""Authentication and scenario setup steps for executable BDD scenarios."""

import os
import time
from datetime import datetime

import requests
from behave import given, then, when

from steps.support.services.template_service import TemplateService
from steps.support.status_list_probe import (
    assert_refused_for_the_revoked_bit,
    revoke_credential_bit,
)
from support.api_client import (
    get_with_headers,
    pac_audit_timeline,
    pac_audit_url,
    post_json,
    template_search_url,
)
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
    AuthService.set_headers_for_roles(context, role_list)


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


# ----------------------------------------------------------------------
# Federated OID4VP login walked step by step (initiate -> Hydra challenge
# -> wallet VP -> token). Each step drives one stage of the same headless
# flow AuthService.exchange_roles_for_access_token composes, but without
# its token cache: the login asserted on is always performed in-scenario.
# ----------------------------------------------------------------------

def _federated_timeout(context) -> float:
    return float(getattr(context, "http_timeout_seconds", os.getenv("BDD_HTTP_TIMEOUT_SECONDS", "60")))


@when("I initiate a federated login")
def step_when_initiate_federated_login(context):
    session = requests.Session()
    session.headers.update({
        "User-Agent": "bdd-auth-service",
        "Accept": "application/json",
    })
    context.federated_api_base = getattr(
        context, "base_url", os.getenv("BDD_DCS_BASE_URL", "http://localhost:5173/api")
    )
    context.federated_session = session
    context.federated_initiation = AuthService.initiate_login(
        session,
        context.federated_api_base,
        timeout=_federated_timeout(context),
    )


@when("I bind the Hydra login challenge to the pending presentation")
def step_when_bind_hydra_login_challenge(context):
    AuthService.bind_hydra_login_challenge(
        context.federated_session,
        context.federated_api_base,
        state=context.federated_initiation.state,
        authorize_url=context.federated_initiation.authorize_url,
        timeout=_federated_timeout(context),
    )


@when('I present a wallet credential with roles: "{roles}"')
def step_when_present_wallet_credential(context, roles):
    timeout = _federated_timeout(context)
    credentials = AuthService.parse_auth_credentials(
        [role.strip() for role in roles.split(",")]
    )
    auth_request = AuthService.fetch_authorization_request(
        context.federated_session,
        context.federated_initiation.request_uri,
        timeout=timeout,
    )
    vp_token = AuthService.build_vp_token(
        credentials,
        nonce=auth_request.nonce,
        client_id=auth_request.client_id,
    )
    context.federated_redirect_uri = AuthService.submit_presentation(
        context.federated_session,
        api_base=context.federated_api_base,
        response_uri=auth_request.response_uri,
        state=auth_request.state,
        query_id=auth_request.query_id,
        vp_token=vp_token,
        timeout=timeout,
    )


@when("I complete the federated session and obtain an access token")
def step_when_complete_federated_session(context):
    access_token, _ = AuthService.complete_session(
        context.federated_session,
        context.federated_api_base,
        context.federated_redirect_uri,
        timeout=_federated_timeout(context),
    )
    assert access_token, "federated login completed without an access token"
    context.federated_access_token = access_token
    context.headers = {
        "Authorization": f"Bearer {access_token}",
        "Content-Type": "application/json",
    }


@then("the access token authorizes an authenticated API call")
def step_then_token_authorizes_api_call(context):
    response = get_with_headers(context, template_search_url(context), headers=context.headers)
    assert response.status_code == 200, (
        f"expected the federated access token to authorize GET /template/search, "
        f"got {response.status_code}: {response.text[:300]}"
    )
    assert isinstance(response.json(), list), (
        f"expected a template search result list, got: {response.text[:300]}"
    )


@then("the login presentation audit event records the actor and timestamp")
def step_then_login_presentation_audited(context):
    # The successful wallet presentation is recorded as an
    # OID4VP_PRESENTATION_SUCCEEDED event under component SYSTEM, keyed by
    # the presentation state issued at /auth/login (auth_login.go
    # PresentationCallback -> auth/audit.Recorder). Anchoring is async
    # (outbox -> TSA -> IPFS), hence the poll.
    state = context.federated_initiation.state
    headers = AuthService.get_headers_for_roles(["Auditor"])
    matches = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        response = post_json(
            context,
            pac_audit_url(context),
            {"scope": "SYSTEM", "justification": "BDD authentication audit"},
            headers=headers,
        )
        assert response.status_code == 200, (
            f"SYSTEM-scope audit failed: {response.status_code} {response.text[:300]}"
        )
        matches = [
            entry
            for entry in pac_audit_timeline(response)
            if entry.get("event_type") == "OID4VP_PRESENTATION_SUCCEEDED"
            and entry.get("did") == state
        ]
        if matches:
            break
        time.sleep(2)
    assert matches, (
        f"expected an OID4VP_PRESENTATION_SUCCEEDED audit event for presentation "
        f"state '{state}' in the SYSTEM audit trail"
    )
    entry = matches[0]
    event_data = entry.get("event_data") or {}
    assert event_data.get("subject_did"), (
        f"expected the presentation audit event to identify the actor via "
        f"subject_did, got event_data: {event_data}"
    )
    assert event_data.get("occurred_at"), (
        f"expected the presentation audit event to carry an occurred_at "
        f"timestamp, got event_data: {event_data}"
    )
    created_at = entry.get("created_at") or ""
    datetime.fromisoformat(created_at.replace("Z", "+00:00"))

@when('I present a wallet credential with roles "{roles}" whose status-list index is revoked')
def step_when_present_revoked_wallet_credential(context, roles):
    """Builds the same vp_token the happy-path presentation step builds,
    revokes ITS credential's status-list index, then posts the direct_post
    raw — the login callback verifies synchronously (auth_login.go
    PresentationCallback -> oid4vp.Verify, status-list step), so the
    rejection arrives on this response."""
    import json as _json  # noqa: PLC0415

    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.credential import decode_jwt_payload  # noqa: PLC0415
    from dcs_wallet.sdjwt import split_sd_jwt  # noqa: PLC0415

    timeout = _federated_timeout(context)
    # A DEDICATED organization isolates this credential's status-list index:
    # it holds a RESERVED_INDEX of its own (dcs_wallet.status_list), so
    # revoking it can poison only this probe identity and never the
    # suite-shared login credentials — the issuer keeps revocations for the
    # whole run.
    credentials = AuthService.parse_auth_credentials(
        [role.strip() for role in roles.split(",")],
        organization="BDD Revocation Probe Org",
    )
    auth_request = AuthService.fetch_authorization_request(
        context.federated_session,
        context.federated_initiation.request_uri,
        timeout=timeout,
    )
    vp_token = AuthService.build_vp_token(
        credentials,
        nonce=auth_request.nonce,
        client_id=auth_request.client_id,
    )

    revoke_credential_bit(context, decode_jwt_payload(split_sd_jwt(vp_token)[0]))

    context.requests_response = context.federated_session.post(
        auth_request.response_uri,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        data={
            "state": auth_request.state,
            "vp_token": _json.dumps({auth_request.query_id: [vp_token]}, separators=(",", ":")),
        },
        timeout=timeout,
    )


@then("the login presentation is rejected for a revoked credential")
def step_then_login_rejected_revoked(context):
    assert_refused_for_the_revoked_bit(context, context.requests_response, "the login presentation")
