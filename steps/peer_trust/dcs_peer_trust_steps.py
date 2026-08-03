"""BDD steps for two-instance peer trust (features/17_peer_trust; SRS
NFR-BR-08, DCS-FR-CWE-01/-15) AND for the federation agreement credential /
PDP gate (ADR-19, docs/adr-19-federation-agreement-credential.md, requirement
slug `fed-agreement`).

ADR-19 replaces the third trust layer this module used to exercise — the
static `trusted_peers` allowlist table, which this file used to seed directly
via an `INSERT INTO trusted_peers` test seam (`_seed_trusted_peer`, removed) —
with an agreement-credential check (layer 3a) plus a local, per-instance
policy endpoint (PDP, layer 3b), fail-closed. The
`trusted_peers` table, its `DCS_TRUSTED_PEERS` seeding, and
`CheckForUntrustedPeers` are to be removed entirely (ADR-19 decision item 4);
until that removal actually lands, `backend/internal/service/dcs_to_dcs.go`'s
`PostPdf` still calls the old `CheckForUntrustedPeers` first, so the
peer-identity Given step below only produces a cryptographically valid
synthetic peer identity — no DB seeding, and no assumption about whether the
OLD allowlist will incidentally also reject it. Trust/deny for the NEW model
is controlled entirely via the orce trust-PDP flow's control surface (see
steps/peer_trust/dcs_trust_pdp_steps.py, Given "the local policy endpoint
(PDP) is running and allows/denies every request") and via the
agreement-credential Given steps below (missing/invalid credential).

Untrusted-peer single-instance testing technique
------------------------------------------------
The did:web challenge-response signature (see backend/internal/service/
dcs_to_dcs.go's PostPdf): the caller signs a fresh `secret_value` with its
private key, and the receiving instance resolves the peer's did:web
identifier to a document URL (`identity.FetchDIDDocument` ->
`DIDWebPath`/`DIDWebDocumentPath`) to fetch the matching public key.

Resolution follows did-method-web, so PATH SEGMENTS ARE PART OF THE
IDENTITY: `did:web:<host>:<suffix>` resolves to
`https://<host>/<suffix>/did.json`, NOT to `<host>`'s
`/.well-known/did.json`. A synthetic peer therefore cannot be minted by
appending an arbitrary suffix to this instance's own DID — that names a
document this instance does not serve, and PostPdf then fails at its very
first step (FetchDIDDocument) long before any trust-gate layer runs.

What DOES yield a self-resolving synthetic peer is letter case: DNS names are
case-insensitive, so flipping the case of one host letter produces a DIFFERENT
DID STRING that reaches the SAME authority (see
`_self_resolving_peer_variant`). Such an identifier resolves to THIS SAME
running instance's own `/.well-known/did.json`, its own
`/.well-known/dcs-agreement-credential.json` and its own signing key, so
layers 1/2 (challenge-response) and layer 3a (agreement credential: valid
signature, issuer resolving to the same target, matching rules hash) all pass
GENUINELY, leaving layer 3b — the PDP — as the only gate under test.

OUTBOUND ONLY. PostPdf's same-peer guard compares did:web identifiers by what
they denote (`identity.SameDIDWeb`), so inbound this identity is exactly what
that guard exists to refuse — "shipping a contract PDF to the same peer is not
allowed", before any trust layer runs. It is used where the ship goes the other
way (`step_given_local_contract_offered_to_peer`), which applies no such guard.
Inbound scenarios take an orce route instead, which is genuinely another
authority: the one in this module (`_orce_synthetic_peer_credentials`) 404s its
agreement credential and so exercises a layer-3a REFUSAL, and the one in
steps/peer_trust/synthetic_trusted_peer.py publishes a credential that verifies
against the running build's rules hash and so leaves the PDP (layer 3b) as the
only gate an inbound scenario still has to pass.

Consequence: this identity is CONSTANT per instance rather than unique per
scenario (uniqueness would require a distinguishing path segment, which is
exactly what resolution now makes meaningful). Nothing here needs a unique
peer — every assertion is scoped by the scenario's own contract DID, and
`RecordDenialIncidentTxDeduped` dedupes on (contract DID, peer DID,
direction), whose contract-DID component is already per-scenario unique.

A peer with a MISSING credential (layer 3a fails) is simulated separately via
the orce synthetic-peer route (`_orce_synthetic_peer_credentials`), one with a
valid-but-wrong-hash credential via a second orce route
(`_orce_mismatch_peer_credentials`), and one whose credential actually verifies
via a third (`synthetic_trusted_peer.publish_trusted_peer`) — this instance
cannot be made to publish a broken credential about itself. A
"signature-invalid" credential remains uncovered and is flagged as an open
point at its scenario rather than faked.

This technique is the natural single-instance extension of the self-peer
simulation used by the contract-state-machine pack (see
steps/template_management/contract_state_machine_steps.py,
`_self_peer_action_credentials`), adapted here to also cover the PostPdf
same-peer guard.
"""

import base64
import hashlib
import jcs
import json
import os
import time
import uuid
from contextlib import contextmanager
from datetime import datetime, timezone

import requests as _requests
from behave import given, then, when

