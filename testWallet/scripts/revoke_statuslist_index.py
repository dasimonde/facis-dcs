#!/usr/bin/env python3
"""Dev helper: revoke a credential status-list entry by statusListIndex.

Uses the XFSC statuslist-service revoke endpoint (same as DCS C2PA publisher):
  POST /v1/tenants/{tenant}/status/{list}/revoke/{index}

Defaults match values.dev.yml (NodePort 30821, tenant default, list 1).

To look up statusListIndex manually, paste the SD-JWT (e.g. testWallet/credentials/test.jwt) into https://www.sdjwt.co/

Examples:
  python testWallet/scripts/revoke_statuslist_index.py 38021
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from urllib.parse import urlparse

from dcs_wallet.credential import decode_jwt_payload
from dcs_wallet.sdjwt import split_sd_jwt
from dcs_wallet.status_list import (
    DEFAULT_LIST_NUMBER,
    DEFAULT_SERVICE_BASE,
    DEFAULT_TENANT,
    _decompress_bitstring,
    bit_is_revoked,
    credential_status_from_claims,
    encoded_list_from_payload,
    fetch_status_list_payload,
    revoke_status_index,
    status_list_uri,
)


def _parse_list_uri(cred_uri: str) -> tuple[str, str, int]:
    """Return (service_base, tenant, list_number) from a statusListCredential URL."""
    parsed = urlparse(cred_uri.strip())
    parts = [p for p in parsed.path.split("/") if p]
    # v1 / tenants / {tenant} / status / {list}
    if len(parts) < 5 or parts[0] != "v1" or parts[1] != "tenants" or parts[3] != "status":
        raise ValueError(f"unsupported statusListCredential URI: {cred_uri}")
    service_base = f"{parsed.scheme}://{parsed.netloc}"
    return service_base, parts[2], int(parts[4], 10)


def _index_from_credential(path: Path) -> tuple[int, str]:
    raw = path.read_text(encoding="utf-8").strip()
    issuer_jwt, _, _ = split_sd_jwt(raw)
    claims = decode_jwt_payload(issuer_jwt)
    parsed = credential_status_from_claims(claims)
    if parsed is None:
        raise ValueError(f"{path.name}: missing credentialStatus / statusListIndex")
    return parsed


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Revoke a status-list index (dev)",
        epilog=(
            "Look up statusListIndex: paste the SD-JWT into https://www.sdjwt.co/ "
            "(e.g. testWallet/credentials/test.jwt) and read credentialStatus.statusListIndex."
        ),
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "index",
        nargs="?",
        help="statusListIndex to revoke (see https://www.sdjwt.co/ or use --credential)",
    )
    parser.add_argument(
        "--credential",
        type=Path,
        help="read statusListIndex and statusListCredential from a *.jwt file",
    )
    parser.add_argument(
        "--service-base",
        default=os.getenv("STATUSLIST_SERVICE_URL", DEFAULT_SERVICE_BASE),
        help=f"statuslist root URL (default: STATUSLIST_SERVICE_URL or {DEFAULT_SERVICE_BASE})",
    )
    parser.add_argument(
        "--tenant",
        default=os.getenv("STATUSLIST_TENANT_ID", DEFAULT_TENANT),
        help=f"tenant id (default: STATUSLIST_TENANT_ID or {DEFAULT_TENANT})",
    )
    parser.add_argument(
        "--list",
        type=int,
        default=DEFAULT_LIST_NUMBER,
        help=f"status list number (default: {DEFAULT_LIST_NUMBER})",
    )
    args = parser.parse_args()

    list_uri = status_list_uri(args.service_base, args.list, args.tenant)
    service_base = args.service_base
    tenant = args.tenant
    list_number = args.list

    if args.credential is not None:
        idx, cred_uri = _index_from_credential(args.credential)
        if args.index is not None and str(args.index) != str(idx):
            print(
                f"warning: CLI index {args.index} differs from credential index {idx}; using credential",
                file=sys.stderr,
            )
        service_base, tenant, list_number = _parse_list_uri(cred_uri)
        list_uri = cred_uri
    elif args.index is None:
        parser.error("provide index or --credential")
    else:
        idx = int(args.index, 10)

    print(f"POST revoke index={idx} tenant={tenant} list={list_number}")
    result = revoke_status_index(
        idx,
        service_base=service_base,
        tenant=tenant,
        list_number=list_number,
    )
    if result:
        print("response:", result)

    payload = fetch_status_list_payload(list_uri)
    encoded = encoded_list_from_payload(payload)
    bitstring = _decompress_bitstring(encoded)
    byte_idx = idx // 8
    byte_val = bitstring[byte_idx] if byte_idx < len(bitstring) else 0
    direct_byte = bitstring[idx] if idx < len(bitstring) else 0
    svc_revoked = bit_is_revoked(encoded, idx)
    nz = [(i, b) for i, b in enumerate(bitstring) if b]
    print(
        f"bitstring: {len(bitstring)} bytes, {len(nz)} non-zero "
        f"(sparse; scrolling hex mostly shows 00)"
    )
    print(
        f"index {idx} -> bit-packed byte[{byte_idx}] = 0x{byte_val:02x} "
        f"(NOT byte[{idx}] = 0x{direct_byte:02x})"
    )
    if byte_val == 0x20:
        print("  0x20 = LSB bit 5 set (revoked); do not expect 0x01 at this index")
    ones = [(i, b) for i, b in nz if b == 0x01]
    if ones:
        print("  0x01 bytes elsewhere (LSB bit 0):", [f"byte[{i}] -> index {i*8}" for i, _ in ones[:5]])
    print(f"verified (XFSC / LSB, matches DCS): index={idx} -> {'revoked' if svc_revoked else 'active'}")
    if not svc_revoked:
        print("  revoke may not have persisted — retry POST or check statuslist-service logs", file=sys.stderr)
    return 0 if svc_revoked else 1


if __name__ == "__main__":
    raise SystemExit(main())
