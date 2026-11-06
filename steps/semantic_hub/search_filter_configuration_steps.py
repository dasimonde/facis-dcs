"""Steps for the Semantic Hub search-filter configuration."""

from copy import deepcopy
from uuid import uuid4

from behave import given, then, when

from steps.support.services.auth_service import AuthService
from steps.support.services.search_filter_configuration_service import SearchFilterConfigurationService
from steps.support.services.semantic_template_search_service import SemanticTemplateSearchService


@when("I retrieve the Semantic Hub search filter configuration")
@given("I retrieve the Semantic Hub search filter configuration")
def retrieve_configuration(context):
    context.requests_response = SearchFilterConfigurationService.retrieve(context)


@then("the search filter configuration contains these labeled ontology fields")
def assert_default_fields(context):
    configuration = context.requests_response.json()
    filters = configuration["filters"]
    for row in context.table:
        matches = [item for item in filters if item["label"] == row["label"]]
        assert len(matches) == 1, f"Expected one filter for {row['label']!r}, got {matches!r}"
        item = matches[0]
        assert item["field_iri"] == row["field_iri"], item
        assert item.get("id"), item
        assert item.get("source", {}).get("name"), item
        assert item["source"].get("kind") == "ontology", item
        assert item["source"].get("version"), item
    assert configuration.get("revision") is not None, configuration


