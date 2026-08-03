"""The one synthetic peer that PASSES ADR-19's agreement-credential check
(layer 3a), so an inbound scenario can put the policy endpoint (layer 3b)
under test on its own.

The other two synthetic peers cannot do this by construction:

* a case-varied copy of this instance's own DID resolves to this instance, and
  PostPdf refuses it as the same peer (identity.SameDIDWeb) before any trust
  layer runs;
* the orce synthetic-peer route (did:web:dcs-orce%3A1880) 404s its agreement
  credential on purpose — that IS the AC4 case — so layer 3a refuses every
  ship towards or from it.

Mirroring this instance's did.json (the trick the other orce routes use to keep
the challenge-response genuine) cannot be extended to a peer that passes layer
3a: the gate requires the credential's issuer to resolve to the SAME target the
credential was fetched from (trustgate.verifyAgreementCredential) and its proof
to name a key that document publishes as an assertionMethod. A mirrored did.json
carries THIS instance's id and keys, so the credential would have to be signed
with this instance's HSM-held key and issued by this instance — which is exactly
what the issuer check refuses to accept from another host.

So this peer is a genuinely separate authority with key material of its own:
this module mints a P-256 key pair per scenario, self-signs a did.json and a
federation agreement credential with it, and publishes both through the orce
flow's control surface (POST {control}/synthetic-trusted/identity, served back
under Host dcs-orce-trusted by
deployment/helm/charts/orce/flows/synthetic-peer-flow.json). Nothing about the
identity is checked in, deliberately: the credential's termsOfUse.hash must
name the federation rules hash of the build under test, and a checked-in
credential stops doing that the day backend/internal/base/federation/rules.md
changes. The hash is therefore read from the instance's own published
credential at scenario time.

The eIDAS layer (layer 1) accepts this identity because the certificate of the
key that ANSWERS the challenge is issued for this peer's own hostname and matches
that key's own JWK; chain validation and the qualified-certificate statement only
apply where DCS_FORCE_EIDAS_CERT is on, which no dev/CI stack sets (see
cmd/dcs/main.go). The challenge-response (layer 2) is signed here with the
matching private key, so it verifies genuinely rather than by mirroring.
"""

import base64
import hashlib
import json
import os
import uuid
from datetime import datetime, timedelta, timezone

import requests as _requests
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils as ec_utils
from cryptography.x509.oid import NameOID

from steps.peer_trust.dcs_agreement_credential_steps import (
    _DATA_INTEGRITY_CONTEXT,
    _normalize_rdfc,
    _terms_of_use,
)
from steps.support.api_client import agreement_credential_url

# Deliberately NOT the key labels the instance under test uses for itself
# ("dev-key-1", DCS_HSM_KEY_VC/"dcs-vc"): a peer names its own keys, and the
# receiver has to resolve them from what this document publishes them FOR —
# the credential's proof names its key, the challenge-response is answered by an
# authentication key — rather than from its own naming convention.
VC_KEY_LABEL = os.getenv("BDD_TRUST_VC_KEY_LABEL", "peer-credential-key")
SIGNING_KEY_LABEL = "peer-identity-key"

_B58_ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"


def trusted_peer_did() -> str:
    """DID of the orce route that publishes what this module signs — a live
    contract with the deployment side (the `<release>-orce-trusted` Service
    alias and the Host-header routing in synthetic-peer-flow.json), overridable
    for a deployment that names it differently."""
    return os.getenv("BDD_TRUST_TRUSTED_PEER_DID", "did:web:dcs-orce-trusted%3A1880")


def _control_url() -> str:
    """Same orce control plane the PDP mode is steered through — this BDD
    process reaches the orce pod via its own port-forward, never via the
    in-cluster Service name the backend resolves."""
    return os.getenv("BDD_TRUST_PDP_CONTROL_URL", "http://localhost:18880").rstrip("/")


def _hostname(did: str) -> str:
    return did[len("did:web:"):].replace("%3A", ":").replace("%3a", ":")


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def _b58encode(raw: bytes) -> str:
    num = int.from_bytes(raw, "big")
    out = ""
    while num:
        num, rem = divmod(num, 58)
        out = _B58_ALPHABET[rem] + out
    return "1" * (len(raw) - len(raw.lstrip(b"\x00"))) + out


