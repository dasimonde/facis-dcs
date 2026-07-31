"""BDD steps for encryption-at-rest and key-shredding erasure (ADR-28,
features/26_encryption_at_rest/).

Covers, against the running stack:
  - opaque-at-rest: the bytes a contract's PDF CID resolves to on the IPFS
    node are ciphertext (DCS-NFR-SEC-14), while the authorized export still
    returns the intact PDF (GET /pdf/export/contract/{did});
  - the served did.json's keyAgreement verification method (CEK wrapping key);
  - the peer CEK handshake: after a ship, both instances serve the
    byte-identical contract PDF from their own encrypted stores;
  - key shredding: DELETE /archive/delete destroys the contract CEKs on both
    instances (GET /archive/erasure-status, KEY_SHREDDED audit events, erased
    audit bodies) while checkpoint inclusion proofs keep verifying
    (GET /pac/audit/checkpoint/proof/{entry_cid}, DCS-NFR-SEC-13 +
    DCS-NFR-COMP-03 without weakening ADR-16 tamper evidence).

Identity model: every identity minted here carries the scenario's own
dedicated organization (AuthService.get_headers_for_roles(...,
organization=...)) — key shredding is run-durable state, so these scenarios
must not act through the suite-shared identities.

IPFS access reuses the BDD_IPFS_EXEC exec seam
(steps/support/tamper_seam.py) — the suite's only IPFS-level access path.
"""

import hashlib
import json
import time

import requests as _requests
from behave import given, then, when

