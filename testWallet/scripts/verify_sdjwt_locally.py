#!/usr/bin/env python3
"""Verify a local SD-JWT or SD-JWT+KB using its x5c chain + trust.dev.json.

Debug verification order mirrors the DCS verifier's login path (ADR-35,
backend/internal/auth/oid4vp/sdjwt/keys.go):
  1. read the issuer JWT's x5c chain and take its leaf
  2. check the leaf carries a key the trust entry for payload.iss PINS, and
     that the leaf names that issuer
  3. verify issuer JWT signature with the leaf's key
  4. read payload.cnf.jwk
  5. verify KB-JWT signature with cnf.jwk
  6. verify KB-JWT sd_hash
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

import jwt
from jwt.algorithms import ECAlgorithm

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from dcs_wallet.sdjwt import KB_JWT_TYP, sd_hash, split_sd_jwt
from dcs_wallet.credential import decode_jwt_payload

ROOT = Path(__file__).resolve().parent.parent
_REQUIRED_EC_PUBLIC_FIELDS = ("kty", "crv", "x", "y")


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def public_key_material(jwk: dict[str, Any]) -> dict[str, Any]:
    missing = [name for name in _REQUIRED_EC_PUBLIC_FIELDS if not jwk.get(name)]
    if missing:
        raise ValueError(f"incomplete EC JWK: missing {', '.join(missing)}")
    return {name: jwk[name] for name in _REQUIRED_EC_PUBLIC_FIELDS}


def leaf_identifies_issuer(leaf, issuer: str) -> bool:
    """A chain says an anchor vouched for the leaf; this says whose leaf it is."""
    from cryptography import x509  # noqa: PLC0415
    from cryptography.x509.oid import ExtensionOID, NameOID  # noqa: PLC0415

    try:
        san = leaf.extensions.get_extension_for_oid(ExtensionOID.SUBJECT_ALTERNATIVE_NAME).value
        if issuer in [str(v) for v in san.get_values_for_type(x509.UniformResourceIdentifier)]:
            return True
    except x509.ExtensionNotFound:
        pass
    return issuer in [a.value for a in leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)]


def assert_leaf_is_pinned(*, x5c: list[str], trust: dict[str, Any], issuer: str):
    """Resolve the issuer's key the way a login verifier does: from the leaf the
    trust entry pins, with no certificate authority consulted (ADR-35)."""
    import base64  # noqa: PLC0415

    from cryptography import x509  # noqa: PLC0415
    from cryptography.hazmat.primitives import serialization  # noqa: PLC0415

    entry = trust.get("issuers", {}).get(issuer)
    if not isinstance(entry, dict):
        raise ValueError(f"no trust entry for issuer {issuer!r}")
    pins = entry.get("x5c_leaf_keys") or []

    leaf = x509.load_der_x509_certificate(base64.b64decode(x5c[0]))
    spki = leaf.public_key().public_bytes(
        serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo
    )
    if base64.b64encode(spki).decode() not in pins:
        raise ValueError("the leaf carries a key this issuer's entry does not pin")
    if not leaf_identifies_issuer(leaf, issuer):
        raise ValueError(f"the leaf does not identify issuer {issuer!r}")
    return leaf


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify SD-JWT / SD-JWT+KB locally")
    parser.add_argument("token_file", type=Path, help="file containing SD-JWT or SD-JWT+KB")
    parser.add_argument("--aud", default="dcs-client", help="expected KB-JWT audience")
    parser.add_argument("--nonce", default=None, help="optional expected KB-JWT nonce")
    parser.add_argument(
        "--trust-path",
        type=Path,
        # The document the backend actually loads (values.yaml oid4vp.trust.dataPath).
        default=ROOT.parent / "backend" / "config" / "oid4vp" / "trust.dev.json",
        help="trust list naming the issuers and the leaf keys they are pinned to",
    )
    args = parser.parse_args()

    token = args.token_file.read_text(encoding="utf-8").strip()
    issuer_jwt, disclosures, kb_jwt = split_sd_jwt(token)

    issuer_header = jwt.get_unverified_header(issuer_jwt)
    x5c = issuer_header.get("x5c")
    if not isinstance(x5c, list) or not x5c:
        raise ValueError("issuer JWT header carries no x5c chain")

    issuer_payload_unverified = decode_jwt_payload(issuer_jwt)
    issuer = issuer_payload_unverified.get("iss")
    if not isinstance(issuer, str) or not issuer:
        raise ValueError("issuer JWT payload is missing iss")

    trust = load_json(args.trust_path)
    leaf = assert_leaf_is_pinned(x5c=x5c, trust=trust, issuer=issuer)
    print("issuer leaf pinned and names its issuer: OK")

    issuer_payload = jwt.decode(
        issuer_jwt,
        leaf.public_key(),
        algorithms=["ES256"],
        options={"verify_exp": False, "verify_iat": False},
    )
    print("issuer signature: OK")

    cnf_jwk = issuer_payload.get("cnf", {}).get("jwk")
    if not isinstance(cnf_jwk, dict):
        raise ValueError("holder cnf.jwk is missing")
    holder_public = public_key_material(cnf_jwk)
    print("holder cnf.jwk:", json.dumps(holder_public, separators=(",", ":")))

    if kb_jwt is None:
        print("key binding: not present (plain issued SD-JWT ends with ~)")
        return 0

    kb_header = jwt.get_unverified_header(kb_jwt)
    if kb_header.get("typ") != KB_JWT_TYP:
        raise ValueError(f"KB-JWT typ must be {KB_JWT_TYP!r}, got {kb_header.get('typ')!r}")

    kb_payload = jwt.decode(
        kb_jwt,
        ECAlgorithm.from_jwk(json.dumps(holder_public)),
        algorithms=["ES256"],
        audience=args.aud,
        options={"verify_iat": False},
    )
    print("key binding signature: OK")

    expected = sd_hash(issuer_jwt, disclosures, sd_alg=str(issuer_payload.get("_sd_alg") or "sha-256"))
    if kb_payload.get("sd_hash") != expected:
        raise ValueError("KB-JWT sd_hash mismatch")
    print("key binding sd_hash: OK")

    if args.nonce is not None and kb_payload.get("nonce") != args.nonce:
        raise ValueError("KB-JWT nonce mismatch")
    print("key binding payload:", json.dumps(kb_payload, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
