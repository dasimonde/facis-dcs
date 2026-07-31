"""Sign a prepared contract PDF as the test wallet + QTSP (ADR-12).

The DCS prepared the to-be-signed PDF and downloaded it; this plays the
signatory, driving the external SCA (an EU DSS) to sign the AcroForm field with
the signatory's own key. The DCS holds no signing key — it validates and records
whatever comes back.

Usage: python sign_prepared_pdf.py <prepared_pdf> <signed_out_pdf>
Env:   DSS_URL, E2E_SIGNATORY (cert subject / wallet key), E2E_SIGN_FIELD
"""

import os
import sys
from pathlib import Path

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
sys.path.insert(0, REPO_ROOT)

from steps.support.services.auth_service import AuthService  # noqa: E402


def main() -> None:
    prepared_path, out_path = sys.argv[1], sys.argv[2]
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.remote_signer import sign_pdf

    signed = sign_pdf(
        Path(prepared_path).read_bytes(),
        user=os.environ["E2E_SIGNATORY"],
        dss_url=os.getenv("DSS_URL", "http://localhost:18099"),
        field=os.getenv("E2E_SIGN_FIELD", ""),
        keys_dir=AuthService.resolve_wallet_keys_dir(),
    )
    Path(out_path).write_bytes(signed)


if __name__ == "__main__":
    main()
