#!/usr/bin/env python3
"""Verify a local SD-JWT or SD-JWT+KB using header.jwk + trust.dev.json.

Debug verification order mirrors the intended DCS verifier logic:
  1. read issuer JWT header.jwk
  2. check header.jwk public key material matches the trusted issuer key for payload.iss
  3. verify issuer JWT signature with header.jwk
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


def trusted_issuer_keys(trust: dict[str, Any], issuer: str) -> list[dict[str, Any]]:
    entry = trust.get("issuers", {}).get(issuer)
    if not isinstance(entry, dict):
        return []
    keys = entry.get("jwks", {}).get("keys", [])
    if not isinstance(keys, list):
        return []
    return [key for key in keys if isinstance(key, dict)]


def assert_issuer_key_is_trusted(*, token_header_jwk: dict[str, Any], trust: dict[str, Any], issuer: str) -> None:
    header_key = public_key_material(token_header_jwk)
    for trusted_key in trusted_issuer_keys(trust, issuer):
        if public_key_material(trusted_key) == header_key:
            return
    raise ValueError("issuer header.jwk is not trusted for payload.iss")


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify SD-JWT / SD-JWT+KB locally")
    parser.add_argument("token_file", type=Path, help="file containing SD-JWT or SD-JWT+KB")
    parser.add_argument("--aud", default="dcs-client", help="expected KB-JWT audience")
    parser.add_argument("--nonce", default=None, help="optional expected KB-JWT nonce")
    parser.add_argument(
        "--trust-path",
        type=Path,
        default=ROOT / "trust.dev.json",
        help="trust list containing trusted issuer public keys",
    )
    args = parser.parse_args()

    token = args.token_file.read_text(encoding="utf-8").strip()
    issuer_jwt, disclosures, kb_jwt = split_sd_jwt(token)

    issuer_header = jwt.get_unverified_header(issuer_jwt)
    issuer_header_jwk = issuer_header.get("jwk")
    if not isinstance(issuer_header_jwk, dict):
        raise ValueError("issuer JWT header is missing jwk")

    issuer_payload_unverified = decode_jwt_payload(issuer_jwt)
    issuer = issuer_payload_unverified.get("iss")
    if not isinstance(issuer, str) or not issuer:
        raise ValueError("issuer JWT payload is missing iss")

    trust = load_json(args.trust_path)
    assert_issuer_key_is_trusted(token_header_jwk=issuer_header_jwk, trust=trust, issuer=issuer)
    print("issuer header.jwk trusted: OK")

    issuer_payload = jwt.decode(
        issuer_jwt,
        ECAlgorithm.from_jwk(json.dumps(public_key_material(issuer_header_jwk))),
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
