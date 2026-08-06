#!/usr/bin/env python3
"""BDD preflight: the issuer serves a status list this stack can verify.

Every credential a scenario presents names ISSUER_BASE_URL/status-list/1, which
the ORCE issuer signs and the DCS verifies against the committed anchors
(ADR-34). Nothing seeds it — the issuer serves it from boot — so this confirms
the four things that decide whether the backend can use it: the URL it answers
under, the root its chain ends at (by fingerprint), the issuer its leaf names,
and that it is long enough to hold the indices credentials are allocated.

Checked here rather than left to the suite because each of those failures
refuses a credential with the list unread, which is indistinguishable from the
revocation the suite deliberately tests.
"""

from __future__ import annotations

import sys
from pathlib import Path

SCRIPT_ROOT = Path(__file__).resolve().parent
PROJECT_ROOT = SCRIPT_ROOT.parents[2]
sys.path.insert(0, str(PROJECT_ROOT / "testWallet"))

from dcs_wallet.status_list import check_status_list_ready


def main() -> int:
    print(check_status_list_ready())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
