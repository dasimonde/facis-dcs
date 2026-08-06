from __future__ import annotations

import json
import unittest
from pathlib import Path

import jwt
from jwt.algorithms import ECAlgorithm

from dcs_wallet.credential import decode_jwt_payload, load_credential_sd_jwt
from dcs_wallet.issuer import issue_access_credential
<<<<<<< HEAD
from dcs_wallet.keys import did_jwk_from_public_jwk
=======
from dcs_wallet.issuer_pki import dev_issuer, leaf_public_jwk
>>>>>>> feat/adr-35-anchored-peer-trust
from dcs_wallet.presentation import build_vp_token, load_jwk
from dcs_wallet.status_list import FIXTURE_INDEX, RESERVED_INDEX, role_credential_index
from dcs_wallet.sdjwt import KB_JWT_TYP, decode_disclosure, sd_hash, split_sd_jwt


class PresentationTest(unittest.TestCase):
    def _status_claim(self, presentation: str) -> dict:
        issuer_jwt, _, _ = split_sd_jwt(presentation)
        return decode_jwt_payload(issuer_jwt)["status"]["status_list"]

    def _access_credential(self, *, organization: str, roles: list[str], nonce: str) -> str:
        return issue_access_credential(
            organization=organization,
            roles=roles,
            wallet_private=load_jwk("wallet.jwk"),
            status_index=role_credential_index(organization=organization, roles=roles),
            nonce=nonce,
        )

    def test_access_credential_status_index_identifies_the_identity_not_the_ceremony(self) -> None:
        first = self._access_credential(
            organization="did:web:example:organization", roles=["Contract Signer"], nonce="ceremony-one"
        )
        second = self._access_credential(
            organization="did:web:example:organization", roles=["Contract Signer"], nonce="ceremony-two"
        )
        self.assertEqual(self._status_claim(first)["idx"], self._status_claim(second)["idx"])

    def test_a_different_identity_gets_a_different_bit(self) -> None:
        signer = self._access_credential(
            organization="did:web:example:organization", roles=["Contract Signer"], nonce="n"
        )
        approver = self._access_credential(
            organization="did:web:example:organization", roles=["Contract Approver"], nonce="n"
        )
        other_org = self._access_credential(
            organization="did:web:other:organization", roles=["Contract Signer"], nonce="n"
        )
        indices = {self._status_claim(c)["idx"] for c in (signer, approver, other_org)}
        self.assertEqual(len(indices), 3)

    def test_a_revoked_probe_organization_holds_a_reserved_bit(self) -> None:
        # The scenario that revokes a login credential presents this
        # organization; whatever roles it asks for, it must land on the bit
        # reserved for it and on no other identity's.
        organization = "BDD Revocation Probe Org"
        for roles in (["Contract Signer"], ["Contract Manager", "Auditor"]):
            self.assertEqual(
                role_credential_index(organization=organization, roles=roles),
                RESERVED_INDEX[organization],
            )

    def test_every_committed_credential_owns_its_own_bit(self) -> None:
        self.assertEqual(len(set(FIXTURE_INDEX.values())), len(FIXTURE_INDEX))
        credentials_dir = Path(__file__).resolve().parent.parent / "credentials"
        for path in sorted(credentials_dir.glob("*.jwt")):
            key = path.name.removesuffix(".jwt")
            self.assertIn(key, FIXTURE_INDEX, f"{path.name} has no allocated status-list index")
            issuer_jwt, _, _ = split_sd_jwt(path.read_text(encoding="utf-8").strip())
            claim = decode_jwt_payload(issuer_jwt)["status"]["status_list"]
            self.assertEqual(claim["idx"], FIXTURE_INDEX[key])
            self.assertTrue(claim["uri"].endswith("/status-list/1"), claim["uri"])

    def test_generated_credential_carries_its_issuers_chain_and_holder_cnf(self) -> None:
        issuer_jwt, _, _ = split_sd_jwt(load_credential_sd_jwt("johndoe"))
        header = jwt.get_unverified_header(issuer_jwt)
        self.assertEqual(header["typ"], "dc+sd-jwt")
<<<<<<< HEAD
        self.assertIn("jwk", header)
        self.assertEqual(set(header["jwk"].keys()), {"kty", "crv", "x", "y"})
        # Der Header trägt beides: den Schlüssel selbst, gegen den ein Verifier
        # die Signatur direkt prüfen kann, und den kid als did:jwk desselben
        # Schlüssels. Beide müssen auf dieselbe Identität zeigen, sonst kann ein
        # Verifier je nach gewähltem Pfad zu unterschiedlichen Ergebnissen kommen.
        self.assertEqual(header["kid"], did_jwk_from_public_jwk(header["jwk"]))

        issuer_private = load_jwk("issuer-dev.jwk")
        expected_issuer_public = {k: issuer_private[k] for k in ("kty", "crv", "x", "y")}
        self.assertEqual(header["jwk"], expected_issuer_public)
