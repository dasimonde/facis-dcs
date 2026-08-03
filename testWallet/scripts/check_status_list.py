#!/usr/bin/env python3
"""Dev preflight: the issuer serves a status list this stack can verify.

  ISSUER_BASE_URL=http://localhost:30181 python3 testWallet/scripts/check_status_list.py

Nothing is created — the issuer serves /status-list/1 from boot and keeps its
bits on its own volume. What is checked is everything the backend will hold that
list to, because every one of those failures reaches a developer as the same
thing: a login refused with the list unread, which looks exactly like a revoked
credential. See dcs_wallet.status_list.check_status_list_ready.
"""

from __future__ import annotations

import sys
from pathlib import Path

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.status_list import check_status_list_ready


def main() -> int:
    print(check_status_list_ready())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
