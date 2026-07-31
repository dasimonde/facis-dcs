"""BDD steps for the contract/template bundle export (ZIP) —
GET /contract/export/{did} and GET /template/export/{did}
(FR-TR-24, FR-CWE-30, FR-PACM-06).

The hierarchy-invariant and parent_did-full-scope-query steps (including
two-instance sibling isolation) live in
steps/template_management/dcs_contract_hierarchy_steps.py — this module
imports that file's fixture helpers to build parent/sibling hierarchies for
the bundle scenarios below, rather than duplicating them.
"""

import hashlib
import io
import json
import zipfile

from behave import given, then, when

from steps.support.api_client import (
    contract_export_url,
    get_with_headers,
    post_json,
    template_export_url,
)
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService
from steps.template_management.dcs_contract_hierarchy_steps import (
    _ensure_contract_in_draft,
    _link_contract_to_parent,
)


# ---------------------------------------------------------------------------
# Given — fixtures
# ---------------------------------------------------------------------------


@given('contract "{name}" exists with no exported PDF')
def step_given_contract_no_exported_pdf(context, name):
    ContractService._create_contract_in_draft(context, name)


@given('an approved template "{name}" is available for bundle export')
def step_given_template_for_bundle_export(context, name):
    # Canonical dcs:documentStructure envelope via the shared fixture source
    # (TemplateService.canonical_document_data) — NormalizeTemplateData
    # rejects the flat {"title", "clauses"} shape.
    from steps.support.api_client import template_create_url, template_approve_url
    from steps.support.services.auth_service import AuthService

    creator_headers = AuthService.get_headers_for_roles(["Template Creator"])
    create_resp = post_json(
        context,
        template_create_url(context),
        {
            "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
            "name": name,
            "description": "BDD template for bundle export",
            "template_data": TemplateService.canonical_document_data(name, clause_text="Base clause"),
        },
        headers=creator_headers,
    )
    assert create_resp.status_code == 200, f"Template create failed: {create_resp.text}"
    did = create_resp.json().get("did")
    body = TemplateService.fetch_template(context, did, headers=creator_headers)
    updated_at = body.get("updated_at")

    updated_at = TemplateService.do_submit(context, did, updated_at)
    updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)
    approver_headers = AuthService.get_headers_for_roles(["Template Approver"])
    approve_resp = post_json(
        context,
        template_approve_url(context),
        {"did": did, "updated_at": updated_at},
        headers=approver_headers,
    )
    assert approve_resp.status_code == 200, f"Template approve failed: {approve_resp.text}"
    updated_at = TemplateService.fetch_template(context, did, headers=approver_headers).get("updated_at")
    TemplateService.store_named(context, name, did, updated_at)


@given(
    'contract "{child_name}" and contract "{sibling_name}" both reference contract '
    '"{parent_name}" as their parent'
)
def step_given_hierarchy_with_sibling(context, child_name, sibling_name, parent_name):
    _ensure_contract_in_draft(context, parent_name)
    for name in (child_name, sibling_name):
        _ensure_contract_in_draft(context, name)
        resp = _link_contract_to_parent(context, name, parent_name)
        assert resp.status_code == 200, (
            f"Expected linking '{name}' to parent '{parent_name}' via a single valid "
            f"dcs:parentContract reference to succeed: {resp.status_code} {resp.text}"
        )


# ---------------------------------------------------------------------------
# When
# ---------------------------------------------------------------------------


