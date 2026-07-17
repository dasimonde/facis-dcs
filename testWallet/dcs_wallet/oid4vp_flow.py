"""Unified OpenID4VP presentation flow for DCS login and external verifiers (e.g. EUDIPLO)."""

from __future__ import annotations

import base64
import json
import os
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable
from urllib.parse import parse_qs, unquote, urlencode, urlparse

import jwt
from cryptography import x509
from jwt.algorithms import ECAlgorithm
from jwcrypto import jwe, jwk

from dcs_wallet.credential import CREDENTIAL_EXT, decode_jwt_payload, load_credential_claims
from dcs_wallet.presentation import build_vp_token
from dcs_wallet.sdjwt import split_sd_jwt

DCS_REQUEST_URI_MARKERS = (
    "/auth/pid/presentation/request/",
    "/auth/presentation/request/",
    "/signature/presentation/request/",
)

WALLET_VP_FORMATS_SUPPORTED = {
    "dc+sd-jwt": {
        "sd-jwt_alg_values": ["ES256"],
        "kb-jwt_alg_values": ["ES256"],
    },
    "jwt_vc_json": {"alg_values": ["ES256"]},
    "ldp_vc": {"proof_type_values": ["DataIntegrityProof"]},
}

LogFn = Callable[..., None]


@dataclass(frozen=True)
class PresentationLink:
    request_uri: str
    request_uri_method: str = "post"
    client_id: str = ""


@dataclass(frozen=True)
class CredentialQuery:
    query_id: str
    format: str
    vct_values: list[str]
    claim_paths: list[list[str]]


@dataclass(frozen=True)
class PresentationContext:
    link: PresentationLink
    finish_dcs_session: bool
    api_base: str = ""


def default_log(step: str, msg: str, **extra: object) -> None:
    suffix = ""
    if extra:
        suffix = " " + json.dumps(extra, default=str)
    print(f"[{step}] {msg}{suffix}")


def _http_success(status: int) -> bool:
    return 200 <= status < 300


def _request_uri_marker(path: str) -> str | None:
    for marker in DCS_REQUEST_URI_MARKERS:
        if marker in path:
            return marker
    return None


def is_dcs_request_uri(request_uri: str) -> bool:
    return _request_uri_marker(urlparse(request_uri).path) is not None


def api_base_from_request_uri(request_uri: str) -> str:
    parsed = urlparse(request_uri)
    marker = _request_uri_marker(parsed.path)
    if marker is None:
        raise ValueError(f"request_uri missing one of {DCS_REQUEST_URI_MARKERS}")
    idx = parsed.path.find(marker)
    return f"{parsed.scheme}://{parsed.netloc}{parsed.path[:idx]}"


def _query_from_openid4vp_url(value: str) -> str:
    parsed = urlparse(value)
    if parsed.scheme != "openid4vp":
        raise ValueError("expected openid4vp:// scheme")
    if parsed.query:
        return parsed.query
    if "?" in value:
        return value.split("?", 1)[1]
    raise ValueError("openid4vp:// missing query parameters")


def wallet_metadata_json(client_id: str = "") -> str:
    metadata: dict[str, Any] = {"vp_formats_supported": WALLET_VP_FORMATS_SUPPORTED}
    if client_id and not client_id.startswith("origin:"):
        metadata["request_object_signing_alg_values_supported"] = ["ES256"]
    return json.dumps(metadata, separators=(",", ":"))


def resolve_presentation_link(raw: str) -> PresentationLink:
    value = raw.strip().strip("'\"")
    if not value:
        raise ValueError("empty link")

    method = "post"
    client_id = ""
    if value.startswith("openid4vp:"):
        params = parse_qs(_query_from_openid4vp_url(value))
        refs = params.get("request_uri") or []
        if not refs or not refs[0].strip():
            raise ValueError("openid4vp:// missing request_uri parameter")
        value = unquote(refs[0].strip())
        methods = params.get("request_uri_method") or []
        if methods and methods[0].strip():
            method = methods[0].strip().lower()
        client_ids = params.get("client_id") or []
        if client_ids and client_ids[0].strip():
            client_id = unquote(client_ids[0].strip())

    parsed = urlparse(value)
    if parsed.scheme not in ("http", "https"):
        raise ValueError("request_uri must be an http(s) URL")
    if not parsed.netloc:
        raise ValueError("request_uri must have a host")
    if method not in ("get", "post"):
        raise ValueError(f"unsupported request_uri_method: {method!r}")

    return PresentationLink(request_uri=value, request_uri_method=method, client_id=client_id)


