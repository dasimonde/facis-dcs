"""Headless test wallet+QTSP stand-in: drives the DCS wallet-driven signing
ceremony end to end (ADR-12).

The DCS is the relying party — it prepares the to-be-signed document, and
validates and records whatever signed document comes back; it holds no signing
key. This stand-in plays the far side of that touch point exactly as a real
EUDI wallet + QTSP would: it fetches the prepared PDF, drives its external SCA
(an EU DSS) to sign the data-to-be-signed with the SIGNATORY's own key (sole
control), and submits the signed document back. The DCS treats it identically
to a real wallet; a real one swaps in by pointing at a real QTSP.

    resp = sign_contract_via_dcs(
        dcs_url="http://localhost:8991/api", token=jwt, contract_did=did,
        signer_did=signer, field="SignerOne", user="johndoe",
        dss_url="http://localhost:18099", keys_dir=Path("~/.dcs/wallet-keys"),
    )
"""

from __future__ import annotations

import base64
import json
import urllib.request
from pathlib import Path

from dcs_wallet.remote_signer import sign_pdf


def _dcs_post(dcs_url: str, path: str, token: str, body: dict) -> dict:
    req = urllib.request.Request(
        dcs_url.rstrip("/") + path,
        data=json.dumps(body).encode(),
        headers={
            "Content-Type": "application/json",
            "Accept": "application/json",
            "Authorization": f"Bearer {token}",
        },
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        return json.loads(resp.read())


def sign_contract_via_dcs(
    *,
    dcs_url: str,
    token: str,
    contract_did: str,
    signer_did: str,
    field: str,
    user: str,
    dss_url: str,
    keys_dir: Path,
    credential_type: str = "AES",
) -> dict:
    """Run the full ceremony: POST /signature/prepare to fetch the to-be-signed
    PDF, sign it with the signatory's own key via the external SCA, then POST
    /signature/submit. Returns the DCS submit response.

    user is the sole-control token: the signing certificate's subject is
    "CN=DCS Signatory <user>", which the DCS's validation gate requires to
    identify the ceremony's signatory.
    """
    prepared = _dcs_post(dcs_url, "/signature/prepare", token, {
        "did": contract_did,
        "signer_did": signer_did,
        "field_name": field,
        "credential_type": credential_type,
    })
    prepared_pdf = base64.b64decode(prepared["document"])

    signed_pdf = sign_pdf(
        prepared_pdf, user=user, dss_url=dss_url, field=field, keys_dir=keys_dir
    )

    return _dcs_post(dcs_url, "/signature/submit", token, {
        "did": contract_did,
        "signer_did": signer_did,
        "field_name": field,
        "credential_type": credential_type,
        "signed_pdf": base64.b64encode(signed_pdf).decode(),
        "jades_signature": "",
    })


def _main() -> None:
    import argparse
    import os
    import sys

    parser = argparse.ArgumentParser(
        description="Sign a DCS contract as the test wallet+QTSP (prepare -> sign -> submit)."
    )
    parser.add_argument("--dcs-url", default=os.getenv("DCS_URL", "http://localhost:8991/api"))
    parser.add_argument("--token", required=True, help="Contract Signer JWT")
    parser.add_argument("--contract-did", required=True)
    parser.add_argument("--signer-did", default="", help="signatory DID (single-signer flow resolves by ceremony when empty)")
    parser.add_argument("--field", default="", help="signature field for multi-signer contracts")
    parser.add_argument("--user", required=True, help="signatory name; the signing cert is 'CN=DCS Signatory <user>'")
    parser.add_argument("--dss-url", default=os.getenv("DSS_URL", "http://localhost:18099"))
    parser.add_argument("--keys-dir", default=os.getenv("BDD_TEST_WALLET_KEYS_DIR", str(Path.home() / ".dcs" / "wallet-keys")))
    args = parser.parse_args()

    resp = sign_contract_via_dcs(
        dcs_url=args.dcs_url,
        token=args.token,
        contract_did=args.contract_did,
        signer_did=args.signer_did,
        field=args.field,
        user=args.user,
        dss_url=args.dss_url,
        keys_dir=Path(args.keys_dir),
    )
    print(json.dumps(resp, indent=2))
    env = resp.get("signature_envelope") or {}
    if env.get("status") != "SIGNED":
        sys.exit(f"signature not SIGNED: {resp}")


if __name__ == "__main__":
    _main()
