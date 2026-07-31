"""Status list helpers for testWallet issuance and dev scripts."""

from __future__ import annotations

import asyncio
import base64
import gzip
import hashlib
import json
import struct
import uuid
import zlib
from typing import Any
from urllib.error import HTTPError
from urllib.request import Request, urlopen

LIST_SIZE = 131072
DEFAULT_LIST_NUMBER = 1
DEFAULT_TENANT = "default"
BDD_CREDENTIAL_TENANT = "credential"
DEFAULT_SERVICE_BASE = "http://localhost:30821"
DEFAULT_NATS_URL = "nats://localhost:30422"
DEFAULT_CREATION_TOPIC = "status"


def status_list_index(seed: str) -> int:
    """Deterministic bit index (same algorithm as C2PA StatusListIndex)."""
    digest = hashlib.sha256(seed.encode("utf-8")).digest()
    return struct.unpack(">I", digest[:4])[0] % LIST_SIZE


def status_list_index_seed(*, sub: str, organization: str, roles: list[str]) -> str:
    """Stable seed so each credential (sub + org + roles) maps to its own index."""
    role_part = ",".join(roles)
    return f"{sub}\x1e{organization}\x1e{role_part}"


def status_list_uri(
    service_base: str = DEFAULT_SERVICE_BASE,
    list_number: int = DEFAULT_LIST_NUMBER,
    tenant: str = DEFAULT_TENANT,
) -> str:
    base = service_base.strip().rstrip("/")
    if not base.startswith("http://") and not base.startswith("https://"):
        base = f"http://{base}"
    return f"{base}/v1/tenants/{tenant}/status/{list_number}"


def build_credential_status(
    *,
    sub: str,
    organization: str,
    roles: list[str],
    service_base: str = DEFAULT_SERVICE_BASE,
    list_number: int = DEFAULT_LIST_NUMBER,
    tenant: str = DEFAULT_TENANT,
) -> dict[str, Any]:
    """Return IETF status.status_list reference for issuer JWT visible claims."""
    uri = status_list_uri(service_base, list_number, tenant)
    idx = status_list_index(status_list_index_seed(sub=sub, organization=organization, roles=roles))
    return {
        "status_list": {
            "idx": idx,
            "uri": uri,
        },
    }


def credential_status_from_claims(claims: dict[str, Any]) -> tuple[int, str] | None:
    status_claim = claims.get("status")
    if isinstance(status_claim, dict):
        sl = status_claim.get("status_list")
        if isinstance(sl, dict):
            uri = sl.get("uri")
            index_raw = sl.get("idx")
            if isinstance(uri, str) and uri.strip():
                if isinstance(index_raw, int):
                    return index_raw, uri.strip()
                if isinstance(index_raw, str) and index_raw.isdigit():
                    return int(index_raw, 10), uri.strip()
    cs = claims.get("credentialStatus")
    if not isinstance(cs, dict):
        return None
    uri = cs.get("statusListCredential")
    index_raw = cs.get("statusListIndex")
    if not isinstance(uri, str) or not uri.strip():
        return None
    if isinstance(index_raw, int):
        return index_raw, uri.strip()
    if isinstance(index_raw, str) and index_raw.isdigit():
        return int(index_raw, 10), uri.strip()
    return None


