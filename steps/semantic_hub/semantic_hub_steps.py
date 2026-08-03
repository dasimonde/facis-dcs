"""BDD steps for the Semantic Hub (DCS-FR-TR-03, UC-02-08,
backend/design/semantic_hub.go): versioned schema storage
(/semantic/schema/...), public context resolution (/semantic/context/...),
document anchoring (@context hub URL + sh:shapesGraph injected by the normalization layer), and
ontology-prefix enforcement at template creation."""

import json
import re

import requests as _requests
from behave import given, then, when

from steps.support.api_client import (
    contract_retrieve_by_id_url,
    get_with_headers,
    hub_shapes_anchors,
    pac_audit_timeline,
    post_json,
    template_create_url,
)
from steps.support.services.auth_service import AuthService
from steps.support.services.contract_service import ContractService
from steps.support.services.template_service import TemplateService


def _hub_url(context, path: str) -> str:
    return f"{context.base_url}{path}"


@when('the active "{kind}" schema "{name}" is retrieved from the Semantic Hub')
def step_when_retrieve_schema(context, kind, name):
    context.requests_response = _requests.get(
        _hub_url(context, "/semantic/schema/retrieve"),
        params={"name": name, "kind": kind},
        timeout=context.http_timeout_seconds,
    )


@then('the retrieved schema is version {version:d}, active, of kind "{kind}"')
def step_then_schema_version(context, version, kind):
    body = context.requests_response.json()
    assert body.get("version") == version, f"Expected version {version}, got: {body.get('version')}"
    assert body.get("active") is True, f"Expected the retrieved schema to be active: {body}"
    assert body.get("kind") == kind, f"Expected kind {kind!r}, got: {body.get('kind')!r}"
    assert body.get("content"), "Expected non-empty schema content"


@then('the retrieved schema content declares the "{prefix}" ontology IRI "{iri}"')
def step_then_schema_declares_iri(context, prefix, iri):
    content = json.loads(context.requests_response.json().get("content"))
    declared = (content.get("@context") or {}).get(prefix)
    assert declared == iri, f"Expected @context.{prefix} == {iri!r}, got: {declared!r}"


@when('the ontology "{name}" is resolved from the Semantic Hub without authentication')
def step_when_resolve_ontology(context, name):
    context.requests_response = _requests.get(
        _hub_url(context, f"/semantic/ontology/{name}"),
        timeout=context.http_timeout_seconds,
    )


@then(
    'the ontology defines at least {minimum:d} labeled value catalogs with labeled concepts for ODRL left operand "{operand}"'
)
def step_then_ontology_defines_grouped_value_catalogs(context, minimum, operand):
    turtle = context.requests_response.json()["content"]
    statements = re.findall(
        r"(?ms)^([^\s@#]\S*)\s+(.+?\s\.)\s*(?=^[^\s@#]|\Z)",
        "\n".join(line for line in turtle.splitlines() if not line.lstrip().startswith("#")),
    )

    catalogs = {
        catalog
        for _, body in statements
        if "a dcs:ValueConstraint" in body
        and f"dcs:odrlLeftOperand odrl:{operand}" in body
        for catalog in re.findall(r"dcs:valueCatalog\s+(\S+)\s*[;.]", body)
    }
    labeled_schemes = {
        subject
        for subject, body in statements
        if "a skos:ConceptScheme" in body and "skos:prefLabel" in body
    }
    schemes_with_labeled_concepts = {
        scheme
        for _, body in statements
        if "a skos:Concept" in body and "skos:prefLabel" in body
        for scheme in re.findall(r"skos:inScheme\s+(\S+)\s*[;.]", body)
    }
    complete_catalogs = catalogs & labeled_schemes & schemes_with_labeled_concepts

    assert len(complete_catalogs) >= minimum, (
        f"Expected at least {minimum} labeled catalogs with labeled concepts for odrl:{operand}, "
        f"got {sorted(complete_catalogs)} from mapped catalogs {sorted(catalogs)}"
    )


