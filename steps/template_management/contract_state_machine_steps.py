"""BDD steps for the contract-state-machine-refactor requirement.

Covers the new Offer/Withdraw commands, the extended transition table
(OFFERED, WITHDRAWN, ACTIVE, REVOKED), the C2PA lifecycle mapping for the
new states, and the outbox events emitted by Offer/Withdraw.

These steps intentionally build each precondition state (`Given contract
"<name>" has reached contract state "<state>"`) through the *narrowest*
already-existing endpoint chain rather than depending on other, unrelated
steps that are already broken in this codebase (e.g. the "verify" step used
by `ContractService._prepare_contract_pending_approval`, which targets a
`/contract/verify` route that does not exist in the Goa design). This keeps
a scenario's pass/fail signal attributable to the contract-state-machine
refactor itself.
"""

import base64
import os
import re
import shlex
import time
import uuid
from datetime import datetime
from pathlib import Path
from urllib.parse import unquote

import requests as _requests
from behave import given, step, then, when

from steps.support.api_client import (
    contract_approve_url,
    contract_audit_url,
    did_document_url,
    contract_offer_url,
    contract_peer_action_url,
    contract_retrieve_by_id_url,
    contract_search_url,
    contract_submit_url,
    contract_terminate_url,
    contract_withdraw_url,
    get_with_headers,
    post_json,
    signature_revoke_url,
)
from steps.support.signing import wallet_sign
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.pdf_service import PDFService


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _seed_headers(context, name):
    seed = getattr(context, "contract_seed_headers", None) or {}
    if name in seed:
        return seed[name]
    return getattr(context, "headers", None)


def _repo_root() -> Path:
    # This file lives at steps/template_management/<this file>.py.
    return Path(__file__).resolve().parents[2]


def _did_web_to_hostname(did: str) -> str:
    """Mirror identity.DIDWebToHostname (backend/internal/base/identity/did.go)
    so the BDD client can resolve exactly the hostname:port the server itself
    will resolve when it later verifies this signature server-side.
    """
    prefix = "did:web:"
    assert did.startswith(prefix), f"not a did:web identifier: {did}"
    rest = did[len(prefix):]
    host_encoded = rest.split(":", 1)[0]
    assert host_encoded, f"did:web identifier has empty host component: {did}"
    return unquote(host_encoded)


def _dev_signing_token_dir(hostname: str) -> Path:
    """Map a did:web hostname (e.g. 'localhost:8991') to the matching
    per-instance SoftHSM2 token dir (~/.dcs/softhsm-<port>/), the same
    convention dev-stack.sh (8991) and dev-stack2.sh (8992) provision
    (PKCS#11-only key custody).
    Only these two known dev ports are supported: this is a self-peer
    simulation, not a generic did:web resolver, and only works because we
    control the matching HSM token for the instance under test.
    """
    if os.environ.get("BDD_HSMSIGN_EXEC"):
        # Helm/kind harness: the token lives in the cluster, not on this
        # machine; _sign_secret_value_with_dev_key execs into the DCS pod.
        return None
    match = re.search(r":(\d+)$", hostname)
    assert match, (
        f"cannot derive a dev signing token for did:web hostname '{hostname}' "
        "(expected '<host>:<port>', e.g. 'localhost:8991')"
    )
    port = match.group(1)
    token_dir = Path.home() / ".dcs" / f"softhsm-{port}"
    conf_path = token_dir / "softhsm2.conf"
    assert conf_path.is_file(), (
        f"no SoftHSM2 token dir at '{token_dir}' for did:web port {port} — "
        "the peer-path self-simulation in this scenario only supports the "
        "checked-in backend/.env.dev1 (8991) / backend/.env.dev2 (8992) dev "
        "identities, provisioned via scripts/hsm-provision.sh under "
        "dev-stack.sh/dev-stack2.sh. If this DCS instance runs under a "
        "different PKCS11 token layout (e.g. the Helm/kind BDD harness), the "
        "peer-path self-simulation cannot be proven this way."
    )
    return token_dir


