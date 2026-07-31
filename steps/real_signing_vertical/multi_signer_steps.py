"""BDD steps for the multi-signer signing workflow (DCS-FR-SM-07/-17,
UC-03-06, features/22_real_signing_vertical/multi_signer.feature): contracts
declaring multiple dcs:SignatureField nodes need one ceremony + one PAdES
signature per field, sequentially, with every ceremony completed before the
first signature and the deploy gate holding activation until all fields are
signed."""

import requests as _requests
from behave import given, then, when

from steps.support.api_client import (
    contract_deploy_url,
    contract_retrieve_by_id_url,
    contract_update_url,
    get_with_headers,
    post_json,
    put_json,
    signature_view_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (
    _apply_signature,
    _run_full_ceremony,
)


@given('contract "{name}" is a fresh draft declaring signature fields "{field_one}" and "{field_two}"')
def step_given_dual_field_draft(context, name, field_one, field_two):
    ContractService._create_contract_in_draft(context, name)
    did, updated_at = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    doc = retrieve.json().get("contract_data") or {}
    # Declare the fields under the canonical dcs: term the DCS pipeline reads
    # (seedSignatureFields / pdf-core), so these explicit per-signatory fields
    # are the ones rendered into the AcroForm (/T == dcs:signatoryName).
    doc["dcs:signatureFields"] = [
        {"@type": "dcs:SignatureField", "@id": f"{did}#{field}", "dcs:signatoryName": field}
        for field in (field_one, field_two)
    ]
    resp = put_json(
        context,
        contract_update_url(context),
        {"did": did, "updated_at": updated_at, "contract_data": doc},
        headers=headers,
    )
    assert resp.status_code == 200, (
        f"could not seed signature fields on '{name}': {resp.status_code} {resp.text}"
    )
    ContractService._refresh_contract(context, name)


@given('a completed signing ceremony exists for field "{field}" of contract "{name}"')
def step_given_field_ceremony(context, name, field):
    # Each field's ceremony presents with a FRESH ephemeral holder key, so
    # the two fields are signed by two distinct natural-person identities —
    # the shared test wallet key would collapse both into one subject DID.
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.keys import generate_ec_private_jwk  # noqa: PLC0415

    ceremony_id, _presentation, subject_did = _run_full_ceremony(
        context, name, field, field, holder_private=generate_ec_private_jwk()
    )
    signers = getattr(context, "multi_signer_dids", None)
    if signers is None:
        signers = {}
        context.multi_signer_dids = signers
    # _run_full_ceremony also stashes ceremony_id/presentation on context, keyed
    # only by contract name — the SECOND field's ceremony here overwrites the
    # first's before either field signs. Keep our own per-field copy so the
    # apply step below pins the right ceremony (and signatory name) per field.
    signers.setdefault(name, {})[field] = {
        "subject_did": subject_did,
        "ceremony_id": ceremony_id,
        "signatory": field,
    }


@when('the signer of field "{field}" applies their signature to contract "{name}"')
def step_when_field_signer_applies(context, name, field):
    entry = context.multi_signer_dids[name][field]
    context.requests_response = _apply_signature(
        context,
        name,
        signer_did=entry["subject_did"],
        field_name=field,
        ceremony_id=entry["ceremony_id"],
        signatory=entry["signatory"],
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@then("the signature apply is rejected because not all declared fields have a completed ceremony")
def step_then_apply_rejected_ceremonies_incomplete(context):
    resp = context.requests_response
    assert resp.status_code == 422, (
        f"Expected the all-ceremonies-before-first-signature gate to reject with 422, got "
        f"{resp.status_code}: {resp.text}"
    )
    assert "ceremon" in resp.text.lower(), f"Expected the rejection to name the ceremony gate: {resp.text}"


@then("the signature apply is rejected because the field is already signed")
def step_then_apply_rejected_field_signed(context):
    resp = context.requests_response
    assert resp.status_code == 400, (
        f"Expected re-signing an already-signed field to be rejected with 400, got "
        f"{resp.status_code}: {resp.text}"
    )
    assert "already signed" in resp.text.lower(), (
        f"Expected the rejection to name the already-signed field: {resp.text}"
    )


@then('a manual deployment of contract "{name}" is rejected because signing is incomplete')
def step_then_deploy_rejected_incomplete(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(
        context,
        contract_deploy_url(context),
        {"did": did, "updated_at": updated_at},
        headers=manager_h,
    )
    assert resp.status_code == 400, (
        f"Expected the deploy gate to reject a partially signed multi-signer contract with 400, "
        f"got {resp.status_code}: {resp.text}"
    )
    assert "incomplete" in resp.text.lower(), (
        f"Expected the rejection to name the incomplete signing workflow: {resp.text}"
    )


@then('the signature view for contract "{name}" shows two "{status}" signatures covering fields "{field_one}" and "{field_two}"')
def step_then_view_two_signatures(context, name, status, field_one, field_two):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = _requests.get(
        signature_view_url(context),
        params={"did": did},
        headers=manager_h,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"signature view failed: {resp.status_code} {resp.text}"
    signatures = resp.json().get("signatures") or []
    assert len(signatures) == 2, f"Expected exactly two signatures, got: {signatures}"
    by_field = {s.get("field_name"): s for s in signatures}
    for field in (field_one, field_two):
        sig = by_field.get(field)
        assert sig, f"Expected a signature covering field {field!r}, got fields: {list(by_field)}"
        assert sig.get("status") == status, f"Expected {field!r} to be {status!r}, got: {sig.get('status')!r}"
        assert sig.get("signer_did"), f"Expected an independent signer identity on {field!r}: {sig}"
    assert signatures[0].get("signer_did") != signatures[1].get("signer_did"), (
        "Expected the two signatures to be bound to two DISTINCT signer identities"
    )