@when('the JSON-LD context "{name}" is resolved from the Semantic Hub without authentication')
def step_when_resolve_context(context, name):
    context.requests_response = _requests.get(
        _hub_url(context, f"/semantic/context/{name}"),
        timeout=context.http_timeout_seconds,
    )


@then('the resolved document carries a JSON-LD "@context" object')
def step_then_resolved_context(context):
    body = context.requests_response.json()
    assert isinstance(body.get("@context"), dict) and body["@context"], (
        f"Expected the resolved document to carry a non-empty @context object, got: {body}"
    )


@when('the Template Manager registers a new active version of the "{kind}" schema "{name}" extending the genesis context')
def step_when_register_schema(context, kind, name):
    # Fetch the genesis content and extend it — a plausible new version, not
    # a fabricated unrelated document. The hub's registered versions persist
    # across suite runs (the BDD deployment is not re-seeded per run), so
    # remember how many versions existed beforehand and assert RELATIVE to
    # that, never against absolute version numbers.
    before = _requests.get(
        _hub_url(context, "/semantic/schema/versions"),
        params={"name": name, "kind": kind},
        timeout=context.http_timeout_seconds,
    )
    assert before.status_code == 200, f"versions listing failed: {before.status_code} {before.text}"
    context.hub_versions_before = [v["version"] for v in before.json()]

    # Self-heal even when a later step of this scenario fails: the genesis
    # scenario (and production behavior) expects version 1 active, so restore
    # it unconditionally at scenario teardown — otherwise one mid-scenario
    # failure leaves the extension version active and cascades into the next
    # suite run.
    def _restore_genesis_active():
        _requests.post(
            _hub_url(context, "/semantic/schema/rollback"),
            json={"name": name, "kind": kind, "version": 1},
            headers=AuthService.get_headers_for_roles(["Template Manager"]),
            timeout=context.http_timeout_seconds,
        )

    context.add_cleanup(_restore_genesis_active)

    genesis = _requests.get(
        _hub_url(context, "/semantic/schema/retrieve"),
        params={"name": name, "kind": kind, "version": 1},
        timeout=context.http_timeout_seconds,
    )
    assert genesis.status_code == 200, f"could not fetch genesis schema: {genesis.text}"
    content = json.loads(genesis.json()["content"])
    content["@context"]["bddExt"] = "https://example.org/bdd-extension#"
    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/register"),
        {
            "name": name,
            "kind": kind,
            "media_type": "application/ld+json",
            "content": json.dumps(content),
            "activate": True,
        },
        headers=headers,
    )


@then("the schema registration reports a version above the genesis version as active")
def step_then_registration_version(context):
    body = context.requests_response.json()
    expected = max(context.hub_versions_before) + 1
    assert body.get("version") == expected, (
        f"Expected the registration to mint version {expected} "
        f"(pre-existing: {sorted(context.hub_versions_before)}), got: {body}"
    )
    assert body.get("active") is True, f"Expected the registered version to be active: {body}"
    context.hub_registered_version = body["version"]


@then('the Semantic Hub lists the registered version of the "{kind}" schema "{name}" as the single active one')
def step_then_versions_listing_registered_active(context, kind, name):
    _assert_versions_listing(
        context, kind, name,
        expect_active=context.hub_registered_version,
        expect_count=len(context.hub_versions_before) + 1,
    )


@then('the Semantic Hub lists version {active:d} of the "{kind}" schema "{name}" as the single active one')
def step_then_versions_listing_absolute_active(context, active, kind, name):
    _assert_versions_listing(
        context, kind, name,
        expect_active=active,
        expect_count=len(context.hub_versions_before) + 1,
    )


def _assert_versions_listing(context, kind, name, *, expect_active, expect_count):
    resp = _requests.get(
        _hub_url(context, "/semantic/schema/versions"),
        params={"name": name, "kind": kind},
        timeout=context.http_timeout_seconds,
    )
    assert resp.status_code == 200, f"versions listing failed: {resp.status_code} {resp.text}"
    versions = resp.json()
    assert len(versions) == expect_count, (
        f"Expected {expect_count} versions, got: {[v.get('version') for v in versions]}"
    )
    actives = [v["version"] for v in versions if v.get("active")]
    assert actives == [expect_active], f"Expected exactly version {expect_active} active, got: {actives}"


