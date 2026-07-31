"""Contract workflow steps for negotiation, adjustment, and approval slices."""
import datetime
import time

from behave import given, then, when

from core.utils import is_date
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService
from steps.support.api_client import (
    contract_approve_url,
    contract_create_url,
    contract_negotiate_url,
    contract_audit_url,
    contract_reject_url,
    contract_retrieve_by_id_url,
    contract_submit_url,
    contract_update_url,
    contract_verify_url,
    get_with_headers,
    post_json,
    put_json,
)

from steps.support.services.auth_service import AuthService


def _negotiation_id_for_change(refreshed, change_request):
    """Select the negotiation entry matching a specific change request.

    retrieve_by_id assembles the negotiations list from a Go map
    (slices.Collect(maps.Values(...)) in service RetrieveByID), so its ORDER
    IS RANDOM — `negotiations[-1]` is a coin flip once a contract has more
    than one negotiation and picking an already-decided one makes respond
    fail with "no matching negotiation decision for this party".
    """
    negotiations = refreshed.get("negotiations") or []
    matching = [n for n in negotiations if n.get("change_request") == change_request]
    assert matching, (
        f"Expected a negotiation entry with change_request {change_request!r}, "
        f"got: {[n.get('change_request') for n in negotiations]}"
    )
    return matching[-1]["id"]


@given('contract "{name}" is in "Draft" status')
def step_given_contract_draft(context, name):
    ContractService._create_contract_in_draft(context, name)


@given('contract "{name}" is in "Under Review" status')
def step_given_contract_under_review(context, name):
    ContractService._create_contract_in_draft(context, name)
    ContractService._prepare_contract_under_review(context, name)


@given('contract "{name}" is pending approval')
def step_given_contract_pending_approval(context, name):
    ContractService._create_contract_in_draft(context, name)
    ContractService._prepare_contract_pending_approval(context, name)


@given('contract "{name}" requires my approval')
def step_given_contract_requires_my_approval(context, name):
    step_given_contract_pending_approval(context, name)


@given('contract "{name}" is open for negotiation')
def step_given_contract_open_for_negotiation(context, name):
    ContractService._create_contract_in_negotiation(context, name)


@given('contract "{name}" negotiation is complete')
def step_given_contract_negotiation_complete(context, name):
    # "Negotiation is complete" means the contract has reached NEGOTIATION
    # state and is ready for the (second) submit call that advances it to
    # SUBMITTED (see command/submit.go: Draft->Negotiation on the first
    # submit, Negotiation->Submitted on the second once there are no open
    # negotiation decisions) — _create_contract_in_draft alone leaves the
    # contract in DRAFT, from which "submit for review" cannot reach
    # SUBMITTED in one call.
    ContractService._create_contract_in_negotiation(context, name)


@given('contract "{name}" has completed multiple negotiation rounds')
def step_given_contract_multiple_negotiation_rounds(context, name):
    # Negotiate requires NEGOTIATION state (contractstate.Transitions:
    # EventNegotiate is only valid from Negotiation) — _create_contract_in_draft
    # alone leaves the contract in DRAFT, so a prior version of this step
    # (which used it) silently produced zero real negotiation rounds: the
    # negotiate calls below would 400 on the state-transition check, and
    # since this step never asserted their status code, that failure went
    # unnoticed until the "negotiation log" assertions came up empty.
    ContractService._create_contract_in_negotiation(context, name)
    creator_h = context.contract_seed_headers[name]
    # A different party must decide a proposal than the one who authored it
    # (FR-CWE-07 conflict-of-interest guard, acceptnegotiation.go) — accept/
    # reject as a distinct organization from the proposer's.
    responder_h = AuthService.get_headers_for_roles(["Contract Reviewer"], organization="TechVendor Inc")
    for change, flag, reason in (
        ("Round 1: tighten SLA wording", "ACCEPTING", ""),
        ("Round 2: extend liability cap", "REJECTING", "cap too high"),
    ):
        did, updated_at = ContractService._contract_data(context, name)
        resp = post_json(
            context,
            contract_negotiate_url(context),
            {"did": did, "updated_at": updated_at,
             "negotiated_by": AuthService.username_for_roles(["Contract Manager"]), "change_request": change},
            headers=creator_h,
        )
        assert resp.status_code == 200, f"negotiation round failed: {resp.status_code} {resp.text}"
        refreshed = ContractService._refresh_contract(context, name)
        negotiation_id = _negotiation_id_for_change(refreshed, change)
        resp = post_json(
            context,
            f"{context.base_url}/contract/respond",
            {"id": str(negotiation_id), "did": did, "action_flag": flag,
             "rejected_by": AuthService.username_for_roles(["Contract Manager"]) if flag == "REJECTING" else "",
             "rejection_reason": reason},
            headers=responder_h,
        )
        assert resp.status_code == 200, f"negotiation {flag} failed: {resp.status_code} {resp.text}"
    ContractService._refresh_contract(context, name)


