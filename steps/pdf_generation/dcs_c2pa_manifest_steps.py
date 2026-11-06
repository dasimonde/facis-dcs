"""BDD step definitions for C2PA conformance
(features/19_c2pa_conformance; SRS DCS-OR-C2PA-006/-008).

Covers:
  - the public, unauthenticated GET /c2pa/manifest/{did} endpoint
    (backend/design/c2pa.go; raw manifest store bytes by default,
    ?history=true for a parsed chain enumeration);
  - the embedded manifest's `dcs.remote_manifests` assertion referencing
    that endpoint;
  - the verification of a copy whose embedded manifest is removed, from the
    remote manifest it references (POST /pdf/verify/contract/{did});
  - the verify response's four independently named checks.

The lifecycle-banner scenarios and the "has reached contract state"/
"is exported and verified as PDF" setup steps are deliberately NOT
redefined here — they already exist in
steps/template_management/contract_state_machine_steps.py and are reused
as-is (see that module's `_reach_state` helper and its
`the C2PA lifecycle_status for contract "{name}" is "{status}"` step).
"""

import base64

import requests as _requests

from behave import given, then, when

from steps.support.api_client import c2pa_manifest_url, contract_verify_pdf_url
from steps.support.services.contract_service import ContractService


# ---------------------------------------------------------------------------
# When — public manifest requests (deliberately sent WITHOUT any
# Authorization header, proving the "no JWT/auth" requirement honestly —
# not just reusing context.headers with an empty override).
# ---------------------------------------------------------------------------


