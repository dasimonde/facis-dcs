"""BDD step definitions for PDF export and verification endpoints."""

import json
import time

from behave import given, then, when

from steps.support.services.contract_service import ContractService
from steps.support.services.pdf_service import PDFService
from steps.support.tamper_seam import swap_contract_pdf_cid


# ---------------------------------------------------------------------------
# Given helpers
# ---------------------------------------------------------------------------


@given('contract "{name}" exists in "Under Review" state')
def step_given_contract_under_review_state(context, name):
    ContractService._create_contract_in_draft(context, name)
    ContractService._prepare_contract_under_review(context, name)


@given('contract "{name}" has an exported PDF')
def step_given_contract_has_exported_pdf(context, name):
    did, _ = ContractService._contract_data(context, name)
    resp = PDFService.export_contract_pdf(context, did)
    assert resp.status_code == 200, (
        f"Failed to export PDF for contract '{name}': {resp.status_code} — {resp.text}"
    )
    if not hasattr(context, "pdf_bytes"):
        context.pdf_bytes = {}
    context.pdf_bytes[name] = resp.content


@given('contract "{name}" has an exported PDF with a tampered base layer')
def step_given_contract_has_tampered_pdf(context, name):
    # Export the real, untampered PDF first, flip a byte in its base layer
    # (before %%EOF), then inject the tampered bytes as the CONTRACT'S
    # STORED PDF via the IPFS CID-swap seam (steps/support/tamper_seam.py).
    #
    # Mutating `context.pdf_bytes` alone (the previous implementation) never
    # reached the server: GET /pdf/verify/contract/{did} always re-fetches
    # its OWN cached copy from IPFS by the CID in contracts.pdf_ipfs_cid —
    # never the bytes returned to this test process. See tamper_seam.py's
    # module docstring for the full seam rationale.
    did, _ = ContractService._contract_data(context, name)
    resp = PDFService.export_contract_pdf(context, did)
    assert resp.status_code == 200, (
        f"Failed to export PDF for setup: {resp.status_code} — {resp.text}"
    )
    raw = bytearray(resp.content)
    eof_pos = raw.find(b"%%EOF")
    assert eof_pos > 10, "exported PDF has no %%EOF marker to tamper before"
    # Flip a byte well inside the base layer
    raw[eof_pos - 5] ^= 0xFF
    tampered = bytes(raw)

    swap_contract_pdf_cid(context, did, tampered)

    if not hasattr(context, "pdf_bytes"):
        context.pdf_bytes = {}
    context.pdf_bytes[name] = tampered
    context.tampered_contract_did = did


@given('contract "{name}" has been exported in "Draft" state')
def step_given_contract_exported_in_draft(context, name):
    # Self-contained precondition (unlike 'has an exported PDF', which
    # assumes an earlier 'is in "Draft" status' step already created the
    # contract in this scenario) — create it first.
    ContractService._create_contract_in_draft(context, name)
    step_given_contract_has_exported_pdf(context, name)


# ---------------------------------------------------------------------------
# When steps
# ---------------------------------------------------------------------------


