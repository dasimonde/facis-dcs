"""Steps for the SRS System User classes (SRS §2.4 Table 5, ADR-16, ADR-17).

A System User is a machine: it holds no wallet, runs no OID4VP ceremony, and
authenticates against this deployment's Hydra with its own client_id/secret
(client_credentials). What it may do is fixed by the DCS deployment
(`systemClients` in the Helm values), never asked for in the token — so these
steps deliberately obtain a REAL token from Hydra rather than minting one, and
then check what DCS actually lets that identity do.

Client secrets mirror deployment/helm/values.bdd.yml; overridable per client via
BDD_SYSTEM_CLIENT_SECRET_<client id with - replaced by _, uppercased>.
"""

import json
import os
import time

import requests
from behave import then, when

# One client per capability (values.bdd.yml hydra.clients + systemClients).
SYSTEM_CLIENT_SECRETS = {
    "dcs-orce-notary": "dcs-orce-notary-secret",
    "dcs-orce-creator": "dcs-orce-creator-secret",
    "dcs-orce-manager": "dcs-orce-manager-secret",
    "dcs-orce-signer": "dcs-orce-signer-secret",
}


def _token_url() -> str:
    """Hydra's token endpoint, routed through the same ingress as the API."""
    explicit = os.getenv("BDD_HYDRA_TOKEN_URL", "").strip()
    if explicit:
        return explicit
    base = os.getenv("BDD_DCS_BASE_URL", "http://localhost:5173/api").rstrip("/")
    origin = "/".join(base.split("/")[:3])
    return f"{origin}/oauth2/token"


def _client_secret(client_id: str) -> str:
    env_key = "BDD_SYSTEM_CLIENT_SECRET_" + client_id.replace("-", "_").upper()
    override = os.getenv(env_key, "").strip()
    if override:
        return override
    secret = SYSTEM_CLIENT_SECRETS.get(client_id)
    if not secret:
        raise AssertionError(
            f"no secret known for system client '{client_id}' — add it to "
            f"SYSTEM_CLIENT_SECRETS or set {env_key}"
        )
    return secret


@when('the system client "{client_id}" obtains an access token')
def step_system_client_obtains_token(context, client_id):
    response = requests.post(
        _token_url(),
        data={
            "grant_type": "client_credentials",
            "client_id": client_id,
            "client_secret": _client_secret(client_id),
        },
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        timeout=30,
    )
    assert response.status_code == 200, (
        f"Hydra refused the client_credentials grant for '{client_id}': "
        f"{response.status_code} {response.text}"
    )
    token = response.json().get("access_token")
    assert token, f"no access_token in Hydra's response for '{client_id}': {response.text}"
    context.system_client_id = client_id
    context.system_client_token = token


@then("the system client holds a machine access token")
def step_system_client_has_token(context):
    token = getattr(context, "system_client_token", "")
    assert token, "no system client token was obtained"
    # Three dot-separated parts and a real algorithm: a signed JWT from Hydra,
    # not one this suite minted for itself.
    parts = token.split(".")
    assert len(parts) == 3 and parts[2], f"token is not a signed JWT: {token[:40]}..."


def _request(context, method: str, path: str, body=None):
    token = getattr(context, "system_client_token", "")
    assert token, "no system client token was obtained"
    url = f"{context.base_url}{path}"
    context.requests_response = requests.request(
        method,
        url,
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
        json=body,
        timeout=60,
    )


@when('the system client requests GET "{path}"')
def step_system_client_gets(context, path):
    _request(context, "GET", path)


@when('the system client requests POST "{path}" with body')
def step_system_client_posts_with_body(context, path):
    """POST with a well-formed body.

    The body must satisfy the endpoint's payload shape or goa rejects the
    request at the transport layer with 400 missing_field, BEFORE the security
    scheme runs — so an empty body tests nothing about authorization. The
    values themselves are irrelevant: authorization is decided before the
    handler ever looks for the contract.
    """
    _request(context, "POST", path, body=json.loads(context.text))


@then("the system client request is refused as forbidden")
def step_system_client_forbidden(context):
    status = context.requests_response.status_code
    assert status == 403, (
        f"expected the system client '{getattr(context, 'system_client_id', '?')}' to be "
        f"refused with 403, got {status}: {context.requests_response.text[:400]}"
    )


@then("the audit checkpoint head carries a Merkle root")
def step_checkpoint_head_has_root(context):
    # Anchoring is asynchronous (the outbox processor batches on a ~1s tick), so
    # the very first checkpoint of a run can still be in flight: 404 means "not
    # yet", not "broken". Poll rather than race it.
    response = context.requests_response
    deadline = time.monotonic() + 60
    while response.status_code == 404 and time.monotonic() < deadline:
        time.sleep(1)
        _request(context, "GET", "/pac/audit/checkpoint/head")
        response = context.requests_response
    assert response.status_code == 200, (
        f"checkpoint head unavailable: {response.status_code} {response.text[:400]}"
    )
    head = response.json()
    assert head.get("root"), f"no Merkle root in the checkpoint head: {head}"
    assert int(head.get("leaf_count", 0)) >= 1, f"checkpoint commits to no entries: {head}"
    assert int(head.get("seq", 0)) >= 1, f"checkpoint has no sequence number: {head}"
    # The head is what gets published to an external anchor, so it must carry
    # nothing derived from the entries it commits to (ADR-16).
    for leaked in ("leaf_cids", "leaf_hashes", "event_data", "did"):
        assert leaked not in head, f"checkpoint head leaks '{leaked}': {head}"
    context.checkpoint_head = head
