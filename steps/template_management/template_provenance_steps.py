"""BDD steps for DCS-FR-TR-09 per-version template provenance credentials
(GET /template/provenance/{did}, issued at registration by
backend/internal/templaterepository/command/provenance.go)."""

import re

import requests as _requests
from behave import then, when

from steps.support.api_client import did_document_url, template_provenance_url
from steps.support.services.auth_service import AuthService
from steps.support.services.template_service import TemplateService


@when('I retrieve the provenance credentials of template "{name}"')
def step_when_retrieve_provenance(context, name):
    t = TemplateService.named(context, name)
    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = _requests.get(
        template_provenance_url(context, t["did"]),
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when('I attempt to retrieve the provenance credentials of template "{name}"')
def step_when_attempt_retrieve_provenance(context, name):
    t = TemplateService.named(context, name)
    headers = getattr(context, "headers", {})
    context.requests_response = _requests.get(
        template_provenance_url(context, t["did"]),
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


def _single_credential_entry(context):
    body = context.requests_response.json()
    assert isinstance(body, list) and len(body) == 1, (
        f"Expected exactly one provenance credential entry, got: {body}"
    )
    return body[0]


@then("exactly one provenance credential is issued, sealing version 1 with no predecessor link")
def step_then_single_v1_credential(context):
    entry = _single_credential_entry(context)
    assert entry.get("version") == 1, f"Expected version 1, got: {entry.get('version')}"
    assert entry.get("vc_id", "").startswith("urn:dcs:vc:template-provenance:"), (
        f"Unexpected vc_id scheme: {entry.get('vc_id')!r}"
    )
    assert entry.get("previous_vc_id") in (None, ""), (
        f"The first version must not link a predecessor, got: {entry.get('previous_vc_id')!r}"
    )


@then('the provenance credential is a W3C VerifiableCredential in JSON-LD of type "{vc_type}"')
def step_then_credential_type(context, vc_type):
    credential = _single_credential_entry(context).get("credential") or {}
    types = credential.get("type") or []
    assert "VerifiableCredential" in types and vc_type in types, (
        f"Expected type to include VerifiableCredential and {vc_type}, got: {types}"
    )
    ctx_entries = credential.get("@context") or []
    assert "https://www.w3.org/ns/credentials/v2" in ctx_entries, (
        f"Expected the W3C credentials v2 JSON-LD context, got: {ctx_entries}"
    )
    assert credential.get("id") == _single_credential_entry(context).get("vc_id"), (
        "Credential id and stored vc_id must match"
    )


@then("the provenance credential names the template's creator, reviewer, approver, and registrar")
def step_then_credential_actors(context):
    subject = (_single_credential_entry(context).get("credential") or {}).get("credentialSubject") or {}
    for field in ("created_by", "registered_by"):
        assert subject.get(field), f"Expected credentialSubject.{field} to be set, got: {subject}"
    for field in ("reviewed_by", "approved_by"):
        actors = subject.get(field)
        assert isinstance(actors, list) and actors and all(actors), (
            f"Expected credentialSubject.{field} to name at least one actor, got: {subject}"
        )
    assert subject.get("registrar_holder_did", "").startswith("did:"), (
        f"Expected the registrar's holder DID, got: {subject.get('registrar_holder_did')!r}"
    )


@then("the provenance credential binds the registered template content by its hash")
def step_then_credential_content_hash(context):
    subject = (_single_credential_entry(context).get("credential") or {}).get("credentialSubject") or {}
    template_hash = subject.get("template_hash") or ""
    assert re.fullmatch(r"[0-9a-f]{64}", template_hash), (
        f"Expected a sha256 hex template_hash, got: {template_hash!r}"
    )
    assert subject.get("template_did"), f"Expected template_did in the subject, got: {subject}"


@then("the provenance credential carries a DataIntegrityProof issued by this instance")
def step_then_credential_proof(context):
    credential = _single_credential_entry(context).get("credential") or {}
    own_did = _requests.get(
        did_document_url(context.base_url), timeout=context.http_timeout_seconds
    ).json().get("id")
    assert credential.get("issuer") == own_did, (
        f"Expected issuer {own_did}, got: {credential.get('issuer')!r}"
    )
    proof = credential.get("proof") or {}
    if isinstance(proof, list):
        proof = proof[0] if proof else {}
    assert proof.get("type") == "DataIntegrityProof", f"Expected DataIntegrityProof, got: {proof}"
    assert proof.get("cryptosuite") == "ecdsa-rdfc-2019", f"Expected ecdsa-rdfc-2019, got: {proof}"
    assert str(proof.get("proofValue", "")).startswith("z"), (
        f"Expected a multibase base58btc proofValue, got: {proof.get('proofValue')!r}"
    )
