"""BDD steps for the local trust policy endpoint (PDP) gate (ADR-19,
docs/adr-19-federation-agreement-credential.md, requirement slug
`fed-agreement`, AC7-AC10).

Per the ADR: "The trust gate POSTs {peerDID, agreementCredential, direction,
contractDID, targetState} to DCS_TRUST_PDP_URL; 2xx allows, anything else —
including an unconfigured URL or an unreachable endpoint — denies and raises
an incident in the audit trail (fail-closed)." This is consulted on BOTH the
outbound ship path (dcstodcs.DCSToDCSSynchronizer.shipContractPDF) and the
inbound receive path (dcstodcs service PostPdf) — neither calls it yet
(ADR-19's implementation-state table marks the PDP gate itself "pending"),
so every scenario here is expected to fail red.

Unified terminal semantics (architect decision): every PDP failure mode —
a non-2xx deny, an unset DCS_TRUST_PDP_URL, or an unreachable endpoint — is
EQUALLY terminal: the interaction is rejected, exactly one incident is
recorded, and NO sync_fails retry entry is ever created for a PDP-caused
rejection. There is no "transient, worth retrying" PDP failure mode — that
distinction only applies to AC6's DIFFERENT agreement-credential gate (layer
3a), which does still get a sync_fails entry (see
dcs_peer_trust_steps.py's step_then_a_refuses_to_ship_cross_instance).

Controllable trust-PDP: the orce Node-RED flow (NOT a local BDD-process HTTP
stub)
------------------------------------------------------------------------
An earlier version of this module ran its own `ThreadingHTTPServer` inside
the BDD test process and expected the DCS-under-test's `DCS_TRUST_PDP_URL`
to point directly at it. That does not work against the kind/Helm harness:
the DCS pod runs in a different network namespace than the BDD process, so a
plain socket bound in this process is not reachable from the backend (the
same reachability problem steps/contract_deployment/
dcs_contract_deployment_steps.py already documents for its own ORCE target).

Both DCS instances are instead wired (harness-side, not by this module) with
`DCS_TRUST_PDP_URL=http://dcs-orce:1880/trust-pdp` — the already-deployed
orce Node-RED chart, reachable from the backend pods AND (via its own
port-forward) from this BDD process. The flow itself exposes a small control
surface this module drives:
  - `POST {control}/trust-pdp/mode {"mode": "allow"|"deny"|"silent"}` sets
    how `/trust-pdp` itself responds: 200 OK, 4xx/5xx, or (for "silent") no
    response at all, so the backend's own HTTP client times out against it —
    a genuine client-observed unreachable endpoint, not a fabricated one.
  - `GET {control}/trust-pdp/last?contractDID=<iri>` returns the request the
    trust gate made for ONE interaction, which is what AC7's "was it
    consulted" and AC9's field-shape assertions read. Without the parameter
    the same route reports the run's global last request — that form proves
    nothing about the scenario asserting it, since any earlier consult
    already satisfies it, and no assertion here uses it.

`BDD_TRUST_PDP_CONTROL_URL` (default `http://localhost:18880`, the harness's
own orce port-forward) is the control-plane address used by THIS BDD
process; it does not need to equal `DCS_TRUST_PDP_URL` itself (that one is
only ever resolved backend-side, inside the cluster).

Every Given step here resets the flow's mode back to "allow" after the
scenario via `context.add_cleanup` (behave-guaranteed to run whether the
scenario passes or fails) — otherwise a denying/silent mode set by one
scenario would leak into and break every later federation scenario sharing
the same long-lived orce deployment.
"""

import os
import time

import requests as _requests
from behave import given, then

from steps.support.api_client import pac_audit_url, post_json
from steps.support.services.auth_service import AuthService


def _control_url(context) -> str:
    return os.getenv("BDD_TRUST_PDP_CONTROL_URL", "http://localhost:18880").rstrip("/")


def _set_pdp_mode(context, mode: str):
    control = _control_url(context)
    resp = _requests.post(
        f"{control}/trust-pdp/mode", json={"mode": mode}, timeout=context.http_timeout_seconds
    )
    assert resp.status_code == 200, (
        f"Failed to set the trust-PDP orce flow's mode to '{mode}' via {control}/trust-pdp/mode "
        f"(BDD_TRUST_PDP_CONTROL_URL) — is the orce port-forward up? Got {resp.status_code}: {resp.text}"
    )


