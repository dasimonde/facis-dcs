"""Steps for the Power-of-Attorney signing gate and compliance
(features/22_real_signing_vertical/poa_at_signing.feature; UC-14, DCS-FR-SM-03/
-04/-26).

The signatory presents a fresh PoA credential at the ceremony, alongside the
PID, in one wallet vp_token (ADR-20 — the EUDIPLO webhook this used to go
through is removed). The callback verifies the PoA authorizes the very party
(the participating instance DID) being signed; a missing or wrong-organization
PoA blocks signing (UC-14). The compliance viewer re-checks every party node in
the (possibly peer-synced) contract and raises a finding for any that signed
without a valid PoA, which is recorded as an audit event (FR-SM-04/-26).
"""

from __future__ import annotations

import contextlib
import json
import os
import socket
import subprocess
import time
import uuid

from behave import given, then, when

from steps.support.api_client import (
    contract_peer_pdf_url,
    post_json,
    signature_compliance_url,
    signature_request_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (
    ceremony_aud,
    _build_pid_presentation,
    _complete_ceremony_via_presentation,
    _fetch_pending_nonce,
)


def _start_party_ceremony(context, name):
    """Start a ceremony for the signing party (the local instance DID) and build
    the PID presentation the completion steps need, without completing it."""
    party_did = ContractService._local_peer_did(context)
    did, _ = ContractService._contract_data(context, name)
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    resp = post_json(context, signature_request_url(context), {"contract_did": did, "field_name": party_did}, headers=signer_h)
    assert resp.status_code == 200, f"/signature/request failed: {resp.status_code} {resp.text}"
    ceremony_id = resp.json()["ceremony_id"]
    nonce = _fetch_pending_nonce(context, ceremony_id)
    presentation, _issuer, _disc, subject_did = _build_pid_presentation(
        given_name="PoA Signatory", family_name="BDD-Testperson", aud=ceremony_aud(context), nonce=nonce,
    )
    context.poa_ceremony = {"id": ceremony_id, "presentation": presentation, "subject_did": subject_did, "party_did": party_did, "nonce": nonce}


@when('a signing ceremony is started for the signing party of contract "{name}"')
def step_when_start_party_ceremony(context, name):
    _start_party_ceremony(context, name)


@when('the ceremony presentation is completed with no Power of Attorney')
def step_when_presentation_no_poa(context):
    c = context.poa_ceremony
    context.requests_response = _complete_ceremony_via_presentation(
        context, c["id"], c["presentation"], c["subject_did"], "PoA Signatory", "BDD-Testperson",
        poa_organization="", nonce=c["nonce"],
    )


@when('the ceremony presentation is completed with a Power of Attorney for a different party')
def step_when_presentation_wrong_poa(context):
    c = context.poa_ceremony
    context.requests_response = _complete_ceremony_via_presentation(
        context, c["id"], c["presentation"], c["subject_did"], "PoA Signatory", "BDD-Testperson",
        poa_organization="did:web:some-other-org.example", nonce=c["nonce"],
    )


@then('the signing request is rejected because the Power of Attorney does not authorize this signature')
def step_then_poa_rejected(context):
    resp = context.requests_response
    assert resp.status_code == 400, f"expected 400 (PoA gate), got {resp.status_code}: {resp.text}"
    assert "power of attorney" in resp.text.lower(), f"expected a power-of-attorney rejection, got: {resp.text}"


def _compliance_findings(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Contract Manager"])
    resp = post_json(context, signature_compliance_url(context), {"did": did}, headers=headers)
    assert resp.status_code == 200, f"/signature/compliance failed: {resp.status_code} {resp.text}"
    return resp.json().get("findings") or []


@then('the signature compliance for contract "{name}" raises no Power of Attorney finding')
def step_then_no_poa_finding(context, name):
    findings = _compliance_findings(context, name)
    offending = [f for f in findings if "power of attorney" in f.lower()]
    assert not offending, f"expected no Power of Attorney finding, got: {offending}"


@then('the signature compliance for contract "{name}" raises a Power of Attorney finding')
def step_then_poa_finding(context, name):
    findings = _compliance_findings(context, name)
    offending = [f for f in findings if "power of attorney" in f.lower()]
    assert offending, f"expected a Power of Attorney finding, got findings: {findings}"
    context.poa_finding = offending[0]


@when('the counterparty Power of Attorney on contract "{name}" is tampered to authorize a different organization')
def step_when_tamper_counterparty_poa(context, name):
    """Simulate a misconfigured/malicious counterparty DCS: inject the party node
    such a peer would have sealed and synced — a signed party (dcs:hasSignatory)
    whose dcs:hasPowerOfAttorney authorizes a different organization than the
    party itself. Compliance must raise a finding for it (FR-SM-04)."""
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute("SELECT contract_data FROM contracts WHERE did = %s", (did,))
    row = cursor.fetchone()
    assert row, f"contract {did} not found in the test DB"
    doc = row[0] if isinstance(row[0], dict) else json.loads(row[0])
    parties = doc.get("dcs:parties")
    if not isinstance(parties, list):
        parties = []
        doc["dcs:parties"] = parties
    parties.append({
        "@id": "did:web:counterparty-org.example",
        "@type": "dcs:CompanyParty",
        "dcs:hasSignatory": {"@id": "did:jwk:counterparty-signer"},
        "dcs:hasPowerOfAttorney": {"@id": "did:web:impostor-org.example"},
    })
    cursor.execute("UPDATE contracts SET contract_data = %s WHERE did = %s", (json.dumps(doc), did))
    context.db.commit()


# ---------------------------------------------------------------------------
# Mutual Power-of-Attorney binding across instances (ADR-31, ADR-35): each party
# embeds the credential behind its own signature INTO the contract PDF before
# signing it, and the receiver verifies every attachment it finds instead of
# reading the contract's dcs:hasPowerOfAttorney claim as though a peer's own
# assertion were evidence.
# ---------------------------------------------------------------------------


def _poa_presentation_from_untrusted_issuer(organization: str) -> str:
    """A structurally genuine Power of Attorney authorizing organization, issued
    by an issuer no instance configures — the honest "present but does not
    verify" case, and the one an operator most plausibly meets: a counterparty
    whose issuer was never granted the `peer` purpose here."""
    AuthService._ensure_dcs_wallet_importable()
    from dcs_wallet.issuer import (  # noqa: PLC0415
        POA_VCT,
        attach_key_binding,
        sign_credential_sd_jwt,
    )
    from dcs_wallet.keys import cnf_jwk, did_jwk_from_public_jwk, public_jwk  # noqa: PLC0415
    from dcs_wallet.status_list import build_credential_status, role_credential_index  # noqa: PLC0415

    # Built here rather than through dcs_wallet's issuance entry points, which
    # issue as the stack's own credential issuer: the point of this credential
    # is an issuer nobody configured, so it can be resolved by no mechanism the
    # trust document declares.
    keys = AuthService.load_wallet_keys()
    roles = ["Contract Signer"]
    holder_public = public_jwk(keys.wallet_private)
    issued = sign_credential_sd_jwt(
        visible_claims={
            "iss": "did:web:untrusted-poa-issuer.example:issuer:poa",
            "sub": did_jwk_from_public_jwk(holder_public),
            "vct": POA_VCT,
            "iat": 1719129600,
            "exp": 2145916800,
            "cnf": {"jwk": cnf_jwk(holder_public)},
            "status": build_credential_status(
                index=role_credential_index(organization=organization, roles=roles),
            ),
        },
        selective_claims={"organization": organization, "roles": roles},
        issuer_private=keys.issuer_private,
    )
    return attach_key_binding(
        issued_sd_jwt=issued,
        wallet_private=keys.wallet_private,
        aud="https://the-counterparty.example",
        nonce="a-nonce-this-instance-never-issued",
    )


def _pdf_core_service() -> str:
    # Instance A's pdf-core ClusterIP service, named for the release the BDD
    # Helm chart deploys. Overridable for CI parity, like BDD_PDF_CORE_DEPLOYMENT.
    return os.environ.get("BDD_PDF_CORE_SERVICE", "dcs-digital-contracting-service-pdf-core")


@contextlib.contextmanager
def _pdf_core_forwarded():
    """Reach instance A's pdf-core directly, over a kubectl port-forward to its
    ClusterIP service.

    The DCS does not expose an "attach this evidence to this PDF" endpoint —
    embedding is a step of its own signing pipeline — so a scenario that has to
    build a PDF carrying evidence the DCS would never produce speaks to pdf-core
    itself. Same infra-seam convention as the PAdES-B-B fallback scenario's
    kubectl calls (dcs_real_signing_vertical_orce_steps.py): the namespace and
    kubectl binary come from the environment, and a missing one hard-fails
    rather than guessing.
    """
    kubectl = os.environ.get("BDD_KUBECTL") or os.environ.get("KUBECTL_BIN", "kubectl")
    namespace = os.environ.get("K8S_NAMESPACE")
    assert namespace, (
        "K8S_NAMESPACE is not set — required to reach instance A's pdf-core for the "
        "embedded-evidence ship. Hard-failing rather than guessing a namespace."
    )

    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        port = probe.getsockname()[1]

    proc = subprocess.Popen(  # noqa: S603
        [kubectl, "-n", namespace, "port-forward", f"service/{_pdf_core_service()}", f"{port}:8080"],
        stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
    )
    try:
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            if proc.poll() is not None:
                raise AssertionError(
                    f"kubectl port-forward to {_pdf_core_service()} exited: "
                    f"{proc.stderr.read().decode(errors='replace')}"
                )
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=1):
                    break
            except OSError:
                time.sleep(0.5)
        else:
            raise AssertionError(f"kubectl port-forward to {_pdf_core_service()} never became reachable")
        yield f"http://127.0.0.1:{port}"
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()


def _embed_extra_evidence(pdf: bytes, evidence: dict) -> bytes:
    """Append ONE more signing evidence attachment to an already-signed PDF, the
    way a countersigning instance appends its own before signing: an incremental
    update that leaves the signature already there valid. The multipart shape
    mirrors the backend's own client (plain form fields, no filenames)."""
    import requests as _requests  # noqa: PLC0415

    with _pdf_core_forwarded() as base_url:
        resp = _requests.post(
            f"{base_url}/evidence/embed",
            files={
                "pdf": (None, pdf, "application/pdf"),
                "evidence": (None, json.dumps(evidence).encode(), "application/json"),
            },
            timeout=120,
        )
    assert resp.status_code == 200, (
        f"pdf-core /evidence/embed refused the extra attachment: {resp.status_code} {resp.text[:400]}"
    )
    return resp.content


def _retained_signing_summary(context, contract_did: str) -> str:
    """The signing summary instance A issued for the signature it applied
    (migrations/sql/20260732_signature_summary_evidence.sql). The receiver reads
    the attribution from the summary embedded next to the Power of Attorney — so
    the attachment this scenario builds must carry the real one, or the ship is
    refused for unattested evidence instead of for the credential under test."""
    cursor = context.db.cursor()
    cursor.execute(
        "SELECT summary_vc FROM signature_ceremonies "
        " WHERE contract_did = %s AND consumed_at IS NOT NULL AND summary_vc IS NOT NULL "
        " ORDER BY consumed_at DESC LIMIT 1",
        (contract_did,),
    )
    row = cursor.fetchone()
    cursor.close()
    assert row and row[0], (
        f"no signing summary is retained for {contract_did}; the ship this scenario builds is only "
        "the synchronizer's own ship with the Power of Attorney substituted, so the rest of the "
        "evidence has to be the real thing"
    )
    summary = row[0]
    return summary if isinstance(summary, str) else bytes(summary).decode()


@when('instance A ships contract "{name}"\'s PDF to instance B with a Power of Attorney that does not verify')
def step_when_a_ships_bad_poa_to_b(context, name):
    """Instance A's own outbound ship with exactly one substitution, made where
    the evidence actually lives: inside the PDF. A's genuine signed PDF gains one
    further signing evidence attachment for the party A signed as, carrying A's
    real signing summary and a Power of Attorney instance B cannot verify (an
    issuer no instance grants the `peer` purpose). A's identity and
    challenge-response, A's real agreement credential, A's own signed PDF and the
    summary attesting that signature are all genuine, so B's refusal can only
    come from the credential itself — every gate in front of it passes.

    The receiver verifies EVERY attachment it finds rather than the newest per
    field (ADR-35), which is what makes an appended one decisive.

    The ship is built here rather than driven through A's synchronizer because a
    synchronizer ship is retried from sync_fails until it succeeds, and each
    attempt raises its own incident on B; a scenario asserting exactly one
    incident has to make exactly one attempt."""
    import base64  # noqa: PLC0415

    from steps.peer_trust.dcs_peer_trust_steps import (  # noqa: PLC0415
        _as_instance,
        _own_identity,
        _sign_secret_value_with_dev_key,
    )
    from steps.support.services.pdf_service import PDFService  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    party_did, token_dir = _own_identity(context)

    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_a)
    export = PDFService.export_contract_pdf(context, did, headers=manager_h)
    assert export.status_code == 200, (
        f"could not export contract '{name}' from instance A: {export.status_code} {export.text}"
    )

    shipped_pdf = _embed_extra_evidence(export.content, {
        "summary": json.loads(_retained_signing_summary(context, did)),
        "poa_presentation": _poa_presentation_from_untrusted_issuer(party_did),
    })

    secret_value = str(uuid.uuid4())
    secret_hash = base64.b64encode(_sign_secret_value_with_dev_key(token_dir, secret_value)).decode()

    payload = {
        "from_peer_did": party_did,
        "contract_iri": did,
        "pdf": base64.b64encode(shipped_pdf).decode(),
        "secret_value": secret_value,
        "secret_hash": secret_hash,
        "contract_state": "SIGNED",
    }
    with _as_instance(context, context.base_url_b):
        context.requests_response = post_json(context, contract_peer_pdf_url(context), payload, headers={})


