from __future__ import annotations

import json
import time
from typing import Any

import jwt
from jwt.algorithms import ECAlgorithm

from dcs_wallet.credential import decode_jwt_payload, load_credential_claims, load_credential_sd_jwt
from dcs_wallet.sdjwt import (
    KB_JWT_TYP,
    DEFAULT_SD_ALG,
    KB_JWT_IAT_LEEWAY_SEC,
    decode_disclosure,
    disclosure_digest,
    join_sd_jwt,
    sd_hash,
    split_sd_jwt,
)

VP_FORMAT = "dc+sd-jwt"
_REQUIRED_EC_PUBLIC_FIELDS = ("kty", "crv", "x", "y")

_DEFAULT_DISCLOSURE_CLAIMS_BY_VCT: dict[str, list[str]] = {
    "urn:dcs:pid:demo:v1": ["given_name", "family_name", "birthdate", "status"],
    "urn:eudi:eaa:loyalty-card:1": ["given_name", "family_name", "birthdate", "status"],
    "urn:dcs:poa:v1": ["organization", "roles"],
}


def load_jwk(filename: str) -> dict[str, Any]:
    from pathlib import Path

    path = Path(__file__).resolve().parent.parent / "keys" / filename
    if not path.is_file():
        raise FileNotFoundError(
            f"{path} not found — run: python3 testWallet/scripts/generate_keys.py --yes && python3 testWallet/scripts/issue_credentials.py"
        )
    with path.open(encoding="utf-8") as fh:
        return json.load(fh)


def _public_cnf_jwk(jwk: dict[str, Any]) -> dict[str, Any]:
    return {k: jwk[k] for k in _REQUIRED_EC_PUBLIC_FIELDS if k in jwk}


def _assert_holder_binding_matches_credential(*, credential_claims: dict[str, Any], wallet_jwk: dict[str, Any]) -> None:
    cnf = credential_claims.get("cnf")
    if not isinstance(cnf, dict) or not isinstance(cnf.get("jwk"), dict):
        raise ValueError("credential is not holder-bound: missing cnf.jwk in issuer-signed SD-JWT")

    credential_public = _public_cnf_jwk(cnf["jwk"])
    wallet_public = _public_cnf_jwk(wallet_jwk)
    missing = [field for field in _REQUIRED_EC_PUBLIC_FIELDS if field not in credential_public]
    if missing:
        raise ValueError(f"credential cnf.jwk is incomplete: missing {', '.join(missing)}")
    if credential_public != wallet_public:
        raise ValueError("wallet.jwk does not match the credential cnf.jwk holder key")


def build_kb_jwt(*, issuer_jwt: str, disclosures: list[str], nonce: str, aud: str, wallet_jwk: dict[str, Any], sd_alg: str = DEFAULT_SD_ALG) -> str:
    if not nonce:
        raise ValueError("nonce is required for a standards-compliant KB-JWT")
    if not aud:
        raise ValueError("aud/client_id is required for a standards-compliant KB-JWT")

    kb_claims = {
        "iat": int(time.time()) - KB_JWT_IAT_LEEWAY_SEC,
        "aud": aud,
        "nonce": nonce,
        "sd_hash": sd_hash(issuer_jwt, disclosures, sd_alg=sd_alg),
    }
    headers: dict[str, Any] = {"typ": KB_JWT_TYP, "alg": "ES256"}
    return jwt.encode(
        kb_claims,
        ECAlgorithm.from_jwk(json.dumps(wallet_jwk)),
        algorithm="ES256",
        headers=headers,
    )


def _top_level_sd_disclosures(disclosures: list[str], *, issuer_payload: dict[str, Any], sd_alg: str) -> list[str]:
    raw_top_sd = issuer_payload.get("_sd")
    if not isinstance(raw_top_sd, list):
        return disclosures
    top_sd = {str(v) for v in raw_top_sd if isinstance(v, str)}
    if not top_sd:
        return disclosures

    selected: list[str] = []
    for disclosure in disclosures:
        try:
            digest = disclosure_digest(disclosure, sd_alg=sd_alg)
        except Exception:
            continue
        if digest in top_sd:
            selected.append(disclosure)
    return selected if selected else disclosures


def _filter_disclosures_by_claim_names(disclosures: list[str], claim_names: list[str]) -> list[str]:
    if not claim_names:
        return disclosures
    requested = {name.strip() for name in claim_names if name.strip()}
    if not requested:
        return disclosures

    filtered: list[str] = []
    for disclosure in disclosures:
        try:
            decoded = decode_disclosure(disclosure)
        except Exception:
            continue
        if len(decoded) != 3:
            continue
        claim_name = decoded[1]
        if isinstance(claim_name, str) and claim_name in requested:
            filtered.append(disclosure)
    return filtered if filtered else disclosures


def _filter_disclosures_by_requested_claims(disclosures: list[str], requested_claim_paths: list[list[str]] | None) -> list[str]:
    if not requested_claim_paths:
        return disclosures
    requested_top_level = [
        str(path[0]).strip()
        for path in requested_claim_paths
        if isinstance(path, list) and path and str(path[0]).strip()
    ]
    return _filter_disclosures_by_claim_names(disclosures, requested_top_level)


def _select_disclosures(
    disclosures: list[str],
    *,
    issuer_payload: dict[str, Any],
    sd_alg: str,
    requested_claim_paths: list[list[str]] | None,
) -> list[str]:
    disclosures = _top_level_sd_disclosures(disclosures, issuer_payload=issuer_payload, sd_alg=sd_alg)
    if requested_claim_paths:
        return _filter_disclosures_by_requested_claims(disclosures, requested_claim_paths)

    vct = str(issuer_payload.get("vct") or "")
    default_claims = _DEFAULT_DISCLOSURE_CLAIMS_BY_VCT.get(vct)
    if default_claims:
        return _filter_disclosures_by_claim_names(disclosures, default_claims)
    return disclosures


def build_vp_token(
    *,
    credential_name: str,
    nonce: str,
    client_id: str = "",
    requested_claim_paths: list[list[str]] | None = None,
) -> str:
    """Attach a fresh KB-JWT (aud/nonce from the OpenID4VP request) to a stored credential."""
    raw_credential = load_credential_sd_jwt(credential_name)
    issuer_jwt, disclosures, _stored_kb = split_sd_jwt(raw_credential)
    issuer_payload = decode_jwt_payload(issuer_jwt)
    sd_alg = str(issuer_payload.get("_sd_alg") or DEFAULT_SD_ALG)
    disclosures = _select_disclosures(
        disclosures,
        issuer_payload=issuer_payload,
        sd_alg=sd_alg,
        requested_claim_paths=requested_claim_paths,
    )

    credential_claims = load_credential_claims(credential_name)
    wallet_jwk = load_jwk("wallet.jwk")

    _assert_holder_binding_matches_credential(credential_claims=credential_claims, wallet_jwk=wallet_jwk)

    kb_jwt = build_kb_jwt(
        issuer_jwt=issuer_jwt,
        disclosures=disclosures,
        nonce=nonce,
        aud=client_id,
        wallet_jwk=wallet_jwk,
        sd_alg=sd_alg,
    )
    return join_sd_jwt(issuer_jwt, disclosures, kb_jwt)
