"""Status list helpers for testWallet issuance and dev scripts.

Every credential this wallet issues points at the ORCE issuer's own status
list (deployment/helm/charts/orce/flows-issuer/flow-statuslist.json):

  GET  <issuer base>/status-list/1            application/statuslist+jwt
  POST <issuer base>/admin/credentials/<idx>/revoke
  POST <issuer base>/admin/credentials/<idx>/unrevoke

The list is ES256-signed with the issuer's own certificate chain in the JWT
header and its bits are persisted on the issuer's volume, so a revocation
survives a pod restart and a verifier can tell who vouched for the answer.

The issuer's `sub` claim is built from the request's host and forwarded prefix
and the verifier requires sub == the URI in the credential
(backend/internal/auth/oid4vp/status/handler/ietf_token.go), so ISSUER_BASE_URL
must be the exact URL the DCS backend will fetch, path prefix included.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import struct
import time
import zlib
from pathlib import Path
from typing import Any
from urllib.error import HTTPError
from urllib.request import HTTPRedirectHandler, Request, build_opener, urlopen

ISSUER_BASE_ENV = "ISSUER_BASE_URL"
DEFAULT_ISSUER_BASE = "http://localhost:30181"
STATUS_LIST_NUMBER = 1
STATUS_LIST_MEDIA_TYPE = "application/statuslist+jwt"

# The issuer provisions 1250 bytes of one-bit slots (flow-boot.json
# DEFAULT_BITS_LEN) and only grows the buffer when a revocation needs a byte
# past the end. A GET never grows it, so an index at or beyond this is out of
# range for the served list and the verifier rejects the credential outright.
STATUS_LIST_SIZE = 10_000

# How the 10 000 slots are divided. The blocks exist so that a revocation can
# never reach a credential it was not aimed at:
#
#   0     – 999   the issuer's OWN OID4VCI issuance. flow-vci.json hands out
#                 registry.length, counting up from 0, to every credential a
#                 wallet claims through /offer. Nothing here may allocate into
#                 that range.
#   1000  – 1999  the committed fixture credentials in testWallet/credentials,
#                 one named index each (FIXTURE_INDEX below).
#   2000  – 2099  identities a BDD scenario deliberately revokes. A revocation
#                 stands for the whole run, so these must be indices nothing
#                 else can land on — hence named, not derived.
#   2100  – 9999  identities BDD mints per scenario. Derived from the identity
#                 so the same identity always reads the same bit; two different
#                 identities may collide, which is harmless precisely because
#                 nothing in this block is ever revoked. A scenario that revokes
#                 belongs in RESERVED_INDEX above, not here.
FIXTURE_BLOCK_START = 1000
RESERVED_BLOCK_START = 2000
RUNTIME_BLOCK_START = 2100
RUNTIME_BLOCK_SIZE = STATUS_LIST_SIZE - RUNTIME_BLOCK_START

# One index per committed credential file: "<stem>" is the Power of Attorney
# (<stem>.jwt), "<stem>.pid" the PID (<stem>.pid.jwt). Add a line here when you
# add a template — issuance refuses a credential with no index of its own,
# because sharing one means revoking either revokes both.
FIXTURE_INDEX: dict[str, int] = {
    "alicewilliams": 1000,
    "alicewilliams.pid": 1001,
    "bobjohnson": 1002,
    "bobjohnson.pid": 1003,
    "charliebrown": 1004,
    "charliebrown.pid": 1005,
    "dev-stack-signing": 1006,
    "dev-stack2-signing": 1007,
    "janesmith": 1008,
    "janesmith.pid": 1009,
    "johndoe": 1010,
    "johndoe.pid": 1011,
    "mightyuser1": 1012,
    "mightyuser1.pid": 1013,
    "mightyuser2": 1014,
    "mightyuser2.pid": 1015,
    "saoirseconrad": 1016,
    "saoirseconrad.pid": 1017,
    "test": 1018,
    "test.pid": 1019,
    "test2": 1020,
    "test2.pid": 1021,
    "test3": 1022,
    "test3.pid": 1023,
    "test4": 1024,
    "test4.pid": 1025,
}

# Identities whose bit a scenario flips. Keyed by the organization (a PoA
# credential) or by "<given> <family>" (a PID), which is what makes a BDD
# identity distinct.
RESERVED_INDEX: dict[str, int] = {
    # features/01_authentication — revoked login credential (auth_steps.py)
    "BDD Revocation Probe Org": 2000,
    # features/22_real_signing_vertical/signing_acceptance_hardening.feature —
    # revoked PID at the ceremony presentation. The signatory name is the
    # ceremony's field name, so this person exists in that scenario alone.
    "SAHRevokedPidField BDD-Testperson": 2001,
}


def _validate_allocation() -> None:
    for name, table, start, end in (
        ("FIXTURE_INDEX", FIXTURE_INDEX, FIXTURE_BLOCK_START, RESERVED_BLOCK_START),
        ("RESERVED_INDEX", RESERVED_INDEX, RESERVED_BLOCK_START, RUNTIME_BLOCK_START),
    ):
        used: dict[int, str] = {}
        for key, idx in table.items():
            if not start <= idx < end:
                raise ValueError(f"{name}[{key!r}] = {idx} is outside its block {start}..{end - 1}")
            if idx in used:
                raise ValueError(f"{name}: {key!r} and {used[idx]!r} both claim index {idx}")
            used[idx] = key


_validate_allocation()


def issuer_base_url(issuer_base: str | None = None) -> str:
    base = (issuer_base or os.getenv(ISSUER_BASE_ENV, "") or DEFAULT_ISSUER_BASE).strip().rstrip("/")
    if not base.startswith("http://") and not base.startswith("https://"):
        base = f"http://{base}"
    return base


def status_list_uri(issuer_base: str | None = None) -> str:
    return f"{issuer_base_url(issuer_base)}/status-list/{STATUS_LIST_NUMBER}"


def fixture_index(credential_key: str) -> int:
    """Index of a committed testWallet credential; refuses an unregistered one."""
    try:
        return FIXTURE_INDEX[credential_key]
    except KeyError:
        raise KeyError(
            f"no status-list index allocated for credential {credential_key!r} — "
            f"add it to FIXTURE_INDEX in {__name__} (block "
            f"{FIXTURE_BLOCK_START}..{RESERVED_BLOCK_START - 1}); a credential "
            f"may not share another's index"
        ) from None


def _derived_index(identity: str) -> int:
    digest = hashlib.sha256(identity.encode("utf-8")).digest()
    return RUNTIME_BLOCK_START + struct.unpack(">I", digest[:4])[0] % RUNTIME_BLOCK_SIZE


def role_credential_index(*, organization: str, roles: list[str]) -> int:
    """Index of a Power of Attorney credential minted during a test run.

    A reservation is keyed by organization alone, so every role set inside a
    dedicated probe organization shares that one reserved bit — which is the
    point of such an organization: it is revoked wholesale and reaches nothing
    else.
    """
    reserved = RESERVED_INDEX.get(organization)
    if reserved is not None:
        return reserved
    return _derived_index(f"{organization}\x1e{','.join(roles)}")


def pid_credential_index(*, given_name: str, family_name: str) -> int:
    """Index of a PID minted during a test run: one bit per natural person."""
    person = f"{given_name} {family_name}"
    reserved = RESERVED_INDEX.get(person)
    if reserved is not None:
        return reserved
    return _derived_index(person)


def build_credential_status(*, index: int, issuer_base: str | None = None) -> dict[str, Any]:
    """IETF status.status_list reference for a credential's visible claims."""
    if not 0 <= index < STATUS_LIST_SIZE:
        raise ValueError(f"index {index} is outside the served status list (0..{STATUS_LIST_SIZE - 1})")
    return {
        "status_list": {
            "idx": index,
            "uri": status_list_uri(issuer_base),
        },
    }


