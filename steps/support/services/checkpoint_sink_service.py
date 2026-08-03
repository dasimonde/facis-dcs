"""In-process HTTPS reference sink for external audit checkpoints."""

from __future__ import annotations

import atexit
import json
import os
import shutil
import socket
import ssl
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class _CheckpointSinkState:
    def __init__(self) -> None:
        self.lock = threading.Lock()
        self.mode = "accept"
        self.attempts: list[dict] = []
        self.records: list[dict] = []
        self.by_idempotency_key: dict[str, dict] = {}

    def reset(self) -> None:
        with self.lock:
            self.mode = "accept"
            self.attempts.clear()
            self.records.clear()
            self.by_idempotency_key.clear()

    def set_mode(self, mode: str) -> None:
        with self.lock:
            self.mode = mode


class _CheckpointSinkHandler(BaseHTTPRequestHandler):
    server_version = "FACISBDDCheckpointSink/1"

    def log_message(self, _format, *_args) -> None:
        return

    def _json_response(self, status: int, body: dict) -> None:
        encoded = json.dumps(body, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def do_POST(self) -> None:  # noqa: N802
        state: _CheckpointSinkState = self.server.checkpoint_state
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        try:
            payload = json.loads(raw)
        except (UnicodeDecodeError, json.JSONDecodeError):
            self._json_response(400, {"error": "malformed_json"})
            return
        headers = {name.lower(): value for name, value in self.headers.items()}
        attempt = {
            "path": self.path,
            "headers": headers,
            "payload": payload,
        }
        with state.lock:
            state.attempts.append(attempt)
            if self.path != "/checkpoints":
                self._json_response(404, {"error": "wrong_path"})
                return
            if headers.get("authorization") != "Bearer bdd-checkpoint-sink-token":
                self._json_response(401, {"error": "invalid_bearer_token"})
                return
            key = headers.get("idempotency-key", "").strip()
            if not key:
                self._json_response(400, {"error": "missing_idempotency_key"})
                return
            previous = state.by_idempotency_key.get(key)
            if previous is not None:
                if previous != payload:
                    self._json_response(
                        409,
                        {"error": "idempotency_key_payload_mismatch"},
                    )
                    return
            elif state.mode == "sequence_gap":
                self._json_response(409, {"error": "sequence_gap"})
                return
            elif state.mode == "previous_root_mismatch":
                self._json_response(409, {"error": "previous_root_mismatch"})
                return
            else:
                if state.records:
                    last = state.records[-1]
                    if int(payload.get("seq", -1)) != int(last["seq"]) + 1:
                        self._json_response(409, {"error": "sequence_gap"})
                        return
                    if payload.get("prev_root") != last.get("root"):
                        self._json_response(
                            409,
                            {"error": "previous_root_mismatch"},
                        )
                        return
                record = dict(payload)
                state.records.append(record)
                state.by_idempotency_key[key] = record
            lose_response = state.mode == "lost_response"
        if lose_response:
            self.close_connection = True
            try:
                self.connection.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            self.connection.close()
            return
        self._json_response(200, {"accepted": True})


class CheckpointSinkService:
    """Lifecycle and observations for the real HTTPS BDD checkpoint sink."""

    _lock = threading.Lock()
    _state = _CheckpointSinkState()
    _server: ThreadingHTTPServer | None = None
    _thread: threading.Thread | None = None
    _temp_dir: str | None = None

    @classmethod
    def _certificate(cls, temp_dir: str) -> tuple[str, str]:
        cert = os.path.join(temp_dir, "checkpoint-sink.crt")
        key = os.path.join(temp_dir, "checkpoint-sink.key")
        result = subprocess.run(
            [
                "openssl",
                "req",
                "-x509",
                "-newkey",
                "rsa:2048",
                "-nodes",
                "-days",
                "1",
                "-subj",
                "/CN=host.docker.internal",
                "-addext",
                "subjectAltName=DNS:host.docker.internal,DNS:localhost,IP:127.0.0.1",
                "-keyout",
                key,
                "-out",
                cert,
            ],
            capture_output=True,
            text=True,
            timeout=30,
        )
        assert result.returncode == 0, (
            f"Could not create the BDD checkpoint sink certificate: {result.stderr}"
        )
        return cert, key

    @classmethod
    def ensure_started(cls) -> None:
        with cls._lock:
            if cls._server is not None:
                return
            host = os.getenv("BDD_CHECKPOINT_SINK_BIND_HOST", "0.0.0.0")
            port = int(os.getenv("BDD_CHECKPOINT_SINK_PORT", "18443"))
            temp_dir = tempfile.mkdtemp(prefix="facis-bdd-checkpoint-sink-")
            server = None
            try:
                cert, key = cls._certificate(temp_dir)
                server = ThreadingHTTPServer((host, port), _CheckpointSinkHandler)
                server.checkpoint_state = cls._state
                tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
                tls.load_cert_chain(certfile=cert, keyfile=key)
                server.socket = tls.wrap_socket(server.socket, server_side=True)
            except Exception:
                if server is not None:
                    server.server_close()
                shutil.rmtree(temp_dir, ignore_errors=True)
                raise
            thread = threading.Thread(
                target=server.serve_forever,
                name="facis-bdd-checkpoint-sink",
                daemon=True,
            )
            thread.start()
            cls._server = server
            cls._thread = thread
            cls._temp_dir = temp_dir

    @classmethod
    def stop(cls) -> None:
        with cls._lock:
            server, thread, temp_dir = cls._server, cls._thread, cls._temp_dir
            cls._server = None
            cls._thread = None
            cls._temp_dir = None
        if server is not None:
            server.shutdown()
            server.server_close()
        if thread is not None:
            thread.join(timeout=5)
        if temp_dir:
            shutil.rmtree(temp_dir, ignore_errors=True)

    @classmethod
    def reset(cls) -> None:
        cls.ensure_started()
        cls._state.reset()

    @classmethod
    def set_mode(cls, mode: str) -> None:
        cls.ensure_started()
        cls._state.set_mode(mode)

    @classmethod
    def observations(cls) -> list[dict]:
        with cls._state.lock:
            return [
                {
                    "path": attempt["path"],
                    "headers": dict(attempt["headers"]),
                    "payload": dict(attempt["payload"]),
                }
                for attempt in cls._state.attempts
            ]

    @classmethod
    def status(cls) -> dict:
        with cls._state.lock:
            return {
                "mode": cls._state.mode,
                "records": [dict(record) for record in cls._state.records],
                "attempt_count": len(cls._state.attempts),
            }


atexit.register(CheckpointSinkService.stop)
