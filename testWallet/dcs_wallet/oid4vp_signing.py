"""Headless wallet+QTSP stand-in for the OID4VP Document-Retrieval signing
ceremony (ADR-12).

Where ceremony_driver.py calls the DCS's prepare/submit endpoints directly, this
driver consumes the STANDARD OID4VP Document-Retrieval request object instead:
given the QR (the openid4vp:// deep link, or the request_uri out of it, the
DCS's publish step emits), it

    1. fetches the signed request object (JAR) from request_uri by the method the
       deep link asks for, and verifies its signature against the certificate
       chain in its own header,
    2. parses its claims (documentDigests, documentLocations, response_uri, nonce),
    3. fetches the to-be-signed document from documentLocations[].uri,
    4. signs it with the signatory's own key via the external SCA (an EU DSS),
    5. posts the signed document back to response_uri (direct_post,
       form-urlencoded documentWithSignature[]).

The DCS validates the returned signature identifies the signatory (sole control)
and finalizes the contract. Nothing DCS-specific crosses the wallet boundary but
the standard request object, so a real EUDI wallet swaps in by config.

    resp = sign_via_document_retrieval(
        request_uri="https://dcs/api/signature/request/<id>/object",
        user="SignerOne", dss_url="http://localhost:18099",
        keys_dir=Path("~/.dcs/wallet-keys"),
    )
"""

from __future__ import annotations

import base64
import json
import urllib.error
import urllib.request
import uuid
from pathlib import Path
from urllib.parse import urlencode, urlsplit, urlunsplit

from dcs_wallet.jades_signer import sign_jades_payload
from dcs_wallet.oid4vp_flow import (
    resolve_presentation_link,
    verify_authorization_request_jwt,
    wallet_metadata_json,
)
from dcs_wallet.remote_signer import sign_pdf


def _reorigin(url: str, origin_of: str) -> str:
    """Rewrite url to carry the scheme+host of origin_of. The publish step builds
    request_uri, documentLocations, and response_uri all from one public base, so
    they share an origin; pointing them at the origin the caller actually reached
    (the request_uri it was handed) keeps the document and callback reachable even
    when the DCS's advertised public host differs from the caller's route."""
    src, dst = urlsplit(url), urlsplit(origin_of)
    return urlunsplit((dst.scheme, dst.netloc, src.path, src.query, src.fragment))


def _get(url: str, accept: str = "*/*") -> bytes:
    req = urllib.request.Request(url, headers={"Accept": accept})
    with urllib.request.urlopen(req, timeout=120) as resp:
        return resp.read()


def _decode_jwt_claims(compact_jwt: str) -> dict:
    """Decode a compact JWS payload (claims) without verifying the signature.
    Only for inspecting a request object whose signature has already been
    checked, or one being examined rather than acted on."""
    parts = compact_jwt.strip().split(".")
    if len(parts) < 2:
        raise RuntimeError("request object is not a compact JWT")
    payload_b64 = parts[1]
    payload_b64 += "=" * (-len(payload_b64) % 4)
    return json.loads(base64.urlsafe_b64decode(payload_b64))


def _fetch_request_object(request_uri: str, method: str, client_id: str) -> dict:
    """Fetch the Document-Retrieval request object and return its VERIFIED
    claims. POST (what the ceremony's deep link asks for) carries a wallet_nonce
    the request object must echo — without that check the wallet cannot tell a
    fresh request object from a replayed one."""
    wallet_nonce: str | None = None
    if method == "post":
        wallet_nonce = str(uuid.uuid4())
        body = urlencode({
            "wallet_nonce": wallet_nonce,
            "wallet_metadata": wallet_metadata_json(client_id),
        }).encode()
        request = urllib.request.Request(
            request_uri,
            data=body,
            headers={
                "Accept": "application/oauth-authz-req+jwt",
                "Content-Type": "application/x-www-form-urlencoded",
            },
        )
    else:
        request = urllib.request.Request(
            request_uri, headers={"Accept": "application/oauth-authz-req+jwt"}
        )

    with urllib.request.urlopen(request, timeout=120) as resp:
        request_object = resp.read().decode().strip()

    return verify_authorization_request_jwt(request_object, expected_wallet_nonce=wallet_nonce)


