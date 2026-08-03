"""Executable BDD flow for a realistic ContractField-based service agreement.

What validates the dcs:contractData graph below, and what does not.

The domain types this fixture uses — dcs:ContractParties, dcs:ServiceDescription,
dcs:PaymentTerms, dcs:ServiceLevel, dcs:Jurisdiction — are declared in no
ontology and matched by no shape, and that is ADR-23's design rather than a
gap: dcs:contractData is an arbitrary object graph, and a document is
validated against the shapes graphs it declares in its own sh:shapesGraph.
This fixture declares no domain library, so what the gate checks here is
structure only: the graph is closed, every node is typed and addressable by
@id, every reference resolves in-document, and each filled leaf matches its
declared dcs:datatype.

Nothing checks that a dcs:PaymentTerms node means payment terms. To get
vocabulary-level validation, register a shapes library and let normalization
anchor it — that is what features/fixtures/facis-sla-hosting-shapes.ttl does
for the SLA federation pack. Do not read this flow as evidence that the
demo's domain vocabulary is enforced; it is evidence that the envelope and
the field bindings are.
"""

import json
from pathlib import Path

from behave import given, then, when

from steps.support.api_client import (
    contract_create_url,
    contract_retrieve_by_id_url,
    get_with_headers,
    post_json,
    template_approve_url,
    template_create_url,
    template_register_url,
    template_retrieve_by_id_url,
    template_submit_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.template_service import TemplateService


ARTIFACT_DIRECTORY = Path("tests/integration/artifacts/generated")
TEMPLATE_ARTIFACT = ARTIFACT_DIRECTORY / "realistic-contract-field-template.jsonld"
CONTRACT_ARTIFACT = ARTIFACT_DIRECTORY / "realistic-contract-field-contract.jsonld"

FIELD_SPECS = (
    ("customer-name", "Auftraggeber", "xsd:string", "Beispiel Handel GmbH"),
    ("provider-name", "Auftragnehmer", "xsd:string", "FACIS Services GmbH"),
    (
        "service-description",
        "Leistungsbeschreibung",
        "xsd:string",
        "Betrieb und Support der digitalen Vertragsplattform",
    ),
    ("monthly-fee", "Monatliche Vergütung", "xsd:decimal", 12500.00),
    ("currency", "Währung", "xsd:string", "EUR"),
    ("payment-term-days", "Zahlungsziel in Tagen", "xsd:integer", 30),
    ("term-months", "Vertragslaufzeit in Monaten", "xsd:integer", 24),
    ("availability", "Verfügbarkeit in Prozent", "xsd:decimal", 99.9),
    ("reaction-time-hours", "Reaktionszeit in Stunden", "xsd:integer", 4),
    ("venue", "Gerichtsstand", "xsd:string", "Berlin"),
)

SECTION_CHILDREN = {
    "parties": ("clause-parties",),
    "service": ("clause-service", "clause-term"),
    "payment": ("clause-payment",),
    "sla": ("clause-sla",),
    "legal": ("clause-legal",),
}


def _field_ref(name):
    return {"@id": f"urn:uuid:field-{name}"}


def _section(name, title):
    return {"@id": f"urn:uuid:block-{name}", "@type": "dcs:Section", "dcs:title": title}


def _clause(name, title, *content):
    return {
        "@id": f"urn:uuid:block-{name}",
        "@type": "dcs:Clause",
        "dcs:title": title,
        "dcs:content": {"@list": list(content)},
    }


def _layout_node(name, children, *, root=False):
    node = {
        "@id": f"urn:uuid:block-{name}",
        "@type": "dcs:LayoutNode",
        "dcs:children": {
            "@list": [{"@id": f"urn:uuid:block-{child}"} for child in children],
        },
    }
    if root:
        node["dcs:isRoot"] = True
    return node


def _realistic_template_document():
    fields = [
        {
            "@id": f"urn:uuid:field-{name}",
            "@type": "dcs:ContractField",
            "dcs:label": label,
            "dcs:datatype": datatype,
            "dcs:required": True,
            "dcs:value": value,
        }
        for name, label, datatype, value in FIELD_SPECS
    ]
    blocks = [
        _section("parties", "1. Vertragsparteien"),
        _clause(
            "clause-parties",
            "Vertragsparteien",
            "Der Vertrag wird geschlossen zwischen ",
            _field_ref("customer-name"),
            " und ",
            _field_ref("provider-name"),
            ".",
        ),
        _section("service", "2. Leistung und Laufzeit"),
        _clause(
            "clause-service",
            "Leistungsgegenstand",
            "Der Auftragnehmer erbringt folgende Leistung: ",
            _field_ref("service-description"),
            ".",
        ),
        _clause(
            "clause-term",
            "Laufzeit",
            "Die Vertragslaufzeit beträgt ",
            _field_ref("term-months"),
            " Monate.",
        ),
        _section("payment", "3. Vergütung"),
        _clause(
            "clause-payment",
            "Vergütung und Zahlungsziel",
            "Die monatliche Vergütung beträgt ",
            _field_ref("monthly-fee"),
            " ",
            _field_ref("currency"),
            ". Rechnungen sind innerhalb von ",
            _field_ref("payment-term-days"),
            " Tagen fällig.",
        ),
        _section("sla", "4. Service Level"),
        _clause(
            "clause-sla",
            "Verfügbarkeit und Reaktionszeit",
            "Die zugesicherte Verfügbarkeit beträgt ",
            _field_ref("availability"),
            " Prozent; die Reaktionszeit beträgt höchstens ",
            _field_ref("reaction-time-hours"),
            " Stunden.",
        ),
        _section("legal", "5. Schlussbestimmungen"),
        _clause(
            "clause-legal",
            "Gerichtsstand",
            "Ausschließlicher Gerichtsstand ist ",
            _field_ref("venue"),
            ".",
        ),
    ]
    section_names = tuple(SECTION_CHILDREN)
    layout = [_layout_node("root", section_names, root=True)]
    for section_name, clause_names in SECTION_CHILDREN.items():
        layout.append(_layout_node(section_name, clause_names))
        layout.extend(_layout_node(clause_name, ()) for clause_name in clause_names)

    return {
        "@context": {
            "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
            "xsd": "http://www.w3.org/2001/XMLSchema#",
        },
        "@type": "dcs:ContractTemplate",
        "dcs:metadata": {
            "@type": "dcs:TemplateMetadata",
            "dcs:title": "Realistischer Plattform-Servicevertrag",
            "dcs:description": "Servicevertrag mit Vergütung, Laufzeit, SLA und Gerichtsstand",
            "dcs:templateType": "dcs:Contract",
        },
        "dcs:contractFields": fields,
        "dcs:contractData": [
            {
                "@id": "urn:uuid:data-parties",
                "@type": "dcs:ContractParties",
                "dcs:customer": _field_ref("customer-name"),
                "dcs:provider": _field_ref("provider-name"),
            },
            {
                "@id": "urn:uuid:data-service",
                "@type": "dcs:ServiceDescription",
                "dcs:description": _field_ref("service-description"),
                "dcs:termMonths": _field_ref("term-months"),
            },
            {
                "@id": "urn:uuid:data-payment",
                "@type": "dcs:PaymentTerms",
                "dcs:monthlyFee": _field_ref("monthly-fee"),
                "dcs:currency": _field_ref("currency"),
                "dcs:paymentTermDays": _field_ref("payment-term-days"),
            },
            {
                "@id": "urn:uuid:data-service-level",
                "@type": "dcs:ServiceLevel",
                "dcs:availability": _field_ref("availability"),
                "dcs:reactionTimeHours": _field_ref("reaction-time-hours"),
            },
            {
                "@id": "urn:uuid:data-jurisdiction",
                "@type": "dcs:Jurisdiction",
                "dcs:venue": _field_ref("venue"),
            },
        ],
        "dcs:policies": [],
        "dcs:documentStructure": {
            "@type": "dcs:DocumentStructure",
            "dcs:blocks": {"@list": blocks},
            "dcs:layout": {"@list": layout},
        },
    }


def _template_headers(role):
    return AuthService.get_headers_for_roles([role])


def _refresh_template(context, role):
    response = get_with_headers(
        context,
        template_retrieve_by_id_url(context, context.realistic_template_did),
        headers=_template_headers(role),
    )
    assert response.status_code == 200, f"Template retrieve failed: {response.text}"
    context.realistic_template_response = response.json()
    context.realistic_template_updated_at = context.realistic_template_response["updated_at"]
    return context.realistic_template_response


def _scoped_to_did(node_id: str, did: str) -> bool:
    """True when an internal identifier is rebased under the document's DID.
    The backend mints dereferenceable resource IRIs (base.ResourceIRI), so
    the pre-fragment part is either the bare DID or a URL ending in /<did>."""
    base, _, _ = node_id.partition("#")
    return base == did or base.endswith(f"/{did}")


def _document_ids(document):
    ids = []

    def visit(value):
        if isinstance(value, dict):
            node_id = value.get("@id")
            if isinstance(node_id, str):
                ids.append(node_id)
            for nested in value.values():
                visit(nested)
        elif isinstance(value, list):
            for nested in value:
                visit(nested)

    visit(document)
    return ids


def _blocks_by_suffix(document):
    blocks = document["dcs:documentStructure"]["dcs:blocks"]["@list"]
    return {block["@id"].rsplit("#block-", 1)[-1]: block for block in blocks}


def _layout_by_suffix(document):
    layout = document["dcs:documentStructure"]["dcs:layout"]["@list"]
    return {node["@id"].rsplit("#block-", 1)[-1]: node for node in layout}


@given("a realistic service agreement template with contract fields is prepared")
def step_given_realistic_template_prepared(context):
    context.realistic_template_fixture = _realistic_template_document()


@when("the Template Creator creates the realistic service agreement template")
def step_when_creator_creates_realistic_template(context):
    response = post_json(
        context,
        template_create_url(context),
        {
            "template_type": TemplateService.CONTRACT_TEMPLATE_TYPE,
            "name": "Realistischer Plattform-Servicevertrag",
            "description": "BDD ContractField Lifecycle",
            "template_data": context.realistic_template_fixture,
        },
        headers=_template_headers("Template Creator"),
    )
    assert response.status_code == 200, f"Template create failed: {response.text}"
    context.realistic_template_did = response.json().get("did")
    assert context.realistic_template_did, f"Template create returned no DID: {response.text}"
    _refresh_template(context, "Template Creator")


@when("the Template Creator submits the realistic template for review")
def step_when_creator_submits_realistic_template(context):
    context.realistic_template_updated_at = TemplateService.do_submit(
        context,
        context.realistic_template_did,
        context.realistic_template_updated_at,
    )


@when("the Template Reviewer verifies the realistic template")
def step_when_reviewer_verifies_realistic_template(context):
    context.realistic_template_updated_at = TemplateService.do_verify(
        context,
        context.realistic_template_did,
        context.realistic_template_updated_at,
    )


@when("the Template Reviewer forwards the realistic template for approval")
def step_when_reviewer_forwards_realistic_template(context):
    response = post_json(
        context,
        template_submit_url(context),
        TemplateService.template_reviewer_submit_payload(
            context,
            context.realistic_template_did,
            context.realistic_template_updated_at,
        ),
        headers=_template_headers("Template Reviewer"),
    )
    assert response.status_code == 200, f"Template reviewer forward failed: {response.text}"
    _refresh_template(context, "Template Reviewer")


@when("the Template Approver approves the realistic template")
def step_when_approver_approves_realistic_template(context):
    response = post_json(
        context,
        template_approve_url(context),
        {
            "did": context.realistic_template_did,
            "updated_at": context.realistic_template_updated_at,
        },
        headers=_template_headers("Template Approver"),
    )
    assert response.status_code == 200, f"Template approve failed: {response.text}"
    _refresh_template(context, "Template Approver")


@when("the Template Manager registers the realistic template")
def step_when_manager_registers_realistic_template(context):
    response = post_json(
        context,
        template_register_url(context),
        {"did": context.realistic_template_did},
        headers=_template_headers("Template Manager"),
    )
    assert response.status_code == 200, f"Template register failed: {response.text}"


@when("the Template Manager retrieves the registered realistic template")
def step_when_template_user_retrieves_realistic_template(context):
    _refresh_template(context, "Template Manager")
    context.realistic_template_data = context.realistic_template_response.get("template_data")
    assert isinstance(context.realistic_template_data, dict), (
        f"Retrieved template_data is not a JSON object: {context.realistic_template_response}"
    )


@when("the Contract Creator creates a contract from that registered template DID")
def step_when_contract_creator_creates_realistic_contract(context):
    response = post_json(
        context,
        contract_create_url(context),
        {"template_did": context.realistic_template_did},
        headers=AuthService.get_headers_for_roles(["Contract Creator"]),
    )
    assert response.status_code == 200, f"Contract create failed: {response.text}"
    context.realistic_contract_did = response.json().get("did")
    assert context.realistic_contract_did, f"Contract create returned no DID: {response.text}"


@when("the Contract Creator retrieves the realistic contract")
def step_when_contract_creator_retrieves_realistic_contract(context):
    response = get_with_headers(
        context,
        contract_retrieve_by_id_url(context, context.realistic_contract_did),
        headers=AuthService.get_headers_for_roles(["Contract Creator"]),
    )
    assert response.status_code == 200, f"Contract retrieve failed: {response.text}"
    context.realistic_contract_response = response.json()
    context.realistic_contract_data = context.realistic_contract_response.get("contract_data")
    assert isinstance(context.realistic_contract_data, dict), (
        f"Retrieved contract_data is not a JSON object: {context.realistic_contract_response}"
    )


@then("the retrieved realistic template is registered")
def step_then_realistic_template_registered(context):
    assert context.realistic_template_response.get("state") == "REGISTERED", (
        f"Expected REGISTERED template, got: {context.realistic_template_response}"
    )


@then("both retrieved documents contain the complete ContractField model")
def step_then_documents_contain_contract_fields(context):
    expected_suffixes = {f"field-{name}" for name, *_ in FIELD_SPECS}
    for label, document in (
        ("template", context.realistic_template_data),
        ("contract", context.realistic_contract_data),
    ):
        fields = document.get("dcs:contractFields")
        assert isinstance(fields, list) and len(fields) == len(FIELD_SPECS), (
            f"{label} must contain exactly {len(FIELD_SPECS)} contract fields: {fields}"
        )
        actual_suffixes = {field["@id"].rsplit("#", 1)[-1] for field in fields}
        assert actual_suffixes == expected_suffixes, (
            f"{label} field registry differs: expected {expected_suffixes}, got {actual_suffixes}"
        )
        assert all(field.get("@type") == "dcs:ContractField" for field in fields), (
            f"{label} contains a non-ContractField declaration: {fields}"
        )
        declared_ids = {field["@id"] for field in fields}
        contract_data_refs = {
            node_id
            for node_id in _document_ids(document.get("dcs:contractData", []))
            if "#field-" in node_id
        }
        assert contract_data_refs == declared_ids, (
            f"{label} business objects must reference every declared field by @id: "
            f"declared={declared_ids}, referenced={contract_data_refs}"
        )


@then("both retrieved documents retain the section and indented clause structure")
def step_then_documents_retain_layout(context):
    for label, document in (
        ("template", context.realistic_template_data),
        ("contract", context.realistic_contract_data),
    ):
        blocks = _blocks_by_suffix(document)
        layout = _layout_by_suffix(document)
        for section_name, clause_names in SECTION_CHILDREN.items():
            assert blocks[section_name].get("@type") == "dcs:Section", (
                f"{label} block {section_name} is not a Section: {blocks[section_name]}"
            )
            child_suffixes = tuple(
                ref["@id"].rsplit("#block-", 1)[-1]
                for ref in layout[section_name]["dcs:children"]["@list"]
            )
            assert child_suffixes == clause_names, (
                f"{label} clauses are not indented below section {section_name}: "
                f"expected {clause_names}, got {child_suffixes}"
            )
            assert all(blocks[name].get("@type") == "dcs:Clause" for name in clause_names)


@then("the contract records its template provenance and rebases internal identifiers")
def step_then_contract_provenance_and_rebase(context):
    provenance = context.realistic_contract_data.get("derivedFromTemplate") or {}
    # The backend records the template's dereferenceable resource IRI
    # (base.ResourceIRI("template", did)), not the bare system key.
    provenance_id = provenance.get("@id") or ""
    assert provenance_id == context.realistic_template_did or provenance_id.endswith(
        f"/template/{context.realistic_template_did}"
    ), (
        f"Contract provenance does not identify source template: {provenance}"
    )
    assert isinstance(provenance.get("version"), int) and provenance["version"] >= 1, (
        f"Contract provenance has no template version: {provenance}"
    )
    assert context.realistic_contract_response.get("template_did") == context.realistic_template_did

    template_internal_ids = [
        node_id
        for node_id in _document_ids(context.realistic_template_data)
        if "#block-" in node_id or "#field-" in node_id or "#data-" in node_id
    ]
    contract_internal_ids = [
        node_id
        for node_id in _document_ids(context.realistic_contract_data)
        if "#block-" in node_id or "#field-" in node_id or "#data-" in node_id
    ]
    assert template_internal_ids and all(
        _scoped_to_did(node_id, context.realistic_template_did) for node_id in template_internal_ids
    ), f"Template identifiers were not rebased to its DID: {template_internal_ids}"
    assert contract_internal_ids and all(
        _scoped_to_did(node_id, context.realistic_contract_did) for node_id in contract_internal_ids
    ), f"Contract identifiers were not rebased to its DID: {contract_internal_ids}"
    assert not any(
        _scoped_to_did(node_id, context.realistic_template_did) for node_id in contract_internal_ids
    ), f"Contract still contains template-scoped internal identifiers: {contract_internal_ids}"


@then("neither retrieved document contains a dcs Placeholder node")
def step_then_documents_have_no_placeholder(context):
    for label, document in (
        ("template", context.realistic_template_data),
        ("contract", context.realistic_contract_data),
    ):
        serialized = json.dumps(document, ensure_ascii=False)
        assert "dcs:Placeholder" not in serialized, f"{label} still contains dcs:Placeholder"


@then("the retrieved template and contract are saved as pretty JSON-LD artifacts")
def step_then_documents_saved_as_artifacts(context):
    ARTIFACT_DIRECTORY.mkdir(parents=True, exist_ok=True)
    TEMPLATE_ARTIFACT.write_text(
        json.dumps(context.realistic_template_data, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    CONTRACT_ARTIFACT.write_text(
        json.dumps(context.realistic_contract_data, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    for artifact in (TEMPLATE_ARTIFACT, CONTRACT_ARTIFACT):
        saved = artifact.read_text(encoding="utf-8")
        assert saved.endswith("\n") and "\n  " in saved, f"Artifact is not pretty JSON: {artifact}"
        assert isinstance(json.loads(saved), dict), f"Artifact is not a JSON object: {artifact}"
