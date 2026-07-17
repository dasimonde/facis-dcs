"""BDD step definitions for the real signing vertical
(features/22_real_signing_vertical; SRS DCS-FR-SM-08/-14/-16/-18,
DCS-IR-SI-10, DCS-FR-CWE-04): PAdES signing, the EUDIPLO signing ceremony,
and PID binding.

--- Binding decisions this pack relies on ---

1. pdf-core's own POST /sign is NOT reachable from this harness: the BDD
   runner only ever talks to the backend (BDD_DCS_BASE_URL); pdf-core sits
   behind the backend's internal network path (see
   backend/internal/pdfgeneration/pdfcore/client.go). The PAdES scenarios
   therefore exercise signing indirectly through POST /signature/apply and
   inspect the PDF bytes the backend serves afterwards via GET
   /pdf/export/contract/{did} - the same "black-box HTTP only" discipline
   established throughout this codebase's BDD packs. The pdf-core-level,
   pyHanko-based cryptographic conformance proof lives in pdf-core's own
   behave harness (pdf-core/features/), out of scope for this repo-root
   harness.

2. The ceremony endpoints POST /signature/request, GET
   /signature/request/{ceremony_id}, POST
   /signature/presentation/request/{ceremony_id}, and POST
   /signature/request/webhook are defined in
   backend/design/signature_management.go.

3. EUDIPLO itself is never co-deployed or called by this harness. Instead,
   THIS HARNESS plays the "EUDIPLO test client" role: it POSTs directly to
   POST /signature/request/webhook with a real, protocol-correct SD-JWT VC
   + KB-JWT presentation (built with the existing testWallet/dcs_wallet
   signing primitives - the same library AuthService already uses for the
   OID4VP login flow, just with PID-shaped claims (vct urn:eudi:pid:1,
   given_name/family_name) instead of the role-credential shape). The BDD
   harness calls the webhook the way EUDIPLO itself would - a legitimate
   stand-in for a co-deployed EUDIPLO instance.

4. The webhook's shared-secret header is X-EUDIPLO-Webhook-Secret, env
   BDD_EUDIPLO_WEBHOOK_SECRET (default "bdd-eudiplo-webhook-secret"). The
   requirement-accurate claim under test is "a request without the correct
   shared secret is rejected", independent of the exact header name.

5. Several byte-level PDF assertions (SubFilter, x5chain presence, RFC3161
   timestamp token, ByteRange coverage) use the same "direct-byte-search
   over the raw, uncompressed PDF bytes" technique established in
   steps/pdf_generation/pdf_steps.py and
   steps/pki_consolidation/dcs_pki_consolidation_steps.py rather than a
   full PDF/CMS/ASN.1 parse. Each such check documents its own precision
   limits at its point of use.

The PAdES-B-B TSA-fallback scenario is driven via pdf-core's own
DCS_PDF_CORE_TSA_URL env (see dcs_real_signing_vertical_orce_steps.py); the
evidence-tamper scenario uses the IPFS CID-swap seam (see
dcs_real_signing_vertical_tamper_steps.py). The Signature Manager UI
(QR/poll/result flow, AES badge) has no coverage here: no browser
automation exists in this BDD stack, the service-level contract is
exercised by the ceremony scenarios, and the UI-specific claims are
recorded as an explicit coverage gap via the @skip placeholder scenario at
the bottom of the feature file, not a fabricated pass.
"""

from __future__ import annotations

import time
import uuid

from behave import given, then, when

import requests

