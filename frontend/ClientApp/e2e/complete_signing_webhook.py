"""Wallet leg of the signing ceremony: builds the PID and PoA SD-JWT VC + KB-JWT
presentation the way the real wallet does and delivers it over OpenID4VP
direct_post (JAR from request_uri → vp_token to response_uri). Self-contained —
it uses the same testWallet/dcs_wallet signing primitives AuthService uses for
the OID4VP login, without importing the behave step modules (which pull in the
bdd-executor runtime).

Usage: python3 complete_signing_webhook.py <openid4vp://... | request_uri>
Env:   ISSUER_BASE_URL, BDD_DCS_BASE_URL
"""

from __future__ import annotations

import json
import os
import sys
import time
from urllib.parse import parse_qs, urlparse

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
sys.path.insert(0, REPO_ROOT)

from steps.support import localhost_resolver  # noqa: E402

localhost_resolver.install()

import requests  # noqa: E402

from steps.support.api_client import did_document_url  # noqa: E402
from steps.support.services.auth_service import AuthCredentials, AuthService  # noqa: E402

PID_QUERY_ID = "eudi_pid_credential"
POA_QUERY_ID = "dcs_poa_credential"


def resolve_request_uri(pasted: str) -> str:
    pasted = pasted.strip()
    if not pasted:
        raise ValueError("empty presentation URL")
    if pasted.startswith("openid4vp:"):
        query = parse_qs(urlparse(pasted).query)
        request_uri = (query.get("request_uri") or [""])[0]
        if not request_uri:
            raise ValueError("openid4vp URL missing request_uri")
        return request_uri
    if pasted.startswith("http://") or pasted.startswith("https://"):
        return pasted
    raise ValueError(f"unsupported presentation URL: {pasted[:80]}")


def build_pid_presentation(*, given_name: str, family_name: str, aud: str, nonce: str):
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.issuer import DEFAULT_ISSUER_DID, sign_credential_sd_jwt, sign_key_binding_jwt
    from dcs_wallet.keys import cnf_jwk, did_jwk_from_public_jwk, public_jwk
    from dcs_wallet.sdjwt import join_sd_jwt, split_sd_jwt
    from dcs_wallet.status_list import build_credential_status, pid_credential_index

    keys = AuthService.load_wallet_keys()
    holder_key = keys.wallet_private
    holder_public = public_jwk(holder_key)
    subject_did = did_jwk_from_public_jwk(holder_public)

    # A real status claim (ADR-20): VerifyPID's status-list check is no longer
    # skipped, so a PID presentation with none would be rejected outright.
    now = int(time.time())
    issued = sign_credential_sd_jwt(
        visible_claims={
            "iss": DEFAULT_ISSUER_DID,
            "sub": subject_did,
            "vct": "urn:dcs:pid:demo:v1",
            "iat": now - 3600,
            "exp": now + 3600,
            "cnf": {"jwk": cnf_jwk(holder_public)},
            "status": build_credential_status(
                index=pid_credential_index(given_name=given_name, family_name=family_name),
            ),
        },
        selective_claims={"given_name": given_name, "family_name": family_name},
        issuer_private=keys.issuer_private,
    )
    issuer_jwt, disclosures, _ = split_sd_jwt(issued)
    kb_jwt = sign_key_binding_jwt(
        issuer_jwt=issuer_jwt,
        disclosures=disclosures,
        wallet_private=holder_key,
        aud=aud,
        nonce=nonce,
    )
    return join_sd_jwt(issuer_jwt, disclosures, kb_jwt)


def main() -> None:
    wallet_uri = sys.argv[1]
    base_url = os.environ["BDD_DCS_BASE_URL"].rstrip("/")
    # The organization the wallet's Power of Attorney authorizes it to act for.
    # One org runs one DCS, so that organization is the signing org's own DID,
    # resolved from its did:web document — the same public DID trust anchor the
    # testWallet self-issues under and every peer resolves against.
    poa_organization = requests.get(did_document_url(base_url), timeout=30).json()["id"]
    # given_name MUST match E2E_SIGNATORY passed to sign_prepared_pdf.py's
    # ensure_signing_material for the SAME ceremony — the DCS's cert-subject
    # to PID name-match gate (ADR-20) checks these two against each other.
    # Required, no silent default: a caller that forgets to set it would
    # otherwise get a WORKING PID presentation for the wrong identity instead
    # of a loud failure, and only find out from a cert_pid_mismatch two steps
    # later against whatever identity happened to be cached.
    given_name = os.environ["E2E_SIGNATORY"]
    family_name = os.getenv("E2E_SIGNATORY_FAMILY_NAME", "BDD-Testperson")

    request_uri = resolve_request_uri(wallet_uri)
    session = requests.Session()
    auth_request = AuthService.fetch_authorization_request(session, request_uri, timeout=60)

    pid_vp = build_pid_presentation(
        given_name=given_name,
        family_name=family_name,
        aud=auth_request.client_id,
        nonce=auth_request.nonce,
    )
    poa_vp = AuthService.build_vp_token(
        AuthCredentials(organization=poa_organization, roles=["Contract Signer"]),
        nonce=auth_request.nonce,
        client_id=auth_request.client_id,
    )
    vp_token = json.dumps(
        {PID_QUERY_ID: [pid_vp], POA_QUERY_ID: [poa_vp]},
        separators=(",", ":"),
    )
    response = session.post(
        auth_request.response_uri,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        data={"state": auth_request.state, "vp_token": vp_token},
        timeout=60,
    )
    if not response.ok:
        # Surface WHAT the ceremony refused (e.g. which PoA organization was
        # presented versus the party the ceremony is bound to); raise_for_status
        # alone reports only the code, which says nothing about the mismatch.
        raise SystemExit(
            f"direct_post {response.status_code} for {auth_request.response_uri}\n"
            f"  presented poa_organization={poa_organization!r}\n"
            f"  response: {response.text[:600]}"
        )
    print(response.status_code, response.text[:500])


if __name__ == "__main__":
    main()