def _reset_pdp_mode_after_scenario(context):
    """Registers a behave cleanup (runs after the scenario, pass or fail) that
    puts the orce trust-PDP flow back into 'allow' mode — otherwise a deny/
    silent mode set here would leak into and break every later federation
    scenario sharing the same long-lived orce deployment."""
    context.add_cleanup(_set_pdp_mode, context, "allow")


def _consult_for_contract(context, contract_did: str, timeout_seconds: int = 45) -> dict:
    """The consult the trust gate made FOR ONE INTERACTION, read from the orce
    flow's per-contract record (GET /trust-pdp/last?contractDID=...).

    The global "last request" this used to read proves nothing about the
    scenario asserting it: any earlier consult in the run — including the
    outbound one an unrelated scenario triggered — already makes it non-empty,
    so the assertion passed even when the interaction under test never reached
    the PDP at all. Polls, because the outbound consult happens on the
    asynchronous ship path."""
    control = _control_url(context)
    url = f"{control}/trust-pdp/last"
    deadline = time.monotonic() + timeout_seconds
    last_resp = None
    while True:
        last_resp = _requests.get(
            url, params={"contractDID": contract_did}, timeout=context.http_timeout_seconds
        )
        if last_resp.status_code == 200:
            body = last_resp.json()
            if isinstance(body, dict) and body:
                return body
        if time.monotonic() >= deadline:
            break
        time.sleep(2)
    raise AssertionError(
        f"Expected the trust gate to have consulted the policy endpoint for contract "
        f"{contract_did} — no such request recorded at {url} within {timeout_seconds}s "
        f"(last response: {last_resp.status_code} {last_resp.text}). Either the interaction never "
        "reached layer 3b (an earlier trust layer refused it first), or the orce port-forward is "
        "down."
    )


def _interaction_contract_did(context) -> str:
    """The contract the scenario's When step just shipped — set there rather
    than parsed out of the step text, so both the inbound and the outbound ship
    step can bind it."""
    contract_did = getattr(context, "pdp_interaction_contract_did", None)
    assert contract_did, (
        "No interaction contract recorded: the When step that ships the PDF must set "
        "context.pdp_interaction_contract_did before a PDP consult can be asserted for it."
    )
    return contract_did


# ---------------------------------------------------------------------------
# Given: PDP mode (orce flow control surface)
# ---------------------------------------------------------------------------


@given("the local policy endpoint (PDP) is running and allows every request")
def step_given_pdp_allows(context):
    _reset_pdp_mode_after_scenario(context)
    _set_pdp_mode(context, "allow")


@given("the local policy endpoint (PDP) is running and denies every request")
def step_given_pdp_denies(context):
    _reset_pdp_mode_after_scenario(context)
    _set_pdp_mode(context, "deny")


@given("the local policy endpoint (PDP) is not reachable")
def step_given_pdp_unreachable(context):
    """'silent' mode: the orce flow accepts the connection but never responds,
    so the backend's own HTTP client to DCS_TRUST_PDP_URL times out — a
    genuine, client-observed unreachable endpoint (fail-closed), not a
    fabricated response.

    The OTHER AC10 sub-case — DCS_TRUST_PDP_URL entirely UNSET — cannot be
    exercised against the same always-running DCS instance this flow is
    wired into (the env var is fixed at backend process startup); that
    sub-case needs a dedicated deployment variant with the var deliberately
    absent, which does not exist in the current runner (no analogue to
    kind_up_audit's single-purpose stack for this). Flagged as an open point
    rather than silently only testing the reachable sub-case under this AC's
    name.
    """
    _reset_pdp_mode_after_scenario(context)
    _set_pdp_mode(context, "silent")


# ---------------------------------------------------------------------------
# Then: PDP body / consultation assertions (AC7, AC9)
# ---------------------------------------------------------------------------


