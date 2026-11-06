"""BDD steps for DCS-FR-TR-22 template-update notifications: the webhook
platform (backend/internal/webhookplatform) is mounted at /orce/ on the
service ROOT (backend/cmd/dcs/http.go outerMux), not under DCS_API_PATH —
hence origin_url(). The deliveries go to the in-process BDD webhook receiver
(steps/support/services/webhook_receiver_service.py), which verifies the
Standard Webhooks signature with the secret from the 201 response.
"""

import base64
import time

import requests as _requests
from behave import given, then, when

from steps.support.api_client import origin_url
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService
from steps.support.services.webhook_receiver_service import WebhookReceiverService

# Requests that DCS refuses never deliver to this URL.
UNUSED_CALLBACK_URL = "https://receiver.example/hook"


def _webhook_platform_url(context, path: str) -> str:
    return f"{origin_url(context.base_url)}/orce{path}"


@given('a webhook subscription for "{event}" events pointing at the BDD webhook receiver')
def step_given_webhook_subscription(context, event):
    headers = AuthService.get_headers_for_roles(["Integration Manager"])
    context.webhook_receiver_path, callback_url = WebhookReceiverService.new_endpoint()
    resp = _requests.post(
        _webhook_platform_url(context, "/webhooks"),
        json={"event": event, "callback_url": callback_url},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 201, (
        f"webhook subscription for '{event}' failed: {resp.status_code} {resp.text}"
    )
    sub = resp.json()
    context.webhook_subscription_id = sub.get("id")
    assert context.webhook_subscription_id, f"subscription response has no id: {resp.text}"
    # DCS generates the Standard Webhooks signing secret and returns it only
    # here. The receiver verifies every delivery to this subscription with it.
    secret = sub.get("secret") or ""
    assert secret.startswith("whsec_") and len(base64.b64decode(secret[len("whsec_") :])) == 32, (
        f"subscription {context.webhook_subscription_id} response has no whsec_ secret with a 32-byte key"
    )
    WebhookReceiverService.set_secret(context.webhook_receiver_path, secret)

    def _unsubscribe():
        _requests.delete(
            _webhook_platform_url(context, f"/webhooks/{context.webhook_subscription_id}"),
            headers=AuthService.get_headers_for_roles(["Integration Manager"]),
            timeout=context.http_timeout_seconds,
        )

    # Webhook subscriptions are event-wide: leaving one behind would make the
    # platform notify the receiver for every later scenario's template updates.
    context.add_cleanup(_unsubscribe)


def _assert_delivery(context, event, did, subject):
    # GET /deliveries lists the deliveries of the caller organization's
    # subscriptions and needs a subscription management role.
    headers = AuthService.get_headers_for_roles(["Integration Manager"])
    path = context.webhook_receiver_path
    # The outbox publisher reads unpublished events on a ~1s ticker and the
    # dispatcher fans out asynchronously, so poll the receiver and the
    # platform's delivery log.
    deadline = time.monotonic() + 120
    received, logged = [], []
    while time.monotonic() < deadline:
        received = [
            d
            for d in WebhookReceiverService.deliveries(path)
            if (d["payload"] or {}).get("event") == event and (d["payload"] or {}).get("did") == did
        ]
        resp = _requests.get(
            _webhook_platform_url(context, "/deliveries"),
            headers=headers,
            timeout=context.http_timeout_seconds,
        )
        assert resp.status_code == 200, f"GET /deliveries failed: {resp.status_code} {resp.text}"
        logged = [
            d
            for d in resp.json() or []
            if d.get("event") == event and d.get("did") == did and d.get("callback_url", "").endswith(path)
        ]
        if any(d["verified"] for d in received) and any(d.get("status_code") == 200 for d in logged):
            return
        time.sleep(1)
    assert False, (
        f"Expected a '{event}' notification for {subject} ({did}) with a valid Standard Webhooks "
        f"signature at the BDD receiver and HTTP 200 in the delivery log within 120s; "
        f"received: {received}; logged: {logged}"
    )


@then('the "{event}" notification for template "{name}" is delivered to the BDD webhook receiver with a valid signature')
def step_then_notification_delivered(context, event, name):
    t = TemplateService.named(context, name)
    _assert_delivery(context, event, t["did"], f"template '{name}'")


@then('the "{event}" notification for contract "{name}" is delivered to the BDD webhook receiver with a valid signature')
def step_then_contract_notification_delivered(context, event, name):
    # DCS-FR-CSA-20 / DCS-FR-CWE-10: the expiry cron's CONTRACT_EXPIRED
    # event reaches API subscribers as a "contract.expired" alert.
    did, _ = ContractService._contract_data(context, name)
    _assert_delivery(context, event, did, f"contract '{name}'")


@when('a webhook subscription for the unknown event "{event}" is attempted')
def step_when_subscribe_unknown_event(context, event):
    headers = AuthService.get_headers_for_roles(["Integration Manager"])
    context.requests_response = _requests.post(
        _webhook_platform_url(context, "/webhooks"),
        json={"event": event, "callback_url": UNUSED_CALLBACK_URL},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@then("the webhook subscription is rejected as unknown")
def step_then_subscription_rejected_unknown(context):
    resp = context.requests_response
    assert resp.status_code == 400, f"Expected 400 for unknown event, got {resp.status_code}: {resp.text}"
    assert "unknown event" in (resp.json().get("error") or ""), f"Expected 'unknown event' error, got: {resp.text}"


@when('an unauthenticated webhook subscription for "{event}" is attempted')
def step_when_subscribe_unauthenticated(context, event):
    context.requests_response = _requests.post(
        _webhook_platform_url(context, "/webhooks"),
        json={"event": event, "callback_url": UNUSED_CALLBACK_URL},
        timeout=context.http_timeout_seconds,
    )


@then("the webhook subscription is rejected as unauthorized")
def step_then_subscription_rejected_unauthorized(context):
    resp = context.requests_response
    assert resp.status_code == 401, f"Expected 401 without a token, got {resp.status_code}: {resp.text}"


@given('contract "{name}" carries the expiration policy "{policy}" (cron-processing test seam)')
def step_given_contract_expiration_policy(context, name, policy):
    """The expiry cron only processes (persists EXPIRED + emits
    CONTRACT_EXPIRED for) contracts with an expiration policy —
    callExpirationLogic hard-fails on a nil exp_policy
    (contractworkflowengine/cronjobs.go). contract/update cannot set the
    policy on a SIGNED contract (EventUpdate only from Draft), so the same
    context.db seam the expiry-backdate step uses seeds it."""
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute("UPDATE contracts SET exp_policy = %s WHERE did = %s", (policy.strip().upper(), did))
    context.db.commit()
    cursor.close()