@given("the retrieved search filter configuration is saved for comparison")
def remember_configuration(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    context.search_filter_configuration = context.requests_response.json()


@when("I attempt to save the retrieved search filter configuration")
def save_configuration(context):
    context.requests_response = SearchFilterConfigurationService.save(
        context, context.search_filter_configuration
    )


@when("I save a search filter configuration with {invalidity}")
def save_invalid_configuration(context, invalidity):
    configuration = deepcopy(context.search_filter_configuration)
    assert configuration["filters"], "Expected a seeded filter to exercise validation"
    if invalidity == "an empty label":
        configuration["filters"][0]["label"] = ""
    elif invalidity == "an unknown field":
        configuration["filters"][0]["field_iri"] = "urn:facis:bdd:unknown-search-field"
    elif invalidity == "a duplicate id":
        configuration["filters"].append(deepcopy(configuration["filters"][0]))
    else:
        raise AssertionError(f"Unknown invalidity: {invalidity}")
    context.requests_response = SearchFilterConfigurationService.save(context, configuration)


@when("I add and rename a search filter based on the Jurisdiction ontology field")
def add_and_rename_filter(context):
    original = context.search_filter_configuration
    configuration = deepcopy(original)
    source_filter = next(item for item in configuration["filters"] if item["label"] == "Jurisdiction")
    added = deepcopy(source_filter)
    added["id"] = f"bdd-{uuid4()}"
    added["label"] = "BDD jurisdiction"
    configuration["filters"].append(added)
    try:
        response = SearchFilterConfigurationService.save(context, configuration)
        assert response.status_code == 200, response.text
        response = SearchFilterConfigurationService.retrieve(context)
        assert response.status_code == 200, response.text
        configuration = response.json()
        context.added_search_filter = deepcopy(
            next(item for item in configuration["filters"] if item["id"] == added["id"])
        )
        for item in configuration["filters"]:
            if item["id"] == added["id"]:
                item["label"] = "BDD renamed jurisdiction"
        response = SearchFilterConfigurationService.save(context, configuration)
        assert response.status_code == 200, response.text
        response = SearchFilterConfigurationService.retrieve(context)
        assert response.status_code == 200, response.text
        context.renamed_search_filter = next(item for item in response.json()["filters"] if item["id"] == added["id"])
        context.expected_search_filter = added
    finally:
        current = SearchFilterConfigurationService.retrieve(context)
        if current.status_code == 200:
            restored = deepcopy(current.json())
            restored["filters"] = [item for item in restored["filters"] if item["id"] != added["id"]]
            response = SearchFilterConfigurationService.save(context, restored)
            assert response.status_code == 200, f"Failed to remove BDD filter: {response.text}"


@then("the added and renamed filter retains its identifier, field and source")
def assert_renamed_filter(context):
    added = context.added_search_filter
    renamed = context.renamed_search_filter
    expected = context.expected_search_filter
    assert added["label"] == "BDD jurisdiction", added
    assert renamed["label"] == "BDD renamed jurisdiction", renamed
    for key in ("id", "field_iri", "source"):
        assert added[key] == renamed[key] == expected[key], (key, added, renamed)


@when("two managers save changes based on the same filter revision")
def save_conflicting_configuration(context):
    original = context.search_filter_configuration
    first = deepcopy(original)
    second = deepcopy(original)
    assert first["filters"], "Expected a seeded filter to exercise concurrent changes"
    first["filters"][0]["label"] = "BDD first edit"
    second["filters"][0]["label"] = "BDD stale edit"
    try:
        response = SearchFilterConfigurationService.save(context, first)
        assert response.status_code == 200, response.text
        context.stale_configuration_response = SearchFilterConfigurationService.save(context, second)
        response = SearchFilterConfigurationService.retrieve(context)
        assert response.status_code == 200, response.text
        context.configuration_after_conflict = response.json()
    finally:
        current = SearchFilterConfigurationService.retrieve(context)
        if current.status_code == 200:
            restored = current.json()
            restored["filters"] = original["filters"]
            response = SearchFilterConfigurationService.save(context, restored)
            assert response.status_code == 200, f"Failed to restore BDD configuration: {response.text}"


@then("the stale filter configuration is rejected without overwriting the first change")
def assert_conflict(context):
    response = context.stale_configuration_response
    assert response.status_code == 409, f"{response.status_code}: {response.text}"
    assert context.configuration_after_conflict["filters"][0]["label"] == "BDD first edit"


@when("I add and remove a search filter based on the Jurisdiction ontology field")
def add_and_remove_filter(context):
    configuration = deepcopy(context.search_filter_configuration)
    added = deepcopy(next(item for item in configuration["filters"] if item["label"] == "Jurisdiction"))
    added["id"] = f"bdd-{uuid4()}"
    added["label"] = "BDD removable jurisdiction"
    configuration["filters"].append(added)
    try:
        response = SearchFilterConfigurationService.save(context, configuration)
        assert response.status_code == 200, response.text
        response = SearchFilterConfigurationService.retrieve(context)
        assert response.status_code == 200, response.text
        configuration = response.json()
        assert any(item["id"] == added["id"] for item in configuration["filters"]), configuration
        configuration["filters"] = [item for item in configuration["filters"] if item["id"] != added["id"]]
        response = SearchFilterConfigurationService.save(context, configuration)
        assert response.status_code == 200, response.text
        context.removed_filter_id = added["id"]
    finally:
        current = SearchFilterConfigurationService.retrieve(context)
        if current.status_code == 200 and any(item["id"] == added["id"] for item in current.json()["filters"]):
            configuration = current.json()
            configuration["filters"] = [item for item in configuration["filters"] if item["id"] != added["id"]]
            response = SearchFilterConfigurationService.save(context, configuration)
            assert response.status_code == 200, f"Failed to remove BDD filter: {response.text}"


@then("the removed filter is absent from the configuration and cannot be searched")
def assert_removed_filter(context):
    response = SearchFilterConfigurationService.retrieve(context)
    assert response.status_code == 200, response.text
    assert all(item["id"] != context.removed_filter_id for item in response.json()["filters"])
    responses = SemanticTemplateSearchService.search_all_endpoints(
        context, [{"filter_id": context.removed_filter_id, "value": "Berlin"}]
    )
    for response in responses:
        assert response.status_code in (400, 422), f"{response.url}: {response.status_code}: {response.text}"


@when("I retrieve the Semantic Hub search filter configuration as a Template Manager")
def retrieve_as_manager(context):
    headers = AuthService.get_headers_for_roles(["Template Manager"])
    context.requests_response = SearchFilterConfigurationService.retrieve(context, headers=headers)


@then("the search filter configuration is unchanged")
def assert_unchanged(context):
    assert context.requests_response.status_code == 200, context.requests_response.text
    assert context.requests_response.json() == context.search_filter_configuration
