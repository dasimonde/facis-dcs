"""Unified OpenID4VP presentation flow for DCS login and external verifiers (e.g. EUDIPLO)."""

from __future__ import annotations

import base64
import json
import os
import re
import time
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable
from urllib.parse import parse_qs, quote, unquote, urlparse

import jwt
from cryptography import x509
from jwt.algorithms import ECAlgorithm
from jwcrypto import jwe, jwk

from dcs_wallet.credential import CREDENTIAL_EXT, decode_jwt_payload, load_credential_claims
from dcs_wallet.presentation import build_vp_token
from dcs_wallet.sdjwt import merge_disclosed_claims, split_sd_jwt

DCS_REQUEST_URI_MARKERS = (
    "/auth/pid/presentation/request/",
    "/auth/presentation/request/",
)

SIGNING_CEREMONY_REQUEST_RE = re.compile(
    r"/signature/request/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}",
    re.I,
)

PID_VCT_VALUES = {"urn:dcs:pid:demo:v1", "urn:eudi:eaa:loyalty-card:1"}
POA_VCT = "urn:dcs:poa:v1"

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
    if SIGNING_CEREMONY_REQUEST_RE.search(path):
        return "/signature/request/"
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


X509_SAN_DNS_PREFIX = "x509_san_dns:"


def _leaf_certificate(header: dict[str, Any]) -> x509.Certificate | None:
    x5c = header.get("x5c")
    if isinstance(x5c, list) and x5c:
        return x509.load_der_x509_certificate(base64.b64decode(str(x5c[0])))
    return None


def _authorization_request_verification_key(header: dict[str, Any]) -> Any:
    jwk_header = header.get("jwk")
    if isinstance(jwk_header, dict):
        return ECAlgorithm.from_jwk(json.dumps(jwk_header))

    leaf = _leaf_certificate(header)
    if leaf is not None:
        return leaf.public_key()

    raise ValueError("authorization request JWT header missing jwk or x5c")


def _assert_x509_san_dns_client_id(header: dict[str, Any], payload: dict[str, Any]) -> None:
    """Resolve an x509_san_dns client identifier the way a wallet does: the
    identifier carries its scheme as a prefix (OpenID4VP 1.0 / HAIP, the ARF
    profile) and claims a DNS name, which the certificate that signed the request
    object must carry as a SAN — otherwise the verifier is naming a host it cannot
    prove. Requests naming themselves by any other prefix are left to whatever
    that prefix's own rules are."""
    client_id = str(payload.get("client_id") or "")
    if not client_id.startswith(X509_SAN_DNS_PREFIX):
        return

    issuer = payload.get("iss")
    if issuer is not None and str(issuer) != client_id:
        raise ValueError(f"authorization request iss {issuer!r} does not match client_id {client_id!r}")

    leaf = _leaf_certificate(header)
    if leaf is None:
        raise ValueError("x509_san_dns client_id requires an x5c chain in the request object header")

    try:
        sans = leaf.extensions.get_extension_for_class(
            x509.SubjectAlternativeName
        ).value.get_values_for_type(x509.DNSName)
    except x509.ExtensionNotFound:
        sans = []

    dns_name = client_id[len(X509_SAN_DNS_PREFIX):]
    if dns_name not in sans:
        raise ValueError(
            f"client_id claims DNS name {dns_name!r} but the request object's signing "
            f"certificate carries SAN dNSNames {sans}"
        )


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

    _assert_x509_san_dns_client_id(header, payload)

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


def resolve_dcql_credential_queries(dcql_query: object) -> list[CredentialQuery]:
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
                queries: list[CredentialQuery] = []
                for query_id in option:
                    entry = by_id.get(str(query_id))
                    if entry is None:
                        continue
                    parsed = _parse_credential_query(entry)
                    if parsed.format == "dc+sd-jwt":
                        queries.append(parsed)
                if queries:
                    return queries
        raise ValueError("no supported dc+sd-jwt credential query in credential_sets")

    if len(credentials) != 1:
        raise ValueError("wallet supports exactly one dcql credential query without credential_sets")

    entry = credentials[0]
    if not isinstance(entry, dict):
        raise ValueError("dcql credentials[0] must be an object")

    parsed = _parse_credential_query(entry)
    if parsed.format != "dc+sd-jwt":
        raise ValueError(f"unsupported dcql credential format: {parsed.format!r}")
    return [parsed]