from steps.contract_deployment.dcs_contract_deployment_steps import BDD_TARGET_NAME as _SEEDED_TARGET_NAME
from steps.peer_trust.synthetic_trusted_peer import publish_trusted_peer
from steps.support.api_client import (
    contract_create_url,
    contract_offer_url,
    contract_peer_action_url,
    contract_peer_pdf_url,
    contract_peer_post_sync_url,
    contract_peer_settlement_url,
    contract_retrieve_by_id_url,
    did_document_url,
    get_with_headers,
    hub_shapes_anchors,
    origin_url,
    post_json,
    signature_request_url,
    signature_revoke_url,
    signature_view_url,
)
from steps.support.services.pdf_service import PDFService
from steps.support.signing import wallet_sign
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.template_management.contract_state_machine_steps import (
    _dev_signing_token_dir,
    _did_web_to_hostname,
    _sign_secret_value_with_dev_key,
)


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _own_identity(context):
    """Fetch this instance's own did:web document and derive the matching
    checked-in dev signing key path (see contract_state_machine_steps for the
    port-to-token-dir mapping and its documented limitation to the two
    checked-in dev identities, backend/certs/dev/did-8991.json / did-8992.json)."""
    did_url = did_document_url(context.base_url)
    resp = _requests.get(
        did_url,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, (
        f"could not fetch this instance's own did:web document from "
        f"{did_url}: {resp.status_code} {resp.text}"
    )
    real_did = resp.json().get("id")
    assert real_did, f"own did.json response has no 'id' field: {resp.text}"
    hostname = _did_web_to_hostname(real_did)
    token_dir = _dev_signing_token_dir(hostname)
    return real_did, token_dir


def _self_resolving_peer_variant(real_did: str) -> str:
    """Flip the case of one letter in a did:web identifier's host component, so
    the result is a DIFFERENT DID STRING that resolves to exactly the SAME
    target.

    DNS names are case-insensitive, so "Dcs-a.localhost%3A18080" and
    "dcs-a.localhost%3A18080" reach one and the same host. That is what makes a
    single-instance synthetic peer possible at all now that path segments are
    part of the identity (see module docstring): the peer's did.json, agreement
    credential and rules hash are this instance's own real, self-consistent
    ones, while the identifier is still not the instance's own DID string and so
    clears PostPdf's same-peer guard.

    This used to re-encode a host character as %XX instead. The resolver now
    accepts no percent-escape in the authority except %3A for the port, because
    decoding arbitrary escapes there let an identifier smuggle a path separator
    into the host — so the old trick names a host that is refused outright.
    """
    prefix = "did:web:"
    assert real_did.startswith(prefix), f"not a did:web identifier: {real_did}"
    rest = real_did[len(prefix):]
    host_encoded, _, suffix = rest.partition(":")
    assert host_encoded, f"did:web identifier has empty host component: {real_did}"

    for index, char in enumerate(host_encoded):
        if char.isascii() and char.isalpha():
            flipped = char.upper() if char.islower() else char.lower()
            variant_host = host_encoded[:index] + flipped + host_encoded[index + 1:]
            return prefix + variant_host + (f":{suffix}" if suffix else "")

    raise AssertionError(
        f"host component of {real_did} carries no ASCII letter to vary; a synthetic peer "
        "identity that resolves to this same instance cannot be derived from it"
    )


def _synthetic_peer_credentials(context):
    """Build a syntactically valid, cryptographically genuine did:web peer
    identity that is NOT this instance's own DID string (see module
    docstring) and a matching challenge-response signature over a fresh
    secret_value."""
    real_did, token_dir = _own_identity(context)
    synthetic_did = _self_resolving_peer_variant(real_did)
    secret_value = str(uuid.uuid4())
    signature = _sign_secret_value_with_dev_key(token_dir, secret_value)
    secret_hash = base64.b64encode(signature).decode()
    return synthetic_did, secret_value, secret_hash


def _orce_synthetic_peer_did() -> str:
    """DID of the orce trust-PDP flow's synthetic-peer route
    (deployment/helm/charts/orce/flows/), coordinated with the deployment
    side rather than discovered by this BDD step: it mirrors this instance's
    own did.json (so the did:web challenge-response — layers 1/2 — verifies
    genuinely, since _orce_synthetic_peer_credentials signs with the same
    dev key that mirrored did.json publishes) but its own agreement-
    credential endpoint deliberately 404s (layer 3a genuinely fails, not by
    absence of an endpoint that has since been implemented). The exact DID
    string is a live contract with the deployment side — override via
    BDD_TRUST_PDP_SYNTHETIC_PEER_DID once confirmed/changed.

    A BARE AUTHORITY, deliberately: the flow serves this identity at the host
    root and distinguishes it from the AC5 mismatch identity by Host header,
    not by path. A path segment (e.g. ":synthetic-peer") must not be added —
    under spec-conform did:web resolution it would point every lookup
    (did.json, agreement credential, and the peer API the synchronizer ships
    to) at paths this fixture does not serve."""
    return os.getenv("BDD_TRUST_PDP_SYNTHETIC_PEER_DID", "did:web:dcs-orce%3A1880")


def _orce_synthetic_peer_credentials(context):
    """Like _synthetic_peer_credentials, but the DID's hostname component
    resolves (backend-side) to the orce synthetic-peer route rather than
    back to this instance's own did.json — see _orce_synthetic_peer_did.
    Still signs with THIS instance's own dev key, since that route mirrors
    this instance's own did.json/public key, so the resulting
    challenge-response signature is genuinely valid against it."""
    _real_did, token_dir = _own_identity(context)
    synthetic_did = _orce_synthetic_peer_did()
    secret_value = str(uuid.uuid4())
    signature = _sign_secret_value_with_dev_key(token_dir, secret_value)
    secret_hash = base64.b64encode(signature).decode()
    return synthetic_did, secret_value, secret_hash


def _orce_mismatch_peer_did() -> str:
    """DID of the SECOND orce trust-PDP static synthetic peer (ADR-19 AC5):
    a genuinely, validly signed agreement credential (unlike
    _orce_synthetic_peer_did's route, which 404s) but naming a deliberately
    WRONG termsOfUse.hash — a real rules-hash mismatch, not a same-build
    always-matching pair. Host-header-routed service alias on orce
    (deployment-side, not discovered by this BDD step); override via
    BDD_TRUST_MISMATCH_PEER_DID once the implementer confirms/changes it."""
    return os.getenv("BDD_TRUST_MISMATCH_PEER_DID", "did:web:dcs-orce-mismatch%3A1880")


def _orce_mismatch_peer_credentials(context):
    """Like _orce_synthetic_peer_credentials, but resolves to the SECOND
    static peer (_orce_mismatch_peer_did) — a validly signed credential with
    a wrong rules hash rather than a missing one."""
    _real_did, token_dir = _own_identity(context)
    mismatch_did = _orce_mismatch_peer_did()
    secret_value = str(uuid.uuid4())
    signature = _sign_secret_value_with_dev_key(token_dir, secret_value)
    secret_hash = base64.b64encode(signature).decode()
    return mismatch_did, secret_value, secret_hash


def _minimal_remote_contract_payload(from_peer_did: str, contract_did: str) -> dict:
    """A minimal, schema-valid DCSToDCSContractItem (see
    backend/design/dcs_to_dcs.go) plus empty task/negotiation lists —
    enough to exercise PostSync's trust checks and RemoteCreate path without
    depending on unrelated fields this requirement doesn't cover."""
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    return {
        "contract": {
            "did": contract_did,
            "contract_version": 1,
            "state": "DRAFT",
            "created_by": from_peer_did,
            "created_at": now,
            "updated_at": now,
            "template_did": "urn:uuid:bdd-peer-trust-remote-template",
            "template_version": 1,
            "responsible": {
                "creator": from_peer_did,
                "approvers": [],
                "reviewers": [],
                "negotiators": [],
            },
            "contract_data": {
                "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
                "@type": "dcs:Contract",
            },
            "origin": from_peer_did,
        },
        "review_tasks": [],
        "approval_tasks": [],
        "negotiation_tasks": [],
        "negotiation_items": [],
        "negotiation_decisions": [],
    }


# ---------------------------------------------------------------------------
# Given
# ---------------------------------------------------------------------------


@given("a cryptographically valid peer identity")
def step_given_peer_identity(context):
    """Produces a synthetic but cryptographically genuine did:web peer
    identity (see module docstring) with NO trust decision baked in — under
    ADR-19 that decision is made by the agreement-credential check (layer
    3a) and the PDP (layer 3b), each exercised by its own dedicated Given
    step (this file's "...publishes no agreement credential..." below;
    steps/peer_trust/dcs_trust_pdp_steps.py's PDP-stub Givens)."""
    # The orce route rather than a case-varied copy of this instance's own DID:
    # a variant that differs only in spelling names the SAME host, which the
    # same-peer guard now recognises and refuses ("shipping a contract PDF to the
    # same peer is not allowed") before any trust layer runs. The orce
    # synthetic-peer Service is a genuinely different authority that mirrors this
    # instance's did.json, so the challenge-response stays honestly valid.
    synthetic_did, secret_value, secret_hash = _orce_synthetic_peer_credentials(context)
    context.peer_from_did = synthetic_did
    context.peer_secret_value = secret_value
    context.peer_secret_hash = secret_hash


@given("a cryptographically valid peer whose agreement credential this instance accepts")
def step_given_peer_identity_with_accepted_credential(context):
    """The only synthetic peer that gets PAST layer 3a (see
    steps/peer_trust/synthetic_trusted_peer.py): its agreement credential is
    issued by its own DID, signed with the VC key its own did.json publishes,
    and names the federation rules hash the instance under test currently
    publishes. Every scenario using this Given is therefore testing what its AC
    names — the policy endpoint — and not the credential check in front of it."""
    peer_did, secret_value, secret_hash = publish_trusted_peer(context)
    context.peer_from_did = peer_did
    context.peer_secret_value = secret_value
    context.peer_secret_hash = secret_hash


@given("that peer publishes no agreement credential")
def step_given_peer_no_agreement_credential(context):
    """Uses the orce trust-PDP flow's synthetic-peer route (see
    _orce_synthetic_peer_did/_orce_synthetic_peer_credentials) whose
    agreement-credential endpoint deliberately 404s. Replaces an earlier
    "sign with our own key, resolve back to our own /.well-known endpoint"
    technique — that stopped being an honestly-currently-true "missing
    credential" precondition once this instance's own
    /.well-known/dcs-agreement-credential.json was actually implemented (see
    dcs_agreement_credential_steps.py, AC1/AC2) and started returning 200
    instead of 404.
    """
    synthetic_did, secret_value, secret_hash = _orce_synthetic_peer_credentials(context)
    context.peer_from_did = synthetic_did
    context.peer_secret_value = secret_value
    context.peer_secret_hash = secret_hash


@given('contract "{name}" exists locally, created by this instance')
def step_given_local_contract(context, name):
    ContractService._create_contract_in_draft(context, name)


# ---------------------------------------------------------------------------
# When / Then: POST /peer/contracts/pdf (post_pdf, the CURRENT DCS-to-DCS
# receiving endpoint — ADR-19 AC4, AC7 inbound, AC8, AC9).
# ---------------------------------------------------------------------------


def _post_pdf_payload(context, name: str) -> dict:
    """A genuine PDF (this instance's own real export of contract '{name}',
    honest evidence per the content-verify gate in PostPdf — see
    backend/internal/service/dcs_to_dcs.go) shipped by the synthetic peer
    identity set up by a prior Given step."""
    did, _ = ContractService._contract_data(context, name)
    # The interaction the PDP assertions are scoped to: the trust gate names
    # this contract in its consult, so "was the policy endpoint consulted"
    # can be answered for THIS ship rather than for any earlier one.
    context.pdp_interaction_contract_did = did
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    export = PDFService.export_contract_pdf(context, did, headers=manager_h)
    assert export.status_code == 200, (
        f"could not export contract '{name}' as PDF to build the post_pdf payload: "
        f"{export.status_code} {export.text}"
    )
    return {
        "from_peer_did": context.peer_from_did,
        "contract_iri": did,
        "pdf": base64.b64encode(export.content).decode(),
        "secret_value": context.peer_secret_value,
        "secret_hash": context.peer_secret_hash,
    }


@when('that peer ships contract "{name}"\'s PDF to this instance\'s PostPdf endpoint')
def step_when_peer_ships_pdf(context, name):
    payload = _post_pdf_payload(context, name)
    context.requests_response = post_json(context, contract_peer_pdf_url(context), payload, headers={})


@then("the PDF is rejected because the peer's agreement credential does not verify")
def step_then_pdf_rejected_agreement_credential(context):
    resp = context.requests_response
    assert resp.status_code != 200, (
        f"Expected PostPdf to reject a peer with no valid agreement credential (ADR-19), got 200: "
        f"{resp.text}"
    )
    assert "agreement credential" in resp.text.lower() or "agreement_credential" in resp.text.lower(), (
        "Expected the rejection to name the missing/invalid agreement credential specifically — "
        "not a different failure that happens to also be non-200 (today, before ADR-19's gate is "
        "implemented, PostPdf still rejects this synthetic peer via the OLD trusted_peers "
        f"allowlist check instead — see CheckForUntrustedPeers) — got {resp.status_code}: {resp.text}"
    )


# ---------------------------------------------------------------------------
# Outbound trigger (ADR-19 AC6, AC7 outbound): offering a contract to a peer
# counterparty fires shipContractPDF (dcstodcs.DCSToDCSSynchronizer) once the
# regenerated PDF lands — the trust gate must be consulted THERE too, before
# a real ship attempt is made, per the ADR's mermaid diagram (S0->S2->S3->S5).
# ---------------------------------------------------------------------------


@given('contract "{name}" exists locally, offered to a peer counterparty, created by this instance')
def step_given_local_contract_offered_to_peer(context, name):
    """The counterparty is the SELF-RESOLVING variant (_self_resolving_peer_variant),
    not the orce route the inbound Given uses: the outbound gate is reached only
    by a counterparty whose agreement credential VERIFIES, and the orce route
    deliberately publishes none, so it would be refused by layer 3a
    (AgreementFailure, which is retryable and does leave a sync_fails entry)
    before the PDP under test is ever consulted. Resolving to this instance's own
    did.json and credential makes layers 1/2/3a pass genuinely and leaves the PDP
    as the only gate. Nothing on the outbound path applies the same-peer guard
    that makes this identity unusable INBOUND (PostPdf, see step_given_peer_identity)."""
    context.outbound_peer_did = _self_resolving_peer_variant(_own_identity(context)[0])
    t_did = ContractService._create_approved_template_for_contract(context)
    creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
    create_resp = post_json(
        context,
        contract_create_url(context),
        {"template_did": t_did, "counterparty": context.outbound_peer_did},
        headers=creator_h,
    )
    assert create_resp.status_code == 200, create_resp.text
    c_did = create_resp.json().get("did")

    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
    assert retrieve.status_code == 200, retrieve.text
    updated_at = retrieve.json().get("updated_at")

    offer_resp = post_json(
        context, contract_offer_url(context), {"did": c_did, "updated_at": updated_at}, headers=creator_h
    )
    assert offer_resp.status_code == 200, offer_resp.text

    ContractService._ensure_store(context, "contract_dids", {})
    ContractService._ensure_store(context, "contract_updated_at", {})
    ContractService._ensure_store(context, "contract_seed_headers", {})
    context.contract_dids[name] = c_did
    context.contract_updated_at[name] = updated_at
    context.contract_seed_headers[name] = creator_h
    context.requests_response = offer_resp


@when('this instance ships contract "{name}"\'s PDF towards its peer counterparty')
def step_when_ship_towards_peer(context, name):
    """Offering (the Given above) already fires the async ship
    (contractworkflowengine's outbox -> ContractWorkflowEngine's PDF_REGENERATED/Offer
    event -> DCSToDCSSynchronizer.shipContractPDF). This step just gives that
    asynchronous path time to run before Then-steps inspect its side effects
    (sync_fails row, PAC audit trail, PDP stub)."""
    context.pdp_interaction_contract_did, _ = ContractService._contract_data(context, name)
    time.sleep(5)


@when("that peer posts a full-state sync for a brand-new contract to this instance")
def step_when_post_sync_new_contract(context):
    contract_did = f"did:example:bdd-peer-sync-{uuid.uuid4()}"
    context.peer_sync_contract_did = contract_did
    payload = _minimal_remote_contract_payload(context.peer_from_did, contract_did)
    # Every broadcast must carry the sender's JAdES over the canonical
    # contract representation (DCS-FR-SM-02) — sign with this instance's own
    # key, exactly like the challenge-response secret below.
    jades_payload = _canonical_jades_payload(contract_did, 1, payload["contract"]["contract_data"])
    payload["jades_signature"] = _jades_sign_as_own_instance(context, jades_payload)
    payload["from_peer_did"] = context.peer_from_did
    payload["secret_value"] = context.peer_secret_value
    payload["secret_hash"] = context.peer_secret_hash
    context.requests_response = post_json(context, contract_peer_post_sync_url(context), payload, headers={})


@when('that peer attempts to approve contract "{name}" via the peer action endpoint')
def step_when_peer_action_approve(context, name):
    did, updated_at = ContractService._contract_data(context, name)
    payload = {
        "action": "approve",
        "component": "CONTRACT_WORKFLOW_ENGINE",
        "from_peer_did": context.peer_from_did,
        "payload": {"did": did, "updated_at": updated_at},
        "secret_value": context.peer_secret_value,
        "secret_hash": context.peer_secret_hash,
    }
    context.requests_response = post_json(context, contract_peer_action_url(context), payload, headers={})


@when("the initiator creates a contract with a raw peer DID as reviewer, approver, and negotiator")
def step_when_create_contract_raw_peer_did(context):
    t_did = ContractService._create_approved_template_for_contract(context)
    creator_h = AuthService.get_headers_for_roles(["Contract Creator"])
    # A raw did:web peer identity (this instance's own, fetched from its
    # public did.json) — deliberately NOT a username and NOT any
    # authenticated user's JWT sub: entering a raw peer DID as participant
    # must work without a JWT-sub binding (see
    # frontend/ClientApp/src/utils/participant-selection.ts).
    peer_did = ContractService._local_peer_did(context)
    context.raw_peer_did_used = peer_did
    context.contract_creator_headers = creator_h
    context.requests_response = post_json(
        context,
        contract_create_url(context),
        {
            "template_did": t_did,
            "reviewers": [peer_did],
            "negotiators": [peer_did],
            "approvers": [peer_did],
        },
        headers=creator_h,
    )


# ---------------------------------------------------------------------------
# Then
# ---------------------------------------------------------------------------


def _assert_rejected_for_trust_reason(context):
    """NOTE (ADR-19): this helper predates the agreement-credential/PDP model
    and is kept only for the (currently orphaned — see module docstring)
    post_sync/peer-action Then steps below, which target an endpoint shape
    that no longer exists in the Goa design (backend/design/dcs_to_dcs.go
    only has post_pdf/get_provenance now; fixing that ADR-13 drift is out of
    this ADR-19 BDD pass's scope). The wording below accepts either the OLD
    allowlist rejection message or the NEW agreement-credential/policy-gate
    wording, so it does not itself go stale the day CheckForUntrustedPeers is
    actually removed per ADR-19 decision item 4.
    """
    resp = context.requests_response
    assert resp.status_code != 200, (
        "Expected the request from a cryptographically valid but untrusted peer DID to be "
        f"rejected, got 200: {resp.text}"
    )
    body_text = resp.text.lower()
    assert any(marker in body_text for marker in ("trust", "untrusted", "allow", "polic", "agreement credential")), (
        "Expected the rejection to name the trust gate (allowlist today, agreement-credential/PDP "
        "under ADR-19) as the reason — not a different failure that happens to also be non-200 "
        "(e.g. PostPdf's unrelated same-peer guard, a decode/validation error, or a "
        f"transition-table rejection) — got {resp.status_code}: {resp.text}"
    )


@then("the post_sync request is rejected because the peer is untrusted")
def step_then_post_sync_rejected_untrusted(context):
    _assert_rejected_for_trust_reason(context)


@then("the peer action request is rejected because the peer is untrusted")
def step_then_peer_action_rejected_untrusted(context):
    _assert_rejected_for_trust_reason(context)


@then('the contract "{name}" was not modified by the untrusted peer action')
def step_then_contract_unmodified(context, name):
    did, _ = ContractService._contract_data(context, name)
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=manager_h)
    assert retrieve.status_code == 200, retrieve.text
    actual_state = str(retrieve.json().get("state", "")).upper()
    assert actual_state == "DRAFT", (
        f"Expected contract '{name}' to remain unmodified (DRAFT) after the rejected untrusted "
        f"peer action, got '{actual_state}'"
    )


