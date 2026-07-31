from __future__ import annotations

import json
import unittest

import jwt
from jwt.algorithms import ECAlgorithm

from dcs_wallet.credential import decode_jwt_payload, load_credential_sd_jwt
from dcs_wallet.issuer import issue_access_credential
from dcs_wallet.presentation import build_vp_token, load_jwk
from dcs_wallet.sdjwt import KB_JWT_TYP, decode_disclosure, sd_hash, split_sd_jwt


class PresentationTest(unittest.TestCase):
    def test_fresh_access_credential_uses_ceremony_specific_status_index(self) -> None:
        issuer_private = load_jwk("issuer-dev.jwk")
        wallet_private = load_jwk("wallet.jwk")
        first = issue_access_credential(
            organization="did:web:example:organization",
            roles=["Contract Signer"],
            issuer_private=issuer_private,
            wallet_private=wallet_private,
            nonce="ceremony-one",
        )
        second = issue_access_credential(
            organization="did:web:example:organization",
            roles=["Contract Signer"],
            issuer_private=issuer_private,
            wallet_private=wallet_private,
            nonce="ceremony-two",
        )
        first_issuer, _, _ = split_sd_jwt(first)
        second_issuer, _, _ = split_sd_jwt(second)
        first_idx = decode_jwt_payload(first_issuer)["status"]["status_list"]["idx"]
        second_idx = decode_jwt_payload(second_issuer)["status"]["status_list"]["idx"]
        self.assertNotEqual(first_idx, second_idx)

    def test_generated_credential_contains_issuer_header_jwk_and_holder_cnf(self) -> None:
        issuer_jwt, _, _ = split_sd_jwt(load_credential_sd_jwt("johndoe"))
        header = jwt.get_unverified_header(issuer_jwt)
        self.assertEqual(header["typ"], "dc+sd-jwt")
        self.assertIn("jwk", header)
        self.assertNotIn("kid", header)
        self.assertEqual(set(header["jwk"].keys()), {"kty", "crv", "x", "y"})

        issuer_private = load_jwk("issuer-dev.jwk")
        expected_issuer_public = {k: issuer_private[k] for k in ("kty", "crv", "x", "y")}
        self.assertEqual(header["jwk"], expected_issuer_public)

        payload = decode_jwt_payload(issuer_jwt)
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
            ECAlgorithm.from_jwk(json.dumps(issuer_header["jwk"])),
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
