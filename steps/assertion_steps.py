"""Shared assertion steps for executable BDD scenarios."""

import requests as _requests

from behave import then, when


@then("the contract is assigned a unique ID")
def step_then_unique_id(context):
    body = context.requests_response.json()
    assert isinstance(body, dict), body
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), body


@then("the request is denied with an authorization error")
def step_then_denied_authorization(context):
    assert context.requests_response.status_code in (401, 403), context.requests_response.text


@then('the request is denied with error "Credential invalid or access revoked"')
def step_then_denied_credential_invalid(context):
    assert context.requests_response.status_code in (401, 403), context.requests_response.text


@then("the request is denied")
def step_then_denied(context):
    assert context.requests_response.status_code in (401, 403), context.requests_response.text


# ---------------------------------------------------------------------------
# Generic HTTP response assertions
# ---------------------------------------------------------------------------

@when('the system sends "{method}" request to public endpoint "{endpoint}"')
def step_when_public_request(context, method, endpoint):
    url = f"{context.base_url}{endpoint}"
    m = method.upper()
    if m == "GET":
        context.requests_response = _requests.get(url, timeout=context.http_timeout_seconds)
    elif m == "POST":
        context.requests_response = _requests.post(url, json={}, timeout=context.http_timeout_seconds)
    else:
        raise NotImplementedError(f"Method {method} not supported in public endpoint step")


@then("the response status is {status_code:d}")
def step_then_response_status(context, status_code):
    actual = context.requests_response.status_code
    assert actual == status_code, (
        f"Expected HTTP {status_code}, got {actual}: {context.requests_response.text}"
    )


@then('the response JSON includes "{field}"')
def step_then_response_json_includes(context, field):
    body = context.requests_response.json()
    assert isinstance(body, dict), f"Response is not a JSON object: {body}"
    assert field in body, f"Field '{field}' missing from response: {body}"
    assert body[field] is not None and body[field] != "", (
        f"Field '{field}' is empty in response: {body}"
    )