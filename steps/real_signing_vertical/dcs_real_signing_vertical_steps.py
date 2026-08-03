"""BDD step definitions for the real signing vertical
(features/22_real_signing_vertical; SRS DCS-FR-SM-08/-14/-16/-18,
DCS-IR-SI-10, DCS-FR-CWE-04): PAdES signing, the wallet-driven signing ceremony,
and PID binding.

--- Binding decisions this pack relies on ---

1. pdf-core's own POST /sign is NOT reachable from this harness: the BDD
   runner only ever talks to the backend (BDD_DCS_BASE_URL); pdf-core sits
   behind the backend's internal network path (see
   backend/internal/pdfgeneration/pdfcore/client.go). The PAdES scenarios
   therefore exercise signing indirectly through the prepare/submit wallet
   ceremony (ADR-12/ADR-20, no /signature/apply anymore — that transitional
   DCS-as-signatory path is removed) and inspect the PDF bytes the backend
   serves afterwards via GET /pdf/export/contract/{did} - the same
   "black-box HTTP only" discipline established throughout this codebase's
   BDD packs. The pdf-core-level, pyHanko-based cryptographic conformance
   proof lives in pdf-core's own behave harness (pdf-core/features/), out of
   scope for this repo-root harness.

2. The ceremony endpoints POST /signature/request, GET
   /signature/request/{ceremony_id}, and POST
   /signature/request/{ceremony_id}/callback are defined in
   backend/design/signature_management.go.

3. EUDIPLO is not a dependency of this DCS (ADR-20: the remote EUDIPLO PID
   service was broken and is removed). Ceremony PID+PoA verification is
   wallet-presented OID4VP: THIS HARNESS plays the wallet, direct_post'ing a
   real, protocol-correct SD-JWT VC + KB-JWT vp_token — carrying BOTH the PID
   and the Power of Attorney presentation, keyed by their DCQL credential
   query ids — straight to the ceremony's own callback
   (POST /signature/request/{ceremony_id}/callback), the same endpoint the
   signed-document callback later reuses once the ceremony is published. Both
   presentations are built with the existing testWallet/dcs_wallet signing
   primitives, the same library AuthService already uses for the OID4VP login
   flow, just with PID-shaped claims (vct urn:dcs:pid:demo:v1, given_name/
   family_name) instead of the role-credential shape, and bound to the
   ceremony's own request nonce (fetched from its pending-stage request
   object, never invented locally).

4. The callback is authenticated by the unguessable ceremony id in the URL,
   not a shared secret (ADR-12) — there is no separate webhook auth step to
   test. What IS still tested here: a presentation whose KB-JWT nonce does
   not match the ceremony's own request nonce is rejected (the same
   cryptographic binding a shared secret would have gated, without a secret
   to leak or rotate).

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

from steps.support.api_client import (
    signature_request_by_id_url,
    signature_request_leaf_url,
    signature_request_url,
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


# The DCQL credential query ids (backend/internal/auth/oid4vp/pid.go
# PIDCredentialQueryID, poa.go PoACredentialQueryID) a wallet's combined
# vp_token is keyed by.
PID_QUERY_ID = "eudi_pid_credential"
POA_QUERY_ID = "dcs_poa_credential"


def ceremony_aud(context) -> str:
    """The ceremony's OID4VP client_id, and therefore the audience its KB-JWTs
    must be bound to. It is the deployment's own x509_san_dns identifier, so it
    is read from the request object the DCS actually published (recorded by
    _fetch_pending_nonce) rather than hardcoded."""
    client_id = str(getattr(context, "ceremony_client_id", "") or "").strip()
    assert client_id, (
        "no ceremony client_id recorded - fetch the ceremony request object "
        "(_fetch_pending_nonce) before building a presentation for it"
    )
    return client_id


def _fetch_pending_nonce(context, ceremony_id: str) -> str:
    """GET the ceremony's pending-stage request object and return its request
    nonce — the real per-ceremony binding target a presentation's KB-JWT must
    echo, not a value the harness invents locally."""
    import requests as _requests  # noqa: PLC0415

    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.oid4vp_signing import _decode_jwt_claims  # noqa: PLC0415

    resp = _requests.get(
        signature_request_leaf_url(context, ceremony_id, "object"),
        headers={"Accept": "application/oauth-authz-req+jwt"},
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, (
        f"fetch pending ceremony request object failed for {ceremony_id!r}: "
        f"{resp.status_code} {resp.text}"
    )
    claims = _decode_jwt_claims(resp.text.strip())
    nonce = str(claims.get("nonce") or "").strip()
    assert nonce, f"pending ceremony request object carries no nonce: {claims}"
    client_id = str(claims.get("client_id") or "").strip()
    assert client_id, f"pending ceremony request object carries no client_id: {claims}"
    context.ceremony_client_id = client_id
    return nonce


def _build_poa_presentation(*, organization: str, roles: list[str], aud: str, nonce: str) -> str:
    """Build a Power of Attorney SD-JWT VC + KB-JWT presentation authorizing
    organization, bound to the ceremony's own aud/nonce (UC-14, FR-SM-03). It
    names the same issuer status list AuthService.build_vp_token issues the
    login credential against, on the organization's own index."""
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.issuer import DEFAULT_ISSUER_DID, issue_access_credential  # noqa: PLC0415
    from dcs_wallet.status_list import role_credential_index  # noqa: PLC0415

    import os  # noqa: PLC0415

    keys = AuthService.load_wallet_keys()
    issuer_did = os.getenv("BDD_ISSUER_DID", DEFAULT_ISSUER_DID)
    return issue_access_credential(
        organization=organization,
        roles=roles,
        issuer_private=keys.issuer_private,
        wallet_private=keys.wallet_private,
        status_index=role_credential_index(organization=organization, roles=roles),
        issuer_did=issuer_did,
        aud=aud,
        nonce=nonce,
    )


