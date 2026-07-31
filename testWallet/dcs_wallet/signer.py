"""Remote signing primitives — the testWallet as the TSP/CSC stand-in.

Under the eIDAS AES model the SIGNATORY's key is held solely by them (their
wallet/TSP), never by the DCS (DCS-IR-SI-04, SM-06, SM-16). This module makes
the testWallet a remote signer: each signatory has their OWN signing key and an
X.509 certificate issued by a wallet-side dev CA (the "QTSP" of the demo). The
wallet signs the data-to-be-signed (DTBS) the DCS's SCA (EU DSS) computes; the
DCS never touches a contract-signing key.

Sole control is structural here: distinct key per signatory, held wallet-side.
"""

from __future__ import annotations

import base64
import datetime as _dt
import fcntl
from pathlib import Path
from typing import Any

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

from dcs_wallet.keys import generate_ec_private_jwk, load_json, write_json, write_text

_SIGNING_DIR = "signing"  # under the wallet keys dir


def _ec_private_from_jwk(jwk: dict[str, Any]) -> ec.EllipticCurvePrivateKey:
    def _int(field: str) -> int:
        raw = base64.urlsafe_b64decode(str(jwk[field]) + "=" * (-len(str(jwk[field])) % 4))
        return int.from_bytes(raw, "big")

    return ec.derive_private_key(_int("d"), ec.SECP256R1())


def _load_wallet_ca(ca_jwk_path: Path, ca_crt_path: Path) -> tuple[ec.EllipticCurvePrivateKey, x509.Certificate]:
    ca_key = _ec_private_from_jwk(load_json(ca_jwk_path))
    ca_cert = x509.load_pem_x509_certificate(ca_crt_path.read_bytes())
    return ca_key, ca_cert


def _wallet_ca(keys_dir: Path) -> tuple[ec.EllipticCurvePrivateKey, x509.Certificate]:
    """Load (or create) the wallet-side dev CA that issues signatory certs.

    This CA belongs to the WALLET/TSP, not the DCS — the trust anchor for the
    signatory's own signing certificate. Persisted so re-runs keep one CA.

    Playwright runs multiple specs concurrently as separate OS processes, all
    signing against this same shared keys_dir (frontend/ClientApp/e2e's
    sign_prepared_pdf.py subprocesses) - a plain check-then-create here raced:
    two processes could each see no CA yet, mint DIFFERENT CA keypairs, and
    the second write would clobber the first's CA file while a leaf cert
    already signed against the first (now-overwritten) CA key was still in
    flight, producing a leaf cert whose issuer name matches the file on disk
    but whose signature doesn't verify against it. flock + a re-check after
    acquiring the lock closes that window: a process that loses the race to
    mint waits, then finds the CA the winner already persisted.
    """
    ca_dir = keys_dir / _SIGNING_DIR
    ca_dir.mkdir(parents=True, exist_ok=True)
    ca_jwk_path = ca_dir / "wallet-ca.jwk"
    ca_crt_path = ca_dir / "wallet-ca.crt.pem"

    if ca_jwk_path.exists() and ca_crt_path.exists():
        return _load_wallet_ca(ca_jwk_path, ca_crt_path)

    lock_path = ca_dir / ".wallet-ca.lock"
    with lock_path.open("w") as lock_fh:
        fcntl.flock(lock_fh, fcntl.LOCK_EX)
        try:
            if ca_jwk_path.exists() and ca_crt_path.exists():
                return _load_wallet_ca(ca_jwk_path, ca_crt_path)

            ca_key = ec.generate_private_key(ec.SECP256R1())
            subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "DCS Wallet Dev Signing CA")])
            now = _dt.datetime.now(_dt.timezone.utc)
            ca_cert = (
                x509.CertificateBuilder()
                .subject_name(subject)
                .issuer_name(issuer)
                .public_key(ca_key.public_key())
                .serial_number(x509.random_serial_number())
                .not_valid_before(now - _dt.timedelta(hours=1))
                .not_valid_after(now + _dt.timedelta(days=3650))
                .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
                .add_extension(x509.KeyUsage(
                    digital_signature=False, content_commitment=False, key_encipherment=False,
                    data_encipherment=False, key_agreement=False, key_cert_sign=True, crl_sign=True,
                    encipher_only=False, decipher_only=False), critical=True)
                .sign(ca_key, hashes.SHA256())
            )
            # Persist the CA (private JWK + cert).
            d = ca_key.private_numbers()
            pub = d.public_numbers
            coord = 32
            write_json(ca_jwk_path, {
                "kty": "EC", "crv": "P-256",
                "x": base64.urlsafe_b64encode(pub.x.to_bytes(coord, "big")).rstrip(b"=").decode(),
                "y": base64.urlsafe_b64encode(pub.y.to_bytes(coord, "big")).rstrip(b"=").decode(),
                "d": base64.urlsafe_b64encode(d.private_value.to_bytes(coord, "big")).rstrip(b"=").decode(),
            })
            write_text(ca_crt_path, ca_cert.public_bytes(serialization.Encoding.PEM).decode())
            return ca_key, ca_cert
        finally:
            fcntl.flock(lock_fh, fcntl.LOCK_UN)


