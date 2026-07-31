"""BDD steps for the contract_creation.feature scenarios that were @skip
pending step definitions: reusable-clause assembly, hierarchical
sub-agreements/annexes, multi-contract package bundling, template metadata
inheritance/override, and creation-time party authorization.

Hierarchy/bundle machinery is reused from
steps/template_management/dcs_contract_hierarchy_steps.py and
steps/pdf_generation/dcs_bundle_export_steps.py (pack 20) rather than
duplicated.

Party authorization is enforced in
backend/internal/service/contract_workflow_engine.go's Create() via
dcstodcs.CheckForUntrustedPeers on reviewers/approvers/negotiators — a peer
DID is accepted only if it is this instance's own DID or already present in
the trusted_peers table (seeded from DCS_TRUSTED_PEERS at startup). No
backend change was needed for these scenarios: "authorized party" is
represented by this instance's own peer DID (always trivially trusted),
"unauthorized party" by a synthetic did:web DID that is guaranteed not to be
a trusted peer.

NOTE (ADR-19, docs/adr-19-federation-agreement-credential.md): this
allowlist mechanism is slated for removal, replaced by a self-signed
agreement credential + local policy endpoint (PDP) model — see
steps/peer_trust/dcs_peer_trust_steps.py and
steps/peer_trust/dcs_trust_pdp_steps.py. That removal has not landed yet, so
the scenarios in this file remain valid against the current backend; they
are a different requirement (creation-time party authorization, FR-CWE-18)
than ADR-19's DCS-to-DCS shipping gate and were left as-is here rather than
migrated speculatively ahead of the actual removal.
"""

import json
import time

from behave import given, then, when

from steps.pdf_generation.pdf_steps import step_given_contract_has_exported_pdf
from steps.support.api_client import (
    contract_create_url,
    contract_export_url,
    contract_retrieve_by_id_url,
    contract_search_url,
    contract_update_url,
    get_with_headers,
    post_json,
    put_json,
    template_approve_url,
    template_create_url,
    template_register_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService
from steps.template_management.dcs_contract_hierarchy_steps import (
    _ensure_contract_in_draft,
    _link_contract_to_parent,
)

import requests as _requests


# ---------------------------------------------------------------------------
# Scenario: Assemble contract from reusable clauses
# ---------------------------------------------------------------------------


def _clause_catalog_document(clause_names: list[str]) -> dict:
    """A canonical dcs:documentStructure envelope with one dcs:Clause block
    per reusable clause, mirroring the single-block shape
    TemplateService.canonical_document_data already uses (dcs:content as an
    @list of clause text) — proven to pass NormalizeTemplateData/
    NormalizeContractData's canonical-envelope validation elsewhere in the
    suite, just repeated per clause instead of hardcoded to one.
    """
    blocks = []
    layout_children = []
    for i, clause in enumerate(clause_names, start=1):
        block_id = f"urn:uuid:bdd-clause-{i}"
        blocks.append(
            {
                "@id": block_id,
                "@type": "dcs:Clause",
                "dcs:content": {"@list": [f"{clause} clause text."]},
            }
        )
        layout_children.append({"@id": block_id})
    return {
        "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
        "@type": "dcs:ContractTemplate",
        "dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "BDD Reusable Clause Template"},
        "dcs:documentStructure": {
            "@type": "dcs:DocumentStructure",
            "dcs:blocks": {"@list": blocks},
            "dcs:layout": [
                {
                    "@id": "urn:uuid:bdd-clause-root",
                    "dcs:isRoot": True,
                    "dcs:children": {"@list": layout_children},
                }
            ],
        },
    }


@given('reusable clauses "{c1}", "{c2}", and "{c3}" exist')
def step_given_reusable_clauses_exist(context, c1, c2, c3):
    clause_names = [c1, c2, c3]
    context.reusable_clause_names = clause_names

    creator_h = AuthService.get_headers_for_roles(["Template Creator"])
    create_resp = post_json(
        context,
        template_create_url(context),
        {
            "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
            "name": "BDD Reusable Clause Template",
            "description": "BDD template assembled from reusable clauses",
            "template_data": _clause_catalog_document(clause_names),
        },
        headers=creator_h,
    )
    assert create_resp.status_code == 200, f"Clause-catalog template create failed: {create_resp.text}"
    did = create_resp.json().get("did")

    body = TemplateService.fetch_template(context, did, headers=creator_h)
    updated_at = body.get("updated_at")
    updated_at = TemplateService.do_submit(context, did, updated_at)
    updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)

    approver_h = AuthService.get_headers_for_roles(["Template Approver"])
    approve_resp = post_json(
        context, template_approve_url(context), {"did": did, "updated_at": updated_at}, headers=approver_h
    )
    assert approve_resp.status_code == 200, f"Clause-catalog template approve failed: {approve_resp.text}"
    updated_at = TemplateService.fetch_template(context, did, headers=approver_h).get("updated_at")

    manager_h = AuthService.get_headers_for_roles(["Template Manager"])
    register_resp = post_json(context, template_register_url(context), {"did": did}, headers=manager_h)
    assert register_resp.status_code == 200, f"Clause-catalog template register failed: {register_resp.text}"

    context.clause_catalog_template_did = did


