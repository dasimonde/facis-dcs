from __future__ import annotations

import base64
import json
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import jwt
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.x509.oid import NameOID
from jwt.algorithms import ECAlgorithm

from dcs_wallet.keys import cnf_jwk, did_jwk_from_public_jwk, private_key_material, public_key_material, public_jwk, write_text
from dcs_wallet.sdjwt import KB_JWT_TYP, DEFAULT_SD_ALG, KB_JWT_IAT_LEEWAY_SEC, create_property_disclosure, join_sd_jwt, sd_hash, split_sd_jwt
from dcs_wallet.status_list import (
    DEFAULT_SERVICE_BASE,
    DEFAULT_TENANT,
    build_credential_status,
    build_pid_credential_status,
)

POA_VCT = "urn:dcs:poa:v1"
PID_VCT = "urn:eudi:pid:de:1"
CREDENTIAL_JWT_TYP = "dc+sd-jwt"
DEFAULT_ISSUER_DID = "did:web:dev.example:issuer:poa"
TRUSTED_ISSUER_DIDS = [
    "did:web:dev.example:issuer:poa",
    "did:web:dev2.example:issuer:poa",
]
CREDENTIAL_EXT = ".jwt"
CREDENTIAL_IAT = 1719129600
CREDENTIAL_EXP = 2145916800
DEFAULT_KB_AUD = "dcs-client"
DEFAULT_KB_NONCE = "test-nonce"


def _jwt_private_key(jwk: dict[str, Any]) -> Any:
    return ECAlgorithm.from_jwk(json.dumps(private_key_material(jwk)))


def mint_self_signed_x5c(issuer_private: dict[str, Any], *, common_name: str = "BDD EUDI PID Issuer") -> list[str]:
    """Mint a self-signed P-256 leaf cert (DER, std base64) for PID header.x5c.

    VerifyPID resolves the issuer key from x5c only (no JWKS trust lookup), so a
    leaf minted from the same issuer-dev key used for PoA signing is enough for BDD.
    """
    private_key = _jwt_private_key(issuer_private)
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, common_name)])
    now = datetime.now(timezone.utc)
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(days=1))
        .not_valid_after(now + timedelta(days=3650))
        .sign(private_key, hashes.SHA256())
    )
    der = cert.public_bytes(serialization.Encoding.DER)
    return [base64.b64encode(der).decode("ascii")]


def _decode_jwt_payload_unverified(token: str) -> dict[str, Any]:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    data = json.loads(base64.urlsafe_b64decode(payload).decode("utf-8"))
    if not isinstance(data, dict):
        raise ValueError("JWT payload must be a JSON object")
    return data


def sign_credential_sd_jwt(
    *,
    visible_claims: dict[str, Any],
    selective_claims: dict[str, Any],
    issuer_private: dict[str, Any],
) -> str:
    ADD_HEADER_KID = True
    ADD_HEADER_JWK = True

    disclosures: list[str] = []
    sd_digests: list[str] = []
    for claim_name, claim_value in selective_claims.items():
        encoded, digest = create_property_disclosure(claim_name, claim_value)
        disclosures.append(encoded)
        sd_digests.append(digest)

    payload = {**visible_claims, "_sd": sd_digests, "_sd_alg": DEFAULT_SD_ALG}

    issuer_public = public_key_material(issuer_private)
    headers: dict[str, Any] = {
        "typ": CREDENTIAL_JWT_TYP,
        "alg": "ES256",
    }
    if ADD_HEADER_JWK:
        headers["jwk"] = issuer_public
    if ADD_HEADER_KID:
        headers["kid"] = did_jwk_from_public_jwk(issuer_public)

    issuer_jwt = jwt.encode(
        payload,
        _jwt_private_key(issuer_private),
        algorithm="ES256",
        headers=headers,
    )
    return join_sd_jwt(issuer_jwt, disclosures)


def sign_pid_credential_sd_jwt(
    *,
    visible_claims: dict[str, Any],
    selective_claims: dict[str, Any],
    issuer_private: dict[str, Any],
    x5c: list[str] | None = None,
) -> str:
    """Sign a PID SD-JWT with header.x5c."""
    disclosures: list[str] = []
    sd_digests: list[str] = []
    for claim_name, claim_value in selective_claims.items():
        encoded, digest = create_property_disclosure(claim_name, claim_value)
        disclosures.append(encoded)
        sd_digests.append(digest)

    payload = {**visible_claims, "_sd": sd_digests, "_sd_alg": DEFAULT_SD_ALG}
    headers: dict[str, Any] = {
        "typ": CREDENTIAL_JWT_TYP,
        "alg": "ES256",
        "x5c": x5c or mint_self_signed_x5c(issuer_private),
    }
    issuer_jwt = jwt.encode(
        payload,
        _jwt_private_key(issuer_private),
        algorithm="ES256",
        headers=headers,
    )
    return join_sd_jwt(issuer_jwt, disclosures)


