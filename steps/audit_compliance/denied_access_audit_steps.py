"""Access-decision assertions against the real API and disposable BDD database."""

import json
import uuid
from datetime import datetime

from behave import given, then, when

from steps.support.services.access_attempt_service import AccessAttemptService
from steps.support.services.auth_service import AuthService


def _rows(context, sql, params=()):
    cursor = context.db.cursor()
    try:
        cursor.execute(sql, params)
        return cursor.fetchall()
    finally:
        cursor.close()


def _reason():
    return "BDD access audit " + str(uuid.uuid4())


@given('an access audit probe authenticated as "{role}" for scope "{scope}"')
def prepare_probe(context, role, scope):
    context.access_probe_headers = AuthService.get_headers_for_roles([role])
    token = context.access_probe_headers["Authorization"].split(" ", 1)[1]
    context.access_probe_actor = AuthService.decode_jwt_payload(token)["sub"]
    context.access_probe_role = role
    context.access_probe_payload = {
        "scope": scope, "did": "urn:uuid:" + str(uuid.uuid4()), "justification": _reason(),
    }


@when('the access audit probe requests "{action}"')
def request_probe(context, action):
    context.access_probe_action = "audit" if action == "audit" else "audit_report"
    context.requests_response = AccessAttemptService.denied_request(
        context, action, context.access_probe_headers, context.access_probe_payload,
    )


def _probe_rows(context):
    return _rows(context, """
        SELECT attempt_by, roles, attempted_at, service, method, scope, did,
               justification, outcome, success
        FROM access_attempts WHERE justification = %s ORDER BY id
    """, (context.access_probe_payload["justification"],))


def _assert_probe(context, row):
    actor, roles, timestamp, service, method, scope, did, reason, outcome, success = row
    assert actor == context.access_probe_actor, row
    assert context.access_probe_role in json.loads(roles), row
    assert isinstance(timestamp, datetime), row
    assert service == "ProcessAuditAndCompliance", row
    assert method == context.access_probe_action, row
    assert scope == context.access_probe_payload["scope"], row
    assert did == context.access_probe_payload["did"], row
    assert reason == context.access_probe_payload["justification"], row
    assert outcome == "DENIED" and success is True, row


@then("exactly one DENIED access attempt records the probe metadata and successful credentials")
def one_denial(context):
    rows = _probe_rows(context)
    assert len(rows) == 1, rows
    _assert_probe(context, rows[0])


@given("the credential failure count is remembered")
def remember_failures(context):
    context.access_failure_count = _rows(
        context, "SELECT COUNT(*) FROM access_attempts WHERE success = FALSE",
    )[0][0]


@when('the access audit probe requests "{action}" three times')
def repeated_probe(context, action):
    context.access_probe_statuses = []
    for _ in range(3):
        request_probe(context, action)
        context.access_probe_statuses.append(context.requests_response.status_code)


@then("each probe request is forbidden")
def all_forbidden(context):
    assert context.access_probe_statuses == [403, 403, 403], context.access_probe_statuses


@then("the credential failure count is unchanged")
def unchanged_failures(context):
    assert _rows(context, "SELECT COUNT(*) FROM access_attempts WHERE success = FALSE")[0][0] == context.access_failure_count


@then("every probe attempt has DENIED outcome and successful credentials")
def all_denied(context):
    rows = _probe_rows(context)
    assert len(rows) == 3, rows
    for row in rows:
        _assert_probe(context, row)


@when("I request access attempts with a unique justification")
def read_attempts(context):
    context.access_read_reason = _reason()
    context.requests_response = AccessAttemptService.read(
        context, context.headers, justification=context.access_read_reason, limit=2,
    )


@when("the System Auditor requests access attempts with a unique justification")
def machine_read(context):
    context.headers = {"Authorization": "Bearer " + context.system_client_token}
    read_attempts(context)


@then("the access-attempt response exposes no attempt list")
def no_list(context):
    assert "attempts" not in context.requests_response.json(), context.requests_response.text


@when('I request access attempts with "{purpose}" justification')
def invalid_purpose(context, purpose):
    params = {"limit": 2}
    if purpose != "missing":
        params["justification"] = {"empty": "", "blank": "   "}[purpose]
    context.requests_response = AccessAttemptService.read(context, context.headers, **params)


@when("the Auditor reads the first access-attempt page with limit 2")
def first_page(context):
    context.headers = AuthService.get_headers_for_roles(["Auditor"])
    read_attempts(context)
    assert context.requests_response.status_code == 200, context.requests_response.text
    context.access_first_page = context.requests_response.json()


def _key(attempt):
    return (datetime.fromisoformat(attempt["attempted_at"].replace("Z", "+00:00")), int(attempt["id"]))


@then("the access-attempt page is bounded and ordered newest first")
def bounded_page(context):
    page = context.access_first_page
    rows = page["attempts"]
    assert len(rows) == 2, page
    assert _key(rows[0]) > _key(rows[1]), page
    assert page.get("next_cursor"), page
    assert all("success" not in row and "ip_address" not in row for row in rows), rows
    assert all(isinstance(row["roles"], list) for row in rows), rows


@then("the successful access-attempt read is logged with its justification")
def read_logged(context):
    rows = _rows(context, """
        SELECT attempt_by, method, outcome, success FROM access_attempts
        WHERE service = 'ProcessAuditAndCompliance' AND justification = %s
    """, (context.access_read_reason,))
    token = context.headers["Authorization"].split(" ", 1)[1]
    actor = AuthService.decode_jwt_payload(token)["sub"]
    assert rows == [(actor, "access_attempts", "SUCCESS", True)], rows


@when("the Auditor follows the access-attempt cursor")
def next_page(context):
    context.requests_response = AccessAttemptService.read(
        context, context.headers, justification=_reason(), limit=2,
        cursor=context.access_first_page["next_cursor"],
    )
    assert context.requests_response.status_code == 200, context.requests_response.text


@then("the next page continues strictly before the previous page without duplicates")
def stable_page(context):
    previous = context.access_first_page["attempts"]
    rows = context.requests_response.json()["attempts"]
    assert 1 <= len(rows) <= 2, rows
    assert not {row["id"] for row in previous}.intersection(row["id"] for row in rows), rows
    assert all(_key(row) < _key(previous[-1]) for row in rows), rows
    keys = [_key(row) for row in rows]
    assert keys == sorted(keys, reverse=True), rows
    expected = _rows(context, """
        SELECT id FROM access_attempts
        WHERE (attempted_at, id) < (%s, %s)
        ORDER BY attempted_at DESC, id DESC LIMIT 2
    """, (_key(previous[-1])[0], previous[-1]["id"]))
    assert [int(row["id"]) for row in rows] == [row[0] for row in expected], (rows, expected)


@given("the access-attempt sequence is remembered")
def remember_sequence(context):
    context.access_sequence = _rows(context, "SELECT COALESCE(MAX(id), 0) FROM access_attempts")[0][0]


@then("new invalid-credential attempts are FAILED and unsuccessful")
def invalid_credentials_logged(context):
    rows = _rows(context, "SELECT outcome, success FROM access_attempts WHERE id > %s", (context.access_sequence,))
    assert rows, "No credential failure was recorded"
    assert all(row == ("FAILED", False) for row in rows), rows
