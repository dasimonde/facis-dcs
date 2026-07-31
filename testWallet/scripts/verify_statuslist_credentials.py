#!/usr/bin/env python3
"""Verify status list URIs in testWallet credentials match live service data."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.credential import decode_jwt_payload
from dcs_wallet.sdjwt import split_sd_jwt
from dcs_wallet.status_list import (
    bit_is_revoked,
    credential_status_from_claims,
    encoded_list_from_payload,
    fetch_status_list_payload,
)

CREDENTIAL_EXT = ".jwt"


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify wallet credential status vs statuslist-service")
    parser.add_argument(
        "--credentials-dir",
        type=Path,
        default=WALLET_ROOT / "credentials",
        help="directory containing *.jwt wallet credentials",
    )
    args = parser.parse_args()

    jwt_files = sorted(args.credentials_dir.glob(f"*{CREDENTIAL_EXT}"))
    if not jwt_files:
        print(f"no *.jwt in {args.credentials_dir}", file=sys.stderr)
        return 1

    list_cache: dict[str, str] = {}
    failures = 0
    checked = 0

    for path in jwt_files:
        raw = path.read_text(encoding="utf-8").strip()
        issuer_jwt, _, _ = split_sd_jwt(raw)
        claims = decode_jwt_payload(issuer_jwt)
        parsed = credential_status_from_claims(claims)
        if parsed is None:
            print(f"FAIL {path.name}: missing credentialStatus")
            failures += 1
            continue

        idx, uri = parsed
        try:
            if uri not in list_cache:
                payload = fetch_status_list_payload(uri)
                encoded = encoded_list_from_payload(payload)
                list_cache[uri] = encoded

            if bit_is_revoked(list_cache[uri], idx):
                state = "revoked"
            else:
                state = "active"
            print(f"OK   {path.name}: GET {uri} idx={idx} -> {state}")
            checked += 1
        except Exception as exc:  # noqa: BLE001 — CLI tool reports all failures
            print(f"FAIL {path.name}: {exc}", file=sys.stderr)
            failures += 1

    print(f"checked={checked} failures={failures}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