def resolve_dcql_credential_query(dcql_query: object) -> CredentialQuery:
    return resolve_dcql_credential_queries(dcql_query)[0]


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
    """Pick the credential to answer ONE dcql credential query with.

    credential_name may name several credentials, comma-separated: a request
    object can carry more than one credential query (the signing ceremony asks
    for a PID and a Power of Attorney in the same request), and a wallet answers
    each from the credential whose vct the query accepts. Naming one credential
    for a two-query request would fail whichever query it does not match.
    """
    candidates = [name.strip() for name in str(credential_name or "").split(",") if name.strip()]
    if candidates:
        if not vct_values:
            return candidates[0]
        for name in candidates:
            try:
                claims = load_credential_claims(name)
            except (FileNotFoundError, ValueError) as exc:
                log("wallet", f"skipping credential {name}: {exc}")
                continue
            if str(claims.get("vct") or "") in vct_values:
                return name
        log("wallet", f"FAILED no credential of {candidates} carries a vct in {vct_values}")
        return None

    stems = list_credential_stems(include_pid=bool(vct_values))
    entries: list[tuple[str, dict[str, Any]]] = []
    for stem in stems:
        claims = load_credential_claims(stem)
        if vct_values and str(claims.get("vct") or "") not in vct_values:
            continue
        entries.append((stem, claims))

    if POA_VCT in vct_values:
        # Keep DCS role test credentials near the top for faster login testing.
        entries.sort(key=lambda item: (0 if item[0].startswith("test") else 1, item[0]))

    if not entries:
        log("wallet", f"FAILED no credentials matched vct_values={vct_values}")
        return None
    if len(entries) == 1:
        log("wallet", "using only available credential", credential=entries[0][0])
        return entries[0][0]

    if any(v in PID_VCT_VALUES for v in vct_values):
        print("\nSelect PID credential to present:")
    elif POA_VCT in vct_values:
        print("\nSelect PoA credential to present:")
    else:
        print("\nSelect credential to present:")
    for index, (stem, claims) in enumerate(entries, start=1):
        roles_raw = claims.get("roles") or []
        roles = [r for r in roles_raw if isinstance(r, str)]
        if POA_VCT in vct_values:
            org = str(claims.get("organization") or claims.get("organization_name") or "?")
            print(f"  [{index}] {stem}")
            print(f"      organization: {org}")
            print(f"      roles ({len(roles)}): {', '.join(roles) if roles else '(none)'}")
            continue
        if any(v in PID_VCT_VALUES for v in vct_values):
            given = str(claims.get("given_name") or "?")
            family = str(claims.get("family_name") or "?")
            print(f"  [{index}] {stem}")
            print(f"      name: {given} {family}")
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


def extract_login_challenge(session: Any, authorize_url: str, *, log: LogFn = default_log) -> str:
    """Walk Hydra's authorize redirect chain to the login UI and read the
    login_challenge it carries. This is the hop a browser makes; a headless
    wallet has to make it too, because the DCS binds the challenge to the
    pending presentation and refuses the callback without one."""
    url = authorize_url
    for _ in range(8):
        r = session.get(url, allow_redirects=False, timeout=30)
        if r.status_code not in (301, 302, 303, 307, 308):
            raise RuntimeError(f"authorize_url did not redirect to the login UI ({r.status_code})")
        location = str(r.headers.get("location") or "").strip()
        if not location:
            raise RuntimeError("authorize redirect is missing its Location header")
        challenges = parse_qs(urlparse(location).query).get("login_challenge") or []
        if challenges and challenges[0].strip():
            log("login-challenge", "resolved from authorize redirect")
            return challenges[0].strip()
        url = location
    raise RuntimeError("login_challenge not found in the Hydra authorize redirect chain")


def bind_hydra_login_challenge(
    session: Any,
    api_base: str,
    *,
    state: str,
    authorize_url: str,
    log: LogFn = default_log,
) -> None:
    challenge = extract_login_challenge(session, authorize_url, log=log)
    r = session.post(
        f"{api_base.rstrip('/')}/auth/login/challenge",
        json_body={"state": state, "login_challenge": challenge},
        timeout=30,
    )
    if r.status_code not in (200, 204):
        raise RuntimeError(f"/auth/login/challenge failed ({r.status_code}): {r.text[:200]}")
    log("login-challenge-ok", "bound to the pending presentation")


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


def await_login_redirect_uri(
    session: Any,
    api_base: str,
    *,
    state: str,
    log: LogFn = default_log,
    timeout_sec: float = 60.0,
) -> str:
    """Poll GET /auth/login/status until the DCS has accepted the login with
    Hydra and recorded where the browser should go next."""
    if not state:
        raise RuntimeError("login status polling requires the request object's state")

    status_url = f"{api_base.rstrip('/')}/auth/login/status?state={quote(state)}"
    deadline = time.monotonic() + timeout_sec
    body: dict[str, Any] = {}
    while time.monotonic() < deadline:
        r = session.get(status_url, timeout=30)
        if not _http_success(r.status_code):
            raise RuntimeError(f"/auth/login/status failed ({r.status_code}): {r.text[:200]}")
        parsed = r.json()
        body = parsed if isinstance(parsed, dict) else {}
        status = str(body.get("status") or "")
        if status == "complete":
            redirect_uri = str(body.get("redirect_uri") or "")
            if not redirect_uri:
                raise RuntimeError(f"login status is complete but carries no redirect_uri: {body}")
            log("login-status-ok", "login accepted", status=status)
            return redirect_uri
        if status in ("failed", "expired"):
            raise RuntimeError(f"login status is {status!r}: {body}")
        time.sleep(0.2)

    raise RuntimeError(f"login status did not complete within {timeout_sec:.0f}s: {body}")