def _sign_secret_value_with_dev_key(token_dir: Path, secret_value: str) -> bytes:
    """ECDSA P-256 (SHA-256, ASN.1 DER) signature matching DIDDocument.Sign
    (backend/internal/base/identity/did.go): the DID private key lives only
    inside the SoftHSM2 token (no extractable PEM key exists), so this
    shells out to backend/cmd/hsmsign, which opens the same
    token via crypto11 and signs through the HSM.

    Under the Helm/kind BDD harness (BDD_HSMSIGN_EXEC set), the token lives
    inside the cluster, not on this machine: sign by kubectl-exec'ing into the
    DCS pod's already-built /app/hsmsign instead of `go run` locally.
    """
    import subprocess

    exec_prefix = os.environ.get("BDD_HSMSIGN_EXEC")
    if exec_prefix:
        result = subprocess.run(
            [*shlex.split(exec_prefix), "/app/hsmsign", "-label", "dcs-did",
             "-message", secret_value],
            capture_output=True, text=True, timeout=60,
        )
        assert result.returncode == 0, (
            f"hsmsign via kubectl exec failed: {result.stderr.strip()}"
        )
        return base64.b64decode(result.stdout.strip())

    env = dict(os.environ)
    env["SOFTHSM2_CONF"] = str(token_dir / "softhsm2.conf")
    env.setdefault("PKCS11_MODULE_PATH", "/usr/lib/softhsm/libsofthsm2.so")
    env.setdefault("PKCS11_TOKEN_LABEL", "dcs")
    env.setdefault("PKCS11_PIN", "1234")
    backend_dir = _repo_root() / "backend"
    result = subprocess.run(
        ["go", "run", "./cmd/hsmsign", "-label", "dcs-did", "-message", secret_value],
        cwd=str(backend_dir),
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, (
        f"hsmsign failed (token dir '{token_dir}'): {result.stderr.strip()}"
    )
    return base64.b64decode(result.stdout.strip())


def _self_peer_action_credentials(context):
    """Simulate a trusted peer by fetching this DCS instance's own did:web
    document (public, unauthenticated GET /.well-known/did.json — see
    backend/design/did.go) and signing a fresh secret with the matching
    checked-in dev private key. The peer action endpoint
    (backend/internal/service/dcs_to_dcs.go Action()) then does a real,
    successful did:web challenge-response verification against this
    instance's own identity, instead of failing on an unresolvable/invalid
    peer hostname before ever reaching the transition-table check.

    This only proves the peer-path claim because the contract under test
    was also created locally on this same instance (Origin == this DID):
    Approver.Handle's single-writer-per-aggregate forwarding check
    (`processData.Origin != localPeer`) is therefore a no-op, and the very
    same `contractstate.ValidateTransition` the UI-API path hits is reached
    directly — see backend/internal/contractworkflowengine/command/approve.go.
    """
    did_url = did_document_url(context.base_url)
    did_resp = _requests.get(
        did_url,
        timeout=context.http_timeout_seconds,
    )
    assert did_resp.status_code == 200, (
        f"could not fetch this instance's own did:web document from "
        f"{did_url} (required to simulate a "
        f"trusted peer): {did_resp.status_code} {did_resp.text}"
    )
    from_peer_did = did_resp.json().get("id")
    assert from_peer_did, f"own did.json response has no 'id' field: {did_resp.text}"

    hostname = _did_web_to_hostname(from_peer_did)
    token_dir = _dev_signing_token_dir(hostname)

    secret_value = str(uuid.uuid4())
    signature = _sign_secret_value_with_dev_key(token_dir, secret_value)
    secret_hash = base64.b64encode(signature).decode()

    return from_peer_did, secret_value, secret_hash


def _offer_contract(context, name):
    return _post_reissuing_on_conflict(
        context, contract_offer_url(context), {}, _seed_headers(context, name), name,
        "Offer while preparing OFFERED state",
    )


def _withdraw_contract(context, name):
    return _post_reissuing_on_conflict(
        context, contract_withdraw_url(context), {}, _seed_headers(context, name), name,
        "Withdraw while preparing WITHDRAWN state",
    )


def _advance_to_submitted(context, name):
    # DRAFT -> NEGOTIATION -> SUBMITTED via the plain submit chain
    # (deliberately not routed through Offer: this helper only needs *a*
    # contract sitting in SUBMITTED, and the plain chain is the shortest
    # path there).
    ContractService._prepare_contract_under_review(context, name)


def _advance_to_reviewed(context, name):
    _advance_to_submitted(context, name)
    did, _ = ContractService._contract_data(context, name)
    reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"])
    # Same treatment as _post_reissuing_on_conflict, inlined because the
    # reviewer payload is rebuilt around the fresh token rather than merely
    # carrying it: a background writer advancing updated_at between the read
    # and this submit is a retryable conflict, not the thing under test.
    review_submit = None
    retrieved = {}
    for _ in range(4):
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=reviewer_h)
        assert retrieve.status_code == 200, retrieve.text
        retrieved = retrieve.json()
        updated_at = retrieved.get("updated_at")
        review_submit = post_json(
            context,
            contract_submit_url(context),
            ContractService._contract_reviewer_submit_payload(context, did, updated_at),
            headers=reviewer_h,
        )
        if review_submit.status_code != 409:
            break
        time.sleep(1)
    assert review_submit is not None and review_submit.status_code == 200, (
        f"Reviewer submit (forward_to=approval) failed while preparing REVIEWED state for "
        f"'{name}' from retrieved state {retrieved.get('state')!r}: "
        f"{review_submit.status_code} {review_submit.text}"
    )
    ContractService._refresh_contract(context, name)


