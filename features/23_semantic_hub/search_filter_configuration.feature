@DCS-FR-TR-10 @DCS-FR-TR-03
Feature: Ontology-backed search filter configuration

  @REQ-ontology-configured-search-filters-AC1 @REQ-ontology-configured-search-filters-AC5
  Scenario: A manager adds and renames a filter without changing its field mapping
    Given I am authenticated with roles: "Template Manager"
    And I retrieve the Semantic Hub search filter configuration
    And the retrieved search filter configuration is saved for comparison
    When I add and rename a search filter based on the Jurisdiction ontology field
    Then the added and renamed filter retains its identifier, field and source

  @REQ-ontology-configured-search-filters-AC1
  Scenario: A stale configuration revision cannot overwrite a concurrent edit
    Given I am authenticated with roles: "Template Manager"
    And I retrieve the Semantic Hub search filter configuration
    And the retrieved search filter configuration is saved for comparison
    When two managers save changes based on the same filter revision
    Then the stale filter configuration is rejected without overwriting the first change

  @REQ-ontology-configured-search-filters-AC6
  Scenario: A removed filter disappears and its identifier is rejected by local search endpoints
    Given I am authenticated with roles: "Template Manager"
    And I retrieve the Semantic Hub search filter configuration
    And the retrieved search filter configuration is saved for comparison
    When I add and remove a search filter based on the Jurisdiction ontology field
    Then the removed filter is absent from the configuration and cannot be searched

  @REQ-ontology-configured-search-filters-AC7
  Scenario: A freshly seeded Semantic Hub provides the four default field mappings
    Given I am authenticated with roles: "Template Manager"
    When I retrieve the Semantic Hub search filter configuration
    Then get http 200:Success code
    And the search filter configuration contains these labeled ontology fields
      | label                        | field_iri                                                                      |
      | Jurisdiction                 | https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction                |
      | Contract type                | https://w3id.org/facis/dcs/taxonomy/v1#field-contract-type                        |
      | Industry                     | https://w3id.org/facis/dcs/taxonomy/v1#field-contract-industry                    |
      | Regulatory compliance status | https://w3id.org/facis/dcs/taxonomy/v1#field-contract-regulatoryComplianceStatus |

  @REQ-ontology-configured-search-filters-AC11
  Scenario: A role without ontology-management permission cannot save search filters
    Given I am authenticated with roles: "Template Manager"
    And I retrieve the Semantic Hub search filter configuration
    And the retrieved search filter configuration is saved for comparison
    And I am authenticated with roles: "Template Creator"
    When I attempt to save the retrieved search filter configuration
    Then the request is denied with an authorization error
    When I retrieve the Semantic Hub search filter configuration as a Template Manager
    Then the search filter configuration is unchanged

  @REQ-ontology-configured-search-filters-AC13
  Scenario Outline: Invalid filter configuration is rejected atomically
    Given I am authenticated with roles: "Template Manager"
    And I retrieve the Semantic Hub search filter configuration
    And the retrieved search filter configuration is saved for comparison
    When I save a search filter configuration with <invalidity>
    Then the request is denied with a client error
    When I retrieve the Semantic Hub search filter configuration
    Then the search filter configuration is unchanged

    Examples:
      | invalidity       |
      | an empty label   |
      | an unknown field |
      | a duplicate id   |