def _to_api_origin(url: str, api_base: str) -> str:
    """Point a browser-facing URL (the Vite dev server) at the API origin the
    wallet actually reached, so a headless run can follow the same chain."""
    target, api = urlparse(url), urlparse(api_base)
    if target.netloc == api.netloc:
        return url
    return url.replace(f"{target.scheme}://{target.netloc}", f"{api.scheme}://{api.netloc}", 1)


def resolve_oauth_callback_url(session: Any, redirect_uri: str, api_base: str) -> str:
    """Follow the Hydra redirect chain after the VP is accepted until it reaches
    the API's /auth/callback carrying an authorization code. Hydra bounces
    through a consent step, which the DCS auto-accepts at /auth/consent."""
    url = redirect_uri
    for _ in range(12):
        parsed = urlparse(url)
        consent_challenge = (parse_qs(parsed.query).get("consent_challenge") or [""])[0]
        if consent_challenge:
            r = session.get(
                f"{api_base.rstrip('/')}/auth/consent?consent_challenge={quote(consent_challenge)}",
                allow_redirects=False,
                timeout=30,
            )
            if r.status_code not in (301, 302, 303, 307, 308):
                raise RuntimeError(f"/auth/consent failed ({r.status_code}): {r.text[:200]}")
            url = str(r.headers.get("location") or "")
            continue

        url = _to_api_origin(url, api_base)
        parsed = urlparse(url)
        if parsed.path.endswith("/auth/callback") and parse_qs(parsed.query).get("code"):
            return url

        r = session.get(url, allow_redirects=False, timeout=30)
        if r.status_code not in (301, 302, 303, 307, 308):
            raise RuntimeError(f"oauth redirect chain stopped ({r.status_code}) at {url[:160]}")
        location = str(r.headers.get("location") or "").strip()
        if not location:
            raise RuntimeError(f"oauth redirect is missing its Location header at {url[:160]}")
        url = location

    raise RuntimeError("oauth redirect chain did not reach /auth/callback?code=")


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
    disclosed = merge_disclosed_claims(issuer_claims, disclosures)
    org = disclosed.get("organization")
    if org:
        print(f"organization    : {org}")
    roles = disclosed.get("roles")
    if isinstance(roles, list) and roles:
        role_labels = [str(role) for role in roles if isinstance(role, str)]
        if role_labels:
            print(f"roles           : {', '.join(role_labels)}")
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
        queries = resolve_dcql_credential_queries(req_obj.get("dcql_query"))
    except ValueError as exc:
        log("fetch", "FAILED", error=str(exc))
        return 1

    log(
        "fetch-ok",
        "request object verified",
        query_ids=[q.query_id for q in queries],
        response_mode=response_mode,
        nonce_prefix=nonce[:12],
    )

    vp_token_obj: dict[str, list[str]] = {}
    for query in queries:
        chosen = resolve_credential_name(credential_name, vct_values=query.vct_values, log=log)
        if not chosen:
            return 1

        try:
            vp_token = build_vp_token(
                credential_name=chosen,
                nonce=nonce,
                client_id=client_id,
                requested_claim_paths=query.claim_paths,
            )
        except Exception as exc:
            log("present", "FAILED to build VP", error=str(exc))
            return 1

        log_vp_token(vp_token, query_id=query.query_id, credential=chosen, aud=client_id, nonce=nonce)
        vp_token_obj[query.query_id] = [vp_token]

    vp_token_object = json.dumps(vp_token_obj, separators=(",", ":"))

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

    api = ctx.api_base

    # The direct_post handler only acknowledges receipt: the DCS accepts the
    # login with Hydra afterwards and records the redirect target against the
    # presentation state, so the redirect_uri is read from the login status
    # rather than the callback's own response body.
    if not redirect_uri:
        try:
            redirect_uri = await_login_redirect_uri(session, api, state=state, log=log)
        except RuntimeError as exc:
            log("login-status", "FAILED", error=str(exc))
            return 1

    try:
        callback_url = resolve_oauth_callback_url(session, redirect_uri, api)
    except RuntimeError as exc:
        log("oauth-callback", "FAILED", error=str(exc))
        return 1

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
