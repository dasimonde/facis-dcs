"""BDD steps for contract renewal (DCS-FR-CWE-11, DCS-FR-CWE-22, DCS-FR-CSA-15,
UC-06-02): POST /contract/renew (backend/design/contract_workflow_engine.go)
creates a brand-new, independently versioned contract instance that carries a
dcs:renewsContract JSON-LD back-reference to the original — the original
contract is never mutated. See backend/internal/contractworkflowengine/
command/renew.go for the handler.
"""

from datetime import datetime, timedelta, timezone

from behave import given, then, when

from steps.support.api_client import (
    contract_renew_url,
    contract_retrieve_by_id_url,
    contract_update_url,
    get_with_headers,
    post_json,
    put_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.template_management.contract_state_machine_steps import (
    _advance_to_approved,
    _apply_signature,
)


def _renewal_response(context, name):
    ContractService._ensure_store(context, "contract_renewal_response", {})
    assert name in context.contract_renewal_response, (
        f"No renewal response recorded for contract '{name}' — was "
        f"'the contract manager renews contract \"{name}\" for a new term' run first?"
    )
    return context.contract_renewal_response[name]


def _fetch_renewal_contract(context, name):
    renewal = _renewal_response(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = get_with_headers(
        context, contract_retrieve_by_id_url(context, renewal["did"]), headers=manager_h
    )
    assert resp.status_code == 200, (
        f"Could not retrieve renewal contract for '{name}' ({renewal['did']}): "
        f"{resp.status_code} {resp.text}"
    )
    return resp.json()


# ---------------------------------------------------------------------------
# Given
# ---------------------------------------------------------------------------


@given('contract "{name}" with a contract term has reached contract state "SIGNED"')
def step_given_contract_with_term_signed(context, name):
    """Like the plain "has reached contract state" Given, but sets the
    contract's term (start_date/exp_date) via PUT /contract/update while the
    contract is still in DRAFT — the only state from which EventUpdate is
    legal (contractstate/transition.go) — before advancing to SIGNED. The
    renewal scenarios need the original to actually carry a term, otherwise
    the DCS-FR-CWE-22 carryover assertion would have nothing to carry over.
    Both dates must be at least one day in the future (command/update.go).
    """
    ContractService._create_contract_in_draft(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    # /contract/update is scoped to Contract Creator (design Security block);
    # reuse the creator identity that _create_contract_in_draft seeded.
    creator_h = context.contract_seed_headers[name]
    start = datetime.now(timezone.utc) + timedelta(days=2)
    resp = put_json(
        context,
        contract_update_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "start_date": start.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "exp_date": (start + timedelta(days=365)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        },
        headers=creator_h,
    )
    assert resp.status_code == 200, (
        f"Setting the contract term for '{name}' while in DRAFT failed: "
        f"{resp.status_code} {resp.text}"
    )
    ContractService._refresh_contract(context, name)
    _advance_to_approved(context, name)
    _apply_signature(context, name)
    ContractService._refresh_contract(context, name)


# ---------------------------------------------------------------------------
# When
# ---------------------------------------------------------------------------


@when('the contract manager renews contract "{name}" for a new term')
def step_when_renew_contract(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(
        context,
        contract_renew_url(context),
        {"did": did, "updated_at": updated_at},
        headers=manager_h,
    )
    if context.requests_response.status_code == 200:
        ContractService._ensure_store(context, "contract_renewal_response", {})
        context.contract_renewal_response[name] = context.requests_response.json()


# ---------------------------------------------------------------------------
# Then
# ---------------------------------------------------------------------------


@then('the renewal of "{name}" is a new contract in state "{state}"')
def step_then_renewal_in_state(context, name, state):
    renewal = _renewal_response(context, name)
    did, _ = ContractService._contract_data(context, name)
    assert renewal["did"] != did, (
        f"Expected the renewal of '{name}' to be a new contract instance with its "
        f"own DID, got the same DID as the original: {renewal['did']}"
    )
    body = _fetch_renewal_contract(context, name)
    actual_state = str(body.get("state", "")).upper()
    assert actual_state == state.strip().upper(), (
        f"Expected the renewal of '{name}' to be in state '{state}', got '{actual_state}'"
    )


@then('the renewal of "{name}" has its own term dates')
def step_then_renewal_has_term_dates(context, name):
    body = _fetch_renewal_contract(context, name)
    assert body.get("start_date") or body.get("exp_date"), (
        f"Expected the renewal of '{name}' to carry its own start/expiry term "
        f"(carried over from the original per DCS-FR-CWE-22 automatic metadata "
        f"carryover), got: {body}"
    )


@then('the renewal of "{name}" references the original contract\'s DID and version')
def step_then_renewal_references_original(context, name):
    original_did, _ = ContractService._contract_data(context, name)
    renewal = _renewal_response(context, name)
    assert renewal["renews_did"] == original_did, (
        f"Expected renewal response renews_did '{renewal['renews_did']}' to match "
        f"the original contract '{name}' DID '{original_did}'"
    )
    assert isinstance(renewal.get("renews_contract_version"), int), (
        f"Expected renewal response to carry an integer renews_contract_version, "
        f"got: {renewal}"
    )

    body = _fetch_renewal_contract(context, name)
    contract_data = body.get("contract_data") or {}
    reference = contract_data.get("dcs:renewsContract")
    assert reference is not None, (
        f"Expected the renewal contract's JSON-LD data to carry a "
        f"'dcs:renewsContract' reference back to the original, got contract_data "
        f"keys: {list(contract_data.keys())}"
    )
    # The back-reference is the original's dereferenceable resource IRI,
    # whose last path segment is the system key.
    reference_key = str(reference.get("@id", "")).rstrip("/").rsplit("/", 1)[-1]
    assert reference_key == original_did, (
        f"Expected dcs:renewsContract/@id to reference the original contract "
        f"'{original_did}', got: {reference}"
    )
    assert reference.get("dcs:version") == renewal["renews_contract_version"], (
        f"Expected dcs:renewsContract/dcs:version to match the renewal response's "
        f"renews_contract_version ({renewal['renews_contract_version']}), got: {reference}"
    )