def sign_key_binding_jwt(
    *,
    issuer_jwt: str,
    disclosures: list[str],
    wallet_private: dict[str, Any],
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
    sd_alg: str = DEFAULT_SD_ALG,
) -> str:
    if not aud:
        raise ValueError("aud is required for KB-JWT")
    if not nonce:
        raise ValueError("nonce is required for KB-JWT")

    kb_claims = {
        "iat": int(time.time()) - KB_JWT_IAT_LEEWAY_SEC,
        "aud": aud,
        "nonce": nonce,
        "sd_hash": sd_hash(issuer_jwt, disclosures, sd_alg=sd_alg),
    }
    return jwt.encode(
        kb_claims,
        _jwt_private_key(wallet_private),
        algorithm="ES256",
        headers={"typ": KB_JWT_TYP, "alg": "ES256"},
    )


def attach_key_binding(
    *,
    issued_sd_jwt: str,
    wallet_private: dict[str, Any],
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
) -> str:
    issuer_jwt, disclosures, _old_kb = split_sd_jwt(issued_sd_jwt)
    issuer_payload = _decode_jwt_payload_unverified(issuer_jwt)
    kb_jwt = sign_key_binding_jwt(
        issuer_jwt=issuer_jwt,
        disclosures=disclosures,
        wallet_private=wallet_private,
        aud=aud,
        nonce=nonce,
        sd_alg=str(issuer_payload.get("_sd_alg") or DEFAULT_SD_ALG),
    )
    return join_sd_jwt(issuer_jwt, disclosures, kb_jwt)


def issue_stored_credential(
    *,
    organization: str,
    roles: list[str],
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    credential_status: dict[str, Any] | None = None,
    statuslist_service_base: str | None = None,
    statuslist_tenant: str | None = None,
) -> str:
    """Issuer-signed SD-JWT for wallet storage (no KB-JWT; aud/nonce belong to presentation)."""
    holder_public = public_jwk(wallet_private)
    holder_did_value = did_jwk_from_public_jwk(holder_public)
    holder_jwk = cnf_jwk(holder_public)
    status_base = (
        statuslist_service_base.strip()
        if statuslist_service_base and statuslist_service_base.strip()
        else DEFAULT_SERVICE_BASE
    )
    status_tenant = (
        statuslist_tenant.strip()
        if statuslist_tenant and statuslist_tenant.strip()
        else DEFAULT_TENANT
    )
    visible_claims = {
        "iss": issuer_did,
        "sub": holder_did_value,
        "vct": POA_VCT,
        "iat": CREDENTIAL_IAT,
        "exp": CREDENTIAL_EXP,
        "cnf": {"jwk": holder_jwk},
        "status": credential_status
        or build_credential_status(
            sub=holder_did_value,
            organization=organization,
            roles=roles,
            service_base=status_base,
            tenant=status_tenant,
        ),
    }
    selective_claims = {
        "organization": organization,
        "roles": roles,
    }
    return sign_credential_sd_jwt(
        visible_claims=visible_claims,
        selective_claims=selective_claims,
        issuer_private=issuer_private,
    )


def issue_access_credential(
    *,
    organization: str,
    roles: list[str],
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    aud: str = DEFAULT_KB_AUD,
    nonce: str = DEFAULT_KB_NONCE,
) -> str:
    """Build SD-JWT+KB vp_token for an OpenID4VP request (presentation-time)."""
    issued_sd_jwt = issue_stored_credential(
        organization=organization,
        roles=roles,
        issuer_private=issuer_private,
        wallet_private=wallet_private,
        issuer_did=issuer_did,
    )
    return attach_key_binding(
        issued_sd_jwt=issued_sd_jwt,
        wallet_private=wallet_private,
        aud=aud,
        nonce=nonce,
    )


