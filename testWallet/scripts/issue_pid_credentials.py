#!/usr/bin/env python3
"""Self-issue PID credentials from *.pid.template.json using the SAME local
dev-issuer signing primitives dcs_wallet.issuer already uses for the Power of
Attorney credential (ADR-20).

The remote EUDIPLO playground API this used to call
(https://playground.eudi-wallet.org/api/issue) is broken and is removed as a
dependency: there is no live PID issuer for a dev/test environment to call. The
DEV-ONLY trust anchor is the SAME dev issuer key PoA credentials are trusted
under (testWallet/keys/issuer-dev.jwk, did:web:dev.example:issuer:poa in
backend/config/oid4vp/trust.dev.json) — never a production trust anchor; a real
deployment configures a real PID issuer registry instead (config, not code).

Outputs one file per template:
  <stem>.pid.template.json -> <stem>.pid.jwt
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.issuer import DEFAULT_ISSUER_DID, sign_credential_sd_jwt
from dcs_wallet.keys import (
    cnf_jwk,
    did_jwk_from_public_jwk,
    load_json,
    private_key_material,
    public_key_material,
    write_text,
)
from dcs_wallet.status_list import build_credential_status, fixture_index

DEFAULT_CREDENTIALS_DIR = WALLET_ROOT / "credentials"
PID_VCT = "urn:dcs:pid:demo:v1"
CREDENTIAL_IAT = 1719129600
CREDENTIAL_EXP = 2145916800


def issue_pid_credential_from_claims(
    claims: dict[str, Any],
    *,
    wallet_private_jwk: dict[str, Any],
    issuer_private_jwk: dict[str, Any],
    status_index: int,
    issuer_did: str = DEFAULT_ISSUER_DID,
    issuer_base: str | None = None,
) -> str:
    """Self-sign a PID SD-JWT VC: every claim in `claims` (given_name,
    family_name, birthdate, address, ...) is individually selectively
    disclosable, matching the real PID's per-attribute disclosure shape. No
    KB-JWT — this is the stored form; the wallet attaches key binding at
    presentation time (see _build_pid_presentation / oid4vp_signing.py).
    """
    given_name = str(claims.get("given_name") or "")
    family_name = str(claims.get("family_name") or "")
    if not given_name or not family_name:
        raise ValueError("template claims must include given_name and family_name")

    holder_public = public_key_material(wallet_private_jwk)
    subject_did = did_jwk_from_public_jwk(holder_public)

    visible_claims = {
        "iss": issuer_did,
        "sub": subject_did,
        "vct": PID_VCT,
        "iat": CREDENTIAL_IAT,
        "exp": CREDENTIAL_EXP,
        "cnf": {"jwk": cnf_jwk(holder_public)},
        "status": build_credential_status(index=status_index, issuer_base=issuer_base),
    }
    return sign_credential_sd_jwt(
        visible_claims=visible_claims,
        selective_claims=dict(claims),
        issuer_private=issuer_private_jwk,
    )


def _template_stem(path: Path) -> str:
    suffix = ".pid.template.json"
    if not path.name.endswith(suffix):
        raise ValueError(f"unexpected template name: {path.name}")
    return path.name[: -len(suffix)]


def issue_pid_credentials(
    *,
    credentials_dir: Path,
    wallet_private_jwk: dict[str, Any],
    wallet_public_jwk: dict | None = None,
    credential_names: list[str] | None = None,
    issuer_private_jwk: dict[str, Any] | None = None,
    issuer_did: str = DEFAULT_ISSUER_DID,
    issuer_base: str | None = None,
) -> list[Path]:
    del wallet_public_jwk  # kept for call-site compatibility; derived from wallet_private_jwk

    if issuer_private_jwk is None:
        issuer_private_jwk = private_key_material(load_json(credentials_dir.parent / "keys" / "issuer-dev.jwk"))

    if credential_names:
        templates = [credentials_dir / f"{name}.pid.template.json" for name in credential_names]
    else:
        templates = sorted(credentials_dir.glob("*.pid.template.json"))

    if not templates:
        raise FileNotFoundError(f"no *.pid.template.json files found in {credentials_dir}")

    output_paths: list[Path] = []
    for template_path in templates:
        if not template_path.is_file():
            raise FileNotFoundError(f"template not found: {template_path}")
        payload = json.loads(template_path.read_text(encoding="utf-8"))
        claims = payload.get("claims")
        if not isinstance(claims, dict):
            raise ValueError(f"{template_path} requires a claims object")
        stem = _template_stem(template_path)
        jwt_value = issue_pid_credential_from_claims(
            claims,
            wallet_private_jwk=wallet_private_jwk,
            issuer_private_jwk=issuer_private_jwk,
            status_index=fixture_index(f"{stem}.pid"),
            issuer_did=issuer_did,
            issuer_base=issuer_base,
        )
        out_path = credentials_dir / f"{stem}.pid.jwt"
        write_text(out_path, jwt_value)
        output_paths.append(out_path)
    return output_paths


def main() -> int:
    parser = argparse.ArgumentParser(description="Self-issue *.pid.jwt from *.pid.template.json (dev-only issuer, ADR-20)")
    parser.add_argument("--credentials-dir", type=Path, default=DEFAULT_CREDENTIALS_DIR)
    parser.add_argument("--credential", action="append", help="base credential stem to issue, e.g. johndoe")
    parser.add_argument("--issuer-did", default=DEFAULT_ISSUER_DID)
    parser.add_argument("--keys-dir", type=Path, default=WALLET_ROOT / "keys")
    parser.add_argument(
        "--issuer-base",
        help="ORCE issuer base URL serving /status-list/1 (default: ISSUER_BASE_URL or the dev NodePort)",
    )
    args = parser.parse_args()

    wallet_private_jwk = private_key_material(load_json(args.keys_dir / "wallet.jwk"))
    issuer_private_jwk = private_key_material(load_json(args.keys_dir / "issuer-dev.jwk"))

    for path in issue_pid_credentials(
        credentials_dir=args.credentials_dir,
        wallet_private_jwk=wallet_private_jwk,
        credential_names=args.credential,
        issuer_private_jwk=issuer_private_jwk,
        issuer_did=args.issuer_did,
        issuer_base=args.issuer_base,
    ):
        print(f"issued: {path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
