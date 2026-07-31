"""JAdES production for the OID4VP Document-Retrieval signing ceremony
(ADR-20 nonce binding, ADR-12 SM-02/-11).

The ceremony's request object offers TWO documents to sign over the same
content hash (ADR-12): the to-be-signed PDF (PAdES, produced by remote_signer.
sign_pdf via the external SCA) and the canonical machine-readable JSON-LD
payload (JAdES). The wallet signs the payload directly with the SAME
signatory key it uses for the PAdES (sole control: one key, one signatory,
both signature events) — this module does that.

The DCS's nonce-binding gate (backend/internal/signingmanagement/command/
apply.go, ADR-20 item 1) requires the ceremony's fresh request nonce
cryptographically bound INSIDE the JAdES's protected header, so a holder of
just the ceremony URL cannot forge an acceptance without the signatory's own
key: the DCS extracts the nonce claim only after DSS has already validated the
JWS signature over that same protected header, so the nonce cannot be
stripped or substituted without invalidating the signature.

The DCS's byte-pin check (ADR-20 item 2) compares the JWS payload segment,
decoded, byte-for-byte against the ceremony's PinnedPayload — so this signs
the RAW bytes fetched from the ceremony's payload document location directly,
with NO re-serialization round-trip (a JSON encode/decode pass would not
reproduce Go's exact byte-for-byte canonical form).

This carries x5c (the signatory's own leaf certificate, the same one signing
the PAdES) in the protected header so DSS's validateSignature can resolve the
signing certificate and the DCS's AssertValidAES sole-control gate sees a real
signatory certificate here too — not a bare unauthenticated JSON blob.
"""

from __future__ import annotations

import base64
import json
import time
from pathlib import Path
from typing import Any

from jwt.algorithms import ECAlgorithm

from dcs_wallet.keys import private_key_material
from dcs_wallet.signer import ensure_signing_material

JADES_TYP = "JOSE+JAdES"


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def _ec_private_key(jwk: dict[str, Any]) -> Any:
    return ECAlgorithm.from_jwk(json.dumps(private_key_material(jwk)))


def sign_jades_payload(
    payload: bytes,
    *,
    user: str,
    keys_dir: Path,
    nonce: str,
    given_name: str | None = None,
    family_name: str | None = None,
) -> str:
    """Sign payload (the ceremony's pinned canonical JSON-LD contract
    representation, byte-for-byte) as a compact JAdES-shaped JWS, with the
    ceremony's request nonce bound into the protected header — covered by the
    same ES256 signature DSS validates — using the SAME key/certificate that
    signs the PAdES for this ceremony (sole control, ADR-20). The JWS payload
    segment is the RAW input bytes, unmodified: no JSON re-serialization, so
    the DCS's byte-pin check sees exactly what it pinned at prepare.

    given_name/family_name must match whatever was passed to the PAdES's
    sign_pdf for the SAME user/ceremony — this is the SAME certificate.
    """
    signing_jwk, cert_der = ensure_signing_material(user, keys_dir, given_name=given_name, family_name=family_name)
    header = {
        "typ": JADES_TYP,
        "alg": "ES256",
        "x5c": [base64.b64encode(cert_der).decode()],
        "nonce": nonce,
        "iat": int(time.time()),
    }
    header_b64 = _b64url(json.dumps(header, separators=(",", ":")).encode("utf-8"))
    payload_b64 = _b64url(payload)
    signing_input = f"{header_b64}.{payload_b64}".encode("ascii")

    algorithm = ECAlgorithm(ECAlgorithm.SHA256)
    key = algorithm.prepare_key(_ec_private_key(signing_jwk))
    signature = algorithm.sign(signing_input, key)

    return f"{header_b64}.{payload_b64}.{_b64url(signature)}"