@then('the contract data is accepted and stored locally with state "{state}"')
def step_then_post_sync_accepted_and_stored(context, state):
    resp = context.requests_response
    assert resp.status_code == 200, (
        f"Expected the trusted peer's post_sync to be accepted, got {resp.status_code}: {resp.text}"
    )
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"])
    retrieve = get_with_headers(
        context, contract_retrieve_by_id_url(context, context.peer_sync_contract_did), headers=manager_h
    )
    assert retrieve.status_code == 200, (
        f"Expected the synced contract to be retrievable locally after post_sync, got "
        f"{retrieve.status_code}: {retrieve.text}"
    )
    actual_state = str(retrieve.json().get("state", "")).upper()
    assert actual_state == state.strip().upper(), (
        f"Expected the locally-stored contract state to be '{state}' after accepting the "
        f"trusted peer's post_sync, got '{actual_state}'"
    )


@then("the contract is created with that raw peer DID recorded as reviewer, approver, and negotiator")
def step_then_raw_peer_did_recorded(context):
    resp = context.requests_response
    assert resp.status_code == 200, (
        "Expected contract creation with a raw peer DID as reviewer/approver/negotiator to "
        f"succeed without a JWT-sub binding check, got {resp.status_code}: {resp.text}"
    )
    c_did = resp.json().get("did")
    assert c_did, f"contract create response has no 'did': {resp.text}"
    retrieve = get_with_headers(
        context, contract_retrieve_by_id_url(context, c_did), headers=context.contract_creator_headers
    )
    assert retrieve.status_code == 200, retrieve.text
    responsible = retrieve.json().get("responsible") or {}
    peer_did = context.raw_peer_did_used
    assert peer_did.startswith("did:web:"), (
        f"expected a raw did:web peer identity, got '{peer_did}'"
    )
    for role_key in ("reviewers", "approvers", "negotiators"):
        assert peer_did in (responsible.get(role_key) or []), (
            f"Expected raw peer DID '{peer_did}' among '{role_key}': {responsible}"
        )


# ---------------------------------------------------------------------------
# Genuine two-instance scenarios (@two-instance)
#
# These require a SECOND real DCS process (instance B) that trusts, and is
# trusted by, instance A, targeting BDD_DCS_BASE_URL_A / BDD_DCS_BASE_URL_B
# instead of the single-instance BDD_DCS_BASE_URL. The runners providing
# that: dev-stack2.sh locally, tests/bdd/scripts/run_bdd_helm.sh (dcs-a /
# dcs-b releases) in CI. If the URLs are unset, the scenarios fail fast
# with an explicit message naming the missing wiring.
# ---------------------------------------------------------------------------


@contextmanager
def _as_instance(context, base_url):
    """Temporarily point context.base_url at a different running DCS
    instance so the existing single-instance ContractService/AuthService
    helpers (which all read context.base_url via steps.support.api_client)
    can be reused against instance A or instance B without duplicating their
    ~80 lines of template/contract setup logic for two-instance scenarios."""
    previous = context.base_url
    context.base_url = base_url
    try:
        yield
    finally:
        context.base_url = previous


@given("instance A and instance B are both running and trust each other")
def step_given_two_instances_running(context):
    # "trust each other" is, under ADR-19, a property of each instance's OWN
    # PDP endpoint (fail-closed authority) plus a matching agreement
    # credential — not a pre-seeded DCS_TRUSTED_PEERS allowlist. The current
    # two-instance runner (tests/bdd/Makefile kind_deploy_b, dev-stack2.sh)
    # still wires the old DCS_TRUSTED_PEERS env var; wiring both instances'
    # DCS_TRUST_PDP_URL at a default-allow Node-RED flow instead is AC11's
    # own open point (see step_given_default_pdp_flow_wired below) and is not
    # assumed to already hold just because this Given step's did.json
    # reachability check passes.
    base_url_a = os.getenv("BDD_DCS_BASE_URL_A", "http://localhost:5173/api").rstrip("/")
    base_url_b = os.getenv("BDD_DCS_BASE_URL_B", "http://localhost:5174/api").rstrip("/")
    assert base_url_a and base_url_b, (
        "BDD_DCS_BASE_URL_A and BDD_DCS_BASE_URL_B must both be set to run this @two-instance "
        "scenario: a second DCS instance reachable and mutually trusting instance A (dev-stack2.sh "
        "locally, tests/bdd/scripts/run_bdd_helm.sh in CI)."
    )
    context.base_url_a = base_url_a
    context.base_url_b = base_url_b

    did_a = _requests.get(did_document_url(base_url_a), timeout=context.http_timeout_seconds)
    assert did_a.status_code == 200, f"instance A did.json unreachable: {did_a.status_code} {did_a.text}"
    did_b = _requests.get(did_document_url(base_url_b), timeout=context.http_timeout_seconds)
    assert did_b.status_code == 200, f"instance B did.json unreachable: {did_b.status_code} {did_b.text}"

    context.peer_did_a = did_a.json().get("id")
    context.peer_did_b = did_b.json().get("id")
    assert context.peer_did_a, f"instance A did.json has no 'id': {did_a.text}"
    assert context.peer_did_b, f"instance B did.json has no 'id': {did_b.text}"


@given("the counterparty is a synthetic peer whose agreement credential names a different rules hash")
def step_given_counterparty_synthetic_peer_mismatched_hash(context):
    """ADR-19 AC5: overrides context.peer_did_b with the SECOND orce static
    synthetic peer (_orce_mismatch_peer_did/_orce_mismatch_peer_credentials)
    — a validly signed agreement credential naming a deliberately WRONG
    termsOfUse.hash, a real rules-hash mismatch rather than requiring a
    whole second deployment BUILD variant. Reformulated as an OUTBOUND
    sender-side refusal (same shape as AC6, reusing its Then-steps below) —
    an earlier draft of this scenario needed a same-build/mismatched-build
    two-instance pair and a receiver-side (inbound) assertion; the outbound
    formulation exercises the identical hash-comparison gate condition
    honestly, without a second build.

    NOTE: like step_given_counterparty_synthetic_peer_no_credential (AC6),
    this runs BEFORE step_when_create_and_offer_cross_instance swaps
    context.base_url to base_url_a, so it inherits the same "only correct if
    BDD_DCS_BASE_URL_A == BDD_DCS_BASE_URL" assumption.
    """
    mismatch_did, _secret_value, _secret_hash = _orce_mismatch_peer_credentials(context)
    context.peer_did_b = mismatch_did


@given("the default trust-PDP Node-RED flow is wired on both instances")
def step_given_default_pdp_flow_wired(context):
    """ADR-19 AC11: the shipped default is DCS_TRUST_PDP_URL pointing at the
    orce chart's Node-RED (deployment/helm/charts/orce/flows/trust-pdp-flow.json,
    now present), answering a bare 200 OK. This Given step's own env-var gate
    stays (the coordinator sets BDD_TRUST_PDP_DEFAULT_FLOW_WIRED=1 once both
    instances' DCS_TRUST_PDP_URL wiring is confirmed live in a given harness
    run) rather than assuming the flow file's mere presence in the chart
    means every currently-running deployment actually has it wired.
    """
    assert os.getenv("BDD_TRUST_PDP_DEFAULT_FLOW_WIRED") == "1", (
        "This scenario requires the default trust-PDP Node-RED flow "
        "(deployment/helm/charts/orce/flows/, DCS_TRUST_PDP_URL=http://dcs-orce:1880/<route>) to "
        "be wired into both instances' deployments. That flow file and the DCS_TRUST_PDP_URL env "
        "wiring do not exist yet (ADR-19 implementation-state table: pending). Set "
        "BDD_TRUST_PDP_DEFAULT_FLOW_WIRED=1 only once they do — open infrastructure point."
    )


@given("the counterparty is a synthetic peer with no valid agreement credential")
def step_given_counterparty_synthetic_peer_no_credential(context):
    """ADR-19 AC6 (sender-side refusal to ship): overrides context.peer_did_b
    — normally instance B's real DID, set by step_given_two_instances_running
    — with the orce trust-PDP flow's synthetic-peer identity (see
    _orce_synthetic_peer_did/_orce_synthetic_peer_credentials in this
    module). AC6 tests instance A's OWN refusal to ship towards a peer whose
    agreement credential does not verify — that only needs a counterparty
    whose credential genuinely fails, not necessarily real instance B (whose
    own agreement-credential endpoint, now implemented, would actually
    verify and defeat the point of this scenario).

    NOTE: this Given step runs BEFORE step_when_create_and_offer_cross_instance
    swaps context.base_url to base_url_a, so _orce_synthetic_peer_credentials
    signs with whatever context.base_url's own dev key is (the single-instance
    default, set in before_all) — the same "only correct if BDD_DCS_BASE_URL_A
    == BDD_DCS_BASE_URL" assumption step_when_create_and_offer_cross_instance
    already documents for the two-instance dev convention.
    """
    synthetic_did, _secret_value, _secret_hash = _orce_synthetic_peer_credentials(context)
    context.peer_did_b = synthetic_did