def issue_stored_pid_credential(
    *,
    given_name: str,
    family_name: str,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    credential_status: dict[str, Any] | None = None,
    statuslist_service_base: str | None = None,
    statuslist_tenant: str | None = None,
    x5c: list[str] | None = None,
) -> str:
    """Issuer-signed PID SD-JWT for wallet storage.

    Mirrors issue_stored_credential for the PID path used by VerifyPID / ceremony
    callback..
    """
    holder_public = public_jwk(wallet_private)
    holder_did_value = did_jwk_from_public_jwk(holder_public)
    holder_jwk = cnf_jwk(holder_public)
    status_base = (
        statuslist_service_base.strip()
        if statuslist_service_base and statuslist_service_base.strip()
        else DEFAULT_SERVICE_BASE
    )
    status_tenant = (
        statuslist_tenant.strip()
        if statuslist_tenant and statuslist_tenant.strip()
        else DEFAULT_TENANT
    )
    visible_claims = {
        "iss": issuer_did,
        "sub": holder_did_value,
        "vct": PID_VCT,
        "iat": CREDENTIAL_IAT,
        "exp": CREDENTIAL_EXP,
        "cnf": {"jwk": holder_jwk},
        "status": credential_status
        or build_pid_credential_status(
            sub=holder_did_value,
            given_name=given_name,
            family_name=family_name,
            service_base=status_base,
            tenant=status_tenant,
        ),
    }
    selective_claims = {
        "given_name": given_name,
        "family_name": family_name,
    }
    return sign_pid_credential_sd_jwt(
        visible_claims=visible_claims,
        selective_claims=selective_claims,
        issuer_private=issuer_private,
        x5c=x5c,
    )


def issue_pid_presentation(
    *,
    given_name: str,
    family_name: str,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    aud: str,
    nonce: str,
    issuer_did: str = DEFAULT_ISSUER_DID,
    statuslist_service_base: str | None = None,
    statuslist_tenant: str | None = None,
) -> str:
    """Build SD-JWT+KB PID vp_token for an OpenID4VP presentation request."""
    issued_sd_jwt = issue_stored_pid_credential(
        given_name=given_name,
        family_name=family_name,
        issuer_private=issuer_private,
        wallet_private=wallet_private,
        issuer_did=issuer_did,
        statuslist_service_base=statuslist_service_base,
        statuslist_tenant=statuslist_tenant,
    )
    return attach_key_binding(
        issued_sd_jwt=issued_sd_jwt,
        wallet_private=wallet_private,
        aud=aud,
        nonce=nonce,
    )


def issue_credential_from_template(
    *,
    template_path: Path,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    credential_status: dict[str, Any] | None = None,
) -> str:
    with template_path.open(encoding="utf-8") as fh:
        template_data = json.load(fh)
    organization = template_data.get("organization")
    roles = template_data.get("roles")
    if not isinstance(organization, str) or not organization:
        raise ValueError(f"{template_path} must contain a non-empty organization")
    if not isinstance(roles, list) or not all(isinstance(role, str) for role in roles):
        raise ValueError(f"{template_path} must contain roles as a list of strings")
    return issue_stored_credential(
        organization=organization,
        roles=roles,
        issuer_private=issuer_private,
        wallet_private=wallet_private,
        issuer_did=issuer_did,
        credential_status=credential_status,
    )


def issue_credential_file(
    *,
    credentials_dir: Path,
    credential_name: str,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    credential_status: dict[str, Any] | None = None,
) -> Path:
    stem = credential_name.removesuffix(CREDENTIAL_EXT).removesuffix(".template")
    template_path = credentials_dir / f"{stem}.template.json"
    if not template_path.is_file():
        raise FileNotFoundError(f"template not found: {template_path}")
    token = issue_credential_from_template(
        template_path=template_path,
        issuer_private=issuer_private,
        wallet_private=wallet_private,
        issuer_did=issuer_did,
        credential_status=credential_status,
    )
    output_path = credentials_dir / f"{stem}{CREDENTIAL_EXT}"
    write_text(output_path, token)
    return output_path


def issue_all_template_files(
    *,
    credentials_dir: Path,
    issuer_private: dict[str, Any],
    wallet_private: dict[str, Any],
    issuer_did: str = DEFAULT_ISSUER_DID,
    credential_status: dict[str, Any] | None = None,
) -> list[Path]:
    paths: list[Path] = []
    for template_path in sorted(credentials_dir.glob("*.template.json")):
        stem = template_path.name.replace(".template.json", "")
        paths.append(
            issue_credential_file(
                credentials_dir=credentials_dir,
                credential_name=stem,
                issuer_private=issuer_private,
                wallet_private=wallet_private,
                issuer_did=issuer_did,
                credential_status=credential_status,
            )
        )
    return paths