@when('I request the public C2PA manifest for contract "{name}"')
def step_when_request_public_manifest(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = _requests.get(
        c2pa_manifest_url(context, did),
        timeout=context.http_timeout_seconds,
    )


@when('I request the C2PA manifest history for contract "{name}"')
def step_when_request_manifest_history(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = _requests.get(
        c2pa_manifest_url(context, did),
        params={"history": "true"},
        timeout=context.http_timeout_seconds,
    )


# ---------------------------------------------------------------------------
# Then — raw manifest store response shape
# ---------------------------------------------------------------------------


@then('the response has Content-Type "{content_type}"')
def step_then_response_content_type(context, content_type):
    actual = context.requests_response.headers.get("Content-Type", "")
    actual_media_type = actual.split(";")[0].strip()
    assert actual_media_type == content_type, (
        f"Expected Content-Type '{content_type}', got '{actual}'"
    )


@then("the response body is a non-empty C2PA JUMBF manifest store")
def step_then_response_is_jumbf_manifest_store(context):
    body = context.requests_response.content
    assert body, (
        f"Response body is empty (status {context.requests_response.status_code})"
    )
    # 'jumb' is the JUMBF superbox type marker pdf-core's own BDD suite uses
    # to recognize a manifest store (pdf-core/features/steps/
    # dcs_pdf_core_steps.py: "the manifest store response contains the
    # JUMBF marker").
    assert b"jumb" in body, (
        "Response body does not contain the JUMBF superbox marker 'jumb' — "
        f"not a valid C2PA manifest store: {body[:200]!r}"
    )


# ---------------------------------------------------------------------------
# Then — parsed history enumeration response shape
# ---------------------------------------------------------------------------


@then("the response is a JSON list of manifest labels with dcs.lifecycle assertions")
def step_then_history_response_shape(context):
    body = context.requests_response.json()
    assert isinstance(body, list) and body, (
        f"Expected a non-empty JSON list of manifest chain entries, got: {body}"
    )
    for entry in body:
        assert isinstance(entry, dict) and entry.get("label"), (
            f"Each manifest history entry must carry a non-empty 'label' field: {entry}"
        )
    lifecycle_entries = [e for e in body if e.get("lifecycle") or e.get("dcs.lifecycle")]
    assert lifecycle_entries, (
        "Expected at least one manifest history entry to carry a dcs.lifecycle "
        f"assertion (key 'lifecycle' or 'dcs.lifecycle'), got: {body}"
    )


@then('the C2PA manifest history of contract "{name}" ends with lifecycle status "{status}"')
def step_then_history_ends_with_lifecycle_status(context, name, status):
    did, _ = ContractService._contract_data(context, name)
    resp = _requests.get(
        c2pa_manifest_url(context, did),
        params={"history": "true"},
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"manifest history of '{name}' failed: {resp.status_code} {resp.text}"
    statuses = [
        (entry.get("lifecycle") or entry.get("dcs.lifecycle") or {}).get("status")
        for entry in resp.json()
    ]
    statuses = [s for s in statuses if s]
    assert statuses and statuses[-1] == status, (
        f"Expected the provenance chain of '{name}' to end with lifecycle status {status!r}, got {statuses}"
    )


# ---------------------------------------------------------------------------
# Then — dcs.remote_manifests assertion
# ---------------------------------------------------------------------------


@then(
    'the C2PA manifest response for contract "{name}" declares a remote_manifests field '
    "pointing to its own public manifest endpoint"
)
def step_then_manifest_declares_remote_manifests(context, name):
    did, _ = ContractService._contract_data(context, name)
    manifest_bytes = context.requests_response.content
    assert context.requests_response.status_code == 200, (
        f"C2PA manifest request failed for contract '{name}': "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )
    # The remote-manifest URL is the "dcs.remote_manifests" assertion of the
    # manifest (pdf-core/compiler/compiler_c2pa.go), not a claim field:
    # c2pa-rs rejects an unknown "remote_manifests" field in a V2 claim. The
    # XMP dcterms:provenance property carries the same URL.
    assert b"remote_manifests" in manifest_bytes, (
        "C2PA manifest store does not declare a 'remote_manifests' field at all: "
        f"{manifest_bytes[:300]!r}"
    )
    expected_path = f"/c2pa/manifest/{did}".encode()
    assert expected_path in manifest_bytes, (
        "remote_manifests field does not reference this contract's own public "
        f"manifest endpoint path '{expected_path.decode()}' — manifest store: "
        f"{manifest_bytes[:300]!r}"
    )


# ---------------------------------------------------------------------------
# Then — four independently named verify checks
# ---------------------------------------------------------------------------


@then(
    'the verify response for contract "{name}" includes four named checks: '
    "PDF signature, C2PA manifest, VC signature, and status list"
)
def step_then_verify_four_named_checks(context, name):
    assert context.requests_response.status_code == 200, (
        f"Verify failed for contract '{name}': {context.requests_response.status_code} "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    # c2pa_manifest_found / c2pa_signature_status / vc_proof_status /
    # status_list_status / pdf_signature_status live on PDFVerifyResult
    # (backend/design/pdf_generation.go).
    required_fields = {
        "pdf_signature_status": "PDF signature",
        "c2pa_manifest_found": "C2PA manifest",
        "vc_proof_status": "VC signature",
        "status_list_status": "status list",
    }
    missing = [
        f"{field} ({label})"
        for field, label in required_fields.items()
        if field not in body
    ]
    assert not missing, (
        f"Verify response for contract '{name}' is missing named check field(s): "
        f"{', '.join(missing)} — response: {body}"
    )
    # The two signature checks are three-state, never a bare boolean: a check
    # that was not performed must be distinguishable from one that passed.
    for field in ("c2pa_signature_status", "vc_proof_status"):
        assert body.get(field) in ("valid", "invalid", "indeterminate", "not_available"), (
            f"Expected {field} for contract '{name}' to report one of "
            f"valid/invalid/indeterminate/not_available, got {body.get(field)!r} "
            f"in response {body}"
        )


@then('the PDF signature check for contract "{name}" is reported valid by the EU DSS')
def step_then_pdf_signature_valid_by_dss(context, name):
    body = context.requests_response.json()
    assert body.get("pdf_signature_source") == "eu-dss", (
        f"Expected the PDF signature of contract '{name}' to be checked by the EU DSS, got: {body}"
    )
    assert body.get("pdf_signature_status") == "valid", (
        f"Expected the EU DSS to validate every PAdES signature of contract '{name}', "
        f"got: {body.get('pdf_signature_status')!r} ({body.get('pdf_signature_detail')!r}) in response {body}"
    )


# ---------------------------------------------------------------------------
# Strip-then-verify (DCS-OR-C2PA-008)
# ---------------------------------------------------------------------------

_C2PA_ATTACHMENT_NAME = b"(content_credential.c2pa)"


@given('a local copy of contract "{name}"\'s PDF with the content_credential.c2pa attachment removed')
def step_given_stripped_local_copy(context, name):
    pdf = context.pdf_bytes[name]
    # Every name of the attachment is overwritten with a name of the same
    # length: no reader finds the embedded manifest any more, and the xref
    # offsets stay valid. The XMP dcterms:provenance reference stays.
    stripped = pdf.replace(_C2PA_ATTACHMENT_NAME, b"(" + b"x" * (len(_C2PA_ATTACHMENT_NAME) - 2) + b")")
    assert stripped != pdf, f"the exported PDF of contract '{name}' has no content_credential.c2pa attachment"
    if not hasattr(context, "stripped_pdf_bytes"):
        context.stripped_pdf_bytes = {}
    context.stripped_pdf_bytes[name] = stripped


@when('the local copy of contract "{name}" is verified via the backend verify endpoint')
def step_when_verify_local_copy(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = _requests.post(
        contract_verify_pdf_url(context, did),
        json={"pdf": base64.b64encode(context.stripped_pdf_bytes[name]).decode()},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )


@then("the verification result reports c2pa_manifest_found is true, served from the remote manifest fallback")
def step_then_verified_from_remote_manifest(context):
    resp = context.requests_response
    assert resp.status_code == 200, f"copy verification failed: {resp.status_code} {resp.text}"
    body = resp.json()
    assert body.get("c2pa_manifest_found") is True, f"no C2PA manifest found for the stripped copy: {body}"
    assert body.get("c2pa_manifest_source") == "remote", (
        f"expected the remote manifest to be used for the stripped copy, got: {body}"
    )
    assert body.get("c2pa_signature_status") == "valid", f"remote manifest signature not valid: {body}"