@when(
    "the initiator on instance A creates and offers a contract with instance B "
    "as counterparty"
)
def step_when_create_and_offer_cross_instance(context):
    with _as_instance(context, context.base_url_a):
        # NOTE: ContractService._create_approved_template_for_contract calls
        # AuthService.get_headers_for_roles(...) internally WITHOUT an
        # explicit api_base, so those internal template-lifecycle calls
        # authenticate against os.getenv("BDD_DCS_BASE_URL") rather than the
        # context.base_url swapped in by _as_instance. This only produces a
        # correct evidence trail if BDD_DCS_BASE_URL_A == BDD_DCS_BASE_URL
        # (i.e. instance A is conventionally "the" default single-instance
        # URL in the two-instance dev setup).
        # Flagging this here rather than silently relying on it: if the
        # two-instance runner ever assigns A a different URL than the
        # single-instance default, this helper needs an api_base-aware
        # variant of ContractService's template setup.
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"], api_base=context.base_url_a)
        # Instance B is the counterparty (ADR-13): the peer the contract is
        # offered to. Review/approval are A's own internal RBAC.
        create_resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": t_did,
                "counterparty": context.peer_did_b,
            },
            headers=creator_h,
        )
        assert create_resp.status_code == 200, create_resp.text
        c_did = create_resp.json().get("did")
        context.cross_instance_contract_did = c_did
        context.cross_instance_creator_headers = creator_h

        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")

        offer_resp = post_json(
            context, contract_offer_url(context), {"did": c_did, "updated_at": updated_at}, headers=creator_h
        )
        context.requests_response = offer_resp
        assert offer_resp.status_code == 200, offer_resp.text


@then("instance A does not ship the PDF and a sync_fails retry entry exists for the cross-instance contract")
def step_then_a_refuses_to_ship_cross_instance(context):
    """Shared by AC5 (agreement credential validly signed but names a wrong
    rules hash — step_given_counterparty_synthetic_peer_mismatched_hash) and
    AC6 (agreement credential does not verify at all —
    step_given_counterparty_synthetic_peer_no_credential): either way,
    instance A must refuse to ship towards a counterparty whose agreement
    credential fails the layer-3a check, recording a RETRYABLE sync_fails
    entry (AgreementFailure) rather than silently dropping the ship — this
    is the DIFFERENT, retryable gate from the PDP's terminal denial (AC8/
    AC10, see dcs_trust_pdp_steps.py's step_then_no_sync_fails_entry)."""
    c_did = context.cross_instance_contract_did
    time.sleep(10)
    cursor = context.db.cursor()
    cursor.execute("SELECT retry_count FROM sync_fails WHERE did = %s", (c_did,))
    row = cursor.fetchone()
    cursor.close()
    assert row is not None, (
        f"Expected a sync_fails retry entry for cross-instance contract {c_did} after instance A "
        "refused to ship towards a peer with an invalid/mismatched agreement credential (ADR-19 "
        "AC5/AC6), got none — today (before ADR-19's agreement-credential check exists) the ship is "
        "instead attempted unconditionally via the OLD trusted_peers/CheckForUntrustedPeers path"
    )


@then("an incident report naming instance A's refusal to ship is recorded in instance A's audit trail")
def step_then_a_incident_recorded(context):
    """Shared by AC5 and AC6 (see step_then_a_refuses_to_ship_cross_instance
    above) — scoped to this scenario's own cross-instance contract DID, same
    reasoning as dcs_trust_pdp_steps.py's contract-scoped incident counts."""
    from steps.peer_trust.dcs_trust_pdp_steps import _count_trust_gate_incidents  # noqa: PLC0415

    c_did = context.cross_instance_contract_did
    with _as_instance(context, context.base_url_a):
        deadline = time.monotonic() + 60
        matching = []
        while time.monotonic() < deadline:
            matching = _count_trust_gate_incidents(context, contract_did=c_did, api_base=context.base_url_a)
            if matching:
                break
            time.sleep(2)
    assert len(matching) == 1, (
        f"Expected exactly one incident report in instance A's own audit trail, scoped to contract "
        f"{c_did}, for its refusal to ship (ADR-19 AC5/AC6), got {len(matching)}: {matching}"
    )


@then("the contract appears on instance B in state {expected} within a few seconds")
def step_then_contract_offered_on_b(context, expected):
    expected = expected.upper()
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)
    deadline = time.monotonic() + 45
    actual_state = None
    last_resp = None
    while time.monotonic() < deadline:
        last_resp = _requests.get(
            f"{context.base_url_b}/contract/retrieve/{context.cross_instance_contract_did}",
            headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        if last_resp.status_code == 200:
            actual_state = str(last_resp.json().get("state", "")).upper()
            if actual_state == expected:
                return
        time.sleep(1)
    assert actual_state == expected, (
        "Expected the contract offered on instance A to appear on its counterparty B as "
        f"{expected} within a few seconds; last observed state: '{actual_state}' (last response: "
        f"{last_resp.status_code if last_resp else 'n/a'} {last_resp.text if last_resp else ''})"
    )


def _close_the_round_on_a(context):
    """OFFERED (or NEGOTIATION) -> SUBMITTED on instance A, by the participant
    that created the contract (ADR-13: each DCS runs its own workflow; a
    counterparty create assigns all RBAC roles to A's own peer).

    Driven on the STATE rather than on a fixed number of submits, the same way
    the instance-B step is. A round that has a redline to fold in stays in
    NEGOTIATION for one extra submit while the merge bumps contract_version
    (submit.go's NEGOTIATION branch), and one submit too many would land in the
    reviewer's branch and be refused for the role — so a caller that reaches
    this after proposing a change cannot pass a fixed count.

    NEGOTIATION -> SUBMITTED is also the transition that SETTLES: it is where
    this instance states it agrees to the document as it stands, and the
    outbox handler behind it signs and ships that statement to every
    counterparty (dcstodcs shipSettlement)."""
    c_did = context.cross_instance_contract_did
    creator_h = context.cross_instance_creator_headers
    with _as_instance(context, context.base_url_a):
        for _ in range(4):
            if _cross_instance_state(context, context.base_url_a) == "SUBMITTED":
                break
            retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
            assert retrieve.status_code == 200, retrieve.text
            resp = post_json(
                context,
                f"{context.base_url_a}/contract/submit",
                {"did": c_did, "updated_at": retrieve.json().get("updated_at")},
                headers=creator_h,
            )
            assert resp.status_code == 200, f"submit failed: {resp.status_code} {resp.text}"
            context.requests_response = resp
    state = _cross_instance_state(context, context.base_url_a)
    assert state == "SUBMITTED", (
        f"expected instance A to close its negotiation round and reach SUBMITTED, got {state!r}"
    )


def _review_and_approve_on_a(context):
    """SUBMITTED -> REVIEWED -> APPROVED on instance A. Review and approval are
    peer-scoped tasks, so the suite's shared role tokens are the right
    callers."""
    c_did = context.cross_instance_contract_did
    with _as_instance(context, context.base_url_a):
        reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"], api_base=context.base_url_a)
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=reviewer_h)
        assert retrieve.status_code == 200, retrieve.text
        review_submit = post_json(
            context,
            f"{context.base_url_a}/contract/submit",
            {"did": c_did, "updated_at": retrieve.json().get("updated_at"), "forward_to": "approval"},
            headers=reviewer_h,
        )
        assert review_submit.status_code == 200, (
            f"reviewer forward-to-approval failed: {review_submit.status_code} {review_submit.text}"
        )

        approver_h = AuthService.get_headers_for_roles(["Contract Approver"], api_base=context.base_url_a)
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=approver_h)
        assert retrieve.status_code == 200, retrieve.text
        approve = post_json(
            context,
            f"{context.base_url_a}/contract/approve",
            {"did": c_did, "updated_at": retrieve.json().get("updated_at")},
            headers=approver_h,
        )
        assert approve.status_code == 200, f"approve failed: {approve.status_code} {approve.text}"
        context.requests_response = approve


@when("instance A drives the contract to APPROVED through its own local workflow")
def step_when_drive_to_approved_locally(context):
    """OFFERED -> NEGOTIATION -> SUBMITTED -> REVIEWED -> APPROVED, entirely
    on instance A."""
    _close_the_round_on_a(context)
    _review_and_approve_on_a(context)


@when("instance A drives the contract to SUBMITTED through its own local workflow")
def step_when_drive_to_submitted_locally(context):
    """The same workflow stopped at the transition that settles, so the
    scenario can act while instance A stands behind the version it just agreed
    to and its reviewer has not yet decided."""
    _close_the_round_on_a(context)


# ---------------------------------------------------------------------------
# Reopening a settled round (contractworkflowengine/command/settledagreement.go
# withdrawOwnSettlement).
#
# The settlement rows themselves are read straight from instance A's database:
# no API exposes them, and the alternative — inferring them from the signing
# gate's answer — cannot be used here, because reaching that gate needs
# APPROVED, which is past both edges that reopen a round. `context.db` is
# instance A's database in the two-instance harness (the same connection the
# sync_fails assertion above reads), so these steps are A-side only by
# construction.
# ---------------------------------------------------------------------------


def _own_settlements_of_the_cross_instance_contract(context):
    """The settlement rows instance A produced for this scenario's contract:
    contract_settlements is keyed by the SETTLING party, so this instance's own
    are the ones whose from_peer_did is its own did:web. The counterparty's
    rows in the same table are the signing gate's evidence and are deliberately
    not read here — withdrawing an agreement must not touch them."""
    cursor = context.db.cursor()
    try:
        cursor.execute(
            "SELECT to_peer_did, document_digest FROM contract_settlements "
            "WHERE did = %s AND from_peer_did = %s",
            (context.cross_instance_contract_did, context.peer_did_a),
        )
        return cursor.fetchall()
    finally:
        cursor.close()
        # psycopg2 opens a transaction for the read and the connection is
        # shared across the whole suite, so it is closed again here rather
        # than left idle-in-transaction across this step's polling loop.
        context.db.rollback()


def _stored_document_digest(context, base_url: str) -> str:
    """The digest a settlement of the document as it currently stands would
    carry: SHA-256 over the RFC 8785 canonicalization, prefixed "sha256:"
    (base/jades.ContractDocumentDigest). Re-derived here from the document the
    API returns rather than read from the row under test, so the assertion is
    that the row names THIS version and not merely that a row exists."""
    body, _ = _cross_instance_contract(context, base_url)
    document = body.get("contract_data")
    if document is None:
        document = {}
    return "sha256:" + hashlib.sha256(jcs.canonicalize(document)).hexdigest()


@then("instance A holds its own settlement of the contract as it stands")
def step_then_a_holds_own_settlement(context):
    """The premise every later step in this scenario rests on. Without it the
    rejection has nothing to withdraw and the redline that follows proves
    nothing: it would be permitted because no agreement was ever recorded,
    not because rejecting took one back.

    Polled, because the settlement is written by the outbox handler that ships
    it and therefore trails the SUBMITTED transition that produced it."""
    expected = _stored_document_digest(context, context.base_url_a)
    rows = []
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        rows = _own_settlements_of_the_cross_instance_contract(context)
        if any(digest == expected for _to_peer, digest in rows):
            return
        time.sleep(2)
    raise AssertionError(
        f"Expected instance A to hold its own settlement of {context.cross_instance_contract_did} "
        f"naming the document it just submitted ({expected}) — the statement its "
        f"NEGOTIATION -> SUBMITTED transition signs and ships to the counterparty. Rows held: {rows}"
    )