def presentation_context_from_link(link: PresentationLink) -> PresentationContext:
    finish_dcs = is_dcs_request_uri(link.request_uri)
    api_base = api_base_from_request_uri(link.request_uri) if finish_dcs else ""
    return PresentationContext(link=link, finish_dcs_session=finish_dcs, api_base=api_base)


def _authorization_request_verification_key(header: dict[str, Any]) -> Any:
    jwk_header = header.get("jwk")
    if isinstance(jwk_header, dict):
        return ECAlgorithm.from_jwk(json.dumps(jwk_header))

    x5c = header.get("x5c")
    if isinstance(x5c, list) and x5c:
        cert_der = base64.b64decode(str(x5c[0]))
        cert = x509.load_der_x509_certificate(cert_der)
        return cert.public_key()

    raise ValueError("authorization request JWT header missing jwk or x5c")


def verify_authorization_request_jwt(
    token: str,
    *,
    expected_wallet_nonce: str | None,
) -> dict[str, Any]:
    header = jwt.get_unverified_header(token)
    typ = str(header.get("typ") or "")
    if typ != "oauth-authz-req+jwt":
        raise ValueError(f"authorization request JWT typ must be oauth-authz-req+jwt, got {typ!r}")

    key = _authorization_request_verification_key(header)
    payload = jwt.decode(
        token,
        key,
        algorithms=["ES256"],
        options={"verify_aud": False, "require": ["exp"]},
    )

    if expected_wallet_nonce is not None:
        echoed = payload.get("wallet_nonce")
        if echoed is None:
            raise ValueError("authorization request JWT missing wallet_nonce echo")
        if str(echoed) != expected_wallet_nonce:
            raise ValueError("authorization request wallet_nonce echo mismatch")

    return payload


def _parse_credential_query(entry: dict[str, Any]) -> CredentialQuery:
    query_id = str(entry.get("id") or "").strip()
    if not query_id:
        raise ValueError("dcql credential query id is required")

    fmt = str(entry.get("format") or "").strip()
    vct_values: list[str] = []
    meta = entry.get("meta")
    if isinstance(meta, dict):
        raw_vct_values = meta.get("vct_values")
        if isinstance(raw_vct_values, list):
            vct_values = [str(v).strip() for v in raw_vct_values if str(v).strip()]

    claim_paths: list[list[str]] = []
    raw_claims = entry.get("claims")
    if isinstance(raw_claims, list):
        for raw_claim in raw_claims:
            if not isinstance(raw_claim, dict):
                continue
            raw_path = raw_claim.get("path")
            if isinstance(raw_path, list):
                path = [str(p).strip() for p in raw_path if str(p).strip()]
                if path:
                    claim_paths.append(path)

    return CredentialQuery(
        query_id=query_id,
        format=fmt,
        vct_values=vct_values,
        claim_paths=claim_paths,
    )


def resolve_dcql_credential_query(dcql_query: object) -> CredentialQuery:
    if not isinstance(dcql_query, dict):
        raise ValueError("dcql_query must be an object")

    credentials = dcql_query.get("credentials")
    if not isinstance(credentials, list) or not credentials:
        raise ValueError("dcql_query.credentials is required")

    by_id: dict[str, dict[str, Any]] = {}
    for entry in credentials:
        if isinstance(entry, dict) and entry.get("id"):
            by_id[str(entry["id"])] = entry

    credential_sets = dcql_query.get("credential_sets")
    if isinstance(credential_sets, list) and credential_sets:
        for cred_set in credential_sets:
            if not isinstance(cred_set, dict):
                continue
            options = cred_set.get("options")
            if not isinstance(options, list):
                continue
            for option in options:
                if not isinstance(option, list):
                    continue
                for query_id in option:
                    entry = by_id.get(str(query_id))
                    if entry is None:
                        continue
                    parsed = _parse_credential_query(entry)
                    if parsed.format == "dc+sd-jwt":
                        return parsed
        raise ValueError("no supported dc+sd-jwt credential query in credential_sets")

    if len(credentials) != 1:
        raise ValueError("wallet supports exactly one dcql credential query without credential_sets")

    entry = credentials[0]
    if not isinstance(entry, dict):
        raise ValueError("dcql credentials[0] must be an object")

    parsed = _parse_credential_query(entry)
    if parsed.format != "dc+sd-jwt":
        raise ValueError(f"unsupported dcql credential format: {parsed.format!r}")
    return parsed