@when('the Template Manager rolls the "{kind}" schema "{name}" back to version {version:d}')
def step_when_rollback(context, kind, name, version):
    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/rollback"),
        {"name": name, "kind": kind, "version": version},
        headers=headers,
    )


@when("I attempt to register a Semantic Hub schema version with my current role")
def step_when_attempt_register(context):
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/register"),
        {
            "name": "bdd-unauthorized",
            "kind": "profile",
            "media_type": "text/plain",
            "content": "unauthorized",
        },
        headers=getattr(context, "headers", {}),
    )


@then('the contract "{name}" carries a Semantic Hub schema anchor')
def step_then_contract_anchored(context, name):
    did, _ = ContractService._contract_data(context, name)
    headers = context.contract_seed_headers[name]
    retrieve = get_with_headers(context, contract_retrieve_by_id_url(context, did), headers=headers)
    assert retrieve.status_code == 200, retrieve.text
    contract_data = retrieve.json().get("contract_data") or {}
    # The context anchor IS the document's @context (or the string entry of
    # its array form) — standard JSON-LD, resolvable by external tooling.
    raw_context = contract_data.get("@context")
    candidates = raw_context if isinstance(raw_context, list) else [raw_context]
    anchor = next(
        (c for c in candidates if isinstance(c, str) and "/semantic/context/" in c),
        None,
    )
    assert anchor, (
        f"Expected @context to carry a hub-served versioned context URL, got: {raw_context}"
    )
    # The shapes pin rides on sh:shapesGraph (ADR-8).
    shapes_anchors = hub_shapes_anchors(contract_data)
    assert shapes_anchors, (
        f"Expected a hub-served sh:shapesGraph anchor, got: {contract_data.get('sh:shapesGraph')}"
    )
    context.hub_anchor_url = anchor


@then('the contract "{name}"\'s JSON-LD context anchor resolves to the hub\'s registered context')
def step_then_anchor_resolves(context, name):
    anchor = context.hub_anchor_url
    # DCS_PUBLIC_URL is unset in the BDD deployment (same convention as the
    # C2PA remote_manifests field), so the anchor is API-root-relative.
    url = anchor if anchor.startswith("http") else f"{context.base_url}{anchor}"
    resp = _requests.get(url, timeout=context.http_timeout_seconds)
    assert resp.status_code == 200, (
        f"Expected the schema anchor {anchor!r} to resolve against the Semantic Hub, got "
        f"{resp.status_code}: {resp.text}"
    )
    body = resp.json()
    assert isinstance(body.get("@context"), dict), (
        f"Expected the resolved anchor to be the registered JSON-LD context, got: {body}"
    )


@when('a template is created whose "@context" redefines the "{prefix}" prefix to "{iri}"')
def step_when_create_conflicting_template(context, prefix, iri):
    doc = TemplateService.canonical_document_data("BDD Conflicting Ontology Template")
    doc["@context"][prefix] = iri
    headers = AuthService.get_headers_for_roles(["Template Creator"])
    context.requests_response = post_json(
        context,
        template_create_url(context),
        {
            "template_type": TemplateService.template_type_for_category("legal"),
            "name": "BDD Conflicting Ontology Template",
            "description": "must be rejected",
            "template_data": doc,
        },
        headers=headers,
    )


def _external_context_template_payload(iri: str) -> dict:
    doc = TemplateService.canonical_document_data("BDD External Context Template")
    doc["@context"] = [doc["@context"], iri]
    return {
        "template_type": TemplateService.template_type_for_category("legal"),
        "name": "BDD External Context Template",
        "description": "references an externally anchored context",
        "template_data": doc,
    }


@when('a template is created whose "@context" references the external context "{iri}"')
def step_when_create_external_context_template(context, iri):
    headers = AuthService.get_headers_for_roles(["Template Creator"])
    context.requests_response = post_json(
        context,
        template_create_url(context),
        _external_context_template_payload(iri),
        headers=headers,
    )