@then("instance A holds no settlement of its own for the contract")
def step_then_a_holds_no_own_settlement(context):
    """Withdrawal, read directly: the rejection above took the agreement back
    rather than leaving a statement instance A no longer stands behind. Deleted
    rather than superseded on purpose — between the withdrawal and the next
    submit the truth is that this instance has not settled anything, and the
    signing gate says so."""
    rows = _own_settlements_of_the_cross_instance_contract(context)
    assert rows == [], (
        "Expected the reopened round to have withdrawn instance A's own settlement of "
        f"{context.cross_instance_contract_did}, but it still holds: {rows}"
    )


@when("instance A's reviewer rejects the submission back into negotiation")
def step_when_a_reviewer_rejects(context):
    """SUBMITTED -> NEGOTIATION (submit.go's actionflag.Reject branch): the
    reviewer sends the submission back, which reopens the review, negotiation
    and approval tasks — and undoes the transition that settled."""
    c_did = context.cross_instance_contract_did
    with _as_instance(context, context.base_url_a):
        reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"], api_base=context.base_url_a)
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=reviewer_h)
        assert retrieve.status_code == 200, retrieve.text
        reject = post_json(
            context,
            f"{context.base_url_a}/contract/submit",
            {
                "did": c_did,
                "updated_at": retrieve.json().get("updated_at"),
                "forward_to": "reject",
                "comments": ["Reopening the round to change the document before anyone signs."],
            },
            headers=reviewer_h,
        )
        assert reject.status_code == 200, (
            f"reviewer rejection failed on instance A: {reject.status_code} {reject.text}"
        )
        context.requests_response = reject
    state = _cross_instance_state(context, context.base_url_a)
    assert state == "NEGOTIATION", (
        f"expected the reviewer's rejection to reopen the round on instance A, got state {state!r}"
    )


_REDLINED_CLAUSE_TEXT = "Confidentiality clause, as reworded after the round was reopened"


def _first_clause_content(document: dict):
    """The clause content list of the canonical envelope every contract in this
    pack is instantiated from (TemplateService.canonical_document_data): one
    dcs:Clause block whose dcs:content is an explicit {"@list": [...]} of
    strings. Located rather than assumed at a fixed index, since a reseed may
    have added party and signature-field nodes around it."""
    blocks = ((document or {}).get("dcs:documentStructure") or {}).get("dcs:blocks") or {}
    for block in blocks.get("@list") or []:
        content = (block or {}).get("dcs:content") or {}
        items = content.get("@list")
        if isinstance(items, list) and items and isinstance(items[0], str):
            return content
    raise AssertionError(
        "could not find a clause with textual content in the cross-instance contract document; "
        f"this step redlines the canonical envelope's single clause. Document: {json.dumps(document)[:800]}"
    )


@when("instance A redlines the reopened contract")
def step_when_a_redlines_reopened_contract(context):
    """A structured redline (a change_request carrying contract_data), which
    negotiate.go applies to the document immediately and the regenerator
    re-ships to the counterparty as a fresh PDF (ADR-13).

    This is the call requireUnsettledAgreement gates. It is made by the same
    participant that submitted the round — the negotiation decision it records
    is keyed by participant, and a decision left open under another identity
    would block the submit that closes the new round for a reason this
    scenario is not about."""
    c_did = context.cross_instance_contract_did
    with _as_instance(context, context.base_url_a):
        creator_h = context.cross_instance_creator_headers
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve.status_code == 200, retrieve.text
        body = retrieve.json()
        document = body.get("contract_data")
        _first_clause_content(document)["@list"] = [_REDLINED_CLAUSE_TEXT]
        resp = post_json(
            context,
            f"{context.base_url_a}/contract/negotiate",
            {
                "did": c_did,
                "updated_at": body.get("updated_at"),
                "negotiated_by": AuthService.username_for_roles(["Contract Creator"]),
                "change_request": {"contract_data": document},
            },
            headers=creator_h,
        )
        context.requests_response = resp
    assert resp.status_code == 200, (
        "Expected the reopened round to accept a redline on instance A. A refusal naming the "
        "agreement this instance already made means the rejection above reopened the round without "
        "taking that agreement back (settledagreement.go withdrawOwnSettlement), which leaves the "
        f"party that rejected unable to change anything: {resp.status_code} {resp.text}"
    )


@then("the redlined document reaches instance B within a few seconds")
def step_then_redline_reaches_b(context):
    """The redline travels as a re-rendered PDF over the DCS-to-DCS exchange,
    and instance B has to hold it BEFORE it runs its own round: B settles
    whatever document it holds, and a B that settled the pre-redline version
    would name a digest instance A's settlement never matches — the signing
    gate would then refuse both parties forever, for a divergence this scenario
    did not intend to create."""
    deadline = time.monotonic() + 120
    observed = None
    while time.monotonic() < deadline:
        body, _ = _cross_instance_contract(context, context.base_url_b)
        try:
            observed = _first_clause_content(body.get("contract_data")).get("@list")
        except AssertionError:
            observed = None
        if observed == [_REDLINED_CLAUSE_TEXT]:
            return
        time.sleep(3)
    raise AssertionError(
        f"Expected instance A's redline to reach instance B's copy of "
        f"{context.cross_instance_contract_did} over the PDF exchange, last observed clause "
        f"content: {observed!r}"
    )


# ---------------------------------------------------------------------------
# Revocation propagation across instances (DCS-NFR-BR-06)
# ---------------------------------------------------------------------------


def _run_ceremony_and_sign(context, base_url, label, field_name, given_name, await_settlement=True):
    """Run the full signing attempt for one party of the cross-instance
    contract and return whatever /signature/prepare or /signature/submit
    answered — 200 or a refusal.

    Shared by the two signing steps and by the settlement-gate refusal step:
    the gate under test runs INSIDE prepare, after the ceremony and after the
    state-machine check, so a scenario asserting the refusal has to get all the
    way here and cannot short-cut the ceremony. Reuses the real-signing pack's
    ceremony machinery verbatim — every URL builder reads context.base_url,
    which _as_instance swaps to the instance being driven.

    The settlement gate answers three different things under one API code, and
    they are not waited on alike (apply.go assertCounterpartiesSettled):

    - "this instance has not settled ..." is this instance's OWN bookkeeping,
      written by the outbox handler that ships the settlement, so it trails its
      own SUBMITTED by a moment. Always retried — a step signing right after
      its own approve would otherwise race its own row.
    - "no settlement from ... is held" is the peer's evidence, which travels
      over the DCS-to-DCS channel after the peer's SUBMITTED. Retried only when
      await_settlement is on; scenarios that ASSERT the refusal pass False, and
      it is exactly this answer they mean.
    - "... settled document X, this instance settled Y" is two settlements that
      disagree, which no waiting resolves: PostSettlement refuses a divergent
      artifact at the door, so the stored pair cannot change on its own. Never
      retried — waiting on it would spend the whole window to report the same
      thing.

    A refused prepare persists nothing (the gate is checked before anything
    mutates), so retrying it costs one request.
    """
    from steps.real_signing_vertical.dcs_real_signing_vertical_steps import (  # noqa: PLC0415
        ceremony_aud,
        _build_pid_presentation,
        _complete_ceremony_via_presentation,
        _fetch_pending_nonce,
    )

    with _as_instance(context, base_url):
        c_did = context.cross_instance_contract_did
        signer_h = AuthService.get_headers_for_roles(["Contract Signer"], api_base=base_url)
        start = post_json(
            context,
            signature_request_url(context),
            {"contract_did": c_did, "field_name": field_name},
            headers=signer_h,
        )
        assert start.status_code == 200, (
            f"POST /signature/request failed on instance {label}: {start.status_code} {start.text}"
        )
        ceremony_id = start.json().get("ceremony_id")
        assert ceremony_id, f"/signature/request response has no ceremony_id: {start.text}"

        nonce = _fetch_pending_nonce(context, ceremony_id)
        presentation, _issuer_jwt, _disclosures, subject_did = _build_pid_presentation(
            given_name=given_name, family_name="BDD-Testperson",
            aud=ceremony_aud(context), nonce=nonce,
        )
        completion = _complete_ceremony_via_presentation(
            context, ceremony_id, presentation, subject_did, given_name, "BDD-Testperson",
            poa_organization=field_name, nonce=nonce,
        )
        assert completion.status_code == 200, (
            f"ceremony presentation failed on instance {label}: {completion.status_code} {completion.text}"
        )

        deadline = time.monotonic() + 90
        while True:
            resp = wallet_sign(
                context,
                c_did,
                signer_did=subject_did,
                signatory=given_name,
                field_name=field_name,
                credential_type="AES",
                headers=signer_h,
                ceremony_id=ceremony_id,
            )
            gate_refusal = resp.status_code == 400 and "counterparty_not_settled" in resp.text
            waiting_for_own = gate_refusal and "this instance has not settled" in resp.text
            waiting_for_peer = gate_refusal and "no settlement from" in resp.text
            keep_waiting = waiting_for_own or (await_settlement and waiting_for_peer)
            if not keep_waiting or time.monotonic() > deadline:
                return resp
            time.sleep(3)


@when("instance A applies a ceremony-backed signature to the contract")
def step_when_sign_cross_instance(context):
    # The seeded signature fields name the PARTIES (create.go
    # seedSignatureFields: one field per instance DID), and the ceremony's PoA
    # must authorize exactly the signed party (UC-14) — so the field is A's own
    # peer DID.
    apply_resp = _run_ceremony_and_sign(
        context, context.base_url_a, "A", context.peer_did_a, "PeerRevocation",
    )
    assert apply_resp.status_code == 200, (
        f"wallet signing failed on instance A: {apply_resp.status_code} {apply_resp.text}"
    )
    context.requests_response = apply_resp


@when("instance {label} attempts a ceremony-backed signature on the contract")
def step_when_attempt_signature(context, label):
    """Attempts, not asserts: the signing party is complete on its own side —
    APPROVED, a verified ceremony, a PoA for its own party — so whether this is
    granted is the counterparty's business, which is what the Then step reads.
    The wait for the COUNTERPARTY's settlement is off here (that answer is the
    point of the attempt); the wait for this instance's own settlement row is
    not, so the refusal read below is about the peer and not about this
    instance's outbox still catching up with its own approve."""
    base_url = context.base_url_a if label == "A" else context.base_url_b
    field_name = context.peer_did_a if label == "A" else context.peer_did_b
    given_name = "PeerSettlementGate" if label == "A" else "PeerSettlementGateB"
    context.requests_response = _run_ceremony_and_sign(
        context, base_url, label, field_name, given_name, await_settlement=False,
    )


