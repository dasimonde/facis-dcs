"""Maintain the x5c trust-anchor PEM bundle (OID4VP_X5C_TRUST_ANCHORS_PATH).

The bundle holds one anchor per issuer whose certificate chains a deployment has
to verify, and more than one script mints one of those anchors. Each writer
therefore replaces only the anchor it minted last time and keeps the rest: a
writer that rewrote the whole file would drop every other issuer's anchor, and
that surfaces much later as one issuer's credentials being refused for want of a
root.

Which anchor is a writer's own is decided by SHA-256 fingerprint, never by
subject. Every ORCE issuer mints its own root at runtime and they ALL carry the
subject "CN = FACIS Demo Root CA" while holding different keys, so a bundle
maintained by name deletes one issuer's anchor the moment another writes, and
every log line afterwards names the same CA (ADR-34, Consequences).
"""

from __future__ import annotations

from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization


def fingerprint(certificate: x509.Certificate) -> str:
    digest = certificate.fingerprint(hashes.SHA256())
    return ":".join(f"{byte:02X}" for byte in digest)


def load_certificate(path: Path) -> x509.Certificate | None:
    """The certificate at path, or None if it is absent or empty.

    Callers use it to hand upsert_anchor the anchor they published last time,
    which is the only thing identifying which entry in the bundle is theirs.
    """
    if not path.is_file():
        return None
    data = path.read_bytes()
    if not data.strip():
        return None
    return x509.load_pem_x509_certificate(data)


def upsert_anchor(
    bundle_path: Path,
    certificate: x509.Certificate,
    *,
    replacing: x509.Certificate | None = None,
) -> list[x509.Certificate]:
    """Write certificate into bundle_path, dropping the anchor it supersedes.

    `replacing` is the caller's PREVIOUS anchor — what it wrote the last time it
    ran. Without it the certificate is added alongside whatever is already
    there, which is right for a new issuer and wrong for a rotated one, so a
    caller that keeps its certificate on disk reads it with load_certificate()
    before overwriting it.

    Re-running a writer that changed nothing is a no-op: an anchor whose
    fingerprint the bundle already holds is not duplicated.
    """
    existing: list[x509.Certificate] = []
    if bundle_path.is_file():
        data = bundle_path.read_bytes()
        if data.strip():
            existing = x509.load_pem_x509_certificates(data)

    superseded = {fingerprint(certificate)}
    if replacing is not None:
        superseded.add(fingerprint(replacing))

    anchors = [cert for cert in existing if fingerprint(cert) not in superseded]
    anchors.append(certificate)

    bundle_path.parent.mkdir(parents=True, exist_ok=True)
    bundle_path.write_bytes(
        b"".join(cert.public_bytes(serialization.Encoding.PEM) for cert in anchors)
    )
    return anchors


def print_bundle(bundle_path: Path, anchors: list[x509.Certificate]) -> None:
    print(f"{bundle_path}: {len(anchors)} anchor(s)")
    for cert in anchors:
        print(f"   {fingerprint(cert)}  {cert.subject.rfc4514_string()}")