from steps.support.api_client import (
    signature_apply_url,
    signature_request_by_id_url,
    signature_request_url,
    signature_request_webhook_url,
    signature_retrieve_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.pdf_service import PDFService
from steps.template_management.contract_state_machine_steps import (
    _advance_to_approved,
)


EUDIPLO_WEBHOOK_SECRET_HEADER = "X-EUDIPLO-Webhook-Secret"


def _webhook_secret() -> str:
    import os

    return os.getenv("BDD_EUDIPLO_WEBHOOK_SECRET", "bdd-eudiplo-webhook-secret")


# ---------------------------------------------------------------------------
# PID SD-JWT VC + KB-JWT presentation builder (see module docstring, point 3)
# ---------------------------------------------------------------------------


def _build_pid_presentation(*, given_name: str, family_name: str, aud: str, nonce: str, holder_private=None):
    """Build a real, protocol-correct PID SD-JWT VC + KB-JWT presentation
    using the same testWallet/dcs_wallet signing primitives already used by
    AuthService for the DCS role-credential OID4VP login flow — just with
    PID-shaped claims (vct urn:eudi:pid:1) instead of organization/roles.
    Returns (compact_presentation, issuer_jwt, disclosures, subject_did).

    holder_private lets a scenario present as a DIFFERENT natural person than
    the shared test wallet: the trusted test issuer binds the credential's cnf
    to whatever holder key it is given, so a fresh ephemeral key yields a
    fresh subject DID (needed by multi-signer scenarios, where two fields
    must be signed by two distinct identities).
    """
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.issuer import (  # noqa: PLC0415
        DEFAULT_ISSUER_DID,
        sign_credential_sd_jwt,
        sign_key_binding_jwt,
    )
    from dcs_wallet.keys import cnf_jwk, did_jwk_from_public_jwk, public_jwk  # noqa: PLC0415
    from dcs_wallet.sdjwt import join_sd_jwt, split_sd_jwt  # noqa: PLC0415

    keys = AuthService.load_wallet_keys()
    holder_key = holder_private or keys.wallet_private
    holder_public = public_jwk(holder_key)
    subject_did = did_jwk_from_public_jwk(holder_public)

    now = int(time.time())
    visible_claims = {
        "iss": DEFAULT_ISSUER_DID,
        "sub": subject_did,
        "vct": "urn:eudi:pid:1",
        "iat": now - 3600,
        "exp": now + 3600,
        "cnf": {"jwk": cnf_jwk(holder_public)},
    }
    selective_claims = {"given_name": given_name, "family_name": family_name}
    issued = sign_credential_sd_jwt(
        visible_claims=visible_claims,
        selective_claims=selective_claims,
        issuer_private=keys.issuer_private,
    )
    issuer_jwt, disclosures, _old_kb = split_sd_jwt(issued)
    kb_jwt = sign_key_binding_jwt(
        issuer_jwt=issuer_jwt,
        disclosures=disclosures,
        wallet_private=holder_key,
        aud=aud,
        nonce=nonce,
    )
    presentation = join_sd_jwt(issuer_jwt, disclosures, kb_jwt)
    return presentation, issuer_jwt, disclosures, subject_did


# ---------------------------------------------------------------------------
# Ceremony helpers
# ---------------------------------------------------------------------------


def _start_ceremony(context, name, field_name, headers):
    did, _ = ContractService._contract_data(context, name)
    resp = post_json(
        context,
        signature_request_url(context),
        {"contract_did": did, "field_name": field_name},
        headers=headers,
    )
    return resp


def _complete_ceremony_via_webhook(context, ceremony_id, presentation, subject_did, given_name, family_name, *, secret=None):
    payload = {
        "ceremony_id": ceremony_id,
        "vp_token": presentation,
        "pid_claims": {
            "sub": subject_did,
            "given_name": given_name,
            "family_name": family_name,
        },
    }
    header_value = _webhook_secret() if secret is None else secret
    headers = {"Content-Type": "application/json"}
    if header_value is not None:
        headers[EUDIPLO_WEBHOOK_SECRET_HEADER] = header_value
    return post_json(context, signature_request_webhook_url(context), payload, headers=headers)


def _run_full_ceremony(context, name, field_name, signatory_name, holder_private=None):
    """Start a ceremony, complete it headlessly via the assumed webhook
    contract (see module docstring point 3), and stash the presentation +
    ceremony id on context for later PDF-embedding assertions.
    """
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    start_resp = _start_ceremony(context, name, field_name, signer_h)
    assert start_resp.status_code == 200, (
        f"POST /signature/request failed for contract '{name}': "
        f"{start_resp.status_code} {start_resp.text}"
    )
    ceremony_id = start_resp.json().get("ceremony_id")
    assert ceremony_id, f"/signature/request response has no ceremony_id: {start_resp.text}"

    nonce = str(uuid.uuid4())
    given_name, family_name = signatory_name, "BDD-Testperson"
    presentation, issuer_jwt, disclosures, subject_did = _build_pid_presentation(
        given_name=given_name, family_name=family_name, aud="dcs-signature-ceremony", nonce=nonce,
        holder_private=holder_private,
    )
    webhook_resp = _complete_ceremony_via_webhook(
        context, ceremony_id, presentation, subject_did, given_name, family_name
    )
    assert webhook_resp.status_code == 200, (
        f"POST /signature/request/webhook failed for ceremony '{ceremony_id}': "
        f"{webhook_resp.status_code} {webhook_resp.text}"
    )

    if not hasattr(context, "ceremony_ids"):
        context.ceremony_ids = {}
    if not hasattr(context, "pid_presentations"):
        context.pid_presentations = {}
    context.ceremony_ids[name] = ceremony_id
    context.pid_presentations[name] = {
        "presentation": presentation,
        "subject_did": subject_did,
        "given_name": given_name,
        "family_name": family_name,
    }
    return ceremony_id, presentation, subject_did


def _apply_signature(context, name, *, signer_did, credential_type="AES", field_name=None):
    did, updated_at = ContractService._contract_data(context, name)
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    payload = {
        "did": did,
        "signer_did": signer_did,
        "credential_type": credential_type,
        "updated_at": updated_at,
    }
    if field_name is not None:
        payload["field_name"] = field_name
    return post_json(context, signature_apply_url(context), payload, headers=signer_h)


# ---------------------------------------------------------------------------
# Given — the shared "fully signed via a real ceremony" precondition, reused
# by most scenarios in this pack.
# ---------------------------------------------------------------------------


@given('contract "{name}" is APPROVED and has completed a signing ceremony for signatory "{signatory_name}"')
def step_given_approved_with_completed_ceremony(context, name, signatory_name):
    ContractService._create_contract_in_draft_with_signature_field(context, name, signatory_name)
    _advance_to_approved(context, name)
    _run_full_ceremony(context, name, signatory_name, signatory_name)


@given('contract "{name}" has an AES-signed PDF via a completed ceremony for signatory "{signatory_name}"')
def step_given_aes_signed_pdf_via_ceremony(context, name, signatory_name):
    ContractService._create_contract_in_draft_with_signature_field(context, name, signatory_name)
    _advance_to_approved(context, name)

    ceremony_id, presentation, subject_did = _run_full_ceremony(context, name, signatory_name, signatory_name)

    apply_resp = _apply_signature(context, name, signer_did=subject_did, credential_type="AES")
    assert apply_resp.status_code == 200, (
        f"POST /signature/apply failed for contract '{name}' after a completed ceremony: "
        f"{apply_resp.status_code} {apply_resp.text}"
    )
    ContractService._refresh_contract(context, name)

    signed_did, _ = ContractService._contract_data(context, name)
    context.headers = AuthService.get_headers_for_roles(["Contract Manager"])
    export_resp = PDFService.export_contract_pdf(context, signed_did)
    assert export_resp.status_code == 200, (
        f"PDF export failed for signed contract '{name}': {export_resp.status_code} {export_resp.text}"
    )
    if not hasattr(context, "pdf_bytes"):
        context.pdf_bytes = {}
    context.pdf_bytes[name] = export_resp.content


# ---------------------------------------------------------------------------
# When — (re-)export, apply variants, revoke-as-post-sign-update
# ---------------------------------------------------------------------------


@when('I re-export the signed PDF for contract "{name}"')
def step_when_reexport_signed_pdf(context, name):
    did, _ = ContractService._contract_data(context, name)
    resp = PDFService.export_contract_pdf(context, did)
    context.requests_response = resp
    if resp.status_code == 200:
        if not hasattr(context, "pdf_bytes"):
            context.pdf_bytes = {}
        context.pdf_bytes[name] = resp.content


@when('contract signer applies a signature to contract "{name}" without a prior signing ceremony')
def step_when_apply_without_ceremony(context, name):
    context.requests_response = _apply_signature(
        context, name, signer_did="did:example:bdd-no-ceremony-signer", credential_type="AES"
    )


@when(
    'contract signer applies a signature to contract "{name}" with signer_did "{signer_did}" and '
    'credential_type "{credential_type}"'
)
def step_when_apply_with_explicit_fields(context, name, signer_did, credential_type):
    context.requests_response = _apply_signature(
        context, name, signer_did=signer_did, credential_type=credential_type
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('contract signer applies a signature to contract "{name}" using the ceremony\'s signer_did and credential_type "{credential_type}"')
def step_when_apply_with_ceremony_signer_did(context, name, credential_type):
    signer_did = context.pid_presentations[name]["subject_did"]
    context.ceremony_signer_did = signer_did
    context.requests_response = _apply_signature(
        context, name, signer_did=signer_did, credential_type=credential_type
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('the signature for contract "{name}" is revoked as a post-sign C2PA update')
def step_when_revoke_post_sign_update(context, name):
    import requests as _requests  # noqa: PLC0415

    from steps.support.api_client import signature_revoke_url, signature_view_url  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    presentation = getattr(context, "pid_presentations", {}).get(name, {})
    signer_did = presentation.get("subject_did")
    if not signer_did:
        # No ceremony ran in this scenario — resolve the actual signer from
        # the signature view; a fabricated DID would be rejected with a 400
        # (db.ErrSignatureNotFound) instead of silently revoking nothing.
        view = _requests.get(
            signature_view_url(context), params={"did": did}, headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        assert view.status_code == 200, f"signature view failed: {view.status_code} {view.text}"
        signatures = view.json().get("signatures") or []
        assert signatures, f"Expected an applied signature to revoke on '{name}', got: {view.json()}"
        signer_did = signatures[0]["signer_did"]
    context.requests_response = post_json(
        context,
        signature_revoke_url(context),
        {"did": did, "signer_did": signer_did},
        headers=manager_h,
    )


@when('I start a signing ceremony for contract "{name}" field "{field_name}" as "{role}"')
def step_when_start_ceremony_as_role(context, name, field_name, role):
    headers = AuthService.get_headers_for_roles([role])
    context.requests_response = _start_ceremony(context, name, field_name, headers)
    if context.requests_response.status_code == 200:
        ceremony_id = context.requests_response.json().get("ceremony_id")
        if not hasattr(context, "ceremony_ids"):
            context.ceremony_ids = {}
        context.ceremony_ids[name] = ceremony_id

        # Build (but do not submit) the PID presentation the webhook steps
        # need — scenarios that start a ceremony via this low-level step
        # complete it separately via the webhook steps, which expect
        # context.pid_presentations[name] to already be populated (same
        # contract as _run_full_ceremony, minus the webhook POST itself).
        nonce = str(uuid.uuid4())
        given_name, family_name = field_name, "BDD-Testperson"
        presentation, _issuer_jwt, _disclosures, subject_did = _build_pid_presentation(
            given_name=given_name, family_name=family_name, aud="dcs-signature-ceremony", nonce=nonce
        )
        if not hasattr(context, "pid_presentations"):
            context.pid_presentations = {}
        context.pid_presentations[name] = {
            "presentation": presentation,
            "subject_did": subject_did,
            "given_name": given_name,
            "family_name": family_name,
        }


@when('I poll the signing ceremony status for contract "{name}"')
def step_when_poll_ceremony_status(context, name):
    ceremony_id = context.ceremony_ids[name]
    context.requests_response = get_with_headers(
        context,
        signature_request_by_id_url(context, ceremony_id),
        headers=AuthService.get_headers_for_roles(["Contract Signer"]),
    )


@when('the EUDIPLO webhook confirms the presentation for contract "{name}" with the correct shared secret')
def step_when_webhook_confirms_correct_secret(context, name):
    ceremony_id = context.ceremony_ids[name]
    presentation_info = context.pid_presentations[name]
    context.requests_response = _complete_ceremony_via_webhook(
        context,
        ceremony_id,
        presentation_info["presentation"],
        presentation_info["subject_did"],
        presentation_info["given_name"],
        presentation_info["family_name"],
    )


@when('a caller posts the EUDIPLO webhook for contract "{name}" with an incorrect shared secret')
def step_when_webhook_wrong_secret(context, name):
    ceremony_id = context.ceremony_ids[name]
    presentation_info = context.pid_presentations[name]
    context.requests_response = _complete_ceremony_via_webhook(
        context,
        ceremony_id,
        presentation_info["presentation"],
        presentation_info["subject_did"],
        presentation_info["given_name"],
        presentation_info["family_name"],
        secret="wrong-secret-value",
    )


# "I validate the signature for contract ..." is already defined in
# steps/pki_consolidation/dcs_pki_consolidation_steps.py and reused here as-is.


# ---------------------------------------------------------------------------
# Then — byte-level PAdES/PDF assertions
# ---------------------------------------------------------------------------


def _pdf_bytes_for(context, name) -> bytes:
    pdf_bytes = getattr(context, "pdf_bytes", {}).get(name)
    assert pdf_bytes, f"no exported PDF bytes recorded for contract '{name}'"
    return pdf_bytes


def _utf16be(ascii_bytes: bytes) -> bytes:
    result = bytearray([0xFE, 0xFF])
    for b in ascii_bytes:
        result.extend([0x00, b])
    return bytes(result)


def _last_byte_range(pdf_bytes: bytes):
    """Parse the LAST '/ByteRange [o1 l1 o2 l2]' occurrence — the final
    incremental-update revision's signature dictionary, i.e. the one that
    should cover everything appended before it (order enforcement:
    embed-first-sign-second).
    """
    idx = pdf_bytes.rfind(b"/ByteRange")
    assert idx != -1, "no /ByteRange entry found — PDF does not contain a PAdES signature dictionary"
    start = pdf_bytes.find(b"[", idx)
    end = pdf_bytes.find(b"]", start)
    assert start != -1 and end != -1, "/ByteRange entry is not followed by a well-formed array"
    nums = [int(tok) for tok in pdf_bytes[start + 1 : end].split()]
    assert len(nums) == 4, f"/ByteRange array does not have exactly 4 integers: {nums}"
    o1, l1, o2, l2 = nums
    return (o1, o1 + l1), (o2, o2 + l2)


def _offset_covered(pdf_bytes: bytes, needle: bytes, ranges) -> bool:
    pos = pdf_bytes.find(needle)
    assert pos != -1, f"expected byte sequence not found in PDF at all: {needle[:40]!r}"
    (a0, a1), (b0, b1) = ranges
    return (a0 <= pos < a1) or (b0 <= pos < b1)


@then('the signed PDF for contract "{name}" contains a PAdES signature naming AcroForm field "{field_name}"')
def step_then_pades_names_field(context, name, field_name):
    pdf_bytes = _pdf_bytes_for(context, name)
    needle_ascii = f"/T ({field_name})".encode()
    needle_ascii_nospace = f"/T({field_name})".encode()
    needle_utf16 = _utf16be(field_name.encode())
    assert (
        needle_ascii in pdf_bytes or needle_ascii_nospace in pdf_bytes or needle_utf16 in pdf_bytes
    ), (
        f"Expected the signed PDF to name AcroForm field '/T' == '{field_name}' "
        "(the signer signs an existing signature field by name: /T == signatoryName "
        "from the JSON-LD, NOT title), found neither ASCII nor UTF-16BE form"
    )
    assert b"/ByteRange" in pdf_bytes, (
        "Expected a /ByteRange entry (PAdES signature dictionary) in the signed PDF - none found"
    )


@then('the signed PDF for contract "{name}" has a structurally valid PAdES ByteRange')
def step_then_byte_range_structurally_valid(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    (a0, a1), (b0, b1) = _last_byte_range(pdf_bytes)
    assert a0 == 0, f"Expected the ByteRange's first segment to start at file offset 0, got {a0}"
    assert b1 <= len(pdf_bytes), (
        f"ByteRange's second segment end ({b1}) exceeds the actual PDF byte length ({len(pdf_bytes)})"
    )
    assert a1 < b0, (
        f"Expected a gap between the two ByteRange segments (the excluded /Contents hex signature "
        f"blob) — got [{a0},{a1}) and [{b0},{b1})"
    )


@then('the signed PDF for contract "{name}" declares SubFilter ETSI.CAdES.detached')
def step_then_subfilter_cades_detached(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    assert (
        b"/SubFilter/ETSI.CAdES.detached" in pdf_bytes or b"/SubFilter /ETSI.CAdES.detached" in pdf_bytes
    ), (
        "Expected the signed PDF's signature dictionary to declare "
        "'/SubFilter/ETSI.CAdES.detached' (PAdES) - not found"
    )


@then('the signed PDF for contract "{name}" embeds a non-empty X.509 certificate chain')
def step_then_x5chain_embedded(context, name):
    # Precision limit (see module docstring point 5): this checks the /Contents
    # hex-string CMS blob's length is large enough to plausibly carry an
    # embedded certificate chain (a bare ECDSA signature without any
    # certificates would be well under 1KB; a chain adds several KB of DER),
    # rather than fully ASN.1-parsing the CMS SignedData to enumerate
    # certificates. A full parse is the pdf-core-level pyHanko conformance
    # test's job (pdf-core/features/).
    pdf_bytes = _pdf_bytes_for(context, name)
    # Scan every "/Contents" occurrence and take the one with the largest hex
    # blob: page objects reference /Contents indirectly ("/Contents 19 0 R"),
    # embedded-file/evidence dicts may have their own small /Contents-like
    # entries, and only the /Sig dictionary's /Contents holds the multi-KB
    # CMS SignedData hex string (chain + signature).
    best_hex_len = -1
    search_from = 0
    while True:
        contents_idx = pdf_bytes.find(b"/Contents", search_from)
        if contents_idx == -1:
            break
        hex_start = pdf_bytes.find(b"<", contents_idx)
        hex_end = pdf_bytes.find(b">", hex_start) if hex_start != -1 else -1
        if hex_start != -1 and hex_end != -1:
            best_hex_len = max(best_hex_len, hex_end - hex_start - 1)
        search_from = contents_idx + 1
    assert best_hex_len != -1, "No /Contents hex string found in the signed PDF"
    hex_len = best_hex_len
    assert hex_len > 4000, (
        f"/Contents hex blob is only {hex_len} hex chars - too small to plausibly contain a full "
        "X.509 chain alongside the CMS signature (expected several KB for chain + signature); "
        "the CMS SignedData likely carries no embedded certificates"
    )


_TIMESTAMP_TOKEN_OID_DER = bytes.fromhex("060b2a864886f70d010910020e")


@then('the signed PDF for contract "{name}" embeds an RFC3161 timestamp token')
def step_then_rfc3161_timestamp_embedded(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    hex_needle_lower = _TIMESTAMP_TOKEN_OID_DER.hex().encode()
    hex_needle_upper = _TIMESTAMP_TOKEN_OID_DER.hex().upper().encode()
    assert hex_needle_lower in pdf_bytes or hex_needle_upper in pdf_bytes, (
        "Expected the CMS SignedData's unsigned attributes to embed an RFC3161 "
        "signatureTimeStampToken (OID 1.2.840.113549.1.9.16.2.14, PAdES-B-T per "
        "module docstring point 5) - its DER-encoded hex representation was not found anywhere "
        "in the signed PDF's /Contents hex string"
    )


@then('the signed PDF for contract "{name}" still has a structurally valid PAdES signature')
def step_then_pades_still_valid_after_update(context, name):
    step_then_byte_range_structurally_valid(context, name)


@then('the SD-JWT VC presentation for contract "{name}" is embedded verbatim inside the PAdES ByteRange')
def step_then_presentation_embedded_verbatim_covered(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    presentation = context.pid_presentations[name]["presentation"]
    needle = presentation.encode("ascii")
    assert needle in pdf_bytes, (
        "Expected the exact, verbatim SD-JWT VC + KB-JWT compact presentation string to appear "
        "unmodified somewhere in the signed PDF (the presentation must be embedded verbatim "
        "do NOT re-filter or re-serialize it) - not found at all"
    )
    ranges = _last_byte_range(pdf_bytes)
    assert _offset_covered(pdf_bytes, needle, ranges), (
        "The embedded SD-JWT VC presentation was found, but its byte offset falls OUTSIDE the "
        "PAdES signature's /ByteRange-covered regions - the identity credential must be embedded "
        "BEFORE signing, embed-first-sign-second, so the ByteRange covers it)"
    )


@then('a ContractSigningSummaryCredential for contract "{name}" is embedded inside the PAdES ByteRange')
def step_then_summary_credential_embedded_covered(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    needle = b"ContractSigningSummaryCredential"
    assert needle in pdf_bytes, (
        "Expected a ContractSigningSummaryCredential (DCS-FR-SM-08) to be embedded in the "
        "signed PDF - not found"
    )
    ranges = _last_byte_range(pdf_bytes)
    assert _offset_covered(pdf_bytes, needle, ranges), (
        "The ContractSigningSummaryCredential was found, but its byte offset falls OUTSIDE the "
        "PAdES signature's /ByteRange-covered regions - it must be embedded BEFORE signing"
    )


# ---------------------------------------------------------------------------
# Then — contract_signatures / DB-level assertions
# ---------------------------------------------------------------------------


def _fetch_signature_row(context, name):
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT * FROM contract_signatures WHERE contract_did = %s ORDER BY signed_at DESC NULLS LAST LIMIT 1",
        (did,),
    )
    row = cursor.fetchone()
    columns = [desc[0] for desc in cursor.description] if cursor.description else []
    cursor.close()
    assert row is not None, f"No contract_signatures row found for contract '{name}' (did={did})"
    return dict(zip(columns, row))


@then('the contract_signatures row for contract "{name}" is a real signature, not the STUB placeholder')
def step_then_no_stub_placeholder(context, name):
    row = _fetch_signature_row(context, name)
    sig_bytes = row.get("signature_bytes")
    # psycopg2 returns BYTEA columns as memoryview by default, not bytes.
    if sig_bytes is not None and not isinstance(sig_bytes, (bytes, bytearray)):
        sig_bytes = bytes(sig_bytes)
    assert sig_bytes != b"STUB_SIGNATURE_PLACEHOLDER", (
        f"contract_signatures.signature_bytes for '{name}' is the literal stub placeholder "
        f"bytes instead of a real PAdES signature. Row: {row}"
    )
    assert row.get("ipfs_cid"), (
        f"Expected contract_signatures.ipfs_cid to be populated for the signed PDF artefact "
        f"(DCS-FR-SM-15), got: {row.get('ipfs_cid')!r}"
    )


@then('the contract_signatures row for contract "{name}" records both a PDF hash and a JSON-LD content hash')
def step_then_binds_pdf_and_content_hash(context, name):
    # The assertion introspects the contract_signatures row generically for
    # two independent hash-shaped values (FR-SM-11: PDF hash + JSON-LD
    # content hash) rather than hardcoding column/evidence-key names,
    # keeping it robust to schema naming.
    row = _fetch_signature_row(context, name)

    def _find_hash_like(*name_fragments):
        for key, value in row.items():
            lowered = key.lower()
            if any(fragment in lowered for fragment in name_fragments) and value:
                return key, value
        return None, None

    pdf_hash_key, pdf_hash_value = _find_hash_like("pdf_hash", "base_pdf_hash")
    content_hash_key, content_hash_value = _find_hash_like("content_hash", "jsonld_hash", "contenthash")

    assert pdf_hash_key, (
        f"Expected a PDF-hash-shaped column on contract_signatures for '{name}' (FR-SM-11: "
        f"'record both the PDF hash and the JSON-LD contentHash in the signature row or evidence "
        f"JSON') - no column name containing 'pdf_hash'/'base_pdf_hash' with a non-null value found. "
        f"Row columns: {list(row.keys())}"
    )
    assert content_hash_key, (
        f"Expected a JSON-LD-contentHash-shaped column on contract_signatures for '{name}' "
        f"(FR-SM-11) - no column name containing 'content_hash'/'jsonld_hash'/'contenthash' with a "
        f"non-null value found. Row columns: {list(row.keys())}"
    )
    assert pdf_hash_value != content_hash_value, (
        f"Expected the PDF hash ({pdf_hash_key}) and the JSON-LD content hash ({content_hash_key}) "
        "to be independently computed, distinct values, not the same value duplicated into two columns"
    )


@then(
    'the signature envelope for contract "{name}" has signer_did "{signer_did}" and '
    'credential_type "{credential_type}"'
)
def step_then_envelope_has_signer_and_credential_type(context, name, signer_did, credential_type):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = get_with_headers(context, signature_retrieve_url(context, did), headers=manager_h)
    assert resp.status_code == 200, f"GET /signature/retrieve/{{did}} failed: {resp.status_code} {resp.text}"
    envelope = resp.json().get("signature_envelope") or {}
    assert envelope.get("signer_did") == signer_did, (
        f"Expected the applied signature's signer_did to be the REQUESTED '{signer_did}' "
        f"(apply must honor req.SignerDid rather than silently discarding it in favor of "
        f"the authenticated caller's own participant id, see backend/internal/service/"
        f"signature_management.go's Apply handler), got: {envelope.get('signer_did')!r}"
    )
    assert envelope.get("credential_type") == credential_type, (
        f"Expected credential_type '{credential_type}' (apply must thread req.CredentialType "
        f"through instead of leaving command.ApplyCmd.CredentialType unset), got: "
        f"{envelope.get('credential_type')!r}"
    )


@then('the signature envelope for contract "{name}" reflects the ceremony\'s signer_did and credential_type "{credential_type}"')
def step_then_envelope_reflects_ceremony_signer_did(context, name, credential_type):
    step_then_envelope_has_signer_and_credential_type(
        context, name, context.ceremony_signer_did, credential_type
    )


# ---------------------------------------------------------------------------
# Then — apply gate, ceremony endpoints, validate
# ---------------------------------------------------------------------------


@then("the apply request is rejected with a typed ceremony-required error")
def step_then_apply_rejected_ceremony_required(context):
    resp = context.requests_response
    assert resp.status_code in (400, 403, 409, 422), (
        f"Expected /signature/apply to refuse signing without a completed PID presentation for "
        f"this signer+contract, got {resp.status_code}: {resp.text}"
    )
    body_text = resp.text.lower()
    assert "ceremony" in body_text or "presentation" in body_text or "pid" in body_text, (
        "Expected the rejection body to name the missing ceremony/PID-presentation precondition "
        f"as a typed, understandable error (not a generic internal_error) - got: {resp.text}"
    )


@then("the ceremony response includes a ceremony_id, wallet_uri, and expires_at")
def step_then_ceremony_start_response_shape(context):
    import base64
    import json
    from urllib.parse import parse_qs, unquote

    resp = context.requests_response
    assert resp.status_code == 200, f"POST /signature/request failed: {resp.status_code} {resp.text}"
    body = resp.json()
    for field in ("ceremony_id", "wallet_uri", "expires_at"):
        assert body.get(field), f"/signature/request response missing '{field}': {body}"

    wallet_uri = body["wallet_uri"]
    assert wallet_uri.startswith("openid4vp://?"), f"wallet_uri must be an openid4vp deep link: {wallet_uri}"
    query = wallet_uri.split("?", 1)[1]
    params = parse_qs(query)
    assert params.get("client_id", [""])[0] == "dcs-signature-ceremony", params
    assert params.get("request_uri_method", [""])[0] == "post", params
    assert "nonce" not in params, f"nonce belongs in the JAR, not the deep link: {wallet_uri}"
    request_uri = unquote(params.get("request_uri", [""])[0])
    ceremony_id = body["ceremony_id"]
    assert request_uri.endswith(f"/signature/presentation/request/{ceremony_id}"), (
        f"request_uri must point at the ceremony JAR endpoint, got: {request_uri}"
    )
    # OpenID4VP request_uri_method=post: form-urlencoded POST.
    jar_resp = requests.post(
        request_uri,
        timeout=context.http_timeout_seconds,
        headers={
            "Accept": "application/oauth-authz-req+jwt, application/jwt",
            "Content-Type": "application/x-www-form-urlencoded",
        },
        data={},
    )
    assert jar_resp.status_code == 200, (
        f"POST ceremony presentation request failed: {jar_resp.status_code} {jar_resp.text}"
    )
    content_type = (jar_resp.headers.get("Content-Type") or "").split(";", 1)[0].strip()
    assert content_type == "application/oauth-authz-req+jwt", (
        f"expected JAR content type, got {content_type!r}"
    )
    jar = jar_resp.text.strip()
    parts = jar.split(".")
    assert len(parts) >= 2, "authorization request must look like a JWT"
    header = json.loads(base64.urlsafe_b64decode(parts[0] + "=="))
    assert header.get("typ") == "oauth-authz-req+jwt", header
    claims = json.loads(base64.urlsafe_b64decode(parts[1] + "=="))
    assert claims.get("client_id") == "dcs-signature-ceremony", claims
    assert claims.get("response_type") == "vp_token", claims
    assert claims.get("response_mode") == "direct_post", claims
    assert claims.get("state") == ceremony_id, claims
    assert claims.get("nonce"), "JAR must carry the ceremony nonce"
    assert str(claims.get("response_uri", "")).endswith("/signature/presentation/callback"), claims
    assert claims.get("dcql_query"), "JAR must include PID dcql_query"

@then("the ceremony start request is denied for that role")
def step_then_ceremony_start_denied(context):
    resp = context.requests_response
    assert resp.status_code in (401, 403), (
        f"Expected POST /signature/request to reject an unauthorized/unauthorized-role caller "
        f"(FR-SM-14: 'Requests MUST only be valid if the signer's role and authorization are "
        f"verified'), got {resp.status_code}: {resp.text}"
    )


@then('the signing ceremony for contract "{name}" has status "{status}"')
def step_then_ceremony_status(context, name, status):
    resp = context.requests_response
    assert resp.status_code == 200, f"GET /signature/request/{{id}} failed: {resp.status_code} {resp.text}"
    body = resp.json()
    assert str(body.get("status", "")).lower() == status.lower(), (
        f"Expected ceremony status '{status}' for contract '{name}', got: {body}"
    )


@then("the webhook request is rejected for the incorrect shared secret")
def step_then_webhook_rejected(context):
    resp = context.requests_response
    assert resp.status_code in (401, 403), (
        f"Expected POST /signature/request/webhook to reject a request presenting the wrong "
        f"shared-secret header value, got {resp.status_code}: {resp.text}"
    )


@then('the signature validation findings for contract "{name}" cross-check the embedded PID evidence')
def step_then_validate_crosschecks_pid_evidence(context, name):
    resp = context.requests_response
    assert resp.status_code == 200, f"/signature/validate failed: {resp.status_code} {resp.text}"
    findings = resp.json().get("findings") or []
    body_text = " ".join(findings).lower()
    failure_markers = ("pid verification failed", "kb-jwt invalid", "sd-jwt invalid", "evidence mismatch")
    hit = [m for m in failure_markers if m in body_text]
    assert not hit, (
        f"Expected the re-verified, embedded PID presentation to cross-check successfully against "
        f"the signature record for contract '{name}', got findings "
        f"suggesting a mismatch ({hit}): {findings}"
    )


@then('the contract_signatures row for contract "{name}" is linked to a signature_ceremonies row')
def step_then_signature_linked_to_ceremony(context, name):
    row = _fetch_signature_row(context, name)
    ceremony_key = next((k for k in row if "ceremony" in k.lower()), None)
    assert ceremony_key and row.get(ceremony_key), (
        f"Expected a ceremony-linking column (e.g. 'ceremony_id') on contract_signatures for "
        f"'{name}' (contract_signatures links to its ceremony via a nullable "
        f"ceremony_id column) with a non-null value. Row columns: "
        f"{list(row.keys())}"
    )
    expected_ceremony_id = context.ceremony_ids.get(name)
    if expected_ceremony_id:
        assert str(row.get(ceremony_key)) == str(expected_ceremony_id), (
            f"contract_signatures.{ceremony_key} ({row.get(ceremony_key)!r}) does not match the "
            f"ceremony this contract was actually signed through ({expected_ceremony_id!r})"
        )
