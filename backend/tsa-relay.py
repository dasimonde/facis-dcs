#!/usr/bin/env python3
"""Local dev-only self-signed TSA stub, signing RFC3161 timestamps locally.

Serves two request shapes against the same self-signed dev CA/cert (see
backend/tsa-relay-test.sh for how those were generated under /tmp/local-tsa):

  * GET /tsa/:hash — backend's TSA_URL contract (backend/internal/base/tsa/
    tsa.go): builds an RFC3161 TSQ from the hash via openssl, returns a TSR.
  * POST — the standard RFC3161 contract external PAdES signers use (e.g.
    pyhanko in the BDD signing harness): the request body is a DER-encoded
    TimeStampReq (application/timestamp-query), the response an
    application/timestamp-reply TSR (PAdES-B-T).

Both avoid forwarding to a real https://freetsa.org/tsr.

Backend's TSA client (backend/internal/base/tsa/tsa.go,
verifyTimestampForData) only checks that the returned token's hashed
message matches the request — it does not validate the signer's cert
chain against any trust store — so a self-signed TSA cert round-trips
successfully with no backend changes required.

This also fixes the real Rancher Desktop egress block (this WSL host has
internet, the cluster pods don't) AND removes per-call network latency to
a third-party service, which was the bottleneck draining the outbox
backlog at ~4-5 events/sec.
"""

import re
import subprocess
import tempfile
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

TSA_DIR = "/tmp/local-tsa"
SIGNER_CERT = os.path.join(TSA_DIR, "tsa-cert.pem")
SIGNER_KEY = os.path.join(TSA_DIR, "tsa-key.pem")
CHAIN_CERT = os.path.join(TSA_DIR, "tsa-ca-cert.pem")


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        m = re.match(r"^/tsa/([a-fA-F0-9]{64})$", self.path)
        if not m:
            self.send_response(400)
            self.end_headers()
            self.wfile.write(b"Invalid SHA-256 hash")
            return

        hash_hex = m.group(1)
        with tempfile.TemporaryDirectory() as tmp:
            tsq_path = os.path.join(tmp, "request.tsq")
            tsr_path = os.path.join(tmp, "response.tsr")

            query = subprocess.run(
                ["openssl", "ts", "-query", "-digest", hash_hex, "-sha256", "-cert", "-out", tsq_path],
                capture_output=True,
            )
            if query.returncode != 0:
                self.send_response(500)
                self.end_headers()
                self.wfile.write(query.stderr)
                return

            reply = subprocess.run(
                [
                    "openssl", "ts", "-reply",
                    "-queryfile", tsq_path,
                    "-signer", SIGNER_CERT,
                    "-inkey", SIGNER_KEY,
                    "-chain", CHAIN_CERT,
                    "-out", tsr_path,
                ],
                capture_output=True,
            )
            if reply.returncode != 0 or not os.path.exists(tsr_path):
                self.send_response(500)
                self.end_headers()
                self.wfile.write(reply.stderr)
                return

            with open(tsr_path, "rb") as f:
                tsr_bytes = f.read()

        self.send_response(200)
        self.send_header("Content-Type", "application/timestamp-reply")
        self.end_headers()
        self.wfile.write(tsr_bytes)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        tsq_bytes = self.rfile.read(length) if length else b""
        if not tsq_bytes:
            self.send_response(400)
            self.end_headers()
            self.wfile.write(b"Empty RFC3161 timestamp query")
            return

        with tempfile.TemporaryDirectory() as tmp:
            tsq_path = os.path.join(tmp, "request.tsq")
            tsr_path = os.path.join(tmp, "response.tsr")
            with open(tsq_path, "wb") as f:
                f.write(tsq_bytes)

            reply = subprocess.run(
                [
                    "openssl", "ts", "-reply",
                    "-queryfile", tsq_path,
                    "-signer", SIGNER_CERT,
                    "-inkey", SIGNER_KEY,
                    "-chain", CHAIN_CERT,
                    "-out", tsr_path,
                ],
                capture_output=True,
            )
            if reply.returncode != 0 or not os.path.exists(tsr_path):
                self.send_response(500)
                self.end_headers()
                self.wfile.write(reply.stderr)
                return

            with open(tsr_path, "rb") as f:
                tsr_bytes = f.read()

        self.send_response(200)
        self.send_header("Content-Type", "application/timestamp-reply")
        self.end_headers()
        self.wfile.write(tsr_bytes)

    def log_message(self, fmt, *args):
        print(f"[tsa-relay] {self.address_string()} {fmt % args}")


if __name__ == "__main__":
    HTTPServer(("127.0.0.1", 9091), Handler).serve_forever()
