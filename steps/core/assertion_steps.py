"""Shared assertion steps for executable BDD scenarios."""
import ast
import json
import re

import requests as _requests

from behave import then, when
from behave.matchers import use_step_matcher


@then("the contract is assigned a unique ID")
def step_then_unique_id(context):
    body = context.requests_response.json()
    assert isinstance(body, dict), body
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), body


@then("the request is denied with an authorization error")
def step_then_denied_authorization(context):
    assert context.requests_response.status_code in (401, 403), context.requests_response.text


@then('the request is denied because of credential expiration')
def step_then_denied_credential_invalid(context):
    response = context.requests_response.json()
    assert context.requests_response.status_code in (401, 403) and "token is expired" in response["message"], response

@then('the request is denied because of too many failed attempts')
def step_then_denied_to_many_attempts(context):
    response = context.requests_response.json()
    assert context.requests_response.status_code in (401, 403) and "too many failed attempts" in response["message"], response


@then("the request is denied")
def step_then_denied(context):
    assert context.requests_response.status_code in (401, 403), context.requests_response.text

# ---------------------------------------------------------------------------
# Generic HTTP response assertions
# ---------------------------------------------------------------------------

@when('the system sends "{method}" request to endpoint "{endpoint}" without payload')
def step_when_request(context, method, endpoint):
    url = f"{context.base_url}{endpoint}"
    m = method.upper()
    if m == "GET":
        context.requests_response = _requests.get(url, timeout=context.http_timeout_seconds)
    elif m == "POST":
        context.requests_response = _requests.post(url, json={}, timeout=context.http_timeout_seconds)
    elif m == "PUT":
        context.requests_response = _requests.put(url, json={}, timeout=context.http_timeout_seconds)
    elif m == "DELETE":
        context.requests_response = _requests.delete(url, json={}, timeout=context.http_timeout_seconds)
    else:
        raise NotImplementedError(f"Method {method} not supported in public endpoint step")


@when('the system sends "{method}" request to endpoint "{endpoint}" with "{payload}"')
def step_when_request_with_payload(context, method, endpoint, payload=None):
    url = f"{context.base_url}{endpoint}"
    m = method.upper()
    body = {}
    params = {}

    if payload:
        if payload.startswith("{"):
            # Einfache Anführungszeichen → gültiges JSON
            parsed = ast.literal_eval(payload)
            if m == "GET":
                params = parsed
            else:
                body = parsed
        elif "=" in payload:
            params = dict(p.split("=", 1) for p in payload.split("&"))

    if m == "GET":
        context.requests_response = _requests.get(
            url, params=params, timeout=context.http_timeout_seconds
        )
    elif m == "POST":
        context.requests_response = _requests.post(
            url, json=body, timeout=context.http_timeout_seconds
        )
    elif m == "PUT":
        context.requests_response = _requests.put(
            url, json=body, timeout=context.http_timeout_seconds
        )
    elif m == "DELETE":
        context.requests_response = _requests.delete(
            url, json=body, timeout=context.http_timeout_seconds
        )
    else:
        raise NotImplementedError(f"Method {method} not supported")

@when('the system sends "{method}" request to internal endpoint "{endpoint}"')
def step_when_internal_request(context, method, endpoint):
    # For internal endpoints, we ignore any path in the base URL and construct the URL directly from the endpoint to ensure it targets the correct service.
    url = "/".join(context.base_url.split("/", 3)[:3]) + endpoint
    m = method.upper()
    if m == "GET":
        context.requests_response = _requests.get(url, timeout=context.http_timeout_seconds)
    elif m == "POST":
        context.requests_response = _requests.post(url, json={}, timeout=context.http_timeout_seconds)
    else:
        raise NotImplementedError(f"Method {method} not supported in internal endpoint step")


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