# ---------------------------------------------------------------------------
# PID SD-JWT VC + KB-JWT presentation builder (see module docstring, point 3)
# ---------------------------------------------------------------------------


def _build_pid_presentation(*, given_name: str, family_name: str, aud: str, nonce: str, holder_private=None):
    """Build a real, protocol-correct PID SD-JWT VC + KB-JWT presentation
    using the same testWallet/dcs_wallet signing primitives already used by
    AuthService for the DCS role-credential OID4VP login flow — just with
    PID-shaped claims (vct urn:dcs:pid:demo:v1) instead of organization/roles.
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
    from dcs_wallet.status_list import build_credential_status, pid_credential_index  # noqa: PLC0415

    keys = AuthService.load_wallet_keys()
    holder_key = holder_private or keys.wallet_private
    holder_public = public_jwk(holder_key)
    subject_did = did_jwk_from_public_jwk(holder_public)

    now = int(time.time())
    # A real status claim (ADR-20): VerifyPID's status-list check is no longer
    # skipped now that EUDIPLO — which omitted status — is gone, so a PID
    # presentation with no status claim would be rejected outright. One bit per
    # natural person, on the issuer's own signed list.
    visible_claims = {
        "iss": DEFAULT_ISSUER_DID,
        "sub": subject_did,
        "vct": "urn:dcs:pid:demo:v1",
        "iat": now - 3600,
        "exp": now + 3600,
        "cnf": {"jwk": cnf_jwk(holder_public)},
        "status": build_credential_status(
            index=pid_credential_index(given_name=given_name, family_name=family_name),
        ),
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


def _build_pid_presentation_x5c(*, given_name: str, family_name: str, aud: str, nonce: str, trusted: bool = True):
    """Same as _build_pid_presentation, but the issuer credential JWT carries
    an x5c certificate chain in its header instead of a bare jwk+kid — what a
    real EUDI wallet's issued PID actually looks like. The dev issuer's cert
    names its DID in a SAN URI, because the chain is only accepted for an
    issuer the leaf identifies. trusted=False signs with an UNRELATED
    self-signed cert never configured as an OID4VP_X5C_TRUST_ANCHORS_PATH
    root, for the negative "untrusted issuer is refused" case.
    """
    AuthService._ensure_dcs_wallet_importable()
    from cryptography import x509  # noqa: PLC0415
    from cryptography.hazmat.primitives import serialization  # noqa: PLC0415

    from dcs_wallet.issuer import sign_credential_sd_jwt_x5c, sign_key_binding_jwt  # noqa: PLC0415
    from dcs_wallet.keys import cnf_jwk, did_jwk_from_public_jwk, load_json, private_key_material, public_jwk  # noqa: PLC0415
    from dcs_wallet.sdjwt import join_sd_jwt, split_sd_jwt  # noqa: PLC0415
    from dcs_wallet.status_list import build_credential_status, pid_credential_index  # noqa: PLC0415

    keys_dir = AuthService.resolve_wallet_keys_dir()
    if trusted:
        issuer_private = private_key_material(load_json(keys_dir / "issuer-dev-x5c.jwk"))
        issuer_cert_der = x509.load_pem_x509_certificate(
            (keys_dir / "issuer-dev-x5c.crt.pem").read_bytes()
        ).public_bytes(serialization.Encoding.DER)
        issuer_did = "did:web:dev.example:issuer:pid-x5c"
    else:
        # An UNRELATED, freshly-minted self-signed cert — never configured as
        # a trust anchor anywhere, deliberately not the trusted dev issuer.
        from cryptography.hazmat.primitives import hashes  # noqa: PLC0415
        from cryptography.hazmat.primitives.asymmetric import ec  # noqa: PLC0415
        from cryptography.x509.oid import NameOID  # noqa: PLC0415
        import base64 as _b64  # noqa: PLC0415
        import datetime as _dt  # noqa: PLC0415

        untrusted_key = ec.generate_private_key(ec.SECP256R1())
        subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "Untrusted PID Issuer (BDD negative case)")])
        now = _dt.datetime.now(_dt.timezone.utc)
        untrusted_cert = (
            x509.CertificateBuilder()
            .subject_name(subject).issuer_name(subject).public_key(untrusted_key.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(now - _dt.timedelta(hours=1)).not_valid_after(now + _dt.timedelta(hours=1))
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .sign(untrusted_key, hashes.SHA256())
        )
        d = untrusted_key.private_numbers()
        pub = d.public_numbers
        coord = 32
        issuer_private = {
            "kty": "EC", "crv": "P-256",
            "x": _b64.urlsafe_b64encode(pub.x.to_bytes(coord, "big")).rstrip(b"=").decode(),
            "y": _b64.urlsafe_b64encode(pub.y.to_bytes(coord, "big")).rstrip(b"=").decode(),
            "d": _b64.urlsafe_b64encode(d.private_value.to_bytes(coord, "big")).rstrip(b"=").decode(),
        }
        issuer_cert_der = untrusted_cert.public_bytes(serialization.Encoding.DER)
        issuer_did = "did:web:untrusted.example:issuer:pid-x5c"

    wallet_private = private_key_material(load_json(keys_dir / "wallet.jwk"))
    holder_public = public_jwk(wallet_private)
    subject_did = did_jwk_from_public_jwk(holder_public)

    now_ts = int(time.time())
    visible_claims = {
        "iss": issuer_did,
        "sub": subject_did,
        "vct": "urn:dcs:pid:demo:v1",
        "iat": now_ts - 3600,
        "exp": now_ts + 3600,
        "cnf": {"jwk": cnf_jwk(holder_public)},
        "status": build_credential_status(
            index=pid_credential_index(given_name=given_name, family_name=family_name),
        ),
    }
    selective_claims = {"given_name": given_name, "family_name": family_name}
    issued = sign_credential_sd_jwt_x5c(
        visible_claims=visible_claims,
        selective_claims=selective_claims,
        issuer_private=issuer_private,
        issuer_cert_der=issuer_cert_der,
    )
    issuer_jwt, disclosures, _old_kb = split_sd_jwt(issued)
    kb_jwt = sign_key_binding_jwt(
        issuer_jwt=issuer_jwt, disclosures=disclosures, wallet_private=wallet_private, aud=aud, nonce=nonce,
    )
    presentation = join_sd_jwt(issuer_jwt, disclosures, kb_jwt)
    return presentation, subject_did


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


def _complete_ceremony_via_presentation(
    context, ceremony_id, presentation, subject_did, given_name, family_name,
    *, poa_organization=None, nonce=None,
):
    """Complete a ceremony's PID(+PoA) presentation the way a wallet actually
    does it: direct_post a vp_token — keyed by the PID and PoA DCQL credential
    query ids — to the ceremony's own callback (ADR-20; this replaces the
    removed EUDIPLO webhook, see module docstring point 3).

    poa_organization=None omits the PoA credential from vp_token entirely
    (the "no PoA presented" negative case); any other value builds a PoA
    credential authorizing that organization (a mismatched one for the
    "wrong PoA" negative case). subject_did/given_name/family_name are
    unused by the request itself (the callback derives them by verifying
    presentation) but kept for symmetry with the old signature so callers
    changed minimally.
    """
    del subject_did, given_name, family_name
    nonce = nonce or _fetch_pending_nonce(context, ceremony_id)
    vp_token = {PID_QUERY_ID: [presentation]}
    if poa_organization:
        vp_token[POA_QUERY_ID] = [
            _build_poa_presentation(organization=poa_organization, roles=["Contract Signer"], aud=ceremony_aud(context), nonce=nonce)
        ]
    import json  # noqa: PLC0415

    import requests as _requests  # noqa: PLC0415

    return _requests.post(
        signature_request_leaf_url(context, ceremony_id, "callback"),
        data={"state": ceremony_id, "vp_token": json.dumps(vp_token, separators=(",", ":"))},
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        timeout=context.http_timeout_seconds,
    )


def _run_full_ceremony(context, name, field_name, signatory_name, holder_private=None, poa_organization=None):
    """Start a ceremony, complete it headlessly via the wallet's own
    direct_post presentation (see module docstring point 3), and stash the
    presentation + ceremony id on context for later PDF-embedding assertions.

    field_name is the party the signatory signs as — the participating DCS
    instance DID. poa_organization is the organization the signatory presents a
    Power of Attorney for; it must equal field_name for the ceremony to verify
    (UC-14), so it defaults to field_name.
    """
    if poa_organization is None:
        poa_organization = field_name
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    start_resp = _start_ceremony(context, name, field_name, signer_h)
    assert start_resp.status_code == 200, (
        f"POST /signature/request failed for contract '{name}': "
        f"{start_resp.status_code} {start_resp.text}"
    )
    ceremony_id = start_resp.json().get("ceremony_id")
    assert ceremony_id, f"/signature/request response has no ceremony_id: {start_resp.text}"

    nonce = _fetch_pending_nonce(context, ceremony_id)
    given_name, family_name = signatory_name, "BDD-Testperson"
    presentation, issuer_jwt, disclosures, subject_did = _build_pid_presentation(
        given_name=given_name, family_name=family_name, aud=ceremony_aud(context), nonce=nonce,
        holder_private=holder_private,
    )
    resp = _complete_ceremony_via_presentation(
        context, ceremony_id, presentation, subject_did, given_name, family_name,
        poa_organization=poa_organization, nonce=nonce,
    )
    assert resp.status_code == 200, (
        f"ceremony presentation failed for ceremony '{ceremony_id}': "
        f"{resp.status_code} {resp.text}"
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


def _apply_signature(context, name, *, signer_did, credential_type="AES", field_name=None, ceremony_id=None, signatory=None):
    """Drive the wallet-driven signing ceremony (ADR-12): prepare the
    to-be-signed PDF, sign it with the signatory's own key via the external SCA
    (a real EU DSS), then submit it. The DCS holds no signing key — it validates
    and records what the signatory produced. Replaces the removed
    /signature/apply. A precondition failure (e.g. no completed ceremony)
    surfaces from /signature/prepare, so error scenarios see the same responses.

    ceremony_id/signatory default to the contract's single most-recently-run
    ceremony (context.ceremony_ids/pid_presentations, keyed by contract name only)
    — correct for single-signer callers. Multi-signer callers run several
    ceremonies under the same contract name, so that shared state holds only the
    LAST ceremony run; they must pass both explicitly (see multi_signer_steps.py).
    """
    from steps.support.signing import wallet_sign  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    if ceremony_id is None or signatory is None:
        # The signatory name is the natural person (wallet key identity / certificate
        # subject), taken from the ceremony's PID. It is distinct from the signature
        # field, which names the party (instance DID) — passed separately as field_name.
        presentation = (getattr(context, "pid_presentations", {}) or {}).get(name) or {}
        if signatory is None:
            signatory = presentation.get("given_name") or name
        if ceremony_id is None:
            ceremony_id = (getattr(context, "ceremony_ids", {}) or {}).get(name)
    return wallet_sign(
        context,
        did,
        signer_did=signer_did,
        signatory=signatory,
        field_name=field_name,
        credential_type=credential_type,
        ceremony_id=ceremony_id,
    )


# ---------------------------------------------------------------------------
# Given — the shared "fully signed via a real ceremony" precondition, reused
# by most scenarios in this pack.
# ---------------------------------------------------------------------------


@given('contract "{name}" is APPROVED and has completed a signing ceremony for signatory "{signatory_name}"')
def step_given_approved_with_completed_ceremony(context, name, signatory_name):
    party_did = ContractService._local_peer_did(context)
    ContractService._create_contract_in_draft(context, name)
    _advance_to_approved(context, name)
    _run_full_ceremony(context, name, field_name=party_did, signatory_name=signatory_name)


@given('contract "{name}" has an AES-signed PDF via a completed ceremony for signatory "{signatory_name}"')
def step_given_aes_signed_pdf_via_ceremony(context, name, signatory_name):
    party_did = ContractService._local_peer_did(context)
    ContractService._create_contract_in_draft(context, name)
    _advance_to_approved(context, name)

    ceremony_id, presentation, subject_did = _run_full_ceremony(context, name, field_name=party_did, signatory_name=signatory_name)

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
        {
            "did": did,
            "signer_did": signer_did,
            "reason": "Post-sign C2PA lifecycle revocation",
        },
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

        # Build (but do not submit) the PID presentation the presentation-
        # completion steps need — scenarios that start a ceremony via this
        # low-level step complete it separately via those steps, which expect
        # context.pid_presentations[name] to already be populated (same
        # contract as _run_full_ceremony, minus the direct_post itself). Bound
        # to the ceremony's REAL request nonce, so the "correct" completion
        # step actually verifies and a deliberately wrong one is a genuine
        # negative test (ADR-20 nonce binding), not a wrong-secret stand-in.
        nonce = _fetch_pending_nonce(context, ceremony_id)
        given_name, family_name = field_name, "BDD-Testperson"
        presentation, _issuer_jwt, _disclosures, subject_did = _build_pid_presentation(
            given_name=given_name, family_name=family_name, aud=ceremony_aud(context), nonce=nonce
        )
        if not hasattr(context, "pid_presentations"):
            context.pid_presentations = {}
        context.pid_presentations[name] = {
            "presentation": presentation,
            "subject_did": subject_did,
            "given_name": given_name,
            "family_name": family_name,
            "field_name": field_name,
            "nonce": nonce,
        }


@when('I poll the signing ceremony status for contract "{name}"')
def step_when_poll_ceremony_status(context, name):
    ceremony_id = context.ceremony_ids[name]
    context.requests_response = get_with_headers(
        context,
        signature_request_by_id_url(context, ceremony_id),
        headers=AuthService.get_headers_for_roles(["Contract Signer"]),
    )


@when('the wallet presentation confirms the ceremony for contract "{name}" with the correct request nonce')
def step_when_presentation_confirms_correct_nonce(context, name):
    ceremony_id = context.ceremony_ids[name]
    presentation_info = context.pid_presentations[name]
    context.requests_response = _complete_ceremony_via_presentation(
        context,
        ceremony_id,
        presentation_info["presentation"],
        presentation_info["subject_did"],
        presentation_info["given_name"],
        presentation_info["family_name"],
        poa_organization=presentation_info["field_name"],
        nonce=presentation_info["nonce"],
    )


@when('a caller posts a ceremony presentation for contract "{name}" with an incorrect request nonce')
def step_when_presentation_wrong_nonce(context, name):
    ceremony_id = context.ceremony_ids[name]
    presentation_info = context.pid_presentations[name]
    # Re-bind the SAME PID claims to a nonce the ceremony never issued — the
    # presentation is otherwise perfectly valid, isolating the nonce check.
    wrong_nonce = str(uuid.uuid4())
    presentation, _issuer_jwt, _disclosures, subject_did = _build_pid_presentation(
        given_name=presentation_info["given_name"], family_name=presentation_info["family_name"],
        aud=ceremony_aud(context), nonce=wrong_nonce,
    )
    context.requests_response = _complete_ceremony_via_presentation(
        context,
        ceremony_id,
        presentation,
        subject_did,
        presentation_info["given_name"],
        presentation_info["family_name"],
        nonce=wrong_nonce,
    )


def _present_pid_x5c(context, name, field_name, *, trusted):
    """Start a fresh ceremony and present a PID whose issuer credential is
    x5c-signed instead of JWKS-trusted — what a real EUDI wallet's PID
    actually looks like. trusted=False signs with an unrelated cert never
    configured as a trust anchor."""
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    start_resp = _start_ceremony(context, name, field_name, signer_h)
    assert start_resp.status_code == 200, (
        f"POST /signature/request failed for contract '{name}': {start_resp.status_code} {start_resp.text}"
    )
    ceremony_id = start_resp.json().get("ceremony_id")
    assert ceremony_id, f"/signature/request response has no ceremony_id: {start_resp.text}"

    nonce = _fetch_pending_nonce(context, ceremony_id)
    given_name, family_name = field_name, "BDD-Testperson"
    presentation, subject_did = _build_pid_presentation_x5c(
        given_name=given_name, family_name=family_name, aud=ceremony_aud(context), nonce=nonce, trusted=trusted,
    )
    if not hasattr(context, "ceremony_ids"):
        context.ceremony_ids = {}
    context.ceremony_ids[name] = ceremony_id
    context.requests_response = _complete_ceremony_via_presentation(
        context, ceremony_id, presentation, subject_did, given_name, family_name,
        poa_organization=field_name, nonce=nonce,
    )


@when('the signer presents a PID signed with x5c by the trusted dev issuer for contract "{name}" field "{field_name}"')
def step_when_present_pid_x5c_trusted(context, name, field_name):
    _present_pid_x5c(context, name, field_name, trusted=True)


@when('the signer presents a PID signed with x5c by an untrusted issuer for contract "{name}" field "{field_name}"')
def step_when_present_pid_x5c_untrusted(context, name, field_name):
    _present_pid_x5c(context, name, field_name, trusted=False)


@then('the x5c PID presentation for contract "{name}" is accepted')
def step_then_pid_x5c_accepted(context, name):
    resp = context.requests_response
    assert resp.status_code == 200, f"expected the x5c PID presentation to be accepted, got {resp.status_code}: {resp.text}"


@then('the x5c PID presentation for contract "{name}" is rejected as untrusted')
def step_then_pid_x5c_untrusted_rejected(context, name):
    resp = context.requests_response
    assert resp.status_code >= 400, (
        f"expected the untrusted-issuer x5c PID presentation to be rejected, got {resp.status_code}: {resp.text}"
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


@then('the signed PDF for contract "{name}" contains a PAdES signature naming the signing party AcroForm field')
def step_then_pades_names_field(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    field_name = ContractService._local_peer_did(context)
    needle_ascii = f"/T ({field_name})".encode()
    needle_ascii_nospace = f"/T({field_name})".encode()
    needle_utf16 = _utf16be(field_name.encode())
    assert (
        needle_ascii in pdf_bytes or needle_ascii_nospace in pdf_bytes or needle_utf16 in pdf_bytes
    ), (
        f"Expected the signed PDF to name AcroForm field '/T' == the party DID '{field_name}' "
        "(the signer signs the seeded field: /T == signatoryName == the participating "
        "instance DID), found neither ASCII nor UTF-16BE form"
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


@then('the signer PID for contract "{name}" is NOT embedded in the signed PDF, only the pseudonymous binding')
def step_then_pid_not_embedded_only_binding(context, name):
    pdf_bytes = _pdf_bytes_for(context, name)
    info = context.pid_presentations[name]
    # Privacy (eIDAS/GDPR data-minimisation): neither the verbatim PID
    # presentation nor its disclosed personal attributes may appear in the
    # shared PDF. (given_name is the ceremony's signatory label == the AcroForm
    # field name, legitimately in the PDF, so the distinct family_name is the
    # unambiguous disclosure to prove absence of.)
    for secret in (info["presentation"], info["family_name"]):
        assert secret.encode("ascii") not in pdf_bytes, (
            f"Privacy leak: signer PID data ({secret!r}) appears in the shared signed PDF; "
            "the PID must never be embedded (only a pseudonymous binding may be)"
        )
    # The pseudonymous signer binding (holder DID) IS embedded under the signature.
    needle = info["subject_did"].encode("ascii")
    assert needle in pdf_bytes, (
        "Expected the pseudonymous holder DID (the signer binding) embedded in the signed PDF"
    )
    ranges = _last_byte_range(pdf_bytes)
    assert _offset_covered(pdf_bytes, needle, ranges), (
        "The embedded signer binding must fall inside the PAdES /ByteRange (embed-first-sign-second)"
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
    resp = context.requests_response
    assert resp.status_code == 200, f"POST /signature/request failed: {resp.status_code} {resp.text}"
    body = resp.json()
    for field in ("ceremony_id", "wallet_uri", "expires_at"):
        assert body.get(field), f"/signature/request response missing '{field}': {body}"


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


@then("the ceremony presentation is rejected for the incorrect request nonce")
def step_then_presentation_nonce_rejected(context):
    resp = context.requests_response
    assert resp.status_code == 400, (
        f"Expected the ceremony callback to reject a presentation bound to a nonce the ceremony "
        f"never issued, got {resp.status_code}: {resp.text}"
    )


@when('the signing ceremony deadline for contract "{name}" has already passed')
def step_when_ceremony_deadline_passed(context, name):
    # DB seam: the ceremony TTL is 15 minutes (command/ceremony.go
    # ceremonyTTL), unreachable as a real wait — backdate expires_at so the
    # presentation genuinely arrives after the issued deadline.
    ceremony_id = context.ceremony_ids[name]
    cursor = context.db.cursor()
    cursor.execute(
        "UPDATE signature_ceremonies SET expires_at = NOW() - INTERVAL '1 minute' WHERE id = %s",
        (ceremony_id,),
    )
    assert cursor.rowcount == 1, f"Expected to backdate exactly one ceremony row, got {cursor.rowcount}"
    context.db.commit()


@then("the ceremony presentation is rejected because the signing deadline has passed")
def step_then_presentation_deadline_rejected(context):
    resp = context.requests_response
    assert resp.status_code == 400, (
        f"Expected the ceremony callback to reject a presentation arriving after the ceremony "
        f"deadline (DCS-FR-SM-13), got {resp.status_code}: {resp.text}"
    )
    body = resp.text.lower()
    assert "deadline" in body or "expired" in body, (
        f"Expected the rejection to name the passed deadline, got: {resp.text}"
    )


@then('the signature validation findings for contract "{name}" cross-check the embedded signer binding')
def step_then_validate_crosschecks_signer_binding(context, name):
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


@then(
    'the signature view for contract "{name}" carries a JAdES signature '
    "that verifies over the contract JSON-LD"
)
def step_then_jades_verifies(context, name):
    """The completed ceremony must produce a JAdES (ETSI TS 119 182-1) compact
    JWS over the machine-readable JSON-LD alongside the visible PAdES on the
    PDF (DCS-FR-SM-02/-11). Assert it is present, well-formed (ES256 + critical
    sigT + x5c), cryptographically valid against its x5c leaf, and bound to
    this contract's DID.
    """
    import base64  # noqa: PLC0415
    import json  # noqa: PLC0415

    import requests as _requests  # noqa: PLC0415
    from cryptography.hazmat.primitives import hashes  # noqa: PLC0415
    from cryptography.hazmat.primitives.asymmetric import ec, utils  # noqa: PLC0415
    from cryptography.x509 import load_der_x509_certificate  # noqa: PLC0415

    from steps.support.api_client import signature_view_url  # noqa: PLC0415

    def b64url(seg):
        return base64.urlsafe_b64decode(seg + "=" * (-len(seg) % 4))

    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    view = _requests.get(
        signature_view_url(context), params={"did": did}, headers=manager_h,
        timeout=context.http_timeout_seconds,
    )
    assert view.status_code == 200, f"signature view failed: {view.status_code} {view.text}"
    signatures = view.json().get("signatures") or []
    assert signatures, f"Expected an applied signature on '{name}', got: {view.json()}"

    jws = signatures[0].get("jades")
    assert jws, f"Expected a JAdES signature on the signature view for '{name}', got: {signatures[0]}"
    parts = jws.split(".")
    assert len(parts) == 3, f"JAdES must be a compact JWS with 3 segments, got {len(parts)}"

    header = json.loads(b64url(parts[0]))
    assert header.get("alg") == "ES256", f"JAdES alg must be ES256, got {header.get('alg')}"
    assert "sigT" in header and header.get("sigT"), "JAdES must carry a sigT (claimed signing time)"
    assert "sigT" in (header.get("crit") or []), "JAdES sigT must be marked critical"
    x5c = header.get("x5c") or []
    assert x5c, "JAdES must carry an x5c certificate chain"

    payload = json.loads(b64url(parts[1]))
    assert payload.get("dcs:contractDid") == did, (
        f"JAdES payload must bind this contract's DID; got {payload.get('dcs:contractDid')!r}"
    )
    assert "dcs:contractDocument" in payload, "JAdES payload must carry the contract document"

    leaf = load_der_x509_certificate(base64.b64decode(x5c[0]))
    pub = leaf.public_key()
    assert isinstance(pub, ec.EllipticCurvePublicKey), "JAdES x5c leaf key must be EC"
    raw_sig = b64url(parts[2])
    assert len(raw_sig) == 64, f"JOSE ES256 signature must be 64 bytes, got {len(raw_sig)}"
    der_sig = utils.encode_dss_signature(
        int.from_bytes(raw_sig[:32], "big"), int.from_bytes(raw_sig[32:], "big")
    )
    signing_input = (parts[0] + "." + parts[1]).encode()
    # Raises InvalidSignature if the signature does not verify.
    pub.verify(der_sig, signing_input, ec.ECDSA(hashes.SHA256()))
