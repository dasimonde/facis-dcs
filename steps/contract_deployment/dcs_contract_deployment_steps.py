"""BDD steps for contract deployment, execution evidence, and KPIs
(features/05_contract_deployment; SRS DCS-FR-SM-10/-12,
DCS-FR-CWE-06/-09/-20/-31, DCS-IR-SI-02/-05).

Endpoint surface (backend/design/contract_workflow_engine.go):

1. `POST /contract/deploy` (manual deploy trigger, UC-05-01): payload
   `{"did", "updated_at"}`, gated to `SIGNED` contracts, role "Contract
   Manager". The deploy response echoes the payload actually sent to the
   target: `{"did", "contract_version", "content_hash", "timestamp",
   "correlation_id", "payload": {...the machine-readable JSON-LD, including
   an "@type": "odrl:Set" node...}}`. That echo is the test seam this pack
   uses for the payload-shape assertions (no local HTTP-capture server is
   set up for the outbound call; see below).

2. `POST /contract/deployment/callback` (target -> DCS, DCS-IR-SI-05):
   payload `{"did", "correlation_id", "status", ...}` (ack/status update)
   or `{"did", "correlation_id", "kpi": {"metric", "value"}}` (KPI report),
   authenticated as the target's own OAuth2 client (ADR-27): a bearer token
   from the client_credentials grant, and accepted only for deployments
   dispatched to that target. One shared secret proved merely that SOME
   target was calling, so any registered target could acknowledge another's
   deployment. (The signing ceremony's callback authenticates differently
   again, by unguessable ceremony id plus ADR-20 nonce binding — see
   steps/real_signing_vertical.)

3. `GET /contract/retrieve/{did}` carries a `"kpis"` field (list of
   `{"metric", "value", "observed_at", "violation"}`). SLA violations are
   asserted as a per-KPI `"violation": true` marker OR the metric name
   appearing in a top-level `"kpi_violations"` list.

4. Archive entries (`GET /archive/search?did=...`) carry an `"evidence"`
   JSON object with a nested `"deployment"` object:
   `{"correlation_id", "payload_hash", "receipt_hash", "tsa_token",
   "activated_at"}` (see `command.BuildArchiveEntry`,
   backend/internal/contractworkflowengine/command/archive.go).

Why no local HTTP-capture server for the outbound deploy-to-target call:
this BDD suite runs either against a locally-run `air` backend
(dev-stack.sh, same WSL host as this test process: reachable) or against a
Helm/kind-deployed backend pod (run_bdd_helm.sh, a different network
namespace: NOT reachable from a plain `http.server` bound to this test
process's localhost). Since this pack must run in both environments, it
relies on the deploy endpoint's own response echo as the test seam. The
ORCE scenario is the genuine end-to-end counterpart: it talks to the
actual shipped ORCE contract-target-flow directly (a real, independently
reachable service), which does not have this networking problem; see
`BDD_ORCE_TARGET_URL` below.

The force-set DB seam: "an archived + ACTIVE contract still appears in the
live list" is deliberately tested WITHOUT going through the deploy/ORCE/
callback chain. Forcing `state='ACTIVE'` directly via the shared test DB
connection (context.db, see environment.py) isolates the behavior under
test (archived must not be treated as inactive) from the deploy mechanism,
which the other scenarios exercise directly. This mirrors the accepted
precedent of direct-DB seams for preconditions/assertions the API has no
fast/existing path to establish or observe (steps/peer_trust/
dcs_trust_pdp_steps.py's read-only `sync_fails` polling — ADR-19 — and
steps/template_management/contract_state_machine_steps's exp_date
backdate).

ORCE reachability: `BDD_ORCE_TARGET_URL` (no default) must point at the
deployed contract-target-flow's HTTP-in endpoint
(deployment/helm/charts/orce/flows/contract-target-flow.json). If unset,
the ORCE scenario fails fast with an explicit message naming the missing
wiring. This is a single, real, independently reachable ORCE service, not
a second DCS instance, so it is NOT tagged @two-instance.
"""

import base64
import hashlib

import jcs
import json
import os
import time

import requests as _requests
from behave import given, step, then, when

