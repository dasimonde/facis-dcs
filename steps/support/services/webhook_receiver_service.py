"""In-process HTTP receiver for DCS webhook deliveries (Standard Webhooks v1).

Each subscription gets its own path. The receiver verifies every delivery with
the secret that DCS returned for that subscription and answers 401 when the
signature does not verify, so the DCS delivery log shows the outcome too.
Like the checkpoint sink, it runs in the behave process and the cluster
reaches it through BDD_WEBHOOK_RECEIVER_HOST (the kind network gateway).
"""

from __future__ import annotations

import atexit
import base64
import hashlib
import hmac
import json
import os
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

TIMESTAMP_TOLERANCE_SECONDS = 300


def signature_verifies(secret: str, headers: dict, body: bytes) -> bool:
    msg_id = headers.get("webhook-id", "")
    timestamp = headers.get("webhook-timestamp", "")
    if not msg_id or not timestamp.isdigit():
        return False
    if abs(time.time() - int(timestamp)) > TIMESTAMP_TOLERANCE_SECONDS:
        return False
    key = base64.b64decode(secret.removeprefix("whsec_"))
    mac = hmac.new(key, f"{msg_id}.{timestamp}.".encode() + body, hashlib.sha256)
    expected = "v1," + base64.b64encode(mac.digest()).decode()
    return any(hmac.compare_digest(entry, expected) for entry in headers.get("webhook-signature", "").split(" "))


class _ReceiverState:
    def __init__(self) -> None:
        self.lock = threading.Lock()
        self.secrets: dict[str, str] = {}
        self.deliveries: list[dict] = []


class _ReceiverHandler(BaseHTTPRequestHandler):
    server_version = "FACISBDDWebhookReceiver/1"

    def log_message(self, _format, *_args) -> None:
        return

    def do_POST(self) -> None:  # noqa: N802
        state: _ReceiverState = self.server.receiver_state
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        headers = {name.lower(): value for name, value in self.headers.items()}
        with state.lock:
            secret = state.secrets.get(self.path)
        verified = bool(secret) and signature_verifies(secret, headers, body)
        try:
            payload = json.loads(body)
        except (UnicodeDecodeError, json.JSONDecodeError):
            payload = None
        with state.lock:
            state.deliveries.append({"path": self.path, "payload": payload, "verified": verified})
        self.send_response(200 if verified else 401)
        self.send_header("Content-Length", "0")
        self.end_headers()


class WebhookReceiverService:
    _lock = threading.Lock()
    _state = _ReceiverState()
    _server: ThreadingHTTPServer | None = None

    @classmethod
    def ensure_started(cls) -> None:
        with cls._lock:
            if cls._server is not None:
                return
            host = os.getenv("BDD_WEBHOOK_RECEIVER_BIND_HOST", "0.0.0.0")
            port = int(os.getenv("BDD_WEBHOOK_RECEIVER_PORT", "18444"))
            server = ThreadingHTTPServer((host, port), _ReceiverHandler)
            server.receiver_state = cls._state
            threading.Thread(target=server.serve_forever, name="facis-bdd-webhook-receiver", daemon=True).start()
            cls._server = server

    @classmethod
    def stop(cls) -> None:
        with cls._lock:
            server, cls._server = cls._server, None
        if server is not None:
            server.shutdown()
            server.server_close()

    @classmethod
    def new_endpoint(cls) -> tuple[str, str]:
        """Returns (path, callback URL as the cluster reaches it)."""
        cls.ensure_started()
        path = f"/webhooks/{uuid.uuid4()}"
        host = os.getenv("BDD_WEBHOOK_RECEIVER_HOST", "host.docker.internal")
        port = os.getenv("BDD_WEBHOOK_RECEIVER_PORT", "18444")
        return path, f"http://{host}:{port}{path}"

    @classmethod
    def set_secret(cls, path: str, secret: str) -> None:
        with cls._state.lock:
            cls._state.secrets[path] = secret

    @classmethod
    def deliveries(cls, path: str) -> list[dict]:
        with cls._state.lock:
            return [dict(d) for d in cls._state.deliveries if d["path"] == path]


atexit.register(WebhookReceiverService.stop)
