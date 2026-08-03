#!/usr/bin/env python3
"""Dev helper: revoke (or restore) a credential's bit on the issuer's status list.

The ORCE issuer owns the list it signs, so revocation goes through its own admin
endpoint (deployment/helm/charts/orce/flows-issuer/flow-admin.json):

  POST <issuer base>/admin/credentials/<idx>/revoke

Examples:
  python3 testWallet/scripts/revoke_statuslist_index.py --credential testWallet/credentials/test.jwt
  python3 testWallet/scripts/revoke_statuslist_index.py 1018 --restore
"""

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
    encoded_list_from_claims,
    fetch_status_list,
    revoke_status_index,
    status_list_uri,
    unrevoke_status_index,
)


def _index_from_credential(path: Path) -> tuple[int, str]:
    issuer_jwt, _, _ = split_sd_jwt(path.read_text(encoding="utf-8").strip())
    parsed = credential_status_from_claims(decode_jwt_payload(issuer_jwt))
    if parsed is None:
        raise ValueError(f"{path.name}: credential carries no status.status_list claim")
    return parsed


def main() -> int:
    parser = argparse.ArgumentParser(description="Revoke a status-list index on the issuer (dev)")
    parser.add_argument("index", nargs="?", type=int, help="status list index to revoke")
    parser.add_argument(
        "--credential",
        type=Path,
        help="read the index and the list URI out of a *.jwt credential instead",
    )
    parser.add_argument(
        "--issuer-base",
        help="ORCE issuer base URL (default: ISSUER_BASE_URL or the dev NodePort)",
    )
    parser.add_argument("--restore", action="store_true", help="clear the bit instead of setting it")
    args = parser.parse_args()

    if args.credential is not None:
        idx, list_uri = _index_from_credential(args.credential)
        if args.index is not None and args.index != idx:
            parser.error(f"index {args.index} contradicts the credential's own index {idx}")
    elif args.index is None:
        parser.error("provide an index or --credential")
    else:
        idx, list_uri = args.index, status_list_uri(args.issuer_base)

    action = unrevoke_status_index if args.restore else revoke_status_index
    action(idx, status_list_uri=list_uri)

    revoked = bit_is_revoked(encoded_list_from_claims(fetch_status_list(list_uri)), idx)
    print(f"{list_uri} idx={idx} -> {'revoked' if revoked else 'active'}")
    return 0 if revoked != args.restore else 1


if __name__ == "__main__":
    raise SystemExit(main())
