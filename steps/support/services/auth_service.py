"""Authentication and authorization service for BDD steps."""

from __future__ import annotations

import base64
import json
import os
import re
import sys
import time
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

import jwt
import requests
from jwt.algorithms import ECAlgorithm

@dataclass(frozen=True)
class AuthCredentials:
    """Roles and organization used to build the OID4VP presentation."""

    roles: list[str]
    organization: str


@dataclass(frozen=True)
class WalletKeys:
    """Issuer and holder private keys from testWallet."""

    issuer_private: dict[str, Any]
    wallet_private: dict[str, Any]


@dataclass(frozen=True)
class LoginInitiation:
    """Response from POST /auth/login."""

    state: str
    request_uri: str
    authorize_url: str


@dataclass(frozen=True)
class AuthorizationRequest:
    """Parsed OpenID4VP authorization request object (JAR)."""

    nonce: str
    client_id: str
    response_uri: str
    state: str
    query_id: str


class AuthService:
    """Handle auth headers and role-based OID4VP token exchange."""

    CLIENT_ID = "dcs-client"
    DEFAULT_ORGANIZATION = "Acme Corp"

    # Per-(api_base, roles, organization) access-token cache. Each token
    # normally costs a full headless OID4VP round trip (login -> consent ->
    # VP presentation -> callback -> refresh), which dominates BDD runtime
    # when hundreds of scenarios each call get_headers_for_roles for the same
    # role. Hydra access tokens are valid for a fixed lifespan (returned as
    # expires_in by /auth/refresh) — safe to reuse across scenarios within
    # that window. Never used for use_expired_jwt=True (those intentionally
    # need a token that is NOT valid).
    _token_cache: dict[tuple[str, tuple[str, ...], str], tuple[str, float]] = {}
    _TOKEN_EXPIRY_MARGIN_SECONDS = 15

    # ------------------------------------------------------------------
    # Step 1 — resolve roles / organization
    # ------------------------------------------------------------------

    @staticmethod
    def parse_auth_credentials(roles: list[str], organization: str | None = None) -> AuthCredentials:
        """Normalize roles and organization for VP issuance."""
        normalized_roles = [role.strip() for role in roles if role and role.strip()]
        if not normalized_roles:
            raise ValueError("roles must contain at least one non-empty role")
        org = (organization or AuthService.DEFAULT_ORGANIZATION).strip()
        if not org:
            org = AuthService.DEFAULT_ORGANIZATION
        return AuthCredentials(
            roles=normalized_roles,
            organization=org,
        )

    # ------------------------------------------------------------------
    # Step 2 — load testWallet keys and build VP JWT
    # ------------------------------------------------------------------

    @staticmethod
    def _authorization_request_key(header: dict[str, Any]):
        """Resolve the key that signed the authorization request object.

        A verifier that identifies itself with the x509_san_dns client id
        prefix proves it owns that hostname with a certificate chain, so the
        request object carries x5c rather than a bare jwk. Both encodings are
        accepted here: the request is signed either way, and which one arrives
        depends on how the deployment names itself to the wallet.
        """
        jwk = header.get("jwk")
        if isinstance(jwk, dict):
            return ECAlgorithm.from_jwk(json.dumps(jwk))

        x5c = header.get("x5c")
        if isinstance(x5c, list) and x5c:
            from cryptography import x509 as _x509

            return _x509.load_der_x509_certificate(base64.b64decode(str(x5c[0]))).public_key()

        raise RuntimeError("authorization request JWT header missing jwk or x5c")

    @staticmethod
    def resolve_wallet_root() -> Path:
        """Locate testWallet root (repo checkout or helm/BDD override)."""
        override = os.getenv("BDD_TEST_WALLET_DIR", "").strip()
        if override:
            return Path(override).expanduser().resolve()
        return Path(__file__).resolve().parents[3] / "testWallet"

    @staticmethod
    def resolve_wallet_keys_dir() -> Path:
        """Locate wallet key directory; defaults to testWallet/keys."""
        override = os.getenv("BDD_TEST_WALLET_KEYS_DIR", "").strip()
        if override:
            return Path(override).expanduser().resolve()
        return AuthService.resolve_wallet_root() / "keys"

    @staticmethod
    def _ensure_dcs_wallet_importable() -> None:
        wallet_root = str(AuthService.resolve_wallet_root())
        if wallet_root not in sys.path:
            sys.path.insert(0, wallet_root)

    @staticmethod
    def load_wallet_keys(keys_dir: Path | None = None) -> WalletKeys:
        """Load issuer-dev.jwk and wallet.jwk used to sign SD-JWT + KB-JWT."""
        keys_path = keys_dir or AuthService.resolve_wallet_keys_dir()
        issuer_path = keys_path / "issuer-dev.jwk"
        wallet_path = keys_path / "wallet.jwk"
        missing = [name for name, path in (
            ("issuer-dev.jwk", issuer_path),
            ("wallet.jwk", wallet_path),
        ) if not path.is_file()]
        if missing:
            raise FileNotFoundError(
                f"missing {', '.join(missing)} in {keys_path} — "
                "run: python3 testWallet/scripts/generate_keys.py --yes"
            )

        AuthService._ensure_dcs_wallet_importable()
        from dcs_wallet.keys import load_json, private_key_material  # noqa: PLC0415

        return WalletKeys(
            issuer_private=private_key_material(load_json(issuer_path)),
            wallet_private=private_key_material(load_json(wallet_path)),
        )

    @staticmethod
    def build_vp_token(
        credentials: AuthCredentials,
        *,
        nonce: str,
        client_id: str,
        keys: WalletKeys | None = None,
    ) -> str:
        """Build vp_token: issuer SD-JWT from roles, then KB-JWT with request aud/nonce."""
        if not nonce:
            raise ValueError("nonce is required to build vp_token")
        if not client_id:
            raise ValueError("client_id is required to build vp_token")

        wallet_keys = keys or AuthService.load_wallet_keys()
        AuthService._ensure_dcs_wallet_importable()
        from dcs_wallet.issuer import (
            DEFAULT_ISSUER_DID,
            attach_key_binding,
            issue_stored_credential,
        )
        from dcs_wallet.status_list import ISSUER_BASE_ENV, role_credential_index

        issuer_did = os.getenv("BDD_ISSUER_DID", DEFAULT_ISSUER_DID)
        issuer_base = os.getenv(ISSUER_BASE_ENV, "").strip()
        if not issuer_base:
            raise RuntimeError(
                f"{ISSUER_BASE_ENV} is required for BDD OID4VP credentials — it names the "
                "ORCE issuer serving the status list the credential points at (set by "
                "run_bdd_helm.sh)"
            )
        stored_sd_jwt = issue_stored_credential(
            organization=credentials.organization,
            roles=credentials.roles,
            issuer_private=wallet_keys.issuer_private,
            wallet_private=wallet_keys.wallet_private,
            status_index=role_credential_index(
                organization=credentials.organization, roles=credentials.roles
            ),
            issuer_did=issuer_did,
            issuer_base=issuer_base,
        )
        return attach_key_binding(
            issued_sd_jwt=stored_sd_jwt,
            wallet_private=wallet_keys.wallet_private,
            aud=client_id,
            nonce=nonce,
        )

    # ------------------------------------------------------------------
    # Step 3 — OID4VP login API calls
    # ------------------------------------------------------------------

    @staticmethod
    def initiate_login(session: requests.Session, api_base: str, *, timeout: float) -> LoginInitiation:
        """POST /auth/login and return state, request_uri, and Hydra authorize_url."""
        response = session.post(f"{api_base.rstrip('/')}/auth/login", timeout=timeout)
        response.raise_for_status()
        body = response.json()
        state = body.get("state")
        request_uri = body.get("request_uri")
        authorize_url = body.get("authorize_url")
        if not state or not request_uri or not authorize_url:
            raise RuntimeError(f"/auth/login missing state, request_uri, or authorize_url: {body}")
        return LoginInitiation(
            state=str(state),
            request_uri=str(request_uri),
            authorize_url=str(authorize_url),
        )

    @staticmethod
    def extract_login_challenge(authorize_url: str, *, timeout: float, session: requests.Session | None = None) -> str:
        """Follow Hydra authorize redirect and read login_challenge from the login UI URL."""
        http = session or requests.Session()
        url = authorize_url
        for _ in range(8):
            response = http.get(url, allow_redirects=False, timeout=timeout)
            if response.status_code not in (301, 302, 303, 307, 308):
                raise RuntimeError(
                    f"authorize_url did not redirect to login UI ({response.status_code}): {response.text[:200]}"
                )
            location = response.headers.get("Location", "").strip()
            if not location:
                raise RuntimeError("authorize redirect missing Location header")
            query = parse_qs(urlparse(location).query)
            challenges = query.get("login_challenge") or []
            if challenges and challenges[0].strip():
                return challenges[0].strip()
            url = location
        raise RuntimeError("login_challenge not found in Hydra authorize redirect chain")

    @staticmethod
    def bind_hydra_login_challenge(
        session: requests.Session,
        api_base: str,
        *,
        state: str,
        authorize_url: str,
        timeout: float,
    ) -> None:
        """Bind Hydra login_challenge to the pending OID4VP presentation (browser step, headless)."""
        login_challenge = AuthService.extract_login_challenge(
            authorize_url,
            timeout=timeout,
            session=session,
        )
        response = session.post(
            f"{api_base.rstrip('/')}/auth/login/challenge",
            json={"state": state, "login_challenge": login_challenge},
            timeout=timeout,
        )
        if response.status_code not in (200, 204):
            raise RuntimeError(
                f"/auth/login/challenge failed ({response.status_code}): {response.text[:300]}"
            )

    @staticmethod
    def fetch_authorization_request(
        session: requests.Session,
        request_uri: str,
        *,
        timeout: float,
    ) -> AuthorizationRequest:
        """POST request_uri (JAR), verify JWS, and parse request parameters."""
        wallet_nonce = str(uuid.uuid4())
        wallet_metadata = {
            "vp_formats_supported": {
                "dc+sd-jwt": {
                    "sd-jwt_alg_values": ["ES256"],
                    "kb-jwt_alg_values": ["ES256"],
                }
            }
        }
        response = session.post(
            request_uri,
            timeout=timeout,
            headers={
                "Accept": "application/oauth-authz-req+jwt, application/jwt",
                "Content-Type": "application/x-www-form-urlencoded",
            },
            data={
                "wallet_nonce": wallet_nonce,
                "wallet_metadata": json.dumps(wallet_metadata, separators=(",", ":")),
            },
        )
        response.raise_for_status()
        jar_token = response.text.strip()
        if not jar_token.startswith("eyJ"):
            raise RuntimeError("authorization request response is not a JWT")

        try:
            header = jwt.get_unverified_header(jar_token)
        except Exception as exc:
            raise RuntimeError(f"authorization request JWT header parse failed: {exc}") from exc

        if str(header.get("typ") or "") != "oauth-authz-req+jwt":
            raise RuntimeError("authorization request JWT typ is invalid")
        verification_key = AuthService._authorization_request_key(header)

        try:
            payload = jwt.decode(
                jar_token,
                verification_key,
                algorithms=["ES256"],
                options={"verify_aud": False, "require": ["exp"]},
            )
        except Exception as exc:
            raise RuntimeError(f"authorization request JWT verification failed: {exc}") from exc

        if str(payload.get("wallet_nonce") or "") != wallet_nonce:
            raise RuntimeError("authorization request wallet_nonce echo mismatch")

        nonce = str(payload.get("nonce") or "")
        client_id = str(payload.get("client_id") or "")
        if not client_id:
            raise RuntimeError("authorization request JWT missing client_id")
        state = str(payload.get("state") or "")
        response_uri = str(payload.get("response_uri") or "")
        if str(payload.get("response_mode") or "") != "direct_post":
            raise RuntimeError("unsupported response_mode in authorization request")

        dcql_query = payload.get("dcql_query")
        if not isinstance(dcql_query, dict):
            raise RuntimeError("authorization request missing dcql_query object")
        credentials = dcql_query.get("credentials")
        if not isinstance(credentials, list) or len(credentials) == 0 or not isinstance(credentials[0], dict):
            raise RuntimeError("dcql_query.credentials[0] is required")
        query_id = str(credentials[0].get("id") or "").strip()
        if not query_id:
            raise RuntimeError("dcql_query.credentials[0].id is required")

        if not nonce:
            raise RuntimeError("authorization request JWT missing nonce")
        if not state or not response_uri:
            raise RuntimeError("authorization request JWT missing state or response_uri")
        return AuthorizationRequest(
            nonce=nonce,
            client_id=client_id,
            response_uri=response_uri,
            state=state,
            query_id=query_id,
        )

    @staticmethod
    def submit_presentation(
        session: requests.Session,
        *,
        api_base: str,
        response_uri: str,
        state: str,
        query_id: str,
        vp_token: str,
        timeout: float,
    ) -> str:
        """direct_post vp_token to response_uri, then poll GET /auth/login/status
        for the redirect_uri. PresentationCallback (the direct_post handler) only
        acknowledges receipt (`{}`) and persists the redirect target asynchronously
        via Hydra's AcceptLoginAndConsent — the caller retrieves it by polling
        loginStatus until status == "complete" (see backend/design/auth.go's
        `loginStatus` method / `LoginStatus` handler in auth_login.go).
        """
        response = session.post(
            response_uri,
            headers={"Content-Type": "application/x-www-form-urlencoded"},
            data={
                "state": state,
                "vp_token": json.dumps({query_id: [vp_token]}, separators=(",", ":")),
            },
            timeout=timeout,
        )
        if response.status_code != 200:
            raise RuntimeError(
                f"direct_post failed ({response.status_code}): {response.text[:300]}"
            )

        status_url = f"{api_base.rstrip('/')}/auth/login/status"
        deadline = time.time() + timeout
        last_body: dict = {}
        while time.time() < deadline:
            status_response = session.get(status_url, params={"state": state}, timeout=timeout)
            if status_response.status_code != 200:
                raise RuntimeError(
                    f"GET /auth/login/status failed ({status_response.status_code}): "
                    f"{status_response.text[:300]}"
                )
            last_body = status_response.json()
            status = last_body.get("status")
            if status == "complete":
                redirect_uri = last_body.get("redirect_uri")
                if not redirect_uri:
                    raise RuntimeError(f"login status complete but missing redirect_uri: {last_body}")
                return str(redirect_uri)
            if status in ("failed", "expired"):
                raise RuntimeError(f"login status is '{status}': {last_body}")
            time.sleep(0.2)

        raise RuntimeError(f"login status did not reach 'complete' within {timeout}s: {last_body}")

    @staticmethod
    def resolve_oauth_callback_url(
        session: requests.Session,
        redirect_uri: str,
        api_base: str,
        *,
        timeout: float,
    ) -> str:
        """Follow Hydra redirects after VP accept until the API /auth/callback has ?code=."""
        url = redirect_uri
        for _ in range(12):
            if "consent_challenge" in url:
                parsed = urlparse(url)
                consent_challenge = (parse_qs(parsed.query).get("consent_challenge") or [""])[0]
                if consent_challenge:
                    response = session.get(
                        f"{api_base.rstrip('/')}/auth/consent?consent_challenge={consent_challenge}",
                        allow_redirects=False,
                        timeout=timeout,
                    )
                    if response.status_code not in (301, 302, 303, 307, 308):
                        raise RuntimeError(
                            f"/auth/consent failed ({response.status_code}): {response.text[:300]}"
                        )
                    url = response.headers.get("Location", "")
                    continue

            if url.startswith("http://localhost:5173") or url.startswith("https://localhost:5173"):
                url = AuthService.normalize_callback_url(url, api_base)

            # A refused login redirects to the UI with the reason in the query
            # and, being same-origin, does it with a relative Location. Fetching
            # that raises MissingSchema and buries the reason under an invalid
            # URL, so report what the authorization server actually said.
            auth_error = parse_qs(urlparse(url).query).get("auth_error")
            if auth_error:
                description = parse_qs(urlparse(url).query).get(
                    "auth_error_description", [""]
                )[0]
                raise RuntimeError(
                    f"login refused ({auth_error[0]}): {description or 'no description given'}"
                )

            parsed = urlparse(url)
            if parsed.path.endswith("/auth/callback") and parse_qs(parsed.query).get("code"):
                return url

            response = session.get(url, allow_redirects=False, timeout=timeout)
            if response.status_code not in (301, 302, 303, 307, 308):
                raise RuntimeError(
                    f"oauth redirect chain stopped ({response.status_code}) at {url[:160]}: {response.text[:200]}"
                )
            location = response.headers.get("Location", "").strip()
            if not location:
                raise RuntimeError(f"oauth redirect missing Location at {url[:160]}")
            if location.startswith("http://localhost:5173") or location.startswith("https://localhost:5173"):
                location = AuthService.normalize_callback_url(location, api_base)
            parsed = urlparse(location)
            if parsed.path.endswith("/auth/callback") and parse_qs(parsed.query).get("code"):
                return location
            url = location

        raise RuntimeError("oauth redirect chain did not reach /auth/callback?code=")

    @staticmethod
    def complete_session(
        session: requests.Session,
        api_base: str,
        redirect_uri: str,
        *,
        timeout: float,
    ) -> tuple[str, int]:
        """Follow OAuth callback and refresh to obtain Hydra access_token.

        Returns (access_token, expires_in_seconds).
        """
        callback_url = AuthService.resolve_oauth_callback_url(
            session,
            redirect_uri,
            api_base,
            timeout=timeout,
        )
        callback_response = session.get(callback_url, allow_redirects=False, timeout=timeout)
        if callback_response.status_code != 302:
            raise RuntimeError(
                f"/auth/callback failed ({callback_response.status_code}): {callback_response.text[:300]}"
            )

        refresh_response = session.post(f"{api_base.rstrip('/')}/auth/refresh", timeout=timeout)
        refresh_response.raise_for_status()
        body = refresh_response.json()
        access_token = body.get("access_token")
        if not access_token:
            raise RuntimeError(f"/auth/refresh missing access_token: {refresh_response.text[:300]}")
        return str(access_token), int(body.get("expires_in") or 0)

    @staticmethod
    def exchange_roles_for_access_token(
        api_base: str,
        roles: list[str],
        *,
        organization: str | None = None,
        timeout: float = 60,
    ) -> str:
        """Full OID4VP headless login: roles → vp_token → access_token.

        Reuses a cached access_token for the same (api_base, roles,
        organization) tuple until it is within _TOKEN_EXPIRY_MARGIN_SECONDS of
        expiry — the headless login round trip otherwise dominates BDD
        runtime since most scenarios request the same handful of roles.
        """
        credentials = AuthService.parse_auth_credentials(roles, organization)
        cache_key = (api_base, tuple(sorted(credentials.roles)), credentials.organization)
        cached = AuthService._token_cache.get(cache_key)
        if cached is not None:
            token, expires_at = cached
            if time.time() < expires_at - AuthService._TOKEN_EXPIRY_MARGIN_SECONDS:
                return token

        session = requests.Session()
        session.headers.update({
            "User-Agent": "bdd-auth-service",
            "Accept": "application/json",
        })

        initiation = AuthService.initiate_login(session, api_base, timeout=timeout)
        AuthService.bind_hydra_login_challenge(
            session,
            api_base,
            state=initiation.state,
            authorize_url=initiation.authorize_url,
            timeout=timeout,
        )
        auth_request = AuthService.fetch_authorization_request(
            session,
            initiation.request_uri,
            timeout=timeout,
        )
        vp_token = AuthService.build_vp_token(
            credentials,
            nonce=auth_request.nonce,
            client_id=auth_request.client_id,
        )
        redirect_uri = AuthService.submit_presentation(
            session,
            api_base=api_base,
            response_uri=auth_request.response_uri,
            state=auth_request.state,
            query_id=auth_request.query_id,
            vp_token=vp_token,
            timeout=timeout,
        )
        access_token, expires_in = AuthService.complete_session(
            session,
            api_base,
            redirect_uri,
            timeout=timeout,
        )
        if expires_in > 0:
            AuthService._token_cache[cache_key] = (access_token, time.time() + expires_in)
        return access_token


    # ------------------------------------------------------------------
    # Step 4 — set context headers for Behave steps
    # ------------------------------------------------------------------

    @staticmethod
    def set_headers_for_roles(
        context,
        roles: list[str],
        use_expired_jwt: bool = False,
        organization: str | None = None,
    ):
        """Set context.headers with a bearer token for the given roles."""
        # TBD: extend issue_credentials.py to support generating expired JWT credentials
        if use_expired_jwt:
            username = AuthService.username_for_roles(roles)
            token = AuthService.create_expired_jwt(AuthService.CLIENT_ID, username, roles)
        else:
            api_base = getattr(context, "base_url", os.getenv("BDD_DCS_BASE_URL", "http://localhost:5173/api"))
            timeout = float(getattr(context, "http_timeout_seconds", os.getenv("BDD_HTTP_TIMEOUT_SECONDS", "60")))
            token = AuthService.exchange_roles_for_access_token(
                api_base,
                roles,
                organization=organization,
                timeout=timeout,
            )
        context.headers = {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        }

    @staticmethod
    def get_headers_for_roles(
        roles: list[str],
        use_expired_jwt: bool = False,
        organization: str | None = None,
        api_base: str | None = None,
        timeout: float = 60,
    ) -> dict:
        """Return auth headers for a given role (without modifying context).

        `organization` is the only way to obtain a credential distinct from the
        suite-shared one for the same roles: the OID4VP login derives identity
        from the presented VP, so (roles, organization) is the whole identity —
        both for the token cache and for anything the server keys per client.
        """
        if use_expired_jwt:
            username = AuthService.username_for_roles(roles)
            token = AuthService.create_expired_jwt(AuthService.CLIENT_ID, username, roles)
        else:
            token = AuthService.exchange_roles_for_access_token(
                api_base or os.getenv("BDD_DCS_BASE_URL", "http://localhost:5173/api"),
                roles,
                organization=organization,
                timeout=timeout,
            )
        return {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        }

    # ------------------------------------------------------------------
    # Helpers (JWT decode, expired-token fixtures, username)
    # ------------------------------------------------------------------

    @staticmethod
    def decode_jwt_payload(token: str) -> dict[str, Any]:
        parts = token.strip().split(".")
        if len(parts) < 2:
            raise ValueError("token is not a compact JWT")
        payload = parts[1]
        padding = "=" * (-len(payload) % 4)
        data = json.loads(base64.urlsafe_b64decode(payload + padding).decode())
        if not isinstance(data, dict):
            raise ValueError("JWT payload must be a JSON object")
        return data

    @staticmethod
    def normalize_callback_url(redirect_uri: str, api_base: str) -> str:
        """Map Vite (:5173) OAuth callback URLs to the BDD API base when using local air."""
        api = urlparse(api_base)
        target = urlparse(redirect_uri)
        if target.netloc.endswith(":5173"):
            api_origin = f"{api.scheme}://{api.netloc}"
            return redirect_uri.replace(f"{target.scheme}://{target.netloc}", api_origin, 1)
        return redirect_uri

    # TBD: credential didn't contain any username info.
    @staticmethod
    def username_for_roles(roles: list[str]) -> str:
        """Convert role names to a deterministic BDD username."""
        role_safe = [
            re.sub(r"[^A-Za-z0-9]+", "-", role.lower()).strip("-")
            for role in roles
            if role and role.strip()
        ]
        return f"bdd-{'-'.join(role_safe)}"

    @staticmethod
    def create_expired_jwt(client_id, username, roles):
        """Create an expired JWT token for negative credential tests.

        The issuer must match the backend's HYDRA_PUBLIC_ISSUER_URL: go-oidc
        checks issuer before expiry, so a foreign issuer fails with
        "issued by a different provider" instead of the "token is expired"
        message the scenarios assert on. Expiry is checked before the
        signature, so the token needs no valid signature.
        """
        issuer = (
            os.getenv("BDD_HYDRA_ISSUER_URL", "").strip()
            or os.getenv("BDD_PUBLIC_ORIGIN", "").strip()
            or "http://localhost:18080"
        ).rstrip("/")
        header = {"alg": "none"}
        payload = {
            "sub": username,
            "iss": issuer,
            "client_id": client_id,
            "ext": {"roles": roles, "iss": issuer},
            "exp": int(time.time()) - 3600,
        }

        encoded_header = base64.urlsafe_b64encode(json.dumps(header).encode()).decode().rstrip("=")
        encoded_payload = base64.urlsafe_b64encode(json.dumps(payload).encode()).decode().rstrip("=")
        return f"{encoded_header}.{encoded_payload}."
