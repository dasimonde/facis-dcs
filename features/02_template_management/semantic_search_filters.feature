@DCS-FR-TR-10 @DCS-IR-TR-02
Feature: Search templates by configured ontology fields

  @clean_db @REQ-ontology-configured-search-filters-AC3 @REQ-ontology-configured-search-filters-AC15 @REQ-ontology-configured-search-filters-AC18
  Scenario: Semantic filters and a system filter intersect by field identity
    Given I am authenticated with roles: "Template Manager"
    And templates exist with jurisdiction and industry field values
      | name                        | jurisdiction | industry |
      | BDD Semantic Search Match   | DEU          | Energy   |
      | BDD Semantic Search Other   | DEU          | Finance  |
      | BDD Semantic Search Swapped | AUT          | DEU      |
    When I search templates named "BDD Semantic Search" with semantic filters
      | label        | value  |
      | Jurisdiction | DEU    |
      | Industry     | eNeRg  |
    Then the result is one template where its name contains "BDD Semantic Search Match"
    When I search templates named "BDD Semantic Search Other" with semantic filters
      | label        | value  |
      | Jurisdiction | DEU    |
      | Industry     | eNeRg  |
    Then the semantic template search returns no results

  @clean_db @REQ-restore-ontology-field-catalog-AC5
  Scenario Outline: Jurisdiction matches the complete stored country code only
    Given I am authenticated with roles: "Template Manager"
    And templates exist with jurisdiction and industry field values
      | name                       | jurisdiction | industry |
      | BDD Country Search Germany | DEU          | Energy   |
      | BDD Country Search Austria | AUT          | Energy   |
    When I search templates named "BDD Country Search" with semantic filters
      | label        | value  |
      | Jurisdiction | <code> |
    Then the semantic template search returns exactly the templates
      | name       |
      | <template> |
    When I search templates named "BDD Country Search" with semantic filters
      | label        | value     |
      | Jurisdiction | <partial> |
    Then the semantic template search returns no results

    Examples:
      | code | partial | template                   |
      | DEU  | DE      | BDD Country Search Germany |
      | AUT  | AU      | BDD Country Search Austria |

  @clean_db @REQ-ontology-configured-search-filters-AC14
  Scenario: Scalar and JSON-LD literal values produce the same template search results
    Given I am authenticated with roles: "Template Manager"
    And templates exist with equivalent scalar and JSON-LD literal jurisdiction values
    When I search templates named "BDD Literal Search" with semantic filters
      | label        | value  |
      | Jurisdiction | DEU    |
    Then the semantic template search returns exactly the templates
      | name                       |
      | BDD Literal Search Scalar  |
      | BDD Literal Search JSON-LD |

  @clean_db @REQ-ontology-configured-search-filters-AC17
  Scenario: An absent value and an ODRL equality constraint do not count as filled values
    Given I am authenticated with roles: "Template Manager"
    And templates exist with filled, empty and ODRL-constrained jurisdiction fields
    When I search templates named "BDD Empty Search" with semantic filters
      | label        | value  |
      | Jurisdiction | DEU    |
    Then the semantic template search returns exactly the templates
      | name                    |
      | BDD Empty Search Filled |
