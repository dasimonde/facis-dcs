"""Black-box bindings for immutable, server-controlled Semantic Hub pins."""

from __future__ import annotations

import copy
import re
from urllib.parse import parse_qs, urlparse

from behave import given, then, when

from steps.semantic_hub.workflow_gate_steps import _request_gate
from steps.support.api_client import (
    contract_retrieve_by_id_url,
    contract_update_url,
    get_with_headers,
    put_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


PIN_FIELDS = (
    "@context",
    "sh:shapesGraph",
    "dcs:effectiveShapes",
    "dcterms:conformsTo",
)


def _contract_document(context, name: str) -> dict:
    did, _ = ContractService._contract_data(context, name)
    response = get_with_headers(
        context,
        contract_retrieve_by_id_url(context, did),
        headers=context.contract_seed_headers[name],
    )
    assert response.status_code == 200, response.text
    document = response.json().get("contract_data")
    assert isinstance(document, dict), response.text
    return document


def _pin_snapshot(document: dict) -> dict:
    snapshot = {field: copy.deepcopy(document.get(field)) for field in PIN_FIELDS}
    missing = [field for field, value in snapshot.items() if value is None]
    assert not missing, (
        f"Freshly produced contract has an incomplete semantic bundle; "
        f"missing {missing}: {document}"
    )
    effective = snapshot["dcs:effectiveShapes"]
    assert isinstance(effective, list) and effective, (
        f"Freshly produced contract has no effective shapes: {document}"
    )
    return snapshot


def _set_business_title(document: dict, title: str) -> None:
    metadata = document.get("dcs:metadata") or document.get("metadata")
    assert isinstance(metadata, dict), f"Contract has no metadata object: {document}"
    metadata["dcs:title"] = title


def _replace_version(value):
    if isinstance(value, str):
        changed = re.sub(r"([?&]version=)\d+", r"\g<1>999999", value)
        return changed if changed != value else value
    if isinstance(value, list):
        return [_replace_version(item) for item in value]
    if isinstance(value, dict):
        return {key: _replace_version(item) for key, item in value.items()}
    return value


def _client_document(document: dict, title: str, client_pins: str) -> dict:
    submitted = copy.deepcopy(document)
    _set_business_title(submitted, title)
    if client_pins in {"absent", "removed"}:
        for field in PIN_FIELDS:
            submitted.pop(field, None)
    elif client_pins == "manipulated":
        for field in PIN_FIELDS:
            submitted[field] = _replace_version(submitted[field])
        assert _pin_snapshot(submitted) != _pin_snapshot(document), (
            "The malicious fixture did not change the semantic bundle"
        )
    else:
        raise AssertionError(f"Unknown client pin fixture {client_pins!r}")
    return submitted


def _anchor(value) -> str:
    if isinstance(value, dict):
        value = value.get("@id")
    assert isinstance(value, str) and value, f"Invalid semantic anchor: {value!r}"
    return value


def _profile_version(value) -> int:
    versions = parse_qs(urlparse(_anchor(value)).query).get("version") or []
    assert len(versions) == 1 and versions[0].isdigit(), (
        f"Profile pin has no exact version: {value!r}"
    )
    return int(versions[0])


@given('the server-controlled semantic bundle of contract "{name}" is remembered')
def step_remember_server_bundle(context, name):
    context.remembered_semantic_bundles = getattr(
        context, "remembered_semantic_bundles", {}
    )
    context.remembered_semantic_bundles[name] = _pin_snapshot(
        _contract_document(context, name)
    )


@when(
    'contract "{name}" is updated with business title "{title}" and no semantic pin fields'
)
def step_update_without_pins(context, name, title):
    _update_with_client_pins(context, name, title, "absent")


@when(
    'contract "{name}" is updated with business title "{title}" and "{client_pins}" semantic pin fields'
)
def step_update_with_pins(context, name, title, client_pins):
    _update_with_client_pins(context, name, title, client_pins)


def _update_with_client_pins(context, name: str, title: str, client_pins: str) -> None:
    did, updated_at = ContractService._contract_data(context, name)
    document = _client_document(
        _contract_document(context, name),
        title,
        client_pins,
    )
    context.requests_response = put_json(
        context,
        contract_update_url(context),
        {"did": did, "updated_at": updated_at, "contract_data": document},
        headers=context.contract_seed_headers[name],
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@then('the draft update for contract "{name}" succeeds')
def step_update_succeeds(context, name):
    assert context.requests_response.status_code == 200, (
        f"Draft update for {name!r} failed: "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )


@then('contract "{name}" stores business title "{title}"')
def step_business_title_stored(context, name, title):
    document = _contract_document(context, name)
    metadata = document.get("dcs:metadata") or document.get("metadata")
    assert isinstance(metadata, dict), document
    assert metadata.get("dcs:title") == title, document


@then(
    'contract "{name}" still has the remembered server-controlled semantic bundle'
)
def step_bundle_unchanged(context, name):
    actual = _pin_snapshot(_contract_document(context, name))
    expected = context.remembered_semantic_bundles[name]
    assert actual == expected, (
        f"Client-controlled content changed the immutable semantic bundle: "
        f"expected={expected}, actual={actual}"
    )


@when(
    'the submission gate for contract "{name}" includes business title "{title}" and "{client_pins}" semantic pin fields'
)
def step_submit_with_contract_data(context, name, title, client_pins):
    did, updated_at = ContractService._contract_data(context, name)
    document = _client_document(
        _contract_document(context, name),
        title,
        client_pins,
    )
    payload = ContractService._contract_submit_payload(context, did, updated_at)
    payload["contract_data"] = document
    original_payload_builder = ContractService._contract_submit_payload

    def payload_with_contract_data(_context, payload_did, payload_updated_at):
        assert payload_did == did and payload_updated_at == updated_at
        return payload

    ContractService._contract_submit_payload = staticmethod(payload_with_contract_data)
    try:
        _request_gate(context, "submission", name)
    finally:
        ContractService._contract_submit_payload = original_payload_builder


@then(
    "that workflow-gate request used the remembered server-controlled semantic bundle"
)
def step_gate_used_server_bundle(context):
    payload = context.last_gate_observation["payload"]
    snapshot = payload.get("snapshot") or {}
    name = next(
        contract_name
        for contract_name, did in context.contract_dids.items()
        if did == snapshot.get("contract_did")
        and contract_name in context.remembered_semantic_bundles
    )
    remembered = context.remembered_semantic_bundles[name]
    expected_shapes = [_anchor(value) for value in remembered["dcs:effectiveShapes"]]
    assert snapshot.get("effective_shapes") == expected_shapes, (
        f"Workflow gate did not use the stored effective shapes: "
        f"expected={expected_shapes}, snapshot={snapshot}"
    )
    expected_profile = _profile_version(remembered["dcterms:conformsTo"])
    assert snapshot.get("profile_version") == expected_profile, (
        f"Workflow gate did not use the stored profile version: "
        f"expected={expected_profile}, snapshot={snapshot}"
    )
