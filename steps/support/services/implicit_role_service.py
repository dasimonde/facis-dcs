"""HTTP support and honest fixtures for implicit bilateral template roles."""

from urllib.parse import quote

from steps.support.api_client import (
    contract_create_url,
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService


class ImplicitRoleService:
    """Keep feature bindings free of URL construction and lifecycle plumbing."""

    @staticmethod
    def template_document(roles: list[str], *, reverse_and_repeat: bool = False) -> dict:
        """Build a canonical template whose roles occur only as top-level
        odrl:assigner/odrl:assignee party references.

        ``reverse_and_repeat`` deliberately repeats both identities with their
        direction swapped. A correct role derivation therefore cannot use a
        fixed assigner=originator convention and must deduplicate by identity.
        """
        assert roles, "the fixture needs at least one role"
        base = TemplateService.canonical_document_data("BDD Bilateral Role Template")
        base["@context"]["odrl"] = "http://www.w3.org/ns/odrl/2/"
        base["@context"]["xsd"] = "http://www.w3.org/2001/XMLSchema#"
        field_id = "urn:uuid:field-company-location-country"
        base["dcs:contractFields"] = [
            {
                "@id": field_id,
                "@type": "dcs:ContractField",
                "dcs:label": "Company country",
                "dcs:datatype": "xsd:string",
                "dcs:shape": {
                    "@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"
                },
                "dcs:required": True,
            }
        ]

        def party_iri(role: str) -> str:
            # The contractual role is itself an IRI, so it must be encoded
            # inside the template party fragment. Contract derivation has to
            # decode it back to the identical role IRI; treating this as an
            # opaque short-name suffix is the regression under test.
            return f"urn:uuid:bdd-template#party-{quote(role, safe='')}"

        pairs = [(roles[0], roles[1] if len(roles) > 1 else roles[0])]
        for extra in roles[2:]:
            pairs.append((extra, roles[1]))
        if reverse_and_repeat and len(roles) == 2:
            pairs.extend([(roles[1], roles[0]), (roles[0], roles[1])])

        rules = []
        for index, (assigner, assignee) in enumerate(pairs, start=1):
            rules.append(
                {
                    "@id": f"urn:uuid:bdd-party-rule-{index}",
                    "@type": "odrl:Duty",
                    "odrl:action": {"@id": "dcs:provideCompliantValue"},
                    "odrl:assigner": {"@id": party_iri(assigner)},
                    "odrl:assignee": {"@id": party_iri(assignee)},
                    "odrl:target": {"@id": "urn:uuid:pending-target"},
                    "dcs:prose": {"@id": "urn:uuid:block-clause-1"},
                    "odrl:constraint": {
                        "@type": "odrl:Constraint",
                        "odrl:leftOperand": {"@id": field_id},
                        "odrl:operator": {"@id": "odrl:isAnyOf"},
                        "odrl:rightOperand": ["DEU", "AUT", "CHE"],
                    },
                }
            )
        base["dcs:policies"] = {
            "@id": "urn:uuid:bdd-party-policy-set",
            "@type": "odrl:Offer",
            "odrl:profile": {"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
            "odrl:obligation": rules,
        }
        return base

    @staticmethod
    def register_template(context, roles: list[str], *, reverse_and_repeat: bool = False) -> str:
        document = ImplicitRoleService.template_document(roles, reverse_and_repeat=reverse_and_repeat)
        return ContractService._create_approved_template_for_contract(context, template_data=document)

    @staticmethod
    def create_contract(context, template_did: str, originator_role: str):
        headers = AuthService.get_headers_for_roles(["Contract Creator"])
        response = post_json(
            context,
            contract_create_url(context),
            {"template_did": template_did, "originator_role": originator_role},
            headers=headers,
        )
        contract = None
        if response.status_code == 200:
            did = response.json().get("did")
            assert did, f"contract create response contains no DID: {response.text}"
            retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
            assert retrieve.status_code == 200, f"created contract cannot be retrieved: {retrieve.status_code} {retrieve.text}"
            contract = retrieve.json()
        return response, contract