@when('the Template Manager registers the external JSON-LD context "{iri}" in the Semantic Hub')
def step_when_register_external_context(context, iri):
    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/register"),
        {
            "name": iri,
            "kind": "context",
            "media_type": "application/ld+json",
            "content": json.dumps({"@context": {"ex": "https://example.org/ns#"}}),
            "activate": True,
        },
        headers=headers,
    )


@then("the rejection names the Semantic Hub's active context")
def step_then_rejection_names_hub(context):
    assert "Semantic Hub" in context.requests_response.text, (
        f"Expected the rejection to name the Semantic Hub's active context, got: "
        f"{context.requests_response.text}"
    )


# Phase 1 / ADR-8: enforcement (AuditContractContent) now reads its SHACL
# shapes from the hub's ACTIVE (or, for revalidation, PINNED) version rather
# than a fixed disk file — proving activate/rollback actually changes what
# gets enforced (steps/semantic_hub/semantic_hub.feature "Activating a
# stricter SHACL shapes version..."). Validation itself runs on goRDFlib, a
# conformant SHACL-core processor (ADR-9) — it reports only non-conformance,
# so a passing contract has no finding for a rule at all, not an "info" one.

_BDD_STRICT_TITLE_IN_VALUE = "IMPOSSIBLE-BDD-TITLE-VALUE-NO-CONTRACT-HAS-THIS"


@when('the Template Manager registers a stricter version of the "shapes" schema "facis-dcs" that narrows the canonical contract title')
def step_when_register_stricter_shapes(context):
    name, kind = "facis-dcs", "shapes"

    before = _requests.get(
        _hub_url(context, "/semantic/schema/versions"),
        params={"name": name, "kind": kind},
        timeout=context.http_timeout_seconds,
    )
    assert before.status_code == 200, f"versions listing failed: {before.status_code} {before.text}"
    context.hub_versions_before = [v["version"] for v in before.json()]

    def _restore_genesis_active():
        _requests.post(
            _hub_url(context, "/semantic/schema/rollback"),
            json={"name": name, "kind": kind, "version": 1},
            headers=AuthService.get_headers_for_roles(["Template Manager"]),
            timeout=context.http_timeout_seconds,
        )

    context.add_cleanup(_restore_genesis_active)

    genesis = _requests.get(
        _hub_url(context, "/semantic/schema/retrieve"),
        params={"name": name, "kind": kind, "version": 1},
        timeout=context.http_timeout_seconds,
    )
    assert genesis.status_code == 200, f"could not fetch genesis shapes: {genesis.text}"
    ttl = genesis.json()["content"]

    # dcs:ContractMetadataShape's dcs:title property (see
    # docs/semantic-ontology/shapes/facis-dcs-contract-canonical-shapes.ttl)
    # requires xsd:string + minCount 1 today. Adding an sh:in restriction no
    # real contract title satisfies turns "no finding" into a real SHACL
    # sh:in violation (rule ID "title-InConstraintComponent" — goRDFlib rule
    # IDs are <path local name>-<constraint component local name>).
    anchor = "sh:path dcs:title ;"
    assert anchor in ttl, f"Expected the genesis shapes to declare {anchor!r}, got:\n{ttl}"
    stricter_ttl = ttl.replace(
        anchor,
        f'{anchor}\n    sh:in ( "{_BDD_STRICT_TITLE_IN_VALUE}" ) ;',
        1,
    )
    assert stricter_ttl != ttl, "Expected the sh:in injection to change the shapes content"

    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = post_json(
        context,
        _hub_url(context, "/semantic/schema/register"),
        {
            "name": name,
            "kind": kind,
            "media_type": "text/turtle",
            "content": stricter_ttl,
            "activate": True,
        },
        headers=headers,
    )


