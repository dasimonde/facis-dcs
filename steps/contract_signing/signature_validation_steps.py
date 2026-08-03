"""BDD steps for the signature validate/audit/compliance endpoints
(DCS-FR-SM-18, DCS-FR-SM-19, DCS-FR-SM-21, UC-04): POST /signature/validate,
GET /signature/audit, POST /signature/compliance
(backend/design/signature_management.go) - all three are already implemented
(backend/internal/service/signature_management.go); this module drives them
against a contract that has actually gone through the real signing ceremony
(contract_state_machine_steps.py's "has reached contract state \"SIGNED\""
Given, reused rather than re-invented here).
"""

import time
from datetime import datetime

import requests
from behave import then, when

import json

from steps.support.api_client import (
    post_json,
    signature_audit_url,
    signature_compliance_url,
    signature_retrieve_url,
    signature_revoke_url,
    signature_view_url,
    signature_validate_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


@when('the contract signer retrieves contract "{name}" for signing')
def step_when_signer_retrieves(context, name):
    # DCS-FR-SM-15: the retrieval a signer performs before signing —
    # GET /signature/retrieve/{did} — as the "Contract Signer" role the
    # endpoint scopes to (backend/design/signature_management.go
    # retrieve_by_id).
    did, _ = ContractService._contract_data(context, name)
    signer_h = AuthService.get_headers_for_roles(["Contract Signer"])
    context.requests_response = requests.get(
        signature_retrieve_url(context, did),
        headers=signer_h,
        timeout=context.http_timeout_seconds,
    )


@when('the contract manager validates the signature for contract "{name}"')
def step_when_validate_signature(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(context, signature_validate_url(context), {"did": did}, headers=manager_h)


@when('the applied signature of contract "{name}" is revoked')
def step_when_applied_signature_revoked(context, name):
    """Revokes the ACTUAL signer bound to the applied signature (read from
    /signature/view), not a placeholder DID — so the signature ROW flips to
    REVOKED and the compliance check can observe it."""
    import requests as _requests  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    view = _requests.get(
        signature_view_url(context), params={"did": did}, headers=manager_h,
        timeout=context.http_timeout_seconds,
    )
    assert view.status_code == 200, f"signature view failed: {view.status_code} {view.text}"
    signatures = view.json().get("signatures") or []
    assert signatures, f"Expected an applied signature to revoke, got: {view.json()}"
    signer_did = signatures[0]["signer_did"]
    context.requests_response = post_json(
        context, signature_revoke_url(context),
        {
            "did": did,
            "signer_did": signer_did,
            "reason": "Compliance check revocation",
        },
        headers=manager_h,
    )


@when('the contract manager requests a compliance check for contract "{name}"')
def step_when_compliance_check(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = post_json(
        context, signature_compliance_url(context), {"did": did}, headers=manager_h
    )


# The validate endpoint's findings list mixes defect findings with positive
# confirmations (signingmanagement/db/pg/contractrepository.go's
# CollectValidationFindings appends "Document integrity check passed" on
# success; signingmanagement/query/validate.go appends the signer-binding
# cross-check confirmation). A healthy signature therefore reports *only*
# entries from this confirmation set — an empty list would itself be a bug.
_PASSING_VALIDATION_FINDINGS = {
    "Document integrity check passed",
    "Embedded signer binding cross-checked against the signature record",
    # Phase 4 (ADR-9): crossCheckSHACLDrift's positive confirmation — the
    # pinned-hub-version SHACL report re-ran and matched the hash embedded
    # in the signing evidence (signingmanagement/query/validate.go).
    "SHACL validation report re-verified against the pinned hub schema version — no drift",
    "Validation passed",
}

# The EU DSS leg reports what DSS actually returned rather than claiming DSS
# confirmed an AES: a non-qualified dev-CA chain yields INDETERMINATE, which is
# expected here and not a defect. The indication is appended to the finding, so
# this one is matched by prefix (signingmanagement/query/validate.go
# ValidAESFinding).
_PASSING_VALIDATION_PREFIXES = (
    "EU DSS reports no integrity or cryptographic failure",
)


@then('the signature validation for contract "{name}" reports only passing checks')
def step_then_validation_no_findings(context, name):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    findings = body.get("findings") or []
    assert findings, (
        f"Expected the signature validation of freshly-signed contract '{name}' to report "
        f"its passing confirmations (e.g. the document-integrity check), got an empty findings list"
    )
    negative = [
        f
        for f in findings
        if f not in _PASSING_VALIDATION_FINDINGS
        and not f.startswith(_PASSING_VALIDATION_PREFIXES)
    ]
    assert not negative, (
        f"Expected a freshly-signed contract '{name}' to report only passing validation "
        f"confirmations, got defect findings: {negative} (full list: {findings})"
    )
    assert "Document integrity check passed" in findings, (
        f"Expected the MR/HR document-integrity check confirmation for contract '{name}', "
        f"got: {findings}"
    )


@then('the compliance check for contract "{name}" reports that all checks passed')
def step_then_compliance_all_passed(context, name):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    findings = body.get("findings") or []
    # /signature/compliance now COMPUTES its findings (DCS-FR-SM-21:
    # signature level SES/AES/QES, signature status, active signed
    # credentials — CollectComplianceFindings) and reports "Compliance
    # checks passed" as the single positive confirmation when nothing is
    # flagged. An empty list would itself be a bug.
    assert findings == ["Compliance checks passed"], (
        f"Expected the compliance check to report exactly the positive confirmation, got: {findings}"
    )


@then('the compliance check for contract "{name}" flags a revoked signature')
def step_then_compliance_flags_revoked(context, name):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    findings = body.get("findings") or []
    assert any("revoked" in str(f).lower() for f in findings), (
        f"Expected a revoked-signature compliance finding, got: {findings}"
    )


@when('the signature view for contract "{name}" is requested as "{role}"')
def step_when_signature_view(context, name, role):
    import requests as _requests  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles([role])
    context.requests_response = _requests.get(
        signature_view_url(context),
        params={"did": did},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when('I attempt to request the signature view for contract "{name}" with my current role')
def step_when_attempt_signature_view(context, name):
    import requests as _requests  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    context.requests_response = _requests.get(
        signature_view_url(context),
        params={"did": did},
        headers=getattr(context, "headers", {}),
        timeout=context.http_timeout_seconds,
    )


@then('the signature view for contract "{name}" shows one "{status}" signature with signer identity, credential class "{cred}", timestamp, and intact integrity')
def step_then_signature_view_contents(context, name, status, cred):
    assert context.requests_response.status_code == 200, (
        f"Expected 200, got {context.requests_response.status_code}: {context.requests_response.text}"
    )
    body = context.requests_response.json()
    did, _ = ContractService._contract_data(context, name)
    assert body.get("did") == did
    signatures = body.get("signatures") or []
    assert len(signatures) == 1, f"Expected exactly one signature, got: {signatures}"
    sig = signatures[0]
    assert sig.get("status") == status, f"Expected status {status!r}, got: {sig.get('status')!r}"
    assert sig.get("signer_did"), f"Expected a signer identity, got: {sig}"
    assert sig.get("credential_type") == cred, (
        f"Expected credential class {cred!r}, got: {sig.get('credential_type')!r}"
    )
    assert sig.get("signed_at"), f"Expected a signing timestamp, got: {sig}"
    assert "PAdES" in str(sig.get("format")), f"Expected a PAdES container format, got: {sig.get('format')}"
    findings = body.get("integrity_findings") or []
    negative = [
        f
        for f in findings
        if f not in _PASSING_VALIDATION_FINDINGS
        and not f.startswith(_PASSING_VALIDATION_PREFIXES)
    ]
    assert findings and not negative, (
        f"Expected only passing integrity confirmations in the signature view, got: {negative} "
        f"(full list: {findings})"
    )


@then(
    'the signature view for contract "{name}" shows the achieved level meeting '
    'the contract\'s required level "{required}", not qualified, '
    'and a non-empty signing certificate subject'
)
def step_then_signature_view_level_and_cert(context, name, required):
    """ADR-20 SM-01/SM-26: the compliance-viewer fields the sole-control gate
    and the per-contract level requirement actually feed — required_credential_type,
    qualified, signer_cert_subject, signer_cert_serial — sourced from the
    ceremony row (backend/internal/service/signature_management.go's View),
    not re-derived here; this checks the API contract the frontend's
    ComplianceViewerView.vue renders from actually carries them."""
    body = context.requests_response.json()
    signatures = body.get("signatures") or []
    assert len(signatures) == 1, f"Expected exactly one signature, got: {signatures}"
    sig = signatures[0]

    assert sig.get("required_credential_type") == required, (
        f"Expected required_credential_type {required!r}, got: {sig.get('required_credential_type')!r}"
    )
    achieved = sig.get("credential_type")
    _LEVEL_RANK = {"SES": 0, "AES": 1, "QES": 2}
    assert _LEVEL_RANK.get(achieved, -1) >= _LEVEL_RANK.get(required, 0), (
        f"Expected achieved level {achieved!r} to meet required level {required!r}"
    )
    assert sig.get("qualified") is False, (
        f"Expected qualified=False for a non-QES signature, got: {sig.get('qualified')!r}"
    )
    assert sig.get("signer_cert_subject"), (
        f"Expected a non-empty signing certificate subject (ADR-20 sole-control evidence), got: {sig}"
    )
    assert sig.get("signer_cert_serial"), (
        f"Expected a non-empty signing certificate serial, got: {sig}"
    )


@then('the signature audit log for contract "{name}" includes an action of type "{event_type}"')
def step_then_signature_audit_includes(context, name, event_type):
    # The audit trail is anchored asynchronously by the outbox processor
    # (~1s poll interval) — same polling convention as
    # contract_state_machine_steps.py's audit-event step.
    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    event_types = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = requests.get(
            signature_audit_url(context),
            params={"did": did},
            headers=auditor_h,
            timeout=context.http_timeout_seconds,
        )
        assert resp.status_code == 200, f"Signature audit query failed for '{name}': {resp.status_code} {resp.text}"
        entries = resp.json()
        assert isinstance(entries, list), f"Expected a list of signature audit entries, got: {entries}"
        event_types = [str(e.get("event_type", "")).upper() for e in entries]
        if event_type.upper() in event_types:
            return
        time.sleep(1)
    assert event_type.upper() in event_types, (
        f"Expected a '{event_type}' signature audit event for contract '{name}', got event types: {event_types}"
    )


def _signature_audit_entry(context, name, event_type):
    """Fetches the audit entry of the given event type for the contract.
    Presence is guaranteed by the polling step above (the outbox processor
    has already anchored the entry), so a single read suffices; event_data
    arrives either JSON-decoded or as a JSON string depending on the
    transport, both are handled."""
    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    resp = requests.get(
        signature_audit_url(context),
        params={"did": did},
        headers=auditor_h,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"Signature audit query failed for '{name}': {resp.status_code} {resp.text}"
    entries = [e for e in resp.json() if str(e.get("event_type", "")).upper() == event_type.upper()]
    assert entries, f"Expected a '{event_type}' signature audit entry for contract '{name}'"
    entry = entries[-1]
    event_data = entry.get("event_data")
    if isinstance(event_data, str):
        event_data = json.loads(event_data)
    return did, entry, event_data or {}


@then('the "{event_type}" signature audit entry for contract "{name}" carries the signer DID, credential type "{cred}", and an RFC3339 timestamp')
def step_then_audit_entry_apply_fields(context, event_type, name, cred):
    # DCS-FR-SM-19: signer ID, credential used, and timestamp captured in
    # the log entry itself — the ApplyEvent's fields (signingmanagement/
    # event/event.go). The signer is identified twice: applied_by is the
    # PARTICIPANT identifier from the auth context (middleware.
    # GetParticipantID = the token's ext.iss claim — the organization in the
    # BDD wallet), and holder_did is the signer's wallet DID.
    _, _, event_data = _signature_audit_entry(context, name, event_type)
    applied_by = event_data.get("applied_by")
    assert applied_by, (
        f"Expected the '{event_type}' audit entry of contract '{name}' to carry the "
        f"applying participant in 'applied_by', got event_data: {event_data}"
    )
    holder_did = event_data.get("holder_did")
    assert holder_did and str(holder_did).startswith("did:"), (
        f"Expected the '{event_type}' audit entry of contract '{name}' to name the "
        f"signer's wallet DID in 'holder_did', got event_data: {event_data}"
    )
    assert event_data.get("credential_type") == cred, (
        f"Expected the '{event_type}' audit entry of contract '{name}' to record "
        f"credential_type {cred!r}, got: {event_data.get('credential_type')!r} "
        f"(event_data: {event_data})"
    )
    occurred_at = event_data.get("occurred_at")
    assert occurred_at, (
        f"Expected the '{event_type}' audit entry of contract '{name}' to carry a "
        f"non-empty 'occurred_at', got event_data: {event_data}"
    )
    try:
        datetime.fromisoformat(str(occurred_at).replace("Z", "+00:00"))
    except ValueError:
        raise AssertionError(
            f"Expected the '{event_type}' audit entry timestamp of contract '{name}' "
            f"to parse as RFC3339, got occurred_at: {occurred_at!r}"
        )


@then('the retrieval of contract "{name}" is recorded with the retrieving signer, a timestamp, and the contract ID')
def step_then_retrieval_recorded(context, name):
    # DCS-FR-SM-15: the retrieval is logged with timestamp, signer ID, and
    # contract ID — the RetrieveByIDEvent (signingmanagement/event/event.go)
    # persisted through the transactional outbox. Read-only RETRIEVE_* events
    # are deliberately filtered OUT of the audit-result presentation
    # (base.IsAuditVisibleEventType: operational traces, not findings), so
    # the recording itself is asserted on the persisted outbox row.
    import json as _json  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    deadline = time.monotonic() + 30
    row = None
    while time.monotonic() < deadline:
        cursor = context.db.cursor()
        cursor.execute(
            """SELECT event_data FROM outbox_events
               WHERE component = 'SIGNATURE_MANAGEMENT'
                 AND event_type = 'RETRIEVE_CONTRACT_BY_ID' AND did = %s
               ORDER BY id DESC LIMIT 1""",
            (did,),
        )
        row = cursor.fetchone()
        cursor.close()
        if row:
            break
        time.sleep(1)
    assert row, f"Expected a persisted RETRIEVE_CONTRACT_BY_ID record for contract '{name}' ({did})"
    event_data = row[0] if isinstance(row[0], dict) else _json.loads(row[0])
    assert event_data.get("retrieved_by"), (
        f"Expected the retrieval record to name the retrieving signer (retrieved_by), got: {event_data}"
    )
    assert event_data.get("occurred_at"), (
        f"Expected the retrieval record to carry a timestamp (occurred_at), got: {event_data}"
    )
    assert event_data.get("did") == did, (
        f"Expected the retrieval record to carry contract ID '{did}', got: {event_data}"
    )


def _exported_pdf_bytes(context, name):
    pdf_store = getattr(context, "pdf_bytes", {}) or {}
    assert name in pdf_store, (
        f"No exported PDF bytes recorded for contract '{name}' — was "
        f"'contract \"{name}\" has an exported PDF' run first?"
    )
    return pdf_store[name]


@then('the exported PDF for contract "{name}" declares PDF/A-3 conformance in its XMP metadata')
def step_then_pdf_declares_pdfa(context, name):
    # DCS-FR-SM-27 / ISO 19005-3 clause 6.6.4: PDF/A version and conformance
    # level are declared via the pdfaid XMP extension schema. pdf-core
    # compiles part=3, conformance=A (compiler/compiler_pdf.go).
    pdf = _exported_pdf_bytes(context, name)
    assert b'pdfaid:part="3"' in pdf, (
        f"Expected the exported PDF of '{name}' to declare pdfaid:part=\"3\" "
        f"(PDF/A-3) in its XMP metadata"
    )
    assert b'pdfaid:conformance="A"' in pdf, (
        f"Expected the exported PDF of '{name}' to declare pdfaid:conformance=\"A\" "
        f"in its XMP metadata"
    )


@then('the exported PDF for contract "{name}" embeds the canonical JSON-LD payload as an associated file')
def step_then_pdf_embeds_jsonld(context, name):
    # The machine-readable payload rides inside the PDF/A-3 container as an
    # associated file: Filespec (contract.jsonld) with AFRelationship /Source
    # and an application/ld+json embedded file stream.
    pdf = _exported_pdf_bytes(context, name)
    assert b"(contract.jsonld)" in pdf, (
        f"Expected a (contract.jsonld) Filespec in the exported PDF of '{name}'"
    )
    assert b"/AFRelationship /Source" in pdf, (
        f"Expected the contract.jsonld attachment of '{name}' to be an associated "
        f"file with AFRelationship /Source"
    )
    assert b"application#2Fld+json" in pdf, (
        f"Expected an application/ld+json embedded file stream in the exported PDF of '{name}'"
    )
