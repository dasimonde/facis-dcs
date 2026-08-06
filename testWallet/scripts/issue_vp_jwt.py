#!/usr/bin/env python3
"""Issue an SD-JWT+KB vp_token to stdout for BDD experiments.

Builds a holder-bound dc+sd-jwt presentation (issuer JWT + disclosures + KB-JWT)
from the given organization and roles. Writes the compact token to stdout only.

Prerequisites:
  python3 testWallet/scripts/generate_keys.py --yes

Examples:
  python3 testWallet/scripts/issue_vp_jwt.py \\
    --organization "Acme Corp" \\
    --roles "Contract Manager,Auditor"

  python3 testWallet/scripts/issue_vp_jwt.py \\
    --organization "Acme Corp" \\
    --roles "Contract Manager" \\
    --nonce "$NONCE" \\
    --aud dcs-client
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.issuer import (
    DEFAULT_KB_AUD,
    DEFAULT_KB_NONCE,
    issue_access_credential,
)
from dcs_wallet.keys import load_json, private_key_material
from dcs_wallet.status_list import role_credential_index


def _load_wallet_key(keys_dir: Path) -> dict:
    wallet_path = keys_dir / "wallet.jwk"
    if not wallet_path.is_file():
        raise FileNotFoundError(
            f"missing wallet.jwk in {keys_dir} — "
            "run: python3 testWallet/scripts/generate_keys.py --yes"
        )
    return private_key_material(load_json(wallet_path))


def _parse_roles(raw: str) -> list[str]:
    raw = raw.strip()
    if not raw:
        raise ValueError("--roles must not be empty")

    if raw.startswith("["):
        roles = json.loads(raw)
        if not isinstance(roles, list) or not all(isinstance(role, str) for role in roles):
            raise ValueError("--roles JSON must be an array of strings")
    else:
        roles = [role.strip() for role in raw.split(",") if role.strip()]

    if not roles:
        raise ValueError("--roles must contain at least one role")
    return roles


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Issue an SD-JWT+KB vp_token to stdout for BDD testing",
    )
    parser.add_argument("--organization", required=True, help="organization claim value")
    parser.add_argument(
        "--roles",
        required=True,
        help='comma-separated roles or JSON array, e.g. "Contract Manager,Auditor"',
    )
    parser.add_argument("--keys-dir", type=Path, default=WALLET_ROOT / "keys")
    parser.add_argument(
        "--issuer-base",
        help="ORCE issuer this credential is issued as, serving /status-list/1 "
        "(default: ISSUER_BASE_URL or the dev NodePort)",
    )
    parser.add_argument(
        "--aud",
        default=DEFAULT_KB_AUD,
        help="KB-JWT aud / OpenID4VP verifier client_id (default: dcs-client)",
    )
    parser.add_argument(
        "--nonce",
        default=DEFAULT_KB_NONCE,
        help="KB-JWT nonce matching the authorization request",
    )
    args = parser.parse_args()

    organization = args.organization.strip()
    roles = _parse_roles(args.roles)
    token = issue_access_credential(
        organization=organization,
        roles=roles,
        wallet_private=_load_wallet_key(args.keys_dir),
        status_index=role_credential_index(organization=organization, roles=roles),
        issuer_base=args.issuer_base,
        aud=args.aud,
        nonce=args.nonce,
    )
    sys.stdout.write(token)
    if not token.endswith("\n"):
        sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, FileNotFoundError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
