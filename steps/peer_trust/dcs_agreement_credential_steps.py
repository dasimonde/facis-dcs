"""BDD steps for the federation agreement credential (ADR-19,
docs/adr-19-federation-agreement-credential.md, requirement slug
`fed-agreement`): the self-signed W3C Verifiable Credential every instance
publishes at GET /.well-known/dcs-agreement-credential.json, next to its
did.json, naming the federation rules it has bound itself to.

Per the ADR's implementation-state table this endpoint does not exist yet
(state: "pending") — every scenario here is expected to fail red today; that
is the point of writing them ahead of the implementation.

AC1 (issuer/termsOfUse shape) and AC2 (signature verifies against the
second verificationMethod in this instance's own did.json) are both
single-instance: the credential a running instance publishes about itself is
fully observable without a second peer. AC3's rule-hash-equality half lives
here too since it only needs to compare the SAME field (termsOfUse.hash) on
both instances' credentials, reusing the two-instance Given from
dcs_peer_trust_steps.py.

AC2 proof format
-----------------
The credential carries an `ecdsa-rdfc-2019` `DataIntegrityProof` (W3C
VC-DI-ECDSA, RDFC-1.0/URDNA2015 canonicalization) — NOT a JsonWebSignature2020
detached JWS as an earlier draft of this step assumed. This is the SAME
scheme this codebase already uses for provenance credentials (see
backend/internal/pdfgeneration/provenance/vc_signer_hsm.go's HSMVCSigner /
vc_verifier.go's VerifyDataIntegrityProof): canonicalize the proof options
(the proof object minus `proofValue`, with its own
`@context: ["https://w3id.org/security/data-integrity/v2"]`) and the
document-without-proof separately via RDFC-1.0, SHA-256 each, concatenate
(proofHash || docHash), SHA-256 that concatenation again, and ECDSA-P256
verify the multibase-base58btc-decoded `proofValue` (raw r||s) against that
final digest — using the EC public key from the JWK (crv/x/y) published at
did.json's SECOND verificationMethod, not an x5c certificate.

This step reuses the EXACT SAME embedded JSON-LD context documents the Go
signer/verifier load
(backend/internal/pdfgeneration/provenance/contexts/credentials-v2.json,
data-integrity-v2.json) via a local, offline document loader, so
canonicalization is byte-identical and does not depend on network access to
w3.org/w3id.org for the two known context IRIs. Any OTHER context IRI (e.g.
a not-yet-known dcs-specific extension context for termsOfUse's policyId/
hash terms) falls through to pyld's normal network-fetching loader.

Needs `pyld` (RDFC-1.0/URDNA2015 canonicalization) in the BDD venv — as of
this writing NOT a dependency there (tests/bdd/Makefile setup_environment);
imported lazily inside the one step that needs it so the rest of this
module's steps (AC1, AC3) still register and run without it.
"""

import base64
import hashlib
import json
from pathlib import Path

import requests as _requests
from behave import given, then, when
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import ec, utils as ec_utils

from steps.support.api_client import agreement_credential_url, did_document_url


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _fetch_agreement_credential(context, base_url):
    return _requests.get(agreement_credential_url(base_url), timeout=context.http_timeout_seconds)


def _fetch_did_document(context, base_url):
    resp = _requests.get(did_document_url(base_url), timeout=context.http_timeout_seconds)
    assert resp.status_code == 200, (
        f"could not fetch did.json from {did_document_url(base_url)}: {resp.status_code} {resp.text}"
    )
    return resp.json()


def _b64url_decode(segment: str) -> bytes:
    return base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4))


def _terms_of_use(credential: dict) -> dict:
    """termsOfUse may legally be a single object or an array (W3C VC Data
    Model 2.0 §5.5) — normalize to the single TrustFrameworkPolicy entry this
    credential is expected to carry."""
    tou = credential.get("termsOfUse")
    if isinstance(tou, list):
        assert tou, "credential carries an empty termsOfUse array"
        matching = [t for t in tou if t.get("type") == "TrustFrameworkPolicy"]
        assert matching, f"no termsOfUse entry of type TrustFrameworkPolicy, got: {tou}"
        return matching[0]
    assert isinstance(tou, dict), f"expected termsOfUse to be an object or array, got: {tou!r}"
    return tou