def fetch_status_list_payload(uri: str, timeout: float = 10.0) -> dict[str, Any]:
    req = Request(
        uri,
        headers={
            "Accept": "application/json",
            "Content-Type": "application/json",
        },
    )
    with urlopen(req, timeout=timeout) as resp:
        body = resp.read()
    data = json.loads(body.decode("utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"status list response is not a JSON object: {uri}")
    return data


def encoded_list_from_payload(payload: dict[str, Any]) -> str:
    subject = payload.get("credentialSubject")
    if isinstance(subject, dict):
        encoded = subject.get("encodedList")
        if isinstance(encoded, str) and encoded:
            return encoded
    encoded = payload.get("list")
    if isinstance(encoded, str) and encoded:
        return encoded
    status_list = payload.get("status_list")
    if isinstance(status_list, dict):
        encoded = status_list.get("lst")
        if isinstance(encoded, str) and encoded:
            return encoded
    raise ValueError("status list response has no encodedList/list/lst field")


def _decompress_bitstring(encoded_list: str) -> bytes:
    raw = base64.urlsafe_b64decode(encoded_list + "=" * (-len(encoded_list) % 4))
    if raw[:2] == b"\x1f\x8b":
        return gzip.decompress(raw)
    return zlib.decompress(raw)


def bit_is_revoked(encoded_list: str, index: int) -> bool:
    """Return whether index is revoked (LSB: matches statuslist-service)."""
    bitstring = _decompress_bitstring(encoded_list)
    byte_idx = index // 8
    if byte_idx >= len(bitstring):
        raise ValueError(f"index {index} out of range for bitstring length {len(bitstring)}")
    return bool(bitstring[byte_idx] & (1 << (index % 8)))


def revoke_status_index(
    index: int,
    *,
    service_base: str = DEFAULT_SERVICE_BASE,
    tenant: str = DEFAULT_TENANT,
    list_number: int = DEFAULT_LIST_NUMBER,
    timeout: float = 10.0,
) -> dict[str, Any]:
    """POST /v1/tenants/{tenant}/status/{list}/revoke/{index}."""
    if index < 0:
        raise ValueError(f"index must be non-negative, got {index}")
    base = service_base.strip().rstrip("/")
    if not base.startswith("http://") and not base.startswith("https://"):
        base = f"http://{base}"
    url = f"{base}/v1/tenants/{tenant}/status/{list_number}/revoke/{index}"
    req = Request(
        url,
        data=b"",
        method="POST",
        headers={
            "Accept": "application/json",
            "Content-Type": "application/json",
        },
    )
    try:
        with urlopen(req, timeout=timeout) as resp:
            body = resp.read()
    except HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"revoke failed HTTP {exc.code}: {detail}") from exc
    if not body:
        return {}
    data = json.loads(body.decode("utf-8"))
    if not isinstance(data, dict):
        raise ValueError(f"revoke response is not a JSON object: {body!r}")
    return data

def _nats_create_event(*, tenant_id: str, request_id: str, origin: str) -> dict[str, Any]:
    return {
        "specversion": "1.0",
        "id": str(uuid.uuid4()),
        "source": "testWallet/bootstrap",
        "type": "create",
        "datacontenttype": "application/json",
        "data": {
            "tenant_id": tenant_id,
            "request_id": request_id,
            "origin": origin.rstrip("/"),
        },
    }


async def _nats_ensure_list_async(
    *,
    nats_url: str = DEFAULT_NATS_URL,
    topic: str = DEFAULT_CREATION_TOPIC,
    tenant_id: str = DEFAULT_TENANT,
    service_base: str = DEFAULT_SERVICE_BASE,
    list_number: int = DEFAULT_LIST_NUMBER,
    timeout: float = 10.0,
) -> None:
    uri = status_list_uri(service_base, list_number, tenant_id)
    try:
        fetch_status_list_payload(uri, timeout=timeout)
        return
    except HTTPError as exc:
        if exc.code != 400:
            raise

    try:
        import nats
    except ImportError as exc:
        raise RuntimeError(
            "status list not initialized and nats-py is not installed "
            "(pip install -r testWallet/requirements.txt)"
        ) from exc

    origin = f"{service_base.rstrip('/')}/v1/tenants/{tenant_id}"
    event = _nats_create_event(
        tenant_id=tenant_id,
        request_id=f"bootstrap-{uuid.uuid4()}",
        origin=origin,
    )
    nc = await nats.connect(nats_url)
    try:
        reply = await nc.request(topic, json.dumps(event).encode("utf-8"), timeout=timeout)
    finally:
        await nc.close()

    if reply is None:
        raise RuntimeError(f"NATS request on {topic} timed out ({nats_url})")

    envelope = json.loads(reply.data.decode("utf-8"))
    data = envelope.get("data")
    if not isinstance(data, dict):
        raise RuntimeError(f"unexpected NATS reply: {reply.data!r}")
    err = data.get("error")
    if err:
        raise RuntimeError(f"statuslist NATS create failed: {err}")

    fetch_status_list_payload(uri, timeout=timeout)


def ensure_status_list_initialized(**kwargs: Any) -> None:
    """Create tenant list via NATS when GET /status/1 returns 400 (empty DB)."""
    asyncio.run(_nats_ensure_list_async(**kwargs))
