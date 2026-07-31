Feature: Amendment
  Scenario: Amended PDFs are returned as incremental updates preserving original content
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:amendment-base",
        "@type": "ContractTemplate",
        "documentTitle": "Amendment Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Amendment Test Document"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:amendment-base#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:amendment-base#s1",
              "children": ["urn:doc:amendment-base#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:amendment-base#s1",
              "title": "1. Original Terms"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:amendment-base#c1",
              "content": ["Original clause established at document creation."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    And an amended semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:amendment-base",
        "@type": "ContractTemplate",
        "documentTitle": "Amendment Test Document",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "Amendment Test Document"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:amendment-base#s1", "urn:doc:amendment-base#s2"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:amendment-base#s1",
              "children": ["urn:doc:amendment-base#c1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:amendment-base#s2",
              "children": ["urn:doc:amendment-base#c2"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:amendment-base#s1",
              "title": "1. Original Terms"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:amendment-base#c1",
              "content": ["Original clause established at document creation."]
            },
            {
              "@type": "Section",
              "@id": "urn:doc:amendment-base#s2",
              "title": "2. Amendment"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:amendment-base#c2",
              "content": ["Amendment clause added to reflect updated terms."]
            }
          ]
        }
      }
      """
    When I update the compiled PDF with the amended payload through /update
    Then the response content type is "application/pdf"
    And the amended PDF is longer than the original
    And the amended PDF preserves the original bytes as a prefix
    And the amended PDF embeds the new JSON-LD payload
    And the response body contains "Amendment clause added to reflect updated terms."

  Scenario: Updating with an unchanged payload returns a conflict error
    Given the compiler service is running
    And a semantic payload:
      """
      {
        "@context": {
          "@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
          "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
          "xsd": "http://www.w3.org/2001/XMLSchema#"
        },
        "@id": "urn:doc:amendment-nochange",
        "@type": "ContractTemplate",
        "documentTitle": "No-Change Test",
        "metadata": {
          "@type": "TemplateMetadata",
          "title": "No-Change Test"
        },
        "documentStructure": {
          "@type": "DocumentStructure",
          "layout": [
            {
              "@type": "LayoutNode",
              "isRoot": true,
              "children": ["urn:doc:amendment-nochange#s1"]
            },
            {
              "@type": "LayoutNode",
              "@id": "urn:doc:amendment-nochange#s1",
              "children": ["urn:doc:amendment-nochange#c1"]
            }
          ],
          "blocks": [
            {
              "@type": "Section",
              "@id": "urn:doc:amendment-nochange#s1",
              "title": "1. Terms"
            },
            {
              "@type": "Clause",
              "@id": "urn:doc:amendment-nochange#c1",
              "content": ["This clause will not change."]
            }
          ]
        }
      }
      """
    And I compile the payload through /download
    When I update the compiled PDF with the same payload through /update
    Then the response status is 409
