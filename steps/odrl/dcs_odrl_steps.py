"""BDD steps for ODRL soundness (features/18_odrl_soundness; SRS
DCS-FR-PACM-03).

The structure and enforcement scenarios build their fixtures against the
canonical Offer/Agreement-enclosed ODRL shape the backend emits and validates
(`extractContractODRLPolicies`,
backend/internal/base/validation/contractcontentaudit.go). Testing
enforcement against the enclosed shape is what catches the regression where
the emitted shape and the extraction drift apart — approve/apply would then
silently see zero policies and let everything through.

The operator Scenario Outline and the bare-shape rejection scenario use
the bare flat-array fixture instead: operator evaluation is independent of
the enclosing Set (and additionally covered by the Go unit tests in
backend/internal/base/validation/contractcontentaudit_test.go), and the
bare-Duty shape (no action, no enclosing policy node) must be REJECTED by structural validation.

The peer-action entry path is not separately re-tested: it dispatches
through the same command.Approver handler as the UI/API path (see the
feature file header).
"""

from behave import given, then, when

from steps.support.api_client import (
    contract_approve_url,
    contract_retrieve_by_id_url,
    contract_update_url,
    get_with_headers,
    post_json,
    put_json,
)
from steps.support.services import odrl_fixture_service as odrl
from steps.support.signing import wallet_sign
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService

# Reuse the already-built, already-working contract-state-advancement helpers
# from the contract-state-machine-refactor spec instead of re-implementing
# the submit/review/approve/sign chains a second time.
from steps.template_management.contract_state_machine_steps import (
    _advance_to_approved,
    _advance_to_reviewed,
    _apply_signature,
)


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _scalar(raw: str):
    raw = raw.strip()
    try:
        if "." in raw:
            return float(raw)
        return int(raw)
    except ValueError:
        return raw


def _parse_operand(raw: str):
    if "," in raw:
        return [_scalar(v) for v in raw.split(",")]
    return _scalar(raw)


def _update_contract_policies(context, name, field, policies, actual_value):
    did, updated_at = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    doc = odrl.build_contract_document(did, field, policies, actual_value)
    # A full-document PUT replaces contract_data wholesale, so carry the
    # signature field(s) seeded at genesis (keyed by the signer's party DID)
    # forward — dropping them leaves the contract unsignable.
    current = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    if current.status_code == 200:
        sig_fields = (current.json().get("contract_data") or {}).get("dcs:signatureFields")
        if sig_fields:
            doc["dcs:signatureFields"] = sig_fields
    resp = put_json(
        context,
        contract_update_url(context),
        {"did": did, "updated_at": updated_at, "contract_data": doc},
        headers=headers,
    )
    if resp.status_code == 200:
        ContractService._refresh_contract(context, name)
    return resp


def _stored_policies(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    contract_data = retrieve.json().get("contract_data") or {}
    return contract_data.get("dcs:policies", contract_data.get("policies"))


def _contract_state(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    return str(retrieve.json().get("state", "")).upper()


# ---------------------------------------------------------------------------
# Given
# ---------------------------------------------------------------------------


@given('a fresh draft contract "{name}"')
def step_given_fresh_draft_contract(context, name):
    ContractService._create_contract_in_draft(context, name)


# ---------------------------------------------------------------------------
# When — mutating the policies
# ---------------------------------------------------------------------------


@when(
    'the policies of contract "{name}" are updated to a real ODRL 2.2 policy '
    'set (rule "{rule_type}", field "{field}", operator "{operator}") '
    'requiring "{right_operand}" while the actual value is "{actual_value}"'
)
def step_when_policies_updated_to_odrl_set(context, name, rule_type, field, operator, right_operand, actual_value):
    did, _ = ContractService._contract_data(context, name)
    right = _parse_operand(right_operand)
    actual = _parse_operand(actual_value)
    policies = odrl.odrl_set_policies(did, field, operator, right, rule_type=f"odrl:{rule_type}")
    context.requests_response = _update_contract_policies(context, name, field, policies, actual)


@when(
    'the policies of contract "{name}" are updated to the bare-Duty '
    'form (field "{field}", operator "{operator}") requiring "{right_operand}" '
    'while the actual value is "{actual_value}"'
)
def step_when_policies_updated_to_bare_duty_form(context, name, field, operator, right_operand, actual_value):
    right = _parse_operand(right_operand)
    actual = _parse_operand(actual_value)
    policies = odrl.bare_duty_policies(field, operator, right)
    context.requests_response = _update_contract_policies(context, name, field, policies, actual)


@given(
    'contract "{name}" is a fresh draft whose ODRL policy constrains '
    'field "{field}" using operator "{operator}" against "{right_operand}" '
    'while the actual value is "{actual_value}"'
)
def step_given_operator_fixture(context, name, field, operator, right_operand, actual_value):
    # Deliberately the canonical enclosing-policy shape (not the bare flat form):
    # a fixture identical in shape to the rejected bare form cannot also
    # be the accepted fixture the operator scenarios approve against; the
    # two would be mutually unsatisfiable otherwise. Operator-evaluation
    # correctness is exercised identically regardless of the enclosing
    # shape, and is additionally covered by the Go unit tests in
    # backend/internal/base/validation/contractcontentaudit_test.go.
    ContractService._create_contract_in_draft(context, name)
    did, _ = ContractService._contract_data(context, name)
    right = _parse_operand(right_operand)
    actual = _parse_operand(actual_value)
    policies = odrl.odrl_set_policies(did, field, operator, right)
    resp = _update_contract_policies(context, name, field, policies, actual)
    assert resp.status_code == 200, (
        f"could not seed the operator fixture for '{name}' "
        f"(operator {operator!r}): {resp.status_code} {resp.text}"
    )


# ---------------------------------------------------------------------------
# When — exercising enforcement
# ---------------------------------------------------------------------------


@when('approval is attempted for contract "{name}"')
def step_when_approval_attempted(context, name):
    _advance_to_reviewed(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Contract Approver"])
    context.requests_response = post_json(
        context, contract_approve_url(context), {"did": did, "updated_at": updated_at}, headers=headers
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when(
    'the contract "{name}" is submitted, reviewed, approved, and signed via '
    "the standard workflow"
)
def step_when_full_workflow_to_signed(context, name):
    _advance_to_approved(context, name)
    _apply_signature(context, name)


@when('a direct signing API call is attempted against contract "{name}" before it is approved')
def step_when_direct_sign_before_approval(context, name):
    _advance_to_reviewed(context, name)
    did, _updated_at = ContractService._contract_data(context, name)
    # Signing an un-APPROVED contract must be refused; the transition gate in
    # /signature/prepare rejects it before any signature is produced.
    context.requests_response = wallet_sign(
        context, did, signer_did="did:example:bdd-odrl-signer", signatory="bdd-odrl-signer"
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


# ---------------------------------------------------------------------------
# Then — policy update outcome
# ---------------------------------------------------------------------------


@then('the policy update for contract "{name}" is accepted')
def step_then_policy_update_accepted(context, name):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"expected the ODRL policy update for '{name}' to be accepted, got "
        f"{resp.status_code}: {resp.text}"
    )


@then(
    'the policy update for contract "{name}" is rejected because the '
    "bare-Duty form lacks an action and enclosing policy"
)
def step_then_policy_update_rejected_bare_duty(context, name):
    resp = context.requests_response
    assert resp.status_code != 200, (
        f"expected the bare-Duty policy shape (no odrl:action, no "
        f"enclosing policy node, no parties/target) to be explicitly rejected "
        f"by structural validation for '{name}', but the update succeeded: "
        f"{resp.status_code} {resp.text}"
    )


# ---------------------------------------------------------------------------
# Then — structural assertions
# ---------------------------------------------------------------------------


@then(
    'the stored policies of contract "{name}" form a single enclosing '
    '{policy_type} whose @id is anchored to the contract DID and which '
    "declares an odrl:profile"
)
def step_then_policies_form_enclosing_set(context, name, policy_type):
    """policy_type reflects the ODRL policy lifecycle: an unsigned contract
    instance carries an odrl:Offer (parties still open); the first signature
    seals it into the odrl:Agreement the signatures bind."""
    did, _ = ContractService._contract_data(context, name)
    policies = _stored_policies(context, name)
    assert isinstance(policies, dict), (
        f"expected dcs:policies to be ONE enclosing policy object, got a "
        f"{type(policies).__name__}: {policies!r}"
    )
    assert policies.get("@type") == policy_type, (
        f"expected the enclosing policy node's @type to be {policy_type!r}, "
        f"got {policies.get('@type')!r}"
    )
    policy_id = policies.get("@id") or ""
    assert did in policy_id, (
        f"expected the {policy_type}'s @id (its odrl:uid) to be anchored to the "
        f"contract DID {did!r}, got {policy_id!r}"
    )
    assert "uid" not in policies, (
        f"a separate uid key duplicates the policy identity (@id): {policies.get('uid')!r}"
    )
    profile = policies.get("odrl:profile")
    assert profile, f"expected odrl:profile to be declared on the enclosing {policy_type}, got: {profile!r}"


@then('every stored policy rule of contract "{name}" declares exactly one odrl:action')
def step_then_every_rule_has_one_action(context, name):
    policies = _stored_policies(context, name)
    rules = odrl.extract_policy_rules(policies)
    assert rules, f"expected at least one policy rule for '{name}', got: {policies!r}"
    for rule in rules:
        action = rule.get("odrl:action")
        assert action, f"policy rule {rule.get('@id')} is missing odrl:action: {rule!r}"
        assert not isinstance(action, list) or len(action) == 1, (
            f"policy rule {rule.get('@id')} must declare exactly one odrl:action, got: {action!r}"
        )


@then(
    'every stored policy rule of contract "{name}" declares an odrl:assigner, '
    "odrl:assignee, and odrl:target"
)
def step_then_every_rule_has_parties_and_target(context, name):
    policies = _stored_policies(context, name)
    rules = odrl.extract_policy_rules(policies)
    assert rules, f"expected at least one policy rule for '{name}', got: {policies!r}"
    for rule in rules:
        for prop in ("odrl:assigner", "odrl:assignee", "odrl:target"):
            assert rule.get(prop), f"policy rule {rule.get('@id')} is missing {prop}: {rule!r}"


# ---------------------------------------------------------------------------
# Then — enforcement outcomes
# ---------------------------------------------------------------------------


@then("the approval is rejected because an ODRL constraint is violated")
def step_then_approval_rejected_constraint_violated(context):
    resp = context.requests_response
    assert resp.status_code != 200, (
        f"expected approval of a contract with a violated ODRL constraint to "
        f"be rejected, but it succeeded: {resp.status_code} {resp.text}"
    )


@then('the sign attempt for contract "{name}" is rejected and the contract remains unsigned')
def step_then_sign_attempt_rejected(context, name):
    resp = context.requests_response
    assert resp.status_code != 200, (
        f"expected the direct signing API call for '{name}' to be rejected, "
        f"but it succeeded: {resp.status_code} {resp.text}"
    )
    state = _contract_state(context, name)
    assert state != "SIGNED", f"contract '{name}' must not reach SIGNED, but state is '{state}'"


@then('the contract "{name}" reaches SIGNED state')
def step_then_contract_reaches_signed(context, name):
    # ACTIVE is reachable exclusively from SIGNED via the automatic
    # deployment chain's real target acknowledgement
    # (contractstate.Transitions), so observing it proves SIGNED was reached.
    state = _contract_state(context, name)
    assert state in ("SIGNED", "ACTIVE"), f"expected contract '{name}' to reach SIGNED, got '{state}'"


# ---------------------------------------------------------------------------
# Then — operator matrix outcome (Scenario Outline)
# ---------------------------------------------------------------------------


@then('the approval outcome for contract "{name}" is "{expected}"')
def step_then_approval_outcome(context, name, expected):
    resp = context.requests_response
    normalized = expected.strip().lower()
    if normalized == "satisfied":
        assert resp.status_code == 200, (
            f"expected the constraint-satisfied approval for '{name}' to "
            f"succeed, got {resp.status_code}: {resp.text}"
        )
    elif normalized == "violated":
        assert resp.status_code != 200, (
            f"expected the constraint-violating approval for '{name}' to be "
            f"rejected, but it succeeded: {resp.text}"
        )
    else:
        raise NotImplementedError(f"unknown expected outcome {expected!r} — use 'satisfied' or 'violated'")
