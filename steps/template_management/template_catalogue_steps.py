"""BDD steps for the template catalogue integration endpoints (DCS-IR-SI-01,
UC-02): POST /template/publish (backend/design/template_repository.go) and
GET /catalogue/template/retrieve, /catalogue/template/retrieve/{did},
/catalogue/template/search (backend/design/template_catalogue_integration.go)
- the XFSC Federated Catalogue integration, deployed in-cluster for the BDD
harness (deployment/helm/charts/federated-catalogue), already exercised
indirectly by the passing "register" scenarios in template_workflow.feature.
"""

import time

from behave import given, then, when

from steps.support.api_client import (
    catalogue_template_retrieve_url,
    catalogue_template_search_url,
    post_json,
    template_publish_url,
)
from steps.support.services.template_service import TemplateService


@when('I publish template "{name}"')
def step_when_publish_template(context, name):
    t = TemplateService.named(context, name)
    started = time.monotonic()
    context.requests_response = post_json(
        context, template_publish_url(context), {"did": t["did"], "updated_at": t["updated_at"]}
    )
    _record_catalogue_call(context, "publish", started)
    if context.requests_response.status_code == 200:
        updated_at = TemplateService.fetch_template(context, t["did"]).get("updated_at")
        TemplateService.store_named(context, name, t["did"], updated_at)


@given('I publish template "{name}"')
def step_given_publish_template(context, name):
    # Given-position variant (used as setup by the retrieve/search scenarios):
    # unlike the When form, a failure here is a broken precondition, so assert
    # the publish succeeded instead of leaving that to a later Then.
    step_when_publish_template(context, name)
    assert context.requests_response.status_code == 200, (
        f"Publishing template '{name}' as a scenario precondition failed: "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )


@when('I attempt to publish template "{name}" with my current role')
def step_when_attempt_publish_template(context, name):
    t = TemplateService.named(context, name)
    headers = getattr(context, "headers", {})
    context.requests_response = post_json(
        context, template_publish_url(context), {"did": t["did"], "updated_at": t["updated_at"]}, headers=headers
    )


@when("I retrieve the template catalogue")
def step_when_retrieve_catalogue(context):
    import requests as _requests  # noqa: PLC0415

    started = time.monotonic()
    context.requests_response = _requests.get(
        catalogue_template_retrieve_url(context),
        params={"offset": 0, "limit": 100},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )
    _record_catalogue_call(context, "retrieve", started)


@when('I search the template catalogue by name "{name}"')
def step_when_search_catalogue(context, name):
    import requests as _requests  # noqa: PLC0415

    started = time.monotonic()
    context.requests_response = _requests.get(
        catalogue_template_search_url(context),
        params={"name": name, "offset": 0, "limit": 100},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )
    _record_catalogue_call(context, "search", started)


def _record_catalogue_call(context, operation, started):
    if not hasattr(context, "catalogue_calls"):
        context.catalogue_calls = []
    context.catalogue_calls.append(
        {"operation": operation, "duration": time.monotonic() - started}
    )


def _catalogue_items(context):
    body = context.requests_response.json()
    if isinstance(body, dict):
        items = body.get("items")
        assert isinstance(items, list), f"Expected catalogue response to carry an 'items' list, got: {body}"
        return items
    assert isinstance(body, list), f"Expected a catalogue items list, got: {body}"
    return body


def _assert_catalogue_contains_template(context, name):
    """Assert the first catalogue response.

    Readiness now includes functional FC verification, so retrying here would
    hide precisely the cold-start/timeout regression covered by AC6.
    """
    t = TemplateService.named(context, name)
    items = _catalogue_items(context)
    dids = [item.get("did") for item in items if isinstance(item, dict)]
    assert t["did"] in dids, (
        f"Expected the first catalogue response to include template '{name}' "
        f"(did={t['did']}), got dids: {dids}"
    )


@then('the catalogue result includes template "{name}"')
def step_then_catalogue_includes(context, name):
    _assert_catalogue_contains_template(context, name)


@then('the catalogue search result includes template "{name}"')
def step_then_catalogue_search_includes(context, name):
    _assert_catalogue_contains_template(context, name)