# ---------------------------------------------------------------------------
# AC1: GET /.well-known/dcs-agreement-credential.json shape
# ---------------------------------------------------------------------------


@when("I fetch this instance's own agreement credential")
@given("I fetch this instance's own agreement credential")
def step_when_fetch_own_agreement_credential(context):
    context.requests_response = _fetch_agreement_credential(context, context.base_url)


@then("the agreement credential is a W3C Verifiable Credential whose issuer is this instance's own DID")
def step_then_credential_issuer_is_own_did(context):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected GET /.well-known/dcs-agreement-credential.json to succeed, got "
        f"{resp.status_code}: {resp.text}"
    )
    credential = resp.json()
    vc_types = credential.get("type") or credential.get("@type") or []
    if isinstance(vc_types, str):
        vc_types = [vc_types]
    assert "VerifiableCredential" in vc_types, (
        f"Expected the credential's type to include 'VerifiableCredential' (W3C VC Data Model "
        f"2.0), got: {vc_types}"
    )
    context.own_did_document = _fetch_did_document(context, context.base_url)
    own_did = context.own_did_document.get("id")
    assert own_did, f"own did.json has no 'id': {context.own_did_document}"

    issuer = credential.get("issuer")
    issuer_id = issuer.get("id") if isinstance(issuer, dict) else issuer
    assert issuer_id == own_did, (
        f"Expected the agreement credential's issuer to be this instance's own DID "
        f"({own_did}), got: {issuer_id!r}"
    )
    context.own_agreement_credential = credential


@then("the agreement credential's termsOfUse names the federation rules by policyId and hash")
def step_then_credential_terms_of_use_shape(context):
    credential = context.requests_response.json()
    tou = _terms_of_use(credential)
    assert tou.get("type") == "TrustFrameworkPolicy", (
        f"Expected termsOfUse.type == 'TrustFrameworkPolicy', got: {tou.get('type')!r}"
    )
    assert tou.get("policyId"), f"Expected a non-empty termsOfUse.policyId, got: {tou}"
    assert tou.get("hash"), f"Expected a non-empty termsOfUse.hash (of the embedded rules document), got: {tou}"


# ---------------------------------------------------------------------------
# AC2: credential signature verifies against did.json's second
# verificationMethod (the dedicated VC key, distinct from the eIDAS/JAdES
# signing key at verificationMethod[0]).
# ---------------------------------------------------------------------------


def _find_verification_method(did_document: dict, method_id: str) -> dict:
    methods = did_document.get("verificationMethod") or []
    matching = [m for m in methods if m.get("id") == method_id]
    assert matching, (
        f"No verificationMethod with id '{method_id}' in did.json — proof.verificationMethod "
        f"must reference an entry actually published there, got ids: "
        f"{[m.get('id') for m in methods]}"
    )
    return matching[0]


def _ec_public_key_from_jwk(vm: dict):
    """EC public key straight from the JWK's crv/x/y (base64url, RFC 7518
    §6.2.1) — NOT the x5c certificate chain: the coordinator's correction for
    AC2 is that the agreement-credential proof verifies against the
    published JWK itself, distinct from the x5c-based eIDAS/JAdES check
    verificationMethod[0] carries."""
    jwk = vm.get("publicKeyJwk") or {}
    crv = jwk.get("crv")
    assert crv == "P-256", (
        f"verificationMethod '{vm.get('id')}' publicKeyJwk.crv must be 'P-256', got {crv!r}"
    )
    x = jwk.get("x")
    y = jwk.get("y")
    assert x and y, f"verificationMethod '{vm.get('id')}' publicKeyJwk carries no x/y: {jwk}"
    x_int = int.from_bytes(_b64url_decode(x), "big")
    y_int = int.from_bytes(_b64url_decode(y), "big")
    from cryptography.hazmat.primitives.asymmetric.ec import (  # noqa: PLC0415
        SECP256R1,
        EllipticCurvePublicNumbers,
    )

    return EllipticCurvePublicNumbers(x_int, y_int, SECP256R1()).public_key()


_DATA_INTEGRITY_CONTEXT = "https://w3id.org/security/data-integrity/v2"