from steps.support.api_client import (
    archive_search_url,
    contract_deploy_url,
    contract_deployment_callback_url,
    contract_retrieve_by_id_url,
    contract_target_designate_url,
    contract_targets_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.template_management.contract_state_machine_steps import (
    _advance_to_approved,
    _apply_signature as _apply_signature_via_ceremony,
)

# The target authenticates as its own registered OAuth2 client (ADR-27). One
# shared secret proved only that SOME target was calling, so any registered
# target could acknowledge another's deployment.
TARGET_CLIENT_ID = os.getenv("BDD_TARGET_CLIENT_ID", "dcs-orce-target")
TARGET_CLIENT_SECRET = os.getenv("BDD_TARGET_CLIENT_SECRET", "dcs-orce-target-secret")


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _target_access_token(client_id: str = TARGET_CLIENT_ID, secret: str = TARGET_CLIENT_SECRET) -> str:
    """A client-credentials token for the contract target, the same grant a real
    target system uses before calling back."""
    response = _requests.post(
        _hydra_token_url(),
        data={
            "grant_type": "client_credentials",
            "client_id": client_id,
            "client_secret": secret,
        },
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        timeout=30,
    )
    assert response.status_code == 200, (
        f"Hydra refused the client_credentials grant for '{client_id}': "
        f"{response.status_code} {response.text}"
    )
    token = response.json().get("access_token")
    assert token, f"no access_token for '{client_id}': {response.text}"
    return token


def _hydra_token_url() -> str:
    explicit = os.getenv("BDD_HYDRA_TOKEN_URL", "").strip()
    if explicit:
        return explicit
    base = os.getenv("BDD_DCS_BASE_URL", "http://localhost:5173/api").rstrip("/")
    origin = "/".join(base.split("/")[:3])
    return f"{origin}/oauth2/token"


def _target_callback_headers() -> dict:
    return {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {_target_access_token()}",
    }


def _archive_entry_for(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    resp = get_with_headers(context, f"{archive_search_url(context)}?did={did}", headers=headers)
    assert resp.status_code == 200, (
        f"Archive search failed for contract '{name}': {resp.status_code} {resp.text}"
    )
    entries = resp.json()
    assert isinstance(entries, list), f"Expected archive search to return a list, got: {entries}"
    for entry in entries:
        if entry.get("did") == did:
            return entry
    return None


def _find_odrl_set(node) -> bool:
    """Recursively search a JSON-serializable structure for an odrl:Set
    node. The exact key path the deploy response nests the machine-readable
    payload under is not pinned by the assertion, so this walks the whole
    structure instead of relying on one specific key path."""
    if isinstance(node, dict):
        if node.get("@type") == "odrl:Set":
            return True
        return any(_find_odrl_set(v) for v in node.values())
    if isinstance(node, list):
        return any(_find_odrl_set(item) for item in node)
    return False


def _kpi_entries(retrieve_json: dict) -> list:
    kpis = retrieve_json.get("kpis")
    return kpis if isinstance(kpis, list) else []


def _kpi_violation_names(retrieve_json: dict) -> list:
    violations = retrieve_json.get("kpi_violations")
    return violations if isinstance(violations, list) else []


def _odrl_bound_field_iri(context, name: str) -> str:
    """The @id of the contract's ODRL-bound contract field — the node IRI a
    KPI reports against (EvaluateKPIViolation binds by this IRI, not a
    label). Read straight from the stored contract so it reflects the @id the
    backend rebased the fixture's urn: node to."""
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
    assert retrieve.status_code == 200, retrieve.text
    fields = (retrieve.json().get("contract_data") or {}).get("dcs:contractFields") or []
    assert fields, f"contract '{name}' declares no dcs:ContractField to bind a KPI to"
    return fields[0]["@id"]


# ---------------------------------------------------------------------------
# Given — force-set DB seam
# ---------------------------------------------------------------------------


@given(
    'contract "{name}" is force-set to state "{state}" directly in the '
    "database (pre-deploy test seam, bypassing the deployment chain)"
)
def step_given_force_state(context, name, state):
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute("UPDATE contracts SET state = %s WHERE did = %s", (state.strip().upper(), did))
    context.db.commit()
    cursor.close()
    ContractService._refresh_contract(context, name)


# ---------------------------------------------------------------------------
# Given — full submit/review/approve/sign chain as a precondition
# ---------------------------------------------------------------------------


@step('contract "{name}" is submitted, reviewed, approved, and signed via the standard workflow')
def step_given_full_workflow_to_signed(context, name):
    # Reuses the ceremony-aware helpers from contract_state_machine_steps.py
    # / real_signing_vertical rather than re-implementing the
    # submit -> review -> approve -> sign chain a third time.
    contract_dids = getattr(context, "contract_dids", None) or {}
    if name not in contract_dids:
        ContractService._create_contract_in_draft(context, name)
    _advance_to_approved(context, name)
    _apply_signature_via_ceremony(context, name)


# ---------------------------------------------------------------------------
# Given/When — manual deploy trigger
# ---------------------------------------------------------------------------


BDD_TARGET_NAME = "BDD Contract Target"


def _registered_target_id(context) -> str:
    """The registry entry for the shipped ORCE contract-target flow (ADR-25).

    Deployment used to address one endpoint from CONTRACT_TARGET_URL; it now
    goes to a target the contract designates, so the suite registers one once
    and reuses it. Registration is idempotent by name.
    """
    cached = getattr(context, "bdd_target_id", None)
    if cached:
        return cached
    admin_h = AuthService.get_headers_for_roles(["Sys. Administrator"])
    listed = get_with_headers(context, contract_targets_url(context), headers=admin_h)
    assert listed.status_code == 200, f"could not list contract targets: {listed.status_code} {listed.text}"
    for entry in listed.json() or []:
        if entry.get("name") == BDD_TARGET_NAME:
            context.bdd_target_id = entry["id"]
            return context.bdd_target_id

    created = post_json(
        context,
        contract_targets_url(context),
        {
            "name": BDD_TARGET_NAME,
            "url": os.getenv("BDD_CONTRACT_TARGET_URL", "http://dcs-orce:1880/contract-target/deploy"),
            "description": "Shipped ORCE contract-target flow used by the BDD suite.",
            "enabled": True,
        },
        headers=admin_h,
    )
    assert created.status_code == 200, f"could not register the contract target: {created.status_code} {created.text}"
    context.bdd_target_id = created.json()["id"]
    return context.bdd_target_id


def _ensure_target_designated(context, name, target_id=None):
    """Point the contract at a registered target so it has somewhere to deploy.

    A contract designates its own destination (ADR-25) because the automatic
    trigger on signing completion has no human present to choose one.
    """
    ContractService._refresh_contract(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    resolved = target_id if target_id is not None else _registered_target_id(context)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(
        context,
        contract_target_designate_url(context),
        {"did": did, "updated_at": updated_at, "target_id": resolved},
        headers=manager_h,
    )
    assert resp.status_code == 200, (
        f"could not designate a target system for contract '{name}' with "
        f"updated_at token {updated_at!r}: {resp.status_code} {resp.text}"
    )
    ContractService._refresh_contract(context, name)


@step('contract "{name}" deploys to the configured target system')
def step_contract_deploys_to_configured_target(context, name):
    """Point the contract at the seeded target (ADR-25).

    Stated explicitly rather than folded into the workflow steps because
    deployment is opt-in: a contract that designates no target simply does not
    deploy, which is an ordinary outcome for a party that only holds an
    agreement. A scenario that expects deployment has to say where to.
    """
    _ensure_target_designated(context, name)


@step('an authorized user deploys contract "{name}" to the configured contract target')
def step_when_deploy_contract(context, name):
    # Several scenarios use this step as an "And" continuing a *Given* block
    # (asserting an intermediate setup call succeeded before the scenario's
    # actual precondition is fully built). behave's Given/When/Then
    # decorators register into separate per-type lookup tables, so a step
    # registered only via @when is "undefined" when behave looks it up as a
    # Given; `@step` registers this text under given/when/then alike, and
    # the step is also genuinely used as a real When.
    _ensure_target_designated(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(
        context, contract_deploy_url(context), {"did": did, "updated_at": updated_at}, headers=manager_h
    )
    if context.requests_response.status_code == 200:
        body = context.requests_response.json()
        context.deployment_correlation_id = body.get("correlation_id")
        context.deployment_content_hash = body.get("content_hash")
        ContractService._refresh_contract(context, name)


@then("the deployment response includes a correlation ID")
def step_then_deployment_response_has_correlation_id(context):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the deploy request to succeed, got {resp.status_code}: {resp.text}"
    )
    body = resp.json()
    correlation_id = body.get("correlation_id")
    assert correlation_id, f"Expected a non-empty 'correlation_id' in the deploy response, got: {body}"


@then(
    'the deployment response declares the contract DID, version, content '
    'hash, timestamp, and the odrl:Set policy for "{name}"'
)
def step_then_deployment_payload_declared(context, name):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the deploy request to succeed, got {resp.status_code}: {resp.text}"
    )
    did, _ = ContractService._contract_data(context, name)
    body = resp.json()
    assert body.get("did") == did, f"Expected deploy response 'did' to equal {did!r}, got: {body.get('did')!r}"
    assert body.get("contract_version") is not None, f"Expected 'contract_version' in deploy response: {body}"
    content_hash = body.get("content_hash")
    assert content_hash, f"Expected a non-empty 'content_hash' in the deploy response: {body}"
    assert body.get("timestamp"), f"Expected a non-empty 'timestamp' in the deploy response: {body}"
    assert _find_odrl_set(body), (
        "Expected the deployment payload declared in the deploy response to include an "
        f"odrl:Set node (FR-SM-12 / F1's odrl:Set-enclosed policy shape) somewhere: {body}"
    )


# ---------------------------------------------------------------------------
# Given — automatic, event-driven deployment
# ---------------------------------------------------------------------------


@then('the archive entry for contract "{name}" records an automatic deployment correlation ID')
def step_then_archive_records_auto_deployment(context, name):
    # The dispatch is event-driven (outbox anchor -> NATS -> deploy
    # subscriber), so the deployment row lands asynchronously after SIGNED —
    # poll like the other async-evidence assertions instead of racing it.
    import time as _time  # noqa: PLC0415

    entry, evidence, deployment, correlation_id = None, {}, {}, None
    deadline = _time.monotonic() + 60
    while _time.monotonic() < deadline:
        entry = _archive_entry_for(context, name)
        evidence = (entry or {}).get("evidence") or {}
        deployment = evidence.get("deployment") or {}
        correlation_id = deployment.get("correlation_id")
        if correlation_id:
            break
        _time.sleep(2)
    assert entry is not None, (
        f"Expected an archive entry for contract '{name}' after the signing workflow completed "
        "(archive entry is created on SIGNED) — none was found"
    )
    assert correlation_id, (
        f"Expected the archive entry's evidence.deployment.correlation_id to be populated "
        f"automatically (event-driven, via the NATS outbox subscriber) without any explicit "
        f"POST /contract/deploy call in this scenario, got evidence: {evidence!r}"
    )


# ---------------------------------------------------------------------------
# When/Then — callback caller authorisation
# ---------------------------------------------------------------------------


@when('another registered system sends a deployment callback for contract "{name}"')
def step_when_callback_invalid_secret(context, name):
    did, _ = ContractService._contract_data(context, name)
    payload = {
        "did": did,
        "correlation_id": getattr(context, "deployment_correlation_id", None) or "bdd-unknown-correlation",
        "status": "ACKNOWLEDGED",
    }
    # A token from a client that is not this deployment's target: it
    # authenticates perfectly well and still may not speak for this target.
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {_target_access_token('dcs-orce-notary', 'dcs-orce-notary-secret')}",
    }
    context.requests_response = post_json(context, contract_deployment_callback_url(context), payload, headers=headers)


@then("the callback request is rejected because that caller is not this deployment's target")
def step_then_callback_rejected(context):
    resp = context.requests_response
    assert resp.status_code in (401, 403), (
        "Expected POST /contract/deployment/callback to reject a caller that is not the "
        f"target this deployment was dispatched to, got {resp.status_code}: {resp.text}"
    )


# ---------------------------------------------------------------------------
# When — the REAL target acknowledgement (ORCE flow callback legs)
# ---------------------------------------------------------------------------


@step('the contract target acknowledges the deployment of contract "{name}"')
def step_when_target_acknowledges(context, name):
    """The real acknowledgement path (DCS-FR-SM-10/-12, DCS-IR-SI-02): the
    backend dispatched the deployment to the shipped ORCE
    contract-target-flow (CONTRACT_TARGET_URL), whose callback legs POST the
    authoritative ack (status + execution-evidence receipt) back to
    /contract/deployment/callback as its own registered client. Nothing is
    simulated here — this step only waits for that ack to land, observable
    as the SIGNED -> ACTIVE transition it drives. Registered as @step so it
    reads naturally in Given ("And the contract target acknowledges ...")
    and When positions alike."""
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    actual_state = None
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
        assert retrieve.status_code == 200, retrieve.text
        actual_state = str(retrieve.json().get("state", "")).upper()
        if actual_state == "ACTIVE":
            ContractService._refresh_contract(context, name)
            return
        time.sleep(2)
    raise AssertionError(
        f"Expected the ORCE contract-target-flow's acknowledgement callback to move "
        f"contract '{name}' ({did}) to ACTIVE within 90s of deployment, state is still "
        f"'{actual_state}'"
    )


@then('the archive entry for contract "{name}" contains an RFC-3161 TSA timestamp over the execution-evidence receipt')
def step_then_archive_has_tsa_timestamp(context, name):
    # The auto-dispatch subscriber can insert a second (un-acked) deployment
    # row concurrently with the explicit deploy+ack this scenario drives; the
    # archive view prefers acknowledged rows, but the ack itself follows the
    # async dispatch — poll briefly rather than racing it.
    import time as _time  # noqa: PLC0415

    entry, deployment, receipt_hash, tsa_token_b64 = None, {}, None, None
    deadline = _time.monotonic() + 60
    while _time.monotonic() < deadline:
        entry = _archive_entry_for(context, name)
        deployment = ((entry or {}).get("evidence") or {}).get("deployment") or {}
        receipt_hash = deployment.get("receipt_hash")
        tsa_token_b64 = deployment.get("tsa_token")
        if receipt_hash and tsa_token_b64:
            break
        _time.sleep(2)
    assert entry is not None, f"Expected an archive entry for contract '{name}'"
    assert receipt_hash, (
        f"Expected the archive entry's evidence.deployment.receipt_hash (canonical hash of the "
        f"execution-evidence receipt) to be populated, got: {deployment!r}"
    )
    assert tsa_token_b64, (
        f"Expected the archive entry's evidence.deployment.tsa_token (RFC-3161 TSA response over "
        f"the receipt's canonical hash) to be populated, got: {deployment!r}"
    )
    try:
        tsa_bytes = base64.b64decode(tsa_token_b64, validate=True)
    except Exception as exc:  # noqa: BLE001
        raise AssertionError(f"evidence.deployment.tsa_token is not valid base64: {exc}") from exc
    first_byte_desc = f"{tsa_bytes[0]:#x}" if tsa_bytes else "empty bytes"
    assert tsa_bytes and tsa_bytes[0] == 0x30, (
        "Expected evidence.deployment.tsa_token to decode to a DER-encoded ASN.1 SEQUENCE "
        f"(RFC 3161 TimeStampResp/Token starts with tag 0x30), got first byte: {first_byte_desc}"
    )


@when('the target reports a KPI value "{metric}" = "{value}" for contract "{name}"')
def step_when_target_reports_kpi(context, metric, value, name):
    did, _ = ContractService._contract_data(context, name)
    payload = {
        "did": did,
        "correlation_id": getattr(context, "deployment_correlation_id", None) or "bdd-unknown-correlation",
        "kpi": {"metric": metric, "value": value},
    }
    headers = _target_callback_headers()
    context.requests_response = post_json(context, contract_deployment_callback_url(context), payload, headers=headers)


# @step, not @when: a scenario that asserts something AFTER a breach needs the
# breach as part of its Given block, so this must bind in any block.
@step('the target reports a KPI value for the ODRL-bound field of contract "{name}" = "{value}"')
def step_when_target_reports_kpi_for_bound_field(context, name, value):
    # The metric IS the bound placeholder's @id, so EvaluateKPIViolation binds
    # it to the ODRL constraint by node IRI (DCS-FR-CWE-09).
    step_when_target_reports_kpi(context, _odrl_bound_field_iri(context, name), value, name)


@then('the contract detail for "{name}" shows a KPI violation flag for its ODRL-bound field')
def step_then_contract_detail_shows_kpi_violation_for_bound_field(context, name):
    step_then_contract_detail_shows_kpi_violation(context, name, _odrl_bound_field_iri(context, name))


@then('the semantic KPI observations for "{name}" record a violated observation for its ODRL-bound field')
def step_then_semantic_kpi_observations_for_bound_field(context, name):
    step_then_semantic_kpi_observations(context, name, _odrl_bound_field_iri(context, name))


@then('the contract detail for "{name}" shows a target-reported KPI "{metric}"')
def step_then_contract_detail_shows_target_kpi(context, name, metric):
    """DCS-FR-CWE-31 ("KPIs sent from the target system"): the metric below
    is measured and reported by the ORCE contract-target-flow itself (its
    activation latency), not by any harness callback — so its value is only
    known to be a positive number, and it may land moments after the ack."""
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    kpis = []
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
        assert retrieve.status_code == 200, retrieve.text
        kpis = _kpi_entries(retrieve.json())
        matching = [k for k in kpis if str(k.get("metric")) == metric]
        if matching:
            value = str(matching[-1].get("value"))
            assert value and float(value) > 0, (
                f"Expected the target-measured KPI '{metric}' to carry a positive number, got {value!r}"
            )
            return
        time.sleep(2)
    raise AssertionError(
        f"Expected the ORCE contract-target-flow to report KPI '{metric}' for contract "
        f"'{name}' ({did}) via the deployment callback within 60s, got kpis: {kpis!r}"
    )


@then('the contract detail for "{name}" shows KPI "{metric}" with value "{value}"')
def step_then_contract_detail_shows_kpi(context, name, metric, value):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
    assert retrieve.status_code == 200, retrieve.text
    kpis = _kpi_entries(retrieve.json())
    matching = [k for k in kpis if str(k.get("metric")) == metric]
    assert matching, (
        f"Expected contract '{name}' detail to include a KPI entry for metric '{metric}' "
        f"(FR-CWE-31/FR-CWE-09 dashboard), got kpis: {kpis!r}"
    )
    actual_value = str(matching[-1].get("value"))
    assert actual_value == value, (
        f"Expected KPI '{metric}' on contract '{name}' to have value '{value}', got '{actual_value}'"
    )


@then('the contract detail for "{name}" shows a KPI violation flag for "{metric}"')
def step_then_contract_detail_shows_kpi_violation(context, name, metric):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
    assert retrieve.status_code == 200, retrieve.text
    body = retrieve.json()
    kpis = _kpi_entries(body)
    matching = [k for k in kpis if str(k.get("metric")) == metric]
    flagged_on_entry = any(bool(k.get("violation")) for k in matching)
    flagged_top_level = metric in _kpi_violation_names(body)
    assert flagged_on_entry or flagged_top_level, (
        f"Expected a KPI violation flag/alert for metric '{metric}' on contract '{name}' after a "
        f"reported KPI value crossed its contractually declared SLA threshold (odrl:Set "
        f"rightOperand), got kpis: {kpis!r}, kpi_violations: {body.get('kpi_violations')!r}"
    )


# ---------------------------------------------------------------------------
# Given/When/Then — archive-entry trigger at SIGNED, not APPROVED
# ---------------------------------------------------------------------------


@then('the archive has no entry for contract "{name}"')
def step_then_archive_has_no_entry(context, name):
    entry = _archive_entry_for(context, name)
    assert entry is None, (
        f"Expected NO archive entry for contract '{name}' yet (archive-entry creation is gated to "
        f"the SIGNED transition, not APPROVED), but found one: {entry!r}"
    )


@then('the archive has an entry for contract "{name}"')
def step_then_archive_has_entry(context, name):
    entry = _archive_entry_for(context, name)
    assert entry is not None, (
        f"Expected an archive entry for contract '{name}' after it reached SIGNED, found none"
    )


# ---------------------------------------------------------------------------
# Given/When/Then — the shipped ORCE contract-target-flow directly
# ---------------------------------------------------------------------------


@given("the example ORCE contract-target-flow is reachable")
def step_given_orce_reachable(context):
    orce_url = os.getenv("BDD_ORCE_TARGET_URL", "").strip()
    assert orce_url, (
        "BDD_ORCE_TARGET_URL must be set to the deployed contract-target-flow's HTTP-in endpoint "
        "(deployment/helm/charts/orce/flows/contract-target-flow.json) to run this scenario."
    )
    context.orce_target_url = orce_url


@when('a deployment payload for contract "{name}" is posted directly to the ORCE contract-target-flow')
def step_when_post_to_orce_directly(context, name):
    """Posts the SAME envelope shape the backend dispatches
    (command/deploy.go): a JSON-LD document whose dcs:contentHash covers the
    whole envelope minus the hash field itself, canonicalized per RFC 8785
    (JCS) — matching Go's hashDeploymentPayload and the flow's
    sortKeysDeep + JSON.stringify."""
    did, updated_at = ContractService._contract_data(context, name)
    correlation_id = f"bdd-orce-{did}-{updated_at}".replace(":", "-").replace(" ", "-")
    envelope = {
        "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "odrl": "http://www.w3.org/ns/odrl/2/"},
        "@type": "dcs:ContractDeployment",
        "dcs:contractDid": did,
        "dcs:contractVersion": 1,
        "dcs:timestamp": "2026-01-01T00:00:00Z",
        "dcs:correlationId": correlation_id,
        "dcs:contractDocument": {
            "@type": "dcs:Contract",
            "dcs:contractDid": did,
        },
        "odrl:policy": {"@id": "urn:uuid:bdd-orce-policy-set", "@type": "odrl:Set"},
    }
    content_hash = "sha256:" + hashlib.sha256(jcs.canonicalize(envelope)).hexdigest()
    envelope["dcs:contentHash"] = content_hash
    context.orce_sent_correlation_id = correlation_id
    context.orce_sent_content_hash = content_hash

    context.requests_response = _requests.post(
        context.orce_target_url, json=envelope, timeout=context.http_timeout_seconds
    )


@then("the ORCE flow acknowledges with correlation_id, payload_hash, and activated_at matching the sent payload")
def step_then_orce_acknowledges(context):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the ORCE contract-target-flow to acknowledge the deployment payload, got "
        f"{resp.status_code}: {resp.text}"
    )
    ack = resp.json()
    assert ack.get("correlation_id") == context.orce_sent_correlation_id, (
        f"Expected the ORCE ack's correlation_id to echo {context.orce_sent_correlation_id!r}, got "
        f"{ack.get('correlation_id')!r}: {ack!r}"
    )
    assert ack.get("payload_hash") == context.orce_sent_content_hash, (
        f"Expected the ORCE ack's payload_hash to equal the content_hash the flow received and "
        f"verified ({context.orce_sent_content_hash!r}), got {ack.get('payload_hash')!r}: {ack!r}"
    )
    assert ack.get("activated_at"), f"Expected a non-empty 'activated_at' in the ORCE ack: {ack!r}"


