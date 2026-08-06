"""A credential must be issued by the issuer that publishes the list it names.

A status list says only that SOME issuer published it until it is bound to the
credential whose status it carries, so the verifier requires the list's `iss` to
equal the credential's (backend/internal/auth/oid4vp/status/credentialbinding.go,
RequireCredentialIssuer). This wallet minted credentials under a dev issuer DID
while pointing them at the ORCE issuer's list, and every login was refused with
the list unread — which reads exactly like a revoked credential.

What is checked here is the pairing itself, plus the two things that make the
resulting `iss` believable to a login verifier: the leaf carries the key the
trust document pins, and it names that issuer.
"""

from __future__ import annotations

import base64
import json
import unittest
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtensionOID, NameOID

from dcs_wallet.credential import decode_jwt_payload
from dcs_wallet.issuer import issue_stored_credential
from dcs_wallet.issuer_pki import PKI_DEV_DIR, dev_issuer, issuer_did_for
from dcs_wallet.presentation import load_jwk
from dcs_wallet.sdjwt import split_sd_jwt
from dcs_wallet.status_list import status_list_uri

REPO_ROOT = Path(__file__).resolve().parents[2]
TRUST_PATH = REPO_ROOT / "backend" / "config" / "oid4vp" / "trust.dev.json"
CREDENTIALS_DIR = Path(__file__).resolve().parent.parent / "credentials"

# The two stacks whose issuer this wallet mints as: the kind/BDD release behind
# its stripped /issuer prefix, and the dev NodePort.
ISSUER_BASES = ["http://localhost:18080/issuer", "http://localhost:30181"]


def _leaf(x5c: list[str]) -> x509.Certificate:
    return x509.load_der_x509_certificate(base64.b64decode(x5c[0]))


def _uri_sans(certificate: x509.Certificate) -> list[str]:
    try:
        san = certificate.extensions.get_extension_for_oid(ExtensionOID.SUBJECT_ALTERNATIVE_NAME).value
    except x509.ExtensionNotFound:
        return []
    return [str(value) for value in san.get_values_for_type(x509.UniformResourceIdentifier)]


def _status_reference(issuer_jwt: str) -> dict:
    return decode_jwt_payload(issuer_jwt)["status"]["status_list"]


class IssuerPublishesTheListItNamesTest(unittest.TestCase):
    def test_minted_credential_is_issued_by_the_list_it_points_at(self) -> None:
        for base in ISSUER_BASES:
            with self.subTest(base=base):
                issuer_jwt, _, _ = split_sd_jwt(
                    issue_stored_credential(
                        organization="Acme Corp",
                        roles=["Contract Signer"],
                        wallet_private=load_jwk("wallet.jwk"),
                        status_index=2100,
                        issuer_base=base,
                    )
                )
                claims = decode_jwt_payload(issuer_jwt)
                self.assertEqual(claims["iss"], base)
                # The issuer serves its list at <iss>/status-list/1 and signs
                # the token with iss == that same base
                # (flows-issuer/flow-statuslist.json).
                self.assertEqual(claims["status"]["status_list"]["uri"], status_list_uri(base))

    def test_every_committed_credential_names_an_issuer_that_signs_its_list(self) -> None:
        for path in sorted(CREDENTIALS_DIR.glob("*.jwt")):
            with self.subTest(credential=path.name):
                issuer_jwt, _, _ = split_sd_jwt(path.read_text(encoding="utf-8").strip())
                claims = decode_jwt_payload(issuer_jwt)
                uri = claims["status"]["status_list"]["uri"]
                self.assertEqual(uri, status_list_uri(claims["iss"]))


class IssuerLeafTest(unittest.TestCase):
    def setUp(self) -> None:
        self.trust = json.loads(TRUST_PATH.read_text(encoding="utf-8"))

    def test_the_leaf_carries_the_key_the_trust_document_pins(self) -> None:
        """Login terminates at the leaf (ADR-35): a chain to the dev CA is not
        enough, the leaf must carry the key the operator wrote down."""
        for base in ISSUER_BASES:
            with self.subTest(base=base):
                entry = self.trust["issuers"][base]
                self.assertEqual(entry["mechanism"], "x5c")
                spki = _leaf(dev_issuer(base).x5c).public_key().public_bytes(
                    serialization.Encoding.DER,
                    serialization.PublicFormat.SubjectPublicKeyInfo,
                )
                self.assertIn(base64.b64encode(spki).decode(), entry["x5c_leaf_keys"])

    def test_the_leaf_names_the_issuer_it_speaks_for(self) -> None:
        """A chain proves an anchor vouched for the certificate; naming the
        issuer is the separate requirement, without which any certificate under
        any anchor speaks for any issuer (keys.go, leafIdentifiesIssuer)."""
        for base in ISSUER_BASES:
            with self.subTest(base=base):
                leaf = _leaf(dev_issuer(base).x5c)
                self.assertIn(base, _uri_sans(leaf))
                self.assertIn(issuer_did_for(base), _uri_sans(leaf))

    def test_the_chain_verifies_to_the_committed_root(self) -> None:
        root = x509.load_pem_x509_certificate((PKI_DEV_DIR / "root-ca.crt").read_bytes())
        for base in ISSUER_BASES:
            with self.subTest(base=base):
                chain = dev_issuer(base).x5c
                leaf = _leaf(chain)
                self.assertEqual(leaf.issuer, root.subject)
                # Signed by the root the dev and BDD backends anchor, which is
                # what an x5c PID resolves against — login pins the leaf key
                # instead and never walks the chain.
                root.public_key().verify(
                    leaf.signature,
                    leaf.tbs_certificate_bytes,
                    ec.ECDSA(leaf.signature_hash_algorithm),
                )
                self.assertEqual(
                    base64.b64decode(chain[-1]),
                    root.public_bytes(serialization.Encoding.DER),
                )

    def test_the_leaf_is_the_certificate_the_issuer_itself_would_serve(self) -> None:
        """Same subject the ORCE flow mints under, so a credential signed here
        and one signed by a running issuer are the same statement."""
        leaf = _leaf(dev_issuer(ISSUER_BASES[0]).x5c)
        self.assertEqual(
            [attribute.value for attribute in leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)],
            ["FACIS Demo Issuer"],
        )


if __name__ == "__main__":
    unittest.main()