@when('I export contract "{name}" as PDF')
def step_when_export_contract_pdf(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = PDFService.export_contract_pdf(context, did)
    if context.requests_response.status_code == 200:
        if not hasattr(context, "pdf_bytes"):
            context.pdf_bytes = {}
        context.pdf_bytes[name] = context.requests_response.content


@when('I verify the MR/HR hash consistency for contract "{name}"')
def step_when_verify_contract_pdf(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = PDFService.verify_contract_pdf(context, did)
    if context.requests_response.status_code == 200:
        context.verify_result = context.requests_response.json()


@when('contract "{name}" transitions to "{state}" state')
def step_when_contract_transitions(context, name, state):
    context.last_transitioned_contract = name
    # Drive a REAL lifecycle-state transition through the actual workflow
    # API (not just a GET/retrieve stub) so the contract's `state` column
    # genuinely advances, and the C2PA chain-linkage machinery this
    # scenario exercises has a second, real event to react to.
    if state == "Under Review":
        ContractService._prepare_contract_under_review(context, name)
    else:
        raise NotImplementedError(
            f"contract transition to state {state!r} is not wired by this "
            "test harness step — only 'Under Review' is implemented "
            "(the C2PA chain-linkage scenario's only user of this step)"
        )

    # The state transition above triggers an ASYNC NATS subscriber
    # (backend/internal/pdfgeneration/event/subscriber.go) that appends a
    # second C2PA lifecycle assertion to the cached PDF in the background.
    # Rather than assume a fixed race-prone delay (or rely on the very next
    # "I export contract ... as PDF" step to deterministically observe an
    # assertion that may not have landed yet), poll the export endpoint
    # here until the second assertion is actually present. This is a
    # read-only poll — export_contract_pdf is idempotent once the cached
    # PDF state matches the current contract state/payload (see
    # backend/internal/pdfgeneration/query/exportcontract.go's cache-hit
    # branch), so repeated calls are safe.
    did, _ = ContractService._contract_data(context, name)
    deadline = time.monotonic() + 90
    last_count = 0
    last_status = None
    while time.monotonic() < deadline:
        resp = PDFService.export_contract_pdf(context, did)
        last_status = resp.status_code
        if resp.status_code == 200:
            last_count = len(_lifecycle_regions(resp.content))
            if last_count >= 2:
                context.requests_response = resp
                if not hasattr(context, "pdf_bytes"):
                    context.pdf_bytes = {}
                context.pdf_bytes[name] = resp.content
                return
        time.sleep(1)
    raise AssertionError(
        f"Timed out waiting for the async NATS subscriber to append the second "
        f"C2PA lifecycle assertion for contract {name!r} after transitioning to "
        f"{state!r} (last export status={last_status}, last assertion count="
        f"{last_count})"
    )


# ---------------------------------------------------------------------------
# Then assertions — PDF content
# ---------------------------------------------------------------------------


@then("the response is a valid PDF document")
def step_then_valid_pdf(context):
    assert context.requests_response.status_code == 200, (
        f"Expected 200 from PDF export, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    assert context.requests_response.content[:4] == b"%PDF", (
        "Response body does not start with PDF magic bytes (%PDF)"
    )


@then('the PDF contains an embedded JSON-LD attachment named "contract.jsonld"')
def step_then_pdf_has_jsonld_attachment(context):
    pdf_bytes = context.requests_response.content
    assert b"contract.jsonld" in pdf_bytes or _utf16be(b"contract.jsonld") in pdf_bytes, (
        "PDF does not reference an embedded file named 'contract.jsonld'"
    )


@then("the embedded JSON-LD matches the contract source")
def step_then_embedded_jsonld_matches(context):
    # The attachment is present; its content integrity is verified by the /verify endpoint.
    # We validate that the verify endpoint confirms a match rather than re-parsing the PDF here.
    name = "Service Agreement"
    did, _ = ContractService._contract_data(context, name)
    verify_resp = PDFService.verify_contract_pdf(context, did)
    assert verify_resp.status_code == 200, (
        f"Verify endpoint failed: {verify_resp.status_code} — {verify_resp.text}"
    )
    result = verify_resp.json()
    assert result.get("match") is True, (
        f"Embedded JSON-LD does not match contract source: {result}"
    )


# ---------------------------------------------------------------------------
# Then assertions — verification result
# ---------------------------------------------------------------------------


@then("the verification result shows match is true")
def step_then_verify_match_true(context):
    result = getattr(context, "verify_result", None)
    if result is None and context.requests_response.status_code == 200:
        result = context.requests_response.json()
    assert result is not None, (
        f"No verification result available (status="
        f"{context.requests_response.status_code}, body="
        f"{context.requests_response.text[:2000]!r})"
    )
    assert result.get("match") is True, (
        f"Expected match=true in verify result, got: {result}"
    )


@then("the verification result shows match is false")
def step_then_verify_match_false(context):
    result = getattr(context, "verify_result", None)
    if result is None and context.requests_response.status_code == 200:
        result = context.requests_response.json()
    assert result is not None, (
        f"No verification result available (status="
        f"{context.requests_response.status_code}, body="
        f"{context.requests_response.text[:2000]!r})"
    )
    assert result.get("match") is False, (
        f"Expected match=false in verify result, got: {result}"
    )


@then("the response includes jsonld_hash and base_pdf_hash")
def step_then_verify_includes_hashes(context):
    result = getattr(context, "verify_result", None)
    if result is None and context.requests_response.status_code == 200:
        result = context.requests_response.json()
    assert result is not None, (
        f"No verification result available (status="
        f"{context.requests_response.status_code}, body="
        f"{context.requests_response.text[:2000]!r})"
    )
    assert "jsonld_hash" in result, f"Missing 'jsonld_hash' in verify result: {result}"
    assert "base_pdf_hash" in result, f"Missing 'base_pdf_hash' in verify result: {result}"


# ---------------------------------------------------------------------------
# Then assertions — C2PA manifest
# ---------------------------------------------------------------------------


def _cbor_text(text: str) -> bytes:
    """CBOR encoding of a short (<24 bytes) text string: 0x60+len prefix.

    Mirrors pdf-core/compiler/compiler_c2pa.go cborText for the lifecycle
    assertion's keys and values, which are all shorter than 24 bytes.
    """
    raw = text.encode()
    assert len(raw) < 24, f"_cbor_text only handles short strings, got {len(raw)} bytes"
    return bytes([0x60 + len(raw)]) + raw


def _lifecycle_regions(pdf_bytes: bytes) -> list:
    """Return the byte regions of each dcs.lifecycle JUMBF assertion box.

    pdf-core embeds the C2PA manifest store as an uncompressed
    content_credential.c2pa embedded file; the lifecycle assertion is a
    small all-text CBOR map inside a JUMBF box labelled "dcs.lifecycle"
    (renderLifecycleAssertionCBOR, pdf-core/compiler/compiler_c2pa.go).
    """
    regions = []
    start = 0
    while True:
        begin = pdf_bytes.find(b"dcs.lifecycle", start)
        if begin == -1:
            return regions
        # Skip the claim box's hashed-URI reference
        # ("self#jumbf=c2pa.assertions/dcs.lifecycle") — only count the
        # JUMBF box label itself.
        if begin == 0 or pdf_bytes[begin - 1] != ord("/"):
            regions.append(pdf_bytes[begin:begin + 512])
        start = begin + 1


@then("the PDF contains a C2PA manifest")
def step_then_pdf_has_c2pa_manifest(context):
    pdf_bytes = context.requests_response.content
    assert b"content_credential.c2pa" in pdf_bytes and b"c2pa.assertions" in pdf_bytes, (
        "PDF does not contain a C2PA manifest (no content_credential.c2pa "
        "embedded file with a c2pa.assertions JUMBF box)"
    )


@then('the manifest lifecycle assertion includes field "{field}"')
def step_then_manifest_has_field(context, field):
    pdf_bytes = context.requests_response.content
    regions = _lifecycle_regions(pdf_bytes)
    assert regions, "No dcs.lifecycle assertion box found in PDF"
    assert any(_cbor_text(field) in region for region in regions), (
        f"CBOR key '{field}' not found in any dcs.lifecycle assertion"
    )


@then('the manifest contains a lifecycle assertion with field "{field}" equal to "{value}"')
def step_then_manifest_field_equals(context, field, value):
    pdf_bytes = context.requests_response.content
    regions = _lifecycle_regions(pdf_bytes)
    assert regions, "No dcs.lifecycle assertion box found in PDF"
    # In the lifecycle CBOR map the value immediately follows its key.
    needle = _cbor_text(field) + _cbor_text(value)
    assert any(needle in region for region in regions), (
        f"No dcs.lifecycle assertion has field '{field}' equal to '{value}'"
    )


@then("the PDF contains two C2PA assertions")
def step_then_pdf_has_two_c2pa_assertions(context):
    pdf_bytes = context.requests_response.content
    count = len(_lifecycle_regions(pdf_bytes))
    assert count >= 2, (
        f"Expected at least 2 dcs.lifecycle assertion boxes, found {count}"
    )


def _parse_jumbf_boxes(data: bytes):
    """Read sibling BMFF/JUMBF boxes from `data`: 4-byte big-endian size +
    4-byte type + payload. Mirrors backend/internal/pdfgeneration/manifest/
    chain.go's parseBoxes exactly (same box framing pdf-core emits), so the
    raw per-manifest byte ranges this returns line up with what the backend
    hashes for prev_manifest_hash (see below).
    """
    boxes = []
    pos = 0
    while pos + 8 <= len(data):
        size = int.from_bytes(data[pos : pos + 4], "big")
        assert size >= 8 and pos + size <= len(data), (
            f"invalid BMFF box framing at offset {pos} (size={size}, len={len(data)})"
        )
        boxes.append({"type": data[pos + 4 : pos + 8], "raw": data[pos : pos + size]})
        pos += size
    return boxes


def _jumbf_children(superbox_payload: bytes):
    """Return the label and content boxes of a JUMBF superbox (payload =
    everything after the box's own 8-byte size+type header): the first
    child is a 'jumd' description box carrying the label, the rest are the
    superbox's real content boxes. Mirrors chain.go's jumbfChildren.
    """
    children = _parse_jumbf_boxes(superbox_payload)
    assert children and children[0]["type"] == b"jumd", "JUMBF description box (jumd) missing"
    jumd_payload = children[0]["raw"][8:]
    # jumd payload: 16-byte UUID + 1 toggle byte + null-terminated label
    rest = jumd_payload[17:]
    terminator = rest.find(b"\x00")
    assert terminator != -1, "JUMBF label terminator missing"
    label = rest[:terminator].decode("utf-8", errors="replace")
    return label, children[1:]


def _c2pa_manifest_boxes(store_bytes: bytes):
    """Return the raw JUMBF superbox bytes (including their own 8-byte
    size+type header) for each manifest in a C2PA manifest store, in
    chain order — the same top-level 'jumb' children
    pdf-core/manifest/chain.go's ParseChain walks.
    """
    root_boxes = _parse_jumbf_boxes(store_bytes)
    assert root_boxes and root_boxes[0]["type"] == b"jumb", (
        "C2PA manifest store root JUMBF box not found"
    )
    _, manifest_boxes = _jumbf_children(root_boxes[0]["raw"][8:])
    return [b["raw"] for b in manifest_boxes if b["type"] == b"jumb"]


@then("the second assertion's prev_manifest_hash matches the first assertion's hash")
def step_then_c2pa_chain_linkage(context):
    import hashlib

    import requests as _requests  # noqa: PLC0415 — local, mirrors dcs_c2pa_manifest_steps.py

    from steps.support.api_client import c2pa_manifest_url

    name = getattr(context, "last_transitioned_contract", "Service Agreement")
    did, _ = ContractService._contract_data(context, name)

    # GET /c2pa/manifest/{did} is public (no JWT) — same style as
    # dcs_c2pa_manifest_steps.py's step_when_request_public_manifest.
    store_resp = _requests.get(
        c2pa_manifest_url(context, did), timeout=context.http_timeout_seconds
    )
    assert store_resp.status_code == 200, (
        f"GET C2PA manifest store failed for contract '{name}': "
        f"{store_resp.status_code} {store_resp.text}"
    )
    manifest_boxes = _c2pa_manifest_boxes(store_resp.content)
    assert len(manifest_boxes) >= 2, (
        f"Expected at least 2 manifests in the C2PA manifest store, found "
        f"{len(manifest_boxes)}"
    )
    # backend/pdf-core computes prev_manifest_hash as
    # hex(sha256(originalManifestBox[8:])) — the hash of the FIRST manifest's
    # JUMBF superbox payload, excluding that box's own 8-byte size+type
    # header (pdf-core/compiler/compiler_c2pa.go:293).
    expected_prev_hash = hashlib.sha256(manifest_boxes[0][8:]).hexdigest()

    history_resp = _requests.get(
        c2pa_manifest_url(context, did),
        params={"history": "true"},
        timeout=context.http_timeout_seconds,
    )
    assert history_resp.status_code == 200, (
        f"GET C2PA manifest history failed for contract '{name}': "
        f"{history_resp.status_code} {history_resp.text}"
    )
    chain = history_resp.json()
    assert isinstance(chain, list) and len(chain) >= 2, (
        f"Expected at least 2 manifest chain entries, got: {chain}"
    )
    second_lifecycle = chain[1].get("lifecycle") or {}
    actual_prev_hash = second_lifecycle.get("prev_manifest_hash", "")
    assert actual_prev_hash == expected_prev_hash, (
        f"Second assertion's prev_manifest_hash ({actual_prev_hash!r}) does not match "
        f"the first manifest's own SHA-256 ({expected_prev_hash!r}) — chain: {chain}"
    )


# ---------------------------------------------------------------------------
# Utilities
# ---------------------------------------------------------------------------


def _utf16be(ascii_bytes: bytes) -> bytes:
    """Return UTF-16BE encoding (with BOM) of an ASCII byte string."""
    result = bytearray([0xFE, 0xFF])
    for b in ascii_bytes:
        result.extend([0x00, b])
    return bytes(result)