from steps.support.api_client import (
    archive_delete_url,
    archive_erasure_status_url,
    archive_retrieve_url,
    contract_export_pdf_url,
    delete_with_params,
    did_document_url,
    get_with_headers,
    pac_audit_url,
    pac_checkpoint_proof_url,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.tamper_seam import ipfs_cat_bytes
from steps.peer_trust.dcs_peer_trust_steps import _as_instance

AUDIT_JUSTIFICATION = "BDD encryption-at-rest audit review"


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _org_headers(context, roles, api_base=None):
    """Role headers under the scenario's dedicated organization. Hard-fails
    if the scenario forgot its dedicated-organization Given — shredding
    through a suite-shared identity is exactly the poisoning the identity
    model forbids."""
    org = getattr(context, "encryption_org", None)
    assert org, (
        "This scenario must first declare its dedicated organization "
        '(Given the dedicated organization "..." is used for this '
        "scenario's identities) before minting identities."
    )
    return AuthService.get_headers_for_roles(
        roles, organization=org, api_base=api_base or context.base_url
    )


def _base_for_label(context, label):
    label = label.strip().upper()
    assert label in ("A", "B"), f"unknown instance label '{label}' (expected A or B)"
    base = getattr(context, "base_url_a" if label == "A" else "base_url_b", None)
    assert base, (
        f"instance {label} base URL is not set — the scenario must start with "
        '"Given instance A and instance B are both running and trust each other"'
    )
    return base


def _erasure_status(context, base_url, did, extra_ok=()):
    """GET /archive/erasure-status?did= as the dedicated organization's
    Archive Manager on the given instance. Returns the parsed body."""
    headers = _org_headers(context, ["Archive Manager"], api_base=base_url)
    resp = _requests.get(
        f"{base_url}/archive/erasure-status",
        params={"did": did},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200 or resp.status_code in extra_ok, (
        f"GET /archive/erasure-status?did={did} on {base_url} failed: "
        f"{resp.status_code} {resp.text}"
    )
    return resp.json() if resp.status_code == 200 else None


def _pac_audit_entries(context, base_url, scope, did):
    """POST /pac/audit for one scope, filtered client-side to entries of the
    given contract DID (the audit API has no event_type filter by design)."""
    headers = _org_headers(context, ["Auditor"], api_base=base_url)
    resp = _requests.post(
        f"{base_url}/pac/audit",
        json={"scope": scope, "did": did, "justification": AUDIT_JUSTIFICATION},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, (
        f"POST /pac/audit scope={scope} did={did} on {base_url} failed: "
        f"{resp.status_code} {resp.text}"
    )
    body = resp.json()
    assert isinstance(body, list), f"expected a list of audit scopes, got: {body}"
    return [
        entry
        for scope_result in body
        if isinstance(scope_result, dict)
        for entry in (scope_result.get("audit_trail") or [])
        if isinstance(entry, dict) and entry.get("did") == did
    ]


def _merkle_node(left: bytes, right: bytes) -> bytes:
    # RFC 6962 interior node: SHA-256 over 0x01 || left || right
    # (backend/internal/base/merkle.go merkleNode).
    return hashlib.sha256(b"\x01" + left + right).digest()


def _verify_merkle_inclusion(leaf_hash_hex, siblings_hex, index, leaf_count, root_hex):
    """Port of backend/internal/base/merkle.go VerifyMerkleInclusion: recompute
    the root from the blinded leaf hash and the bottom-up sibling path, with
    RFC 6962 odd-node promotion."""
    current = bytes.fromhex(leaf_hash_hex)
    consumed = 0
    size = leaf_count
    while size > 1:
        if index != size - 1 or size % 2 == 0:
            if consumed >= len(siblings_hex):
                return False
            sibling = bytes.fromhex(siblings_hex[consumed])
            consumed += 1
            if index % 2 == 0:
                current = _merkle_node(current, sibling)
            else:
                current = _merkle_node(sibling, current)
        index //= 2
        size = (size + 1) // 2
    return consumed == len(siblings_hex) and current == bytes.fromhex(root_hex)


def _export_contract_pdf(context, base_url, did):
    """GET /pdf/export/contract/{did} as the dedicated organization's Contract
    Manager, polling through the async regenerator's transient not-ready
    responses. Returns the final requests response (which may be the defined
    erasure error — the caller asserts)."""
    headers = _org_headers(context, ["Contract Manager"], api_base=base_url)
    deadline = time.monotonic() + 120
    resp = None
    while time.monotonic() < deadline:
        resp = _requests.get(
            f"{base_url}/pdf/export/contract/{did}",
            headers=headers,
            timeout=context.http_timeout_seconds,
        )
        if resp.status_code == 200:
            return resp
        if resp.status_code == 404:
            # Post-shred exports answer 404 with the defined ShreddedError
            # message — a terminal state, not a transient one.
            if "shredded" in resp.text or "erased" in resp.text:
                return resp
        time.sleep(2)
    return resp


# ---------------------------------------------------------------------------
# Dedicated organization (identity model)
# ---------------------------------------------------------------------------


@given('the dedicated organization "{org}" is used for this scenario\'s identities')
def step_given_dedicated_org(context, org):
    context.encryption_org = org


# ---------------------------------------------------------------------------
# Opaque at rest (DCS-NFR-SEC-14)
# ---------------------------------------------------------------------------


@when('the raw bytes behind contract "{name}"\'s stored PDF CID are fetched directly from the IPFS node')
def step_when_fetch_raw_pdf_cid(context, name):
    did, _ = ContractService._contract_data(context, name)
    # The signed PDF is stored asynchronously (signing apply -> IPFS write),
    # so poll the CID column briefly — same convention as the audit-anchor
    # polls elsewhere in the suite.
    cid = None
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        cursor = context.db.cursor()
        cursor.execute("SELECT pdf_ipfs_cid FROM contracts WHERE did = %s", (did,))
        row = cursor.fetchone()
        cursor.close()
        cid = row[0] if row else None
        if cid:
            break
        time.sleep(2)
    assert cid, f"contract '{name}' ({did}) never got a pdf_ipfs_cid within 90s"
    context.raw_ipfs_cid = cid
    context.raw_ipfs_bytes = ipfs_cat_bytes(cid)


@then("the raw stored bytes do not start with the PDF magic bytes")
def step_then_raw_bytes_not_pdf(context):
    head = context.raw_ipfs_bytes[:5]
    assert head != b"%PDF-", (
        f"the stored artifact behind CID {context.raw_ipfs_cid} starts with the "
        f"PDF magic bytes — it is a PLAINTEXT PDF in IPFS, not ciphertext "
        f"(DCS-NFR-SEC-14 requires encryption at rest)"
    )


@then('the raw stored bytes do not contain "{marker}"')
def step_then_raw_bytes_no_marker(context, marker):
    assert marker.encode() not in context.raw_ipfs_bytes, (
        f"the stored artifact behind CID {context.raw_ipfs_cid} contains the "
        f"plaintext marker '{marker}' — content is readable at rest "
        f"(DCS-NFR-SEC-14)"
    )


@when('the Contract Manager of the dedicated organization exports contract "{name}" as PDF')
def step_when_org_manager_exports(context, name):
    did, _ = ContractService._contract_data(context, name)
    context.requests_response = _export_contract_pdf(context, context.base_url, did)


@then("the export response is an intact PDF document")
def step_then_export_intact_pdf(context):
    resp = context.requests_response
    assert resp is not None and resp.status_code == 200, (
        f"authorized export did not return the PDF: "
        f"{resp.status_code if resp is not None else 'n/a'} "
        f"{resp.text[:300] if resp is not None else ''}"
    )
    assert resp.content[:5] == b"%PDF-", (
        "authorized export response does not start with the PDF magic bytes — "
        "the envelope decrypt must return the byte-identical artifact"
    )


# ---------------------------------------------------------------------------
# did.json keyAgreement (CEK wrapping key)
# ---------------------------------------------------------------------------


@when("I fetch this instance's served DID document")
def step_when_fetch_did_document(context):
    resp = _requests.get(
        did_document_url(context.base_url), timeout=context.http_timeout_seconds
    )
    assert resp.status_code == 200, (
        f"GET /.well-known/did.json failed: {resp.status_code} {resp.text}"
    )
    context.did_document = resp.json()


@then('the DID document\'s keyAgreement relation names exactly one verification method with fragment "{fragment}"')
def step_then_key_agreement_relation(context, fragment):
    doc = context.did_document
    key_agreement = doc.get("keyAgreement")
    assert isinstance(key_agreement, list) and len(key_agreement) == 1, (
        f"expected exactly one keyAgreement entry in did.json, got: {key_agreement!r}"
    )
    assert key_agreement[0] == f"{doc.get('id')}#{fragment}", (
        f"keyAgreement should reference <did>#{fragment}, got: {key_agreement[0]!r}"
    )
    context.key_agreement_vm_id = key_agreement[0]


@then("that key-agreement verification method is a P-256 JsonWebKey2020 published under its own id")
def step_then_key_agreement_vm_shape(context):
    vms = context.did_document.get("verificationMethod")
    assert isinstance(vms, list) and len(vms) >= 3, (
        f"expected at least three verificationMethod entries (identity key, VC key, "
        f"key-agreement key), got: {len(vms) if isinstance(vms, list) else vms!r}"
    )
    # Found by the id the keyAgreement relation names, NOT by position: the order
    # of verificationMethod carries no meaning, so nothing may depend on it.
    matching = [vm for vm in vms if vm.get("id") == context.key_agreement_vm_id]
    assert len(matching) == 1, (
        f"expected exactly one verificationMethod with id "
        f"{context.key_agreement_vm_id!r}, got {len(matching)}"
    )
    vm = matching[0]
    assert vm.get("type") == "JsonWebKey2020", f"unexpected VM type: {vm.get('type')!r}"
    jwk = vm.get("publicKeyJwk") or {}
    assert jwk.get("kty") == "EC" and jwk.get("crv") == "P-256", (
        f"the key-agreement key must be an EC P-256 JWK, got kty={jwk.get('kty')!r} "
        f"crv={jwk.get('crv')!r}"
    )
    assert "d" not in jwk, "did.json must publish the PUBLIC key only — the JWK carries a private 'd' component"


def _relationship_ids(doc, relationship):
    """The method ids a relationship names, absolute or relative — DID Core
    permits both spellings for the same key."""
    ids = []
    for entry in doc.get(relationship) or []:
        method_id = entry.get("id") if isinstance(entry, dict) else entry
        if not isinstance(method_id, str):
            continue
        ids.append(f"{doc.get('id')}{method_id}" if method_id.startswith("#") else method_id)
    return ids


@then("the key-agreement method appears in no other verification relationship")
def step_then_key_agreement_only(context):
    doc = context.did_document
    for relationship in ("authentication", "assertionMethod", "capabilityInvocation", "capabilityDelegation"):
        assert context.key_agreement_vm_id not in _relationship_ids(doc, relationship), (
            f"the CEK wrap key {context.key_agreement_vm_id!r} is also published under "
            f"{relationship!r} — a key published for encryption must not be usable to "
            f"verify a signature"
        )


@then("the DID document publishes its identity key for authentication")
def step_then_authentication_published(context):
    doc = context.did_document
    authentication = _relationship_ids(doc, "authentication")
    assert authentication, (
        "did.json names no authentication method, so the key answering a peer's "
        f"challenge-response is published for nothing: {doc.get('authentication')!r}"
    )
    # The authenticating key must be a published method carrying a certificate
    # chain: the counterparty validates the chain of the key that answers, so the
    # two have to be the same method.
    by_id = {vm.get("id"): vm for vm in doc.get("verificationMethod") or []}
    for method_id in authentication:
        vm = by_id.get(method_id)
        assert vm is not None, f"authentication names {method_id!r}, which is not a published verificationMethod"
        assert (vm.get("publicKeyJwk") or {}).get("x5c"), (
            f"the authentication key {method_id!r} carries no x5c chain for the peer to validate"
        )


# ---------------------------------------------------------------------------
# Peer CEK handshake
# ---------------------------------------------------------------------------


@then("the cross-instance contract's PDF export is byte-identical on instance A and instance B")
def step_then_export_byte_identical(context):
    did = context.cross_instance_contract_did
    deadline = time.monotonic() + 180
    last_reason = "no export attempted yet"
    while time.monotonic() < deadline:
        resp_a = _export_contract_pdf(context, context.base_url_a, did)
        resp_b = _export_contract_pdf(context, context.base_url_b, did)
        if resp_a is not None and resp_b is not None and resp_a.status_code == 200 and resp_b.status_code == 200:
            if resp_a.content == resp_b.content:
                assert resp_a.content[:5] == b"%PDF-", (
                    "exports matched but are not PDFs — both stores are corrupt "
                    "the same way"
                )
                return
            last_reason = (
                f"both exports answered 200 but differ "
                f"({len(resp_a.content)} vs {len(resp_b.content)} bytes)"
            )
        else:
            last_reason = (
                f"A: {resp_a.status_code if resp_a is not None else 'n/a'}, "
                f"B: {resp_b.status_code if resp_b is not None else 'n/a'}"
            )
        time.sleep(3)
    raise AssertionError(
        f"the contract PDF never exported byte-identically on both instances "
        f"within 180s — the peer must decrypt its own stored copy back to the "
        f"exact shipped bytes (wrapped-CEK handshake + verbatim-inbound rule); "
        f"last observation: {last_reason}"
    )


@then('the erasure status of the cross-instance contract reports local status "{status}" on instance {label}')
def step_then_erasure_local_status(context, status, label):
    did = context.cross_instance_contract_did
    base = _base_for_label(context, label)
    expected = status.strip().lower()
    body = None
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        body = _erasure_status(context, base, did)
        if body and body.get("local_status") == expected:
            if expected == "shredded":
                assert body.get("shredded_at"), (
                    f"local_status is shredded but shredded_at is missing: {body}"
                )
            context.last_erasure_status = body
            return
        time.sleep(2)
    raise AssertionError(
        f"erasure status on instance {label} never reported local_status="
        f"'{expected}' for {did} within 120s; last: {body}"
    )


# ---------------------------------------------------------------------------
# Key shredding (archive deletion -> local shred + peer erase)
# ---------------------------------------------------------------------------


@when("an anchored audit entry CID of the cross-instance contract is captured on instance A")
def step_when_capture_anchored_entry_cid(context):
    """Collects the contract's own chain CIDs (each entry's res_log_pred_cid
    names its predecessor in the same per-(component, DID) chain — the audit
    API deliberately never returns an entry's OWN CID) and polls the
    checkpoint-proof endpoint until one of them is committed to a checkpoint.
    Captured BEFORE the shred so the post-shred re-verification proves the
    proof survived the erasure."""
    did = context.cross_instance_contract_did
    base = context.base_url_a
    deadline = time.monotonic() + 240
    last_candidates = []
    while time.monotonic() < deadline:
        entries = _pac_audit_entries(context, base, "CONTRACT_WORKFLOW_ENGINE", did)
        last_candidates = [
            e["res_log_pred_cid"] for e in entries if e.get("res_log_pred_cid")
        ]
        headers = _org_headers(context, ["Auditor"], api_base=base)
        for cid in last_candidates:
            resp = _requests.get(
                f"{base}/pac/audit/checkpoint/proof/{cid}",
                headers=headers,
                timeout=context.http_timeout_seconds,
            )
            if resp.status_code == 200:
                context.captured_entry_cid = cid
                context.pre_shred_proof = resp.json()
                return
        time.sleep(5)
    raise AssertionError(
        f"no audit entry of contract {did} was committed to a checkpoint within "
        f"240s (candidate entry CIDs: {last_candidates}) — the outbox processor "
        f"anchors and checkpoints asynchronously, but not this slowly"
    )


@when('the Archive Manager of the dedicated organization deletes the archived cross-instance contract with justification "{justification}"')
def step_when_org_archive_manager_deletes(context, justification):
    did = context.cross_instance_contract_did
    with _as_instance(context, context.base_url_a):
        headers = _org_headers(context, ["Archive Manager"], api_base=context.base_url_a)
        # The archive entry is written when the contract reaches SIGNED —
        # asynchronously relative to the signing apply. Wait for it before
        # deleting, or the delete would be a no-op on nothing.
        deadline = time.monotonic() + 90
        seen = False
        while time.monotonic() < deadline:
            resp = get_with_headers(context, archive_retrieve_url(context), headers=headers)
            if resp.status_code == 200:
                body = resp.json()
                entries = body.get("contracts") if isinstance(body, dict) else body
                if isinstance(entries, list) and any(
                    isinstance(e, dict) and e.get("did") == did for e in entries
                ):
                    seen = True
                    break
            time.sleep(2)
        assert seen, f"contract {did} never appeared in the archive within 90s"

        delete_resp = delete_with_params(
            context,
            archive_delete_url(context),
            {"did": did, "justification": justification},
            headers=headers,
        )
        assert delete_resp.status_code == 200, (
            f"DELETE /archive/delete failed: {delete_resp.status_code} {delete_resp.text}"
        )
        context.requests_response = delete_resp
        context.erasure_justification = justification


@then("the erasure status on instance A names peer confirmation from instance B")
def step_then_peer_confirmed_on_a(context):
    did = context.cross_instance_contract_did
    peer_did_b = context.peer_did_b
    body = None
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        body = _erasure_status(context, context.base_url_a, did)
        peers = (body or {}).get("peers") or []
        match = next((p for p in peers if p.get("peer_did") == peer_did_b), None)
        if match and match.get("status") == "confirmed":
            assert match.get("confirmed_at"), (
                f"peer erase is confirmed but confirmed_at is missing: {match}"
            )
            return
        time.sleep(2)
    raise AssertionError(
        f"instance A's erasure status never reported peer {peer_did_b} as "
        f"'confirmed' for {did} within 120s; last: {body}"
    )


@then("a KEY_SHREDDED audit event for the cross-instance contract is recorded on instance {label}")
def step_then_key_shredded_recorded(context, label):
    did = context.cross_instance_contract_did
    base = _base_for_label(context, label)
    event_types = []
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        entries = _pac_audit_entries(context, base, "CONTRACT_STORAGE_ARCHIVE", did)
        event_types = [str(e.get("event_type", "")).upper() for e in entries]
        if "KEY_SHREDDED" in event_types:
            return
        time.sleep(3)
    raise AssertionError(
        f"no KEY_SHREDDED audit event for contract {did} on instance {label} "
        f"within 180s — logged key destruction (DCS-NFR-SEC-13) must be "
        f"evidenced on BOTH federation instances; event types seen: {event_types}"
    )


@then("the workflow audit entry bodies of the cross-instance contract are erased on instance A")
def step_then_audit_bodies_erased(context):
    did = context.cross_instance_contract_did
    entries = []
    deadline = time.monotonic() + 120
    not_erased = None
    while time.monotonic() < deadline:
        entries = _pac_audit_entries(
            context, context.base_url_a, "CONTRACT_WORKFLOW_ENGINE", did
        )
        timeline = [e for e in entries if str(e.get("kind", "TIMELINE")).upper() != "CHECK"]
        if timeline:
            not_erased = [
                e for e in timeline if e.get("event_data") != {"erased": True}
            ]
            if not not_erased:
                return
        time.sleep(3)
    raise AssertionError(
        f"expected every workflow audit entry body of {did} to be the defined "
        f'erased marker {{"erased": true}} after the CEK shred; still readable/'
        f"unexpected bodies: "
        f"{json.dumps([e.get('event_data') for e in (not_erased or [])])[:500]} "
        f"({len(entries)} entries total)"
    )


@then("the captured audit entry's checkpoint inclusion proof still verifies on instance A")
def step_then_proof_still_verifies(context):
    cid = context.captured_entry_cid
    with _as_instance(context, context.base_url_a):
        headers = _org_headers(context, ["Auditor"], api_base=context.base_url_a)
        resp = get_with_headers(
            context, pac_checkpoint_proof_url(context, cid), headers=headers
        )
    assert resp.status_code == 200, (
        f"GET /pac/audit/checkpoint/proof/{cid} failed AFTER the shred: "
        f"{resp.status_code} {resp.text} — shredding must erase bodies without "
        f"breaking Merkle verification (ADR-16)"
    )
    proof = resp.json()
    assert proof.get("leaf_hash") == context.pre_shred_proof.get("leaf_hash"), (
        "the entry's blinded leaf hash changed across the shred — the stored "
        "leaf bytes must be untouched by erasure"
    )
    head = proof.get("head") or {}
    ok = _verify_merkle_inclusion(
        proof["leaf_hash"],
        proof.get("siblings") or [],
        int(proof["leaf_index"]),
        int(head["leaf_count"]),
        head["root"],
    )
    assert ok, (
        f"the checkpoint inclusion proof for {cid} does not recompute to the "
        f"checkpoint root after the shred: {json.dumps(proof)[:500]}"
    )


# ---------------------------------------------------------------------------
# Erase-retry ledger (pending -> confirmed via /archive/erasure-status)
# ---------------------------------------------------------------------------


@then("the erasure ledger on instance A records the peer erase request with its request timestamp")
def step_then_erase_ledger_recorded(context):
    did = context.cross_instance_contract_did
    peer_did_b = context.peer_did_b
    body = None
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        body = _erasure_status(context, context.base_url_a, did)
        peers = (body or {}).get("peers") or []
        match = next((p for p in peers if p.get("peer_did") == peer_did_b), None)
        if match:
            assert match.get("status") in ("pending", "confirmed"), (
                f"unexpected ledger status: {match}"
            )
            assert match.get("requested_at"), (
                f"ledger row is missing requested_at: {match}"
            )
            context.erase_ledger_row = match
            return
        time.sleep(2)
    raise AssertionError(
        f"the erase request towards {peer_did_b} was never recorded in the "
        f"erasure ledger for {did} within 60s; last status: {body}"
    )


@then('the erasure ledger on instance A reaches peer status "confirmed" with a confirmation timestamp and retry accounting')
def step_then_erase_ledger_confirmed(context):
    did = context.cross_instance_contract_did
    peer_did_b = context.peer_did_b
    match = None
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        body = _erasure_status(context, context.base_url_a, did)
        peers = (body or {}).get("peers") or []
        match = next((p for p in peers if p.get("peer_did") == peer_did_b), None)
        if match and match.get("status") == "confirmed":
            assert match.get("confirmed_at"), (
                f"confirmed ledger row is missing confirmed_at: {match}"
            )
            assert isinstance(match.get("retry_count"), int) and match["retry_count"] >= 0, (
                f"confirmed ledger row carries no retry accounting: {match}"
            )
            return
        time.sleep(3)
    raise AssertionError(
        f"the peer erase for {did} towards {peer_did_b} never transitioned to "
        f"'confirmed' within 180s (pending rows are retried from the "
        f"contract_erasures ledger); last row: {match}"
    )
