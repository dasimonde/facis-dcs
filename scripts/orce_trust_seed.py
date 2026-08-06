"""Seed a login issuer's pinned key into an OID4VP trust document (ADR-35).

A login issuer terminates at its leaf: the credential's certificate is accepted
because the operator named that issuer's key, not because some CA vouched for
it. So the key has to be IN the trust document — and the ORCE demo issuer
generates its key on its own volume at boot, which no committed file can know.

It publishes that key, though: <base>/pki/issuer.pem is the leaf it signs
credentials and status lists with. This reads the key out of it and writes it
down as `x5c_leaf_keys` — base64 DER SubjectPublicKeyInfo.

The pin is a KEY, not the certificate: the issuer re-mints its leaf when its
public URL changes and keeps the key, so pinning the certificate would refuse a
legitimate reissue. And it is DER rather than a JWK because the mechanism stays
`x5c` — an entry that bundled a `jwks` beside it would be stating two
resolutions for one issuer, and only one of them could be what the operator
meant.

Seeding is not the same as resolving. The operator fetches the key once, at
deploy time, and it becomes configuration the startup attestation hashes and
pins (configattest). Fetching it at verification time instead would mean the
issuer told us what key to believe it by, which is self-attestation — and for
`login`, whose whole job is to say "my organization issued this", that would be
the one thing the check must not do.

Entries are upserted one issuer at a time and everything else in the file is
left alone: more than one writer maintains this document, and one that rewrote
it wholesale would silently drop another's issuer.

Usage:
    python3 orce_trust_seed.py TRUST_JSON ISSUER_BASE_URL [ISSUER_BASE_URL ...]
                               [--organization did:web:host]

The issuer identifier written is the base URL itself, because that is what the
issuer now puts in `iss` (flow-vci.json) — not the did:web form, which named no
credential's `iss` once the issuers moved to an HTTPS `iss`.
"""

from __future__ import annotations

import argparse
import base64
import json
import sys
import urllib.request
from pathlib import Path
from urllib.parse import urlparse

from cryptography import x509
from cryptography.hazmat.primitives import serialization

# The purposes a deployment's own issuer holds. `login` is why the key is pinned
# here at all. `peer` rides along because this instance also presents its OWN
# side of the mutual Power-of-Attorney binding, and a listed entry is the
# operator's complete answer for an issuer — withholding `peer` would deny it
# rather than fall through to the anchored path (ADR-35).
OWN_ISSUER_PURPOSES = ["login", "peer"]


def fetch_issuer_leaf_key(base_url: str) -> str:
    """The base64 DER SubjectPublicKeyInfo of the key this issuer signs with.

    Refused rather than defaulted when it is unreachable or malformed: a trust
    document seeded with no pin would load and then refuse every login, naming
    the issuer instead of this step.
    """
    url = base_url.rstrip("/") + "/pki/issuer.pem"
    try:
        with urllib.request.urlopen(url, timeout=20) as response:
            pem = response.read()
    except Exception as error:  # noqa: BLE001 - reported verbatim, then fatal
        raise SystemExit(f"{url}: could not read the issuer's leaf certificate: {error}")

    try:
        certificate = x509.load_pem_x509_certificate(pem)
    except Exception as error:  # noqa: BLE001
        raise SystemExit(f"{url}: did not serve a PEM certificate: {error}")

    spki = certificate.public_key().public_bytes(
        serialization.Encoding.DER,
        serialization.PublicFormat.SubjectPublicKeyInfo,
    )
    return base64.b64encode(spki).decode()


def organization_for(base_url: str) -> str:
    """The party this issuer speaks for, derived the way the policy derives it.

    An issuer at https://host/issuer attests did:web:host and nothing else
    (policy/trust.rego, peer_authority). Deriving it here rather than asking for
    it keeps the seeded entry consistent with what the verifier would allow an
    unlisted issuer at the same identifier to claim.
    """
    authority = urlparse(base_url).netloc
    if not authority:
        raise SystemExit(f"{base_url}: is not an absolute URL, so it names no authority")
    return "did:web:" + authority.replace(":", "%3A")


def upsert_issuer(trust: dict, iss: str, leaf_key: str, organization: str) -> None:
    issuers = trust.setdefault("issuers", {})
    issuers[iss] = {
        "purposes": list(OWN_ISSUER_PURPOSES),
        "organizations": [organization],
        "mechanism": "x5c",
        "x5c_leaf_keys": [leaf_key],
    }


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("trust_json", type=Path)
    parser.add_argument("issuer_base_urls", nargs="+")
    parser.add_argument(
        "--organization",
        help="Party this issuer speaks for. Derived from the issuer's own authority when omitted.",
    )
    args = parser.parse_args(argv)

    if not args.trust_json.is_file():
        raise SystemExit(f"{args.trust_json}: no such trust document")
    trust = json.loads(args.trust_json.read_text())

    for base_url in args.issuer_base_urls:
        iss = base_url.rstrip("/")
        leaf_key = fetch_issuer_leaf_key(iss)
        organization = args.organization or organization_for(iss)
        upsert_issuer(trust, iss, leaf_key, organization)
        print(f"pinned leaf key for {iss} (organization {organization})")

    args.trust_json.write_text(json.dumps(trust, indent=2) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
