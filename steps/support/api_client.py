"""Shared HTTP and URL helpers for executable BDD scenarios."""

import requests


def origin_url(base_url: str) -> str:
    """Scheme+host only, stripping any path (e.g. route.basePath + '/api').

    did.json is mounted at the bare origin root per the did:web spec
    (backend/cmd/dcs/http.go: didsvr.Mount(mux, didServer) uses the
    unprefixed base mux, not the DCS_API_PATH-prefixed apiMux) — appending
    '/.well-known/did.json' directly to a base_url that already carries
    route.basePath (non-empty in every values.bdd.yml/kind-CI deployment)
    produces a path Goa never registers. Use this helper, not string
    concatenation, wherever the well-known DID document is fetched.
    """
    return "/".join(base_url.split("/", 3)[:3])


def did_document_url(base_url: str) -> str:
    return f"{origin_url(base_url)}/.well-known/did.json"


def agreement_credential_url(base_url: str) -> str:
    """The federation agreement credential (ADR-19): a self-signed W3C VC,
    published next to did.json at the bare origin root for the same reason
    (did:web-style well-known resolution, no route.basePath prefix)."""
    return f"{origin_url(base_url)}/.well-known/dcs-agreement-credential.json"


# URL builders

def contract_create_url(context) -> str:
    return f"{context.base_url}/contract/create"


def contract_update_url(context) -> str:
    return f"{context.base_url}/contract/update"


def contract_submit_url(context) -> str:
    return f"{context.base_url}/contract/submit"


def contract_negotiate_url(context) -> str:
    return f"{context.base_url}/contract/negotiate"


def contract_accept_offer_url(context) -> str:
    """Accepting an INBOUND offer as-is (backend/design/contract_workflow_engine.go
    accept_offer): mints the accepting instance's negotiation task for the
    offer's round and moves OFFERED -> NEGOTIATION. Distinct from
    /contract/respond, which decides one already-proposed change request."""
    return f"{context.base_url}/contract/accept-offer"


def contract_negotiation_draft_url(context, did: str = None) -> str:
    """PUT saves to the bare path; GET/DELETE address the caller's draft by
    contract DID (backend/design/contract_workflow_engine.go
    save_negotiation_draft / retrieve_negotiation_draft /
    delete_negotiation_draft)."""
    base = f"{context.base_url}/contract/negotiation_draft"
    return f"{base}/{did}" if did else base


def contract_review_url(context) -> str:
    return f"{context.base_url}/contract/review"


def contract_approve_url(context) -> str:
    return f"{context.base_url}/contract/approve"


def contract_reject_url(context) -> str:
    return f"{context.base_url}/contract/reject"


def contract_retrieve_url(context) -> str:
    return f"{context.base_url}/contract/retrieve"


def contract_retrieve_by_id_url(context, did: str) -> str:
    return f"{context.base_url}/contract/retrieve/{did}"


def contract_history_url(context, did: str) -> str:
    return f"{context.base_url}/contract/history/{did}"


def contract_verify_url(context) -> str:
    return f"{context.base_url}/contract/verify"


def contract_offer_url(context) -> str:
    return f"{context.base_url}/contract/offer"


def contract_withdraw_url(context) -> str:
    return f"{context.base_url}/contract/withdraw"


def contract_terminate_url(context) -> str:
    return f"{context.base_url}/contract/terminate"


def contract_renew_url(context) -> str:
    return f"{context.base_url}/contract/renew"


def contract_search_url(context) -> str:
    return f"{context.base_url}/contract/search"


def contract_audit_url(context) -> str:
    return f"{context.base_url}/contract/audit"


# Deployment endpoints (backend/design/contract_workflow_engine.go).

def contract_deploy_url(context) -> str:
    return f"{context.base_url}/contract/deploy"


def contract_targets_url(context) -> str:
    return f"{context.base_url}/contract/targets"


def contract_target_designate_url(context) -> str:
    return f"{context.base_url}/contract/target/designate"


def contract_deployment_callback_url(context) -> str:
    return f"{context.base_url}/contract/deployment/callback"


def archive_search_url(context) -> str:
    return f"{context.base_url}/archive/search"


def archive_retrieve_url(context) -> str:
    return f"{context.base_url}/archive/retrieve"


def archive_audit_url(context) -> str:
    return f"{context.base_url}/archive/audit"


