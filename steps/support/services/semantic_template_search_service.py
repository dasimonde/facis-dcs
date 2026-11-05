"""Template fixture and search operations for semantic-filter scenarios."""

import json
from uuid import uuid4

import requests

from steps.support.api_client import archive_search_url, contract_search_url, post_json, template_create_url, template_search_url
from steps.support.services.auth_service import AuthService
from steps.support.services.odrl_fixture_service import odrl_set_policies
from steps.support.services.template_service import TemplateService


class SemanticTemplateSearchService:
    @staticmethod
    def create(context, row, field_iris):
        document = TemplateService.canonical_document_data(row["name"])
        for label in ("Jurisdiction", "Industry"):
            field = {
                "@id": f"urn:uuid:{uuid4()}",
                "@type": "dcs:ContractField",
                "dcs:label": label,
                "dcs:datatype": "xsd:string",
                "dcs:shape": {"@id": field_iris[label]},
                "dcs:required": False,
            }
            value = row[label.lower()]
            if value is not None:
                field["dcs:value"] = {"@value": value} if row.get("literal") and label == "Jurisdiction" else value
            document["dcs:contractFields"].append(field)
            if label == "Jurisdiction" and row.get("constraint"):
                document["@context"]["odrl"] = "http://www.w3.org/ns/odrl/2/"
                clause_id = document["dcs:documentStructure"]["dcs:blocks"]["@list"][0]["@id"]
                policy = odrl_set_policies(clause_id, "country", "eq", "DEU")
                policy["odrl:obligation"][0]["odrl:constraint"]["odrl:leftOperand"] = {"@id": field["@id"]}
                policy["odrl:obligation"][0]["dcs:prose"] = {"@id": clause_id}
                document["dcs:policies"] = policy
        payload = {
            "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
            "party_type": TemplateService.MULTI_PARTY_TYPE,
            "name": row["name"],
            "template_data": document,
        }
        headers = AuthService.get_headers_for_roles(["Template Creator"])
        return post_json(context, template_create_url(context), payload, headers=headers)

    @staticmethod
    def search(context, name, filters):
        return requests.get(
            template_search_url(context),
            params={"name": name, "semantic_filters": json.dumps(filters)},
            headers=context.headers,
            timeout=context.http_timeout_seconds,
        )

    @staticmethod
    def search_all_endpoints(context, filters):
        headers = AuthService.get_headers_for_roles(["Template Manager", "Contract Manager", "Archive Manager"])
        return [
            requests.get(url(context), params={"semantic_filters": json.dumps(filters)}, headers=headers,
                         timeout=context.http_timeout_seconds)
            for url in (template_search_url, contract_search_url, archive_search_url)
        ]
