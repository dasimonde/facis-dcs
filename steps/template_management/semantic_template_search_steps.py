"""Steps for ontology-mapped template searches."""

from behave import given, then, when

from steps.support.services.search_filter_configuration_service import SearchFilterConfigurationService
from steps.support.services.semantic_template_search_service import SemanticTemplateSearchService


@given("templates exist with jurisdiction and industry field values")
def create_semantic_templates(context):
    response = SearchFilterConfigurationService.retrieve(context)
    assert response.status_code == 200, response.text
    filters = response.json()["filters"]
    context.semantic_filters_by_label = {item["label"]: item for item in filters}
    field_iris = {label: context.semantic_filters_by_label[label]["field_iri"] for label in ("Jurisdiction", "Industry")}
    for row in context.table:
        response = SemanticTemplateSearchService.create(context, row.as_dict(), field_iris)
        assert response.status_code == 200, response.text


@given("templates exist with equivalent scalar and JSON-LD literal jurisdiction values")
def create_equivalent_templates(context):
    response = SearchFilterConfigurationService.retrieve(context)
    assert response.status_code == 200, response.text
    context.semantic_filters_by_label = {item["label"]: item for item in response.json()["filters"]}
    field_iris = {label: context.semantic_filters_by_label[label]["field_iri"] for label in ("Jurisdiction", "Industry")}
    for suffix, literal in (("Scalar", False), ("JSON-LD", True)):
        row = {"name": f"BDD Literal Search {suffix}", "jurisdiction": "DEU", "industry": "Energy", "literal": literal}
        response = SemanticTemplateSearchService.create(context, row, field_iris)
        assert response.status_code == 200, response.text


@given("templates exist with filled, empty and ODRL-constrained jurisdiction fields")
def create_empty_and_constrained_templates(context):
    response = SearchFilterConfigurationService.retrieve(context)
    assert response.status_code == 200, response.text
    context.semantic_filters_by_label = {item["label"]: item for item in response.json()["filters"]}
    field_iris = {label: context.semantic_filters_by_label[label]["field_iri"] for label in ("Jurisdiction", "Industry")}
    for suffix, value, constraint in (("Filled", "DEU", False), ("Empty", None, False), ("Constraint", None, True)):
        row = {"name": f"BDD Empty Search {suffix}", "jurisdiction": value, "industry": "Energy", "constraint": constraint}
        response = SemanticTemplateSearchService.create(context, row, field_iris)
        assert response.status_code == 200, response.text


@when('I search templates named "{name}" with semantic filters')
def search_semantic_templates(context, name):
    filters = [
        {"filter_id": context.semantic_filters_by_label[row["label"]]["id"], "value": row["value"]}
        for row in context.table
    ]
    context.requests_response = SemanticTemplateSearchService.search(context, name, filters)


@then("the semantic template search returns no results")
def assert_no_semantic_templates(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    assert context.requests_response.json() == [], context.requests_response.text


@then("the semantic template search returns exactly the templates")
def assert_semantic_templates(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    actual = sorted(item["name"] for item in context.requests_response.json())
    expected = sorted(row["name"] for row in context.table)
    assert actual == expected, (actual, expected)