def _credentials_dir() -> Path:
    return Path(__file__).resolve().parent.parent / "credentials"


def list_credential_stems(*, include_pid: bool = False) -> list[str]:
    stems: list[str] = []
    for path in sorted(_credentials_dir().glob(f"*{CREDENTIAL_EXT}")):
        if not include_pid and path.name.endswith(".pid.jwt"):
            continue
        stems.append(path.stem)
    return stems


def resolve_credential_name(
    credential_name: str | None,
    *,
    vct_values: list[str],
    log: LogFn = default_log,
) -> str | None:
    if credential_name:
        if vct_values:
            claims = load_credential_claims(credential_name)
            vct = str(claims.get("vct") or "")
            if vct not in vct_values:
                log("wallet", f"FAILED credential {credential_name} vct {vct!r} not in {vct_values}")
                return None
        return credential_name

    stems = list_credential_stems(include_pid=bool(vct_values))
    entries: list[tuple[str, dict[str, Any]]] = []
    for stem in stems:
        claims = load_credential_claims(stem)
        if vct_values and str(claims.get("vct") or "") not in vct_values:
            continue
        entries.append((stem, claims))

    if "urn:dcs:poa:v1" in vct_values:
        # Keep DCS role test credentials near the top for faster login testing.
        entries.sort(key=lambda item: (0 if item[0].startswith("test") else 1, item[0]))

    if not entries:
        log("wallet", f"FAILED no credentials matched vct_values={vct_values}")
        return None
    if len(entries) == 1:
        log("wallet", "using only available credential", credential=entries[0][0])
        return entries[0][0]

    print("\nSelect credential to present:")
    for index, (stem, claims) in enumerate(entries, start=1):
        roles_raw = claims.get("roles") or []
        roles = [r for r in roles_raw if isinstance(r, str)]
        if "urn:dcs:poa:v1" in vct_values:
            org = str(claims.get("organization") or claims.get("organization_name") or "?")
            print(f"  [{index}] {stem}")
            print(f"      organization: {org}")
            print(f"      roles ({len(roles)}): {', '.join(roles) if roles else '(none)'}")
            continue
        label = claims.get("vct") or claims.get("organization") or stem
        print(f"  [{index}] {stem} ({label})")
    while True:
        line = input(f"Enter 1–{len(entries)} [default 1]: ").strip()
        if not line:
            return entries[0][0]
        try:
            choice = int(line)
        except ValueError:
            print(f"Invalid input — enter a number from 1 to {len(entries)}.")
            continue
        if 1 <= choice <= len(entries):
            return entries[choice - 1][0]
        print(f"Invalid choice — enter a number from 1 to {len(entries)}.")


def fetch_authorization_request(
    session: Any,
    link: PresentationLink,
    *,
    log: LogFn = default_log,
) -> tuple[str, dict[str, Any]]:
    accept = "application/oauth-authz-req+jwt, application/jwt"
    wallet_nonce: str | None = None

    if link.request_uri_method == "get":
        log("fetch", "GET OpenID4VP request object", url=link.request_uri[:120])
        r = session.get(link.request_uri, timeout=30, accept=accept)
    else:
        wallet_nonce = str(uuid.uuid4())
        log("fetch", "POST OpenID4VP request object", url=link.request_uri[:120])
        r = session.post(
            link.request_uri,
            form_body={
                "wallet_nonce": wallet_nonce,
                "wallet_metadata": wallet_metadata_json(link.client_id),
            },
            timeout=30,
            accept=accept,
        )

    if not _http_success(r.status_code):
        log("fetch", "FAILED", status=r.status_code, body=r.text[:200])
        raise RuntimeError("authorization request fetch failed")

    body = r.text.strip()
    if not body.startswith("eyJ"):
        log("fetch", "FAILED", status=r.status_code, body=body[:200])
        raise RuntimeError("authorization request response is not a JWT")

    payload = verify_authorization_request_jwt(body, expected_wallet_nonce=wallet_nonce)
    return body, payload