def _advance_to_approved(context, name):
    _advance_to_reviewed(context, name)
    did, _ = ContractService._contract_data(context, name)
    approver_h = AuthService.get_headers_for_roles(["Contract Approver"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=approver_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")
    _post_reissuing_on_conflict(
        context, contract_approve_url(context), {}, approver_h, name,
        "Approve while preparing APPROVED state",
    )


def _apply_signature(context, name):
    # /signature/apply is gated on a completed PID presentation ceremony for
    # the exact (contract_did, signer_did) pair (see real_signing_vertical).
    # Every caller of this shared setup helper — across every feature that
    # merely wants a contract "in SIGNED state" as a precondition, not as the
    # thing under test — must therefore run a real ceremony first, or every
    # such scenario 422s with "ceremony_required". Deferred import: avoids a
    # module-load-time cycle (real_signing_vertical's steps import
    # _advance_to_approved etc. from this module).
    from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (  # noqa: PLC0415
        _run_full_ceremony,
    )

    did, _updated_at = ContractService._contract_data(context, name)
    party_did = ContractService._local_peer_did(context)
    ceremony_id, _presentation, subject_did = _run_full_ceremony(
        context, name, party_did, "BDD Counterparty Signer"
    )
    # The refusal is explicit about being temporary — the async regenerator is
    # still writing the PDF this signature must cover — so it is waited out
    # rather than failing a scenario on the pipeline's own pacing.
    deadline = time.monotonic() + 120
    while True:
        resp = wallet_sign(
            context, did, signer_did=subject_did, signatory="BDD Counterparty Signer", ceremony_id=ceremony_id
        )
        if resp.status_code == 200:
            break
        if "still being regenerated" not in resp.text or time.monotonic() >= deadline:
            break
        time.sleep(5)
    assert resp.status_code == 200, (
        f"Wallet signing failed while preparing SIGNED state for '{name}': {resp.status_code} {resp.text}"
    )
    ContractService._refresh_contract(context, name)


def _revoke_signature(context, name):
    # Suspended (C2PA lifecycle banner) is exercised through the existing,
    # wired /signature/revoke command (backend/internal/signingmanagement/
    # command/revoke.go) rather than an invented seam: revoke.go validates
    # the EventRevoke transition and updates the contract's own state to
    # REVOKED after flipping the signature row's status, so
    # ContractState.Revoked is observable through ContractRepo.ReadDataByDID
    # / the verify endpoint's lifecycle_status. The revoked signer is read
    # from /signature/view — revoking an unknown signer is a 400
    # (db.ErrSignatureNotFound), not a silent no-op.
    import requests as _requests  # noqa: PLC0415

    from steps.support.api_client import signature_view_url  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    view = _requests.get(
        signature_view_url(context), params={"did": did}, headers=manager_h,
        timeout=context.http_timeout_seconds,
    )
    assert view.status_code == 200, (
        f"signature view failed while preparing REVOKED state for '{name}': {view.status_code} {view.text}"
    )
    signatures = view.json().get("signatures") or []
    assert signatures, f"Expected an applied signature to revoke on '{name}', got: {view.json()}"
    resp = post_json(
        context,
        signature_revoke_url(context),
        {
            "did": did,
            "signer_did": signatures[0]["signer_did"],
            "reason": "BDD setup to reach REVOKED state",
        },
        headers=manager_h,
    )
    assert resp.status_code == 200, (
        f"Revoke failed while preparing REVOKED state for '{name}': {resp.status_code} {resp.text}"
    )
    ContractService._refresh_contract(context, name)


def _post_reissuing_on_conflict(context, url, payload, headers, name, what):
    """POST a lifecycle setup command, re-reading updated_at immediately before
    each attempt.

    These helpers only exist to put a contract into a given state; the token
    they hold was read a step or more earlier, and a background writer (the PDF
    regenerator, an arriving peer ship) can advance updated_at in between. The
    backend answers that with a retryable conflict, so re-read and reissue
    rather than fail a scenario on a race it is not testing. Steps that
    deliberately present a stale token to prove the guard bites do NOT use this.
    """
    did, _ = ContractService._contract_data(context, name)
    resp = None
    for _ in range(4):
        ContractService._refresh_contract(context, name)
        _, updated_at = ContractService._contract_data(context, name)
        resp = post_json(context, url, dict(payload, did=did, updated_at=updated_at), headers=headers)
        if resp.status_code != 409:
            break
        time.sleep(1)
    assert resp is not None and resp.status_code == 200, (
        f"{what} failed for '{name}': {resp.status_code if resp is not None else 'no request made'} "
        f"{resp.text if resp is not None else ''}"
    )
    ContractService._refresh_contract(context, name)
    return resp


def _terminate_contract(context, name):
    _post_reissuing_on_conflict(
        context,
        contract_terminate_url(context),
        {"reason": "BDD setup"},
        AuthService.get_headers_for_roles(["Contract Manager"]),
        name,
        "Terminate while preparing TERMINATED state",
    )


def _reach_state(context, name, state):
    normalized = state.strip().upper()
    if normalized == "DRAFT":
        ContractService._create_contract_in_draft(context, name)
    elif normalized == "OFFERED":
        ContractService._create_contract_in_draft(context, name)
        _offer_contract(context, name)
    elif normalized == "WITHDRAWN":
        ContractService._create_contract_in_draft(context, name)
        _offer_contract(context, name)
        _withdraw_contract(context, name)
    elif normalized == "NEGOTIATION":
        ContractService._create_contract_in_negotiation(context, name)
    elif normalized == "SUBMITTED":
        ContractService._create_contract_in_draft(context, name)
        _advance_to_submitted(context, name)
    elif normalized == "REVIEWED":
        ContractService._create_contract_in_draft(context, name)
        _advance_to_reviewed(context, name)
    elif normalized == "APPROVED":
        ContractService._create_contract_in_draft(context, name)
        _advance_to_approved(context, name)
    elif normalized == "SIGNED":
        ContractService._create_contract_in_draft(context, name)
        _advance_to_approved(context, name)
        _apply_signature(context, name)
    elif normalized == "TERMINATED":
        ContractService._create_contract_in_draft(context, name)
        _advance_to_approved(context, name)
        _terminate_contract(context, name)
    elif normalized == "REVOKED":
        # Suspended (C2PA lifecycle banner, DCS-OR-C2PA-006) — revoking the
        # signature also transitions the contract's own state column (see
        # _revoke_signature).
        ContractService._create_contract_in_draft(context, name)
        _advance_to_approved(context, name)
        _apply_signature(context, name)
        _revoke_signature(context, name)
    else:
        raise NotImplementedError(
            f"No BDD setup path implemented for target contract state '{state}' — "
            "ACTIVE (deployment/ORCE) is out of scope for the contract-state-machine-"
            "refactor AC set and is not wired here."
        )


# ---------------------------------------------------------------------------
# Given
# ---------------------------------------------------------------------------


@given('contract "{name}" has reached contract state "{state}"')
def step_given_contract_reached_state(context, name, state):
    _reach_state(context, name, state)
    did, _ = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    actual_state = str(retrieve.json().get("state", "")).upper()
    accepted = {state.strip().upper()}
    if state.strip().upper() == "SIGNED":
        # Signing completion triggers the automatic deployment dispatch
        # (DCS-FR-CWE-06/SM-12), and the shipped ORCE contract-target-flow
        # acknowledges it for real — so by the time this read lands the
        # contract may already have taken the only edge out of SIGNED via
        # EventDeploy (contractstate.Transitions: ACTIVE is reachable
        # exclusively from SIGNED). Observing ACTIVE therefore PROVES the
        # contract reached SIGNED; it is not a weaker check.
        accepted.add("ACTIVE")
    assert actual_state in accepted, (
        f"BDD setup could not reach state '{state}' for contract '{name}': "
        f"got '{actual_state}'"
    )


@given('contract "{name}" is a draft with an unfilled required placeholder')
def step_given_draft_with_open_placeholder(context, name):
    """A DRAFT contract derived from a template whose clause carries an inline,
    required dcs:ContractField without a dcs:value — the document is not closed
    (validation.ValidateContractClosed: "prose placeholder binds to unfilled
    field"), which is exactly what the offer gate rejects. Field @ids are
    urn:uuid:* on the template; contract creation rebases them onto the
    contract DID (documentdata.go rebaseIDText), field node and prose
    reference alike, so closedness still pairs them up."""
    from steps.support.services.template_service import TemplateService  # noqa: PLC0415

    field_id = "urn:uuid:field-payment-amount"
    template_data = TemplateService.canonical_document_data("BDD Open Placeholder Template")
    template_data["@context"]["xsd"] = "http://www.w3.org/2001/XMLSchema#"
    template_data["dcs:contractFields"] = [
        {
            "@id": field_id,
            "@type": "dcs:ContractField",
            "dcs:label": "Payment Amount",
            "dcs:datatype": "xsd:decimal",
            "dcs:required": True,
        }
    ]
    clause = template_data["dcs:documentStructure"]["dcs:blocks"]["@list"][0]
    clause["dcs:content"] = {
        "@list": ["The provider invoices ", {"@id": field_id}, " per period."]
    }
    ContractService._create_contract_in_draft(context, name, template_data=template_data)


@when('the initiator fills the required placeholder of contract "{name}" with "{value}"')
def step_when_fill_required_placeholder(context, name, value):
    from steps.support.api_client import contract_update_url, put_json  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    body = retrieve.json()
    contract_data = body.get("contract_data")
    assert contract_data, f"contract '{name}' has no contract_data: {body}"

    filled = 0
    for field in contract_data.get("dcs:contractFields") or []:
        if field.get("dcs:required") and not field.get("dcs:value"):
            # A fill is a typed literal carrying the field's declared
            # datatype — the lexical string keeps the exact agreed token.
            field["dcs:value"] = {"@value": value, "@type": field.get("dcs:datatype", "xsd:string")}
            filled += 1
    assert filled, (
        f"contract '{name}' has no unfilled required field to fill: "
        f"{contract_data.get('dcs:contractFields')}"
    )

    resp = put_json(
        context,
        contract_update_url(context),
        {"did": did, "updated_at": body.get("updated_at"), "contract_data": contract_data},
        headers=headers,
    )
    assert resp.status_code == 200, (
        f"Filling the required placeholder of '{name}' failed: {resp.status_code} {resp.text}"
    )
    ContractService._refresh_contract(context, name)


@given('contract "{name}" has an expiry date in the past')
def step_given_expiry_date_in_past(context, name):
    """Test-only seam for the "Expired" C2PA lifecycle banner
    (DCS-OR-C2PA-006): directly backdate the contract's `exp_date`
    column via the shared test DB connection (context.db, see
    environment.py) instead of exercising `contract/update`, which rejects
    any exp_date less than one day in the future
    (command/update.go:114-118: "expiration date must be at least one day
    in the future") and only accepts EventUpdate from Draft
    (Transitions[Draft][EventUpdate]) — a real 24h+ wait is not practical
    inside an automated BDD run. This mirrors the already-accepted
    precedent of a direct context.db seam for a precondition the API itself
    has no fast path to establish, also used (read-only, for assertions
    rather than seeding) by steps/peer_trust/dcs_trust_pdp_steps.py's
    `sync_fails` polling (ADR-19).

    This step does NOT itself flip the contract's `state` to EXPIRED — that
    remains the job of the already-running expiry cron
    (contractworkflowengine/cronjobs.go, polling every
    conf.ExpirationCronJobTimeOut() = 1 minute; see
    contractworkflowengine/db/pg/contractrepository.go:241-261's
    ReadExpiredContracts query, which only force-flips non-terminal states —
    the contract must therefore already be in a non-terminal state such as
    SIGNED before calling this step). It polls briefly afterwards for that
    cron tick to land, no faster time-travel is invented here.
    """
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute(
        "UPDATE contracts SET exp_date = NOW() - INTERVAL '1 day' WHERE did = %s",
        (did,),
    )
    context.db.commit()
    cursor.close()

    headers = _seed_headers(context, name)
    actual_state = None
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
        assert retrieve.status_code == 200, retrieve.text
        actual_state = str(retrieve.json().get("state", "")).upper()
        if actual_state == "EXPIRED":
            ContractService._refresh_contract(context, name)
            return
        time.sleep(5)
    assert actual_state == "EXPIRED", (
        f"Expected the expiry cron (conf.ExpirationCronJobTimeOut() = 1 minute poll "
        f"interval) to flip contract '{name}' to EXPIRED after backdating exp_date "
        f"into the past, but state is still '{actual_state}' after 90s"
    )


# ---------------------------------------------------------------------------
# When
# ---------------------------------------------------------------------------


@when('the initiator offers contract "{name}"')
def step_when_offer_contract(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    context.requests_response = post_json(
        context, contract_offer_url(context), {"did": did, "updated_at": updated_at}, headers=headers
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('the initiator withdraws contract "{name}"')
def step_when_withdraw_contract(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    context.requests_response = post_json(
        context, contract_withdraw_url(context), {"did": did, "updated_at": updated_at}, headers=headers
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


# @step: used both as a When (state-machine pack) and as a Given precondition
# (multi-signer pack).
@step('contract "{name}" is submitted, reviewed, and approved via the standard workflow')
def step_when_full_approval_workflow(context, name):
    _advance_to_approved(context, name)


@when('the counterparty signer applies a signature to contract "{name}"')
def step_when_apply_signature(context, name):
    # /signature/apply rejects a request without a prior signing ceremony
    # with 422 "ceremony_required" (backend/internal/signingmanagement), so
    # this step runs a real ceremony first, reusing
    # steps/real_signing_vertical/dcs_real_signing_vertical_steps.py's
    # `_run_full_ceremony`/`_apply_signature` (deferred import: avoids the
    # module-load-time cycle documented on `_apply_signature` above). The
    # calling scenarios only assert a 200 afterwards, so running the real
    # ceremony here does not change any caller's expected outcome.
    from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (  # noqa: PLC0415
        _apply_signature as _apply_signature_with_ceremony_result,
        _run_full_ceremony,
    )

    _run_full_ceremony(context, name, ContractService._local_peer_did(context), "BDD Counterparty Signer")
    subject_did = context.pid_presentations[name]["subject_did"]
    context.requests_response = _apply_signature_with_ceremony_result(
        context, name, signer_did=subject_did, credential_type="AES"
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('the contract manager terminates contract "{name}" with reason "{reason}"')
def step_when_terminate_contract_with_reason(context, name, reason):
    did, updated_at = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    # The backend records TerminatedBy = middleware.GetParticipantID, which the
    # OIDC validator reads from the token's ext.iss claim (oidc.go
    # ValidateToken); capture it here so the audit-event step can match the
    # recorded identity against the actual caller.
    manager_token = manager_h["Authorization"].removeprefix("Bearer ").strip()
    manager_claims = AuthService.decode_jwt_payload(manager_token)
    context.terminating_participant = (manager_claims.get("ext") or {}).get("iss")
    context.requests_response = post_json(
        context,
        contract_terminate_url(context),
        {"did": did, "reason": reason, "updated_at": updated_at},
        headers=manager_h,
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('the initiator attempts to update terminated contract "{name}"')
def step_when_attempt_update_terminated(context, name):
    from steps.support.api_client import contract_update_url, put_json  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    # Fresh updated_at: the optimistic-concurrency guard in command/update.go
    # runs before the transition check, so a stale timestamp would fail as a
    # concurrency error (500) instead of the TERMINATED transition rejection
    # this scenario asserts.
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    context.requests_response = put_json(
        context,
        contract_update_url(context),
        {
            "did": did,
            "updated_at": retrieve.json().get("updated_at"),
            "description": "post-termination edit attempt",
        },
        headers=headers,
    )


@when('the initiator attempts to negotiate a change on terminated contract "{name}"')
def step_when_attempt_negotiate_terminated(context, name):
    from steps.support.api_client import contract_negotiate_url  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    # Fresh updated_at for the same reason as the update attempt above:
    # negotiate.go checks content_updated_at before ValidateTransition.
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    context.requests_response = post_json(
        context,
        contract_negotiate_url(context),
        {
            "did": did,
            "updated_at": retrieve.json().get("updated_at"),
            "negotiated_by": AuthService.username_for_roles(["Contract Creator"]),
            "change_request": "post-termination change attempt",
        },
        headers=headers,
    )


@when('the reviewer returns contract "{name}" for modification with finding "{finding}"')
def step_when_reviewer_returns_with_finding(context, name, finding):
    # POST /contract/submit from SUBMITTED with forward_to=reject reopens the
    # review/negotiation/approval task rows and returns the contract to
    # NEGOTIATION (command/submit.go, actionflag.Reject branch); the finding
    # travels in the comments array onto the SUBMIT_CONTRACT audit event.
    did, _ = ContractService._contract_data(context, name)
    reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=reviewer_h)
    assert retrieve.status_code == 200, retrieve.text
    context.requests_response = post_json(
        context,
        contract_submit_url(context),
        {
            "did": did,
            "updated_at": retrieve.json().get("updated_at"),
            "forward_to": "reject",
            "comments": [finding],
        },
        headers=reviewer_h,
    )
    if context.requests_response.status_code == 200:
        ContractService._refresh_contract(context, name)


@when('a peer attempts to approve contract "{name}" via the peer action endpoint')
def step_when_peer_attempts_approve(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    # Simulate a trusted, successfully-authenticated peer (see
    # _self_peer_action_credentials docstring) so a 4xx/5xx here can only be
    # the transition-table rejection itself, not a did:web auth failure.
    from_peer_did, secret_value, secret_hash = _self_peer_action_credentials(context)
    payload = {
        "action": "approve",
        "component": "CONTRACT_WORKFLOW_ENGINE",
        "from_peer_did": from_peer_did,
        "payload": {"did": did, "updated_at": updated_at},
        "secret_value": secret_value,
        "secret_hash": secret_hash,
    }
    context.requests_response = post_json(context, contract_peer_action_url(context), payload, headers={})


@when('contract "{name}" is exported and verified as PDF')
def step_when_export_and_verify(context, name):
    did, _ = ContractService._contract_data(context, name)
    export_resp = PDFService.export_contract_pdf(context, did)
    assert export_resp.status_code == 200, (
        f"PDF export failed for contract '{name}': {export_resp.status_code} {export_resp.text}"
    )
    context.requests_response = PDFService.verify_contract_pdf(context, did)


@when('the contract search endpoint is queried with state filter "{state}"')
def step_when_search_by_state(context, state):
    # /contract/search is JWT-gated; lifecycle Givens seed with per-role
    # headers and never set context.headers, so self-authorize when absent.
    headers = getattr(context, "headers", None) or AuthService.get_headers_for_roles(["Contract Manager"])
    context.requests_response = _requests.get(
        contract_search_url(context),
        params={"state": state},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


# ---------------------------------------------------------------------------
# Then
# ---------------------------------------------------------------------------


@then('the contract "{name}" is in state "{state}"')
def step_then_contract_in_state(context, name, state):
    did, _ = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    actual = str(retrieve.json().get("state", "")).upper()
    assert actual == state.strip().upper(), (
        f"Expected contract '{name}' to be in state '{state}', got '{actual}'"
    )


@then('the contract "{name}" has completed signing')
def step_then_contract_completed_signing(context, name):
    """Signing completion assertion that is stable under the REAL deployment
    chain: EventSign lands the contract in SIGNED, the automatic dispatch
    (DCS-FR-CWE-06/SM-12) goes to the shipped ORCE contract-target-flow, and
    its acknowledgement callback may flip SIGNED -> ACTIVE within moments.
    ACTIVE is reachable exclusively from SIGNED (contractstate.Transitions),
    so either state proves the signature completed."""
    did, _ = ContractService._contract_data(context, name)
    headers = _seed_headers(context, name)
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    actual = str(retrieve.json().get("state", "")).upper()
    assert actual in ("SIGNED", "ACTIVE"), (
        f"Expected contract '{name}' to have completed signing (SIGNED, or ACTIVE once "
        f"the target acknowledged the automatic deployment), got '{actual}'"
    )


@then("the withdraw request is rejected")
def step_then_withdraw_rejected(context):
    assert context.requests_response.status_code in (400, 404, 409, 422), (
        "Expected withdraw to be rejected once the contract is no longer in a "
        f"pre-approval state, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )


@then("the request is denied with a client error")
def step_then_denied_client_error(context):
    assert context.requests_response.status_code in (400, 401, 403, 404, 409, 422), (
        "Expected the invalid state transition to be rejected, got "
        f"{context.requests_response.status_code}: {context.requests_response.text}"
    )


@then("the peer action request fails")
def step_then_peer_action_fails(context):
    resp = context.requests_response
    assert resp.status_code != 200, (
        "Expected the invalid transition attempted via the peer action endpoint to "
        f"fail, got 200: {resp.text}"
    )
    # The peer-auth handshake (did:web fetch + eIDAS check + challenge-response
    # verify, see backend/internal/service/dcs_to_dcs.go Action()) is simulated
    # as succeeding (see _self_peer_action_credentials), so a failure here can
    # only honestly evidence the peer-path claim if it is the same
    # contractstate.ValidateTransition rejection the UI-API path hits — not a
    # did:web auth error that happens to also return a non-200.
    body_text = resp.text.lower()
    assert "transition" in body_text or "not allowed" in body_text, (
        "Expected the peer action to fail because of the invalid state "
        "transition itself (backend/internal/contractworkflowengine/datatype/"
        "contractstate ErrInvalidTransition), not a did:web peer-auth error — "
        f"got {resp.status_code}: {resp.text}"
    )


@then('the contract "{name}" has an audit event of type "{event_type}"')
def step_then_contract_has_audit_event(context, name, event_type):
    # The audit trail is a hash-chained log the outbox processor persists to
    # IPFS asynchronously (~1s poll interval, see conf.OutboxProcessorTimeOut
    # and base/audittrail.go's ReadLogCID) — it is not written synchronously
    # within the offer/withdraw request, so this polls briefly instead of
    # asserting immediately.
    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    event_types = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = post_json(context, contract_audit_url(context), {"did": did}, headers=auditor_h)
        assert resp.status_code == 200, f"Audit query failed for contract '{name}': {resp.status_code} {resp.text}"
        events = resp.json()
        assert isinstance(events, list), f"Expected audit response to be a list, got: {events}"
        event_types = [str(e.get("event_type", "")).upper() for e in events]
        if event_type.upper() in event_types:
            return
        time.sleep(1)
    assert event_type.upper() in event_types, (
        f"Expected an audit event of type '{event_type}' for contract '{name}', "
        f"got event types: {event_types}"
    )


def _parse_rfc3339(value):
    # Go time.Time marshals as RFC3339Nano ("...T...Z" with 0-9 fractional
    # digits); Python 3.10 fromisoformat accepts neither "Z" nor fractions
    # other than 3/6 digits, so normalize both before parsing.
    text = str(value)
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    text = re.sub(r"\.(\d+)", lambda m: "." + m.group(1)[:6].ljust(6, "0"), text)
    return datetime.fromisoformat(text)


@then("the CREATE_CONTRACT audit event for the created contract has a non-zero RFC3339 occurred_at timestamp")
def step_then_create_event_has_effective_timestamp(context):
    creation = context.requests_response
    assert creation.status_code == 200, (
        "Contract creation must succeed before its audit event can be checked, "
        f"got {creation.status_code}: {creation.text}"
    )
    did = creation.json().get("did")
    assert did, f"Contract creation response has no DID: {creation.text}"

    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    create_event = None
    events = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = post_json(context, contract_audit_url(context), {"did": did}, headers=auditor_h)
        assert resp.status_code == 200, (
            f"Audit query failed for created contract '{did}': "
            f"{resp.status_code} {resp.text}"
        )
        events = resp.json()
        assert isinstance(events, list), f"Expected audit response to be a list, got: {events}"
        create_event = next(
            (
                entry
                for entry in events
                if str(entry.get("event_type", "")).upper() == "CREATE_CONTRACT"
            ),
            None,
        )
        if create_event is not None:
            break
        time.sleep(1)

    assert create_event is not None, (
        f"Expected a CREATE_CONTRACT audit event for contract '{did}', "
        f"got event types: {[entry.get('event_type') for entry in events]}"
    )
    event_data = create_event.get("event_data") or {}
    occurred_at = event_data.get("occurred_at")
    assert occurred_at, f"occurred_at missing from CREATE_CONTRACT event data: {event_data}"
    assert re.fullmatch(
        r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})",
        str(occurred_at),
    ), f"occurred_at is not RFC3339: {occurred_at!r}"
    parsed = _parse_rfc3339(occurred_at)
    assert parsed.year > 1, (
        "CREATE_CONTRACT occurred_at must be an effective timestamp, "
        f"not Go's zero time: {occurred_at!r}"
    )


@then('the TERMINATE_CONTRACT audit event for contract "{name}" records reason "{reason}", the terminating identity and role, and an effective timestamp')
def step_then_terminate_event_records_payload(context, name, reason):
    # DCS-FR-CSA-16: the termination record is the TERMINATE_CONTRACT audit
    # event's payload — reason/terminated_by/occurred_at/user_roles JSON tags
    # on TerminateEvent (backend/internal/contractworkflowengine/event/
    # event.go). Same async-outbox polling rationale as the generic
    # audit-event step above.
    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    events = []
    record = None
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = post_json(context, contract_audit_url(context), {"did": did}, headers=auditor_h)
        assert resp.status_code == 200, (
            f"Audit query failed for contract '{name}': {resp.status_code} {resp.text}"
        )
        events = resp.json()
        assert isinstance(events, list), f"Expected audit response to be a list, got: {events}"
        for entry in events:
            if str(entry.get("event_type", "")).upper() != "TERMINATE_CONTRACT":
                continue
            data = entry.get("event_data") or {}
            if data.get("reason") == reason:
                record = data
                break
        if record is not None:
            break
        time.sleep(2)
    if record is None:
        terminate_payloads = [
            e.get("event_data") for e in events
            if str(e.get("event_type", "")).upper() == "TERMINATE_CONTRACT"
        ]
        raise AssertionError(
            f"Expected a TERMINATE_CONTRACT audit event for contract '{name}' carrying "
            f"reason '{reason}', got TERMINATE_CONTRACT payloads: {terminate_payloads}"
        )

    expected_participant = getattr(context, "terminating_participant", None)
    assert expected_participant, (
        "context.terminating_participant is unset — the terminate When step must "
        "run first so the recorded identity can be matched against the caller"
    )
    assert record.get("terminated_by") == expected_participant, (
        f"terminated_by should be the initiating participant '{expected_participant}', "
        f"got: {record.get('terminated_by')!r}"
    )
    assert "Contract Manager" in (record.get("user_roles") or []), (
        f"user_roles should record the initiating role 'Contract Manager', "
        f"got: {record.get('user_roles')!r}"
    )
    occurred_at = record.get("occurred_at")
    assert occurred_at, f"occurred_at missing from termination record: {record}"
    try:
        _parse_rfc3339(occurred_at)
    except ValueError as exc:
        raise AssertionError(
            f"occurred_at {occurred_at!r} does not parse as an RFC3339 timestamp: {exc}"
        ) from exc


@then('the SUBMIT_CONTRACT audit event for contract "{name}" records the reviewer finding "{finding}"')
def step_then_submit_event_records_finding(context, name, finding):
    # DCS-IR-CWE-06: the reviewer's finding comment is persisted on the
    # SUBMIT_CONTRACT audit event (SubmitEvent.Comments) together with the
    # REJECT action flag that sent the contract back to NEGOTIATION.
    did, _ = ContractService._contract_data(context, name)
    auditor_h = AuthService.get_headers_for_roles(["Auditor"])
    events = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        resp = post_json(context, contract_audit_url(context), {"did": did}, headers=auditor_h)
        assert resp.status_code == 200, (
            f"Audit query failed for contract '{name}': {resp.status_code} {resp.text}"
        )
        events = resp.json()
        assert isinstance(events, list), f"Expected audit response to be a list, got: {events}"
        for entry in events:
            if str(entry.get("event_type", "")).upper() != "SUBMIT_CONTRACT":
                continue
            data = entry.get("event_data") or {}
            if str(data.get("action_flag", "")).upper() == "REJECT" and finding in (data.get("comments") or []):
                return
        time.sleep(2)
    submit_payloads = [
        e.get("event_data") for e in events
        if str(e.get("event_type", "")).upper() == "SUBMIT_CONTRACT"
    ]
    raise AssertionError(
        f"Expected a SUBMIT_CONTRACT audit event for contract '{name}' with action_flag "
        f"REJECT carrying the reviewer finding '{finding}' in its comments, got "
        f"SUBMIT_CONTRACT payloads: {submit_payloads}"
    )


@then('the C2PA lifecycle_status for contract "{name}" is "{status}"')
def step_then_c2pa_lifecycle_status(context, name, status):
    assert context.requests_response.status_code == 200, (
        f"Verify failed for contract '{name}': {context.requests_response.status_code} "
        f"{context.requests_response.text}"
    )
    body = context.requests_response.json()
    actual = str(body.get("lifecycle_status", "")).lower()
    assert actual == status.lower(), (
        f"Expected C2PA lifecycle_status '{status}' for contract '{name}', got '{actual}': {body}"
    )


@then('the search results include contract "{name}"')
def step_then_search_includes_contract(context, name):
    did, _ = ContractService._contract_data(context, name)
    results = context.requests_response.json()
    assert isinstance(results, list), f"Expected search response to be a list, got: {results}"
    dids = [r.get("did") for r in results]
    assert did in dids, f"Expected contract '{name}' ({did}) in search results, got dids: {dids}"