def ensure_signing_material(
    user: str, keys_dir: Path, *, given_name: str | None = None, family_name: str | None = None,
) -> tuple[dict[str, Any], bytes]:
    """Return (private signing JWK, leaf cert DER) for `user`, minting both on
    first use. Distinct key per signatory — the crux of sole control.

    given_name/family_name are embedded in the leaf certificate's subject as
    GIVEN_NAME/SURNAME (eIDAS Annex I; ADR-20's cert-subject to PID name-match
    gate reads exactly these RDN attributes). Default to (user, "BDD-Testperson")
    — the established BDD PID convention (given_name=signatory label,
    family_name=the fixed test-person surname, see
    dcs_real_signing_vertical_steps._build_pid_presentation) — so every
    existing caller keeps matching without changes; pass explicit values to
    mint a signer whose certificate deliberately does NOT match its PID (the
    negative test), under its own distinct `user` label so it never collides
    with the matching identity's cached key/cert.
    """
    given_name = given_name if given_name is not None else user
    family_name = family_name if family_name is not None else "BDD-Testperson"

    sdir = keys_dir / _SIGNING_DIR
    sdir.mkdir(parents=True, exist_ok=True)
    jwk_path = sdir / f"{user}.signing.jwk"
    crt_path = sdir / f"{user}.signing.crt.pem"

    def _load() -> tuple[dict[str, Any], bytes] | None:
        if jwk_path.exists() and crt_path.exists():
            jwk = load_json(jwk_path)
            cert = x509.load_pem_x509_certificate(crt_path.read_bytes())
            return jwk, cert.public_bytes(serialization.Encoding.DER)
        return None

    cached = _load()
    if cached is not None:
        return cached

    # Per-user lock (a distinct file from _wallet_ca's own lock, so this never
    # nests with it): concurrent Playwright specs mint different users' certs
    # against this same shared keys_dir, and a plain check-then-create here
    # would let two processes race the same {user}.signing.{jwk,crt.pem} pair
    # the same way _wallet_ca's CA file could (see its docstring).
    lock_path = sdir / f".{user}.lock"
    with lock_path.open("w") as lock_fh:
        fcntl.flock(lock_fh, fcntl.LOCK_EX)
        try:
            cached = _load()
            if cached is not None:
                return cached

            jwk = generate_ec_private_jwk()
            user_key = _ec_private_from_jwk(jwk)
            ca_key, ca_cert = _wallet_ca(keys_dir)
            now = _dt.datetime.now(_dt.timezone.utc)
            subject = x509.Name([
                x509.NameAttribute(NameOID.COMMON_NAME, f"DCS Signatory {user}"),
                x509.NameAttribute(NameOID.GIVEN_NAME, given_name),
                x509.NameAttribute(NameOID.SURNAME, family_name),
            ])
            leaf = (
                x509.CertificateBuilder()
                .subject_name(subject)
                .issuer_name(ca_cert.subject)
                .public_key(user_key.public_key())
                .serial_number(x509.random_serial_number())
                .not_valid_before(now - _dt.timedelta(hours=1))
                .not_valid_after(now + _dt.timedelta(days=825))
                .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
                .add_extension(x509.KeyUsage(
                    digital_signature=True, content_commitment=True, key_encipherment=False,
                    data_encipherment=False, key_agreement=False, key_cert_sign=False, crl_sign=False,
                    encipher_only=False, decipher_only=False), critical=True)
                .add_extension(x509.ExtendedKeyUsage([x509.ExtendedKeyUsageOID.CODE_SIGNING]), critical=False)
                .sign(ca_key, hashes.SHA256())
            )
            write_json(jwk_path, jwk)
            write_text(crt_path, leaf.public_bytes(serialization.Encoding.PEM).decode())
            return jwk, leaf.public_bytes(serialization.Encoding.DER)
        finally:
            fcntl.flock(lock_fh, fcntl.LOCK_UN)


def sign_dtbs(dtbs: bytes, signing_jwk: dict[str, Any]) -> bytes:
    """Sign the DSS data-to-be-signed with the signatory's key. Returns the DER
    ECDSA signature the DSS signDocument call embeds (algorithm ECDSA_SHA256).
    """
    key = _ec_private_from_jwk(signing_jwk)
    return key.sign(dtbs, ec.ECDSA(hashes.SHA256()))