def _select_encryption_jwk(client_metadata: object) -> tuple[dict[str, Any], str, str]:
    if not isinstance(client_metadata, dict):
        raise ValueError("direct_post.jwt requires client_metadata.jwks")

    jwks = client_metadata.get("jwks")
    if not isinstance(jwks, dict):
        raise ValueError("direct_post.jwt requires client_metadata.jwks")

    keys = jwks.get("keys")
    if not isinstance(keys, list) or not keys:
        raise ValueError("direct_post.jwt requires client_metadata.jwks.keys")

    allowed_algs = {
        "ECDH-ES",
        "ECDH-ES+A128KW",
        "ECDH-ES+A192KW",
        "ECDH-ES+A256KW",
        "RSA-OAEP",
        "RSA-OAEP-256",
    }
    enc_values = client_metadata.get("encrypted_response_enc_values_supported")
    enc = "A128GCM"
    if isinstance(enc_values, list) and enc_values:
        enc = str(enc_values[0])

    for key in keys:
        if not isinstance(key, dict):
            continue
        alg = str(key.get("alg") or "")
        if alg in allowed_algs and key.get("use") != "sig":
            return key, alg, enc

    raise ValueError("direct_post.jwt requires a verifier encryption JWK with alg")


def submit_presentation(
    session: Any,
    *,
    response_mode: str,
    response_uri: str,
    state: str,
    vp_token_object: str,
    client_metadata: object,
    log: LogFn = default_log,
) -> str | None:
    if response_mode == "direct_post":
        log("present", "direct_post to response_uri")
        r = session.post(
            response_uri,
            form_body={"state": state, "vp_token": vp_token_object},
            timeout=60,
        )
        if not _http_success(r.status_code):
            log("present", "FAILED", status=r.status_code, body=r.text[:300])
            raise RuntimeError("direct_post failed")
        try:
            body = r.json()
        except (json.JSONDecodeError, ValueError):
            return None
        if isinstance(body, dict):
            redirect = body.get("redirect_uri")
            return str(redirect) if redirect else None
        return None

    if response_mode == "direct_post.jwt":
        enc_jwk, alg, enc = _select_encryption_jwk(client_metadata)
        payload = json.dumps(
            {"vp_token": json.loads(vp_token_object), **({"state": state} if state else {})},
            separators=(",", ":"),
        )
        protected = {"alg": alg, "enc": enc}
        kid = enc_jwk.get("kid")
        if isinstance(kid, str) and kid:
            protected["kid"] = kid

        jwetoken = jwe.JWE(payload.encode("utf-8"), protected=json.dumps(protected))
        jwetoken.add_recipient(jwk.JWK.from_json(json.dumps(enc_jwk)))
        response_jwt = jwetoken.serialize(compact=True)

        log("present", "direct_post.jwt to response_uri")
        r = session.post(
            response_uri,
            form_body={"response": response_jwt},
            timeout=60,
        )
        if not _http_success(r.status_code):
            log("present", "FAILED", status=r.status_code, body=r.text[:300])
            raise RuntimeError("direct_post.jwt failed")
        return None

    raise ValueError(f"unsupported response_mode: {response_mode!r}")


def log_vp_token(vp_token: str, *, query_id: str, credential: str, aud: str, nonce: str) -> None:
    issuer_jwt, disclosures, kb_jwt = split_sd_jwt(vp_token)
    issuer_claims = decode_jwt_payload(issuer_jwt)
    kb_claims = decode_jwt_payload(kb_jwt) if kb_jwt else {}

    print()
    print("=== Verifiable Presentation (vp_token) ===")
    print(f"credential file : credentials/{credential}{CREDENTIAL_EXT}")
    print(f"presentation    : issuer-jwt + {len(disclosures)} disclosure(s) + kb-jwt")
    print(f"issuer          : {issuer_claims.get('iss', '?')}")
    print(f"holder (sub)    : {issuer_claims.get('sub', '?')}")
    print(f"vct             : {issuer_claims.get('vct', '?')}")
    print(f"KB aud          : {kb_claims.get('aud', aud)}")
    print(f"KB nonce        : {kb_claims.get('nonce', nonce)}")
    print()
    print(json.dumps({query_id: [vp_token]}, separators=(",", ":")))
    print("=== end vp_token ===")
    print()