def _public_jwk(public_key, kid: str, certificate: x509.Certificate = None) -> dict:
    numbers = public_key.public_numbers()
    jwk = {
        "kty": "EC",
        "crv": "P-256",
        "alg": "ES256",
        "kid": kid,
        "x": _b64url(numbers.x.to_bytes(32, "big")),
        "y": _b64url(numbers.y.to_bytes(32, "big")),
    }
    if certificate is not None:
        # Standard base64 DER, RFC 7517 §4.7 — what
        # identity.DIDDocument.loadCertificateChain decodes.
        jwk["x5c"] = [base64.b64encode(certificate.public_bytes(serialization.Encoding.DER)).decode()]
    return jwk


def _self_signed_certificate(key, hostname: str) -> x509.Certificate:
    """A certificate for the peer's OWN hostname: identity.VerifyEIDASCertificate
    checks it against the did:web authority (port stripped) and against the
    published JWK, both of which this satisfies. It chains to nothing, which
    only matters under DCS_FORCE_EIDAS_CERT."""
    host = hostname.split(":")[0]
    subject = x509.Name([
        x509.NameAttribute(NameOID.COUNTRY_NAME, "DE"),
        x509.NameAttribute(NameOID.ORGANIZATION_NAME, "DCS Test"),
        x509.NameAttribute(NameOID.COMMON_NAME, hostname),
    ])
    now = datetime.now(timezone.utc)
    return (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=5))
        .not_valid_after(now + timedelta(days=1))
        .add_extension(x509.SubjectAlternativeName([x509.DNSName(host)]), critical=False)
        .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
        .sign(key, hashes.SHA256())
    )


def _did_document(did: str, signing_certificate, signing_key, vc_key) -> dict:
    """A conformant document that shares none of this deployment's habits: the
    credential key comes FIRST (verificationMethod order carries no meaning in
    DID Core, so no consumer may rely on the identity key being entry zero), the
    relationships name their keys with relative DID URLs (permitted, and what a
    consumer comparing raw strings fails on), and the identity key is published
    under `authentication` — the relationship a challenge-response actually
    needs — while the credential key is only allowed to assert."""
    return {
        "@context": [
            "https://www.w3.org/ns/did/v1",
            "https://w3id.org/security/suites/jws-2020/v1",
        ],
        "id": did,
        "verificationMethod": [
            {
                "id": f"{did}#{VC_KEY_LABEL}",
                "type": "JsonWebKey2020",
                "controller": did,
                "publicKeyJwk": _public_jwk(vc_key.public_key(), VC_KEY_LABEL),
            },
            {
                "id": f"{did}#{SIGNING_KEY_LABEL}",
                "type": "JsonWebKey2020",
                "controller": did,
                "publicKeyJwk": _public_jwk(
                    signing_key.public_key(), SIGNING_KEY_LABEL, signing_certificate
                ),
            },
        ],
        "authentication": [f"#{SIGNING_KEY_LABEL}"],
        "assertionMethod": [f"#{SIGNING_KEY_LABEL}", f"#{VC_KEY_LABEL}"],
    }


def _sign_data_integrity_proof(document: dict, key, verification_method: str, created: str) -> dict:
    """The ecdsa-rdfc-2019 Data Integrity proof
    backend/internal/pdfgeneration/provenance/vc_signer_hsm.go produces and
    vc_verifier.go checks: RDFC-1.0-canonicalize the proof options and the
    document separately, SHA-256 each, SHA-256 the concatenation, and sign that
    digest — the signing counterpart of the AC2 step's verification."""
    proof_options = {
        "type": "DataIntegrityProof",
        "cryptosuite": "ecdsa-rdfc-2019",
        "created": created,
        "proofPurpose": "assertionMethod",
        "verificationMethod": verification_method,
    }
    canonical_options = dict(proof_options, **{"@context": [_DATA_INTEGRITY_CONTEXT]})
    proof_hash = hashlib.sha256(_normalize_rdfc(canonical_options).encode()).digest()
    document_hash = hashlib.sha256(_normalize_rdfc(document).encode()).digest()
    digest = hashlib.sha256(proof_hash + document_hash).digest()

    der = key.sign(digest, ec.ECDSA(ec_utils.Prehashed(hashes.SHA256())))
    r, s = ec_utils.decode_dss_signature(der)
    raw = r.to_bytes(32, "big") + s.to_bytes(32, "big")
    return dict(proof_options, proofValue="z" + _b58encode(raw))