def credential_status_from_claims(claims: dict[str, Any]) -> tuple[int, str] | None:
    status_claim = claims.get("status")
    if not isinstance(status_claim, dict):
        return None
    sl = status_claim.get("status_list")
    if not isinstance(sl, dict):
        return None
    uri = sl.get("uri")
    index_raw = sl.get("idx")
    if not isinstance(uri, str) or not uri.strip():
        return None
    if isinstance(index_raw, int):
        return index_raw, uri.strip()
    if isinstance(index_raw, str) and index_raw.isdigit():
        return int(index_raw, 10), uri.strip()
    return None


def _decode_jwt_segment(token: str, position: int, what: str) -> dict[str, Any]:
    parts = token.split(".")
    if len(parts) != 3:
        raise ValueError("status list response is not a compact JWT")
    segment = parts[position]
    data = json.loads(base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4)).decode("utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"status list JWT {what} is not a JSON object")
    return data


def fetch_status_list_token(uri: str, timeout: float = 10.0) -> str:
    """GET the signed status list as the compact JWT the verifier sees."""
    req = Request(uri, headers={"Accept": STATUS_LIST_MEDIA_TYPE})
    with urlopen(req, timeout=timeout) as resp:
        return resp.read().decode("utf-8").strip()


def fetch_status_list(uri: str, timeout: float = 10.0) -> dict[str, Any]:
    """GET the signed status list and return its (unverified) claims.

    The signature is deliberately not checked here: these are dev tools that
    read the bits, and the only verdict that counts is the DCS backend's, which
    verifies the chain against its configured anchors. What IS checked, by
    assert_served_root_is_a_configured_anchor, is that the backend HOLDS the
    anchor that verdict needs.
    """
    return _decode_jwt_segment(fetch_status_list_token(uri, timeout=timeout), 1, "payload")


def encoded_list_from_claims(claims: dict[str, Any]) -> str:
    status_list = claims.get("status_list")
    if isinstance(status_list, dict):
        encoded = status_list.get("lst")
        if isinstance(encoded, str) and encoded:
            return encoded
    raise ValueError("status list JWT carries no status_list.lst")


def decode_status_bits(encoded_list: str) -> bytes:
    return zlib.decompress(base64.urlsafe_b64decode(encoded_list + "=" * (-len(encoded_list) % 4)))


def bit_is_revoked(encoded_list: str, index: int) -> bool:
    """Whether index is revoked (LSB-first, as the issuer flow writes it)."""
    bitstring = decode_status_bits(encoded_list)
    byte_idx = index // 8
    if byte_idx >= len(bitstring):
        raise ValueError(f"index {index} out of range for bitstring length {len(bitstring)}")
    return bool(bitstring[byte_idx] & (1 << (index % 8)))


# The anchor bundle the dev and BDD backends mount
# (OID4VP_X5C_TRUST_ANCHORS_PATH in backend/.env.dev1, values.dev.yml,
# values.bdd.yml). Committed, unlike a production bundle, because both stacks
# are rebuilt from nothing and the backend reads it before any issuer has
# booted — which is why the issuer is handed the matching root rather than
# generating one (scripts/orce-dev-root-ca.py).
DEV_ANCHORS_PATH = (
    Path(__file__).resolve().parents[2] / "backend" / "config" / "oid4vp" / "x5c-trust-anchors.dev.pem"
)


def _certificate_fingerprint(der: bytes) -> str:
    """SHA-256 over the DER, in the colon-separated form openssl prints."""
    return ":".join(f"{byte:02X}" for byte in hashlib.sha256(der).digest())


def _pem_certificates(pem_text: str) -> list[bytes]:
    begin, end = "-----BEGIN CERTIFICATE-----", "-----END CERTIFICATE-----"
    ders: list[bytes] = []
    rest = pem_text
    while begin in rest:
        _, _, rest = rest.partition(begin)
        body, marker, rest = rest.partition(end)
        if not marker:
            raise ValueError("PEM bundle has a BEGIN CERTIFICATE with no END")
        ders.append(base64.b64decode("".join(body.split())))
    return ders


def served_chain(uri: str, timeout: float = 10.0) -> list[bytes]:
    """The x5c chain the issuer signs this status list with, leaf first (DER).

    RFC 7515 §4.1.6 orders the chain leaf first, so the root is the last entry.
    """
    header = _decode_jwt_segment(fetch_status_list_token(uri, timeout=timeout), 0, "header")
    chain = header.get("x5c")
    if not isinstance(chain, list) or not chain:
        raise RuntimeError(
            f"{uri} serves a status list with no x5c chain, so the backend has no "
            f"certificate to verify it with (ADR-34 §3)"
        )
    return [base64.b64decode(entry) for entry in chain]


def assert_served_root_is_a_configured_anchor(
    uri: str, anchors_path: Path | None = None, timeout: float = 10.0
) -> str:
    """Check by fingerprint that the backend anchors the root this list chains to.

    An anchor is matched on the SHA-256 of its DER and never on its subject.
    Every ORCE issuer mints its own root at runtime and they all carry
    "CN = FACIS Demo Root CA" while holding different keys, so a comparison by
    name reports a match between two unrelated CAs — which is how a correctly
    signed list came to be unverifiable while every log line named the CA it was
    supposed to chain to (ADR-34, Consequences).

    Without this the mismatch first shows up as a refused login with the list
    unread, which reads exactly like a revoked credential.
    """
    path = anchors_path or DEV_ANCHORS_PATH
    if not path.is_file():
        raise RuntimeError(
            f"no x5c trust anchors at {path}; the backend mounts this file and "
            f"refuses every status list without it"
        )
    anchored = {_certificate_fingerprint(der) for der in _pem_certificates(path.read_text(encoding="utf-8"))}
    if not anchored:
        raise RuntimeError(f"{path} holds no certificates")

    root = served_chain(uri, timeout=timeout)[-1]
    served = _certificate_fingerprint(root)
    if served not in anchored:
        raise RuntimeError(
            f"{uri} signs under a root {path} does not anchor.\n"
            f"  served:  {served}\n"
            f"  anchored: {', '.join(sorted(anchored))}\n"
            f"The issuer generated its own root instead of adopting the committed "
            f"one — usually a volume left over from before orce.pkiRootCA.devFixture "
            f"was set. Delete the issuer's PersistentVolumeClaim and reinstall, or "
            f"re-mint the fixture with scripts/orce-dev-root-ca.py."
        )
    return served


def assert_served_leaf_names_the_issuer(uri: str, timeout: float = 10.0) -> str:
    """Check the signing leaf identifies the issuer the token's `iss` names.

    A chain reaching a trusted anchor says an anchor vouched for the leaf; it
    does not say the leaf may speak for this issuer. The backend requires a URI
    SAN equal to `iss`, a DNS SAN equal to its host, or CN == iss
    (backend/internal/auth/oid4vp/sdjwt/keys.go, leafIdentifiesIssuer). The ORCE
    boot path mints a leaf carrying neither, because the public URL is not
    knowable before a request arrives, so a stack whose issuer never re-minted
    for its own base URL serves a list nothing can verify.
    """
    from cryptography import x509  # noqa: PLC0415 — only the preflight pays for this import
    from cryptography.x509.oid import ExtensionOID, NameOID  # noqa: PLC0415

    token = fetch_status_list_token(uri, timeout=timeout)
    issuer = str(_decode_jwt_segment(token, 1, "payload").get("iss") or "")
    header = _decode_jwt_segment(token, 0, "header")
    leaf = x509.load_der_x509_certificate(base64.b64decode(header["x5c"][0]))

    names: list[str] = []
    try:
        san = leaf.extensions.get_extension_for_oid(ExtensionOID.SUBJECT_ALTERNATIVE_NAME).value
        names = [str(v) for v in san.get_values_for_type(x509.UniformResourceIdentifier)]
    except x509.ExtensionNotFound:
        pass
    common_names = [a.value for a in leaf.subject.get_attributes_for_oid(NameOID.COMMON_NAME)]

    if issuer not in names and issuer not in common_names:
        raise RuntimeError(
            f"{uri} is signed by a leaf that does not identify {issuer!r}: its URI SANs "
            f"are {names or 'absent'} and its CN is {common_names or 'absent'}. The "
            f"issuer minted this certificate before it knew the URL it is reached on; "
            f"a request through that URL re-mints it (ensureIssuerCertFor)."
        )
    return issuer


def fetch_list_the_verifier_would_accept(uri: str, timeout: float = 10.0) -> dict[str, Any]:
    """GET the list and refuse it unless it is the one `uri` claims to be.

    The verifier requires the token's `sub` to equal the credential's URI
    (backend/internal/auth/oid4vp/status/handler/ietf_token.go) and the issuer
    builds `sub` from the request's Host and X-Forwarded-Prefix. A mismatch
    means every credential naming this URI is refused with the list unread,
    which at login looks exactly like a revocation.
    """
    claims = fetch_status_list(uri, timeout=timeout)
    subject = str(claims.get("sub") or "")
    if subject != uri:
        raise RuntimeError(
            f"the issuer serves this list as {subject!r} but credentials name {uri!r}; "
            f"set {ISSUER_BASE_ENV} to the URL the DCS backend reaches the issuer on, "
            f"path prefix included"
        )
    return claims


def check_status_list_ready(
    issuer_base: str | None = None,
    wait_seconds: float = 120.0,
    anchors_path: Path | None = None,
) -> str:
    """Preflight the list credentials name; returns a one-line description.

    Nothing has to be created — the issuer serves /status-list/1 and persists
    its bits on its own volume. What can still be wrong is everything the
    backend will hold the list to, and each of those failures reaches a
    developer as the same thing: a login refused with the list unread, which is
    indistinguishable from a revoked credential. So all four are checked here,
    where the message can say which one it was.

      * the URL the issuer believes it is serving (`sub`) is the one credentials
        name;
      * the root its chain ends at is one the backend anchors, compared by
        fingerprint;
      * the leaf identifies the issuer `iss` names;
      * the list is long enough to hold the indices credentials are allocated.

    A Pod reports ready as soon as Node-RED answers, which is before it has
    deployed its flows and therefore before /status-list/1 exists, so the first
    fetches are retried. A wrong URL is not retried: it would still be wrong in
    two minutes, and reporting it at once is the point of this check.
    """
    uri = status_list_uri(issuer_base)

    deadline = time.monotonic() + wait_seconds
    while True:
        try:
            claims = fetch_list_the_verifier_would_accept(uri)
            break
        except RuntimeError:
            raise
        except Exception as exc:
            if time.monotonic() >= deadline:
                raise RuntimeError(
                    f"{uri} did not serve a status list within {wait_seconds:.0f}s: {exc}"
                ) from exc
            time.sleep(2.0)

    anchor = assert_served_root_is_a_configured_anchor(uri, anchors_path=anchors_path)
    assert_served_leaf_names_the_issuer(uri)

    slots = len(decode_status_bits(encoded_list_from_claims(claims))) * 8
    if slots < STATUS_LIST_SIZE:
        raise RuntimeError(
            f"the served list holds {slots} slots but credentials are allocated up to "
            f"{STATUS_LIST_SIZE - 1}; an index past the end is rejected as out of range"
        )

    return (
        f"status list ready: GET {uri} (iss={claims.get('iss')}, {slots} slots, "
        f"anchored root {anchor})"
    )


def served_bit_is_set(index: int, *, status_list_uri: str, timeout: float = 10.0) -> bool:
    """Whether the list the issuer is serving right now marks `index` revoked."""
    claims = fetch_list_the_verifier_would_accept(status_list_uri, timeout=timeout)
    return bit_is_revoked(encoded_list_from_claims(claims), index)


def _admin_post(url: str, timeout: float) -> None:
    # The admin endpoints answer 302 back to the /admin page. urllib would
    # follow that as a GET and report the HTML page's 200, hiding a failure.
    class _NoRedirect(HTTPRedirectHandler):
        def redirect_request(self, *_args, **_kwargs):
            return None

    req = Request(url, data=b"", method="POST")
    try:
        with build_opener(_NoRedirect).open(req, timeout=timeout) as resp:
            code = resp.status
    except HTTPError as exc:
        if exc.code in (301, 302, 303, 307, 308):
            return
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"{url} failed HTTP {exc.code}: {detail}") from exc
    if code >= 400:
        raise RuntimeError(f"{url} failed HTTP {code}")