# The exact same locally embedded JSON-LD context documents the Go signer/
# verifier use (backend/internal/pdfgeneration/provenance/vc_contexts.go) —
# read directly from the backend source tree (read-only; no production code
# is modified) so RDFC-1.0 canonicalization is byte-identical and does not
# depend on network access to w3.org/w3id.org for these two known IRIs.
_LOCAL_CONTEXT_FILES = {
    "https://www.w3.org/ns/credentials/v2": (
        "backend/internal/pdfgeneration/provenance/contexts/credentials-v2.json"
    ),
    _DATA_INTEGRITY_CONTEXT: "backend/internal/pdfgeneration/provenance/contexts/data-integrity-v2.json",
}
_context_cache: dict = {}


def _repo_root() -> Path:
    # This file lives at steps/peer_trust/dcs_agreement_credential_steps.py.
    return Path(__file__).resolve().parents[2]


def _local_context_document_loader():
    """A pyld documentLoader that serves the two known VC/Data-Integrity
    context IRIs from the backend's own embedded copies (offline, byte-exact
    match with the Go canonicalizer), falling through to pyld's default
    network-fetching loader for any other IRI — e.g. a not-yet-known
    dcs-specific extension context for termsOfUse's policyId/hash terms,
    which this step cannot predict since the endpoint does not exist yet."""

    def loader(url, options=None):
        rel_path = _LOCAL_CONTEXT_FILES.get(url)
        if rel_path is not None:
            if url not in _context_cache:
                doc_path = _repo_root() / rel_path
                _context_cache[url] = json.loads(doc_path.read_text(encoding="utf-8"))
            return {"contextUrl": None, "documentUrl": url, "document": _context_cache[url]}
        from pyld.jsonld import requests_document_loader  # noqa: PLC0415

        return requests_document_loader()(url, options)

    return loader


def _normalize_rdfc(doc: dict) -> str:
    from pyld import jsonld  # noqa: PLC0415 — see module docstring: optional/lazy dependency

    options = {
        "algorithm": "URDNA2015",  # RDFC-1.0 is the standardized successor of URDNA2015
        "format": "application/n-quads",
        "documentLoader": _local_context_document_loader(),
    }
    return jsonld.normalize(doc, options)


_B58_ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"