def archive_statistics_url(context) -> str:
    return f"{context.base_url}/archive/statistics"


def archive_delete_url(context) -> str:
    return f"{context.base_url}/archive/delete"


def archive_erasure_status_url(context) -> str:
    """GET /archive/erasure-status?did= (backend/design/contract_storage_archive.go
    erasure_status): local CEK shred state plus the per-peer erase-handshake
    ledger for one contract."""
    return f"{context.base_url}/archive/erasure-status"


def archive_annotate_url(context) -> str:
    return f"{context.base_url}/archive/annotate"


def signature_view_url(context) -> str:
    return f"{context.base_url}/signature/view"


def pac_audit_url(context) -> str:
    return f"{context.base_url}/pac/audit"


def pac_audit_timeline(response) -> list[dict]:
    """The DCS-procured audit entries of one POST /pac/audit response.

    /pac/audit answers with a single external-audit-executor run envelope
    (backend/design/process_audit_and_compliance.go, PACExternalAuditResponse),
    not with a list of per-scope trails: `findings` is the executor's verdict
    and `timeline` is the evidence the DCS itself gathered and submitted.
    Entries carry their own `did`, so the per-resource grouping the executor
    request uses is not reproduced here.
    """
    body = response.json()
    assert isinstance(body, dict) and body.get("audit_id"), (
        f"Expected an executor-run audit envelope from /pac/audit, got: {body!r}"
    )
    return [entry for entry in (body.get("timeline") or []) if isinstance(entry, dict)]


def pac_checkpoint_head_url(context) -> str:
    return f"{context.base_url}/pac/audit/checkpoint/head"


def pac_checkpoint_proof_url(context, entry_cid: str) -> str:
    return f"{context.base_url}/pac/audit/checkpoint/proof/{entry_cid}"


def admin_hsm_keys_url(context) -> str:
    """GET /admin/hsm-keys (backend/design/key_inventory.go): the read-only
    HSM key inventory for the Sys. Administrator."""
    return f"{context.base_url}/admin/hsm-keys"


def pac_report_url(context) -> str:
    return f"{context.base_url}/pac/report"


def pac_monitor_url(context) -> str:
    return f"{context.base_url}/pac/monitor"


def contract_peer_action_url(context) -> str:
    return f"{context.base_url}/peer/contracts/action"


def contract_peer_post_sync_url(context) -> str:
    return f"{context.base_url}/peer/contracts/"


def contract_peer_pdf_url(context) -> str:
    """POST /peer/contracts/pdf (post_pdf, backend/design/dcs_to_dcs.go): the
    CURRENT DCS-to-DCS receiving endpoint (ADR-13's PDF-shipping model). The
    older contract_peer_post_sync_url / contract_peer_action_url above target
    a full-JSON-state-sync shape that predates ADR-13 and no longer exists in
    the Goa design — kept as-is here since fixing that drift is outside
    ADR-19's scope, but new agreement-credential (ADR-19) scenarios must
    target this endpoint instead."""
    return f"{context.base_url}/peer/contracts/pdf"


def contract_peer_provenance_url(context) -> str:
    return f"{context.base_url}/peer/contracts/provenance"


def contract_peer_settlement_url(context) -> str:
    """POST /peer/contracts/settlement (post_settlement,
    backend/design/dcs_to_dcs.go): where a peer deposits its signed statement
    that it settled a named version of a contract this instance holds."""
    return f"{context.base_url}/peer/contracts/settlement"


def signature_prepare_url(context) -> str:
    return f"{context.base_url}/signature/prepare"


def signature_submit_url(context) -> str:
    return f"{context.base_url}/signature/submit"


def signature_revoke_url(context) -> str:
    return f"{context.base_url}/signature/revoke"


def signature_validate_url(context) -> str:
    return f"{context.base_url}/signature/validate"


def signature_retrieve_url(context, did: str) -> str:
    return f"{context.base_url}/signature/retrieve/{did}"


def signature_audit_url(context) -> str:
    return f"{context.base_url}/signature/audit"


def signature_compliance_url(context) -> str:
    return f"{context.base_url}/signature/compliance"


# Signing-ceremony endpoints (backend/design/signature_management.go).

def signature_request_url(context) -> str:
    return f"{context.base_url}/signature/request"


def signature_request_by_id_url(context, ceremony_id: str) -> str:
    return f"{context.base_url}/signature/request/{ceremony_id}"