def revoke_status_index(index: int, *, status_list_uri: str, timeout: float = 10.0) -> None:
    """Flip a credential's bit through the issuer's admin endpoint."""
    if index < 0:
        raise ValueError(f"index must be non-negative, got {index}")
    _admin_post(f"{admin_base(status_list_uri)}/admin/credentials/{index}/revoke", timeout)


def unrevoke_status_index(index: int, *, status_list_uri: str, timeout: float = 10.0) -> None:
    if index < 0:
        raise ValueError(f"index must be non-negative, got {index}")
    _admin_post(f"{admin_base(status_list_uri)}/admin/credentials/{index}/unrevoke", timeout)


def revoke_and_prove(index: int, *, status_list_uri: str, timeout: float = 10.0) -> str:
    """Revoke `index` and prove from the served list that a refusal which
    follows can only be the bit. Returns a one-line description.

    A verifier refuses a presentation both when the bit is set and when it
    cannot use the list at all, and the two are one error string apart
    (backend/internal/auth/oid4vp/statuslist_verify.go). A scenario that just
    calls /admin and asserts a rejection therefore passes unchanged with the
    issuer switched off, proving nothing about revocation. Two successful
    fetches here show the list is retrievable and correctly identified; a bit
    that reads 0 before the admin call and 1 after shows this call is what set
    it.

    The bits are persisted on the issuer's volume, so an index a previous run
    revoked is still revoked — hence the clear first, which also leaves the
    index reusable by the next run without a scenario cleanup step.
    """
    unrevoke_status_index(index, status_list_uri=status_list_uri, timeout=timeout)
    if served_bit_is_set(index, status_list_uri=status_list_uri, timeout=timeout):
        raise RuntimeError(
            f"{status_list_uri} idx={index} is still revoked after /admin/credentials/"
            f"{index}/unrevoke; the issuer is not applying admin calls, so a revocation "
            f"proves nothing"
        )

    revoke_status_index(index, status_list_uri=status_list_uri, timeout=timeout)
    if not served_bit_is_set(index, status_list_uri=status_list_uri, timeout=timeout):
        raise RuntimeError(
            f"{status_list_uri} idx={index} still reads active after /admin/credentials/"
            f"{index}/revoke"
        )

    return f"revoked: {status_list_uri} idx={index} (was active before the admin call)"


def admin_base(status_list_uri: str) -> str:
    """Issuer base URL behind a status list URI it serves."""
    marker = "/status-list/"
    uri = status_list_uri.strip().rstrip("/")
    if marker not in uri:
        raise ValueError(f"not an issuer status list URI: {status_list_uri}")
    return uri[: uri.rindex(marker)]
