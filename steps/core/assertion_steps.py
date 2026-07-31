"""Shared assertion steps for executable BDD scenarios."""
import ast
import json
import os
import re

import requests as _requests

from behave import given, then, when
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


@then("the request is denied")
def step_then_denied(context):
    assert context.requests_response.status_code in (401, 403), context.requests_response.text

@then('the request is denied because of too many failed attempts')
def step_then_denied_to_many_attempts(context):
    response = context.requests_response.json()
    assert context.requests_response.status_code in (401, 403) and "too many failed attempts" in response["message"], response

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
    # Internal endpoints (e.g. /metrics) live at the service root, outside the
    # API prefix; the ingress does not route them. Prefer the direct service
    # origin (port-forward set up by run_bdd_helm.sh), falling back to the
    # base URL's origin for local air/Vite runs.
    origin = os.getenv("BDD_DCS_INTERNAL_ORIGIN", "").strip().rstrip("/")
    if not origin:
        origin = "/".join(context.base_url.split("/", 3)[:3])
    url = origin + endpoint
    m = method.upper()
    if m == "GET":
        context.requests_response = _requests.get(url, timeout=context.http_timeout_seconds)
    elif m == "POST":
        context.requests_response = _requests.post(url, json={}, timeout=context.http_timeout_seconds)
    else:
        raise NotImplementedError(f"Method {method} not supported in internal endpoint step")


@given("get http 200:Success code")
def step_given_get_http_200(context):
    """Duplicate of eu.xfsc.bdd.core.steps.rest._200 (pip package
    bdd-executor), registered as @given too.

    behave registers @given/@when/@then into separate per-type lookup
    tables; the upstream bdd-executor package only registers this exact
    text under @then, so behave reports it as "undefined" wherever a
    scenario uses it as an "And" continuing a Given block (several
    scenarios in features/05_contract_deployment/contract_deployment.feature
    do exactly this, to assert an intermediate setup call succeeded before
    the Given block's actual precondition is fully built). We cannot edit
    the installed pip package's source, so this is a thin,
    behavior-identical duplicate scoped to the Given case
    only — the upstream @then registration is untouched and still handles
    every other (Then) usage across the rest of this codebase's features.
    """
    status_code = context.requests_response.status_code
    assert status_code == 200, (status_code, context.requests_response.content)


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