=======
        # The issuer publishes its key through a certificate chain, so a bare
        # header jwk would be a key from somewhere its trust entry never named
        # (backend/internal/auth/oid4vp/sdjwt/keys.go).
        self.assertIn("x5c", header)
        self.assertNotIn("jwk", header)
        self.assertNotIn("kid", header)
>>>>>>> feat/adr-35-anchored-peer-trust

        payload = decode_jwt_payload(issuer_jwt)
        # The leaf carries the key of the issuer this credential names —
        # resolved from the credential rather than from the environment, so the
        # assertion reads the same whether or not ISSUER_BASE_URL is exported.
        # The certificate's own bytes are not compared: a leaf is re-minted
        # whenever one is needed and an ECDSA signature is randomized, so two
        # certificates for the same issuer differ while stating the same thing.
        issuer = dev_issuer(payload["iss"])
        self.assertEqual(len(header["x5c"]), 2, "leaf and the root it chains to")
        self.assertEqual(
            leaf_public_jwk(header["x5c"][0]),
            {k: issuer.private_jwk[k] for k in ("kty", "crv", "x", "y")},
        )

        self.assertIn("cnf", payload)
        self.assertIn("jwk", payload["cnf"])
        cnf_jwk = payload["cnf"]["jwk"]
        self.assertEqual(set(cnf_jwk.keys()), {"kty", "crv", "x", "y"})

    def test_stored_credential_is_issuer_sd_jwt_without_kb(self) -> None:
        issuer_jwt, disclosures, kb_jwt = split_sd_jwt(load_credential_sd_jwt("johndoe"))
        self.assertTrue(issuer_jwt.startswith("eyJ"))
        self.assertGreater(len(disclosures), 0)
        self.assertIsNone(kb_jwt, "stored credentials must not include presentation-time KB-JWT")

        issuer_payload = decode_jwt_payload(issuer_jwt)
        self.assertIn("sub", issuer_payload)
        self.assertIn("cnf", issuer_payload)

    def test_vp_token_contains_valid_kb_jwt(self) -> None:
        vp = build_vp_token(credential_name="johndoe", nonce="unit-test-nonce", client_id="unit-test-aud")
        issuer_jwt, disclosures, kb_jwt = split_sd_jwt(vp)
        self.assertIsNotNone(kb_jwt)

        issuer_header = jwt.get_unverified_header(issuer_jwt)
        issuer_payload = jwt.decode(
            issuer_jwt,
            ECAlgorithm.from_jwk(json.dumps(leaf_public_jwk(issuer_header["x5c"][0]))),
            algorithms=["ES256"],
            options={"verify_exp": False, "verify_iat": False},
        )
        cnf_jwk = issuer_payload["cnf"]["jwk"]

        kb_header = jwt.get_unverified_header(kb_jwt)
        self.assertEqual(kb_header["typ"], KB_JWT_TYP)
        kb_payload = jwt.decode(
            kb_jwt,
            ECAlgorithm.from_jwk(json.dumps(cnf_jwk)),
            algorithms=["ES256"],
            audience="unit-test-aud",
            options={"verify_iat": False},
        )
        self.assertEqual(kb_payload["nonce"], "unit-test-nonce")
        self.assertEqual(kb_payload["sd_hash"], sd_hash(issuer_jwt, disclosures))
        self.assertNotIn("sub", kb_payload)

    def test_vp_token_selective_disclosure_filters_claims(self) -> None:
        vp = build_vp_token(
            credential_name="johndoe",
            nonce="unit-test-nonce",
            client_id="unit-test-aud",
            requested_claim_paths=[["organization"]],
        )
        _issuer_jwt, disclosures, _kb_jwt = split_sd_jwt(vp)
        disclosed_claim_names = []
        for disclosure in disclosures:
            value = decode_disclosure(disclosure)
            self.assertEqual(len(value), 3)
            disclosed_claim_names.append(value[1])

        self.assertEqual(disclosed_claim_names, ["organization"])

    def test_playground_pid_discloses_only_default_identity_claims(self) -> None:
        from pathlib import Path

        cred_path = Path(__file__).resolve().parent.parent / "credentials" / "alicewilliams.pid.jwt"
        if not cred_path.is_file():
            self.skipTest("alicewilliams.pid.jwt not present")

        vp = build_vp_token(
            credential_name="alicewilliams.pid",
            nonce="unit-test-nonce",
            client_id="unit-test-aud",
        )
        _issuer_jwt, disclosures, _kb_jwt = split_sd_jwt(vp)
        disclosed_claim_names = []
        for disclosure in disclosures:
            value = decode_disclosure(disclosure)
            self.assertEqual(len(value), 3)
            disclosed_claim_names.append(value[1])

        self.assertEqual(disclosed_claim_names, ["given_name", "family_name", "birthdate"])


if __name__ == "__main__":
    unittest.main()