def sign_via_document_retrieval(
    *,
    request_uri: str,
    user: str,
    dss_url: str,
    keys_dir: Path,
    field: str = "",
    given_name: str | None = None,
    family_name: str | None = None,
) -> dict:
    """Consume the OID4VP Document-Retrieval request object and return the DCS's
    callback response (JSON). request_uri is the openid4vp:// deep link the
    publish step emits, or the request_uri out of it. user is the sole-control
    token: the signing certificate's subject identifies the ceremony's signatory
    (ADR-20 cert↔PID name match) — GIVEN_NAME/SURNAME default to (user,
    "BDD-Testperson"), matching the ceremony's PID (see
    signer.ensure_signing_material); pass given_name/family_name explicitly to
    mint a deliberately mismatched cert.
    """
    link = resolve_presentation_link(request_uri)
    request_uri = link.request_uri
    claims = _fetch_request_object(request_uri, link.request_uri_method, link.client_id)

    locations = claims.get("documentLocations") or []
    response_uri = claims.get("response_uri")
    nonce = claims.get("nonce")
    if not locations:
        raise RuntimeError("request object carries no documentLocations")
    if not response_uri:
        raise RuntimeError("request object carries no response_uri")
    if not nonce:
        raise RuntimeError("request object carries no nonce")

    document_uri = _reorigin(locations[0]["uri"], request_uri)
    response_uri = _reorigin(response_uri, request_uri)

    to_be_signed = _get(document_uri, accept="application/pdf")
    signed_pdf = sign_pdf(
        to_be_signed, user=user, dss_url=dss_url, field=field, keys_dir=keys_dir,
        given_name=given_name, family_name=family_name,
    )

    # The EUDI walletdriven-signer direct_post: an application/x-www-form-urlencoded
    # body carrying the PAdES-signed document (enveloped in the PDF) in the
    # documentWithSignature[] list. The ceremony identity is the response_uri path,
    # so no state is echoed.
    form = {
        "documentWithSignature[0]": base64.b64encode(signed_pdf).decode(),
    }

    # The SECOND documentLocations entry, when offered (ADR-12), is the
    # canonical JSON-LD payload: sign it as a JAdES with the ceremony's nonce
    # bound into the protected header (ADR-20 item 1) and post it as
    # signatureObject[0] — a DETACHED signature value (CSC obtainSignedDoc's
    # own shape: documentWithSignature and signatureObject are independent
    # lists for the same document, not a positional split across documents).
    # The DCS's byte-pin check requires the RAW fetched bytes signed with no
    # re-serialization (see jades_signer.py).
    if len(locations) > 1:
        payload_uri = _reorigin(locations[1]["uri"], request_uri)
        payload_bytes = _get(payload_uri, accept="application/json")
        jades = sign_jades_payload(
            payload_bytes, user=user, keys_dir=keys_dir, nonce=nonce,
            given_name=given_name, family_name=family_name,
        )
        form["signatureObject[0]"] = jades

    body = urlencode(form).encode()
    post = urllib.request.Request(
        response_uri,
        data=body,
        headers={"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
    )
    try:
        with urllib.request.urlopen(post, timeout=120) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as exc:
        # HTTPError's default str() is just "HTTP Error 400: Bad Request" — the
        # DCS's typed error body (name/message, ADR-20) is what actually says
        # WHY, and is otherwise silently discarded.
        detail = exc.read().decode("utf-8", "replace")[:3000]
        raise RuntimeError(f"ceremony callback POST {response_uri} returned HTTP {exc.code}: {detail}") from exc


def _main() -> None:
    import argparse
    import os
    import sys

    parser = argparse.ArgumentParser(
        description="Sign a DCS contract via the OID4VP Document-Retrieval ceremony (fetch request object -> fetch document -> sign -> post to callback)."
    )
    parser.add_argument("--request-uri", required=True, help="openid4vp:// deep link (or the request_uri inside it) from the publish QR")
    parser.add_argument("--user", required=True, help="signatory name; the signing cert is 'CN=DCS Signatory <user>'")
    parser.add_argument("--field", default="", help="signature field for multi-signer contracts")
    parser.add_argument("--dss-url", default=os.getenv("DSS_URL", "http://localhost:18099"))
    parser.add_argument("--keys-dir", default=os.getenv("BDD_TEST_WALLET_KEYS_DIR", str(Path.home() / ".dcs" / "wallet-keys")))
    args = parser.parse_args()

    resp = sign_via_document_retrieval(
        request_uri=args.request_uri,
        user=args.user,
        field=args.field,
        dss_url=args.dss_url,
        keys_dir=Path(args.keys_dir),
    )
    print(json.dumps(resp, indent=2))
    if resp.get("status") != "SIGNED":
        sys.exit(f"contract not SIGNED: {resp}")


if __name__ == "__main__":
    _main()