@then("the signature attempt on instance {label} is refused because the counterparty has not settled")
def step_then_signature_refused_counterparty_not_settled(context, label):
    """The mutual-settlement gate, asserted positively: local APPROVED is not
    enough to sign a federated contract, because APPROVED is this instance's
    own intrinsic state (ADR-13) and says nothing about the counterparty. The
    refusal must be the gate's OWN typed error — counterparty_not_settled is
    the code the frontend routes on ("waiting for the counterparty", not "you
    may not sign") — so matching the code is what makes this scenario fail if
    the refusal ever comes from somewhere else.

    The refusal must also NAME THE COUNTERPARTY. The same code covers "this
    instance has not settled yet", which is only this instance's outbox
    trailing its own approve — a scenario satisfied by that would pass without
    the counterparty ever being consulted."""
    counterparty = context.peer_did_b if label == "A" else context.peer_did_a
    resp = context.requests_response
    assert resp.status_code == 400, (
        f"Expected instance {label} to refuse signing a version its counterparty has not settled, "
        f"got {resp.status_code}: {resp.text}"
    )
    assert "counterparty_not_settled" in resp.text, (
        f"Expected the refusal to be the settlement gate's own error code, got: {resp.text}"
    )
    assert counterparty in resp.text, (
        f"Expected the refusal to name the missing settlement of {counterparty}, not this instance's "
        f"own: {resp.text}"
    )


@then("instance {label} holds an applied signature for its own party field")
def step_then_instance_holds_own_signature(context, label):
    """What the gate lifting looks like: the same signer, on the same
    contract, in the same state, now records a signature — so the refusal
    before it was about the counterparty's settlement and nothing else."""
    base_url = context.base_url_a if label == "A" else context.base_url_b
    field_name = context.peer_did_a if label == "A" else context.peer_did_b
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=base_url)
    view = _requests.get(
        f"{base_url}/signature/view",
        params={"did": context.cross_instance_contract_did},
        headers=manager_h,
        timeout=context.http_timeout_seconds,
    )
    assert view.status_code == 200, f"signature view failed on instance {label}: {view.status_code} {view.text}"
    applied = [
        signature for signature in (view.json().get("signatures") or [])
        if signature.get("field_name") == field_name and signature.get("status") != "REVOKED"
    ]
    assert applied, (
        f"Expected instance {label} to hold its own applied signature for {field_name} once the "
        f"counterparty had settled, got: {view.json()}"
    )


@when("instance A revokes the applied signature of the cross-instance contract")
def step_when_revoke_cross_instance(context):
    with _as_instance(context, context.base_url_a):
        c_did = context.cross_instance_contract_did
        manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_a)
        view = _requests.get(
            signature_view_url(context), params={"did": c_did}, headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        assert view.status_code == 200, f"signature view failed on instance A: {view.status_code} {view.text}"
        signatures = view.json().get("signatures") or []
        assert signatures, f"Expected an applied signature to revoke, got: {view.json()}"
        revoke = post_json(
            context,
            signature_revoke_url(context),
            {
                "did": c_did,
                "signer_did": signatures[0]["signer_did"],
                "reason": "Cross-instance revocation replication",
            },
            headers=manager_h,
        )
        assert revoke.status_code == 200, (
            f"signature revoke failed on instance A: {revoke.status_code} {revoke.text}"
        )
        context.requests_response = revoke


@then('the contract state "{state}" is replicated on both instance A and instance B')
def step_then_state_replicated_both(context, state):
    c_did = context.cross_instance_contract_did
    expected = state.upper()
    for label, base_url in (("A", context.base_url_a), ("B", context.base_url_b)):
        manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=base_url)
        deadline = time.monotonic() + 45
        actual_state = None
        last_resp = None
        while time.monotonic() < deadline:
            last_resp = _requests.get(
                f"{base_url}/contract/retrieve/{c_did}", headers=manager_h,
                timeout=context.http_timeout_seconds,
            )
            if last_resp.status_code == 200:
                actual_state = str(last_resp.json().get("state", "")).upper()
                if actual_state == expected:
                    break
            time.sleep(1)
        assert actual_state == expected, (
            f"Expected contract state {expected} to be replicated on instance {label}, last "
            f"observed state: '{actual_state}' (last response: "
            f"{last_resp.status_code if last_resp else 'n/a'} {last_resp.text if last_resp else ''})"
        )


# ---------------------------------------------------------------------------
# Approval quorum with two distinct approver peers (DCS-FR-CWE-15/25)
# ---------------------------------------------------------------------------


@when("the initiator on instance A creates and offers a contract requiring approval from both instances")
def step_when_create_offer_dual_approver(context):
    with _as_instance(context, context.base_url_a):
        t_did = ContractService._create_approved_template_for_contract(context)
        creator_h = AuthService.get_headers_for_roles(["Contract Creator"], api_base=context.base_url_a)
        # Reviewer and negotiator = A's own peer so the pre-approval drive
        # stays local; approvers = BOTH peers so the quorum needs two
        # observably distinct CauserDIDs (the point of this scenario).
        create_resp = post_json(
            context,
            contract_create_url(context),
            {
                "template_did": t_did,
                "reviewers": [context.peer_did_a],
                "negotiators": [context.peer_did_a],
                "approvers": [context.peer_did_a, context.peer_did_b],
            },
            headers=creator_h,
        )
        assert create_resp.status_code == 200, create_resp.text
        c_did = create_resp.json().get("did")
        context.cross_instance_contract_did = c_did
        context.cross_instance_creator_headers = creator_h

        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
        assert retrieve.status_code == 200, retrieve.text
        updated_at = retrieve.json().get("updated_at")

        offer_resp = post_json(
            context, contract_offer_url(context), {"did": c_did, "updated_at": updated_at}, headers=creator_h
        )
        context.requests_response = offer_resp
        assert offer_resp.status_code == 200, offer_resp.text


@when("instance A drives the contract to the approval stage")
def step_when_drive_to_approval_stage(context):
    c_did = context.cross_instance_contract_did
    with _as_instance(context, context.base_url_a):
        creator_h = context.cross_instance_creator_headers
        submit_payload = {
            "did": c_did,
            "reviewers": [context.peer_did_a],
            "approvers": [context.peer_did_a, context.peer_did_b],
            "negotiators": [context.peer_did_a],
        }
        # OFFERED -> NEGOTIATION -> SUBMITTED: two creator submits (A is the
        # sole negotiator and there are no open negotiation decisions, same
        # pattern as the single-instance state-machine pack).
        for _ in range(2):
            retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=creator_h)
            assert retrieve.status_code == 200, retrieve.text
            submit_payload["updated_at"] = retrieve.json().get("updated_at")
            resp = post_json(context, f"{context.base_url_a}/contract/submit", submit_payload, headers=creator_h)
            assert resp.status_code == 200, f"submit failed: {resp.status_code} {resp.text}"

        # SUBMITTED -> REVIEWED: reviewer forwards to approval.
        reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"], api_base=context.base_url_a)
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=reviewer_h)
        assert retrieve.status_code == 200, retrieve.text
        review_submit = post_json(
            context,
            f"{context.base_url_a}/contract/submit",
            {"did": c_did, "updated_at": retrieve.json().get("updated_at"), "forward_to": "approval"},
            headers=reviewer_h,
        )
        assert review_submit.status_code == 200, (
            f"reviewer forward-to-approval failed: {review_submit.status_code} {review_submit.text}"
        )


def _approve_from_instance(context, base_url):
    c_did = context.cross_instance_contract_did
    creator_h = context.cross_instance_creator_headers
    approver_h = AuthService.get_headers_for_roles(["Contract Approver"], api_base=base_url)
    # Always read the authoritative updated_at from A (the origin) — B's
    # replica catches up asynchronously (same convention as the
    # APPROVED-replication scenario).
    retrieve = get_with_headers(
        context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h
    )
    assert retrieve.status_code == 200, retrieve.text
    resp = post_json(
        context,
        f"{base_url}/contract/approve",
        {"did": c_did, "updated_at": retrieve.json().get("updated_at")},
        headers=approver_h,
    )
    context.requests_response = resp
    assert resp.status_code == 200, f"approve via {base_url} failed: {resp.status_code} {resp.text}"


@when("instance A's approver approves the contract")
def step_when_approver_a_approves(context):
    _approve_from_instance(context, context.base_url_a)


@when("instance B's approver approves the contract")
def step_when_approver_b_approves(context):
    _approve_from_instance(context, context.base_url_b)


@then("the contract is still not APPROVED because instance B's required approval is open")
def step_then_partial_quorum_holds(context):
    c_did = context.cross_instance_contract_did
    creator_h = context.cross_instance_creator_headers
    retrieve = get_with_headers(
        context, f"{context.base_url_a}/contract/retrieve/{c_did}", headers=creator_h
    )
    assert retrieve.status_code == 200, retrieve.text
    state = str(retrieve.json().get("state", "")).upper()
    assert state != "APPROVED", (
        "Quorum violation: the contract reached APPROVED after only instance A's approval, "
        "although instance B's approval task must still be OPEN (approve.go AnyTasksInState guard)"
    )
    assert state == "REVIEWED", (
        f"Expected the contract to remain in REVIEWED awaiting instance B's approval, got '{state}'"
    )


@then("both peers' approval decisions are recorded on the contract's approval tasks")
def step_then_both_approvals_recorded(context):
    # GET /contract/retrieve lists the approval tasks assigned to the VIEWING
    # instance's own peer DID, so each peer's recorded decision is asserted
    # against its own instance. B's task state arrives via the async
    # post_sync broadcast from A (the origin) — poll briefly.
    c_did = context.cross_instance_contract_did
    for label, base_url, peer_did in (
        ("A", context.base_url_a, context.peer_did_a),
        ("B", context.base_url_b, context.peer_did_b),
    ):
        approver_h = AuthService.get_headers_for_roles(["Contract Approver"], api_base=base_url)
        states = {}
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            resp = get_with_headers(context, f"{base_url}/contract/retrieve", headers=approver_h)
            assert resp.status_code == 200, (
                f"contract retrieve on instance {label} failed: {resp.status_code} {resp.text}"
            )
            tasks = [t for t in (resp.json().get("approval_tasks") or []) if t.get("did") == c_did]
            states = {t.get("approver"): str(t.get("state", "")).upper() for t in tasks}
            if states.get(peer_did) == "APPROVED":
                break
            time.sleep(1)
        assert states.get(peer_did) == "APPROVED", (
            f"Expected instance {label}'s own approval task (approver={peer_did}) to be "
            f"recorded APPROVED on instance {label}, got tasks: {states}"
        )


# ---------------------------------------------------------------------------
# JAdES sync provenance (DCS-FR-SM-02)
# ---------------------------------------------------------------------------


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def _b64url_decode(segment: str) -> bytes:
    return base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4))


def _canonical_jades_payload(contract_did: str, contract_version: int, contract_document: dict) -> bytes:
    """The canonical contract representation the backend signs
    (internal/base/jades.BuildContractPayload): RFC 8785 JSON
    Canonicalization Scheme."""
    payload = {
        "dcs:contractDid": contract_did,
        "dcs:contractVersion": contract_version,
        "dcs:contractDocument": contract_document,
    }
    return jcs.canonicalize(payload)


def _der_to_jose(der: bytes) -> bytes:
    """Convert an ASN.1 DER ECDSA signature (what hsmsign emits, mirroring
    DIDDocument.Sign) into the 64-byte r||s form JWS ES256 requires."""
    assert der[0] == 0x30, "expected a DER SEQUENCE"
    idx = 2
    if der[1] & 0x80:
        idx = 2 + (der[1] & 0x7F)
    assert der[idx] == 0x02, "expected DER INTEGER (r)"
    rlen = der[idx + 1]
    r = der[idx + 2 : idx + 2 + rlen]
    idx = idx + 2 + rlen
    assert der[idx] == 0x02, "expected DER INTEGER (s)"
    slen = der[idx + 1]
    s = der[idx + 2 : idx + 2 + slen]
    r = r.lstrip(b"\x00").rjust(32, b"\x00")
    s = s.lstrip(b"\x00").rjust(32, b"\x00")
    return r + s


