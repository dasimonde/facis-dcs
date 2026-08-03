"""The preflight refuses a status list the backend could not verify.

Two roots are built here with the SAME subject and different keys, because that
is the situation in the field: every ORCE issuer mints its own root at runtime
and they all call themselves "FACIS Demo Root CA" (ADR-34, Consequences). A
check that compares names reports these two as the same CA, which is how a
correctly signed list stayed unverifiable while every log line named the CA it
was supposed to chain to. The test that matters is therefore the one where the
subjects match and the answer is still no.
"""

from __future__ import annotations

import base64
import datetime as dt
import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

from dcs_wallet import status_list

ISSUER = "http://localhost:30181"
LIST_URI = f"{ISSUER}/status-list/1"
# Both roots carry this. It is the subject the ORCE flow hard-codes.
SHARED_ROOT_CN = "FACIS Demo Root CA"

NOT_BEFORE = dt.datetime(2020, 1, 1, tzinfo=dt.timezone.utc)
NOT_AFTER = dt.datetime(2040, 1, 1, tzinfo=dt.timezone.utc)


def _root() -> tuple[ec.EllipticCurvePrivateKey, x509.Certificate]:
    key = ec.generate_private_key(ec.SECP256R1())
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, SHARED_ROOT_CN)])
    cert = (
        x509.CertificateBuilder()
        .subject_name(name)
        .issuer_name(name)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(NOT_BEFORE)
        .not_valid_after(NOT_AFTER)
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .sign(key, hashes.SHA256())
    )
    return key, cert


def _leaf(
    root_key: ec.EllipticCurvePrivateKey,
    root_cert: x509.Certificate,
    *,
    issuer_uri: str | None,
) -> x509.Certificate:
    """A signing leaf, optionally carrying the URI SAN that names its issuer.

    issuer_uri=None reproduces the ORCE boot path, which mints the leaf before a
    request has told it what URL it is reached on.
    """
    key = ec.generate_private_key(ec.SECP256R1())
    builder = (
        x509.CertificateBuilder()
        .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "FACIS Demo PID Issuer")]))
        .issuer_name(root_cert.subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(NOT_BEFORE)
        .not_valid_after(NOT_AFTER)
    )
    if issuer_uri is not None:
        builder = builder.add_extension(
            x509.SubjectAlternativeName([x509.UniformResourceIdentifier(issuer_uri)]),
            critical=False,
        )
    return builder.sign(root_key, hashes.SHA256())


def _token(chain: list[x509.Certificate]) -> str:
    """A status-list JWT carrying `chain` in its x5c header, leaf first.

    Only the header and payload are read by the code under test; the signature
    is the backend's business, so the third segment is a placeholder.
    """
    header = {
        "alg": "ES256",
        "typ": "statuslist+jwt",
        "x5c": [
            base64.b64encode(cert.public_bytes(serialization.Encoding.DER)).decode()
            for cert in chain
        ],
    }
    payload = {"iss": ISSUER, "sub": LIST_URI, "iat": 1577836800}

    def seg(obj: dict) -> str:
        return base64.urlsafe_b64encode(json.dumps(obj).encode()).decode().rstrip("=")

    return f"{seg(header)}.{seg(payload)}.signature"


def _bundle(directory: Path, *certs: x509.Certificate) -> Path:
    path = directory / "x5c-trust-anchors.pem"
    path.write_bytes(b"".join(c.public_bytes(serialization.Encoding.PEM) for c in certs))
    return path


class StatusListAnchorPreflightTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = Path(self.tmp.name)
        self.anchored_key, self.anchored_root = _root()
        self.other_key, self.other_root = _root()

    def _serving(self, chain: list[x509.Certificate]):
        return mock.patch.object(
            status_list, "fetch_status_list_token", return_value=_token(chain)
        )

    def test_accepts_the_root_the_bundle_holds(self) -> None:
        leaf = _leaf(self.anchored_key, self.anchored_root, issuer_uri=ISSUER)
        with self._serving([leaf, self.anchored_root]):
            fingerprint = status_list.assert_served_root_is_a_configured_anchor(
                LIST_URI, anchors_path=_bundle(self.dir, self.anchored_root)
            )
        self.assertEqual(
            fingerprint,
            status_list._certificate_fingerprint(
                self.anchored_root.public_bytes(serialization.Encoding.DER)
            ),
        )

    def test_refuses_a_different_root_with_the_same_subject(self) -> None:
        """The whole point: identical names, different keys, and the answer is no."""
        self.assertEqual(self.anchored_root.subject, self.other_root.subject)
        leaf = _leaf(self.other_key, self.other_root, issuer_uri=ISSUER)
        with self._serving([leaf, self.other_root]):
            with self.assertRaises(RuntimeError) as caught:
                status_list.assert_served_root_is_a_configured_anchor(
                    LIST_URI, anchors_path=_bundle(self.dir, self.anchored_root)
                )
        self.assertIn("does not anchor", str(caught.exception))

    def test_finds_its_anchor_among_several(self) -> None:
        """A bundle holds one root per issuer, so the right one need not be first."""
        leaf = _leaf(self.anchored_key, self.anchored_root, issuer_uri=ISSUER)
        with self._serving([leaf, self.anchored_root]):
            status_list.assert_served_root_is_a_configured_anchor(
                LIST_URI, anchors_path=_bundle(self.dir, self.other_root, self.anchored_root)
            )

    def test_refuses_a_list_signed_without_a_chain(self) -> None:
        with mock.patch.object(
            status_list,
            "fetch_status_list_token",
            return_value=".".join(
                [
                    base64.urlsafe_b64encode(json.dumps({"alg": "ES256"}).encode()).decode().rstrip("="),
                    base64.urlsafe_b64encode(json.dumps({"iss": ISSUER}).encode()).decode().rstrip("="),
                    "signature",
                ]
            ),
        ):
            with self.assertRaises(RuntimeError) as caught:
                status_list.assert_served_root_is_a_configured_anchor(
                    LIST_URI, anchors_path=_bundle(self.dir, self.anchored_root)
                )
        self.assertIn("no x5c chain", str(caught.exception))

    def test_hard_fails_when_the_backend_has_no_anchors(self) -> None:
        leaf = _leaf(self.anchored_key, self.anchored_root, issuer_uri=ISSUER)
        with self._serving([leaf, self.anchored_root]):
            with self.assertRaises(RuntimeError) as caught:
                status_list.assert_served_root_is_a_configured_anchor(
                    LIST_URI, anchors_path=self.dir / "absent.pem"
                )
        self.assertIn("no x5c trust anchors", str(caught.exception))


class StatusListLeafIdentityPreflightTest(unittest.TestCase):
    """A chain to a trusted root is not permission to speak for this issuer.

    Reaching an anchor says an anchor vouched for the leaf. Naming the issuer is
    a separate requirement, and without it any certificate under any configured
    anchor could publish any issuer's revocation status (ADR-34 §3).
    """

    def setUp(self) -> None:
        self.root_key, self.root = _root()

    def _serving(self, chain: list[x509.Certificate]):
        return mock.patch.object(
            status_list, "fetch_status_list_token", return_value=_token(chain)
        )

    def test_accepts_a_leaf_whose_uri_san_is_the_issuer(self) -> None:
        with self._serving([_leaf(self.root_key, self.root, issuer_uri=ISSUER), self.root]):
            self.assertEqual(status_list.assert_served_leaf_names_the_issuer(LIST_URI), ISSUER)

    def test_refuses_the_boot_minted_leaf_that_names_nobody(self) -> None:
        with self._serving([_leaf(self.root_key, self.root, issuer_uri=None), self.root]):
            with self.assertRaises(RuntimeError) as caught:
                status_list.assert_served_leaf_names_the_issuer(LIST_URI)
        self.assertIn("does not identify", str(caught.exception))

    def test_refuses_a_leaf_that_names_a_different_issuer(self) -> None:
        other = "https://dcs-ionos.facis.cloud/issuer"
        with self._serving([_leaf(self.root_key, self.root, issuer_uri=other), self.root]):
            with self.assertRaises(RuntimeError):
                status_list.assert_served_leaf_names_the_issuer(LIST_URI)


class CommittedDevAnchorsTest(unittest.TestCase):
    def test_the_committed_bundle_parses_and_holds_anchors(self) -> None:
        """The dev/CI backend mounts this file; an unparseable one fails closed
        at startup, and the preflight would then blame the issuer."""
        self.assertTrue(
            status_list.DEV_ANCHORS_PATH.is_file(),
            f"{status_list.DEV_ANCHORS_PATH} is what dev and BDD mount as their anchors",
        )
        ders = status_list._pem_certificates(
            status_list.DEV_ANCHORS_PATH.read_text(encoding="utf-8")
        )
        self.assertGreaterEqual(len(ders), 1)
        for der in ders:
            x509.load_der_x509_certificate(der)


if __name__ == "__main__":
    unittest.main()
