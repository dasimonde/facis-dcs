"""BDD steps for the contract-hierarchy invariant: a child links to its
parent via a single dcs:parentContract reference, never the reverse
(FR-TR-02, FR-CWE-02) — plus the parent_did full-scope search filter and the
two-instance sibling isolation that follows structurally from the
child→parent-only direction.

The ZIP-bundle-export steps live in
steps/pdf_generation/dcs_bundle_export_steps.py, since they build directly
on PDFService/pdf_generation's existing export/verify HTTP helpers; that
module imports the two helpers this one defines
(`_minimal_canonical_contract_data`, `_link_contract_to_parent`) to build
parent/sibling fixtures for the bundle scenarios.

contract/update only allows EventUpdate from the Draft state (see
backend/internal/contractworkflowengine/datatype/contractstate/
transition.go's Transitions[Draft][EventUpdate] — no other source state has
that edge). Every dcs:parentContract link below is therefore established
while the contract is still in Draft, before any further state advance.
"""

import time

import requests as _requests
from behave import given, then, when

from steps.support.api_client import (
    contract_create_url,
    contract_offer_url,
    contract_retrieve_by_id_url,
    contract_search_url,
    contract_submit_url,
    contract_update_url,
    get_with_headers,
    post_json,
    put_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService

# Deferred import (not at module load time): steps/peer_trust/dcs_peer_trust_steps.py
# imports steps.template_management.contract_state_machine_steps, and
# steps/template_management/__init__.py imports this module — a top-level
# import here would form a circular import that behave's step-loader trips
# over (partially-initialized module error). _as_instance is only needed
# inside step_given_two_instances_two_children below, so it is imported
# lazily there instead.


# ---------------------------------------------------------------------------
# Shared helpers (also imported by steps/pdf_generation/dcs_bundle_export_steps.py)
# ---------------------------------------------------------------------------


def _minimal_canonical_contract_data(extra_top_level: dict | None = None) -> dict:
    """A minimal, schema-valid canonical dcs:Contract envelope (same shape
    ContractService._create_approved_template_for_contract already uses for
    template_data) plus optional extra top-level JSON-LD properties — enough
    to pass NormalizeContractData's canonical-envelope branch
    (validateCanonicalEnvelope/validateCanonicalReferences) without
    depending on unrelated ODRL/semantic fields this requirement doesn't
    cover."""
    data = {
        "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
        "@type": "dcs:Contract",
        "dcs:metadata": {
            "@type": "dcs:ContractMetadata",
            "dcs:title": "BDD Hierarchy Contract",
        },
        "dcs:documentStructure": {
            "@type": "dcs:DocumentStructure",
            "dcs:blocks": {
                "@list": [
                    {
                        "@id": "urn:uuid:block-clause-1",
                        "@type": "dcs:Clause",
                        "dcs:content": {"@list": ["Base clause"]},
                    }
                ]
            },
            "dcs:layout": [
                {
                    "@id": "urn:uuid:block-root",
                    "dcs:isRoot": True,
                    "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]},
                }
            ],
        },
    }
    if extra_top_level:
        data.update(extra_top_level)
    return data


def _link_contract_to_parent(context, child_name: str, parent_name: str):
    """PUT /contract/update on `child_name` with a single dcs:parentContract
    reference pointing at `parent_name`. Does NOT assert the outcome — some
    callers (the parent-cycle scenario) need to observe a rejection."""
    child_did, updated_at = ContractService._contract_data(context, child_name)
    parent_did, _ = ContractService._contract_data(context, parent_name)
    headers = context.contract_seed_headers[child_name]
    resp = put_json(
        context,
        contract_update_url(context),
        {
            "did": child_did,
            "updated_at": updated_at,
            "contract_data": _minimal_canonical_contract_data(
                {"dcs:parentContract": {"@id": parent_did}}
            ),
        },
        headers=headers,
    )
    if resp.status_code == 200:
        ContractService._refresh_contract(context, child_name)
    return resp


def _advance_existing_draft_to_negotiation(context, name: str):
    """Submit an EXISTING Draft contract once (Draft -> Negotiation), without
    re-creating it — used after a dcs:parentContract link has already been
    established while the contract was still Draft (contract/update only
    accepts EventUpdate from Draft)."""
    did, updated_at = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    resp = post_json(
        context,
        contract_submit_url(context),
        ContractService._contract_submit_payload(context, did, updated_at),
        headers=headers,
    )
    assert resp.status_code == 200, (
        f"Expected submitting '{name}' from Draft to Negotiation to succeed: "
        f"{resp.status_code} {resp.text}"
    )
    ContractService._refresh_contract(context, name)


def _ensure_contract_in_draft(context, name: str):
    if not hasattr(context, "contract_dids") or name not in context.contract_dids:
        ContractService._create_contract_in_draft(context, name)


# ---------------------------------------------------------------------------
# Given — invariant fixtures
# ---------------------------------------------------------------------------


@given('contract "{name}" exists with no parent reference')
def step_given_contract_no_parent(context, name):
    _ensure_contract_in_draft(context, name)


@given('contracts "{name_a}" and "{name_b}" exist locally with no parent reference')
def step_given_two_contracts_no_parent(context, name_a, name_b):
    _ensure_contract_in_draft(context, name_a)
    _ensure_contract_in_draft(context, name_b)


# ---------------------------------------------------------------------------
# Given — hierarchy fixtures (a single valid parent link is legitimate —
# the invariant forbids only multiple references, cycles, and
# child-enumerating properties)
# ---------------------------------------------------------------------------


@given('contract "{child_name}" references contract "{parent_name}" as its parent')
def step_given_contract_references_parent(context, child_name, parent_name):
    _ensure_contract_in_draft(context, parent_name)
    _ensure_contract_in_draft(context, child_name)
    resp = _link_contract_to_parent(context, child_name, parent_name)
    assert resp.status_code == 200, (
        f"Expected linking '{child_name}' to parent '{parent_name}' via a single "
        f"dcs:parentContract reference to succeed (the hierarchy invariant does not "
        f"forbid a single valid link, only >1 references / cycles / child-enumerating "
        f"properties): {resp.status_code} {resp.text}"
    )


@given(
    'contract "{child_name}" references contract "{parent_name}" as its parent, '
    'then reaches contract state "{state}"'
)
def step_given_contract_references_parent_then_state(context, child_name, parent_name, state):
    step_given_contract_references_parent(context, child_name, parent_name)
    normalized = state.strip().upper()
    if normalized == "DRAFT":
        return
    if normalized == "NEGOTIATION":
        _advance_existing_draft_to_negotiation(context, child_name)
        return
    raise NotImplementedError(
        f"No post-link state-advance path implemented for target state '{state}' — "
        "only DRAFT and NEGOTIATION are supported by this fixture step."
    )


# ---------------------------------------------------------------------------
# When — invariant violations
# ---------------------------------------------------------------------------


@when('contract "{name}" is updated with two dcs:parentContract references')
def step_when_update_two_parent_refs(context, name):
    unrelated_name = f"{name} :: unrelated-parent-target"
    _ensure_contract_in_draft(context, unrelated_name)
    unrelated_did, _ = ContractService._contract_data(context, unrelated_name)
    did, updated_at = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    context.requests_response = put_json(
        context,
        contract_update_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "contract_data": _minimal_canonical_contract_data(
                {
                    "dcs:parentContract": [
                        {"@id": unrelated_did},
                        {"@id": "did:example:bdd-second-unrelated-parent"},
                    ]
                }
            ),
        },
        headers=headers,
    )


