"""BDD steps for the Contract Storage & Archive endpoints (UC-07,
backend/design/contract_storage_archive.go): /archive/retrieve,
/archive/search, /archive/audit, /archive/delete. Archive-entry
creation/evidence content is covered by
05_contract_deployment/contract_deployment.feature — this module only
exercises the archive endpoints themselves.
"""

import time

from behave import given, then, when

from steps.support.api_client import (
    pac_audit_timeline,
    pac_audit_url,
    archive_annotate_url,
    archive_audit_url,
    archive_delete_url,
    archive_retrieve_url,
    archive_search_url,
    archive_statistics_url,
    delete_with_params,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService


@when("the Archive Manager retrieves the archive")
def step_when_archive_manager_retrieves(context):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = get_with_headers(context, archive_retrieve_url(context), headers=headers)


@when('the Archive Manager searches the archive with state filter "{state}"')
def step_when_archive_manager_searches(context, state):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"state": state},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when("I attempt to retrieve the archive with my current role")
def step_when_attempt_retrieve_archive(context):
    headers = getattr(context, "headers", {})
    context.requests_response = get_with_headers(context, archive_retrieve_url(context), headers=headers)


@when("the Auditor retrieves the archive audit log")
def step_when_auditor_retrieves_archive_audit(context):
    headers = AuthService.get_headers_for_roles(["Auditor"])
    context.requests_response = get_with_headers(context, archive_audit_url(context) + "?justification=BDD%20archive%20audit%20review", headers=headers)


def _contract_dids_in_response(context, name):
    body = context.requests_response.json()
    entries = body.get("contracts") if isinstance(body, dict) else body
    assert isinstance(entries, list), f"Expected a list of archive entries, got: {body}"
    did, _ = ContractService._contract_data(context, name)
    return entries, did


@then('the archive retrieval result includes contract "{name}"')
def step_then_archive_retrieval_includes(context, name):
    entries, did = _contract_dids_in_response(context, name)
    dids = [e.get("did") for e in entries if isinstance(e, dict)]
    assert did in dids, f"Expected archive retrieval to include contract '{name}' (did={did}), got dids: {dids}"


@then('the archive search result includes contract "{name}"')
def step_then_archive_search_includes(context, name):
    entries, did = _contract_dids_in_response(context, name)
    dids = [e.get("did") for e in entries if isinstance(e, dict)]
    assert did in dids, f"Expected archive search to include contract '{name}' (did={did}), got dids: {dids}"


@then("the archive audit log is a non-empty list")
def step_then_archive_audit_nonempty(context):
    # The audit trail is anchored asynchronously by the outbox processor
    # (~1s poll interval, see conf.OutboxProcessorTimeOut) — a contract that
    # reached SIGNED (and so wrote its archive-store event) moments before
    # this call may not be anchored yet. Same re-trigger-and-poll convention
    # as steps/audit_compliance/dcs_process_audit_steps.py's
    # step_then_audit_response_includes_contract.
    headers = AuthService.get_headers_for_roles(["Auditor"])
    body = []
    deadline = time.monotonic() + 90
    while True:
        body = context.requests_response.json()
        if isinstance(body, list) and len(body) > 0:
            return
        if time.monotonic() > deadline:
            break
        time.sleep(2)
        context.requests_response = get_with_headers(context, archive_audit_url(context) + "?justification=BDD%20archive%20audit%20review", headers=headers)
        assert context.requests_response.status_code == 200, (
            f"archive audit re-trigger failed: {context.requests_response.status_code} "
            f"{context.requests_response.text}"
        )
    assert isinstance(body, list) and len(body) > 0, (
        f"Expected /archive/audit to return a non-empty list of audit log entries, got: {body}"
    )


def _archive_store_url(context) -> str:
    # /archive/store has no helper in steps/support/api_client.py yet; the
    # route is POST /archive/store (backend/design/contract_storage_archive.go).
    return f"{context.base_url}/archive/store"


@when("the Archive Manager stores a contract in the archive")
def step_when_archive_manager_stores(context):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = post_json(context, _archive_store_url(context), {}, headers=headers)


@when("I attempt to store a contract in the archive with my current role")
def step_when_attempt_store_archive(context):
    headers = getattr(context, "headers", {})
    context.requests_response = post_json(context, _archive_store_url(context), {}, headers=headers)


