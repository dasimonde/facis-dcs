"""Round-trip against a stand-in that implements the ORCE issuer flow's exact
contract (deployment/helm/charts/orce/flows-issuer/flow-statuslist.json and
flow-admin.json): 1250 bytes of LSB-first bits, zlib-deflated and base64url'd
into a compact JWT whose `sub` is the list's own URI, and POST
/admin/credentials/<idx>/{revoke,unrevoke} answering 302 back to /admin.

What is being tested is not that a bit can be flipped but that flipping it is
DISTINGUISHABLE from the list being unusable — the failure that made every
revocation scenario in the suite pass for the wrong reason.
"""

from __future__ import annotations

import base64
import json
import threading
import unittest
import zlib
from http.server import BaseHTTPRequestHandler, HTTPServer

from dcs_wallet.status_list import (
    bit_is_revoked,
    encoded_list_from_claims,
    fetch_status_list,
    revoke_and_prove,
)

BITS_LEN = 1250


class _IssuerState:
    def __init__(self) -> None:
        self.bits = bytearray(BITS_LEN)
        self.subject_override: str | None = None
        self.apply_admin_calls = True


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


class _Handler(BaseHTTPRequestHandler):
    state: _IssuerState

    def log_message(self, *_args):  # noqa: A003 — silence the test server
        pass

    def _base(self) -> str:
        return f"http://{self.headers.get('Host')}"

    def do_GET(self):  # noqa: N802 — BaseHTTPRequestHandler's spelling
        if self.path != "/status-list/1":
            self.send_error(404)
            return
        uri = self.state.subject_override or f"{self._base()}/status-list/1"
        header = {"alg": "ES256", "typ": "statuslist+jwt"}
        payload = {
            "iss": self._base(),
            "sub": uri,
            "iat": 1577836800,
            "status_list": {
                "bits": 1,
                "lst": _b64url(zlib.compress(bytes(self.state.bits))),
            },
        }
        # The signature is never checked by these helpers (only the DCS backend
        # verifies it, against its configured anchors), so a placeholder keeps
        # the compact three-part shape without a key.
        token = f"{_b64url(json.dumps(header).encode())}.{_b64url(json.dumps(payload).encode())}.c2ln"
        body = token.encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/statuslist+jwt")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):  # noqa: N802
        parts = self.path.strip("/").split("/")
        if len(parts) != 4 or parts[0] != "admin" or parts[1] != "credentials":
            self.send_error(404)
            return
        idx, action = int(parts[2]), parts[3]
        if self.state.apply_admin_calls:
            byte_idx, bit_idx = idx >> 3, idx % 8
            if action == "revoke":
                self.state.bits[byte_idx] |= 1 << bit_idx
            else:
                self.state.bits[byte_idx] &= ~(1 << bit_idx)
        self.send_response(302)
        self.send_header("Location", "/admin")
        self.send_header("Content-Length", "0")
        self.end_headers()


class RevocationProofTest(unittest.TestCase):
    def setUp(self) -> None:
        self.state = _IssuerState()
        handler = type("BoundHandler", (_Handler,), {"state": self.state})
        self.server = HTTPServer(("127.0.0.1", 0), handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)
        host, port = self.server.server_address
        self.uri = f"http://{host}:{port}/status-list/1"

    def _served_bits(self) -> bytes:
        encoded = encoded_list_from_claims(fetch_status_list(self.uri))
        return zlib.decompress(base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4)))

    def test_revoking_sets_that_bit_and_leaves_its_neighbours_alone(self) -> None:
        revoke_and_prove(1010, status_list_uri=self.uri)
        encoded = encoded_list_from_claims(fetch_status_list(self.uri))
        self.assertTrue(bit_is_revoked(encoded, 1010))
        self.assertFalse(bit_is_revoked(encoded, 1009))
        self.assertFalse(bit_is_revoked(encoded, 1011))
        self.assertEqual(len(self._served_bits()), BITS_LEN)

    def test_the_bit_is_the_one_the_backend_reads(self) -> None:
        # LSB-first within the byte, matching codec.LSBFirst in
        # handler/ietf_token.go. Read back off the raw bytes rather than
        # through bit_is_revoked, which would agree with itself either way.
        revoke_and_prove(2001, status_list_uri=self.uri)
        self.assertEqual(self._served_bits()[2001 >> 3], 1 << (2001 % 8))

    def test_an_index_a_previous_run_revoked_is_cleared_first(self) -> None:
        # The issuer persists bits on its volume, so a reserved index stays
        # revoked between runs. Without the clear, the assertion after the
        # admin call would pass without the call having done anything.
        self.state.bits[2000 >> 3] |= 1 << (2000 % 8)
        revoke_and_prove(2000, status_list_uri=self.uri)
        self.assertTrue(bit_is_revoked(encoded_list_from_claims(fetch_status_list(self.uri)), 2000))

    def test_an_issuer_that_ignores_admin_calls_fails_instead_of_passing(self) -> None:
        self.state.apply_admin_calls = False
        with self.assertRaises(RuntimeError) as caught:
            revoke_and_prove(2000, status_list_uri=self.uri)
        self.assertIn("still reads active", str(caught.exception))

    def test_a_list_served_under_a_different_uri_is_refused_not_believed(self) -> None:
        # The verifier requires sub == the credential's URI, so this list would
        # refuse every credential naming it — a refusal with nothing revoked.
        self.state.subject_override = "http://elsewhere.invalid/status-list/1"
        with self.assertRaises(RuntimeError) as caught:
            revoke_and_prove(2000, status_list_uri=self.uri)
        self.assertIn("ISSUER_BASE_URL", str(caught.exception))

    def test_an_unreachable_issuer_fails_the_revocation_not_the_presentation(self) -> None:
        self.server.shutdown()
        self.server.server_close()
        with self.assertRaises(OSError):
            revoke_and_prove(2000, status_list_uri=self.uri, timeout=2.0)


if __name__ == "__main__":
    unittest.main()