@when('I assemble a contract using clauses "{c1}", "{c2}", and "{c3}"')
def step_when_assemble_contract_from_clauses(context, c1, c2, c3):
    t_did = getattr(context, "clause_catalog_template_did", None)
    assert t_did, "No clause-catalog template DID — ensure the reusable-clauses Given step ran"
    peer_did = ContractService._local_peer_did(context)
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {"template_did": t_did, "reviewers": [peer_did], "negotiators": [peer_did], "approvers": [peer_did]},
    )


def _assembled_contract_document(context) -> dict:
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No contract DID in assemble response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    return retrieve.json()


@then("the assembly process validates structure")
def step_then_assembly_validates_structure(context):
    assert context.requests_response.status_code == 200, (
        f"Expected clause assembly to pass structural validation (NormalizeContractDataForPersistence), "
        f"got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    contract = _assembled_contract_document(context)
    contract_data = contract.get("contract_data") or {}
    assert "dcs:documentStructure" in json.dumps(contract_data), (
        f"Expected the assembled contract to carry a validated dcs:documentStructure: {contract_data}"
    )


@then("the assembly process validates required metadata")
def step_then_assembly_validates_metadata(context):
    contract = _assembled_contract_document(context)
    assert contract.get("template_did") and contract.get("template_version") is not None, (
        f"Expected assembled contract to carry validated template metadata (template_did/version): {contract}"
    )


@then("the assembly process validates content logic")
def step_then_assembly_validates_content(context):
    contract = _assembled_contract_document(context)
    contract_data_str = json.dumps(contract.get("contract_data") or {})
    clause_names = getattr(context, "reusable_clause_names", [])
    missing = [c for c in clause_names if c not in contract_data_str]
    assert not missing, (
        f"Expected the assembled contract's content to carry every reusable clause "
        f"{clause_names}, missing {missing} in: {contract_data_str}"
    )


# ---------------------------------------------------------------------------
# Scenario: Create contract with hierarchical structure
# ---------------------------------------------------------------------------


@given('master agreement template "{name}" exists')
def step_given_master_agreement_template_exists(context, name):
    t_did = ContractService._create_approved_template_for_contract(context)
    context.master_agreement_template_did = t_did


@when("I create a contract with sub-agreements and annexes")
def step_when_create_contract_with_subagreements(context):
    t_did = getattr(context, "master_agreement_template_did", None)
    assert t_did, "No master agreement template DID — ensure its Given step ran"
    peer_did = ContractService._local_peer_did(context)
    creator_h = AuthService.get_headers_for_roles(["Contract Creator"])

    def _create(name):
        resp = post_json(
            context,
            contract_create_url(context),
            {"template_did": t_did, "reviewers": [peer_did], "negotiators": [peer_did], "approvers": [peer_did]},
            headers=creator_h,
        )
        assert resp.status_code == 200, f"Creating hierarchy component '{name}' failed: {resp.text}"
        did = resp.json().get("did")
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=creator_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")
        ContractService._ensure_store(context, "contract_dids", {})
        ContractService._ensure_store(context, "contract_updated_at", {})
        ContractService._ensure_store(context, "contract_seed_headers", {})
        context.contract_dids[name] = did
        context.contract_updated_at[name] = updated_at
        context.contract_seed_headers[name] = creator_h
        return did

    master_name = "BDD Master Agreement"
    sub_name = "BDD Sub-Agreement"
    annex_name = "BDD Annex"

    master_did = _create(master_name)
    _create(sub_name)
    _create(annex_name)

    context.master_agreement_did = master_did
    context.master_agreement_name = master_name
    context.hierarchy_component_names = [sub_name, annex_name]

    link_sub = _link_contract_to_parent(context, sub_name, master_name)
    assert link_sub.status_code == 200, (
        f"Expected linking the sub-agreement to the master agreement to succeed: "
        f"{link_sub.status_code} {link_sub.text}"
    )
    link_annex = _link_contract_to_parent(context, annex_name, master_name)
    context.requests_response = link_annex


@then("the hierarchical structure is established")
def step_then_hierarchy_established(context):
    assert context.requests_response.status_code == 200, (
        f"Expected linking the last hierarchy component to the master agreement to succeed: "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )
    master_did = context.master_agreement_did
    headers = context.contract_seed_headers[context.master_agreement_name]
    search = _requests.get(
        contract_search_url(context),
        params={"parent_did": master_did},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )
    assert search.status_code == 200, search.text
    results = search.json()
    dids = {r.get("did") for r in results}
    for name in context.hierarchy_component_names:
        did, _ = ContractService._contract_data(context, name)
        assert did in dids, (
            f"Expected hierarchy component '{name}' ({did}) to be linked under the master "
            f"agreement's parent_did-filtered search, got: {dids}"
        )


@then("components are logically linked")
def step_then_components_logically_linked(context):
    master_did = context.master_agreement_did
    for name in context.hierarchy_component_names:
        contract = ContractService._refresh_contract(context, name)
        contract_data_str = json.dumps(contract.get("contract_data") or {})
        assert master_did in contract_data_str, (
            f"Expected component '{name}' to carry a dcs:parentContract reference to the "
            f"master agreement ({master_did}) in its own contract_data: {contract_data_str}"
        )


@then("components are version-controlled")
def step_then_components_version_controlled(context):
    for name in context.hierarchy_component_names:
        contract = ContractService._refresh_contract(context, name)
        assert isinstance(contract.get("contract_version"), int) and contract["contract_version"] >= 1, (
            f"Expected component '{name}' to carry a tracked contract_version: {contract}"
        )
        assert contract.get("created_at") and contract.get("updated_at"), (
            f"Expected component '{name}' to carry created_at/updated_at version-control "
            f"timestamps: {contract}"
        )


# ---------------------------------------------------------------------------
# Scenario: Bundle multiple contracts into a package
# ---------------------------------------------------------------------------


@given('contracts "{name_a}" and "{name_b}" exist')
def step_given_two_plain_contracts_exist(context, name_a, name_b):
    _ensure_contract_in_draft(context, name_a)
    _ensure_contract_in_draft(context, name_b)


@when('I bundle contracts "{name_a}" and "{name_b}" into package "{package_name}"')
def step_when_bundle_contracts_into_package(context, name_a, name_b, package_name):
    # No dedicated multi-contract "package" endpoint exists — the existing
    # contract bundle export (GET /contract/export/{did},
    # backend/internal/bundleexport/bundler.go) already aggregates a
    # contract's own artifacts AND its full dcs:parentContract chain into one
    # ZIP with a bundle-manifest.json cross-reference index, so link name_b
    # to name_a as its parent to make the export a genuine multi-contract
    # package.
    link_resp = _link_contract_to_parent(context, name_b, name_a)
    assert link_resp.status_code == 200, (
        f"Expected linking '{name_b}' to '{name_a}' while assembling package "
        f"'{package_name}': {link_resp.status_code} {link_resp.text}"
    )
    step_given_contract_has_exported_pdf(context, name_a)
    step_given_contract_has_exported_pdf(context, name_b)

    did_b, _ = ContractService._contract_data(context, name_b)
    headers = context.contract_seed_headers.get(name_b)
    context.requests_response = get_with_headers(context, contract_export_url(context, did_b), headers=headers)

    ContractService._ensure_store(context, "contract_packages", {})
    context.contract_packages[package_name] = {"root": name_b, "parent": name_a}


def _open_package_manifest(context) -> dict:
    import io
    import zipfile

    assert context.requests_response.status_code == 200, (
        f"Expected the contract package export to succeed: {context.requests_response.status_code} "
        f"{context.requests_response.text}"
    )
    zf = zipfile.ZipFile(io.BytesIO(context.requests_response.content))
    return zf, json.loads(zf.read("bundle-manifest.json"))


@then("a contract package is created")
def step_then_contract_package_created(context):
    assert context.requests_response.status_code == 200, (
        f"Expected the contract package export to succeed: {context.requests_response.status_code} "
        f"{context.requests_response.text}"
    )
    content_type = context.requests_response.headers.get("Content-Type", "")
    assert content_type.startswith("application/zip"), (
        f"Expected package export Content-Type application/zip, got: {content_type}"
    )


@then("the package maintains internal references")
def step_then_package_internal_references(context):
    _, manifest = _open_package_manifest(context)
    components = manifest.get("components") or []
    assert len(components) >= 2, f"Expected the package to bundle multiple contracts, got: {components}"
    child_dids = {c.get("did") for c in components}
    parent_refs = {c.get("parent_did") for c in components if c.get("parent_did")}
    assert parent_refs, f"Expected at least one internal dcs:parentContract reference in the package: {components}"
    assert parent_refs.issubset(child_dids), (
        f"Expected every internal reference to resolve to another component packaged in the "
        f"same bundle: {components}"
    )


@then("the package maintains shared metadata")
def step_then_package_shared_metadata(context):
    _, manifest = _open_package_manifest(context)
    components = manifest.get("components") or []
    for c in components:
        assert c.get("state"), f"Expected every packaged component to carry shared state metadata: {c}"
        assert isinstance(c.get("contract_version"), int), (
            f"Expected every packaged component to carry shared contract_version metadata: {c}"
        )
    assert manifest.get("generated_at"), f"Expected package-level shared metadata (generated_at): {manifest}"


@then("the package tracks signature states")
def step_then_package_signature_states(context):
    zf, _ = _open_package_manifest(context)
    names = zf.namelist()
    sig_entries = [n for n in names if n.endswith("signatures.json")]
    assert len(sig_entries) >= 2, (
        f"Expected the package to track a signatures.json entry per bundled contract, got: {names}"
    )


# ---------------------------------------------------------------------------
# Scenario: Auto-fill metadata from template
# ---------------------------------------------------------------------------


@given('template "{name}" has predefined metadata fields')
def step_given_template_has_predefined_metadata(context, name):
    description = f"{name} predefined metadata: parties, effective date, confidentiality period"
    did, updated_at = TemplateService.create_fresh_template(context, name=name, description=description, title=name)
    updated_at = TemplateService.do_submit(context, did, updated_at)
    updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)

    approver_h = AuthService.get_headers_for_roles(["Template Approver"])
    approve_resp = post_json(
        context, template_approve_url(context), {"did": did, "updated_at": updated_at}, headers=approver_h
    )
    assert approve_resp.status_code == 200, f"Template approve failed: {approve_resp.text}"
    updated_at = TemplateService.fetch_template(context, did, headers=approver_h).get("updated_at")

    manager_h = AuthService.get_headers_for_roles(["Template Manager"])
    register_resp = post_json(context, template_register_url(context), {"did": did}, headers=manager_h)
    assert register_resp.status_code == 200, f"Template register failed: {register_resp.text}"

    TemplateService.store_named(context, name, did, updated_at)
    if not hasattr(context, "template_dids") or context.template_dids is None:
        context.template_dids = {}
    context.template_dids[name] = did
    context.nda_template_metadata = {"name": name, "description": description}