def _multibase_base58btc_decode(value: str) -> bytes:
    """Decodes a multibase 'z'-prefixed base58btc string (the encoding
    ecdsa-rdfc-2019 uses for proofValue — see
    backend/internal/pdfgeneration/provenance/vc_signer_hsm.go's
    `"z" + base58.Encode(sig)`). Hand-rolled rather than adding a `base58`
    dependency: the algorithm is a few lines and this avoids asking for a
    second new BDD-venv package on top of `pyld`.
    """
    assert value.startswith("z"), f"expected a multibase base58btc value ('z' prefix), got: {value!r}"
    s = value[1:]
    num = 0
    for char in s:
        idx = _B58_ALPHABET.find(char)
        assert idx >= 0, f"invalid base58 character {char!r} in proofValue"
        num = num * 58 + idx
    body = num.to_bytes((num.bit_length() + 7) // 8, "big") if num else b""
    leading_zeros = len(s) - len(s.lstrip("1"))
    return b"\x00" * leading_zeros + body


@then("the credential's proof verifies against the key it names, which this instance publishes for assertions and not for authentication")
def step_then_credential_proof_verifies(context):
    """Verifies the credential's ecdsa-rdfc-2019 DataIntegrityProof exactly
    like backend/internal/pdfgeneration/provenance/vc_verifier.go's
    VerifyDataIntegrityProof does (see module docstring for the full
    algorithm) — against the EC public key of the method the PROOF names
    (publicKeyJwk, not an x5c certificate). Which entry of verificationMethod
    that happens to be is not asserted: the receiver resolves it by id within the
    assertionMethod relationship (dcstodcs.trustgate), so position is exactly what
    must not matter."""
    resp = _fetch_agreement_credential(context, context.base_url)
    assert resp.status_code == 200, (
        f"Expected GET /.well-known/dcs-agreement-credential.json to succeed, got "
        f"{resp.status_code}: {resp.text}"
    )
    credential = resp.json()
    proof = credential.get("proof")
    assert isinstance(proof, dict), f"Expected a single embedded 'proof' object, got: {proof!r}"

    assert proof.get("type") == "DataIntegrityProof", (
        f"Expected proof.type == 'DataIntegrityProof' (W3C VC-DI-ECDSA), got: {proof.get('type')!r}"
    )
    assert proof.get("cryptosuite") == "ecdsa-rdfc-2019", (
        f"Expected proof.cryptosuite == 'ecdsa-rdfc-2019', got: {proof.get('cryptosuite')!r}"
    )

    method_id = proof.get("verificationMethod")
    assert method_id, f"proof carries no verificationMethod, got: {proof}"

    did_document = _fetch_did_document(context, context.base_url)

    def relationship_ids(relationship):
        ids = []
        for entry in did_document.get(relationship) or []:
            entry_id = entry.get("id") if isinstance(entry, dict) else entry
            if isinstance(entry_id, str):
                ids.append(
                    f"{did_document.get('id')}{entry_id}" if entry_id.startswith("#") else entry_id
                )
        return ids

    assert method_id in relationship_ids("assertionMethod"), (
        f"Expected proof.verificationMethod ({method_id!r}) to be published under "
        f"assertionMethod — being present in did.json is not authorization to sign a "
        f"credential — got: {did_document.get('assertionMethod')!r}"
    )
    assert method_id not in relationship_ids("authentication"), (
        f"The agreement credential must carry its OWN dedicated key (ADR-19), but "
        f"{method_id!r} is also the key this instance authenticates itself to peers with."
    )
    vm = _find_verification_method(did_document, method_id)
    pub = _ec_public_key_from_jwk(vm)

    proof_value = proof.get("proofValue", "")
    sig = _multibase_base58btc_decode(proof_value)
    assert len(sig) == 64, f"Expected a 64-byte raw ECDSA signature (r||s), got {len(sig)} bytes"

    proof_options = {k: v for k, v in proof.items() if k != "proofValue"}
    proof_options["@context"] = [_DATA_INTEGRITY_CONTEXT]
    doc_without_proof = {k: v for k, v in credential.items() if k != "proof"}

    proof_nq = _normalize_rdfc(proof_options)
    doc_nq = _normalize_rdfc(doc_without_proof)
    proof_hash = hashlib.sha256(proof_nq.encode()).digest()
    doc_hash = hashlib.sha256(doc_nq.encode()).digest()
    digest = hashlib.sha256(proof_hash + doc_hash).digest()

    r = int.from_bytes(sig[:32], "big")
    s = int.from_bytes(sig[32:], "big")
    der_sig = ec_utils.encode_dss_signature(r, s)
    # Raises cryptography.exceptions.InvalidSignature if verification fails —
    # let it propagate so a broken/self-signed-with-the-wrong-key credential
    # fails this scenario loudly rather than silently. Prehashed: `digest` is
    # already the final SHA-256 the Go verifier computes and hands straight
    # to ecdsa.Verify — cryptography must be told not to hash it again.
    pub.verify(der_sig, digest, ec.ECDSA(ec_utils.Prehashed(hashes.SHA256())))


# ---------------------------------------------------------------------------
# AC3 (rule-hash equality half — the offer-replication half is covered by the
# existing cross-instance scenario in dcs_peer_trust_steps.py).
# ---------------------------------------------------------------------------


@then("instance A and instance B's agreement credentials name the identical federation rules hash")
def step_then_same_rules_hash_both_instances(context):
    resp_a = _fetch_agreement_credential(context, context.base_url_a)
    resp_b = _fetch_agreement_credential(context, context.base_url_b)
    assert resp_a.status_code == 200, (
        f"Expected instance A's agreement credential to be published, got {resp_a.status_code}: {resp_a.text}"
    )
    assert resp_b.status_code == 200, (
        f"Expected instance B's agreement credential to be published, got {resp_b.status_code}: {resp_b.text}"
    )
    hash_a = _terms_of_use(resp_a.json()).get("hash")
    hash_b = _terms_of_use(resp_b.json()).get("hash")
    assert hash_a, f"instance A's credential carries no termsOfUse.hash: {resp_a.json()}"
    assert hash_a == hash_b, (
        f"Expected both instances of the same build to embed the identical federation rules hash "
        f"(go:embed, ADR-19) — instance A: {hash_a!r}, instance B: {hash_b!r}"
    )
