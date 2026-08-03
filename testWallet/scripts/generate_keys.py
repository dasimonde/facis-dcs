#!/usr/bin/env python3
"""Generate or refresh testWallet issuer/wallet keys and trust.dev.json.

Entry point:
  python3 testWallet/scripts/generate_keys.py --yes
  python3 testWallet/scripts/generate_keys.py --regenerate --yes

This script only handles key material and trust config. It does not issue
credentials. Use scripts/issue_credentials.py for credentials/*.jwt.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path
from typing import Any

WALLET_ROOT = Path(__file__).resolve().parent.parent
REPO_ROOT = WALLET_ROOT.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.issuer import DEFAULT_ISSUER_DID, POA_VCT, TRUSTED_ISSUER_DIDS

# The demo PID issuer publishes its key through a certificate chain rather than a
# JWKS, so it needs an entry of its own -- regenerating without it produces a
# trust document under which the x5c PID scenario cannot pass.
PID_X5C_ISSUER_DID = "did:web:dev.example:issuer:pid-x5c"
PID_X5C_VCT = "urn:dcs:pid:demo:v1"
from dcs_wallet.keys import (
    build_trust_json,
    generate_ec_private_jwk,
    load_json,
    private_key_material,
    public_jwk,
    write_json,
)


def _read_existing_private_key(path: Path) -> dict[str, Any] | None:
    if not path.is_file():
        return None
    return private_key_material(load_json(path))


def _existing_material(keys_dir: Path, trust_path: Path) -> list[Path]:
    paths: list[Path] = []
    for name in ("issuer-dev.jwk", "wallet.jwk", "issuer-dev.public.jwk", "wallet.public.jwk"):
        path = keys_dir / name
        if path.is_file():
            paths.append(path)
    if trust_path.is_file():
        paths.append(trust_path)
    return paths


def _confirm(paths: list[Path], *, regenerate: bool) -> bool:
    print("Existing key/trust files found:")
    for path in paths:
        print(f"  {path}")
    print()
    if regenerate:
        print("This run will replace issuer-dev.jwk and wallet.jwk with new key pairs.")
    else:
        print("This run will keep existing private keys and rewrite public keys/trust.dev.json in clean format.")
    try:
        answer = input("Continue? [y/N]: ").strip().lower()
    except (EOFError, KeyboardInterrupt):
        print()
        return False
    return answer in {"y", "yes"}


def materialize_keys(*, keys_dir: Path, trust_path: Path, issuer_did: str, regenerate: bool) -> None:
    issuer_path = keys_dir / "issuer-dev.jwk"
    wallet_path = keys_dir / "wallet.jwk"

    issuer_private = None if regenerate else _read_existing_private_key(issuer_path)
    wallet_private = None if regenerate else _read_existing_private_key(wallet_path)

    if issuer_private is None:
        issuer_private = generate_ec_private_jwk()
    if wallet_private is None:
        wallet_private = generate_ec_private_jwk()

    issuer_public = public_jwk(issuer_private)
    wallet_public = public_jwk(wallet_private)
    trusted_dids = list(dict.fromkeys([issuer_did, *TRUSTED_ISSUER_DIDS]))
    trust = build_trust_json(
        issuer_public=issuer_public,
        issuer_dids=trusted_dids,
        vcts=[POA_VCT, PID_X5C_VCT],
        x5c_issuers=[PID_X5C_ISSUER_DID],
    )

    write_json(issuer_path, issuer_private)
    write_json(keys_dir / "issuer-dev.public.jwk", issuer_public)
    write_json(wallet_path, wallet_private)
    write_json(keys_dir / "wallet.public.jwk", wallet_public)
    write_json(trust_path, trust)


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate testWallet keys and trust.dev.json")
    parser.add_argument("--issuer-did", default=DEFAULT_ISSUER_DID)
    parser.add_argument("--keys-dir", type=Path, default=WALLET_ROOT / "keys")
    parser.add_argument(
        "--trust-path",
        type=Path,
        # The file the backend actually loads and the image bakes in
        # (deployment/docker/Dockerfile, values.yaml oid4vp.trust.dataPath).
        # Writing beside the wallet instead left the shipped fixture stale.
        default=REPO_ROOT / "backend/config/oid4vp/trust.dev.json",
    )
    parser.add_argument("--regenerate", action="store_true", help="replace issuer and wallet private keys")
    parser.add_argument("--yes", action="store_true", help="skip confirmation")
    args = parser.parse_args()

    existing = _existing_material(args.keys_dir, args.trust_path)
    if existing and not args.yes:
        if not sys.stdin.isatty():
            print("Existing key/trust files found; use --yes to continue non-interactively.", file=sys.stderr)
            return 1
        if not _confirm(existing, regenerate=args.regenerate):
            print("Aborted.")
            return 0

    materialize_keys(
        keys_dir=args.keys_dir,
        trust_path=args.trust_path,
        issuer_did=args.issuer_did,
        regenerate=args.regenerate,
    )
    print(f"keys: {args.keys_dir}")
    print(f"trust: {args.trust_path}")
    print(f"issuer: {args.issuer_did}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