def _own_x5c(context):
    """The certificate chain of the verification method that publishes the key
    _sign_secret_value_with_dev_key actually signs with.

    Which method that is gets PROBED, not assumed by position: a did.json
    publishes several keys (instance signer, credential issuance, key
    agreement) in no promised order, and the backend pairs its own signer with
    a method by matching the key rather than by index too
    (identity.methodHoldingKey). A chain taken from any other method would make
    every JAdES built here fail signature verification — a rejection that looks
    like the one under test but is the harness's."""
    from cryptography.hazmat.primitives import hashes  # noqa: PLC0415
    from cryptography.hazmat.primitives.asymmetric import ec  # noqa: PLC0415

    _real_did, token_dir = _own_identity(context)
    did_url = did_document_url(context.base_url)
    resp = _requests.get(did_url, timeout=context.http_timeout_seconds)
    assert resp.status_code == 200, f"could not fetch own did.json: {resp.status_code} {resp.text}"
    methods = resp.json().get("verificationMethod") or []
    assert methods, "own did.json has no verificationMethod"

    probe = str(uuid.uuid4())
    signature = _sign_secret_value_with_dev_key(token_dir, probe)
    for method in methods:
        jwk = method.get("publicKeyJwk") or {}
        x5c = jwk.get("x5c") or []
        if isinstance(x5c, str):
            x5c = [x5c]
        if not x5c or not jwk.get("x") or not jwk.get("y"):
            continue
        numbers = ec.EllipticCurvePublicNumbers(
            int.from_bytes(_b64url_decode(jwk["x"]), "big"),
            int.from_bytes(_b64url_decode(jwk["y"]), "big"),
            ec.SECP256R1(),
        )
        try:
            numbers.public_key().verify(signature, probe.encode(), ec.ECDSA(hashes.SHA256()))
        except Exception:  # noqa: BLE001 — any failure means this is not the signer's method
            continue
        return x5c

    raise AssertionError(
        f"no verification method in {did_url} publishes the key this harness signs with — the "
        f"instance's did.json and its HSM token have drifted apart: {[m.get('id') for m in methods]}"
    )


def _jades_sign_as_own_instance(context, payload_bytes: bytes) -> str:
    """Produce a genuine JAdES baseline-B compact JWS with this instance's
    own dev/HSM key and x5c chain — the same trick the synthetic-peer
    challenge-response signature uses (the synthetic DID resolves to this
    instance's own did.json and key)."""
    _real_did, token_dir = _own_identity(context)
    header = {
        "alg": "ES256",
        "typ": "jose",
        "cty": "application/json",
        "x5c": _own_x5c(context),
        "sigT": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "crit": ["sigT"],
    }
    signing_input = _b64url(json.dumps(header, separators=(",", ":")).encode()) + "." + _b64url(payload_bytes)
    der = _sign_secret_value_with_dev_key(token_dir, signing_input)
    return signing_input + "." + _b64url(_der_to_jose(der))


@when("that peer posts a full-state sync whose JAdES signature covers a different contract document")
def step_when_post_sync_tampered_jades(context):
    """The challenge-response secret and trust listing are VALID here — only
    the JAdES payload binding is wrong (it signs a different contract
    document than the one being synced), so a rejection can only come from
    the receiver's JAdES payload check (DCS-FR-SM-02)."""
    contract_did = f"did:example:bdd-peer-sync-{uuid.uuid4()}"
    context.peer_sync_contract_did = contract_did
    payload = _minimal_remote_contract_payload(context.peer_from_did, contract_did)
    tampered_document = {"@type": "dcs:Contract", "dcs:name": "a different document than the synced one"}
    jades_payload = _canonical_jades_payload(contract_did, 1, tampered_document)
    payload["jades_signature"] = _jades_sign_as_own_instance(context, jades_payload)
    payload["from_peer_did"] = context.peer_from_did
    payload["secret_value"] = context.peer_secret_value
    payload["secret_hash"] = context.peer_secret_hash
    context.requests_response = post_json(context, contract_peer_post_sync_url(context), payload, headers={})


# ---------------------------------------------------------------------------
# Settlement artifacts (POST /peer/contracts/settlement) — the evidence the
# signing gate reads. The binding under test is the DOCUMENT DIGEST: a
# settlement is a statement about one version of one document, and
# contract_version cannot carry that across the boundary (it is a per-instance
# counter — the sender bumps it on merging a redline, the receiver on every
# inbound ship).
# ---------------------------------------------------------------------------


def _canonical_settlement_payload(
    contract_did: str, contract_version: int, document_digest: str,
    settled_by: str, settled_with: str, settled_at: str,
) -> bytes:
    """The canonical settlement artifact the backend signs and re-derives
    (internal/base/jades.BuildSettlementPayload): RFC 8785. settled_at is
    written at second precision because the receiver re-formats the parsed
    timestamp as RFC3339Nano — which drops a zero fraction — and compares the
    bytes."""
    return jcs.canonicalize({
        "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
        "@type": "dcs:ContractSettlement",
        "dcs:contractDid": contract_did,
        "dcs:contractVersion": contract_version,
        "dcs:contractDocumentDigest": document_digest,
        "dcs:settledBy": settled_by,
        "dcs:settledWith": settled_with,
        "dcs:settledAt": settled_at,
    })


@when("instance A ships instance B a settlement naming a document instance B does not hold")
def step_when_ship_settlement_for_another_document(context):
    """Everything about this artifact is genuine except the version it covers:
    it is signed with instance A's own key, by instance A's own identity,
    towards instance B, for a contract B holds and a party B knows — only the
    document digest names something else. So the refusal can come from nothing
    but the digest binding."""
    digest = "sha256:" + hashlib.sha256(b"a contract document instance B never held").hexdigest()
    context.foreign_settlement_digest = digest

    body, _manager_h = _cross_instance_contract(context, context.base_url_a)
    with _as_instance(context, context.base_url_a):
        _real_did, token_dir = _own_identity(context)
        secret_value = str(uuid.uuid4())
        secret_hash = base64.b64encode(_sign_secret_value_with_dev_key(token_dir, secret_value)).decode()
        payload = _canonical_settlement_payload(
            contract_did=context.cross_instance_contract_did,
            contract_version=int(body.get("contract_version") or 1),
            document_digest=digest,
            settled_by=context.peer_did_a,
            settled_with=context.peer_did_b,
            settled_at=datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        )
        settlement_jades = _jades_sign_as_own_instance(context, payload)

    with _as_instance(context, context.base_url_b):
        context.requests_response = post_json(
            context,
            contract_peer_settlement_url(context),
            {
                "from_peer_did": context.peer_did_a,
                "contract_iri": context.cross_instance_contract_did,
                "secret_value": secret_value,
                "secret_hash": secret_hash,
                "settlement_jades": settlement_jades,
            },
            headers={},
        )


@then("instance B refuses the settlement because it covers another document")
def step_then_settlement_refused_other_document(context):
    resp = context.requests_response
    assert resp.status_code == 400, (
        f"Expected instance B to refuse a settlement covering a document it does not hold, got "
        f"{resp.status_code}: {resp.text}"
    )
    assert context.foreign_settlement_digest in resp.text, (
        "Expected the refusal to name the document the settlement covers — the digest binding is the "
        f"whole point of the artifact, and no other check reports it. Got: {resp.text}"
    )


@then("the post_sync request is rejected because the JAdES payload does not match")
def step_then_post_sync_rejected_jades(context):
    resp = context.requests_response
    assert resp.status_code == 400, (
        f"Expected post_sync with a mismatching JAdES payload to be rejected with 400, got "
        f"{resp.status_code}: {resp.text}"
    )
    assert "jades" in resp.text.lower(), (
        f"Expected the rejection to name the JAdES check, got: {resp.text}"
    )


@then("instance B stores a JAdES sync-provenance artifact for that contract signed by instance A")
def step_then_provenance_on_b(context):
    """GET /peer/contracts/provenance on instance B (DCS-FR-SM-02): the
    stored artifact must be a structurally valid JAdES baseline-B compact
    JWS from instance A whose payload binds exactly the synced contract.
    (Cryptographic verification already happened server-side — the sync
    would have been rejected otherwise; see the tampered-JAdES scenario.)"""
    c_did = context.cross_instance_contract_did
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=context.base_url_b)
    resp = None
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        resp = _requests.get(
            f"{context.base_url_b}/peer/contracts/provenance",
            params={"did": c_did},
            headers=manager_h,
            timeout=context.http_timeout_seconds,
        )
        if resp.status_code == 200:
            break
        time.sleep(1)
    assert resp is not None and resp.status_code == 200, (
        f"Expected instance B to store sync provenance for {c_did}, got "
        f"{resp.status_code if resp else 'n/a'}: {resp.text if resp else ''}"
    )
    body = resp.json()
    assert body.get("did") == c_did
    assert body.get("from_peer_did") == context.peer_did_a, (
        f"Expected the provenance to name instance A ({context.peer_did_a}) as signer, got: "
        f"{body.get('from_peer_did')}"
    )
    jws = body.get("jades_signature") or ""
    parts = jws.split(".")
    assert len(parts) == 3, f"Expected a compact JWS with three segments, got: {jws[:120]}"

    header = json.loads(_b64url_decode(parts[0]))
    assert header.get("alg") == "ES256", f"Expected alg ES256, got: {header.get('alg')}"
    assert header.get("sigT"), "Expected a sigT claimed-signing-time header"
    assert header.get("crit") == ["sigT"], f"Expected crit [sigT], got: {header.get('crit')}"
    assert header.get("x5c"), "Expected an x5c certificate chain in the protected header"

    payload = json.loads(_b64url_decode(parts[1]))
    assert payload.get("dcs:contractDid") == c_did, (
        f"Expected the JAdES payload to bind contract {c_did}, got: {payload.get('dcs:contractDid')}"
    )
    assert payload.get("dcs:contractVersion") == body.get("contract_version"), (
        "Expected the JAdES payload's version to match the stored provenance version"
    )
    assert "dcs:contractDocument" in payload, "Expected the JAdES payload to embed the contract document"


# ---------------------------------------------------------------------------
# Federated deployment gate (DCS-NFR-BR-03)
#
# The seeded signature fields name the PARTIES (create.go seedSignatureFields:
# one field per instance DID), and each party's signature row stays in its own
# database. The deploy gate therefore satisfies a counterparty's field from the
# evidence the instance actually holds for it — the JAdES that peer ships with
# its own signed copy — and refuses while no such artifact exists, on the
# manual endpoint and on the auto-deploy subscriber alike.
# ---------------------------------------------------------------------------


def _instance_target_id(context, base_url: str) -> str:
    """The SEEDED registry entry for the shipped ORCE contract-target flow on ONE
    named instance (values.bdd.yml / values.bdd2.yml contractTargets), by name.

    Not registered here on the fly. A target registered through the API holds no
    credential until one is issued for it, and authorizeCaller refuses a callback
    from a target with no credential (ADR-27) — a target this suite invented
    could dispatch but never acknowledge, so its contract would stay SIGNED. The
    seeded entry carries the oauth_client_id its instance's Hydra and
    systemClients both declare, which is what lets the acknowledgement land.

    Resolved per instance: the single-instance helper caches one id on the
    context, which is wrong across two instances — a target registered on A does
    not exist on B, and the two entries point the flow's callback at different
    deployments."""
    cache = getattr(context, "peer_target_ids", None)
    if cache is None:
        cache = {}
        context.peer_target_ids = cache
    if base_url in cache:
        return cache[base_url]
    admin_h = AuthService.get_headers_for_roles(["Sys. Administrator"], api_base=base_url)
    listed = _requests.get(
        f"{base_url}/contract/targets", headers=admin_h, timeout=context.http_timeout_seconds
    )
    assert listed.status_code == 200, f"could not list contract targets on {base_url}: {listed.text}"
    entries = listed.json() or []
    for entry in entries:
        if entry.get("name") == _SEEDED_TARGET_NAME:
            cache[base_url] = entry["id"]
            return entry["id"]
    raise AssertionError(
        f"{base_url} has no seeded contract target {_SEEDED_TARGET_NAME!r} — deploy this instance with a "
        f"contractTargets entry carrying an oauth_client_id, or its deployments can never be acknowledged. "
        f"Registered: {[e.get('name') for e in entries]}"
    )