def signature_request_publish_url(context, ceremony_id: str) -> str:
    return f"{context.base_url}/signature/request/{ceremony_id}/publish"


def signature_request_leaf_url(context, ceremony_id: str, leaf: str) -> str:
    """Harness-reachable URL for a per-ceremony signing-request sub-resource
    (object/document/callback). The request object the DCS publishes carries these
    URLs built from its advertised public base; this rebuilds them on the origin
    the harness actually routes to."""
    return f"{context.base_url}/signature/request/{ceremony_id}/{leaf}"


def template_create_url(context) -> str:
    return f"{context.base_url}/template/create"


def template_retrieve_by_id_url(context, did: str) -> str:
    return f"{context.base_url}/template/retrieve/{did}"


def template_retrieve_url(context) -> str:
    return f"{context.base_url}/template/retrieve"


def template_submit_url(context) -> str:
    return f"{context.base_url}/template/submit"


def template_update_url(context) -> str:
    return f"{context.base_url}/template/update"


def template_update_manage_url(context) -> str:
    return f"{context.base_url}/template/update_manage"


def template_verify_url(context) -> str:
    return f"{context.base_url}/template/verify"


def template_provenance_url(context, did: str) -> str:
    return f"{context.base_url}/template/provenance/{did}"


def template_approve_url(context) -> str:
    return f"{context.base_url}/template/approve"


def template_reject_url(context) -> str:
    return f"{context.base_url}/template/reject"


def template_register_url(context) -> str:
    return f"{context.base_url}/template/register"


def template_archive_url(context) -> str:
    return f"{context.base_url}/template/archive"


def template_audit_url(context) -> str:
    return f"{context.base_url}/template/audit"


def template_search_url(context) -> str:
    return f"{context.base_url}/template/search"


def template_publish_url(context) -> str:
    return f"{context.base_url}/template/publish"


def catalogue_template_retrieve_url(context) -> str:
    return f"{context.base_url}/catalogue/template/retrieve"


def catalogue_template_retrieve_by_id_url(context, did: str) -> str:
    return f"{context.base_url}/catalogue/template/retrieve/{did}"


def catalogue_template_search_url(context) -> str:
    return f"{context.base_url}/catalogue/template/search"


# HTTP helpers

def post_json(context, url: str, payload: dict, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.post(
        url,
        json=payload,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


def put_json(context, url: str, payload: dict, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.put(
        url,
        json=payload,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


def get_with_headers(context, url: str, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.get(
        url,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


def delete_with_params(context, url: str, params: dict, headers=None):
    h = headers if headers is not None else getattr(context, "headers", {})
    return requests.delete(
        url,
        params=params,
        headers=h,
        timeout=context.http_timeout_seconds,
    )


# PDF generation URL builders

def contract_export_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/export/contract/{did}"


def template_export_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/export/template/{did}"


def contract_verify_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/verify/contract/{did}"


def template_verify_pdf_url(context, did: str) -> str:
    return f"{context.base_url}/pdf/verify/template/{did}"


# C2PA remote-manifest URL: a public, unauthenticated sibling of
# GET /.well-known/did.json (DCS-OR-C2PA-008).

def c2pa_manifest_url(context, did: str) -> str:
    return f"{context.base_url}/c2pa/manifest/{did}"


# Bundle export URLs: one ZIP per contract/template with an integrity
# manifest (FR-TR-24, FR-CWE-30).

def contract_export_url(context, did: str) -> str:
    return f"{context.base_url}/contract/export/{did}"


def template_export_url(context, did: str) -> str:
    return f"{context.base_url}/template/export/{did}"


# sh:shapesGraph is SHACL's multi-valued data-graph -> shapes-graph link
# (ADR-8): a document declares the canonical hub shapes first and, when its
# data objects are modelled against registered SHACL libraries (ADR-23), one
# further anchor per library. Steps asserting the pin read the canonical one.

def hub_shapes_anchors(document: dict) -> list:
    declared = document.get("sh:shapesGraph")
    entries = declared if isinstance(declared, list) else [declared]
    anchors = []
    for entry in entries:
        anchor = entry.get("@id") if isinstance(entry, dict) else entry
        if isinstance(anchor, str) and "/semantic/shapes/" in anchor:
            anchors.append(anchor)
    return anchors
