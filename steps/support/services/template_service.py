import re

from docutils.nodes import description
from requests import get

from steps.support.api_client import get_with_headers, post_json, template_approve_url, template_create_url, template_retrieve_by_id_url, template_retrieve_url, template_submit_url, template_verify_url
from steps.support.services.auth_service import AuthService


class TemplateService: 
    """Service class for template-related operations."""

    CONTRACT_TEMPLATE_TYPE = "CONTRACT_TEMPLATE"
    COMPONENT_TEMPLATE_TYPE = "COMPONENT"

    @staticmethod
    def template_env_key(name: str) -> str:
        normalized = re.sub(r"[^A-Za-z0-9]+", "_", name).strip("_").upper()
        return f"BDD_TEMPLATE_DID_{normalized}"


    @staticmethod
    def template_type_for_category(category: str) -> str:
        category_key = category.strip().lower()
        return {
            "legal": TemplateService.CONTRACT_TEMPLATE_TYPE,
            "procurement": TemplateService.COMPONENT_TEMPLATE_TYPE,
        }.get(category_key, category.strip().upper().replace(" ", "_"))

    @staticmethod
    def canonical_document_data(title: str, clause_text: str = "Confidentiality clause", document_type: str = "dcs:ContractTemplate") -> dict:
        """Build the canonical dcs:documentStructure envelope (single fixture
        source for template_data/contract_data across all step modules — the
        flat {"title", "clauses"} shape is rejected by
        NormalizeTemplateData/NormalizeContractData, see
        backend/internal/base/validation/documentdata.go isCanonicalEnvelope).
        """
        metadata_type = "dcs:ContractMetadata" if document_type == "dcs:Contract" else "dcs:TemplateMetadata"
        return {
            "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
            "@type": document_type,
            "dcs:metadata": {
                "@type": metadata_type,
                "dcs:title": title,
            },
            "dcs:documentStructure": {
                "@type": "dcs:DocumentStructure",
                "dcs:blocks": {
                    "@list": [
                        {
                            "@id": "urn:uuid:block-clause-1",
                            "@type": "dcs:Clause",
                            "dcs:content": {"@list": [clause_text]},
                        }
                    ]
                },
                "dcs:layout": [
                    {
                        "@id": "urn:uuid:block-root",
                        "@type": "dcs:LayoutNode",
                        "dcs:isRoot": True,
                        "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]},
                    }
                ],
            },
        }

    @staticmethod
    def create_fresh_template(context, name="Standard Template", description="BDD auto-created template", title="BDD Standard NDA") -> tuple:
        """Create a Draft template as Template Creator; return (did, updated_at)."""
        headers = AuthService.get_headers_for_roles(["Template Creator"])
        payload = {
            "template_type": TemplateService.template_type_for_category("legal"),
            "name": name,
            "description": description,
            "template_data": TemplateService.canonical_document_data(title),
        }
        resp = post_json(context, template_create_url(context), payload, headers=headers)
        assert resp.status_code == 200, f"Template create failed: {resp.text}"
        did = resp.json().get("did")
        assert did, f"No DID in create response: {resp.text}"
        body = TemplateService.fetch_template(context, did, headers=headers)
        return did, body.get("updated_at")
        
    @staticmethod
    def create_approved_template(context) -> tuple:
        """Create and approve a template; return (did, updated_at)."""
        did, updated_at = TemplateService.create_fresh_template(context)
        updated_at = TemplateService.do_submit(context, did, updated_at)
        updated_at = TemplateService.do_recommend_for_approval(context, did, updated_at)
        approver_headers = AuthService.get_headers_for_roles(["Template Approver"])
        approve_resp = post_json(
            context,
            template_approve_url(context),
            {"did": did, "updated_at": updated_at},
            headers=approver_headers,
        )
        assert approve_resp.status_code == 200, f"Template approve failed: {approve_resp.text}"
        updated_at = TemplateService.fetch_template(context, did, headers=approver_headers).get("updated_at")
        return did, updated_at

    @staticmethod
    def fetch_template(context, did: str, headers=None) -> dict:
        resp = get_with_headers(context, template_retrieve_by_id_url(context, did), headers=headers)
        assert resp.status_code == 200, f"Template retrieve failed: {resp.text}"
        return resp.json()

    @staticmethod
    def fetch_all_templates(context, headers=None) -> dict:
        resp = get_with_headers(context, template_retrieve_url(context), headers=headers)
        assert resp.status_code == 200, f"Template retrieve failed: {resp.text}"
        return resp.json()

    @staticmethod
    def template_submit_payload(context, did: str, updated_at: str) -> dict:
        AuthService.get_headers_for_roles(["Template Reviewer"])
        AuthService.get_headers_for_roles(["Template Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "reviewers": [AuthService.username_for_roles(["Template Reviewer"])],
            "approver": AuthService.username_for_roles(["Template Approver"]),
        }

    @staticmethod
    def template_update_payload(context, did: str, updated_at: str, name=None, description=None) -> dict:
        AuthService.get_headers_for_roles(["Template Creator"])
        return {
            "did": did,
            "updated_at": updated_at,
            "name": name,
            "description": description,
        }

    @staticmethod
    def template_submit_payload_with_flag(context, did: str, updated_at: str, flag: str) -> dict:
        AuthService.get_headers_for_roles(["Template Reviewer"])
        AuthService.get_headers_for_roles(["Template Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "reviewers": [AuthService.username_for_roles(["Template Reviewer"])],
            "approver": AuthService.username_for_roles(["Template Approver"]),
            "forward_to": flag
        }

    @staticmethod
    def template_submit_payload_without_reviewers(context, did: str, updated_at: str) -> dict:
        AuthService.get_headers_for_roles(["Template Reviewer"])
        AuthService.get_headers_for_roles(["Template Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "approver": AuthService.username_for_roles(["Template Approver"]),
        }

    @staticmethod
    def template_submit_payload_without_approver(context, did: str, updated_at: str) -> dict:
        AuthService.get_headers_for_roles(["Template Reviewer"])
        AuthService.get_headers_for_roles(["Template Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "reviewers": [AuthService.username_for_roles(["Template Reviewer"])]
        }

    @staticmethod
    def template_reviewer_submit_payload(context, did: str, updated_at: str, flag="approval") -> dict:
        AuthService.get_headers_for_roles(["Template Approver"])
        return {
            "did": did,
            "updated_at": updated_at,
            "approver": AuthService.username_for_roles(["Template Approver"]),
            "forward_to": flag,
        }

    @staticmethod
    def do_submit(context, did: str, updated_at: str) -> str:
        """Submit template as Template Creator; return refreshed updated_at.

        The genesis PDF render that create triggers is asynchronous and bumps
        updated_at shortly after create, so the first submit can lose the
        optimistic-lock race ("updated elsewhere"). Re-fetch the current
        updated_at and retry on that conflict.
        """
        import time

        headers = AuthService.get_headers_for_roles(["Template Creator"])
        for _ in range(6):
            resp = post_json(
                context,
                template_submit_url(context),
                TemplateService.template_submit_payload(context, did, updated_at),
                headers=headers,
            )
            if resp.status_code == 200:
                return TemplateService.fetch_template(context, did, headers=headers).get("updated_at")
            if "updated elsewhere" not in resp.text:
                break
            time.sleep(0.5)
            updated_at = TemplateService.fetch_template(context, did, headers=headers).get("updated_at")
        assert resp.status_code == 200, f"Template submit failed: {resp.text}"

    @staticmethod
    def do_verify(context, did: str, updated_at: str) -> str:
        """Verify template content as Template Reviewer; return refreshed updated_at."""
        headers = AuthService.get_headers_for_roles(["Template Reviewer"])
        resp = post_json(context, template_verify_url(context), {"did": did, "updated_at": updated_at}, headers=headers)
        assert resp.status_code == 200, f"Template verify failed: {resp.text}"
        return TemplateService.fetch_template(context, did, headers=headers).get("updated_at")

    @staticmethod
    def do_recommend_for_approval(context, did: str, updated_at: str) -> str:
        """Submit reviewer recommendation and advance review workflow."""
        # Backend requires verification before reviewer recommendation submit.
        updated_at = TemplateService.do_verify(context, did, updated_at)
        headers = AuthService.get_headers_for_roles(["Template Reviewer"])
        resp = post_json(
            context,
            template_submit_url(context),
            TemplateService.template_reviewer_submit_payload(context, did, updated_at),
            headers=headers,
        )
        assert resp.status_code == 200, f"Template review submit failed: {resp.text}"
        return TemplateService.fetch_template(context, did, headers=headers).get("updated_at")

    @staticmethod
    def do_recommend_for_rejected(context, did: str, updated_at: str) -> str:
        """Submit reviewer recommendation and advance review workflow."""
        # Backend requires verification before reviewer recommendation submit.
        updated_at = TemplateService.do_verify(context, did, updated_at)
        headers = AuthService.get_headers_for_roles(["Template Reviewer"])
        resp = post_json(
            context,
            template_submit_url(context),
            TemplateService.template_reviewer_submit_payload(context, did, updated_at, "draft"),
            headers=headers,
        )
        assert resp.status_code == 200, f"Template review submit failed: {resp.text}"
        return TemplateService.fetch_template(context, did, headers=headers).get("updated_at")

    @staticmethod
    def named(context, name: str) -> dict:
        return (getattr(context, "named_templates", None) or {}).get(name, {})

    @staticmethod
    def store_named(context, name: str, did: str, updated_at: str):
        if not hasattr(context, "named_templates") or context.named_templates is None:
            context.named_templates = {}
        context.named_templates[name] = {"did": did, "updated_at": updated_at}