def _agreement_credential(did: str, rules_hash: str, vc_key) -> dict:
    """The same document federation.BuildAgreementCredential builds, issued by
    this peer about itself: same @context (including the dcs vocabulary, so the
    termsOfUse terms are covered by the signature rather than dropped as
    unmapped), same termsOfUse shape, and the rules hash of the instance the
    credential is going to be presented to."""
    minted = datetime.now(timezone.utc).replace(microsecond=0)
    now = minted.strftime("%Y-%m-%dT%H:%M:%SZ")
    # The gate requires a bounded validUntil and refuses a window longer than
    # federation.MaxAgreementCredentialLifetime, so a credential without one — or
    # with an open-ended one — is refused before the PDP is ever consulted, which
    # is not what an AC7/AC8/AC9 scenario is about. An hour outlives any scenario.
    valid_until = (minted + timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
    credential = {
        "@context": [
            "https://www.w3.org/ns/credentials/v2",
            _DATA_INTEGRITY_CONTEXT,
            {
                "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
                "FederationAgreementCredential": "dcs:FederationAgreementCredential",
                "TrustFrameworkPolicy": "dcs:TrustFrameworkPolicy",
                "policyId": {"@id": "dcs:policyId", "@type": "@id"},
                "hash": "dcs:rulesHash",
            },
        ],
        "type": ["VerifiableCredential", "FederationAgreementCredential"],
        "issuer": did,
        "validFrom": now,
        "validUntil": valid_until,
        "credentialSubject": {"id": did},
        "termsOfUse": {
            "type": "TrustFrameworkPolicy",
            "policyId": f"http://{_hostname(did)}/.well-known/dcs-federation-rules.md",
            "hash": rules_hash,
        },
    }
    credential["id"] = "urn:dcs:vc:" + hashlib.sha256(
        json.dumps(credential, sort_keys=True).encode()
    ).hexdigest()
    credential["proof"] = _sign_data_integrity_proof(
        credential, vc_key, f"{did}#{VC_KEY_LABEL}", now
    )
    return credential


def _own_rules_hash(context) -> str:
    """The federation rules hash of the build under test, read from its own
    published credential — never a constant: two instances of one build agree on
    it (AC3), and a peer naming any other value is exactly what AC5 rejects."""
    url = agreement_credential_url(context.base_url)
    resp = _requests.get(url, timeout=context.http_timeout_seconds)
    assert resp.status_code == 200, (
        f"could not read this instance's own federation rules hash from {url} "
        f"({resp.status_code} {resp.text}) — the trusted synthetic peer's credential has to name "
        "the hash of the build under test"
    )
    rules_hash = _terms_of_use(resp.json()).get("hash")
    assert rules_hash, f"this instance's own agreement credential carries no termsOfUse.hash: {resp.text}"
    return rules_hash


def publish_trusted_peer(context):
    """Mints and publishes the identity, and returns the peer DID plus a
    challenge-response pair signed with its own signing key — the shape
    PostPdf's from_peer_did/secret_value/secret_hash expects."""
    did = trusted_peer_did()
    signing_key = ec.generate_private_key(ec.SECP256R1())
    vc_key = ec.generate_private_key(ec.SECP256R1())
    certificate = _self_signed_certificate(signing_key, _hostname(did))

    payload = {
        "didDocument": _did_document(did, certificate, signing_key, vc_key),
        "agreementCredential": _agreement_credential(did, _own_rules_hash(context), vc_key),
    }
    control = _control_url()
    resp = _requests.post(
        f"{control}/synthetic-trusted/identity", json=payload, timeout=context.http_timeout_seconds
    )
    assert resp.status_code == 200, (
        f"could not publish the trusted synthetic peer identity via {control}"
        f"/synthetic-trusted/identity — is the orce port-forward up and is "
        f"orce.syntheticPeer.enabled set? Got {resp.status_code}: {resp.text}"
    )

    secret_value = str(uuid.uuid4())
    signature = signing_key.sign(secret_value.encode(), ec.ECDSA(hashes.SHA256()))
    return did, secret_value, base64.b64encode(signature).decode()