@then("instance B refuses the ship because the counterparty's Power of Attorney does not verify")
def step_then_pdf_rejected_counterparty_poa(context):
    resp = context.requests_response
    assert resp.status_code != 200, (
        f"Expected the ship to be refused: a Power of Attorney that is shipped and does not verify "
        f"fails closed (ADR-31), got 200: {resp.text}"
    )
    body = resp.text.lower()
    assert "power of attorney" in body, (
        f"Expected the rejection to name the Power of Attorney specifically, not some other "
        f"non-200 outcome, got {resp.status_code}: {resp.text}"
    )


@then('exactly one incident is recorded in instance B\'s audit trail for contract "{name}"')
def step_then_one_incident_on_b(context, name):
    from steps.peer_trust.dcs_peer_trust_steps import _as_instance  # noqa: PLC0415
    from steps.peer_trust.dcs_trust_pdp_steps import _count_trust_gate_incidents  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    with _as_instance(context, context.base_url_b):
        deadline = time.monotonic() + 60
        matching = []
        while time.monotonic() < deadline:
            matching = _count_trust_gate_incidents(context, contract_did=did, api_base=context.base_url_b)
            if matching:
                break
            time.sleep(2)
    assert len(matching) == 1, (
        f"Expected exactly one PAC_TRUST_GATE_DENIAL incident in instance B's audit trail for "
        f"contract '{name}' (did={did}), got {len(matching)}: {matching}"
    )