@then("the policy endpoint recorded a request naming the peer, the agreement credential, the direction, the contract, and the target state")
def step_then_pdp_body_shape(context):
    """AC9. The recorded request is the one made for THIS scenario's contract,
    and the fields are checked against what the scenario actually shipped —
    the peer that shipped it and that contract — so the body shape is evidence
    about this interaction rather than about whatever consult happened last."""
    contract_did = _interaction_contract_did(context)
    body = _consult_for_contract(context, contract_did)
    expected_keys = {"peerDID", "agreementCredential", "direction", "contractDID", "targetState"}
    actual_keys = set(body.keys())
    assert expected_keys <= actual_keys, (
        f"Expected the PDP request body to carry at least {sorted(expected_keys)}, got keys: "
        f"{sorted(actual_keys)} (body: {body})"
    )
    assert body.get("contractDID") == contract_did, (
        f"Expected the recorded request to name this scenario's contract ({contract_did}), got: "
        f"{body.get('contractDID')!r}"
    )
    assert body.get("peerDID") == context.peer_from_did, (
        f"Expected the recorded request to name the peer that shipped the PDF "
        f"({context.peer_from_did}), got: {body.get('peerDID')!r}"
    )
    assert body.get("agreementCredential"), (
        "Expected the recorded request to carry the peer's agreement credential — an empty one "
        f"means layer 3a never produced it: {body}"
    )
    assert body.get("direction") in ("inbound", "outbound"), (
        f"Expected direction to be 'inbound' or 'outbound', got: {body.get('direction')!r}"
    )


@then("the policy endpoint was consulted for this interaction")
def step_then_pdp_was_consulted(context):
    """AC7. Scoped to the contract the When step shipped: the orce flow records
    every consult under its contractDID, so this asserts that THIS interaction
    reached layer 3b — not merely that some consult happened at some point in
    the run."""
    _consult_for_contract(context, _interaction_contract_did(context))


# ---------------------------------------------------------------------------
# Then: fail-closed / incident-report assertions (AC8, AC10)
# ---------------------------------------------------------------------------


def _pac_audit_entries(context, scope="PROCESS_AUDIT_AND_COMPLIANCE", api_base=None):
    # api_base names the instance to audit, for BOTH the token and the query:
    # AuthService.get_headers_for_roles authenticates against api_base when
    # given, else os.getenv("BDD_DCS_BASE_URL") — NOT context.base_url. Each
    # instance's Hydra issues under its own issuer URL, so a token minted on one
    # instance and presented to the other is rejected with 401; deriving the URL
    # from api_base too keeps the two sides paired without also requiring the
    # caller to be inside an _as_instance(...) block.
    if api_base:
        headers = AuthService.get_headers_for_roles(["Auditor"], api_base=api_base)
        url = f"{api_base.rstrip('/')}/pac/audit"
    else:
        headers = AuthService.get_headers_for_roles(["Auditor"])
        url = pac_audit_url(context)
    resp = post_json(context, url, {"scope": scope, "justification": "BDD PDP-gate audit re-trigger"}, headers=headers)
    assert resp.status_code == 200, f"PAC-scope audit failed: {resp.status_code} {resp.text}"
    body = resp.json()
    return [
        entry
        for scope_result in body
        for entry in (scope_result.get("audit_trail") or [])
        if isinstance(entry, dict)
    ]


def _count_trust_gate_incidents(context, peer_did=None, contract_did=None, api_base=None):
    """Expected event_type contract for the implementer, mirroring the
    already-established PAC_COMPLIANCE_RISK / PAC_INCIDENT_REPORT anchoring
    pattern (steps/audit_compliance/dcs_process_audit_steps.py,
    querymonitor.go): PAC_TRUST_GATE_DENIAL, anchored per-contract when a
    contractDID is known, or globally (no resource DID) when the denial
    happened before any contract context existed. This event-type name is an
    assumption for the implementer to confirm/correct — ADR-19 only commits
    to "raises an incident in the audit trail", not a concrete event type.
    """
    entries = _pac_audit_entries(context, api_base=api_base)
    matching = [e for e in entries if e.get("event_type") == "PAC_TRUST_GATE_DENIAL"]
    if contract_did:
        matching = [e for e in matching if e.get("did") == contract_did]
    if peer_did:
        matching = [e for e in matching if (e.get("event_data") or {}).get("peer_did") == peer_did]
    return matching


