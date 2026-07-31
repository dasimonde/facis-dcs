Feature: Network runtime
  Scenario: The service exposes the documented machine and human interfaces
    Given the compiler service is running
    When I fetch "/ui/"
    Then the response status is 200
    And the response content type starts with "text/html"
    And the response body contains "SwaggerUIBundle"
    When I fetch "/swagger.json"
    Then the response status is 200
    And the response content type starts with "application/json"
    And the response body contains "/download"
    And the response body contains "/verify"

  Scenario: The service self-hosts ontology assets
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:network-runtime",
        "@type": "ContractTemplate",
        "documentTitle": "Hosted Ontology",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Hosted Ontology"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:network-runtime#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:network-runtime#s1",
              "children": ["urn:doc:network-runtime#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:network-runtime#s1",
              "title": "1. Runtime"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:network-runtime#c1",
              "content": ["The runtime serves its own structural ontology and patch metadata."]
            }
          ]
        }
      }
      """
    When I fetch "/index.html"
    Then the response status is 200
    And the response content type starts with "text/html"
    And the response body contains "SwaggerUIBundle"
    When I fetch "/ontology/dcs-pdf-core"
    Then the response status is 200
    And the response content type starts with "application/ld+json"
    And the response body contains "@context"
    When I fetch "/ontology/dcs-pdf-core.owl"
    Then the response status is 200
    And the response content type starts with "text/turtle"
    And the response body contains "@prefix"

  Scenario: The service rejects invalid content types
    Given the compiler service is running
    When I POST plain text to "/verify"
    Then the response status is 415