@when('the Archive Manager deletes the archived contract "{name}" with justification "{justification}"')
def step_when_archive_manager_deletes(context, name, justification):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = delete_with_params(
        context, archive_delete_url(context), {"did": did, "justification": justification}, headers=headers
    )


@when('I attempt to delete the archived contract "{name}" with my current role')
def step_when_attempt_delete_archive(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = getattr(context, "headers", {})
    context.requests_response = delete_with_params(
        context, archive_delete_url(context), {"did": did, "justification": "BDD unauthorized deletion attempt"}, headers=headers
    )


@then('the archive deletion of contract "{name}" is recorded in the archive audit log')
def step_then_archive_deletion_audited(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    event_types_for_did = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        # Workflow events (delete/annotate) live in the PAC audit trail under
        # the CONTRACT_STORAGE_ARCHIVE component; /archive/audit serves the
        # archive-integrity view (entries + notary-chain checks).
        resp = post_json(
            context,
            pac_audit_url(context),
            {"scope": "CONTRACT_STORAGE_ARCHIVE", "justification": "BDD archive audit review"},
            headers=headers,
        )
        assert resp.status_code == 200, f"Archive audit query failed: {resp.status_code} {resp.text}"
        event_types_for_did = [
            str(entry.get("event_type", "")).upper()
            for entry in pac_audit_timeline(resp)
            if entry.get("did") == did
        ]
        if "DELETE_ARCHIVED_CONTRACT" in event_types_for_did:
            return
        time.sleep(2)
    assert "DELETE_ARCHIVED_CONTRACT" in event_types_for_did, (
        f"Expected a DELETE_ARCHIVED_CONTRACT audit event for contract '{name}' ({did}), "
        f"got event types for this DID: {event_types_for_did}"
    )


@when('the Archive Manager searches the archive with full-text query "{query}"')
def step_when_archive_fulltext_search(context, query):
    """DCS-FR-CSA-13: /archive/search?contract_data=... queries the stored
    tsvector over the whole contract JSON-LD (search_vector), not the
    name/description metadata columns."""
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"contract_data": query},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when('the Archive Manager searches the archive by tag "{tag}"')
def step_when_archive_tag_search(context, tag):
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"tag": tag},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@given('contract "{name}" has counterparty "{party_did}" set directly in the database (party-search test seam)')
def step_given_contract_counterparty(context, name, party_did):
    """No API path assigns a distinct counterparty DID in the single-instance
    BDD flow, so the responsible JSONB's counterparty slot is seeded via the
    shared test DB connection — the same accepted seam the expiry-window
    step below uses."""
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute(
        "UPDATE contracts SET responsible = jsonb_set(COALESCE(responsible, '{}'::jsonb), '{counterparty}', to_jsonb(%s::text)) WHERE did = %s",
        (party_did, did),
    )
    context.db.commit()
    cursor.close()


@given('contract "{name}" has validity period "{start}" to "{end}" set directly in the database (validity-search test seam)')
def step_given_contract_validity_period(context, name, start, end):
    """No API path sets start_date/exp_date after creation, so the validity
    period is seeded via the shared test DB connection — the same accepted
    seam the expiry-window step below uses."""
    did, _ = ContractService._contract_data(context, name)
    cursor = context.db.cursor()
    cursor.execute(
        "UPDATE contracts SET start_date = %s, exp_date = %s WHERE did = %s",
        (start, end, did),
    )
    context.db.commit()
    cursor.close()


@when('the Archive Manager searches the archive by party "{party_did}"')
def step_when_archive_party_search(context, party_did):
    """DCS-FR-CSA-10/-13: /archive/search?party=... matches the DID against
    the contract's creator or counterparty in the responsible JSONB."""
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"party": party_did},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@when('the Archive Manager searches the archive with validity period from "{valid_from}" until "{valid_until}"')
def step_when_archive_validity_search(context, valid_from, valid_until):
    """DCS-FR-CSA-10/-13: /archive/search?valid_from=...&valid_until=...
    bounds the contract validity period (start_date >= valid_from,
    exp_date <= valid_until)."""
    import requests as _requests  # noqa: PLC0415

    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = _requests.get(
        archive_search_url(context),
        params={"valid_from": valid_from, "valid_until": valid_until},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )


@then('the archive search result does not include contract "{name}"')
def step_then_archive_search_excludes(context, name):
    entries, did = _contract_dids_in_response(context, name)
    dids = [e.get("did") for e in entries if isinstance(e, dict)]
    assert did not in dids, (
        f"Expected archive search to NOT include contract '{name}' (did={did}), got dids: {dids}"
    )