@then('the interaction is denied and exactly one incident is recorded in the audit trail for contract "{name}"')
def step_then_denied_exactly_one_incident(context, name):
    """Scoped to THIS scenario's own contract DID — an unscoped, global
    "exactly one PAC_TRUST_GATE_DENIAL" count is not reliable in a shared
    long-lived environment: orphaned sync_fails retries from earlier runs
    (or other scenarios' denials) can already have raised unrelated
    incidents, which a global count would wrongly attribute to this
    scenario's denial."""
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    resp = context.requests_response
    assert resp.status_code != 200, (
        f"Expected the PDP-denied interaction to be rejected, got 200: {resp.text}"
    )
    # WHICH gate denied it, not just that something did: the peer this scenario
    # ships as passes the agreement-credential check (layer 3a), so a rejection
    # naming that check would mean the scenario is proving the wrong thing —
    # which is exactly what happened while the only available inbound peer
    # published no credential at all.
    body_text = resp.text.lower()
    assert "policy endpoint" in body_text, (
        "Expected the rejection to name the policy endpoint (ADR-19 layer 3b, the gate this AC is "
        f"about), got {resp.status_code}: {resp.text}"
    )
    assert "agreement credential" not in body_text, (
        "Expected the denial to come from the policy endpoint, but the rejection names the "
        f"agreement-credential check (layer 3a) instead: {resp.text}"
    )
    deadline = time.monotonic() + 60
    matching = []
    while time.monotonic() < deadline:
        matching = _count_trust_gate_incidents(context, contract_did=did)
        if matching:
            break
        time.sleep(2)
    for entry in matching:
        reason = str((entry.get("event_data") or {}).get("reason", "")).lower()
        assert "policy endpoint" in reason, (
            "Expected the recorded incident to name the policy endpoint as the denying gate, got "
            f"reason: {reason!r}"
        )
    assert len(matching) == 1, (
        f"Expected exactly one PAC_TRUST_GATE_DENIAL incident for contract '{name}' (did={did})'s "
        f"denial (Deny is TERMINAL per the architect's decision — no retry, no duplicate incidents), "
        f"got {len(matching)}: {matching}"
    )


@then('the outbound ship attempt is denied and exactly one incident is recorded in the audit trail for contract "{name}"')
def step_then_outbound_denied_exactly_one_incident(context, name):
    """Like step_then_denied_exactly_one_incident, but for the OUTBOUND ship
    path (contract offer -> async shipContractPDF): there is no synchronous
    HTTP response to assert a non-200 on (the offer itself returns 200; the
    ship attempt and its denial happen asynchronously), so this only checks
    the audit-trail side effect — scoped to this scenario's own contract DID
    for the same reason as step_then_denied_exactly_one_incident above."""
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    deadline = time.monotonic() + 60
    matching = []
    while time.monotonic() < deadline:
        matching = _count_trust_gate_incidents(context, contract_did=did)
        if matching:
            break
        time.sleep(2)
    assert len(matching) == 1, (
        f"Expected exactly one PAC_TRUST_GATE_DENIAL incident for contract '{name}' (did={did})'s "
        f"denied outbound ship attempt (Deny is TERMINAL per the architect's decision), got "
        f"{len(matching)}: {matching}"
    )


@then('no sync_fails retry entry exists for contract "{name}"')
def step_then_no_sync_fails_entry(context, name):
    """Unified terminal semantics (architect decision): every PDP-caused
    rejection — a non-2xx deny (AC8) or an unreachable endpoint (AC10) — is
    equally terminal. Neither creates a sync_fails retry entry; both raise
    exactly one incident. (Contrast AC6, where a rejection caused by the
    DIFFERENT agreement-credential gate — not the PDP — DOES get a
    sync_fails entry; see step_then_a_refuses_to_ship_cross_instance in
    dcs_peer_trust_steps.py.)"""
    from steps.support.services.contract_service import ContractService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute("SELECT 1 FROM sync_fails WHERE did = %s", (did,))
    row = cursor.fetchone()
    cursor.close()
    assert row is None, (
        f"Expected NO sync_fails retry entry for contract '{name}' (did={did}) after a PDP-caused "
        "rejection (deny or unreachable) — the architect's decision makes every PDP failure mode "
        f"terminal, not a transient failure worth retrying — but found a row"
    )
