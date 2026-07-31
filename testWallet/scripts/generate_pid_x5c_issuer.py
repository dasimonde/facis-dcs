#!/usr/bin/env python3
"""Mint the dev PID issuer that signs with an x5c certificate chain.

  python3 testWallet/scripts/generate_pid_x5c_issuer.py

A real EUDI wallet's PID carries its issuer's certificate chain, and DCS
resolves such an issuer's key from that chain verified to a configured anchor
(OID4VP_X5C_TRUST_ANCHORS_PATH). The chain proves an anchor vouched for the
leaf; binding it to an identity is a separate check
(backend/internal/auth/oid4vp/sdjwt/keys.go, leafIdentifiesIssuer), which is why
the certificate carries the issuer DID as a URI SAN: a did:web authority may
hold a port, which decodes to host:port and can never equal a DNS name.

The certificate is its own trust anchor, so the same bytes are written to both
the wallet's key directory and the anchor bundle the BDD deployment mounts.
Keeps an existing private key unless --regenerate is given, so reissuing the
certificate does not invalidate credentials already minted with that key.
"""

from __future__ import annotations

import argparse
import datetime as dt
import sys
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

WALLET_ROOT = Path(__file__).resolve().parent.parent
REPO_ROOT = WALLET_ROOT.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.keys import generate_ec_private_jwk, load_json, private_key_material, public_jwk, write_json  # noqa: E402

ISSUER_DID = "did:web:dev.example:issuer:pid-x5c"
SUBJECT_CN = "DCS Dev PID Issuer (x5c, DEV ONLY)"
VALID_YEARS = 10


def _private_key(jwk: dict) -> ec.EllipticCurvePrivateKey:
    material = private_key_material(jwk)
    numbers = ec.EllipticCurvePrivateNumbers(
        private_value=int.from_bytes(_b64url(material["d"]), "big"),
        public_numbers=ec.EllipticCurvePublicNumbers(
            x=int.from_bytes(_b64url(material["x"]), "big"),
            y=int.from_bytes(_b64url(material["y"]), "big"),
            curve=ec.SECP256R1(),
        ),
    )
    return numbers.private_key()


def _b64url(value: str) -> bytes:
    import base64

    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


def mint(*, keys_dir: Path, anchors_path: Path, regenerate: bool) -> None:
    jwk_path = keys_dir / "issuer-dev-x5c.jwk"
    if regenerate or not jwk_path.is_file():
        issuer_jwk = generate_ec_private_jwk()
    else:
        issuer_jwk = private_key_material(load_json(jwk_path))

    key = _private_key(issuer_jwk)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, SUBJECT_CN)])
    now = dt.datetime.now(dt.timezone.utc)
    certificate = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - dt.timedelta(days=1))
        .not_valid_after(now + dt.timedelta(days=365 * VALID_YEARS))
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=False,
                crl_sign=False,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .add_extension(x509.SubjectAlternativeName([x509.UniformResourceIdentifier(ISSUER_DID)]), critical=False)
        .sign(key, hashes.SHA256())
    )

    pem = certificate.public_bytes(serialization.Encoding.PEM)
    write_json(jwk_path, issuer_jwk)
    write_json(keys_dir / "issuer-dev-x5c.public.jwk", public_jwk(issuer_jwk))
    (keys_dir / "issuer-dev-x5c.crt.pem").write_bytes(pem)
    anchors_path.write_bytes(pem)

    print(f"issuer: {ISSUER_DID}")
    print(f"cert:   {keys_dir / 'issuer-dev-x5c.crt.pem'}")
    print(f"anchor: {anchors_path}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Mint the dev x5c PID issuer certificate")
    parser.add_argument("--keys-dir", type=Path, default=WALLET_ROOT / "keys")
    parser.add_argument(
        "--anchors-path",
        type=Path,
        default=REPO_ROOT / "backend" / "config" / "oid4vp" / "x5c-trust-anchors.dev.pem",
    )
    parser.add_argument("--regenerate", action="store_true", help="replace the issuer private key too")
    args = parser.parse_args()

    mint(keys_dir=args.keys_dir, anchors_path=args.anchors_path, regenerate=args.regenerate)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
