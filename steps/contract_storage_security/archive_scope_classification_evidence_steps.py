"""Bindings for the archive snapshot, party classification, organization scope and evidence scenarios."""

import base64
import hashlib
from datetime import datetime

from behave import given, then, when

from steps.support.services.contract_service import ContractService
from steps.support.services.archive_scope_classification_evidence_service import ArchiveScopeClassificationEvidenceService as Service
from steps.support.services.template_service import TemplateService


@given('a dedicated contract "{name}" has reached SIGNED')
def signed_contract(context, name):
    Service.signed_contract(context, name)


@given('the Observer remembers the archived version of contract "{name}"')
def remember_archive(context, name):
    context.archive_entry = Service.archive_entry(context, name)
    response = Service.archive_detail(context, name, context.archive_entry["contract_version"])
    assert response.status_code == 200, response.text
    context.frozen_snapshot = response.json()
    cursor = context.db.cursor()
    try:
        did, _ = ContractService._contract_data(context, name)
        cursor.execute("SELECT content_hash FROM contract_archive_entries WHERE did=%s AND contract_version=%s", (did, context.archive_entry["contract_version"]))
        context.snapshot_hash = cursor.fetchone()[0]
    finally:
        cursor.close()


@given('the live contract "{name}" advances to a newer version through the database fixture seam')
def advance_live(context, name):
    Service.advance_live_version(context, name)


@when('the Observer opens the remembered archived version of contract "{name}"')
def open_archive(context, name):
    context.requests_response = Service.archive_detail(context, name, context.archive_entry["contract_version"])


@then('the archive detail is the unchanged snapshot for contract "{name}" with matching identity and hash')
def archive_unchanged(context, name):
    response = context.requests_response
    assert response.status_code == 200, response.text
    snapshot = response.json()
    did, _ = ContractService._contract_data(context, name)
    assert snapshot == context.frozen_snapshot
    assert snapshot["did"] == did
    assert snapshot["contract_version"] == context.archive_entry["contract_version"]
    assert Service.snapshot_hash(snapshot) == context.snapshot_hash
    current = Service.get(context, f"/contract/retrieve/{did}", ["Contract Observer"], context.contract_organizations[name])
    assert current.status_code == 200, current.text
    assert current.json()["contract_version"] > snapshot["contract_version"]
    assert current.json()["name"] != snapshot["name"]


@when('the Observer requests a non-archived version of contract "{name}"')
def missing_archive_version(context, name):
    context.requests_response = Service.archive_detail(context, name, context.archive_entry["contract_version"] + 1)


@given('template "{name}" is created as "{template_type}" with party classification "{party_type}"')
def classified_template(context, name, template_type, party_type):
    Service.classified_template(context, name, party_type, template_type)


@when('the template "{name}" is edited without a party field and copied')
def edit_and_copy(context, name):
    context.copied_did = Service.update_and_copy_template(context, name)


@then('template "{name}", its history and its copy retain "{template_type}" and "{party_type}"')
def preserved_party(context, name, template_type, party_type):
    did = TemplateService.named(context, name)["did"]
    headers = Service.headers(context, ["Template Creator"])
    bodies = [TemplateService.fetch_template(context, candidate, headers=headers) for candidate in (did, context.copied_did)]
    history = Service.get(context, f"/template/history/{did}", ["Template Creator"])
    assert history.status_code == 200, history.text
    assert isinstance(history.json(), list) and history.json(), history.text
    bodies.extend(history.json())
    for body in bodies:
        assert body["party_type"] == party_type, body
        assert body["template_type"] == template_type, body


@when('templates "{first}", "{second}" and "{third}" are published to the catalogue')
def publish_templates(context, first, second, third):
    for name in (first, second, third):
        Service.publish_classified_template(context, name)


@when('the {surface} search combines name "{name}" and party classification "{party_type}"')
def combined_search(context, surface, name, party_type):
    assert surface in ("local", "catalogue"), surface
    path = "/template/search" if surface == "local" else "/catalogue/template/search"
    role = "Template Manager" if surface == "local" else "Contract Creator"
    context.requests_response = Service.get(context, path, [role], name=name, party_type=party_type, offset=1 if surface == "local" else 0, limit=100)


@then('the search includes template "{included}" and excludes "{wrong_party}" and "{wrong_name}"')
def combined_results(context, included, wrong_party, wrong_name):
    response = context.requests_response
    assert response.status_code == 200, response.text
    body = response.json()
    items = body.get("items") if isinstance(body, dict) else body
    assert isinstance(items, list), body
    dids = {item["did"] for item in items}
    assert TemplateService.named(context, included)["did"] in dids, body
    for name in (wrong_party, wrong_name):
        assert TemplateService.named(context, name)["did"] not in dids, body


