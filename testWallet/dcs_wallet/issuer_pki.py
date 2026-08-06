"""The identity this wallet issues credentials under: the stack's ORCE issuer.

A credential and the status list it points at are one issuer's statement, and
the verifier holds them to that — a list must be issued by the same issuer as
the credential whose status it carries
(backend/internal/auth/oid4vp/status/credentialbinding.go). The only thing in a
dev or BDD stack that publishes a status list is the ORCE credential issuer, so
a credential naming anyone else has a status claim nobody signs, and is refused
with the list unread.

So this wallet mints AS that issuer: `iss` is the issuer's base URL, the same
string its status list carries in `iss` and builds its `sub` from
(deployment/helm/charts/orce/flows-issuer/flow-statuslist.json), and the
credential carries the certificate chain that issuer signs with — which is what
the trust entry for it declares (mechanism x5c).

Both halves of that chain are committed. The ORCE issuer of a dev or BDD stack
is handed its root CA and its own signing key rather than generating them
(deployment/helm/charts/orce/pki-dev, orce.pkiRootCA.devFixture), because a
verifier cannot anchor a root that does not exist until the issuer has booted
and cannot pin a login issuer's leaf key it has never seen. Holding the same
files, this wallet can mint the leaf itself, exactly as ensureIssuerCertFor does
on the issuer's first request — no round trip to a running issuer, and the leaf
carries the key backend/config/oid4vp/trust.dev.json pins.

DEV ONLY. Every private key here is in the repository, which is why the backend
refuses to trust any of it unless DCS_ALLOW_DEV_TRUST says the stack is one.
"""

from __future__ import annotations

import base64
import datetime as dt
import hashlib
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import quote

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

from dcs_wallet.keys import b64url_uint
from dcs_wallet.status_list import issuer_base_url

# The fixture the dev and BDD issuer releases mount (orce.pkiRootCA.devFixture).
PKI_DEV_DIR = Path(__file__).resolve().parents[2] / "deployment" / "helm" / "charts" / "orce" / "pki-dev"

# What the ORCE flow puts in the leaf's subject, kept identical so a certificate
# minted here is the same certificate that issuer would serve.
ISSUER_SUBJECT_CN = "FACIS Demo Issuer"

# The ORCE flow backdates its certificates to 2020 with faketime, because a
# container clock running ahead stamps a notBefore that every verifier on a real
# device reads as "not yet valid". Fixed dates here for the same reason, and so
# that a reissued leaf is byte-identical to the last one.
NOT_BEFORE = dt.datetime(2020, 1, 1, tzinfo=dt.timezone.utc)
NOT_AFTER = dt.datetime(2040, 1, 1, tzinfo=dt.timezone.utc)


@dataclass(frozen=True)
class DevIssuer:
    """One issuer identity: what it is called, what it signs with, what it shows."""

    iss: str
    private_jwk: dict[str, Any]
    # base64 DER, leaf first (RFC 7515 §4.1.6) — the JWS x5c header verbatim.
    x5c: list[str]
    leaf: x509.Certificate


def issuer_did_for(base_url: str) -> str:
    """The did:web form of an issuer base URL.

    Same derivation as the ORCE flow's issuerDidFor: authority first, then each
    path segment, colon-separated (w3c-ccg did-method-web). An authority with a
    port is percent-encoded, so did:web:localhost%3A18080:issuer is one
    identifier and not a host called "localhost" on a path called "18080".
    """
    rest = base_url.split("://", 1)[-1]
    parts = [segment for segment in rest.split("/") if segment]
    authority = quote(parts[0], safe="")
    return ":".join(["did:web", authority, *parts[1:]])


def _private_jwk(key: ec.EllipticCurvePrivateKey) -> dict[str, Any]:
    numbers = key.private_numbers()
    public = numbers.public_numbers
    size = (key.curve.key_size + 7) // 8
    return {
        "kty": "EC",
        "crv": "P-256",
        "x": b64url_uint(public.x, size),
        "y": b64url_uint(public.y, size),
        "d": b64url_uint(numbers.private_value, size),
    }