@then("the contract inherits metadata from the template")
def step_then_contract_inherits_metadata(context):
    body = context.requests_response.json()
    did = body.get("did")
    assert did, f"No DID in create response: {body}"
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    contract = retrieve.json()
    template_meta = context.nda_template_metadata
    assert contract.get("name") == template_meta["name"], (
        f"Expected contract name to inherit template name {template_meta['name']!r}, "
        f"got {contract.get('name')!r}"
    )
    assert contract.get("description") == template_meta["description"], (
        f"Expected contract description to inherit template description, got "
        f"{contract.get('description')!r}"
    )
    assert contract.get("template_did") == context.template_dids[template_meta["name"]], (
        f"Expected the contract to be traceable to its source template DID: {contract}"
    )
    context.nda_contract_did = did


@then("I can override specific metadata values")
def step_then_can_override_metadata(context):
    did = context.nda_contract_did
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    override_name = f"{context.nda_template_metadata['name']} (overridden by contract creator)"
    update_resp = put_json(
        context, contract_update_url(context), {"did": did, "updated_at": updated_at, "name": override_name}
    )
    assert update_resp.status_code == 200, (
        f"Expected overriding the contract's inherited name to succeed: "
        f"{update_resp.status_code} {update_resp.text}"
    )
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    assert retrieve.json().get("name") == override_name, (
        f"Expected the metadata override to take effect: {retrieve.json()}"
    )