@given('contracts "{own}" and "{foreign}" have open local review, approval and negotiation tasks through the database fixture seam')
def seed_tasks(context, own, foreign):
    from steps.support.services.archive_scope_classification_evidence_service import seed_open_tasks

    seed_open_tasks(context, (own, foreign))


@when('the organization of contract "{name}" retrieves the contract dashboard')
def dashboard(context, name):
    context.requests_response = Service.get(context, "/contract/retrieve", ["Contract Reviewer", "Contract Approver", "Contract Negotiator"], context.contract_organizations[name], offset=1, limit=1000)


@then('the dashboard includes contract "{own}" and its open tasks but excludes "{foreign}" and its tasks')
def scoped_dashboard(context, own, foreign):
    response = context.requests_response
    assert response.status_code == 200, response.text
    own_did, _ = ContractService._contract_data(context, own)
    foreign_did, _ = ContractService._contract_data(context, foreign)
    for field in ("contracts", "review_tasks", "approval_tasks", "negotiation_tasks"):
        items = response.json()[field] or []
        dids = {item["did"] for item in items}
        assert own_did in dids, (field, items)
        assert foreign_did not in dids, (field, items)


@then('the organization of contract "{own}" sees it in contract, archive and signing searches but cannot see "{foreign}"')
def scoped_lists(context, own, foreign):
    from steps.support.services.archive_scope_classification_evidence_service import assert_scoped_lists

    assert_scoped_lists(context, own, foreign)


@when('a Contract Manager from the organization of "{own}" requests "{path}" for contract "{target}"')
def indirect_read(context, own, path, target):
    did, _ = ContractService._contract_data(context, target)
    context.requests_response = Service.get(context, path.replace("{did}", did), ["Contract Manager"], context.contract_organizations[own])


@then('the cross-organization roles of an unrelated organization cannot retrieve contract "{name}"')
def cross_organization_roles_refused(context, name):
    # QA F03: a role does not cross organizations.
    did, _ = ContractService._contract_data(context, name)
    for role in ("Auditor", "Sys. Administrator", "Sys. Contract Manager"):
        response = Service.get(context, f"/contract/retrieve/{did}", ["Contract Observer", role], "BDD Unrelated Privileged Org")
        assert response.status_code == 403, (role, response.status_code, response.text)


@then('a system client acting for the instance can retrieve contract "{name}"')
def instance_client_read(context, name):
    did, _ = ContractService._contract_data(context, name)
    response = Service.get(context, f"/contract/retrieve/{did}", ["Sys. Contract Manager"], Service.instance_did(context))
    assert response.status_code == 200, response.text
    assert response.json()["did"] == did


@given('the evidence file "{filename}" contains hexadecimal bytes "{content}"')
def evidence_file(context, filename, content):
    context.evidence_file = Service.evidence_file(context, filename, content)


@when('the Contract Manager uploads the file to contract "{name}" with description "{description}"')
def upload_file(context, name, description):
    context.evidence_description = description
    context.evidence_id, context.evidence_version = Service.upload_evidence(context, name, context.evidence_file, description)


@when('the Contract Manager downloads that evidence from contract "{name}"')
def download_file(context, name):
    context.requests_response = Service.evidence(context, name, context.evidence_id)


@then('the evidence has the original file bytes, metadata and server SHA256 for contract "{name}"')
def original_evidence(context, name):
    response = context.requests_response
    assert response.status_code == 200, response.text
    body = response.json()
    expected = context.evidence_file.read_bytes()
    assert base64.b64decode(body["content"], validate=True) == expected
    assert body["sha256"] == hashlib.sha256(expected).hexdigest()
    assert body["description"] == context.evidence_description
    assert body["filename"] == context.evidence_file.name
    assert body["media_type"] == "application/octet-stream"
    assert body["evidence_id"] == context.evidence_id
    assert body["did"] == context.contract_dids[name]
    assert body["contract_version"] == context.evidence_version
    assert body["recorded_by"] == context.contract_organizations[name]
    assert datetime.fromisoformat(body["recorded_at"].replace("Z", "+00:00")).tzinfo


@when('the Contract Manager terminates the signed contract "{name}"')
def terminate_contract(context, name):
    Service.terminate_contract(context, name)


@when('the Archive Manager deletes contract "{name}" with an erasure justification')
def delete_contract(context, name):
    context.requests_response = Service.delete_archived_contract(context, name)


@then('the contract "{name}" has a shredded key and its evidence is unreadable')
def shredded_evidence(context, name):
    did, _ = ContractService._contract_data(context, name)
    status = Service.get(context, "/archive/erasure-status", ["Archive Manager"], context.contract_organizations[name], did=did)
    assert status.status_code == 200, status.text
    assert status.json()["local_status"] == "shredded", status.text
    response = Service.evidence(context, name, context.evidence_id)
    assert response.status_code == 404, response.text
    assert base64.b64encode(context.evidence_file.read_bytes()).decode() not in response.text
