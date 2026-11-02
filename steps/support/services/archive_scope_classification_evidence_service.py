"""HTTP and fixture support for the archive snapshot, party classification, organization scope and evidence scenarios."""

import base64
import hashlib
import json
import tempfile
import uuid
from pathlib import Path

import requests

from steps.support.api_client import (
    delete_with_params, did_document_url, get_with_headers, post_json, put_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService


class ArchiveScopeClassificationEvidenceService:
    @staticmethod
    def headers(context, roles, organization=None):
        return AuthService.get_headers_for_roles(
            roles, organization=organization, api_base=context.base_url,
        )

    @staticmethod
    def get(context, path, roles, organization=None, **params):
        return requests.get(
            f"{context.base_url}{path}", params=params,
            headers=ArchiveScopeClassificationEvidenceService.headers(context, roles, organization),
            timeout=context.http_timeout_seconds,
        )

    @staticmethod
    def instance_did(context):
        response = requests.get(did_document_url(context.base_url), timeout=context.http_timeout_seconds)
        assert response.status_code == 200, response.text
        return response.json()["id"]

    @staticmethod
    def signed_contract(context, name):
        from steps.template_management.contract_state_machine_steps import step_given_contract_reached_state

        organization = f"BDD Archive Scope {uuid.uuid4()}"
        previous = AuthService.DEFAULT_ORGANIZATION
        try:
            # Existing lifecycle fixtures resolve every role through this default.
            AuthService.DEFAULT_ORGANIZATION = organization
            step_given_contract_reached_state(context, name, "SIGNED")
        finally:
            AuthService.DEFAULT_ORGANIZATION = previous
        if not hasattr(context, "contract_organizations"):
            context.contract_organizations = {}
        context.contract_organizations[name] = organization

    @staticmethod
    def archive_entry(context, name):
        did, _ = ContractService._contract_data(context, name)
        response = ArchiveScopeClassificationEvidenceService.get(
            context, "/archive/search", ["Contract Observer"],
            context.contract_organizations[name], did=did,
        )
        assert response.status_code == 200, response.text
        entries = response.json()
        assert isinstance(entries, list) and entries, entries
        return max(entries, key=lambda entry: entry["contract_version"])

    @staticmethod
    def archive_detail(context, name, version):
        did, _ = ContractService._contract_data(context, name)
        return ArchiveScopeClassificationEvidenceService.get(
            context, "/archive/detail", ["Contract Observer"],
            context.contract_organizations[name], did=did, contract_version=version,
        )

    @staticmethod
    def snapshot_hash(snapshot):
        raw = json.dumps(snapshot, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
        for char, escaped in (("&", "\\u0026"), ("<", "\\u003c"), (">", "\\u003e"), ("\u2028", "\\u2028"), ("\u2029", "\\u2029")):
            raw = raw.replace(char, escaped)
        return "sha256:" + hashlib.sha256(raw.encode()).hexdigest()

    @staticmethod
    def advance_live_version(context, name):
        did, _ = ContractService._contract_data(context, name)
        cursor = context.db.cursor()
        try:
            cursor.execute(
                "UPDATE contracts SET contract_version=contract_version+1, name=%s WHERE did=%s",
                ("Newer live version", did),
            )
            assert cursor.rowcount == 1
            context.db.commit()
        finally:
            cursor.close()

    @staticmethod
    def classified_template(context, name, party_type, template_type):
        did, updated = TemplateService.create_fresh_template(
            context, name=name, party_type=party_type, template_type=template_type,
        )
        TemplateService.store_named(context, name, did, updated)
        return did

    @staticmethod
    def update_and_copy_template(context, name):
        template = TemplateService.named(context, name)
        headers = ArchiveScopeClassificationEvidenceService.headers(context, ["Template Creator"])
        body = TemplateService.fetch_template(context, template["did"], headers=headers)
        response = put_json(context, f"{context.base_url}/template/update", {
            "did": template["did"], "updated_at": body["updated_at"],
            "description": "Classification unchanged after edit",
        }, headers=headers)
        assert response.status_code == 200, response.text
        response = post_json(context, f"{context.base_url}/template/copy", {"did": template["did"]}, headers=headers)
        assert response.status_code == 200, response.text
        return response.json()["did"]

    @staticmethod
    def publish_classified_template(context, name):
        template = TemplateService.named(context, name)
        did = template["did"]
        headers = ArchiveScopeClassificationEvidenceService.headers(context, ["Template Creator"])
        updated = TemplateService.fetch_template(context, did, headers=headers)["updated_at"]
        updated = TemplateService.do_submit(context, did, updated)
        updated = TemplateService.do_recommend_for_approval(context, did, updated)
        for action, role in (("approve", "Template Approver"), ("register", "Template Manager"), ("publish", "Template Manager")):
            headers = ArchiveScopeClassificationEvidenceService.headers(context, [role])
            response = post_json(context, f"{context.base_url}/template/{action}", {"did": did, "updated_at": updated}, headers=headers)
            assert response.status_code == 200, response.text
            updated = TemplateService.fetch_template(context, did, headers=headers)["updated_at"]

    @staticmethod
    def evidence_file(context, filename, hex_content):
        directory = tempfile.TemporaryDirectory(prefix="dcs-evidence-")
        context.add_cleanup(directory.cleanup)
        path = Path(directory.name) / filename
        path.write_bytes(bytes.fromhex(hex_content))
        return path

    @staticmethod
    def upload_evidence(context, name, path, description):
        did, _ = ContractService._contract_data(context, name)
        headers = ArchiveScopeClassificationEvidenceService.headers(context, ["Contract Manager"], context.contract_organizations[name])
        current = get_with_headers(context, f"{context.base_url}/contract/retrieve/{did}", headers=headers)
        assert current.status_code == 200, current.text
        payload = {
            "did": did, "updated_at": current.json()["updated_at"],
            "filename": path.name, "description": description,
            "media_type": "application/octet-stream",
            "content": base64.b64encode(path.read_bytes()).decode(),
        }
        response = post_json(context, f"{context.base_url}/contract/store", payload, headers=headers)
        assert response.status_code == 200, response.text
        return response.json()["evidence_id"], current.json()["contract_version"]

    @staticmethod
    def evidence(context, name, evidence_id):
        did, _ = ContractService._contract_data(context, name)
        return ArchiveScopeClassificationEvidenceService.get(
            context, f"/contract/{did}/evidence/{evidence_id}",
            ["Contract Manager"], context.contract_organizations[name],
        )

    @staticmethod
    def terminate_contract(context, name):
        from steps.template_management.contract_state_machine_steps import _terminate_contract

        previous = AuthService.DEFAULT_ORGANIZATION
        try:
            AuthService.DEFAULT_ORGANIZATION = context.contract_organizations[name]
            _terminate_contract(context, name)
        finally:
            AuthService.DEFAULT_ORGANIZATION = previous

    @staticmethod
    def delete_archived_contract(context, name):
        did, _ = ContractService._contract_data(context, name)
        headers = ArchiveScopeClassificationEvidenceService.headers(context, ["Archive Manager"], context.contract_organizations[name])
        return delete_with_params(
            context, f"{context.base_url}/archive/delete",
            {"did": did, "justification": "BDD evidence erasure request"}, headers=headers,
        )


def seed_open_tasks(context, names):
    peer = ContractService._local_peer_did(context)
    cursor = context.db.cursor()
    try:
        for name in names:
            did, _ = ContractService._contract_data(context, name)
            cursor.execute("INSERT INTO contract_review_task (did,state,reviewer,created_by) VALUES (%s,'OPEN',%s,%s)", (did, peer, context.contract_organizations[name]))
            cursor.execute("INSERT INTO contract_approval_task (did,state,approver,created_by) VALUES (%s,'OPEN',%s,%s)", (did, peer, context.contract_organizations[name]))
            cursor.execute("INSERT INTO contract_negotiation_task (did,state,negotiator,created_by,contract_version) SELECT did,'OPEN',%s,%s,contract_version FROM contracts WHERE did=%s ON CONFLICT (did,negotiator,contract_version) DO UPDATE SET state='OPEN'", (peer, context.contract_organizations[name], did))
        context.db.commit()
    finally:
        cursor.close()


def assert_scoped_lists(context, own, foreign):
    own_did, _ = ContractService._contract_data(context, own)
    foreign_did, _ = ContractService._contract_data(context, foreign)
    for path, field in (("/contract/search", None), ("/archive/search", None), ("/signature/retrieve", "contracts")):
        response = ArchiveScopeClassificationEvidenceService.get(context, path, ["Contract Observer"], context.contract_organizations[own], offset=1, limit=1000)
        assert response.status_code == 200, (path, response.text)
        items = response.json()[field] if field else response.json()
        assert isinstance(items, list), (path, items)
        dids = {item["did"] for item in items}
        assert own_did in dids, (path, items)
        assert foreign_did not in dids, (path, items)
