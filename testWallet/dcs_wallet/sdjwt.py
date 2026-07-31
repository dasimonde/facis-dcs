from __future__ import annotations

import base64
import hashlib
import json
import secrets
from typing import Any

KB_JWT_TYP = "kb+jwt"
DEFAULT_SD_ALG = "sha-256"
KB_JWT_IAT_LEEWAY_SEC = 60
SUPPORTED_SD_ALGS = {DEFAULT_SD_ALG: hashlib.sha256}


def random_salt() -> str:
    """Return a cryptographically random salt for SD-JWT Disclosures."""
    return secrets.token_hex(16)


def b64url_encode(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def b64url_decode(value: str) -> bytes:
    padding = "=" * (-len(value) % 4)
    return base64.urlsafe_b64decode(value + padding)


def disclosure_json(claim_name: str, claim_value: Any) -> str:
    return json.dumps([random_salt(), claim_name, claim_value], separators=(",", ":"), ensure_ascii=False)


def disclosure_digest(encoded_disclosure: str, *, sd_alg: str = DEFAULT_SD_ALG) -> str:
    """Hash the base64url-encoded Disclosure as required by SD-JWT."""
    hasher = SUPPORTED_SD_ALGS.get(sd_alg)
    if hasher is None:
        raise ValueError(f"unsupported SD-JWT hash algorithm: {sd_alg}")
    digest = hasher(encoded_disclosure.encode("ascii")).digest()
    return b64url_encode(digest)


def encode_disclosure(disclosure: str) -> str:
    """Base64url-encode the UTF-8 JSON array; this string is the Disclosure."""
    return b64url_encode(disclosure.encode("utf-8"))


def decode_disclosure(encoded: str) -> list[Any]:
    raw = b64url_decode(encoded)
    value = json.loads(raw.decode("utf-8"))
    if not isinstance(value, list):
        raise ValueError("disclosure must decode to a JSON array")
    return value


def create_property_disclosure(claim_name: str, claim_value: Any, *, sd_alg: str = DEFAULT_SD_ALG) -> tuple[str, str]:
    """Return (base64url Disclosure, _sd digest)."""
    disclosure = disclosure_json(claim_name, claim_value)
    encoded = encode_disclosure(disclosure)
    return encoded, disclosure_digest(encoded, sd_alg=sd_alg)


def split_sd_jwt(token: str) -> tuple[str, list[str], str | None]:
    """Split SD-JWT into issuer JWT, disclosures, and optional KB-JWT."""
    raw_parts = [part.strip() for part in token.strip().split("~")]
    if not raw_parts or not raw_parts[0]:
        raise ValueError("sd-jwt is empty")
    if not raw_parts[0].startswith("eyJ"):
        raise ValueError("sd-jwt must start with issuer JWT")

    issuer_jwt = raw_parts[0]
    remainder = raw_parts[1:]
    kb_jwt: str | None = None

    if remainder and remainder[-1] == "":
        remainder = remainder[:-1]
    elif remainder and _looks_like_jwt(remainder[-1]):
        kb_jwt = remainder[-1]
        remainder = remainder[:-1]

    disclosures = [part for part in remainder if part]
    return issuer_jwt, disclosures, kb_jwt


def _looks_like_jwt(value: str) -> bool:
    return value.count(".") >= 2 and value.startswith("eyJ")


def join_sd_jwt(issuer_jwt: str, disclosures: list[str], kb_jwt: str | None = None) -> str:
    """Serialize an SD-JWT or SD-JWT+KB using the compact combined format.

    Plain SD-JWT:
        <issuer-jwt>~<disclosure-1>~...~<disclosure-n>~

    SD-JWT+KB:
        <issuer-jwt>~<disclosure-1>~...~<disclosure-n>~<kb-jwt>
    """
    if kb_jwt is None:
        return issuer_jwt + "~" + "~".join(disclosures) + ("~" if disclosures else "")
    return issuer_jwt + "~" + "~".join(disclosures) + ("~" if disclosures else "") + kb_jwt


def presentation_body_for_sd_hash(issuer_jwt: str, disclosures: list[str]) -> str:
    """Return the US-ASCII string hashed into the KB-JWT sd_hash claim."""
    body = issuer_jwt + "~"
    for disclosure in disclosures:
        body += disclosure + "~"
    return body


def sd_hash(issuer_jwt: str, disclosures: list[str], *, sd_alg: str = DEFAULT_SD_ALG) -> str:
    hasher = SUPPORTED_SD_ALGS.get(sd_alg)
    if hasher is None:
        raise ValueError(f"unsupported SD-JWT hash algorithm: {sd_alg}")
    body = presentation_body_for_sd_hash(issuer_jwt, disclosures)
    digest = hasher(body.encode("ascii")).digest()
    return b64url_encode(digest)


def merge_disclosed_claims(issuer_payload: dict[str, Any], disclosures: list[str]) -> dict[str, Any]:
    claims = dict(issuer_payload)
    claims.pop("_sd", None)
    claims.pop("_sd_alg", None)
    for encoded in disclosures:
        arr = decode_disclosure(encoded)
        if len(arr) != 3:
            raise ValueError("property disclosure must be a three-element array")
        claim_name = arr[1]
        if not isinstance(claim_name, str) or not claim_name:
            raise ValueError("disclosure claim name must be a non-empty string")
        claims[claim_name] = arr[2]
    return claims