@then('the semantic KPI observations for "{name}" record a violated "{metric}" observation')
def step_then_semantic_kpi_observations(context, name, metric):
    """GET /contract/kpis/{did} (DCS-FR-CWE-09/-31): the reported KPIs as a
    JSON-LD observation set — dcs:KPIObservation nodes anchored to the
    Semantic Hub's versioned context, consumable by external tooling."""
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = get_with_headers(context, f"{context.base_url}/contract/kpis/{did}", headers=manager_h)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert isinstance(body.get("@context"), str) and "/semantic/context/" in body["@context"], (
        f"Expected the observation set's @context to be the hub's versioned context URL, got: {body.get('@context')}"
    )
    assert body.get("@type") == "dcs:KPIObservationSet", f"Expected a dcs:KPIObservationSet, got: {body.get('@type')}"
    observations = body.get("dcs:observation") or []
    matching = [
        node for node in observations
        if node.get("@type") == "dcs:KPIObservation" and node.get("dcs:metricName") == metric
    ]
    assert matching, (
        f"Expected a dcs:KPIObservation for metric {metric!r}, got: {observations}"
    )
    assert any(node.get("dcs:violation") is True for node in matching), (
        f"Expected a violated {metric!r} observation, got: {matching}"
    )
    # The observation references the contract's dereferenceable resource IRI,
    # whose last path segment is the system key.
    assert all(
        str(node.get("dcs:aboutContract", {}).get("@id", "")).rstrip("/").rsplit("/", 1)[-1] == did
        for node in matching
    ), f"Expected every observation to reference contract {did}, got: {matching}"
