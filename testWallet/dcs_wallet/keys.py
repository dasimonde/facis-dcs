from __future__ import annotations

import base64
import json
import os
from pathlib import Path
from typing import Any

from cryptography.hazmat.primitives.asymmetric import ec

DEFAULT_EC_CRV = "P-256"
REQUIRED_EC_PUBLIC_FIELDS = ("kty", "x", "y")


def b64url_uint(value: int, length: int | None = None) -> str:
    if length is None:
        length = (value.bit_length() + 7) // 8 or 1
    raw = value.to_bytes(length, "big")
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def generate_ec_private_jwk() -> dict[str, Any]:
    # RFC 7518 §6.2: EC coordinates are FIXED-length (the curve's full octet
    # size, 32 for P-256) — a coordinate whose leading byte happens to be
    # zero must stay zero-padded or strict JWK consumers (e.g. PyJWT) reject
    # the key with "Coords should be 32 bytes for curve P-256".
    coord_len = (ec.SECP256R1().key_size + 7) // 8
    private_key = ec.generate_private_key(ec.SECP256R1())
    numbers = private_key.private_numbers()
    public = numbers.public_numbers
    return {
        "kty": "EC",
        "crv": "P-256",
        "x": b64url_uint(public.x, coord_len),
        "y": b64url_uint(public.y, coord_len),
        "d": b64url_uint(numbers.private_value, coord_len),
    }


def public_key_material(jwk: dict[str, Any]) -> dict[str, Any]:
    missing = [field for field in REQUIRED_EC_PUBLIC_FIELDS if not jwk.get(field)]
    if missing:
        raise ValueError(f"incomplete EC JWK: missing {', '.join(missing)}")

    crv = str(jwk.get("crv") or DEFAULT_EC_CRV)
    return {
        "kty": str(jwk["kty"]),
        "crv": crv,
        "x": str(jwk["x"]),
        "y": str(jwk["y"]),
    }


def private_key_material(jwk: dict[str, Any]) -> dict[str, Any]:
    """Return a private EC JWK preserving existing public metadata."""
    if not isinstance(jwk, dict):
        raise ValueError("JWK must be an object")

    public = public_key_material(jwk)
    if not jwk.get("d"):
        raise ValueError("incomplete EC private JWK: missing d")

    out: dict[str, Any] = dict(jwk)
    out["kty"] = public["kty"]
    out["crv"] = public["crv"]
    out["x"] = public["x"]
    out["y"] = public["y"]
    out["d"] = str(jwk["d"])
    return out


def public_jwk(private_or_public_jwk: dict[str, Any]) -> dict[str, Any]:
    """Return a public JWK preserving all existing public members."""
    if not isinstance(private_or_public_jwk, dict):
        raise ValueError("JWK must be an object")

    required = public_key_material(private_or_public_jwk)
    out: dict[str, Any] = {k: v for k, v in private_or_public_jwk.items() if k != "d"}
    out["kty"] = required["kty"]
    out["crv"] = required["crv"]
    out["x"] = required["x"]
    out["y"] = required["y"]

    if private_or_public_jwk.get("d") is not None:
        ops = out.get("key_ops")
        if isinstance(ops, list) and "sign" in ops and "verify" not in ops:
            out["key_ops"] = ["verify"]

    return out


def cnf_jwk(public_key: dict[str, Any]) -> dict[str, Any]:
    """CNF JWK keeps the current public representation."""
    return public_jwk(public_key)


def did_jwk_from_public_jwk(key: dict[str, Any]) -> str:
    """Build `did:jwk:` from the current public JWK representation."""
    public = public_jwk(key)
    payload = json.dumps(public, separators=(",", ":"), sort_keys=True, ensure_ascii=False).encode("utf-8")
    return "did:jwk:" + base64.urlsafe_b64encode(payload).rstrip(b"=").decode("ascii")


def jwk_from_did_jwk(did: str) -> dict[str, Any]:
    value = did.strip()
    if not value.startswith("did:jwk:"):
        raise ValueError("holder DID must use did:jwk: scheme")
    encoded = value[len("did:jwk:") :]
    if not encoded:
        raise ValueError("holder DID missing did:jwk payload")
    padding = "=" * (-len(encoded) % 4)
    raw = json.loads(base64.urlsafe_b64decode(encoded + padding).decode("utf-8"))
    if not isinstance(raw, dict):
        raise ValueError("did:jwk payload must be a JSON object")
    # Accept both minimal and full did:jwk payloads and preserve what was embedded.
    return public_jwk(raw)


def build_trust_json(
    *,
    issuer_public: dict[str, Any],
    issuer_dids: list[str],
    vcts: list[str],
    x5c_issuers: list[str] | None = None,
) -> dict[str, Any]:
    """Build a trust document in the schema LoadTrustConfig accepts (ADR-31).

    An entry states which purposes the issuer is granted, which organizations it
    may attest, and how its key is resolved. Emitting the older bare-jwks form
    produces a document the backend refuses outright.

    The dev fixture wildcards organizations because its issuers stand in for
    every party the BDD suite invents; a real deployment names them. peer_dynamic
    is off: `peer` is checked when a signing ceremony verifies a Power of
    Attorney, so trusting an unlisted issuer there is self-attestation.
    """
    issuer_key = public_key_material(issuer_public)
    issuers: dict[str, Any] = {}
    for did in issuer_dids:
        issuers[did] = {
            "purposes": ["login", "peer", "pid"],
            "organizations": ["*"],
            "mechanism": "jwks",
            "jwks": {"keys": [issuer_key]},
        }
    # An x5c issuer publishes its key through its certificate chain, verified
    # against the configured anchors, so it carries no jwks here. It is granted
    # `pid` alone: an identity document must not grant access, and it names no
    # organization because a PID attests a person rather than a party.
    for did in x5c_issuers or []:
        issuers[did] = {"purposes": ["pid"], "mechanism": "x5c"}
    return {
        "vcts": list(dict.fromkeys(vcts)),
        "peer_dynamic": False,
        "issuers": issuers,
    }


def load_json(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict):
        raise ValueError(f"{path} must contain a JSON object")
    return data


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        json.dump(data, fh, indent=2)
        fh.write("\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


def write_text(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        fh.write(content.rstrip() + "\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass
