#!/usr/bin/env python3
"""Mint the root CA the dev and BDD ORCE credential issuer signs under.

An ORCE issuer generates its own root CA on first boot and keeps it on its
volume (charts/orce/flows-issuer/flow-pki.json). That is the right shape for a
real deployment: the root is created once, and a verifier is handed its
fingerprint afterwards — which is what tmp/redeploy/build-x5c-anchors.py reads
back out of the status list each issuer is serving. A dev or CI stack cannot do
that. It is rebuilt from nothing, its verifier configuration is committed, and
its backend reads the anchors before any issuer has booted. Worse, every
runtime-generated root is called "FACIS Demo Root CA" whatever key it holds, so
one cannot be told from another by name.

So the dev and BDD issuer is handed THIS root instead of generating one
(orce.pkiRootCA.devFixture), and the same certificate goes into the anchor
bundle the backend mounts. The private key is committed on purpose; it is
recognised as repo-committed material and refused as an anchor unless
DCS_ALLOW_DEV_TRUST says the stack is a development one.

  python3 scripts/orce-dev-root-ca.py [--regenerate]

Keeps the existing key unless --regenerate: replacing it invalidates every
certificate the issuer already minted under it, and the x-coordinate printed at
the end has to be carried into devIssuerKeySources
(backend/internal/auth/oid4vp/trust_config.go) or the committed-key guard no
longer recognises it.
"""

from __future__ import annotations

import argparse
import base64
import datetime as dt
import sys
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

REPO_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(REPO_ROOT / "scripts"))

from x5c_anchor_bundle import (  # noqa: E402
    fingerprint,
    load_certificate,
    print_bundle,
    upsert_anchor,
)

FIXTURE_DIR = REPO_ROOT / "deployment" / "helm" / "charts" / "orce" / "pki-dev"
ANCHORS_PATH = REPO_ROOT / "backend" / "config" / "oid4vp" / "x5c-trust-anchors.dev.pem"

SUBJECT_CN = "FACIS Dev Issuer Root CA (DEV ONLY)"

# The issuer container's clock can be far off real-world time, and the flow
# backdates its own certificates for that reason (flow-pki.json). Match it: a
# notBefore stamped at generation time is "not yet valid" to anything running on
# an earlier clock, and a verifier rejects the whole chain for it.
NOT_BEFORE = dt.datetime(2020, 1, 1, tzinfo=dt.timezone.utc)
NOT_AFTER = dt.datetime(2040, 1, 1, tzinfo=dt.timezone.utc)


def load_or_generate_key(key_path: Path, regenerate: bool) -> ec.EllipticCurvePrivateKey:
    if not regenerate and key_path.is_file():
        return serialization.load_pem_private_key(key_path.read_bytes(), password=None)
    return ec.generate_private_key(ec.SECP256R1())


def mint(*, fixture_dir: Path, anchors_path: Path, regenerate: bool) -> None:
    key_path = fixture_dir / "root-ca.key"
    cert_path = fixture_dir / "root-ca.crt"

    fixture_dir.mkdir(parents=True, exist_ok=True)
    # Read before writing: this is the anchor the bundle currently holds for this
    # issuer, and the only thing that says which entry is ours. Every runtime
    # ORCE root shares its subject with this one, so a bundle maintained by name
    # would drop somebody else's.
    previous = load_certificate(cert_path)
    key = load_or_generate_key(key_path, regenerate)

    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, SUBJECT_CN)])
    certificate = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(NOT_BEFORE)
        .not_valid_after(NOT_AFTER)
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=False,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=True,
                crl_sign=True,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .sign(key, hashes.SHA256())
    )

    key_path.write_bytes(
        key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )
    )
    cert_path.write_bytes(certificate.public_bytes(serialization.Encoding.PEM))

    anchors = upsert_anchor(anchors_path, certificate, replacing=previous)

    coordinate = key.public_key().public_numbers().x.to_bytes(32, "big")
    print(f"root CA:     {cert_path}")
    print(f"fingerprint: {fingerprint(certificate)}")
    print(f"key x:       {base64.urlsafe_b64encode(coordinate).rstrip(b'=').decode()}")
    print_bundle(anchors_path, anchors)


def main() -> int:
    parser = argparse.ArgumentParser(description="Mint the dev/BDD ORCE issuer root CA")
    parser.add_argument("--fixture-dir", type=Path, default=FIXTURE_DIR)
    parser.add_argument("--anchors-path", type=Path, default=ANCHORS_PATH)
    parser.add_argument(
        "--regenerate",
        action="store_true",
        help="replace the private key too, invalidating everything issued under it",
    )
    args = parser.parse_args()

    mint(
        fixture_dir=args.fixture_dir,
        anchors_path=args.anchors_path,
        regenerate=args.regenerate,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