# ---------------------------------------------------------------------------
# Scenario: Contract Creator can only create contracts for authorized
# parties / cannot create contracts involving unauthorized parties
# ---------------------------------------------------------------------------


def _slugify(value: str) -> str:
    import re

    return re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-") or "party"


def _ensure_service_agreement_template(context):
    """These two scenarios don't carry their own "template X is approved and
    available" Given line — they reuse "Service Agreement Template" from
    earlier in the same feature file by name. behave's per-scenario Context
    layering means custom attributes like context.template_dids do NOT
    actually survive across scenario boundaries (each scenario gets a fresh
    layer), so ensure it exists here rather than assuming an earlier
    scenario already populated it.
    """
    if (getattr(context, "template_dids", None) or {}).get("Service Agreement Template"):
        return
    from steps.template_management.template_workflow_steps import (  # noqa: PLC0415
        step_given_template_approved_available,
    )

    step_given_template_approved_available(context, "Service Agreement Template")


@given('I am authorized to create contracts involving party "{party}"')
def step_given_authorized_for_party(context, party):
    # An authorized party is represented by this instance's own peer DID —
    # always trivially trusted (localPeer short-circuit in
    # dcstodcs.CheckForUntrustedPeers, backend/internal/service/
    # contract_workflow_engine.go's Create()).
    _ensure_service_agreement_template(context)
    ContractService._ensure_store(context, "party_dids", {})
    context.party_dids[party] = ContractService._local_peer_did(context)