@when('contract "{name}" is updated with a child-enumerating dcs:childContracts property')
def step_when_update_child_contracts_property(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    context.requests_response = put_json(
        context,
        contract_update_url(context),
        {
            "did": did,
            "updated_at": updated_at,
            "contract_data": _minimal_canonical_contract_data(
                {"dcs:childContracts": [{"@id": "did:example:bdd-fake-child"}]}
            ),
        },
        headers=headers,
    )


@when('contract "{child_name}" is updated to reference contract "{parent_name}" as its parent')
def step_when_set_parent(context, child_name, parent_name):
    context.requests_response = _link_contract_to_parent(context, child_name, parent_name)


# ---------------------------------------------------------------------------
# When/Then — parent_did search filter
# ---------------------------------------------------------------------------


@when('the contract search is queried with parent_did filter for contract "{parent_name}"')
def step_when_search_parent_did(context, parent_name):
    parent_did, _ = ContractService._contract_data(context, parent_name)
    headers = context.contract_seed_headers.get(parent_name) if hasattr(context, "contract_seed_headers") else None
    headers = headers or getattr(context, "headers", {})
    context.requests_response = _requests.get(
        contract_search_url(context),
        params={"parent_did": parent_did},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@then('the search results do not include contract "{name}"')
def step_then_search_excludes_contract(context, name):
    did, _ = ContractService._contract_data(context, name)
    results = context.requests_response.json()
    assert isinstance(results, list), f"Expected search response to be a list, got: {results}"
    dids = [r.get("did") for r in results]
    assert did not in dids, (
        f"Expected contract '{name}' ({did}) to be excluded from the parent_did-filtered "
        f"search results, got dids: {dids}"
    )


@then(
    'the search results for contract "{parent_name}" show contract "{child_name}" '
    'with state "{state}"'
)
def step_then_search_shows_child_state(context, parent_name, child_name, state):
    child_did, _ = ContractService._contract_data(context, child_name)
    results = context.requests_response.json()
    assert isinstance(results, list), f"Expected search response to be a list, got: {results}"
    matches = [r for r in results if r.get("did") == child_did]
    assert matches, (
        f"Expected linked child contract '{child_name}' ({child_did}) among the "
        f"parent_did-filtered results for '{parent_name}', got: {results}"
    )
    actual_state = str(matches[0].get("state", "")).upper()
    assert actual_state == state.strip().upper(), (
        f"Expected linked child contract '{child_name}' to show state '{state}' in the "
        f"parent_did-filtered results, got '{actual_state}': {matches[0]}"
    )


# ---------------------------------------------------------------------------
# Two-instance sibling isolation (@two-instance)
#
# Simulates the three-party frame scenario ("B's child bundle contains B's
# child + the frame, and nothing about C") with 2 physical instances: a frame
# + one child offered/replicated to instance B, plus a second sibling child
# created and kept local to instance A only (never offered). The isolation
# assertion is that instance B's own parent_did-filtered search NEVER
# surfaces the locally-kept sibling's DID — not because of an ACL, but
# because it structurally never left instance A (FR-CSA-26 "per-party-scope"
# property).
#
# This reuses steps/peer_trust/dcs_peer_trust_steps.py's
# `step_given_two_instances_running` ("instance A and instance B are both
# running and trust each other") and `_as_instance` context manager, so it
# fails the same honest way the peer-trust scenarios do when
# BDD_DCS_BASE_URL_A/_B or the second-instance runner are not present.
# ---------------------------------------------------------------------------


@when(
    "child contracts are created on instance A: one linked to a frame and offered to "
    "instance B, another linked to the same frame and kept local to instance A only"
)
def step_when_ac5_setup(context):
    from steps.peer_trust.dcs_peer_trust_steps import _as_instance

    with _as_instance(context, context.base_url_a):
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"], api_base=context.base_url_a)

        # Frame contract: stays local to A, never offered (no counterparty).
        frame_resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": t_did,
            },
            headers=creator_h,
        )
        assert frame_resp.status_code == 200, frame_resp.text
        frame_did = frame_resp.json().get("did")
        context.ac5_frame_did = frame_did

        # Child linked to the frame, offered to B (counterparty=B) so it replicates.
        child_b_resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": t_did,
                "counterparty": context.peer_did_b,
            },
            headers=creator_h,
        )
        assert child_b_resp.status_code == 200, child_b_resp.text
        child_b_did = child_b_resp.json().get("did")
        context.ac5_child_b_did = child_b_did

        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, child_b_did), headers=creator_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")
        link_resp = put_json(
            context,
            contract_update_url(context),
            {
                "did": child_b_did,
                "updated_at": updated_at,
                "contract_data": _minimal_canonical_contract_data({"dcs:parentContract": {"@id": frame_did}}),
            },
            headers=creator_h,
        )
        assert link_resp.status_code == 200, (
            f"Expected linking the offered child to the frame to succeed: "
            f"{link_resp.status_code} {link_resp.text}"
        )
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, child_b_did), headers=creator_h)
        updated_at = retrieve.json().get("updated_at")
        offer_resp = post_json(
            context, contract_offer_url(context), {"did": child_b_did, "updated_at": updated_at}, headers=creator_h
        )
        assert offer_resp.status_code == 200, offer_resp.text

        # Sibling child linked to the same frame, created and kept LOCAL to A only.
        child_c_resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": t_did,
            },
            headers=creator_h,
        )
        assert child_c_resp.status_code == 200, child_c_resp.text
        child_c_did = child_c_resp.json().get("did")
        context.ac5_child_c_did = child_c_did

        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, child_c_did), headers=creator_h)
        updated_at = retrieve.json().get("updated_at")
        link_c_resp = put_json(
            context,
            contract_update_url(context),
            {
                "did": child_c_did,
                "updated_at": updated_at,
                "contract_data": _minimal_canonical_contract_data({"dcs:parentContract": {"@id": frame_did}}),
            },
            headers=creator_h,
        )
        assert link_c_resp.status_code == 200, (
            f"Expected linking the local-only sibling to the frame to succeed: "
            f"{link_c_resp.status_code} {link_c_resp.text}"
        )


