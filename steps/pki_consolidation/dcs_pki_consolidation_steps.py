"""BDD step definitions for PKI consolidation
(features/21_pki_consolidation_pkcs11; SRS DCS-IR-HI-01, DCS-NFR-SEC-02,
DCS-OR-C2PA-007).

The swappable trust-anchor scenario (DCS_TRUST_ANCHORS) is @skip in the
feature file - see its inline comment. The CRL-revocation scenario seeds its
precondition directly via the test DB connection (context.db); the seam is
documented at its point of use.
"""

import base64
import json
import os

import jwt
from cryptography import x509
from cryptography.hazmat.primitives.asymmetric import ec
import requests as _requests
from behave import given, then, when

from steps.support.api_client import (
    did_document_url,
    post_json,
    signature_validate_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


# ---------------------------------------------------------------------------
# This instance's own DID document
# ---------------------------------------------------------------------------


@when("I request this instance's own DID document")
def step_when_request_own_did_document(context):
    context.requests_response = _requests.get(
        did_document_url(context.base_url),
        timeout=context.http_timeout_seconds,
    )


@then("the DID document's verificationMethod key is an ECDSA P-256 JWK, not RSA")
def step_then_did_jwk_is_ec_p256(context):
    body = context.requests_response.json()
    verification_methods = body.get("verificationMethod") or []
    assert verification_methods, (
        f"DID document has no verificationMethod entries: {body}"
    )
    # Every published key, not just the first: no consumer resolves by position,
    # so an RSA key anywhere in the document is an RSA key in use somewhere.
    for method in verification_methods:
        jwk = method.get("publicKeyJwk") or {}
        assert jwk.get("kty") == "EC", (
            f"Expected {method.get('id')!r}'s publicKeyJwk.kty to be 'EC' (hsm.PublicJWK "
            f"for the HSM-backed keys, ECDSA P-256), got: {jwk}"
        )
    assert jwk.get("crv") == "P-256", (
        f"Expected the DID key's publicKeyJwk.crv to be 'P-256', got: {jwk}"
    )
    assert "n" not in jwk and "e" not in jwk, (
        "DID key's publicKeyJwk carries RSA fields ('n'/'e') alongside/instead of "
        f"the EC (x/y) fields the P-256 dcs-did key publishes: {jwk}"
    )
    assert jwk.get("x") and jwk.get("y"), (
        f"Expected EC JWK 'x' and 'y' coordinates to be present, got: {jwk}"
    )


# ---------------------------------------------------------------------------
# OpenID4VP JAR, ES256-signed by the dcs-oid4vp-jar HSM key
# ---------------------------------------------------------------------------


@when("I start an OpenID4VP login and fetch the signed authorization request object")
def step_when_fetch_jar(context):
    login_resp = _requests.post(
        f"{context.base_url}/auth/login", timeout=context.http_timeout_seconds
    )
    assert login_resp.status_code == 200, (
        f"POST /auth/login failed: {login_resp.status_code} {login_resp.text}"
    )
    state = login_resp.json().get("state")
    assert state, f"/auth/login response has no 'state': {login_resp.text}"
    context.oid4vp_login_state = state

    context.requests_response = _requests.get(
        f"{context.base_url}/auth/presentation/request/{state}",
        headers={"Accept": "application/oauth-authz-req+jwt, application/jwt"},
        timeout=context.http_timeout_seconds,
    )


@then(
    "the authorization request JWT is ES256-signed with an EC P-256 key "
    "verifiable against its own certificate chain"
)
def step_then_jar_is_es256_chain_verifiable(context):
    token = context.requests_response.text.strip()
    assert token, "authorization request response body is empty"

    header = jwt.get_unverified_header(token)
    assert header.get("alg") == "ES256", (
        f"Expected the JAR JWT header 'alg' to be 'ES256', got: {header}"
    )
    x5c = header.get("x5c")
    assert isinstance(x5c, list) and x5c, (
        f"Expected the JAR JWT header to carry an x5c certificate chain, got header: {header}"
    )

    # The wallet resolves trust from the certificate the x509_san_dns client
    # identifier names, so the check is that the signature verifies against
    # THAT chain — a stricter statement than a self-describing jwk, which only
    # proves possession of some key. Combined with alg=ES256 it still proves a
    # real ECDSA P-256 private-key operation produced these bytes, which is
    # what the HSM custody requirement is about.
    leaf = x509.load_der_x509_certificate(base64.b64decode(str(x5c[0])))
    public_key = leaf.public_key()
    assert isinstance(public_key, ec.EllipticCurvePublicKey), (
        f"Expected the signing certificate to carry an EC key, got: {type(public_key).__name__}"
    )
    assert public_key.curve.name == "secp256r1", (
        f"Expected the signing certificate's key to be P-256, got: {public_key.curve.name}"
    )

    try:
        jwt.decode(
            token,
            public_key,
            algorithms=["ES256"],
            options={"verify_aud": False, "verify_exp": False},
        )
    except Exception as exc:  # noqa: BLE001 - re-raised as an assertion for behave
        raise AssertionError(
            f"authorization request JWT signature does not verify against the "
            f"certificate in its own x5c chain: {exc}"
        ) from exc


@then("the authorization request JWT's certificate names the hostname its client_id claims")
def step_then_jar_certificate_matches_client_id(context):
    """The x509_san_dns prefix is a claim about a hostname; the certificate has
    to back it. A wallet that cannot match the two refuses the request."""
    token = context.requests_response.text.strip()
    header = jwt.get_unverified_header(token)
    payload = jwt.decode(token, options={"verify_signature": False})

    client_id = str(payload.get("client_id") or "")
    prefix = "x509_san_dns:"
    assert client_id.startswith(prefix), (
        f"Expected a prefixed client_id a wallet can resolve, got: {client_id!r}. "
        f"An unprefixed value means the 'pre-registered' prefix, which a wallet "
        f"outside a pre-agreed federation refuses."
    )
    hostname = client_id[len(prefix):]

    leaf = x509.load_der_x509_certificate(base64.b64decode(str(header["x5c"][0])))
    sans = leaf.extensions.get_extension_for_class(
        x509.SubjectAlternativeName
    ).value.get_values_for_type(x509.DNSName)
    assert hostname in sans, (
        f"client_id claims hostname {hostname!r} but the signing certificate's "
        f"SAN carries {sans}"
    )


# ---------------------------------------------------------------------------
# Contract-Lifecycle-VC proof is ECDSA/ES256, not Ed25519Signature2020
# ---------------------------------------------------------------------------


@then(
    'the embedded contract-lifecycle VC proof for contract "{name}" is ECDSA/ES256, '
    "not Ed25519Signature2020"
)
def step_then_vc_proof_is_ecdsa(context, name):
    pdf_bytes = context.requests_response.content
    assert pdf_bytes, f"PDF export response for contract '{name}' has an empty body"

    # The VC's EmbeddedFile stream is written uncompressed (no /Filter
    # FlateDecode - see pdf-core/compiler/update.go:390-392), so its JSON
    # content is a plain, searchable substring of the PDF bytes, the same
    # way the existing "contract.jsonld" attachment-name check works
    # (steps/pdf_generation/pdf_steps.py:_utf16be usage) - no PDF parsing
    # library is required.
    assert b"Ed25519Signature2020" not in pdf_bytes, (
        f"Exported PDF for contract '{name}' still embeds a VC proof of type "
        "'Ed25519Signature2020' - the HSM-backed VC signer (DCS-IR-HI-01) "
        "requires an ECDSA-based proof suite instead"
    )
    lowered = pdf_bytes.lower()
    assert b"es256" in lowered or b"ecdsa" in lowered, (
        f"Expected the embedded contract-lifecycle VC proof for contract '{name}' to "
        "declare an ECDSA/ES256 proof suite (e.g. a JsonWebSignature2020 proof with "
        "alg ES256, or a DataIntegrityProof with cryptosuite 'ecdsa-rdfc-2019') - found "
        "neither 'ES256' nor 'ecdsa' anywhere in the exported PDF bytes"
    )


# ---------------------------------------------------------------------------
# Two-instance: both instances publish an EC P-256 DID key
#
# The Given/When/Then steps for "instance A and instance B are both running
# and trust each other", "the initiator on instance A creates and offers a
# contract with instance B as negotiator and approver", and "the contract
# appears on instance B in state OFFERED within a few seconds" are already
# registered by steps/peer_trust/dcs_peer_trust_steps.py (behave's step
# registry is global across step modules) and are reused as-is here rather
# than duplicated - only the new EC-P-256-specific assertion below is added.
# ---------------------------------------------------------------------------


@then("instance A and instance B each publish an ECDSA P-256 DID key, not RSA")
def step_then_both_instances_publish_ec_p256(context):
    for label, base_url in (("A", context.base_url_a), ("B", context.base_url_b)):
        resp = _requests.get(
            did_document_url(base_url), timeout=context.http_timeout_seconds
        )
        assert resp.status_code == 200, (
            f"instance {label} did.json unreachable: {resp.status_code} {resp.text}"
        )
        verification_methods = resp.json().get("verificationMethod") or []
        assert verification_methods, (
            f"instance {label}'s DID document has no verificationMethod entries"
        )
        for method in verification_methods:
            jwk = method.get("publicKeyJwk") or {}
            assert jwk.get("kty") == "EC" and jwk.get("crv") == "P-256", (
                f"Expected instance {label}'s key {method.get('id')!r} to be ECDSA P-256 (this "
                "is a breaking change: both instances must switch to the HSM-backed ECDSA DID "
                f"signer simultaneously), got kty={jwk.get('kty')!r} crv={jwk.get('crv')!r}"
            )


# ---------------------------------------------------------------------------
# Full export's COSE alg (pdf-core embeds ES256 COSE_Sign1; the DCS signs the
# Sig_structure with the dcs-c2pa key and pdf-core embeds it — pdf-core is keyless)
# ---------------------------------------------------------------------------


@then("the exported PDF's C2PA COSE_Sign1 protected header declares alg ES256(-7), not EdDSA(-8)")
def step_then_cose_alg_is_es256(context):
    pdf_bytes = context.requests_response.content
    assert pdf_bytes, "PDF export response has an empty body"

    # The COSE protected header is built as a 2-pair CBOR map {1: alg, 33:
    # x5chain} (pdf-core/compiler/compiler_c2pa.go:616-628,
    # buildCoseProtectedHeadersWithX5Chain): cborMap's header byte for a
    # 2-pair map is 0xA2 (major type 5, n=2), followed by cborUint(1) = 0x01
    # (key 1), followed by cborNegInt(alg) - CBOR negative-int encoding
    # stores -(n+1), so alg=-7 (ES256) encodes as byte 0x26 and alg=-8
    # (EdDSA) encodes as byte 0x27. The resulting 3-byte sequences \xa2\x01\x26
    # (ES256) / \xa2\x01\x27 (EdDSA) are specific enough to search for
    # directly in the raw (binary, JUMBF-embedded) manifest bytes, the same
    # direct-byte-search approach steps/pdf_generation/pdf_steps.py already
    # uses for the ASCII "%%C2PA-MANIFEST-BEGIN" marker.
    es256_marker = b"\xa2\x01\x26"
    eddsa_marker = b"\xa2\x01\x27"
    assert eddsa_marker not in pdf_bytes, (
        "Exported PDF's C2PA manifest still declares COSE alg EdDSA(-8) "
        f"(found protected-header byte pattern {eddsa_marker!r}) - the PKI consolidation "
        "refactor requires ES256(-7) instead"
    )
    assert es256_marker in pdf_bytes, (
        "Exported PDF's C2PA manifest does not declare COSE alg ES256(-7) "
        f"(protected-header byte pattern {es256_marker!r} not found) - see "
        "pdf-core/compiler/compiler_c2pa.go:616-628 for the exact CBOR construction "
        "this pattern is derived from"
    )


# ---------------------------------------------------------------------------
# CRL revocation flips a previously valid signature to invalid
#
# The Given step below seeds the revocation marker (the `cert_revoked_at`
# column on `contract_signatures`) directly via context.db, mirroring the
# accepted precedent of direct-DB test seams also used by exp_date-backdating
# (steps/template_management/contract_state_machine_steps.py) and, on the
# read-only assertion side, the `sync_fails` polling in
# steps/peer_trust/dcs_trust_pdp_steps.py (ADR-19). The peer-trust package's
# OWN former DB-seeding seam (`_seed_trusted_peer`, an `INSERT INTO
# trusted_peers`) was removed when that package moved to ADR-19's
# agreement-credential/PDP model — trust is no longer a seedable DB row. The
# Then assertions on /signature/validate are the requirement-accurate,
# load-bearing part.
# ---------------------------------------------------------------------------


@given('signature validation for contract "{name}" currently reports no certificate-revocation finding')
def step_given_baseline_no_cert_revocation_finding(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(context, signature_validate_url(context), {"did": did}, headers=manager_h)
    assert resp.status_code == 200, (
        f"Baseline /signature/validate call failed for contract '{name}': "
        f"{resp.status_code} {resp.text}"
    )
    findings = resp.json().get("findings") or []
    body_text = " ".join(findings).lower()
    assert not ("revoked" in body_text and ("cert" in body_text or "crl" in body_text)), (
        f"Precondition violated: contract '{name}' already reports a certificate-"
        f"revocation finding before any CRL revocation was seeded: {findings}"
    )


@given(
    'the dev signing certificate used for contract "{name}"\'s signature has been revoked '
    "in the CRL"
)
def step_given_cert_revoked_in_crl(context, name):
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    try:
        cursor.execute(
            "UPDATE contract_signatures SET cert_revoked_at = NOW() WHERE contract_did = %s",
            (did,),
        )
        context.db.commit()
    except Exception as exc:  # noqa: BLE001
        context.db.rollback()
        raise AssertionError(
            "Could not seed the CRL-revocation test seam (the 'cert_revoked_at' "
            f"column on 'contract_signatures'): {exc}"
        ) from exc
    finally:
        cursor.close()


@when('I validate the signature for contract "{name}"')
def step_when_validate_signature(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(
        context, signature_validate_url(context), {"did": did}, headers=manager_h
    )


@then('signature validation for contract "{name}" reports the certificate as revoked')
def step_then_validate_reports_cert_revoked(context, name):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"/signature/validate failed for contract '{name}': {resp.status_code} {resp.text}"
    )
    findings = resp.json().get("findings") or []
    body_text = " ".join(findings).lower()
    assert "revoked" in body_text and ("cert" in body_text or "crl" in body_text), (
        f"Expected a certificate/CRL revocation finding (distinct from the existing "
        f"business-level '/signature/revoke' REVOKED-status finding, see "
        f"contractrepository.go's existing 'case \"REVOKED\":' handling) for contract "
        f"'{name}' after revoking its signing certificate in the CRL, got findings: "
        f"{findings}"
    )