def _content_audit_trail_rule_severities(context, name, rule_id):
    assert context.requests_response.status_code == 200, (
        f"Expected 200 from /pac/audit, got {context.requests_response.status_code}: "
        f"{context.requests_response.text}"
    )
    did, _ = ContractService._contract_data(context, name)
    timeline = pac_audit_timeline(context.requests_response)
    entries = [entry for entry in timeline if entry.get("did") == did]
    assert entries, (
        f"Expected a contract-content audit trail entry for '{name}' (did={did}), "
        f"got DIDs: {sorted({str(entry.get('did')) for entry in timeline})}"
    )
    severities = []
    for entry in entries:
        if entry.get("event_type") != "CONTRACT_CONTENT_POLICY_AUDIT_FINDING":
            continue
        event_data = entry.get("event_data")
        if isinstance(event_data, str):
            event_data = json.loads(event_data)
        if event_data.get("ruleId") == rule_id:
            severities.append(event_data.get("severity"))
    return did, severities


@then('the contract content audit trail for "{name}" reports rule "{rule_id}" with severity "{severity}"')
def step_then_content_audit_trail_reports_rule(context, name, rule_id, severity):
    did, severities = _content_audit_trail_rule_severities(context, name, rule_id)
    assert severity in severities, (
        f"Expected contract '{name}' (did={did}) to report rule {rule_id!r} with severity "
        f"{severity!r}, got severities: {severities}"
    )


@then('the contract content audit trail for "{name}" does not report an error for rule "{rule_id}"')
def step_then_content_audit_trail_no_error_for_rule(context, name, rule_id):
    # goRDFlib (ADR-9) only reports non-conformance — a fully compliant
    # contract has NO finding for a conformant rule at all (not an "info"
    # one), so this asserts absence-of-violation rather than a specific
    # passing severity.
    did, severities = _content_audit_trail_rule_severities(context, name, rule_id)
    assert "error" not in severities, (
        f"Expected contract '{name}' (did={did}) to report no error for rule {rule_id!r}, "
        f"got severities: {severities}"
    )


# Phase 3 / ADR-10: the clause catalog endpoint (backend/design/
# semantic_hub.go "clauses") serves a pre-digested form-schema derived
# server-side from the hub's clause-catalog SHACL shapes
# (backend/internal/semantichub/clausecatalog.go) — the same shapes
# validateAgainstHubShapes concatenates into contract validation.


@when("the Semantic Hub clause catalog is requested without authentication")
def step_when_request_clause_catalog(context):
    context.requests_response = _requests.get(
        _hub_url(context, "/semantic/clauses"),
        timeout=context.http_timeout_seconds,
    )


@then('the clause catalog lists a "{clause_type}" clause type whose shape declares "{properties}"')
def step_then_clause_catalog_lists_type(context, clause_type, properties):
    body = context.requests_response.json()
    clauses = body.get("clauses") or []
    matching = next((c for c in clauses if c.get("type") == clause_type), None)
    assert matching is not None, (
        f"Expected the clause catalog to list clause type {clause_type!r}, got types: "
        f"{[c.get('type') for c in clauses]}"
    )
    assert matching.get("shape"), f"Expected a NodeShape IRI on the listing, got: {matching}"
    # The form is generated client-side (shacl-form) from the response's raw
    # shapes Turtle — assert the declared property paths are present there.
    # behave's {properties} capture keeps the INNER quotes of a
    # '"a", "b", "c"' list (only the outermost pair belongs to the step
    # pattern) — strip them per item.
    shapes = body.get("shapes") or ""
    for path in (p.strip().strip('"') for p in properties.split(",")):
        assert f"sh:path {path}" in shapes, (
            f"Expected the raw shapes to declare 'sh:path {path}', got:\n{shapes[:800]}"
        )


@then("the clause catalog response carries the raw SHACL shapes it was derived from")
def step_then_clause_catalog_carries_shapes(context):
    body = context.requests_response.json()
    shapes = body.get("shapes") or ""
    assert "sh:NodeShape" in shapes and "PaymentClause" in shapes, (
        f"Expected the clause catalog response to carry the raw SHACL shapes, got: {shapes[:200]!r}"
    )
