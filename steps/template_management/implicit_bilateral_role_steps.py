"""BDD bindings for implicit bilateral template roles (DCS-IR-CWE-01/02)."""

from urllib.parse import unquote

from behave import given, then, when

from steps.support.services.implicit_role_service import ImplicitRoleService


def _roles(value: str) -> list[str]:
    return [item.strip() for item in value.split(",") if item.strip()]


def _document(context) -> dict:
    assert context.implicit_role_contract is not None, "the preceding contract creation did not produce a document"
    document = context.implicit_role_contract.get("contract_data")
    assert isinstance(document, dict), f"contract response contains no contract_data object: {context.implicit_role_contract}"
    return document


def _policy_rules(document: dict) -> list[dict]:
    policies = document.get("dcs:policies") or {}
    rules = []
    for bucket in ("odrl:permission", "odrl:prohibition", "odrl:obligation"):
        value = policies.get(bucket, []) if isinstance(policies, dict) else []
        if isinstance(value, dict):
            value = [value]
        rules.extend(rule for rule in value if isinstance(rule, dict))
    return rules


def _party_role(node: dict) -> str:
    value = node.get("dcs:role")
    if isinstance(value, dict):
        return str(value.get("@id") or value.get("@value") or "")
    return str(value or "")


@given(
    'a registered bilateral template whose top-level ODRL rules repeat the percent-encoded role URIs "{left}" and "{right}" in both directions'
)
def step_given_direction_switching_uri_template(context, left, right):
    assert left.startswith(("http://", "https://")) and right.startswith(("http://", "https://")), (
        f"role fixtures must be absolute concept URIs, got {left!r} and {right!r}"
    )
    context.implicit_role_template_did = ImplicitRoleService.register_template(
        context, [left, right], reverse_and_repeat=True
    )


@when('I create a contract from that template as originator role "{role}"')
def step_when_create_with_role(context, role):
    response, contract = ImplicitRoleService.create_contract(context, context.implicit_role_template_did, role)
    assert response.status_code == 200, f"valid bilateral contract creation failed: {response.status_code} {response.text}"
    context.requests_response = response
    context.implicit_role_contract = contract


@when('I attempt to create a contract from that template as originator role "{role}"')
def step_when_attempt_create_with_role(context, role):
    response, contract = ImplicitRoleService.create_contract(context, context.implicit_role_template_did, role)
    context.requests_response = response
    context.implicit_role_contract = contract


@then('the derived contract contains exactly the roles "{roles}" once each')
def step_then_contract_roles_are_deduplicated(context, roles):
    actual = [_party_role(node) for node in _document(context).get("dcs:parties", []) if isinstance(node, dict)]
    actual = [role for role in actual if role]
    expected = _roles(roles)
    assert sorted(actual) == sorted(expected), f"expected exactly one party node per role {expected}, got {actual}"


@then("every contractual role is stored as an absolute concept URI string")
def step_then_contract_roles_are_uri_strings(context):
    values = [
        node.get("dcs:role")
        for node in _document(context).get("dcs:parties", [])
        if isinstance(node, dict) and "dcs:role" in node
    ]
    assert values, "derived contract contains no dcs:role values"
    invalid = [value for value in values if not isinstance(value, str) or not value.startswith(("http://", "https://"))]
    assert not invalid, f"dcs:role must contain URI strings only, got invalid values {invalid!r}"


@then("each open party fragment percent-decodes to its identical dcs:role URI")
def step_then_open_party_fragments_decode_to_roles(context):
    checked = []
    for node in _document(context).get("dcs:parties", []):
        if not isinstance(node, dict):
            continue
        iri = node.get("@id")
        role = node.get("dcs:role")
        if not isinstance(iri, str) or "#party-" not in iri:
            continue
        encoded_role = iri.rsplit("#party-", 1)[1]
        checked.append((iri, role))
        assert unquote(encoded_role) == role, (
            f"open party fragment {iri!r} decodes to {unquote(encoded_role)!r}, not dcs:role {role!r}"
        )
    assert checked, "derived contract contains no open percent-encoded party fragment"


@then("every top-level ODRL assigner and assignee still references the corresponding contractual party")
def step_then_rule_party_refs_resolve(context):
    document = _document(context)
    party_ids = {
        node.get("@id") for node in document.get("dcs:parties", []) if isinstance(node, dict) and node.get("@id")
    }
    refs = []
    for rule in _policy_rules(document):
        for side in ("odrl:assigner", "odrl:assignee"):
            value = rule.get(side)
            assert isinstance(value, dict) and value.get("@id"), f"rule {rule.get('@id')} has no {side} reference"
            refs.append(value["@id"])
    assert refs and set(refs) <= party_ids, f"ODRL party refs {set(refs)} do not resolve in dcs:parties {party_ids}"


@then('the originator is bound only to the "{role}" contractual party')
def step_then_originator_bound_only_to_role(context, role):
    document = _document(context)
    origin_nodes = [
        node
        for node in document.get("dcs:parties", [])
        if isinstance(node, dict) and str(node.get("@id", "")).startswith("did:web:")
    ]
    assert len(origin_nodes) == 1, f"expected exactly one DID-bound originator party, got {origin_nodes}"
    assert _party_role(origin_nodes[0]) == role, f"originator bound to {_party_role(origin_nodes[0])!r}, not {role!r}"


@then('the "{role}" contractual party remains open for the counterparty')
def step_then_other_role_remains_open(context, role):
    nodes = [
        node
        for node in _document(context).get("dcs:parties", [])
        if isinstance(node, dict) and _party_role(node) == role
    ]
    assert len(nodes) == 1, f"expected one open party for role {role!r}, got {nodes}"
    assert "#party-" in str(nodes[0].get("@id", "")), f"role {role!r} was already bound: {nodes[0]}"


@then("contract creation is rejected because originator_role is not a role declared by the template")
def step_then_unknown_originator_role_rejected(context):
    response = context.requests_response
    assert response.status_code == 400, f"expected HTTP 400 for unknown originator_role, got {response.status_code}: {response.text}"
    body = response.text.casefold()
    assert ("originator_role" in body or "originator role" in body) and (
        "template" in body or "declared" in body
    ), (
        f"rejection does not identify the undeclared originator_role: {response.text}"
    )