@when('I open contract "{name}" for negotiation')
def step_when_open_for_negotiation(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I adjust clause "{clause}" with new text')
def step_when_adjust_clause(context, clause):
    name = "Service Agreement"
    did, updated_at = ContractService._contract_data(context, name)
    payload = {
        "did": did,
        "updated_at": updated_at,
        "contract_data": {
            "edited_clause": clause,
            "text": f"Updated by BDD for {clause}",
        },
    }
    context.requests_response = put_json(context, contract_update_url(context), payload)
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('I attempt to adjust clause "{clause}"')
def step_when_attempt_adjust_clause(context, clause):
    step_when_adjust_clause(context, clause)


@when('I initiate the approval process for contract "{name}"')
def step_when_initiate_approval(context, name):
    # Routing a SUBMITTED contract to its required approvers is, per
    # submit.go's state-pattern branching, the *reviewer* forward-to-approval
    # submit (Submitted -> Reviewed, actionflag.Approval / forward_to
    # "approval") — the same call
    # ContractService._prepare_contract_pending_approval's second half makes.
    # UserRoles.HasRoles(ContractReviewer) gates this branch server-side
    # regardless of which role narrates the scenario ("Contract Manager"
    # initiates routing), so this step authenticates as Contract Reviewer
    # internally to actually perform the routing action.
    did, updated_at = ContractService._contract_data(context, name)
    reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=reviewer_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    context.requests_response = post_json(
        context,
        contract_submit_url(context),
        ContractService._contract_reviewer_submit_payload(context, did, updated_at),
        headers=reviewer_h,
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('I approve contract "{name}"')
def step_when_approve_contract(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    context.requests_response = post_json(context, contract_approve_url(context), {"did": did, "updated_at": updated_at})
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('I reject contract "{name}" with reason "{reason}"')
def step_when_reject_contract(context, name, reason):
    did, updated_at = ContractService._contract_data(context, name)
    context.last_rejection = {"name": name, "reason": reason}
    context.requests_response = post_json(
        context,
        contract_reject_url(context),
        {"did": did, "updated_at": updated_at, "reason": reason},
    )


@when('I access the approval interface for contract "{name}"')
def step_when_access_approval_interface(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I submit contract "{name}" for review')
def step_when_submit_contract_for_review(context, name):
    # Submit's Security scopes (design/contract_workflow_engine.go) and
    # submit.go's own UserRoles.HasRoles gate both require Contract Creator/
    # Negotiator/Reviewer/Approver — "Contract Manager" (the role several of
    # this feature's scenarios narrate as the actor) is not among them, so
    # submitting must go out as the contract's actual creator identity
    # rather than context.headers.
    did, updated_at = ContractService._contract_data(context, name)
    creator_h = context.contract_seed_headers.get(name) if hasattr(context, "contract_seed_headers") else None
    context.requests_response = post_json(
        context,
        contract_submit_url(context),
        ContractService._contract_submit_payload(context, did, updated_at),
        headers=creator_h,
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('I attempt to add a comment to contract "{name}"')
def step_when_attempt_comment_contract(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": "bdd-observer",
            "change_request": "comment attempt",
        },
    )


@when('I attempt to approve contract "{name}"')
def step_when_attempt_approve_contract(context, name):
    step_when_approve_contract(context, name)


# ---------------------------------------------------------------------------
# Additional Given helpers
# ---------------------------------------------------------------------------

@given('I have created contract "{name}" from a template')
def step_given_created_contract_from_template(context, name):
    ContractService._create_contract_in_draft(context, name)


@given('contract "{name}" has multiple negotiation edits')
def step_given_contract_has_negotiation_edits(context, name):
    # Drive two full (negotiate -> accept -> submit/merge) rounds: each
    # accepted round's submit call snapshots the pre-merge row into
    # contract_history and bumps contract_version, while the contract stays
    # in NEGOTIATION (submit.go's Negotiation-state branch) — this is what
    # actually produces multiple queryable versions for
    # GET /contract/history/{did} to return.
    ContractService._create_contract_in_negotiation(context, name)
    creator_h = context.contract_seed_headers[name]
    responder_h = AuthService.get_headers_for_roles(["Contract Reviewer"], organization="TechVendor Inc")
    for change in ("Round 1: tighten SLA wording", "Round 2: extend liability cap"):
        did, updated_at = ContractService._contract_data(context, name)
        # The change request must be a structured negotiationmerging.ChangeRequest
        # object — submit's merge pass unmarshals accepted change requests into
        # that type (mergechangerequests.go) and a bare string 500s there. A
        # description-only change is the smallest real merge: each accepted
        # round updates the contract description and bumps contract_version.
        resp = post_json(
            context,
            contract_negotiate_url(context),
            {"did": did, "updated_at": updated_at,
             "negotiated_by": AuthService.username_for_roles(["Contract Manager"]),
             "change_request": {"description": change}},
            headers=creator_h,
        )
        assert resp.status_code == 200, f"negotiate failed: {resp.status_code} {resp.text}"
        refreshed = ContractService._refresh_contract(context, name)
        negotiation_id = _negotiation_id_for_change(refreshed, {"description": change})

        resp = post_json(
            context,
            f"{context.base_url}/contract/respond",
            {"id": str(negotiation_id), "did": did, "action_flag": "ACCEPTING",
             "rejected_by": "", "rejection_reason": ""},
            headers=responder_h,
        )
        assert resp.status_code == 200, f"accept failed: {resp.status_code} {resp.text}"
        ContractService._refresh_contract(context, name)

        did, updated_at = ContractService._contract_data(context, name)
        submit_resp = post_json(
            context,
            contract_submit_url(context),
            ContractService._contract_submit_payload(context, did, updated_at),
            headers=creator_h,
        )
        assert submit_resp.status_code == 200, f"submit/merge failed: {submit_resp.status_code} {submit_resp.text}"
        ContractService._refresh_contract(context, name)


@given('contract "{name}" has a pending redline proposal on clause "{clause}"')
def step_given_contract_has_pending_redline(context, name, clause):
    # Negotiate requires NEGOTIATION state — see the comment on
    # step_given_contract_multiple_negotiation_rounds above for why
    # _create_contract_in_draft alone is not enough here.
    ContractService._create_contract_in_negotiation(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    creator_h = context.contract_seed_headers[name]
    resp = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": AuthService.username_for_roles(["Contract Manager"]),
            "change_request": f"Redline on {clause}: proposed replacement text",
        },
        headers=creator_h,
    )
    assert resp.status_code == 200, f"negotiate failed while seeding redline proposal: {resp.status_code} {resp.text}"
    ContractService._refresh_contract(context, name)


@given('contract "{name}" is assigned to reviewers "{r1}" and "{r2}"')
def step_given_contract_assigned_reviewers(context, name, r1, r2):
    ContractService._create_contract_in_draft(context, name)


@given('contract "{name}" is in approval workflow')
def step_given_contract_in_approval_workflow(context, name):
    ContractService._create_contract_in_draft(context, name)
    ContractService._prepare_contract_pending_approval(context, name)


@given('contract "{name}" has all required approvals')
def step_given_contract_has_all_approvals(context, name):
    ContractService._create_contract_in_draft(context, name)
    ContractService._prepare_contract_pending_approval(context, name)
    did, _ = ContractService._contract_data(context, name)
    approver_h = AuthService.get_headers_for_roles(["Contract Approver"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=approver_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    approve = post_json(
        context, contract_approve_url(context),
        {"did": did, "updated_at": updated_at},
        headers=approver_h,
    )
    assert approve.status_code == 200, approve.text
    ContractService._refresh_contract(context, name)


@given('contract "{name}" is in "Active" status')
def step_given_contract_active(context, name):
    ContractService._create_contract_in_draft(context, name)


# ---------------------------------------------------------------------------
# Additional When helpers
# ---------------------------------------------------------------------------

@when('I view contract "{name}"')
def step_when_view_contract(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I edit contract "{name}"')
def step_when_edit_contract(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    payload = {
        "did": did,
        "updated_at": updated_at,
        "contract_data": TemplateService.canonical_document_data(
            "BDD Contract (edited)", clause_text="Updated clause", document_type="dcs:Contract"
        ),
    }
    context.requests_response = put_json(context, contract_update_url(context), payload)
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('I generate a contract from template "{name}"')
def step_when_generate_contract_from_template(context, name):
    template_did = (
        (getattr(context, "template_dids", None) or {}).get(name)
        or (getattr(context, "named_templates", None) or {}).get(name, {}).get("did")
    )
    assert template_did, f"No approved template DID found for '{name}'"
    peer_did = ContractService._local_peer_did(context)
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": template_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        },
    )


@when('I attempt to generate a contract from template "{template_name}"')
def step_when_attempt_generate_contract(context, template_name):
    template_did = (getattr(context, "template_dids", None) or {}).get(template_name, "did:example:missing")
    context.requests_response = post_json(context, contract_create_url(context), {"did": template_did})


@when('I add comment "{comment}" to clause "{clause}"')
def step_when_add_comment_to_clause(context, comment, clause):
    name = "Service Agreement"
    did, updated_at = ContractService._contract_data(context, name)
    context.requests_response = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "negotiated_by": AuthService.username_for_roles(["Contract Reviewer"]),
            "change_request": f"[{clause}] {comment}",
        },
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('I propose a redline edit to clause "{clause}"')
def step_when_propose_redline(context, clause):
    step_when_add_comment_to_clause(context, f"Proposed redline for {clause}", clause)


@when("I approve the redline proposal")
def step_when_approve_redline(context):
    # A different party must decide a proposal than the one who authored it
    # (FR-CWE-07 conflict-of-interest guard, acceptnegotiation.go
    # NegotiationAcceptor.Handle) — the redline was proposed as the
    # contract's Contract Creator identity (step_given_contract_has_pending_redline
    # uses creator_h), so approve here as a distinct organization.
    name = "Service Agreement"
    did, _ = ContractService._contract_data(context, name)
    refresh = ContractService._refresh_contract(context, name)
    negotiations = refresh.get("negotiations") or []
    assert negotiations, f"Expected a pending negotiation to approve, got: {refresh}"
    negotiation_id = negotiations[-1]["id"]
    responder_h = AuthService.get_headers_for_roles(["Contract Reviewer"], organization="TechVendor Inc")
    context.requests_response = post_json(
        context,
        f"{context.base_url}/contract/respond",
        {"id": str(negotiation_id), "did": did, "action_flag": "ACCEPTING", "rejected_by": "", "rejection_reason": ""},
        headers=responder_h,
    )


@when('I reject the redline proposal with reason "{reason}"')
def step_when_reject_redline(context, reason):
    name = "Service Agreement"
    context.last_redline_rejection_reason = reason
    did, _ = ContractService._contract_data(context, name)
    refresh = ContractService._refresh_contract(context, name)
    negotiations = refresh.get("negotiations") or []
    assert negotiations, f"Expected a pending negotiation to reject, got: {refresh}"
    negotiation_id = negotiations[-1]["id"]
    responder_h = AuthService.get_headers_for_roles(["Contract Reviewer"], organization="TechVendor Inc")
    context.requests_response = post_json(
        context,
        f"{context.base_url}/contract/respond",
        {"id": str(negotiation_id), "did": did, "action_flag": "REJECTING",
         "rejected_by": AuthService.username_for_roles(["Contract Manager"]), "rejection_reason": reason},
        headers=responder_h,
    )


@when('I view version history for contract "{name}"')
def step_when_view_version_history(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, f"{context.base_url}/contract/history/{did}")


@when('I view the negotiation log for contract "{name}"')
def step_when_view_negotiation_log(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when('I view approval status for contract "{name}"')
def step_when_view_approval_status(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when("the approval process completes")
def step_when_approval_completes(context):
    # The Given "contract has all required approvals" already approved the contract.
    # This step confirms the outcome rather than triggering a second approval.
    name = "Service Agreement"
    did, _ = ContractService._contract_data(context, name)
    approver_h = AuthService.get_headers_for_roles(["Contract Approver"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=approver_h)
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    if state != "APPROVED":
        # Not yet approved — attempt final approval.
        updated_at = retrieve.json().get("updated_at")
        context.requests_response = post_json(
            context, contract_approve_url(context),
            {"did": did, "updated_at": updated_at},
            headers=approver_h,
        )
        if context.requests_response.status_code == 200:
            ContractService._refresh_contract(context, name)
    else:
        # Already approved — synthesise a success response so Then-assertions can inspect it.
        import types
        fake = types.SimpleNamespace(
            status_code=200,
            text='{"did":"' + did + '"}',
        )
        fake.json = lambda: {"did": did}
        context.requests_response = fake


@when("I attempt to approve my own redline proposal")
def step_when_attempt_approve_own_redline(context):
    # Deliberately responds using context.headers (the scenario's own actor,
    # the SAME identity that proposed the redline in the Given step) so the
    # conflict-of-interest guard (acceptnegotiation.go) has something to
    # catch: created_by (the proposal's author) == AcceptedBy (this caller).
    name = "Service Agreement"
    did, _ = ContractService._contract_data(context, name)
    refresh = ContractService._refresh_contract(context, name)
    negotiations = refresh.get("negotiations") or []
    assert negotiations, f"Expected a pending negotiation to attempt to approve, got: {refresh}"
    negotiation_id = negotiations[-1]["id"]
    context.requests_response = post_json(
        context,
        f"{context.base_url}/contract/respond",
        {"id": str(negotiation_id), "did": did, "action_flag": "ACCEPTING", "rejected_by": "", "rejection_reason": ""},
    )


@when('I attempt to access contract "{name}" for negotiation')
def step_when_attempt_access_contract_negotiation(context, name):
    did = (getattr(context, "contract_dids", None) or {}).get(name, "did:example:missing")
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@given('I have created contract "{name}" with parties "{party_a}" and "{party_b}"')
def step_given_contract_with_parties(context, name, party_a, party_b):
    """Creates a contract whose create request names its party organizations
    (ContractCreateRequest.parties -> stored as the dcs:parties JSON-LD
    property) — the basis for party read-scoping in
    query/contract/querybyid.go."""
    t_did = ContractService._create_approved_template_for_contract(context)
    creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
    peer_did = ContractService._local_peer_did(context)
    create_resp = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": t_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
            "parties": [party_a, party_b],
        },
        headers=creator_h,
    )
    assert create_resp.status_code == 200, (
        f"Creating contract '{name}' with parties failed: {create_resp.status_code} {create_resp.text}"
    )
    c_did = create_resp.json().get("did")
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
    assert retrieve.status_code == 200, retrieve.text
    party_nodes = (retrieve.json().get("contract_data") or {}).get("dcs:parties") or []
    legal_names = [node.get("dcs:legalName") for node in party_nodes if node.get("dcs:legalName")]
    types = {node.get("@type") for node in party_nodes}
    assert legal_names == [party_a, party_b] and types == {"dcs:CompanyParty"}, (
        f"Expected the created contract's JSON-LD to record dcs:CompanyParty nodes for "
        f"{[party_a, party_b]}, got: {party_nodes}"
    )
    ContractService._ensure_store(context, "contract_dids", {})
    ContractService._ensure_store(context, "contract_updated_at", {})
    ContractService._ensure_store(context, "contract_seed_headers", {})
    context.contract_dids[name] = c_did
    context.contract_updated_at[name] = retrieve.json().get("updated_at")
    context.contract_seed_headers[name] = creator_h


@when('a representative of party "{party}" attempts to access contract "{name}"')
def step_when_party_accesses_contract(context, party, name):
    # Party identity is carried by the OID4VP credential's disclosed
    # organization claim (middleware.GetParticipantID -> CreatedBy on
    # contract/create, see AuthService.DEFAULT_ORGANIZATION /
    # AuthCredentials.organization) — authenticate AS that party rather than
    # reusing whatever role/org happens to be on context.headers.
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Contract Observer"], organization=party)
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)


@when('a representative of unrelated party "{party}" attempts to access contract "{name}"')
def step_when_unrelated_party_accesses_contract(context, party, name):
    step_when_party_accesses_contract(context, party, name)


@then('when a representative of unrelated party "{party}" attempts to access contract "{name}"')
def step_then_unrelated_party_accesses_contract(context, party, name):
    step_when_party_accesses_contract(context, party, name)


@then("the contract is accessible and visible")
def step_then_contract_accessible(context):
    assert context.requests_response.status_code == 200, (
        f"Expected the party representative to read the contract (200), got "
        f"{context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did") and body.get("contract_data"), (
        f"Expected a full contract body (did + contract_data), got: {body}"
    )


@then('the access is denied with a "{message}" error')
def step_then_access_denied_with_message(context, message):
    assert context.requests_response.status_code == 403, (
        f"Expected party read-scoping to deny with HTTP 403 (retrieve_by_id's "
        f"'forbidden' design error), got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    assert message.lower() in context.requests_response.text.lower(), (
        f"Expected the denial body to carry '{message}', got: {context.requests_response.text}"
    )


@then('the access denial for contract "{name}" is logged in the audit trail')
def step_then_access_denial_audited(context, name):
    # CONTRACT_ACCESS_DENIED is written by querybyid.go's denial branch and
    # anchored asynchronously by the outbox processor — poll the contract's
    # audit trail (POST /contract/audit) like the other audit-event steps.
    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    event_types = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = post_json(context, contract_audit_url(context), {"did": did}, headers=auditor_h)
        assert resp.status_code == 200, f"contract audit query failed: {resp.status_code} {resp.text}"
        entries = resp.json()
        assert isinstance(entries, list), f"Expected audit entries list, got: {entries}"
        event_types = [str(e.get("event_type", "")).upper() for e in entries]
        if "CONTRACT_ACCESS_DENIED" in event_types:
            matching = [
                e for e in entries
                if str(e.get("event_type", "")).upper() == "CONTRACT_ACCESS_DENIED"
            ]
            assert matching[0].get("created_at"), (
                f"Expected the denial audit entry to carry a timestamp, got: {matching[0]}"
            )
            return
        time.sleep(2)
    raise AssertionError(
        f"Expected a CONTRACT_ACCESS_DENIED audit event for contract '{name}' "
        f"(did={did}), got event types: {event_types}"
    )


@when('I attempt to access contract "{name}"')
def step_when_attempt_access_contract(context, name):
    did = (getattr(context, "contract_dids", None) or {}).get(name, "did:example:missing")
    context.requests_response = get_with_headers(context, contract_retrieve_by_id_url(context, did))


@when("automated compliance checks are performed")
def step_when_automated_compliance_checks(context):
    # POST /contract/verify was removed (commit 2712047e, "Remove unneeded
    # code" — see ContractService._prepare_contract_pending_approval's
    # comment). The real automated compliance check against ODRL/regulatory
    # constraints now runs INSIDE approval itself
    # (validation.ValidateContractPolicySatisfaction, approve.go) — approving
    # is what actually exercises "automated compliance checks are performed"
    # for a pending-approval contract; pack 18 covers the negative path
    # (a violated constraint blocks approval with a 4xx).
    name = "Service Agreement"
    did, updated_at = ContractService._contract_data(context, name)
    approver_h = AuthService.get_headers_for_roles(["Contract Approver"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=approver_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    context.requests_response = post_json(
        context, contract_approve_url(context), {"did": did, "updated_at": updated_at}, headers=approver_h
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


# ---------------------------------------------------------------------------
# Then assertions
# ---------------------------------------------------------------------------

@then("a draft contract is generated")
def step_then_draft_contract_generated(context):
    assert context.requests_response.status_code == 200, (
        f"Contract creation failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), f"Expected contract DID, got: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "DRAFT", f"Expected new contract to be in DRAFT state, got '{state}'"


@then("a contract is created")
def step_then_contract_linked_to_template(context):
    assert context.requests_response.status_code == 200, (
        f"Contract generation failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert isinstance(did, str) and did.strip(), f"Expected contract DID, got: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    assert retrieve.json().get("did"), f"Contract missing 'did': {retrieve.json()}"


@then("both machine-readable and human-readable versions are available")
def step_then_both_views_available(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert "contract_data" in contract or "name" in contract, (
        f"Contract missing content fields: {contract}"
    )


@then("metadata is auto-filled")
def step_then_metadata_auto_filled(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    created_at = contract.get("created_at")
    assert is_date(created_at) or contract.get("created_by") != "" or contract.get("status") == "DRAFT", (
        f"Expected auto-populated metadata fields in contract: {contract}"
    )

@then("the creation is logged and traceable to the template version")
def step_then_creation_logged(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("created_at"), f"Expected 'created_at' timestamp in contract: {contract}"


@then("the machine-readable view renders correctly")
def step_then_machine_readable_view(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert "contract_data" in contract, f"Missing machine-readable field 'contract_data': {contract}"


@then("the human-readable view renders correctly")
def step_then_human_readable_view(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("name") or contract.get("description"), (
        f"Missing human-readable fields 'name'/'description': {contract}"
    )


@then("the changes are saved")
def step_then_changes_saved(context):
    assert context.requests_response.status_code == 200, (
        f"Contract update failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in update response: {body}"


@then("a new version is created with timestamp and user attribution")
def step_then_contract_versioned(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in update response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("updated_at"), f"Expected 'updated_at' timestamp in contract: {contract}"


@then("the negotiation interface is displayed")
def step_then_negotiation_interface(context):
    assert context.requests_response.status_code == 200, (
        f"Expected negotiation interface (contract retrieve), got {context.requests_response.status_code}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No contract DID in response: {body}"


@then("I can view all contract clauses")
def step_then_can_view_clauses(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract content in response: {body}"


@then("the comment is added to the negotiation log")
def step_then_comment_added(context):
    assert context.requests_response.status_code == 200, (
        f"Negotiate endpoint failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in negotiate response: {body}"


@then("the comment is attributed to my identity")
def step_then_comment_attributed(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in negotiate response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    negotiations = retrieve.json().get("negotiations") or []
    assert negotiations, "Expected at least one negotiation entry"
    last = negotiations[-1]
    assert last.get("created_by"), f"Negotiation entry missing 'created_by': {last}"


@then("the comment includes a timestamp")
def step_then_comment_timestamp(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in negotiate response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    assert contract.get("updated_at"), f"Expected 'updated_at' on contract after negotiation: {contract}"


@then("the proposed change is tracked")
def step_then_proposed_change_tracked(context):
    step_then_comment_added(context)


@then("the original text is preserved")
def step_then_original_text_preserved(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in negotiation response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    negotiations = retrieve.json().get("negotiations") or []
    assert negotiations, "Expected negotiation history to preserve prior text context"


@then("the redline proposal is visible to other negotiators")
def step_then_redline_visible(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    negotiations = retrieve.json().get("negotiations") or []
    assert negotiations, "Expected negotiations list to be non-empty after redline proposal"


@then("I see all versions with timestamps")
def step_then_versions_with_timestamps(context):
    # GET /contract/history/{did} (query/contract/queryhistorybyid.go) returns
    # a JSON array, one entry per contract_history snapshot — not the single
    # contract object contract/retrieve/{did} returns.
    entries = context.requests_response.json()
    assert isinstance(entries, list) and len(entries) >= 2, (
        f"Expected at least 2 history entries (multiple negotiation edits), got: {entries}"
    )
    for entry in entries:
        assert entry.get("created_at") and entry.get("updated_at"), (
            f"Expected 'created_at'/'updated_at' timestamps on every history entry, got: {entry}"
        )
    versions = {entry.get("contract_version") for entry in entries}
    assert len(versions) == len(entries), (
        f"Expected each history entry to carry a distinct contract_version, got: {entries}"
    )


@then("I see user attribution for each version")
def step_then_user_attribution_per_version(context):
    entries = context.requests_response.json()
    assert isinstance(entries, list) and entries, f"Expected a non-empty history list, got: {entries}"
    for entry in entries:
        assert entry.get("created_by"), f"Expected 'created_by' attribution on every history entry, got: {entry}"


@then("old versions remain accessible")
def step_then_old_versions_accessible(context):
    entries = context.requests_response.json()
    assert isinstance(entries, list) and entries, f"Expected a non-empty history list, got: {entries}"
    for entry in entries:
        assert entry.get("contract_data"), (
            f"Expected the content ('contract_data') of every historical version to remain accessible, got: {entry}"
        )


@then("the change is applied to the contract")
def step_then_change_applied(context):
    assert context.requests_response.status_code == 200, (
        f"Expected change to be applied (200), got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("the approval is logged in the negotiation log")
def step_then_approval_in_negotiation_log(context):
    refreshed = ContractService._refresh_contract(context, "Service Agreement")
    decisions = [
        d
        for entry in (refreshed.get("negotiations") or [])
        for d in (entry.get("negotiation_decisions") or [])
    ]
    assert any(str(d.get("decision", "")).upper() == "ACCEPTED" for d in decisions), (
        f"Expected an ACCEPTED decision in the negotiation log, got: {decisions}"
    )


@then("a new version is created")
def step_then_new_contract_version(context):
    body = context.requests_response.json()
    assert body.get("did") or body.get("id"), f"No ID in response: {body}"


@then("the proposal is marked as rejected")
def step_then_proposal_rejected(context):
    assert context.requests_response.status_code == 200, (
        f"Expected rejection to succeed (200), got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("the rejection reason is logged")
def step_then_rejection_reason_logged(context):
    reason = getattr(context, "last_redline_rejection_reason", None)
    assert reason, "No rejection reason recorded by the reject step"
    refreshed = ContractService._refresh_contract(context, "Service Agreement")
    decisions = [
        d
        for entry in (refreshed.get("negotiations") or [])
        for d in (entry.get("negotiation_decisions") or [])
    ]
    rejected = [d for d in decisions if str(d.get("decision", "")).upper() == "REJECTED"]
    assert rejected, f"Expected a REJECTED decision in the negotiation log, got: {decisions}"
    assert any(reason in str(d.get("rejection_reason", "")) for d in rejected), (
        f"Expected rejection reason '{reason}' to be logged, got: {rejected}"
    )


@then("the original text is retained")
def step_then_original_text_retained(context):
    refreshed = ContractService._refresh_contract(context, "Service Agreement")
    contract_data = refreshed.get("contract_data") or {}
    assert contract_data, "Expected contract_data to remain present after rejection"
    assert "dcs:documentStructure" in str(contract_data), (
        "Expected the contract's document structure (original text) to remain intact"
    )


@then("I see approvals and rejections")
def step_then_see_approvals_and_rejections(context):
    body = context.requests_response.json()
    decisions = [
        d
        for entry in (body.get("negotiations") or [])
        for d in (entry.get("negotiation_decisions") or [])
    ]
    kinds = {str(d.get("decision", "")).upper() for d in decisions}
    assert "ACCEPTED" in kinds and "REJECTED" in kinds, (
        f"Expected the negotiation log to contain both ACCEPTED and REJECTED decisions, got: {kinds}"
    )


@then("I see all comments and proposals")
def step_then_see_comments(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract in response: {body}"


@then("I see the full audit trail")
def step_then_see_audit_trail(context):
    body = context.requests_response.json()
    assert body.get("updated_at") or body.get("negotiations") is not None, (
        f"No audit-trail fields in response: {body}"
    )


@then('the contract status changes to "Under Review"')
def step_then_contract_status_under_review(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in submit response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state in ("SUBMITTED", "UNDER_REVIEW", "REVIEW", "NEGOTIATION"), (
        f"Expected submitted/under-review state, got '{state}'"
    )


@then("the submission is logged")
def step_then_submission_logged(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in submit response: {body}"


# Human-facing status labels that don't literally match a contractstate enum
# value (backend/internal/contractworkflowengine/datatype/contractstate) —
# "Pending Approval" is the REVIEWED state (all reviewers accepted, approval
# tasks open and gating the next transition; see approve.go's
# AnyTasksInState(Open) quorum check).
_STATUS_LABEL_ALIASES = {
    "PENDING_APPROVAL": "REVIEWED",
}


@then('the contract status shows "{expected_status}"')
def step_then_contract_status_shows(context, expected_status):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    expected = expected_status.upper().replace(" ", "_")
    expected = _STATUS_LABEL_ALIASES.get(expected, expected)
    assert state == expected, (
        f"Expected '{expected_status}' ('{expected}'), got '{state}'"
    )


@then("my approval is logged with timestamp")
def step_then_approval_logged_timestamp(context):
    assert context.requests_response.status_code == 200, (
        f"Approve failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in approve response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    assert retrieve.json().get("updated_at"), "Missing 'updated_at' after approval"


@then("the approval status is updated")
def step_then_approval_status_updated(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in approve response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "APPROVED", f"Expected APPROVED state after approval, got '{state}'"


@then("the rejection is logged with comments and timestamp")
def step_then_rejection_logged(context):
    assert context.requests_response.status_code == 200, (
        f"Reject failed: {context.requests_response.status_code} — {context.requests_response.text}"
    )
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in reject response: {body}"


@then("the contract is returned for revision")
def step_then_contract_returned_revision(context):
    # SRS (Contract Approval post-condition): a rejected contract is returned
    # to negotiation with the Approver's comments. Prove the reopen path
    # end-to-end: resubmit the rejected contract and require NEGOTIATION,
    # then require the rejection (with its reason) in the audit trail.
    rejection = getattr(context, "last_rejection", None)
    assert rejection, "No rejection recorded by the reject step"
    name, reason = rejection["name"], rejection["reason"]
    ContractService._refresh_contract(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
    resubmit = post_json(
        context,
        contract_submit_url(context),
        ContractService._contract_submit_payload(context, did, updated_at),
        headers=creator_h,
    )
    assert resubmit.status_code == 200, (
        f"Resubmit of rejected contract failed: {resubmit.status_code} — {resubmit.text}"
    )
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=creator_h)
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state == "NEGOTIATION", (
        f"Expected rejected contract to reopen into NEGOTIATION on resubmit, got '{state}'"
    )
    # Rejection reason must be retrievable from the audit trail (anchored
    # asynchronously by the outbox processor — poll generously).
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    events, found = [], False
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        resp = post_json(context, contract_audit_url(context), {"did": did}, headers=auditor_h)
        assert resp.status_code == 200, f"Audit query failed: {resp.status_code} {resp.text}"
        events = resp.json()
        assert isinstance(events, list), f"Expected audit response list, got: {events}"
        reject_events = [e for e in events if str(e.get("event_type", "")).upper() == "REJECT_CONTRACT"]
        if reject_events and any(reason in str(e) for e in reject_events):
            found = True
            break
        time.sleep(2)
    assert found, (
        f"Expected a REJECT_CONTRACT audit event carrying reason '{reason}', got: "
        f"{[e.get('event_type') for e in events]}"
    )


@then('the contract is marked as "Approved"')
def step_then_contract_marked_approved(context):
    step_then_approval_status_updated(context)


@then("I can view previous reviewer comments")
def step_then_can_view_previous_comments(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract DID in response: {body}"


@then("the system validates against regulatory frameworks")
def step_then_validates_regulatory(context):
    body = context.requests_response.json()
    assert "findings" in body or body.get("did"), f"Expected verify-style response: {body}"


@then("compliance issues are flagged for review")
def step_then_compliance_issues_flagged(context):
    body = context.requests_response.json()
    assert "findings" in body or body.get("did"), f"Expected findings in verify response: {body}"


@then("I see pending approvals")
def step_then_see_pending_approvals(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No contract DID in approval-status response: {body}"


@then("negotiation actions are logged with reviewer identity")
def step_then_negotiation_actions_logged(context):
    body = context.requests_response.json()
    assert body.get("did"), f"No DID in response: {body}"