def _annotate_archived_contract(context, name, payload_extra, headers):
    did, _ = ContractService._contract_data(context, name)
    payload = {"did": did, **payload_extra}
    context.requests_response = post_json(context, archive_annotate_url(context), payload, headers=headers)


@when('the Archive Manager annotates the archived contract "{name}" with summary "{summary}" and tags "{tags}"')
def step_when_archive_manager_annotates(context, name, summary, tags):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    _annotate_archived_contract(
        context, name, {"summary": summary, "tags": tags.split(",")}, headers
    )


@given('the Archive Manager annotates the archived contract "{name}" with summary "{summary}" and tags "{tags}"')
def step_given_archive_manager_annotates(context, name, summary, tags):
    # Given-position variant (setup for the tag-search scenario): a failure
    # here is a broken precondition, so assert success immediately.
    step_when_archive_manager_annotates(context, name, summary, tags)
    assert context.requests_response.status_code == 200, (
        f"Annotating archived contract '{name}' as a scenario precondition failed: "
        f"{context.requests_response.status_code} {context.requests_response.text}"
    )


@when('the Archive Manager annotates the archived contract "{name}" with tags "{tags}" and no summary')
def step_when_archive_manager_annotates_no_summary(context, name, tags):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    _annotate_archived_contract(context, name, {"tags": tags.split(",")}, headers)


@when('I attempt to annotate the archived contract "{name}" with my current role')
def step_when_attempt_annotate_archive(context, name):
    headers = getattr(context, "headers", {})
    _annotate_archived_contract(
        context, name, {"summary": "BDD unauthorized annotation attempt"}, headers
    )