def _cross_instance_contract(context, base_url: str):
    c_did = context.cross_instance_contract_did
    manager_h = AuthService.get_headers_for_roles(["Contract Manager"], api_base=base_url)
    retrieve = _requests.get(
        f"{base_url}/contract/retrieve/{c_did}", headers=manager_h, timeout=context.http_timeout_seconds
    )
    assert retrieve.status_code == 200, f"could not read {c_did} on {base_url}: {retrieve.text}"
    return retrieve.json(), manager_h


def _designate_target(context, base_url: str):
    body, manager_h = _cross_instance_contract(context, base_url)
    resp = post_json(
        context,
        f"{base_url}/contract/target/designate",
        {
            "did": context.cross_instance_contract_did,
            "updated_at": body.get("updated_at"),
            "target_id": _instance_target_id(context, base_url),
        },
        headers=manager_h,
    )
    assert resp.status_code == 200, (
        f"could not designate a target system on {base_url}: {resp.status_code} {resp.text}"
    )


def _cross_instance_state(context, base_url: str) -> str:
    body, _ = _cross_instance_contract(context, base_url)
    return str(body.get("state", "")).upper()


def _deploy_cross_instance(context, base_url: str):
    body, manager_h = _cross_instance_contract(context, base_url)
    return post_json(
        context,
        f"{base_url}/contract/deploy",
        {"did": context.cross_instance_contract_did, "updated_at": body.get("updated_at")},
        headers=manager_h,
    )


@when("instance {label} points the cross-instance contract at its own target system")
def step_when_designate_target_on_instance(context, label):
    base_url = context.base_url_a if label == "A" else context.base_url_b
    _designate_target(context, base_url)


# The counterparty identity instance B runs its own workflow under. A BDD
# identity is only (roles, organization), and the participant it resolves to
# (the organization — auth/oid4vp/verify.go sets ParticipantDID from it) is
# what the open-decision check compares against: HasOpenNegotiationDecisions
# excludes a pending decision on a change request the CALLER itself authored
# and nobody else's, so the negotiate that opens the round and the submit that
# closes it have to come from one and the same participant. A dedicated
# organization keeps that pairing out of the suite-shared role tokens, whose
# state other scenarios rely on.
_B_COUNTERPARTY_ORG = "BDD Peer Trust Counterparty"

# Contract Manager is the scope /contract/negotiate grants the responder of an
# inbound offer (design/contract_workflow_engine.go); Contract Negotiator is
# the local role submit.go's NEGOTIATION branch accepts. One token carries
# both, so every call of the round is made by one participant.
_B_COUNTERPARTY_ROLES = ["Contract Manager", "Contract Negotiator"]


def _post_on_b_with_fresh_updated_at(context, path: str, payload: dict, headers: dict, what: str):
    """POST a mutating contract command to instance B, reading updated_at from
    B's OWN copy immediately beforehand. B's copy moves without B acting — the
    peer's ships land on it asynchronously — so a value read earlier in the
    step can already be behind the lost-update guard by the time the call is
    made; a guard refusal is answered by re-reading rather than by failing the
    scenario on a race it is not testing."""
    c_did = context.cross_instance_contract_did
    last = None
    for _ in range(3):
        retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, c_did), headers=headers)
        assert retrieve.status_code == 200, f"could not read {c_did} on instance B: {retrieve.text}"
        body = dict(payload, did=c_did, updated_at=retrieve.json().get("updated_at"))
        last = post_json(context, f"{context.base_url_b}{path}", body, headers=headers)
        if last.status_code == 200:
            return last
        if "updated elsewhere" not in last.text.lower():
            break
        time.sleep(2)
    raise AssertionError(f"{what} failed on instance B: {last.status_code} {last.text}")


@when("instance B drives its own copy of the contract to APPROVED through its own local workflow")
def step_when_drive_to_approved_on_b(context):
    """B holds an inbound OFFER, not a draft of its own, and runs its OWN
    workflow on it (ADR-13): OFFERED -> NEGOTIATION -> SUBMITTED -> REVIEWED ->
    APPROVED.

    Leaving OFFERED is where B differs from A. /contract/submit is the
    CREATOR's path — submit.go's DRAFT/OFFERED branch requires the caller to be
    the contract's creator, and receivepdf.go records the ORIGIN peer as
    CreatedBy on a received copy, so no local user of B can ever be it. The
    responder's path is /contract/negotiate: transition.go declares the
    Offered -> Negotiation edge for exactly this, and negotiate.go derives the
    authority for an inbound offer (Origin != localPeer) from being the
    designated counterparty rather than from a local negotiator task.

    The change request is FREE TEXT on purpose. A structured redline is applied
    to contract_data immediately and re-shipped as a fresh PDF (negotiate.go),
    which would rewrite the document instance A has already signed — the very
    thing this scenario measures. Free text decodes into no ChangeRequest, so
    it is recorded for the negotiation audit trail and changes nothing.

    From NEGOTIATION on, the received copy is fully equipped for B's own
    workflow — receivepdf.go assigns B's peer DID to the reviewer, approver and
    negotiator tasks — so the tail is the same submit / submit / review /
    approve sequence instance A runs, and the one the two-instance Playwright
    vertical drives through the UI (multi-dcs-helpers settleToApprovedOn).
    """
    with _as_instance(context, context.base_url_b):
        counterparty_h = AuthService.get_headers_for_roles(
            _B_COUNTERPARTY_ROLES,
            api_base=context.base_url_b,
            organization=_B_COUNTERPARTY_ORG,
        )
        _post_on_b_with_fresh_updated_at(
            context,
            "/contract/negotiate",
            {
                "negotiated_by": AuthService.username_for_roles(_B_COUNTERPARTY_ROLES),
                "change_request": "Reviewed on the counterparty side; the offer is accepted as it stands.",
            },
            counterparty_h,
            "opening the negotiation on the received offer",
        )

        # Submit, from the participant that opened the round, until the round
        # closes: the first submit closes B's negotiation task and folds the
        # round into contract_version + 1, leaving the contract in NEGOTIATION;
        # the next finds no negotiation against the new version and advances to
        # SUBMITTED (submit.go's NEGOTIATION branch). Driving on the state
        # rather than a fixed count keeps this right if a peer ship bumps the
        # version in between and the first submit already finds nothing to
        # merge — one submit too many would land in the reviewer's branch and
        # be refused for the role.
        for _ in range(4):
            if _cross_instance_state(context, context.base_url_b) != "NEGOTIATION":
                break
            _post_on_b_with_fresh_updated_at(context, "/contract/submit", {}, counterparty_h, "submit")
        state = _cross_instance_state(context, context.base_url_b)
        assert state == "SUBMITTED", (
            f"expected instance B's copy to close its negotiation round and reach SUBMITTED, got {state!r}"
        )

        # Review and approval are peer-scoped tasks (IsValidReviewer /
        # IsValidApprover check the instance DID, not the participant), so the
        # suite's shared role tokens are the right callers for them.
        reviewer_h = AuthService.get_headers_for_roles(["Contract Reviewer"], api_base=context.base_url_b)
        _post_on_b_with_fresh_updated_at(
            context,
            "/contract/submit",
            {"forward_to": "approval"},
            reviewer_h,
            "reviewer forward-to-approval",
        )

        approver_h = AuthService.get_headers_for_roles(["Contract Approver"], api_base=context.base_url_b)
        approve = _post_on_b_with_fresh_updated_at(context, "/contract/approve", {}, approver_h, "approve")
        context.requests_response = approve


@when("instance B applies a ceremony-backed signature to the contract")
def step_when_countersign_on_b(context):
    """The counterparty's own signature, on its own copy, for its OWN seeded
    field (its peer DID) — the signature A's database will never hold a row
    for."""
    apply_resp = _run_ceremony_and_sign(
        context, context.base_url_b, "B", context.peer_did_b, "PeerCountersignature",
    )
    assert apply_resp.status_code == 200, (
        f"wallet signing failed on instance B: {apply_resp.status_code} {apply_resp.text}"
    )
    context.requests_response = apply_resp


@then("a manual deployment of the cross-instance contract on instance {label} is rejected because signing is incomplete")
def step_then_cross_instance_deploy_rejected(context, label):
    base_url = context.base_url_a if label == "A" else context.base_url_b
    resp = _deploy_cross_instance(context, base_url)
    assert resp.status_code == 400, (
        f"Expected instance {label} to refuse deploying a contract its counterparty has not signed, "
        f"got {resp.status_code}: {resp.text}"
    )
    assert "incomplete" in resp.text.lower(), (
        f"Expected the refusal to name the incomplete signing workflow: {resp.text}"
    )


@then("the cross-instance contract on instance {label} does not activate while the counterparty has not signed")
def step_then_no_auto_activation(context, label):
    """The auto-deploy subscriber (DCS-FR-CWE-06) fires on this instance's own
    APPLIED_SIGNATURE and runs the same gate. The contract designates a target,
    so an ungated deployment would have been dispatched and the target's
    acknowledgement would have moved it to ACTIVE — staying SIGNED is what says
    the gate held on that path too."""
    base_url = context.base_url_a if label == "A" else context.base_url_b
    assert _cross_instance_state(context, base_url) == "SIGNED", (
        f"Expected instance {label} to hold its own signature and be SIGNED before this is asserted, got "
        f"{_cross_instance_state(context, base_url)!r}"
    )
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        state = _cross_instance_state(context, base_url)
        assert state != "ACTIVE", (
            f"Instance {label} activated a contract its counterparty has never signed (DCS-NFR-BR-03)"
        )
        time.sleep(3)


@then("the cross-instance contract on instance {label} activates automatically once both parties have signed")
def step_then_auto_activation(context, label):
    base_url = context.base_url_a if label == "A" else context.base_url_b
    state = None
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        state = _cross_instance_state(context, base_url)
        if state == "ACTIVE":
            return
        time.sleep(3)
    raise AssertionError(
        f"Expected the auto-deploy subscriber on instance {label} to deploy the countersigned contract "
        f"and the target's acknowledgement to move it to ACTIVE within 120s, state is still {state!r}"
    )


@then("a manual deployment of the cross-instance contract on instance {label} is accepted once the counterparty has countersigned")
def step_then_cross_instance_deploy_accepted(context, label):
    """The countersignature reaches this instance as the JAdES the peer ships
    with its signed copy, so poll until that ship has landed rather than racing
    it."""
    base_url = context.base_url_a if label == "A" else context.base_url_b
    resp = None
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        resp = _deploy_cross_instance(context, base_url)
        if resp.status_code == 200:
            body = resp.json()
            assert body.get("correlation_id"), f"Expected a dispatched deployment, got: {body}"
            return
        time.sleep(3)
    raise AssertionError(
        f"Expected instance {label} to deploy the countersigned contract, got "
        f"{resp.status_code if resp else 'n/a'}: {resp.text if resp else ''}"
    )
