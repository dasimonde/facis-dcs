from __future__ import annotations

import base64
import json
from pathlib import Path
from typing import Any

from dcs_wallet.sdjwt import merge_disclosed_claims, split_sd_jwt

CREDENTIAL_EXT = ".jwt"


def _wallet_root() -> Path:
    return Path(__file__).resolve().parent.parent


def credentials_dir() -> Path:
    return _wallet_root() / "credentials"


def credential_path(stem: str) -> Path:
    name = stem.removesuffix(CREDENTIAL_EXT)
    return credentials_dir() / f"{name}{CREDENTIAL_EXT}"


def decode_jwt_payload(token: str) -> dict[str, Any]:
    parts = token.strip().split(".")
    if len(parts) < 2:
        raise ValueError("credential is not a compact JWT")
    payload = parts[1]
    padding = "=" * (-len(payload) % 4)
    raw = json.loads(base64.urlsafe_b64decode(payload + padding).decode())
    if not isinstance(raw, dict):
        raise ValueError("credential JWT payload is not an object")
    return raw


def load_credential_sd_jwt(name: str) -> str:
    path = credential_path(name)
    if not path.is_file():
        raise FileNotFoundError(
            f"{path} not found — run: python3 testWallet/scripts/generate_keys.py --yes && python3 testWallet/scripts/issue_credentials.py"
        )
    token = path.read_text(encoding="utf-8").strip()
    if not token.startswith("eyJ"):
        raise ValueError(f"{path} must contain an SD-JWT credential")
    return token


def load_credential_jwt(name: str) -> str:
    issuer_jwt, _, _ = split_sd_jwt(load_credential_sd_jwt(name))
    return issuer_jwt


def load_credential_disclosures(name: str) -> list[str]:
    _, disclosures, _ = split_sd_jwt(load_credential_sd_jwt(name))
    return disclosures


def load_credential_claims(name: str) -> dict[str, Any]:
    raw = load_credential_sd_jwt(name)
    issuer_jwt, disclosures, _ = split_sd_jwt(raw)
    issuer_payload = decode_jwt_payload(issuer_jwt)
    return merge_disclosed_claims(issuer_payload, disclosures)