def run_presentation_flow(
    session: Any,
    ctx: PresentationContext,
    *,
    credential_name: str | None,
    log: LogFn = default_log,
) -> int:
    link = ctx.link
    if ctx.finish_dcs_session:
        log("wallet", "present VP for DCS", api_base=ctx.api_base)
    else:
        log("wallet", "present VP for external verifier", request_uri_prefix=link.request_uri[:96])

    try:
        _, req_obj = fetch_authorization_request(session, link, log=log)
    except (RuntimeError, ValueError) as exc:
        log("fetch", "FAILED", error=str(exc))
        return 1

    response_mode = str(req_obj.get("response_mode") or "")
    if response_mode not in ("direct_post", "direct_post.jwt"):
        log("fetch", "FAILED", error=f"unsupported response_mode: {response_mode!r}")
        return 1

    response_uri = str(req_obj.get("response_uri") or "")
    state = str(req_obj.get("state") or "")
    nonce = str(req_obj.get("nonce") or "")
    client_id = str(req_obj.get("client_id") or link.client_id or "")
    if not response_uri or not nonce or not client_id:
        log("fetch", "FAILED", error="request object missing response_uri/nonce/client_id")
        return 1

    try:
        query = resolve_dcql_credential_query(req_obj.get("dcql_query"))
    except ValueError as exc:
        log("fetch", "FAILED", error=str(exc))
        return 1

    log(
        "fetch-ok",
        "request object verified",
        query_id=query.query_id,
        response_mode=response_mode,
        nonce_prefix=nonce[:12],
    )

    chosen = resolve_credential_name(credential_name, vct_values=query.vct_values, log=log)
    if not chosen:
        return 1

    try:
        vp_token = build_vp_token(
            credential_name=chosen,
            nonce=nonce,
            client_id=client_id,
            requested_claim_paths=query.claim_paths,
            top_level_sd_only=ctx.finish_dcs_session,
        )
    except Exception as exc:
        log("present", "FAILED to build VP", error=str(exc))
        return 1

    log_vp_token(vp_token, query_id=query.query_id, credential=chosen, aud=client_id, nonce=nonce)
    vp_token_object = json.dumps({query.query_id: [vp_token]}, separators=(",", ":"))

    try:
        redirect_uri = submit_presentation(
            session,
            response_mode=response_mode,
            response_uri=response_uri,
            state=state,
            vp_token_object=vp_token_object,
            client_metadata=req_obj.get("client_metadata"),
            log=log,
        )
    except (RuntimeError, ValueError) as exc:
        log("present", "FAILED", error=str(exc))
        return 1

    log("present-ok", "VP accepted")

    if not ctx.finish_dcs_session:
        log("wallet", "DONE — check verifier UI for result")
        return 0

    if os.environ.get("DCS_WALLET_FINISH_BROWSER", "1").lower() in ("0", "false", "no"):
        log("wallet", "DONE — VP posted; browser tab should redirect via polling")
        return 0

    if not redirect_uri:
        log("present", "FAILED", error="DCS session finish requires redirect_uri in direct_post response")
        return 1

    api = ctx.api_base
    target = urlparse(redirect_uri)
    callback_url = redirect_uri.replace(
        f"{target.scheme}://{target.netloc}",
        f"{urlparse(api).scheme}://{urlparse(api).netloc}",
        1,
    )
    log("oauth-callback", "GET /auth/callback", callback_prefix=callback_url[:100])
    r = session.get(callback_url, allow_redirects=False, timeout=30)
    if r.status_code != 302:
        log("oauth-callback", "FAILED", status=r.status_code, body=r.text[:200])
        return 1
    log("oauth-callback-ok", "callback OK", redirect=r.headers.get("location"))

    log("session", "POST /auth/refresh")
    r = session.post(f"{api}/auth/refresh", timeout=30)
    if r.status_code != 200:
        log("session", "FAILED", status=r.status_code, body=r.text[:200])
        return 1
    log("session", "refresh OK", access_prefix=str(r.json().get("access_token", ""))[:32])
    log("wallet", "DONE — full headless login path succeeded")
    return 0
