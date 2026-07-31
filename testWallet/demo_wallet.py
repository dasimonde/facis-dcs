#!/usr/bin/env python3
"""
Extended demonstration wallet — DCS login and external OpenID4VP verifiers (e.g. EUDIPLO).

Uses the same credentials/keys as demo_wallet.py but follows browser-based-ssi interop rules:
  - GET request_uri without wallet_nonce (EUDIPLO)
  - POST request_uri with wallet_nonce echo (DCS)
  - direct_post and direct_post.jwt response modes
  - DCQL credential_sets (PID bank KYC)

Usage (from repo root):

  # DCS /ui/ login:
  python3 testWallet/test_wallet2.py

  # EUDIPLO Nordic Bank — paste openid4vp:// from QR:
  python3 testWallet/test_wallet2.py --credential johndoe.pid

  # Headless DCS login:
  python3 testWallet/test_wallet2.py --headless --credential test
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from http.cookiejar import CookieJar
from pathlib import Path
from urllib.parse import urlencode

sys.path.insert(0, str(Path(__file__).resolve().parent))

from dcs_wallet.oid4vp_flow import (
    bind_hydra_login_challenge,
    default_log,
    presentation_context_from_link,
    resolve_presentation_link,
    run_presentation_flow,
)

DEFAULT_API_BASE = os.environ.get("DCS_API_BASE", "http://localhost:8991/api")
DEFAULT_CREDENTIAL = os.environ.get("DCS_WALLET_CREDENTIAL", "test")
_KEYS_DIR = Path(__file__).resolve().parent / "keys"
_REQUIRED_KEYS = ("issuer-dev.jwk", "wallet.jwk")
_GENERATE_HINT = "python3 testWallet/scripts/generate_keys.py --yes && python3 testWallet/scripts/issue_credentials.py"


class _Response:
    def __init__(self, status: int, headers: dict[str, str], body: bytes) -> None:
        self.status_code = status
        self.headers = headers
        self._body = body

    def json(self) -> object:
        if not self._body:
            return {}
        return json.loads(self._body.decode())

    @property
    def text(self) -> str:
        return self._body.decode(errors="replace")


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class HttpSession:
    def __init__(self) -> None:
        self._jar = CookieJar()
        self._opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self._jar))
        self._headers = {
            "User-Agent": "testWallet/test_wallet2",
            "Accept": "application/json",
        }

    def get(self, url: str, *, timeout: float = 30, allow_redirects: bool = True, accept: str | None = None) -> _Response:
        headers = dict(self._headers)
        if accept:
            headers["Accept"] = accept
        req = urllib.request.Request(url, headers=headers, method="GET")
        if allow_redirects:
            return self._open(self._opener, req, timeout)
        opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self._jar),
            _NoRedirect(),
        )
        try:
            return self._open(opener, req, timeout)
        except urllib.error.HTTPError as exc:
            if exc.code in (301, 302, 303, 307, 308):
                return _Response(exc.code, {k.lower(): v for k, v in exc.headers.items()}, exc.read())
            raise

    def post(
        self,
        url: str,
        *,
        json_body: dict | None = None,
        form_body: dict[str, str] | None = None,
        timeout: float = 30,
        accept: str | None = None,
    ) -> _Response:
        if json_body is not None and form_body is not None:
            raise ValueError("post() accepts either json_body or form_body, not both")

        data = b""
        headers = dict(self._headers)
        if json_body is not None:
            data = json.dumps(json_body).encode()
            headers["Content-Type"] = "application/json"
        if form_body is not None:
            data = urlencode(form_body).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        if accept:
            headers["Accept"] = accept
        req = urllib.request.Request(url, data=data, headers=headers, method="POST")
        return self._open(self._opener, req, timeout)

    @staticmethod
    def _open(opener, req: urllib.request.Request, timeout: float) -> _Response:
        try:
            with opener.open(req, timeout=timeout) as fp:
                body = fp.read()
                headers = {k.lower(): v for k, v in fp.headers.items()}
                return _Response(fp.status, headers, body)
        except urllib.error.HTTPError as exc:
            body = exc.read()
            headers = {k.lower(): v for k, v in exc.headers.items()}
            return _Response(exc.code, headers, body)
        except urllib.error.URLError as exc:
            raise RuntimeError(f"request failed: {exc.reason}") from exc


def ensure_wallet_keys() -> bool:
    missing = [name for name in _REQUIRED_KEYS if not (_KEYS_DIR / name).is_file()]
    if not missing:
        return True
    default_log("wallet", "FAILED", error=f"missing keys: {', '.join(missing)} — run: {_GENERATE_HINT}")
    return False


def prompt_presentation_url() -> str:
    print("Paste presentation_url (openid4vp:// from DCS login or external verifier QR), then press Enter:")
    try:
        line = sys.stdin.readline()
    except KeyboardInterrupt:
        print()
        raise SystemExit(130) from None
    if not line:
        raise ValueError("no input (EOF)")
    return line.strip()


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Present credentials via OpenID4VP (DCS login or external verifier interop).",
    )
    parser.add_argument("--api-base", default=DEFAULT_API_BASE, help="DCS API base for --headless")
    parser.add_argument(
        "--presentation-url",
        "--request-uri",
        dest="presentation_url",
        default=os.environ.get("DCS_PRESENTATION_URL", os.environ.get("DCS_OID4VP_REQUEST_URI", "")),
        help="openid4vp:// or HTTPS request_uri; skips prompt",
    )
    parser.add_argument(
        "--credential",
        default=None,
        help=(
            "Credential stem in testWallet/credentials/ (e.g. test, johndoe.pid). "
            "Comma-separate one per credential query when the request asks for "
            "several (the signing ceremony asks for a PoA and a PID): "
            "janesmith,janesmith.pid"
        ),
    )
    parser.add_argument(
        "--headless",
        action="store_true",
        help="POST /auth/login and run full DCS flow without a browser tab",
    )
    args = parser.parse_args()
    credential = args.credential or (DEFAULT_CREDENTIAL if args.headless else None)

    if not ensure_wallet_keys():
        return 1

    session = HttpSession()

    if not args.headless:
        os.environ["DCS_WALLET_FINISH_BROWSER"] = "0"
        pasted = (args.presentation_url or "").strip()
        if not pasted:
            try:
                pasted = prompt_presentation_url()
            except ValueError as exc:
                default_log("login", "FAILED", error=str(exc))
                return 1
        try:
            link = resolve_presentation_link(pasted)
            ctx = presentation_context_from_link(link)
        except ValueError as exc:
            default_log("login", "FAILED", error=str(exc))
            return 1
        return run_presentation_flow(session, ctx, credential_name=credential)

    api = args.api_base.rstrip("/")
    default_log("wallet", "headless DCS login", api_base=api)
    default_log("login", "POST /auth/login")
    r = session.post(f"{api}/auth/login", timeout=30)
    if r.status_code != 200:
        default_log("login", "FAILED", status=r.status_code, body=r.text[:300])
        return 1
    initiation = r.json()
    request_uri = initiation["request_uri"]
    default_log("login-ok", "initiate OK", request_uri_prefix=request_uri[:96])

    # The DCS mints its session through Hydra, so the presentation callback
    # refuses a login attempt no login_challenge is bound to. A browser binds it
    # by following authorize_url; headless, the wallet has to do that walk
    # itself or its presentation is rejected after it is already built.
    try:
        bind_hydra_login_challenge(
            session,
            api,
            state=str(initiation["state"]),
            authorize_url=str(initiation["authorize_url"]),
        )
    except (KeyError, RuntimeError) as exc:
        default_log("login-challenge", "FAILED", error=str(exc))
        return 1

    link = resolve_presentation_link(request_uri)
    ctx = presentation_context_from_link(link)
    return run_presentation_flow(session, ctx, credential_name=credential)


if __name__ == "__main__":
    sys.exit(main())