@when('I request the contract bundle export for "{name}"')
def step_when_request_contract_bundle(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers.get(name) if hasattr(context, "contract_seed_headers") else None
    headers = headers or getattr(context, "headers", {})
    context.requests_response = get_with_headers(context, contract_export_url(context, did), headers=headers)
    context.bundle_export_contract_name = name


@when('I request the contract bundle export for "{name}" with an unauthorized role')
def step_when_request_contract_bundle_unauthorized(context, name):
    did, _ = ContractService._contract_data(context, name)
    unauth_headers = {"Authorization": "Bearer invalid-token", "Content-Type": "application/json"}
    context.requests_response = get_with_headers(context, contract_export_url(context, did), headers=unauth_headers)


@when('I request the template bundle export for "{name}"')
def step_when_request_template_bundle(context, name):
    from steps.support.services.auth_service import AuthService

    t = TemplateService.named(context, name)
    assert t.get("did"), f"No template DID recorded for '{name}' — ensure the Given step ran"
    # export_template_bundle's Security scopes are Template Manager/Reviewer/
    # Creator/Approver, not Contract Manager (the Background's role) — use a
    # role the endpoint actually authorizes.
    headers = AuthService.get_headers_for_roles(["Template Creator"])
    context.requests_response = get_with_headers(context, template_export_url(context, t["did"]), headers=headers)


# ---------------------------------------------------------------------------
# Then — ZIP shape
# ---------------------------------------------------------------------------


def _open_bundle_zip(context) -> zipfile.ZipFile:
    assert context.requests_response.status_code == 200, (
        f"Expected the bundle export to succeed, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    return zipfile.ZipFile(io.BytesIO(context.requests_response.content))


@then('the contract bundle ZIP for "{name}" contains entries: {entries}')
def step_then_contract_bundle_contains_entries(context, name, entries):
    zf = _open_bundle_zip(context)
    names = zf.namelist()
    expected = [e.strip() for e in entries.split(",") if e.strip()]
    missing = [e for e in expected if not any(n == e or n.startswith(e) for n in names)]
    assert not missing, (
        f"Contract bundle ZIP for '{name}' is missing entries {missing}; actual entries: {names}"
    )


@then('the template bundle ZIP for "{name}" contains entries: {entries}')
def step_then_template_bundle_contains_entries(context, name, entries):
    zf = _open_bundle_zip(context)
    names = zf.namelist()
    expected = [e.strip() for e in entries.split(",") if e.strip()]
    missing = [e for e in expected if not any(n == e or n.startswith(e) for n in names)]
    assert not missing, (
        f"Template bundle ZIP for '{name}' is missing entries {missing}; actual entries: {names}"
    )


@then('the template bundle ZIP for "{name}" contains no frame/parent chain directory')
def step_then_template_bundle_no_parent_chain(context, name):
    zf = _open_bundle_zip(context)
    names = zf.namelist()
    parent_chain_entries = [n for n in names if n.startswith("parents/")]
    assert not parent_chain_entries, (
        f"Template bundle ZIP for '{name}' unexpectedly contains a frame/parent chain "
        f"directory (FR-TR-09 template bundles are flat artifacts only, per "
        f"contracttemplatetype.go's CONTRACT_TEMPLATE/COMPONENT-only type set — no frame "
        f"types exist at template level): {parent_chain_entries}"
    )


# ---------------------------------------------------------------------------
# Then — parent chain present (related/ family assertions live in
# steps/contract_bundle_export_steps.py)
# ---------------------------------------------------------------------------


@then('the contract bundle ZIP for "{name}" contains the parent chain for "{parent_name}"')
def step_then_bundle_contains_parent_chain(context, name, parent_name):
    parent_did, _ = ContractService._contract_data(context, parent_name)
    zf = _open_bundle_zip(context)
    names = zf.namelist()
    expected_prefix = f"parents/{parent_did}/"
    matches = [n for n in names if n.startswith(expected_prefix)]
    assert matches, (
        f"Expected the contract bundle ZIP for '{name}' to contain the parent chain for "
        f"'{parent_name}' under '{expected_prefix}', got entries: {names}"
    )


# ---------------------------------------------------------------------------
# Then — bundle-manifest.json SHA-256 integrity
# ---------------------------------------------------------------------------


@then('every entry in the bundle-manifest.json for "{name}" has a SHA-256 matching the packaged bytes')
def step_then_manifest_hashes_match(context, name):
    zf = _open_bundle_zip(context)
    assert "bundle-manifest.json" in zf.namelist(), (
        f"Expected a 'bundle-manifest.json' index entry in the bundle ZIP for '{name}', "
        f"got entries: {zf.namelist()}"
    )
    manifest = json.loads(zf.read("bundle-manifest.json"))
    entries = manifest.get("entries")
    assert isinstance(entries, list) and entries, (
        f"Expected a non-empty 'entries' list in bundle-manifest.json for '{name}': {manifest}"
    )
    mismatches = []
    for entry in entries:
        path = entry.get("path") or entry.get("name")
        expected_sha256 = entry.get("sha256")
        assert path, f"bundle-manifest.json entry missing 'path'/'name' for '{name}': {entry}"
        assert expected_sha256, (
            f"bundle-manifest.json entry '{path}' missing 'sha256' for '{name}': {entry}"
        )
        actual_bytes = zf.read(path)
        actual_sha256 = hashlib.sha256(actual_bytes).hexdigest()
        if actual_sha256 != expected_sha256:
            mismatches.append((path, expected_sha256, actual_sha256))
    assert not mismatches, (
        f"SHA-256 mismatch between bundle-manifest.json and packaged ZIP bytes for '{name}': "
        f"{mismatches}"
    )


# ---------------------------------------------------------------------------
# Then — refusal with findings when a referenced component is missing
# ---------------------------------------------------------------------------


@then('the contract bundle export for "{name}" is refused with a findings list')
def step_then_export_refused_with_findings(context, name):
    resp = context.requests_response
    assert resp.status_code in (400, 404, 409, 422), (
        f"Expected the export to be refused for contract '{name}' with a missing referenced "
        f"component (no exported PDF, per FR-TR-26/FR-PACM-06), got {resp.status_code}: "
        f"{resp.text}"
    )
    body = resp.json()
    assert "findings" in body and isinstance(body["findings"], list) and body["findings"], (
        f"Expected a non-empty 'findings' list explaining the export refusal for contract "
        f"'{name}': {body}"
    )


# ---------------------------------------------------------------------------
# Then — RBAC + audit (RBAC assertion reuses core's generic
# "the request is denied with an authorization error"; audit reuses
# template_management/contract_state_machine_steps.py's generic
# 'the contract "{name}" has an audit event of type "{event_type}"')
# ---------------------------------------------------------------------------