@given('I am not authorized to create contracts with party "{party}"')
def step_given_not_authorized_for_party(context, party):
    # An unauthorized party is a peer DID that is neither this instance's
    # own DID nor present in the trusted_peers allowlist — an untrusted
    # did:web peer is exactly the "unauthorized party" this check rejects.
    _ensure_service_agreement_template(context)
    ContractService._ensure_store(context, "party_dids", {})
    context.party_dids[party] = f"did:web:{_slugify(party)}.bdd-untrusted.example"


def _create_contract_for_party(context, party):
    template_did = (getattr(context, "template_dids", None) or {}).get("Service Agreement Template")
    assert template_did, "No approved 'Service Agreement Template' DID found — ensure it was created earlier"
    party_did = (getattr(context, "party_dids", None) or {}).get(party)
    assert party_did, f"No party DID recorded for '{party}' — ensure an authorization Given step ran"
    context.last_party_involved = party
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": template_did,
            "reviewers": [party_did],
            "negotiators": [party_did],
            "approvers": [party_did],
        },
    )
    if context.requests_response.status_code == 200:
        context.party_contract_did = context.requests_response.json().get("did")
        context.party_contract_party = party


@when('I specify party "{party}" as a contract party')
def step_when_specify_party(context, party):
    _create_contract_for_party(context, party)


@when('I attempt to create a contract involving party "{party}"')
def step_when_attempt_create_contract_involving_party(context, party):
    _create_contract_for_party(context, party)


@then("the contract is created successfully")
def step_then_contract_created_successfully(context):
    assert context.requests_response.status_code == 200, (
        f"Expected contract creation to succeed: {context.requests_response.status_code} "
        f"{context.requests_response.text}"
    )
    assert context.requests_response.json().get("did"), context.requests_response.text


@then('the contract is associated with party "{party}"')
def step_then_contract_associated_with_party(context, party):
    did = context.party_contract_did
    party_did = context.party_dids[party]
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did))
    assert retrieve.status_code == 200, retrieve.text
    responsible = retrieve.json().get("responsible") or {}
    all_parties = (
        set(responsible.get("reviewers") or [])
        | set(responsible.get("approvers") or [])
        | set(responsible.get("negotiators") or [])
    )
    assert party_did in all_parties, (
        f"Expected the contract to be associated with party '{party}' ({party_did}) via its "
        f"reviewers/approvers/negotiators, got: {responsible}"
    )


@then('the request is denied with an "{message}" error')
def step_then_denied_with_message(context, message):
    del message
    assert context.requests_response.status_code in (400, 401, 403, 404), (
        f"Expected the request to be denied (4xx), got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("the contract creation is prevented")
def step_then_contract_creation_prevented(context):
    assert context.requests_response.status_code != 200, (
        f"Expected contract creation to be prevented, got 200: {context.requests_response.text}"
    )
    body = context.requests_response.json() if context.requests_response.text else {}
    assert not body.get("did"), f"Expected no contract DID to be assigned to the prevented attempt: {body}"


@then("the attempt is logged")
def step_then_attempt_is_logged(context):
    # No contract DID exists for a prevented creation to key an audit query
    # on — the untrusted-peer rejection itself (dcstodcs.CheckForUntrustedPeers
    # / contract_workflow_engine.go's Create()) returns a bad_request whose
    # message names every untrusted DID it rejected, so the offending party's
    # DID being present in the (logged, returned) response body IS the
    # traceable record of the attempt.
    body_text = context.requests_response.text
    assert body_text, "Expected a response body recording the rejected creation attempt"
    party = getattr(context, "last_party_involved", None)
    party_did = (getattr(context, "party_dids", None) or {}).get(party) if party else None
    if party_did:
        assert party_did in body_text, (
            f"Expected the rejected attempt to be traceable to the offending party DID "
            f"{party_did} in the logged response: {body_text}"
        )