@then(
    "instance B's parent_did-filtered search for the frame includes the offered child "
    "but never the sibling that stayed local to instance A"
)
def step_then_ac5_isolation(context):
    manager_h_b = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)
    # Cross-instance replication is async (offer -> regen -> IPFS -> ship ->
    # PostPdf -> extract -> re-compact -> store); allow the same generous window
    # the peer-trust offer replication uses, since the shared cluster now also
    # runs the DSS bundle and the child goes through create -> update -> offer.
    deadline = time.monotonic() + 45
    dids = []
    last_resp = None
    while time.monotonic() < deadline:
        last_resp = _requests.get(
            f"{context.base_url_b}/contract/search",
            params={"parent_did": context.ac5_frame_did},
            headers=manager_h_b,
            timeout=context.http_timeout_seconds,
        )
        if last_resp.status_code == 200:
            dids = [r.get("did") for r in last_resp.json()]
            if context.ac5_child_b_did in dids:
                break
        time.sleep(1)
    assert context.ac5_child_b_did in dids, (
        f"Expected the offered/replicated child {context.ac5_child_b_did} to appear in "
        f"instance B's parent_did-filtered search results within 45s, got dids: {dids} "
        f"(last response {last_resp.status_code if last_resp else 'n/a'})"
    )
    assert context.ac5_child_c_did not in dids, (
        f"Expected the sibling child {context.ac5_child_c_did} (created locally on instance "
        f"A only, never offered/PostSynced to B) to NEVER appear in instance B's "
        f"parent_did-filtered search results, got dids: {dids}"
    )