@then("instance B holds instance A's signature with its Power of Attorney verified")
def step_then_counterparty_poa_verified_on_b(context):
    """Instance B accepted a signature ship that carried A's Power of Attorney.
    B verifies that credential before it persists anything of the ship, so the
    stored provenance and the absence of a denial incident together say the
    verification passed — a credential that did not verify would have refused
    the exchange and raised one instead."""
    import requests as _requests  # noqa: PLC0415

    from steps.peer_trust.dcs_trust_pdp_steps import _count_trust_gate_incidents  # noqa: PLC0415

    c_did = context.cross_instance_contract_did
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)

    party = None
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        resp = _requests.get(
            f"{context.base_url_b}/contract/retrieve/{c_did}",
            headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        if resp.status_code == 200:
            nodes = (resp.json().get("contract_data") or {}).get("dcs:parties") or []
            for node in nodes:
                if isinstance(node, dict) and node.get("@id") == context.peer_did_a and node.get("dcs:hasSignatory"):
                    party = node
                    break
        if party:
            break
        time.sleep(2)

    assert party, (
        f"Expected instance B to hold instance A ({context.peer_did_a}) as a signed party of {c_did}; "
        "a refused Power of Attorney would have blocked the whole ship"
    )
    authorized_by = party.get("dcs:hasPowerOfAttorney")
    if isinstance(authorized_by, dict):
        authorized_by = authorized_by.get("@id")
    assert authorized_by == context.peer_did_a, (
        f"Expected the signed party to be authorized for itself, got: {authorized_by}"
    )

    denials = _count_trust_gate_incidents(context, contract_did=c_did, api_base=context.base_url_b)
    assert not denials, (
        f"Expected instance B to record no trust-gate denial for {c_did}, got: {denials}"
    )


@then('an audit event records the Power of Attorney finding for contract "{name}"')
def step_then_audit_records_poa(context, name):
    import requests  # noqa: PLC0415

    from steps.support.api_client import signature_audit_url  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    event_types = []
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        resp = requests.get(signature_audit_url(context), params={"did": did}, headers=auditor_h, timeout=context.http_timeout_seconds)
        assert resp.status_code == 200, f"signature audit read failed: {resp.status_code} {resp.text}"
        entries = resp.json()
        event_types = [str(e.get("event_type", "")).upper() for e in entries]
        if "COMPLIANCE_VALIDATION" in event_types:
            return
        time.sleep(1)
    assert "COMPLIANCE_VALIDATION" in event_types, (
        f"expected a COMPLIANCE_VALIDATION audit event recording the Power of Attorney finding, got: {event_types}"
    )
