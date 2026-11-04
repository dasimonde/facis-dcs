@DCS-FR-TR-10 @DCS-IR-TR-02
Feature: Search templates by configured ontology fields

  @clean_db @REQ-ontology-configured-search-filters-AC3 @REQ-ontology-configured-search-filters-AC15 @REQ-ontology-configured-search-filters-AC18
  Scenario: Semantic filters and a system filter intersect by field identity
    Given I am authenticated with roles: "Template Manager"
    And templates exist with jurisdiction and industry field values
      | name                        | jurisdiction | industry |
      | BDD Semantic Search Match   | Berlin       | Energy   |
      | BDD Semantic Search Other   | Berlin       | Finance  |
      | BDD Semantic Search Swapped | Energy       | Berlin   |
    When I search templates named "BDD Semantic Search" with semantic filters
      | label        | value  |
      | Jurisdiction | berlin |
      | Industry     | Energy |
    Then the result is one template where its name contains "BDD Semantic Search Match"
    When I search templates named "BDD Semantic Search Other" with semantic filters
      | label        | value  |
      | Jurisdiction | berlin |
      | Industry     | Energy |
    Then the semantic template search returns no results

  @clean_db @REQ-ontology-configured-search-filters-AC14
  Scenario: Scalar and JSON-LD literal values produce the same template search results
    Given I am authenticated with roles: "Template Manager"
    And templates exist with equivalent scalar and JSON-LD literal jurisdiction values
    When I search templates named "BDD Literal Search" with semantic filters
      | label        | value  |
      | Jurisdiction | Berlin |
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
      | Jurisdiction | Berlin |
    Then the semantic template search returns exactly the templates
      | name                    |
      | BDD Empty Search Filled |
