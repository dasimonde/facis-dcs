#!/usr/bin/env python3
"""Connected acceptance run without Behave or mocked HTTP services.

Run against an isolated development stack with QA_DCS_API_BASE (including API
prefix), ISSUER_BASE_URL and DSS_URL explicitly set. Uses the existing wallet
and support-library dependencies. Creates uniquely named fixtures, signs one
contract, then destroys only that contract's archive/key to verify erasure.
"""

from __future__ import annotations

import base64
import csv
import hashlib
import io
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
from uuid import uuid4

import requests

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))

from steps.support.api_client import did_document_url  # noqa: E402
from steps.support.services.auth_service import AuthService  # noqa: E402
from steps.support.services.template_service import TemplateService  # noqa: E402


def main() -> None:
    for key in ("QA_DCS_API_BASE", "ISSUER_BASE_URL", "DSS_URL"):
        if not os.environ.get(key, "").strip():
            raise SystemExit(f"{key} must explicitly name the isolated test stack")
    base = os.environ["QA_DCS_API_BASE"].rstrip("/")
    os.environ["BDD_DCS_BASE_URL"] = base
    run = uuid4().hex
    org = f"Connected {run} A"
    foreign_org = f"Connected {run} B"
    AuthService.DEFAULT_ORGANIZATION = org
    context = SimpleNamespace(base_url=base, http_timeout_seconds=180, session=requests.Session())

    def request(method, path, role, body=None, params=None, status=200, organization=None):
        roles = role if isinstance(role, list) else [role]
        response = context.session.request(
            method, base + path, json=body, params=params,
            headers=AuthService.get_headers_for_roles(roles, organization=organization, api_base=base),
            timeout=180,
        )
        assert response.status_code == status, (
            method, path, response.status_code, response.text[:2000]
        )
        return response

    def retrieve(did, role="Contract Manager"):
        return request("GET", "/contract/retrieve/" + did, role).json()

    def mutate(path, did, role, **extra):
        # PDF completion may race the optimistic timestamp after create/submit.
        for _ in range(6):
            current = retrieve(did, role)
            response = context.session.post(
                base + path, json={"did": did, "updated_at": current["updated_at"], **extra},
                headers=AuthService.get_headers_for_roles([role], api_base=base), timeout=180,
            )
            if response.status_code != 409:
                break
            time.sleep(0.5)
        assert response.status_code == 200, (path, response.status_code, response.text[:2000])
        return response.json()

    def check_templates():
        templates = {}
        for party in ("SINGLE_PARTY", "MULTI_PARTY"):
            did, _ = TemplateService.create_fresh_template(
                context, name=f"Connected {run} {party}", description="Connected classification",
                title=f"Connected {run}", party_type=party,
            )
            current = request("GET", "/template/retrieve/" + did, "Template Creator").json()
            assert current["party_type"] == party
            request("PUT", "/template/update", "Template Creator", {
                "did": did, "updated_at": current["updated_at"], "description": "Edited classification",
            })
            assert request("GET", "/template/retrieve/" + did, "Template Creator").json()["party_type"] == party
            history = request("GET", "/template/history/" + did, "Template Creator").json()
            assert history and all(row["party_type"] == party for row in history), history
            copied = request("POST", "/template/copy", "Template Creator", {"did": did}).json()["did"]
            assert request("GET", "/template/retrieve/" + copied, "Template Creator").json()["party_type"] == party
            templates[party] = did
            current = request("GET", "/template/retrieve/" + did, "Template Creator").json()
            stamp = TemplateService.do_submit(context, did, current["updated_at"])
            stamp = TemplateService.do_recommend_for_approval(context, did, stamp)
            request("POST", "/template/approve", "Template Approver", {"did": did, "updated_at": stamp})
            request("POST", "/template/register", "Template Manager", {"did": did})
            current = request("GET", "/template/retrieve/" + did, "Template Manager").json()
            request("POST", "/template/publish", "Template Manager", {
                "did": did, "updated_at": current["updated_at"],
            })
        for party, did in templates.items():
            for path, role in (("/template/search", "Template Creator"),
                               ("/catalogue/template/search", "Template Manager")):
                result = request("GET", path, role, params={
                    "name": f"Connected {run}", "party_type": party,
                    "offset": 1 if path == "/template/search" else 0, "limit": 100,
                }).json()
                rows = result["items"] if isinstance(result, dict) else result
                assert any(row["did"] == did for row in rows), result
                assert all(row["party_type"] == party for row in rows), result
        print("Template create/edit/history/copy and local + real catalogue filters PASS", flush=True)
        return templates["MULTI_PARTY"]

    def check_access(template):
        contracts = [request("POST", "/contract/create", "Contract Creator",
                             {"template_did": template}, organization=organization).json()["did"]
                     for organization in (org, foreign_org)]
        own, foreign = contracts
        retrieve(own)
        for prefix in ("/contract/retrieve/", "/contract/history/", "/signature/retrieve/",
                       "/pdf/export/contract/", "/contract/export/"):
            request("GET", prefix + foreign, "Contract Manager", status=403)
        for path in ("/contract/retrieve", "/contract/search"):
            result = request("GET", path, "Contract Observer", params={"offset": 1, "limit": 1000}).json()
            rows = result["contracts"] if isinstance(result, dict) else result
            dids = {row["did"] for row in rows}
            assert own in dids and foreign not in dids, result
        request("GET", "/contract/retrieve/" + foreign, ["Auditor", "Contract Observer"])
        justification = f"Denial {run}"
        for _ in range(6):
            request("GET", "/pac/access-attempts", "Archive Manager",
                    params={"justification": justification}, status=403)
        # Reuse the same principal after crossing the failed-login threshold.
        request("GET", "/archive/search", "Archive Manager", params={"did": own})
        attempts = request("GET", "/pac/access-attempts", "Auditor", params={
            "justification": f"Review {run}", "limit": 100,
        }).json()["attempts"]
        denied = [row for row in attempts if row["justification"] == justification]
        assert len(denied) == 6 and all(row["outcome"] == "DENIED" for row in denied), denied
        print("Organization scope, additive Auditor and six denials without lockout PASS", flush=True)
        return own

    def check_evidence(did):
        content = bytes([0, 255, 1, 2, 3, 4]) + run.encode() + b"\n"
        description = f"Confidential connected evidence {run}"
        result = mutate("/contract/store", did, "Contract Manager", filename="evidence.bin",
                        description=description, content=base64.b64encode(content).decode(),
                        media_type="application/octet-stream")
        evidence = result["evidence_id"]
        downloaded = request("GET", f"/contract/{did}/evidence/{evidence}", "Contract Manager").json()
        assert base64.b64decode(downloaded["content"]) == content
        assert downloaded["sha256"] == hashlib.sha256(content).hexdigest()
        assert downloaded["description"] == description
        request("GET", f"/contract/{did}/evidence/{evidence}", "Contract Manager",
                organization=foreign_org, status=403)
        print("Encrypted evidence exact bytes/hash/description PASS", flush=True)
        return evidence

    def check_report(did):
        audit = request("POST", "/pac/audit", "Auditor", {
            "scope": "contract", "did": did, "justification": f"Connected audit {run}",
        }).json()
        report = None
        for fmt, mime in (("json", "application/json"), ("csv", "text/csv"), ("pdf", "application/pdf")):
            response = request("GET", "/pac/report", "Auditor", params={
                "scope": "contract", "did": did, "format": fmt, "justification": f"Export {run}",
            })
            assert response.headers["Content-Type"].startswith(mime), response.headers
            assert response.content
            if fmt == "json":
                report = response.json()
                assert report["reportId"] == audit["audit_id"]
                assert report["events"] and report["findings"]
                assert report["summary"]["totalEvents"] == len(report["events"])
            elif fmt == "csv":
                rows = list(csv.DictReader(io.StringIO(response.text)))
                events = [row for row in rows if row["section"] == "event"]
                assert len(events) == len(report["events"]), rows
                assert all(row["timestamp"] and row["did"] == did for row in events), events
            else:
                assert response.content.startswith(b"%PDF-")
        print("Real executor report JSON/CSV/PDF MIME and timeline PASS", flush=True)
        return report

    def sign(did, directory):
        for role, extra in (("Contract Creator", {}), ("Contract Creator", {}),
                            ("Contract Reviewer", {"forward_to": "approval"})):
            mutate("/contract/submit", did, role, **extra)
        mutate("/contract/approve", did, "Contract Approver")
        contract = request("GET", "/signature/retrieve/" + did, "Contract Signer").json()["contract"]
        response = context.session.get(did_document_url(base), timeout=30)
        response.raise_for_status()
        local_did = response.json()["id"]
        field = next(item["dcs:signatoryName"] for item in contract["contract_data"]["dcs:signatureFields"]
                     if item.get("dcs:signatoryName") == local_did)
        ceremony = request("POST", "/signature/request", "Contract Signer", {
            "contract_did": did, "field_name": field,
        }).json()
        env = dict(os.environ, E2E_SIGNATORY=f"Connected {run}", E2E_SIGN_FIELD=field)
        subprocess.run([sys.executable, str(ROOT / "frontend/ClientApp/e2e/complete_signing_webhook.py"),
                        ceremony["wallet_uri"]], env=env, check=True)
        status = request("GET", "/signature/request/" + ceremony["ceremony_id"], "Contract Signer").json()
        assert status["status"] == "verified", status
        payload = {"did": did, "signer_did": status["signer_did"], "field_name": field,
                   "ceremony_id": ceremony["ceremony_id"], "credential_type": "AES"}
        prepared = request("POST", "/signature/prepare", "Contract Signer", payload).json()
        unsigned, signed = directory / "prepared.pdf", directory / "signed.pdf"
        unsigned.write_bytes(base64.b64decode(prepared["document"]))
        subprocess.run([sys.executable, str(ROOT / "frontend/ClientApp/e2e/sign_prepared_pdf.py"),
                        str(unsigned), str(signed)], env=env, check=True)
        request("POST", "/signature/submit", "Contract Signer", {
            **payload, "signed_pdf": base64.b64encode(signed.read_bytes()).decode(), "jades_signature": "",
        })
        print("Real wallet ceremony, PDF preparation, external DSS signing and archive PASS", flush=True)

    def check_archive_and_erase(did, evidence, original_report):
        params = {"did": did}
        entries = request("GET", "/archive/search", "Contract Observer", params=params).json()
        assert entries and all(item["did"] == did for item in entries), entries
        version = max(item["contract_version"] for item in entries)
        detail_params = {"did": did, "contract_version": version}
        snapshot = request("GET", "/archive/detail", "Contract Observer", params=detail_params).json()
        assert snapshot["did"] == did and snapshot["contract_version"] == version
        request("GET", "/archive/detail", "Contract Observer",
                params={"did": did, "contract_version": version + 1000}, status=404)
        assert request("GET", "/archive/search", "Contract Observer", params=params,
                       organization=foreign_org).json() == []
        summary, tag = f"Retained summary {run}", f"updated-{run}"
        annotated = request("POST", "/archive/annotate", "Archive Manager", {
            "did": did, "summary": summary, "tags": [f"initial-{run}"],
        }).json()
        assert annotated["summary"] == summary
        annotated = request("POST", "/archive/annotate", "Archive Manager", {"did": did, "tags": [tag]}).json()
        assert annotated["summary"] == summary and annotated["tags"] == [tag]
        reloaded = request("GET", "/archive/search", "Contract Observer", params={**params, "tag": tag}).json()
        assert reloaded and all(row["archive_summary"] == summary for row in reloaded)
        assert request("GET", "/archive/detail", "Contract Observer", params=detail_params).json() == snapshot
        mutate("/contract/terminate", did, "Contract Manager", reason="Connected frozen snapshot check")
        assert retrieve(did)["state"] == "TERMINATED"
        assert request("GET", "/archive/detail", "Contract Observer", params=detail_params).json() == snapshot
        report_params = {"scope": "contract", "did": did, "format": "json", "justification": f"Frozen {run}"}
        report = request("GET", "/pac/report", "Auditor", params=report_params).json()
        for key in ("reportId", "events", "findings"):
            assert report[key] == original_report[key], key
        print("Tags-only summary retention and frozen archive/report PASS", flush=True)
        # This DID was created above in this run; never accept an external deletion target.
        request("DELETE", "/archive/delete", "Archive Manager", params={
            "did": did, "justification": f"Erase own isolated fixture {run}",
        })
        request("GET", f"/contract/{did}/evidence/{evidence}", "Contract Manager", status=404)
        request("GET", "/pac/report", "Auditor", params=report_params, status=404)
        request("GET", "/archive/detail", "Contract Observer", params=detail_params, status=404)
        print("Cryptographic erasure: evidence, report and snapshot unavailable PASS", flush=True)

    source_keys = AuthService.resolve_wallet_keys_dir()
    with tempfile.TemporaryDirectory(prefix="dcs-connected-") as temporary:
        directory = Path(temporary)
        keys = directory / "wallet-keys"
        keys.mkdir()
        # Cached signing certificates must not leak between test identities.
        for path in source_keys.glob("*.jwk"):
            shutil.copy2(path, keys / path.name)
        os.environ["BDD_TEST_WALLET_KEYS_DIR"] = str(keys)
        template = check_templates()
        did = check_access(template)
        evidence = check_evidence(did)
        report = check_report(did)
        sign(did, directory)
        check_archive_and_erase(did, evidence, report)
    context.session.close()
    print("Connected acceptance PASS", flush=True)


if __name__ == "__main__":
    main()
