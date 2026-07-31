from __future__ import annotations

import base64
import hashlib
import json
import unittest

from dcs_wallet.sdjwt import (
    create_property_disclosure,
    decode_disclosure,
    disclosure_digest,
    encode_disclosure,
    merge_disclosed_claims,
    join_sd_jwt,
    presentation_body_for_sd_hash,
    sd_hash,
    split_sd_jwt,
)

DEMO_SD_JWT = (
    "eyJ0eXAiOiJzZCtqd3QiLCJhbGciOiJFUzI1NiJ9.eyJpZCI6IjEyMzQiLCJfc2QiOlsiYkRUUnZtNS1Zbi1IRzdjcXBWUjVPVlJJWHNTYUJrNTdKZ2lPcV9qMVZJNCIs"
    "ImV0M1VmUnlsd1ZyZlhkUEt6Zzc5aGNqRDFJdHpvUTlvQm9YUkd0TW9zRmsiLCJ6V2ZaTlMxOUF0YlJTVGJvN3NKUm4wQlpRdldSZGNob0M3VVphYkZyalk4Il0sIl9zZF9hbGciOiJzaGEtMjU2In0."
    "n27NCtnuwytlBYtUNjgkesDP_7gN7bhaLhWNL4SWT6MaHsOjZ2ZMp987GgQRL6ZkLbJ7Cd3hlePHS84GBXPuvg~"
    "WyI1ZWI4Yzg2MjM0MDJjZjJlIiwiZmlyc3RuYW1lIiwiSm9obiJd~"
    "WyJjNWMzMWY2ZWYzNTg4MWJjIiwibGFzdG5hbWUiLCJEb2UiXQ~"
    "WyJmYTlkYTUzZWJjOTk3OThlIiwic3NuIiwiMTIzLTQ1LTY3ODkiXQ~"
    "eyJ0eXAiOiJrYitqd3QiLCJhbGciOiJFUzI1NiJ9.eyJpYXQiOjE3MTAwNjk3MjIsImF1ZCI6ImRpZDpleGFtcGxlOjEyMyIsIm5vbmNlIjoiazh2ZGYwbmQ2Iiwic2RfaGFzaCI6Il8tTmJWSzNmczl3VzNHaDNOUktSNEt1NmZDMUwzN0R2MFFfalBXd0ppRkUifQ."
    "pqw2OB5IA5ya9Mxf60hE3nr2gsJEIoIlnuCa4qIisijHbwg3WzTDFmW2SuNvK_ORN0WU6RoGbJx5uYZh8k4EbA"
)


class SDJWTTest(unittest.TestCase):
    def test_disclosure_round_trip(self) -> None:
        disclosure = '["5eb8c8623402cf2e","organization","Acme Corp"]'
        encoded = encode_disclosure(disclosure)
        decoded = decode_disclosure(encoded)
        self.assertEqual(decoded, ["5eb8c8623402cf2e", "organization", "Acme Corp"])
        self.assertEqual(disclosure_digest(encoded), disclosure_digest(encoded))

    def test_digest_hashes_encoded_disclosure(self) -> None:
        disclosure = '["salt","organization","Acme"]'
        encoded = encode_disclosure(disclosure)
        wrong = hashlib.sha256(disclosure.encode("utf-8")).digest()
        right = hashlib.sha256(encoded.encode("ascii")).digest()
        self.assertNotEqual(wrong, right)
        self.assertEqual(
            disclosure_digest(encoded),
            base64.urlsafe_b64encode(right).rstrip(b"=").decode(),
        )

    def test_create_property_disclosure(self) -> None:
        encoded, digest = create_property_disclosure("roles", ["Admin"])
        arr = decode_disclosure(encoded)
        self.assertEqual(arr[1], "roles")
        self.assertEqual(arr[2], ["Admin"])
        self.assertTrue(digest)

    def test_split_sd_jwt_with_kb(self) -> None:
        token = "eyJ.issuer.sig~disc1~disc2~eyJ.kb.sig"
        issuer, disclosures, kb = split_sd_jwt(token)
        self.assertTrue(issuer.startswith("eyJ"))
        self.assertEqual(disclosures, ["disc1", "disc2"])
        self.assertEqual(kb, "eyJ.kb.sig")

    def test_merge_disclosed_claims(self) -> None:
        enc_org, _ = create_property_disclosure("organization", "Acme")
        enc_roles, _ = create_property_disclosure("roles", ["Signer"])
        payload = {"iss": "did:example", "sub": "did:holder", "_sd": ["x", "y"]}
        merged = merge_disclosed_claims(payload, [enc_org, enc_roles])
        self.assertEqual(merged["organization"], "Acme")
        self.assertEqual(merged["roles"], ["Signer"])
        self.assertNotIn("_sd", merged)


    def test_join_sd_jwt_without_kb_has_trailing_tilde(self) -> None:
        token = join_sd_jwt("eyJ.issuer.sig", ["disc1", "disc2"])
        self.assertEqual(token, "eyJ.issuer.sig~disc1~disc2~")

    def test_join_sd_jwt_with_kb_has_no_extra_trailing_tilde(self) -> None:
        token = join_sd_jwt("eyJ.issuer.sig", ["disc1", "disc2"], "eyJ.kb.sig")
        self.assertEqual(token, "eyJ.issuer.sig~disc1~disc2~eyJ.kb.sig")

    def test_demo_sd_hash_matches_reference(self) -> None:
        issuer, disclosures, kb = split_sd_jwt(DEMO_SD_JWT)
        expected = json.loads(base64.urlsafe_b64decode(kb.split(".")[1] + "=="))["sd_hash"]
        self.assertEqual(sd_hash(issuer, disclosures), expected)
        body = presentation_body_for_sd_hash(issuer, disclosures)
        self.assertTrue(body.endswith("~"))
        self.assertIn("~WyI1ZWI4", body)


if __name__ == "__main__":
    unittest.main()