def _archive_entry_for(context, name):
    """Fetch the archive entry for the named contract via a DID-filtered
    archive search, so the assertion reads what the API serves (not what the
    annotate call echoed back)."""
    import requests as _requests  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    resp = _requests.get(
        archive_search_url(context),
        params={"did": did},
        headers=headers,
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"archive search for '{name}' failed: {resp.status_code} {resp.text}"
    entries = resp.json()
    assert isinstance(entries, list) and len(entries) > 0, (
        f"Expected an archive entry for contract '{name}' (did={did}), got: {entries}"
    )
    return entries[0]


@then('the archive entry for contract "{name}" carries summary "{summary}" and tags "{tags}"')
def step_then_archive_entry_annotation(context, name, summary, tags):
    entry = _archive_entry_for(context, name)
    assert entry.get("archive_summary") == summary, (
        f"Expected archive_summary {summary!r}, got: {entry.get('archive_summary')!r}"
    )
    expected_tags = tags.split(",")
    assert entry.get("archive_tags") == expected_tags, (
        f"Expected archive_tags {expected_tags}, got: {entry.get('archive_tags')}"
    )


@then('the archive entry for contract "{name}" carries a generated summary derived from its version and state')
def step_then_archive_entry_generated_summary(context, name):
    # No summary was supplied, so the system derives one from the archived
    # contract's own metadata (name, version, state, creator). The contract's
    # stored name is its template-derived title, so the deterministic anchors
    # to assert are the version and lifecycle state.
    entry = _archive_entry_for(context, name)
    generated = entry.get("archive_summary") or ""
    # The contract reached SIGNED; signing completion auto-deploys to ACTIVE
    # (DCS-FR-CWE-06/SM-12), so by archive time the summary's state token is
    # legitimately either. The invariant is that the derived summary names the
    # version and the current lifecycle state.
    assert "version" in generated.lower() and ("SIGNED" in generated or "ACTIVE" in generated), (
        f"Expected a metadata-derived summary naming the version and state, got: {generated!r}"
    )


@then('the archive annotation of contract "{name}" is recorded in the archive audit log')
def step_then_archive_annotation_audited(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Auditor"])
    event_types_for_did = []
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        # Workflow events (delete/annotate) live in the PAC audit trail under
        # the CONTRACT_STORAGE_ARCHIVE component; /archive/audit serves the
        # archive-integrity view (entries + notary-chain checks).
        resp = post_json(
            context,
            pac_audit_url(context),
            {"scope": "CONTRACT_STORAGE_ARCHIVE", "justification": "BDD archive audit review"},
            headers=headers,
        )
        assert resp.status_code == 200, f"Archive audit query failed: {resp.status_code} {resp.text}"
        event_types_for_did = [
            str(entry.get("event_type", "")).upper()
            for entry in pac_audit_timeline(resp)
            if entry.get("did") == did
        ]
        if "ANNOTATE_ARCHIVED_CONTRACT" in event_types_for_did:
            return
        time.sleep(2)
    assert "ANNOTATE_ARCHIVED_CONTRACT" in event_types_for_did, (
        f"Expected an ANNOTATE_ARCHIVED_CONTRACT audit event for contract '{name}' ({did}), "
        f"got event types for this DID: {event_types_for_did}"
    )


# ---------------------------------------------------------------------------
# Archive dashboard statistics (DCS-FR-CSA-21)
# ---------------------------------------------------------------------------


@given('contract "{name}" is set to expire in {days:d} days directly in the database (expiry-window test seam)')
def step_given_contract_expires_soon(context, name, days):
    """The UI has no expiration editor yet (CSA-23 surface) and contract
    update only accepts EventUpdate from Draft, so the expiry window is
    seeded via the shared test DB connection — the same accepted seam
    steps/contract_deployment/dcs_contract_deployment_steps.py's
    expiry-date step uses."""
    from datetime import datetime, timedelta, timezone  # noqa: PLC0415

    did, _ = ContractService._contract_data(context, name)
    expires = datetime.now(timezone.utc) + timedelta(days=days)
    cursor = context.db.cursor()
    cursor.execute("UPDATE contracts SET exp_date = %s WHERE did = %s", (expires, did))
    context.db.commit()
    cursor.close()


@when("the Archive Manager retrieves the archive statistics")
def step_when_archive_manager_retrieves_statistics(context):
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    context.requests_response = get_with_headers(context, archive_statistics_url(context), headers=headers)


@when("I attempt to retrieve the archive statistics with my current role")
def step_when_attempt_retrieve_statistics(context):
    headers = getattr(context, "headers", {})
    context.requests_response = get_with_headers(context, archive_statistics_url(context), headers=headers)


@then("the archive statistics count at least one archived contract with positive storage volume")
def step_then_statistics_counts_archive(context):
    body = context.requests_response.json()
    assert body.get("archived_total", 0) >= 1, f"expected at least one archive entry: {body}"
    assert body.get("archived_contracts", 0) >= 1, f"expected at least one archived contract: {body}"
    assert body.get("storage_bytes", 0) > 0, f"expected a positive storage volume: {body}"


@then("the archive statistics report a compliant archive entry")
def step_then_statistics_compliant(context):
    body = context.requests_response.json()
    assert body.get("compliant_total", 0) >= 1, (
        f"expected at least one compliant entry (snapshot + content hash + signature "
        f"metadata + TSA receipt): {body}"
    )


@then('the archive statistics list contract "{name}" as expiring')
def step_then_statistics_expiring(context, name):
    did, _ = ContractService._contract_data(context, name)
    body = context.requests_response.json()
    expiring = [entry.get("did") for entry in body.get("expiring_contracts", [])]
    assert did in expiring, f"expected {did} in expiring contracts, got: {expiring}"


@then('the archive statistics do not list contract "{name}" as expiring')
def step_then_statistics_not_expiring(context, name):
    did, _ = ContractService._contract_data(context, name)
    body = context.requests_response.json()
    expiring = [entry.get("did") for entry in body.get("expiring_contracts", [])]
    assert did not in expiring, (
        f"expected {did} NOT to be listed as expiring (outside the configured "
        f"window), got: {expiring}"
    )


@then('the archive statistics include a recent archive action for contract "{name}"')
def step_then_statistics_recent_action(context, name):
    """The archive audit trail is anchored asynchronously (outbox -> IPFS),
    so the statistics are re-read until the contract's own archive action
    surfaces — the same polling convention every audit-trail assertion in
    this suite uses."""
    did, _ = ContractService._contract_data(context, name)
    headers = AuthService.get_headers_for_roles(["Archive Manager"])
    deadline = time.time() + 90
    actions = []
    while time.time() < deadline:
        response = get_with_headers(context, archive_statistics_url(context), headers=headers)
        if response.status_code == 200:
            actions = response.json().get("recent_actions", [])
            matching = [action for action in actions if action.get("did") == did]
            if matching:
                assert all(action.get("actor") for action in matching), (
                    f"recent actions must name their actor: {matching}"
                )
                return
        time.sleep(3)
    raise AssertionError(f"expected a recent archive action for {did} within 90s, got: {actions}")
