#!/usr/bin/env python3
"""Issue testWallet SD-JWT credentials for wallet storage (no KB-JWT).

KB-JWT with aud/nonce is added at presentation time (demo_wallet, issue_vp_jwt).

Entry point:
  python3 testWallet/scripts/issue_credentials.py
  python3 testWallet/scripts/issue_credentials.py --credential test
  python3 testWallet/scripts/issue_credentials.py --name test --organization "Acme Corp" --roles "Contract Manager,Contract Signer"
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

WALLET_ROOT = Path(__file__).resolve().parent.parent
SCRIPTS_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(WALLET_ROOT))
sys.path.insert(0, str(SCRIPTS_DIR))

from dcs_wallet.issuer import (
    CREDENTIAL_EXT,
    issue_credential_file,
    issue_stored_credential,
)
from dcs_wallet.keys import load_json, private_key_material, write_text
from dcs_wallet.status_list import fixture_index
from issue_pid_credentials import issue_pid_credentials


def _load_wallet_key(keys_dir: Path) -> dict:
    """The holder key these credentials are bound to.

    No issuer key is read here: a credential is issued as the stack's ORCE
    issuer, whose signing key and certificate chain are the committed fixture
    that issuer itself is handed (dcs_wallet.issuer_pki).
    """
    wallet_path = keys_dir / "wallet.jwk"
    if not wallet_path.is_file():
        raise FileNotFoundError(
            f"missing wallet.jwk — run: python3 testWallet/scripts/generate_keys.py --yes"
        )
    return private_key_material(load_json(wallet_path))


def _parse_roles(raw: str) -> list[str]:
    roles = [role.strip() for role in raw.split(",") if role.strip()]
    if not roles:
        raise ValueError("--roles must contain at least one role")
    return roles


def _is_role_template(path: Path) -> bool:
    return path.name.endswith(".template.json") and not path.name.endswith(".pid.template.json")


def main() -> int:
    parser = argparse.ArgumentParser(description="Issue testWallet SD-JWT credentials (issuer JWT + disclosures only)")
    parser.add_argument("--keys-dir", type=Path, default=WALLET_ROOT / "keys")
    parser.add_argument("--credentials-dir", type=Path, default=WALLET_ROOT / "credentials")

    source = parser.add_mutually_exclusive_group()
    source.add_argument("--all", action="store_true", help="issue one JWT for every *.template.json file")
    source.add_argument("--credential", action="append", help="issue selected credential template by stem, e.g. test")

    parser.add_argument("--name", default="test", help="output stem when using --organization/--roles")
    parser.add_argument(
        "--issuer-base",
        help="ORCE issuer base URL serving /status-list/1 (default: ISSUER_BASE_URL or the dev NodePort)",
    )
    parser.add_argument("--organization", help="organization for an on-the-fly credential")
    parser.add_argument("--roles", help="comma-separated roles for an on-the-fly credential")
    args = parser.parse_args()

    wallet_private = _load_wallet_key(args.keys_dir)

    if args.organization or args.roles:
        if not args.organization or not args.roles:
            raise ValueError("--organization and --roles must be used together")
        stem = args.name.removesuffix(CREDENTIAL_EXT)
        token = issue_stored_credential(
            organization=args.organization,
            roles=_parse_roles(args.roles),
            wallet_private=wallet_private,
            status_index=fixture_index(stem),
            issuer_base=args.issuer_base,
        )
        output_path = args.credentials_dir / f"{stem}{CREDENTIAL_EXT}"
        write_text(output_path, token)
        print(f"issued: {output_path}")
        return 0

    if args.credential:
        paths = [
            issue_credential_file(
                credentials_dir=args.credentials_dir,
                credential_name=name,
                wallet_private=wallet_private,
                issuer_base=args.issuer_base,
            )
            for name in args.credential
        ]
    else:
        paths = []
        for template_path in sorted(args.credentials_dir.glob("*.template.json")):
            if not _is_role_template(template_path):
                continue
            stem = template_path.name.replace(".template.json", "")
            paths.append(
                issue_credential_file(
                    credentials_dir=args.credentials_dir,
                    credential_name=stem,
                    wallet_private=wallet_private,
                    issuer_base=args.issuer_base,
                )
            )

    if not paths:
        raise FileNotFoundError(f"no role *.template.json files found in {args.credentials_dir}")
    for path in paths:
        print(f"issued: {path}")

    pid_names = args.credential if args.credential else None
    for path in issue_pid_credentials(
        credentials_dir=args.credentials_dir,
        wallet_private_jwk=wallet_private,
        credential_names=pid_names,
        issuer_base=args.issuer_base,
    ):
        print(f"issued: {path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