def leaf_public_jwk(x5c_leaf: str) -> dict[str, Any]:
    """The public JWK of an x5c chain's leaf (base64 DER), which is the key that
    verifies the JWS carrying that chain."""
    certificate = x509.load_der_x509_certificate(base64.b64decode(x5c_leaf))
    public = certificate.public_key().public_numbers()
    size = (certificate.public_key().curve.key_size + 7) // 8
    return {
        "kty": "EC",
        "crv": "P-256",
        "x": b64url_uint(public.x, size),
        "y": b64url_uint(public.y, size),
    }


def _read_pem(name: str) -> bytes:
    path = PKI_DEV_DIR / name
    if not path.is_file():
        raise FileNotFoundError(
            f"{path} is missing. It is the key material the dev and BDD ORCE issuer "
            f"is handed instead of generating its own (orce.pkiRootCA.devFixture), "
            f"and this wallet issues under the same identity — without it a "
            f"credential could only name an issuer that signs no status list."
        )
    return path.read_bytes()


def issuer_signing_key() -> ec.EllipticCurvePrivateKey:
    """The key the ORCE issuer signs credentials and status lists with.

    Its public half is what a login issuer's trust entry pins
    (backend/config/oid4vp/trust.dev.json, x5c_leaf_keys), so a credential minted
    with any other key is refused however well its chain verifies.
    """
    return serialization.load_pem_private_key(_read_pem("issuer.key"), password=None)


def _root_ca() -> tuple[ec.EllipticCurvePrivateKey, x509.Certificate]:
    return (
        serialization.load_pem_private_key(_read_pem("root-ca.key"), password=None),
        x509.load_pem_x509_certificate(_read_pem("root-ca.crt")),
    )


def _serial_for(iss: str) -> int:
    """A serial derived from the issuer identifier rather than drawn at random,
    so re-minting a leaf for the same issuer produces the same certificate and a
    reissued fixture credential differs only where its claims do."""
    return int.from_bytes(hashlib.sha256(iss.encode("utf-8")).digest()[:8], "big") or 1


def mint_issuer_leaf(iss: str) -> x509.Certificate:
    """The certificate the ORCE issuer would serve for this base URL.

    The SANs are the three identifiers that issuer answers to, in the order and
    for the reasons ensureIssuerCertFor lists them: the did:web form, the base
    URL itself — which is what a credential's `iss` and a status list's `iss`
    are — and the bare hostname. DCS accepts a leaf that names its issuer by a
    URI SAN, by a DNS SAN matching the identifier's authority, or by an exactly
    matching CN (backend/internal/auth/oid4vp/sdjwt/keys.go,
    leafIdentifiesIssuer); the URI SAN is the one that always holds, because an
    authority carrying a port is no DNS name.
    """
    root_key, root_cert = _root_ca()
    key = issuer_signing_key()
    rest = iss.split("://", 1)[-1]
    hostname = rest.split("/")[0].split(":")[0]

    return (
        x509.CertificateBuilder()
        .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, ISSUER_SUBJECT_CN)]))
        .issuer_name(root_cert.subject)
        .public_key(key.public_key())
        .serial_number(_serial_for(iss))
        .not_valid_before(NOT_BEFORE)
        .not_valid_after(NOT_AFTER)
        .add_extension(
            x509.SubjectAlternativeName(
                [
                    x509.UniformResourceIdentifier(issuer_did_for(iss)),
                    x509.UniformResourceIdentifier(iss),
                    x509.DNSName(hostname),
                ]
            ),
            critical=False,
        )
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
        .sign(root_key, hashes.SHA256())
    )


_ISSUERS: dict[str, DevIssuer] = {}


def dev_issuer(issuer_base: str | None = None) -> DevIssuer:
    """The issuer identity for a stack, named by the URL its status list lives under.

    issuer_base defaults to ISSUER_BASE_URL and then to the dev NodePort, the
    same resolution the status-list URI uses — the two must agree or the
    credential would name a list its own issuer does not publish.
    """
    iss = issuer_base_url(issuer_base)
    issuer = _ISSUERS.get(iss)
    if issuer is not None:
        return issuer

    _root_key, root_cert = _root_ca()
    leaf = mint_issuer_leaf(iss)
    issuer = DevIssuer(
        iss=iss,
        private_jwk=_private_jwk(issuer_signing_key()),
        x5c=[
            base64.b64encode(cert.public_bytes(serialization.Encoding.DER)).decode()
            for cert in (leaf, root_cert)
        ],
        leaf=leaf,
    )
    _ISSUERS[iss] = issuer
    return issuer
